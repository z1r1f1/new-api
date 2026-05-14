package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/codex"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func normalizeCodexUsagePlanType(value any) string {
	planType := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	switch planType {
	case "free", "plus", "pro", "team", "enterprise":
		return planType
	default:
		return ""
	}
}

func stringFieldFromMap(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func mapFieldFromMap(data map[string]interface{}, key string) map[string]interface{} {
	if data == nil {
		return nil
	}
	value, ok := data[key]
	if !ok || value == nil {
		return nil
	}
	nested, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return nested
}

func extractCodexUsagePlanType(payload any) string {
	data, ok := payload.(map[string]interface{})
	if !ok {
		return ""
	}
	if planType := normalizeCodexUsagePlanType(stringFieldFromMap(data, "plan_type")); planType != "" {
		return planType
	}
	if planType := normalizeCodexUsagePlanType(stringFieldFromMap(mapFieldFromMap(data, "rate_limit"), "plan_type")); planType != "" {
		return planType
	}
	return ""
}

func persistCodexChannelAccountType(ch *model.Channel, planType string) {
	normalizedPlanType := normalizeCodexUsagePlanType(planType)
	if ch == nil || normalizedPlanType == "" {
		return
	}
	otherInfo := ch.GetOtherInfo()
	if strings.TrimSpace(fmt.Sprint(otherInfo["codex_account_type"])) == normalizedPlanType {
		return
	}
	otherInfo["codex_account_type"] = normalizedPlanType
	otherInfo["codex_account_type_updated_at"] = common.GetTimestamp()
	encoded, err := common.Marshal(otherInfo)
	if err != nil {
		common.SysError("failed to marshal codex account type: " + err.Error())
		return
	}
	ch.OtherInfo = string(encoded)
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("other_info", ch.OtherInfo).Error; err != nil {
		common.SysError("failed to persist codex account type: " + err.Error())
	}
}

func getAutoTeamAPIURL() string {
	apiURL := strings.TrimSpace(os.Getenv("AUTOTEAM_API_URL"))
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8788"
	}
	return strings.TrimRight(apiURL, "/")
}

func getAutoTeamAPIKey() string {
	apiKey := strings.TrimSpace(os.Getenv("AUTOTEAM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("AUTOTEAM_TOKEN"))
	}
	return apiKey
}

func callAutoTeamRedo(ctx context.Context, path string, requestBody []byte) (bool, string, int, any, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		getAutoTeamAPIURL()+path,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return false, "", 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey := getAutoTeamAPIKey(); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return false, "", resp.StatusCode, nil, err
	}

	var payload any
	if len(body) > 0 && common.Unmarshal(body, &payload) != nil {
		payload = string(body)
	}

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	message := ""
	if data, ok := payload.(map[string]interface{}); ok {
		message = strings.TrimSpace(fmt.Sprint(data["message"]))
		if message == "<nil>" {
			message = ""
		}
	}
	if !ok && message == "" {
		message = fmt.Sprintf("AutoTeam status: %d", resp.StatusCode)
	}
	return ok, message, resp.StatusCode, payload, nil
}

func RedoChannelAutoTeamOAuth(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ch, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	if ch.Type != constant.ChannelTypeCodex && ch.Type != constant.ChannelTypeChatGPTImage {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not supported"})
		return
	}
	if ch.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}

	email := strings.TrimSpace(ch.Name)
	if email == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel name is required"})
		return
	}

	path := "/api/accounts/login"
	requestPayload := gin.H{"email": email}
	if ch.Type == constant.ChannelTypeChatGPTImage {
		path = "/api/accounts/redo-chatgpt-tokens"
		requestPayload = gin.H{"emails": []string{email}}
	}

	requestBody, err := common.Marshal(requestPayload)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if ch.Type == constant.ChannelTypeChatGPTImage {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			ok, message, statusCode, _, err := callAutoTeamRedo(ctx, path, requestBody)
			if err != nil {
				common.SysError("failed to call autoteam redo chatgpt: " + err.Error())
				return
			}
			if !ok {
				common.SysError(fmt.Sprintf("autoteam redo chatgpt failed: status=%d message=%s", statusCode, message))
			}
		}()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "ChatGPT redo task submitted",
			"async":   true,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	ok, message, statusCode, payload, err := callAutoTeamRedo(ctx, path, requestBody)
	if err != nil {
		common.SysError("failed to call autoteam redo oauth: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "调用 AutoTeam 重做 OAuth 失败，请检查服务状态"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":         ok,
		"message":         message,
		"upstream_status": statusCode,
		"data":            payload,
	})
}

func GetCodexChannelUsage(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ch, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	if ch.Type != constant.ChannelTypeCodex {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not Codex"})
		return
	}
	if ch.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}

	oauthKey, err := codex.ParseOAuthKey(strings.TrimSpace(ch.Key))
	if err != nil {
		common.SysError("failed to parse oauth key: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "解析凭证失败，请检查渠道配置"})
		return
	}
	accessToken := strings.TrimSpace(oauthKey.AccessToken)
	accountID := strings.TrimSpace(oauthKey.AccountID)
	if accessToken == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex channel: access_token is required"})
		return
	}
	if accountID == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "codex channel: account_id is required"})
		return
	}

	client, err := service.NewProxyHttpClient(ch.GetSetting().Proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	statusCode, body, err := service.FetchCodexWhamUsage(ctx, client, ch.GetBaseURL(), accessToken, accountID)
	if err != nil {
		common.SysError("failed to fetch codex usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取用量信息失败，请稍后重试"})
		return
	}

	if (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) && strings.TrimSpace(oauthKey.RefreshToken) != "" {
		refreshCtx, refreshCancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer refreshCancel()

		res, refreshErr := service.RefreshCodexOAuthTokenWithProxy(refreshCtx, oauthKey.RefreshToken, ch.GetSetting().Proxy)
		if refreshErr == nil {
			oauthKey.AccessToken = res.AccessToken
			oauthKey.RefreshToken = res.RefreshToken
			oauthKey.LastRefresh = time.Now().Format(time.RFC3339)
			oauthKey.Expired = res.ExpiresAt.Format(time.RFC3339)
			if strings.TrimSpace(oauthKey.Type) == "" {
				oauthKey.Type = "codex"
			}

			encoded, encErr := common.Marshal(oauthKey)
			if encErr == nil {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("key", string(encoded)).Error
				model.InitChannelCache()
				service.ResetProxyClientCache()
			}

			ctx2, cancel2 := context.WithTimeout(c.Request.Context(), 15*time.Second)
			defer cancel2()
			statusCode, body, err = service.FetchCodexWhamUsage(ctx2, client, ch.GetBaseURL(), oauthKey.AccessToken, accountID)
			if err != nil {
				common.SysError("failed to fetch codex usage after refresh: " + err.Error())
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取用量信息失败，请稍后重试"})
				return
			}
		}
	}

	var payload any
	if common.Unmarshal(body, &payload) != nil {
		payload = string(body)
	}

	ok := statusCode >= 200 && statusCode < 300
	if ok {
		persistCodexChannelAccountType(ch, extractCodexUsagePlanType(payload))
	}
	resp := gin.H{
		"success":         ok,
		"message":         "",
		"upstream_status": statusCode,
		"data":            payload,
	}
	if !ok {
		resp["message"] = fmt.Sprintf("upstream status: %d", statusCode)
	}
	c.JSON(http.StatusOK, resp)
}
