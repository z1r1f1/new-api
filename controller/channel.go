package controller

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/chatgptimg"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/ollama"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OpenAIModel struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	Created    int64          `json:"created"`
	OwnedBy    string         `json:"owned_by"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Permission []struct {
		ID                 string `json:"id"`
		Object             string `json:"object"`
		Created            int64  `json:"created"`
		AllowCreateEngine  bool   `json:"allow_create_engine"`
		AllowSampling      bool   `json:"allow_sampling"`
		AllowLogprobs      bool   `json:"allow_logprobs"`
		AllowSearchIndices bool   `json:"allow_search_indices"`
		AllowView          bool   `json:"allow_view"`
		AllowFineTuning    bool   `json:"allow_fine_tuning"`
		Organization       string `json:"organization"`
		Group              string `json:"group"`
		IsBlocking         bool   `json:"is_blocking"`
	} `json:"permission"`
	Root   string `json:"root"`
	Parent string `json:"parent"`
}

type OpenAIModelsResponse struct {
	Data    []OpenAIModel `json:"data"`
	Success bool          `json:"success"`
}

func parseStatusFilter(statusParam string) int {
	switch strings.ToLower(statusParam) {
	case "enabled", "1":
		return common.ChannelStatusEnabled
	case "disabled", "0":
		return 0
	default:
		return -1
	}
}

func parseChannelStatusCodeFilter(statusCodeParam string) (int, bool) {
	statusCode, err := strconv.Atoi(strings.TrimSpace(statusCodeParam))
	if err != nil || statusCode < 100 || statusCode > 599 {
		return 0, false
	}
	return statusCode, true
}

func applyChannelStatusCodeQuery(query *gorm.DB, statusCode int, ok bool) *gorm.DB {
	if query == nil || !ok {
		return query
	}
	compactPattern := fmt.Sprintf("%%\"last_status_code\":%d%%", statusCode)
	spacedPattern := fmt.Sprintf("%%\"last_status_code\": %d%%", statusCode)
	return query.Where("(other_info LIKE ? OR other_info LIKE ?)", compactPattern, spacedPattern)
}

func channelMatchesStatusCode(channel *model.Channel, statusCode int, ok bool) bool {
	if !ok {
		return true
	}
	if channel == nil || strings.TrimSpace(channel.OtherInfo) == "" {
		return false
	}
	otherInfo := make(map[string]interface{})
	if err := common.Unmarshal([]byte(channel.OtherInfo), &otherInfo); err != nil {
		return false
	}
	lastStatusCode, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(otherInfo["last_status_code"])))
	return err == nil && lastStatusCode == statusCode
}

func filterChannelsByStatusCode(channels []*model.Channel, statusCode int, ok bool) []*model.Channel {
	if !ok {
		return channels
	}
	filtered := make([]*model.Channel, 0, len(channels))
	for _, ch := range channels {
		if channelMatchesStatusCode(ch, statusCode, ok) {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}

func clearChannelInfo(channel *model.Channel) {
	if channel.ChannelInfo.IsMultiKey {
		channel.ChannelInfo.MultiKeyDisabledReason = nil
		channel.ChannelInfo.MultiKeyDisabledTime = nil
	}
}

func codexAccountChannelPatterns(account string) (string, string, bool) {
	normalized := normalizeCodexUsagePlanType(account)
	if normalized == "" {
		return "", "", false
	}
	return fmt.Sprintf("%%\"codex_account_type\":\"%s\"%%", normalized),
		fmt.Sprintf("%%\"plan_type\":\"%s\"%%", normalized),
		true
}

func applyCodexAccountChannelQuery(query *gorm.DB, account string) *gorm.DB {
	accountPattern, planPattern, ok := codexAccountChannelPatterns(account)
	if !ok {
		return query
	}
	return query.
		Where("type = ?", constant.ChannelTypeCodex).
		Where("(LOWER(other_info) LIKE ? OR LOWER(other_info) LIKE ?)", accountPattern, planPattern)
}

func channelMatchesCodexAccount(channel *model.Channel, account string) bool {
	normalized := normalizeCodexUsagePlanType(account)
	if normalized == "" {
		return true
	}
	if channel == nil || channel.Type != constant.ChannelTypeCodex {
		return false
	}
	if channel.OtherInfo == "" {
		return false
	}
	otherInfo := make(map[string]interface{})
	if err := common.Unmarshal([]byte(channel.OtherInfo), &otherInfo); err != nil {
		return false
	}
	return normalizeCodexUsagePlanType(otherInfo["codex_account_type"]) == normalized ||
		normalizeCodexUsagePlanType(otherInfo["plan_type"]) == normalized
}

func filterChannelsByCodexAccount(channels []*model.Channel, account string) []*model.Channel {
	if normalizeCodexUsagePlanType(account) == "" {
		return channels
	}
	filtered := make([]*model.Channel, 0, len(channels))
	for _, ch := range channels {
		if channelMatchesCodexAccount(ch, account) {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}

func applyChannelStatusFilter(query *gorm.DB, statusFilter int) *gorm.DB {
	if statusFilter == common.ChannelStatusEnabled {
		return query.Where("status = ?", common.ChannelStatusEnabled)
	}
	if statusFilter == 0 {
		return query.Where("status != ?", common.ChannelStatusEnabled)
	}
	return query
}

func buildChannelListQuery(group string, statusFilter int, typeFilter int, codexAccount string, statusCode int, hasStatusCode bool) *gorm.DB {
	query := model.DB.Model(&model.Channel{})
	query = model.ApplyChannelGroupFilter(query, group)
	query = applyChannelStatusFilter(query, statusFilter)
	if typeFilter >= 0 {
		query = query.Where("type = ?", typeFilter)
	}
	query = applyCodexAccountChannelQuery(query, codexAccount)
	query = applyChannelStatusCodeQuery(query, statusCode, hasStatusCode)
	return query
}

func GetAllChannels(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	channelData := make([]*model.Channel, 0)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	sortOptions := model.NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort)
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	groupFilter := model.NormalizeChannelGroupFilter(c.Query("group"))
	statusParam := c.Query("status")
	statusCodeFilter, hasStatusCodeFilter := parseChannelStatusCodeFilter(c.Query("status_code"))
	codexAccount := c.Query("codex_account")
	// statusFilter: -1 all, 1 enabled, 0 disabled (include auto & manual)
	statusFilter := parseStatusFilter(statusParam)
	// type filter
	typeStr := c.Query("type")
	typeFilter := -1
	if typeStr != "" {
		if t, err := strconv.Atoi(typeStr); err == nil {
			typeFilter = t
		}
	}

	var total int64

	if enableTagMode {
		tags, err := model.GetPaginatedChannelTags(buildChannelListQuery(groupFilter, statusFilter, typeFilter, codexAccount, statusCodeFilter, hasStatusCodeFilter), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
		if err != nil {
			common.SysError("failed to get paginated tags: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取标签失败，请稍后重试"})
			return
		}
		total, err = model.CountChannelTags(buildChannelListQuery(groupFilter, statusFilter, typeFilter, codexAccount, statusCodeFilter, hasStatusCodeFilter))
		if err != nil {
			common.SysError("failed to count tags: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取标签数量失败，请稍后重试"})
			return
		}
		for _, tag := range tags {
			if tag == nil || *tag == "" {
				continue
			}
			var tagChannels []*model.Channel
			err := sortOptions.Apply(buildChannelListQuery(groupFilter, statusFilter, typeFilter, codexAccount, statusCodeFilter, hasStatusCodeFilter).Where("tag = ?", *tag)).
				Omit("key").
				Find(&tagChannels).Error
			if err != nil {
				common.SysError("failed to get channels by tag: " + err.Error())
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取标签渠道失败，请稍后重试"})
				return
			}
			filtered := make([]*model.Channel, 0, len(tagChannels))
			for _, ch := range tagChannels {
				if !channelMatchesCodexAccount(ch, codexAccount) {
					continue
				}
				if !channelMatchesStatusCode(ch, statusCodeFilter, hasStatusCodeFilter) {
					continue
				}
				filtered = append(filtered, ch)
			}
			channelData = append(channelData, filtered...)
		}
	} else {
		if err := buildChannelListQuery(groupFilter, statusFilter, typeFilter, codexAccount, statusCodeFilter, hasStatusCodeFilter).Count(&total).Error; err != nil {
			common.SysError("failed to count channels: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道数量失败，请稍后重试"})
			return
		}
		err := sortOptions.Apply(buildChannelListQuery(groupFilter, statusFilter, typeFilter, codexAccount, statusCodeFilter, hasStatusCodeFilter)).
			Limit(pageInfo.GetPageSize()).
			Offset(pageInfo.GetStartIdx()).
			Omit("key").
			Find(&channelData).Error
		if err != nil {
			common.SysError("failed to get channels: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道列表失败，请稍后重试"})
			return
		}
	}

	for _, datum := range channelData {
		clearChannelInfo(datum)
	}

	countQuery := buildChannelListQuery(groupFilter, statusFilter, -1, codexAccount, statusCodeFilter, hasStatusCodeFilter)
	var results []struct {
		Type  int64
		Count int64
	}
	if err := countQuery.Select("type, count(*) as count").Group("type").Find(&results).Error; err != nil {
		common.SysError("failed to count channel types: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道类型统计失败，请稍后重试"})
		return
	}
	typeCounts := make(map[int64]int64)
	for _, r := range results {
		typeCounts[r.Type] = r.Count
	}
	common.ApiSuccess(c, gin.H{
		"items":       channelData,
		"total":       total,
		"page":        pageInfo.GetPage(),
		"page_size":   pageInfo.GetPageSize(),
		"type_counts": typeCounts,
	})
	return
}

func buildFetchModelsHeaders(channel *model.Channel, key string) (http.Header, error) {
	var headers http.Header
	switch channel.Type {
	case constant.ChannelTypeAnthropic:
		headers = GetClaudeAuthHeader(key)
	default:
		headers = GetAuthHeader(key)
	}

	headerOverride := channel.GetHeaderOverride()
	for k, v := range headerOverride {
		if relaychannel.IsHeaderPassthroughRuleKey(k) {
			continue
		}
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid header override for key %s", k)
		}
		if strings.Contains(str, "{api_key}") {
			str = strings.ReplaceAll(str, "{api_key}", key)
		}
		headers.Set(k, str)
	}

	return headers, nil
}

func FetchUpstreamModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ids, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取模型列表失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    ids,
	})
}

func FixChannelsAbilities(c *gin.Context) {
	success, fails, err := model.FixAbility()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"success": success,
			"fails":   fails,
		},
	})
}

func SearchChannels(c *gin.Context) {
	keyword := c.Query("keyword")
	modelKeyword := c.Query("model")
	statusParam := c.Query("status")
	statusCodeFilter, hasStatusCodeFilter := parseChannelStatusCodeFilter(c.Query("status_code"))
	codexAccount := c.Query("codex_account")
	group := model.NormalizeChannelGroupFilter(c.Query("group"))
	statusFilter := parseStatusFilter(statusParam)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	sortOptions := model.NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort)
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	channelData := make([]*model.Channel, 0)
	if enableTagMode {
		tags, err := model.SearchTags(keyword, group, modelKeyword, idSort)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		for _, tag := range tags {
			if tag != nil && *tag != "" {
				var tagChannels []*model.Channel
				err := sortOptions.Apply(buildChannelListQuery(group, -1, -1, codexAccount, statusCodeFilter, hasStatusCodeFilter).Where("tag = ?", *tag)).
					Omit("key").
					Find(&tagChannels).Error
				if err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": err.Error(),
					})
					return
				}
				channelData = append(channelData, tagChannels...)
			}
		}
	} else {
		channels, err := model.SearchChannels(keyword, group, modelKeyword, idSort, sortOptions)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		channelData = channels
	}

	if statusFilter == common.ChannelStatusEnabled || statusFilter == 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if statusFilter == common.ChannelStatusEnabled && ch.Status != common.ChannelStatusEnabled {
				continue
			}
			if statusFilter == 0 && ch.Status == common.ChannelStatusEnabled {
				continue
			}
			filtered = append(filtered, ch)
		}
		channelData = filtered
	}

	channelData = filterChannelsByCodexAccount(channelData, codexAccount)
	channelData = filterChannelsByStatusCode(channelData, statusCodeFilter, hasStatusCodeFilter)

	// calculate type counts for search results
	typeCounts := make(map[int64]int64)
	for _, channel := range channelData {
		typeCounts[int64(channel.Type)]++
	}

	typeParam := c.Query("type")
	typeFilter := -1
	if typeParam != "" {
		if tp, err := strconv.Atoi(typeParam); err == nil {
			typeFilter = tp
		}
	}

	if typeFilter >= 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if ch.Type == typeFilter {
				filtered = append(filtered, ch)
			}
		}
		channelData = filtered
	}

	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	total := len(channelData)
	startIdx := (page - 1) * pageSize
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + pageSize
	if endIdx > total {
		endIdx = total
	}

	pagedData := channelData[startIdx:endIdx]

	for _, datum := range pagedData {
		clearChannelInfo(datum)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":       pagedData,
			"total":       total,
			"type_counts": typeCounts,
		},
	})
	return
}

func GetChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel != nil {
		clearChannelInfo(channel)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
	return
}

// GetChannelKey 获取渠道密钥（需要通过安全验证中间件）
// 此函数依赖 SecureVerificationRequired 中间件，确保用户已通过安全验证
func GetChannelKey(c *gin.Context) {
	userId := c.GetInt("id")
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("渠道ID格式错误: %v", err))
		return
	}

	// 获取渠道信息（包含密钥）
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取渠道信息失败: %v", err))
		return
	}

	if channel == nil {
		common.ApiError(c, fmt.Errorf("渠道不存在"))
		return
	}

	// 记录操作日志
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("查看渠道密钥信息 (渠道ID: %d)", channelId))

	// 返回渠道密钥
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取成功",
		"data": map[string]interface{}{
			"key": channel.Key,
		},
	})
}

// validateTwoFactorAuth 统一的2FA验证函数
func validateTwoFactorAuth(twoFA *model.TwoFA, code string) bool {
	// 尝试验证TOTP
	if cleanCode, err := common.ValidateNumericCode(code); err == nil {
		if isValid, _ := twoFA.ValidateTOTPAndUpdateUsage(cleanCode); isValid {
			return true
		}
	}

	// 尝试验证备用码
	if isValid, err := twoFA.ValidateBackupCodeAndUpdateUsage(code); err == nil && isValid {
		return true
	}

	return false
}

// validateChannel 通用的渠道校验函数
func validateChannel(channel *model.Channel, isAdd bool) error {
	// 校验 channel settings
	if err := channel.ValidateSettings(); err != nil {
		return fmt.Errorf("渠道额外设置[channel setting] 格式错误：%s", err.Error())
	}

	// 如果是添加操作，检查 channel 和 key 是否为空
	if isAdd {
		if channel == nil || channel.Key == "" {
			return fmt.Errorf("channel cannot be empty")
		}

		// 检查模型名称长度是否超过 255
		for _, m := range channel.GetModels() {
			if len(m) > 255 {
				return fmt.Errorf("模型名称过长: %s", m)
			}
		}
	}

	// VertexAI 特殊校验
	if channel.Type == constant.ChannelTypeVertexAi {
		if channel.Other == "" {
			return fmt.Errorf("部署地区不能为空")
		}

		regionMap, err := common.StrToMap(channel.Other)
		if err != nil {
			return fmt.Errorf("部署地区必须是标准的Json格式，例如{\"default\": \"us-central1\", \"region2\": \"us-east1\"}")
		}

		if regionMap["default"] == nil {
			return fmt.Errorf("部署地区必须包含default字段")
		}
	}

	// Codex OAuth key validation (optional, only when JSON object is provided)
	if channel.Type == constant.ChannelTypeCodex {
		trimmedKey := strings.TrimSpace(channel.Key)
		if isAdd || trimmedKey != "" {
			if _, err := normalizeCodexKey(trimmedKey); err != nil {
				return err
			}
		}
	}
	if channel.Type == constant.ChannelTypeChatGPTImage {
		trimmedKey := strings.TrimSpace(channel.Key)
		if isAdd || trimmedKey != "" {
			if strings.HasPrefix(trimmedKey, "[") {
				if _, err := chatgptimg.GetOAuthBatchKeys(trimmedKey); err != nil {
					return err
				}
			} else if strings.Contains(trimmedKey, "\n") {
				for _, line := range strings.Split(trimmedKey, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if _, err := chatgptimg.NormalizeOAuthKey(line); err != nil {
						return err
					}
				}
			} else if _, err := chatgptimg.NormalizeOAuthKey(trimmedKey); err != nil {
				return err
			}
		}
	}

	return nil
}

func ensureCodexServiceTierPassthrough(channel *model.Channel) {
	if channel == nil || channel.Type != constant.ChannelTypeCodex {
		return
	}
	settings := channel.GetOtherSettings()
	settings.AllowServiceTier = true
	channel.SetOtherSettings(settings)
}

func RefreshCodexChannelCredential(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	oauthKey, ch, err := service.RefreshCodexChannelCredential(ctx, channelId, service.CodexCredentialRefreshOptions{ResetCaches: true})
	if err != nil {
		common.SysError("failed to refresh codex channel credential: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "刷新凭证失败，请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "refreshed",
		"data": gin.H{
			"expires_at":   oauthKey.Expired,
			"last_refresh": oauthKey.LastRefresh,
			"account_id":   oauthKey.AccountID,
			"email":        oauthKey.Email,
			"channel_id":   ch.Id,
			"channel_type": ch.Type,
			"channel_name": ch.Name,
		},
	})
}

type AddChannelRequest struct {
	Mode                      string                `json:"mode"`
	MultiKeyMode              constant.MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool                  `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *model.Channel        `json:"channel"`
}

const (
	maxChannelCredentialImportFiles                = 1000
	maxChannelCredentialUploadBytes          int64 = 64 << 20
	maxChannelCredentialFileBytes            int64 = 8 << 20
	maxChannelCredentialZipUncompressedBytes       = 64 << 20
)

type channelCredentialImport struct {
	Type   int
	Name   string
	Key    string
	Source string
}

func normalizeChannelTypeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer("-", "", "_", "", " ", "", ".", "")
	return replacer.Replace(name)
}

func parseChannelCredentialType(value any) (int, error) {
	switch v := value.(type) {
	case nil:
		return constant.ChannelTypeCodex, nil
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("channel type must be positive")
		}
		return v, nil
	case int64:
		if v <= 0 {
			return 0, fmt.Errorf("channel type must be positive")
		}
		return int(v), nil
	case float64:
		if v <= 0 || v != float64(int(v)) {
			return 0, fmt.Errorf("channel type must be a positive integer")
		}
		return int(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return constant.ChannelTypeCodex, nil
		}
		if parsed, err := strconv.Atoi(trimmed); err == nil {
			if parsed <= 0 {
				return 0, fmt.Errorf("channel type must be positive")
			}
			return parsed, nil
		}
		normalized := normalizeChannelTypeName(trimmed)
		for channelType, name := range constant.ChannelTypeNames {
			if normalizeChannelTypeName(name) == normalized {
				return channelType, nil
			}
		}
		switch normalized {
		case "chatgpt", "chatgptweb", "chatgptimage", "chatgptimages", "chatgptimg":
			return constant.ChannelTypeChatGPTImage, nil
		case "codex":
			return constant.ChannelTypeCodex, nil
		default:
			return 0, fmt.Errorf("unsupported channel type %q", trimmed)
		}
	default:
		return 0, fmt.Errorf("unsupported channel type value %T", value)
	}
}

func credentialChannelType(raw map[string]any) (int, error) {
	if raw == nil {
		return 0, fmt.Errorf("credential JSON must be an object")
	}
	if value, ok := raw["channel_type"]; ok {
		return parseChannelCredentialType(value)
	}
	if value, ok := raw["type"]; ok {
		return parseChannelCredentialType(value)
	}
	return constant.ChannelTypeCodex, nil
}

func credentialAccountName(raw map[string]any, channelType int) string {
	for _, field := range []string{"email", "account_name", "accountName", "username", "name", "account", "account_id"} {
		if value, ok := raw[field]; ok && value != nil {
			name := strings.TrimSpace(fmt.Sprintf("%v", value))
			if name != "" {
				return name
			}
		}
	}
	channelTypeName := strings.ToLower(constant.GetChannelTypeName(channelType))
	if channelTypeName == "" || channelTypeName == "unknown" {
		channelTypeName = "channel"
	}
	return fmt.Sprintf("%s-%s", channelTypeName, strings.ToLower(common.GetRandomString(6)))
}

func buildChannelCredentialImport(source string, raw map[string]any) (channelCredentialImport, error) {
	channelType, err := credentialChannelType(raw)
	if err != nil {
		return channelCredentialImport{}, err
	}
	keyBytes, err := common.Marshal(raw)
	if err != nil {
		return channelCredentialImport{}, fmt.Errorf("credential JSON encoding failed: %w", err)
	}
	return channelCredentialImport{
		Type:   channelType,
		Name:   credentialAccountName(raw, channelType),
		Key:    string(keyBytes),
		Source: source,
	}, nil
}

func parseChannelCredentialJSONDocument(source string, data []byte) ([]channelCredentialImport, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%s is empty", source)
	}

	rawCredentials := make([]map[string]any, 0)
	switch trimmed[0] {
	case '{':
		var raw map[string]any
		if err := common.Unmarshal(trimmed, &raw); err != nil {
			return nil, fmt.Errorf("%s must be valid JSON: %w", source, err)
		}
		rawCredentials = append(rawCredentials, raw)
	case '[':
		if err := common.Unmarshal(trimmed, &rawCredentials); err != nil {
			return nil, fmt.Errorf("%s must be a JSON object array: %w", source, err)
		}
	default:
		return nil, fmt.Errorf("%s must be a JSON object or object array", source)
	}

	credentials := make([]channelCredentialImport, 0, len(rawCredentials))
	for index, raw := range rawCredentials {
		if len(raw) == 0 {
			return nil, fmt.Errorf("%s credential #%d is empty", source, index+1)
		}
		credential, err := buildChannelCredentialImport(source, raw)
		if err != nil {
			return nil, fmt.Errorf("%s credential #%d invalid: %w", source, index+1, err)
		}
		credentials = append(credentials, credential)
	}
	if len(credentials) == 0 {
		return nil, fmt.Errorf("%s contains no credentials", source)
	}
	return credentials, nil
}

func readZipCredentialFile(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > uint64(maxChannelCredentialFileBytes) {
		return nil, fmt.Errorf("%s exceeds the %d byte per-file limit", file.Name, maxChannelCredentialFileBytes)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxChannelCredentialFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxChannelCredentialFileBytes {
		return nil, fmt.Errorf("%s exceeds the %d byte per-file limit", file.Name, maxChannelCredentialFileBytes)
	}
	return data, nil
}

func parseChannelCredentialZip(source string, data []byte) ([]channelCredentialImport, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid zip file: %w", source, err)
	}

	totalUncompressed := uint64(0)
	jsonFileCount := 0
	credentials := make([]channelCredentialImport, 0)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || strings.ToLower(filepath.Ext(file.Name)) != ".json" {
			continue
		}
		jsonFileCount++
		if jsonFileCount > maxChannelCredentialImportFiles {
			return nil, fmt.Errorf("%s contains more than %d JSON credential files", source, maxChannelCredentialImportFiles)
		}
		totalUncompressed += file.UncompressedSize64
		if totalUncompressed > maxChannelCredentialZipUncompressedBytes {
			return nil, fmt.Errorf("%s exceeds the %d byte uncompressed limit", source, maxChannelCredentialZipUncompressedBytes)
		}
		fileData, err := readZipCredentialFile(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Name, err)
		}
		fileCredentials, err := parseChannelCredentialJSONDocument(file.Name, fileData)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, fileCredentials...)
	}
	if len(credentials) == 0 {
		return nil, fmt.Errorf("%s contains no JSON credential files", source)
	}
	return credentials, nil
}

func readMultipartCredentialFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	maxBytes := maxChannelCredentialFileBytes
	if strings.ToLower(filepath.Ext(fileHeader.Filename)) == ".zip" {
		maxBytes = maxChannelCredentialUploadBytes
	}
	if fileHeader.Size > maxBytes {
		return nil, fmt.Errorf("%s exceeds the %d byte per-file limit", fileHeader.Filename, maxBytes)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds the %d byte per-file limit", fileHeader.Filename, maxBytes)
	}
	return data, nil
}

func parseChannelCredentialFile(fileHeader *multipart.FileHeader) ([]channelCredentialImport, error) {
	fileData, err := readMultipartCredentialFile(fileHeader)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(fileHeader.Filename)) {
	case ".json":
		return parseChannelCredentialJSONDocument(fileHeader.Filename, fileData)
	case ".zip":
		return parseChannelCredentialZip(fileHeader.Filename, fileData)
	default:
		return nil, fmt.Errorf("%s must be a .json or .zip file", fileHeader.Filename)
	}
}

func parseChannelCredentialMultipartFiles(fileHeaders []*multipart.FileHeader) ([]channelCredentialImport, error) {
	if len(fileHeaders) == 0 {
		return nil, errors.New("credential files are required")
	}
	if len(fileHeaders) > maxChannelCredentialImportFiles {
		return nil, fmt.Errorf("cannot import more than %d credential files at once", maxChannelCredentialImportFiles)
	}

	credentials := make([]channelCredentialImport, 0, len(fileHeaders))
	for _, fileHeader := range fileHeaders {
		fileCredentials, err := parseChannelCredentialFile(fileHeader)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, fileCredentials...)
	}
	if len(credentials) == 0 {
		return nil, errors.New("credential files contain no importable credentials")
	}
	return credentials, nil
}

func defaultImportedChannelName(channelType int) string {
	channelTypeName := strings.ToLower(constant.GetChannelTypeName(channelType))
	if channelTypeName == "" || channelTypeName == "unknown" {
		channelTypeName = "channel"
	}
	return fmt.Sprintf("%s-%s", channelTypeName, strings.ToLower(common.GetRandomString(6)))
}

func buildChannelsFromCredentialImports(template *model.Channel, credentials []channelCredentialImport) ([]model.Channel, error) {
	if template == nil {
		return nil, errors.New("channel template is required")
	}
	if len(credentials) == 0 {
		return nil, errors.New("credential files contain no importable credentials")
	}

	channels := make([]model.Channel, 0, len(credentials))
	for index, credential := range credentials {
		if strings.TrimSpace(credential.Key) == "" {
			return nil, fmt.Errorf("credential #%d key is empty", index+1)
		}

		localChannel := *template
		localChannel.Id = 0
		localChannel.Type = credential.Type
		if localChannel.Type == 0 {
			localChannel.Type = constant.ChannelTypeCodex
		}
		localChannel.Key = strings.TrimSpace(credential.Key)
		localChannel.Name = strings.TrimSpace(credential.Name)
		localChannel.CreatedTime = common.GetTimestamp()
		localChannel.ChannelInfo = model.ChannelInfo{}

		if localChannel.Type == constant.ChannelTypeCodex {
			normalized, err := normalizeCodexKey(localChannel.Key)
			if err != nil {
				return nil, fmt.Errorf("%s invalid: %w", credential.Source, err)
			}
			localChannel.Key = normalized
			localChannel.Name = getCodexChannelName(localChannel.Key, localChannel.Name)
		} else if localChannel.Type == constant.ChannelTypeChatGPTImage {
			normalized, err := chatgptimg.NormalizeOAuthKey(localChannel.Key)
			if err != nil {
				return nil, fmt.Errorf("%s invalid: %w", credential.Source, err)
			}
			localChannel.Key = normalized
			localChannel.Name = chatgptimg.GetOAuthChannelName(localChannel.Key, localChannel.Name)
		}
		if strings.TrimSpace(localChannel.Name) == "" {
			localChannel.Name = defaultImportedChannelName(localChannel.Type)
		}

		if err := validateChannel(&localChannel, true); err != nil {
			return nil, fmt.Errorf("%s invalid: %w", credential.Source, err)
		}
		ensureCodexServiceTierPassthrough(&localChannel)
		channels = append(channels, localChannel)
	}
	return channels, nil
}

func ImportChannels(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChannelCredentialUploadBytes)
	if err := c.Request.ParseMultipartForm(maxChannelCredentialUploadBytes); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "导入文件解析失败: " + err.Error(),
		})
		return
	}

	payload := strings.TrimSpace(c.PostForm("payload"))
	if payload == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "payload is required",
		})
		return
	}
	addChannelRequest := AddChannelRequest{}
	if err := common.Unmarshal([]byte(payload), &addChannelRequest); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "payload must be valid JSON: " + err.Error(),
		})
		return
	}
	if addChannelRequest.Channel == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel template is required",
		})
		return
	}

	var fileHeaders []*multipart.FileHeader
	if c.Request.MultipartForm != nil {
		fileHeaders = c.Request.MultipartForm.File["files"]
	}
	credentials, err := parseChannelCredentialMultipartFiles(fileHeaders)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	channels, err := buildChannelsFromCredentialImports(addChannelRequest.Channel, credentials)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if err := model.BatchInsertChannels(channels); err != nil {
		common.ApiError(c, err)
		return
	}
	service.ResetProxyClientCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count": len(channels),
		},
	})
}

func getVertexArrayKeys(keys string) ([]string, error) {
	if keys == "" {
		return nil, nil
	}
	var keyArray []interface{}
	err := common.Unmarshal([]byte(keys), &keyArray)
	if err != nil {
		return nil, fmt.Errorf("批量添加 Vertex AI 必须使用标准的JsonArray格式，例如[{key1}, {key2}...]，请检查输入: %w", err)
	}
	cleanKeys := make([]string, 0, len(keyArray))
	for _, key := range keyArray {
		var keyStr string
		switch v := key.(type) {
		case string:
			keyStr = strings.TrimSpace(v)
		default:
			bytes, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("Vertex AI key JSON 编码失败: %w", err)
			}
			keyStr = string(bytes)
		}
		if keyStr != "" {
			cleanKeys = append(cleanKeys, keyStr)
		}
	}
	if len(cleanKeys) == 0 {
		return nil, fmt.Errorf("批量添加 Vertex AI 的 keys 不能为空")
	}
	return cleanKeys, nil
}

func AddChannel(c *gin.Context) {
	addChannelRequest := AddChannelRequest{}
	err := c.ShouldBindJSON(&addChannelRequest)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	preparedCodexKeys := make([]string, 0)
	preparedChatGPTImageKeys := make([]string, 0)
	if addChannelRequest.Channel != nil &&
		addChannelRequest.Channel.Type == constant.ChannelTypeCodex &&
		addChannelRequest.Mode == "batch" {
		preparedCodexKeys, err = getCodexBatchKeys(addChannelRequest.Channel.Key)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		addChannelRequest.Channel.Key = preparedCodexKeys[0]
	}
	if addChannelRequest.Channel != nil &&
		addChannelRequest.Channel.Type == constant.ChannelTypeChatGPTImage &&
		(addChannelRequest.Mode == "batch" || addChannelRequest.Mode == "multi_to_single") {
		trimmed := strings.TrimSpace(addChannelRequest.Channel.Key)
		if strings.HasPrefix(trimmed, "[") {
			preparedChatGPTImageKeys, err = chatgptimg.GetOAuthBatchKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			addChannelRequest.Channel.Key = preparedChatGPTImageKeys[0]
		} else {
			normalized, normalizeErr := chatgptimg.NormalizeOAuthKey(addChannelRequest.Channel.Key)
			if normalizeErr != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": normalizeErr.Error(),
				})
				return
			}
			preparedChatGPTImageKeys = []string{normalized}
			addChannelRequest.Channel.Key = normalized
		}
	}

	// 使用统一的校验函数
	if err := validateChannel(addChannelRequest.Channel, true); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	ensureCodexServiceTierPassthrough(addChannelRequest.Channel)
	addChannelRequest.Channel.CreatedTime = common.GetTimestamp()
	keys := make([]string, 0)
	switch addChannelRequest.Mode {
	case "multi_to_single":
		addChannelRequest.Channel.ChannelInfo.IsMultiKey = true
		addChannelRequest.Channel.ChannelInfo.MultiKeyMode = addChannelRequest.MultiKeyMode
		if addChannelRequest.Channel.Type == constant.ChannelTypeChatGPTImage {
			if len(preparedChatGPTImageKeys) == 0 {
				trimmed := strings.TrimSpace(addChannelRequest.Channel.Key)
				if strings.HasPrefix(trimmed, "[") {
					preparedChatGPTImageKeys, err = chatgptimg.GetOAuthBatchKeys(addChannelRequest.Channel.Key)
					if err != nil {
						c.JSON(http.StatusOK, gin.H{
							"success": false,
							"message": err.Error(),
						})
						return
					}
				} else if strings.Contains(trimmed, "\n") {
					lines := strings.Split(trimmed, "\n")
					preparedChatGPTImageKeys = make([]string, 0, len(lines))
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line == "" {
							continue
						}
						normalized, normalizeErr := chatgptimg.NormalizeOAuthKey(line)
						if normalizeErr != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": normalizeErr.Error(),
							})
							return
						}
						preparedChatGPTImageKeys = append(preparedChatGPTImageKeys, normalized)
					}
				} else {
					normalized, normalizeErr := chatgptimg.NormalizeOAuthKey(addChannelRequest.Channel.Key)
					if normalizeErr != nil {
						c.JSON(http.StatusOK, gin.H{
							"success": false,
							"message": normalizeErr.Error(),
						})
						return
					}
					preparedChatGPTImageKeys = []string{normalized}
				}
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(preparedChatGPTImageKeys)
			addChannelRequest.Channel.Key = strings.Join(preparedChatGPTImageKeys, "\n")
		} else if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			array, err := getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(array)
			addChannelRequest.Channel.Key = strings.Join(array, "\n")
		} else {
			cleanKeys := make([]string, 0)
			for _, key := range strings.Split(addChannelRequest.Channel.Key, "\n") {
				if key == "" {
					continue
				}
				key = strings.TrimSpace(key)
				cleanKeys = append(cleanKeys, key)
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(cleanKeys)
			addChannelRequest.Channel.Key = strings.Join(cleanKeys, "\n")
		}
		keys = []string{addChannelRequest.Channel.Key}
	case "batch":
		if addChannelRequest.Channel.Type == constant.ChannelTypeCodex {
			if len(preparedCodexKeys) > 0 {
				keys = preparedCodexKeys
			} else {
				keys, err = getCodexBatchKeys(addChannelRequest.Channel.Key)
				if err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": err.Error(),
					})
					return
				}
			}
		} else if addChannelRequest.Channel.Type == constant.ChannelTypeChatGPTImage {
			if len(preparedChatGPTImageKeys) > 0 {
				keys = preparedChatGPTImageKeys
			} else {
				trimmed := strings.TrimSpace(addChannelRequest.Channel.Key)
				if strings.HasPrefix(trimmed, "[") {
					keys, err = chatgptimg.GetOAuthBatchKeys(addChannelRequest.Channel.Key)
					if err != nil {
						c.JSON(http.StatusOK, gin.H{
							"success": false,
							"message": err.Error(),
						})
						return
					}
				} else if strings.Contains(trimmed, "\n") {
					keys = make([]string, 0)
					for _, line := range strings.Split(trimmed, "\n") {
						line = strings.TrimSpace(line)
						if line == "" {
							continue
						}
						normalized, normalizeErr := chatgptimg.NormalizeOAuthKey(line)
						if normalizeErr != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": normalizeErr.Error(),
							})
							return
						}
						keys = append(keys, normalized)
					}
				} else {
					normalized, normalizeErr := chatgptimg.NormalizeOAuthKey(addChannelRequest.Channel.Key)
					if normalizeErr != nil {
						c.JSON(http.StatusOK, gin.H{
							"success": false,
							"message": normalizeErr.Error(),
						})
						return
					}
					keys = []string{normalized}
				}
			}
		} else if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			// multi json
			keys, err = getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			keys = strings.Split(addChannelRequest.Channel.Key, "\n")
		}
	case "single":
		keys = []string{addChannelRequest.Channel.Key}
	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的添加模式",
		})
		return
	}

	channels := make([]model.Channel, 0, len(keys))
	originalName := addChannelRequest.Channel.Name
	for _, key := range keys {
		if key == "" {
			continue
		}
		localChannel := *addChannelRequest.Channel
		localChannel.Key = key
		if localChannel.Type == constant.ChannelTypeCodex {
			localChannel.Name = getCodexChannelName(key, localChannel.Name)
		} else if localChannel.Type == constant.ChannelTypeChatGPTImage {
			localChannel.Name = chatgptimg.GetOAuthChannelName(key, localChannel.Name)
		}
		if addChannelRequest.BatchAddSetKeyPrefix2Name && len(keys) > 1 && strings.TrimSpace(originalName) != "" {
			keyPrefix := localChannel.Key
			if len(localChannel.Key) > 8 {
				keyPrefix = localChannel.Key[:8]
			}
			localChannel.Name = fmt.Sprintf("%s %s", localChannel.Name, keyPrefix)
		}
		channels = append(channels, localChannel)
	}
	err = model.BatchInsertChannels(channels)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	service.ResetProxyClientCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func validateCodexKeyObject(keyMap map[string]any) error {
	if v, ok := keyMap["access_token"]; !ok || v == nil || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
		return fmt.Errorf("Codex key JSON must include access_token")
	}
	if v, ok := keyMap["account_id"]; !ok || v == nil || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
		return fmt.Errorf("Codex key JSON must include account_id")
	}
	return nil
}

func normalizeCodexKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return "", fmt.Errorf("Codex key must be a valid JSON object")
	}
	var keyMap map[string]any
	if err := common.Unmarshal([]byte(trimmed), &keyMap); err != nil {
		return "", fmt.Errorf("Codex key must be a valid JSON object")
	}
	if err := validateCodexKeyObject(keyMap); err != nil {
		return "", err
	}
	normalizedBytes, err := common.Marshal(keyMap)
	if err != nil {
		return "", fmt.Errorf("Codex key JSON 编码失败: %w", err)
	}
	return string(normalizedBytes), nil
}

func normalizeCodexKeyValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return normalizeCodexKey(v)
	case map[string]any:
		if err := validateCodexKeyObject(v); err != nil {
			return "", err
		}
		normalizedBytes, err := common.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("Codex key JSON 编码失败: %w", err)
		}
		return string(normalizedBytes), nil
	default:
		rawBytes, err := common.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("Codex key JSON 编码失败: %w", err)
		}
		return normalizeCodexKey(string(rawBytes))
	}
}

func getCodexChannelName(rawKey string, currentName string) string {
	if strings.TrimSpace(currentName) != "" {
		return currentName
	}

	var keyMap map[string]any
	if err := common.Unmarshal([]byte(strings.TrimSpace(rawKey)), &keyMap); err != nil {
		return currentName
	}

	email, ok := keyMap["email"]
	if !ok || email == nil {
		return fmt.Sprintf("codex-%s", strings.ToLower(common.GetRandomString(6)))
	}

	emailStr := strings.TrimSpace(fmt.Sprintf("%v", email))
	if emailStr == "" {
		return fmt.Sprintf("codex-%s", strings.ToLower(common.GetRandomString(6)))
	}

	return emailStr
}

func getCodexBatchKeys(keys string) ([]string, error) {
	trimmed := strings.TrimSpace(keys)
	if trimmed == "" {
		return nil, fmt.Errorf("批量添加 Codex 渠道的 keys 不能为空")
	}

	if !strings.HasPrefix(trimmed, "[") {
		return nil, fmt.Errorf("批量添加 Codex 必须使用标准 JsonArray 格式")
	}

	var keyArray []any
	if err := common.Unmarshal([]byte(trimmed), &keyArray); err != nil {
		return nil, fmt.Errorf("批量添加 Codex 必须使用标准 JsonArray 格式，请检查输入: %w", err)
	}

	cleanKeys := make([]string, 0, len(keyArray))
	for index, keyValue := range keyArray {
		normalizedKey, err := normalizeCodexKeyValue(keyValue)
		if err != nil {
			return nil, fmt.Errorf("第 %d 个 Codex key 无效: %w", index+1, err)
		}
		if normalizedKey != "" {
			cleanKeys = append(cleanKeys, normalizedKey)
		}
	}
	if len(cleanKeys) == 0 {
		return nil, fmt.Errorf("批量添加 Codex 的 keys 不能为空")
	}
	return cleanKeys, nil
}

func DeleteChannel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	channel := model.Channel{Id: id}
	err := channel.Delete()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteDisabledChannel(c *gin.Context) {
	rows, err := model.DeleteDisabledChannel()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

type ChannelTag struct {
	Tag            string  `json:"tag"`
	NewTag         *string `json:"new_tag"`
	Priority       *int64  `json:"priority"`
	Weight         *uint   `json:"weight"`
	ModelMapping   *string `json:"model_mapping"`
	Models         *string `json:"models"`
	Groups         *string `json:"groups"`
	ParamOverride  *string `json:"param_override"`
	HeaderOverride *string `json:"header_override"`
}

func DisableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.DisableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EnableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.EnableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EditTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	if channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}
	if channelTag.ParamOverride != nil {
		trimmed := strings.TrimSpace(*channelTag.ParamOverride)
		if trimmed != "" && !json.Valid([]byte(trimmed)) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "参数覆盖必须是合法的 JSON 格式",
			})
			return
		}
		channelTag.ParamOverride = common.GetPointer[string](trimmed)
	}
	if channelTag.HeaderOverride != nil {
		trimmed := strings.TrimSpace(*channelTag.HeaderOverride)
		if trimmed != "" && !json.Valid([]byte(trimmed)) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "请求头覆盖必须是合法的 JSON 格式",
			})
			return
		}
		channelTag.HeaderOverride = common.GetPointer[string](trimmed)
	}
	err = model.EditChannelByTag(channelTag.Tag, channelTag.NewTag, channelTag.ModelMapping, channelTag.Models, channelTag.Groups, channelTag.Priority, channelTag.Weight, channelTag.ParamOverride, channelTag.HeaderOverride)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

type ChannelBatch struct {
	Ids []int   `json:"ids"`
	Tag *string `json:"tag"`
}

func DeleteChannelBatch(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.BatchDeleteChannels(channelBatch.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(channelBatch.Ids),
	})
	return
}

type PatchChannel struct {
	model.Channel
	MultiKeyMode *string `json:"multi_key_mode"`
	KeyMode      *string `json:"key_mode"` // 多key模式下密钥覆盖或者追加
}

func UpdateChannel(c *gin.Context) {
	channel := PatchChannel{}
	err := c.ShouldBindJSON(&channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 使用统一的校验函数
	if err := validateChannel(&channel.Channel, false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// Preserve existing ChannelInfo to ensure multi-key channels keep correct state even if the client does not send ChannelInfo in the request.
	originChannel, err := model.GetChannelById(channel.Id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Always copy the original ChannelInfo so that fields like IsMultiKey and MultiKeySize are retained.
	channel.ChannelInfo = originChannel.ChannelInfo

	// If the request explicitly specifies a new MultiKeyMode, apply it on top of the original info.
	if channel.MultiKeyMode != nil && *channel.MultiKeyMode != "" {
		channel.ChannelInfo.MultiKeyMode = constant.MultiKeyMode(*channel.MultiKeyMode)
	}

	// 处理多key模式下的密钥追加/覆盖逻辑
	if channel.KeyMode != nil && channel.ChannelInfo.IsMultiKey {
		switch *channel.KeyMode {
		case "append":
			// 追加模式：将新密钥添加到现有密钥列表
			if originChannel.Key != "" {
				var newKeys []string
				var existingKeys []string

				// 解析现有密钥
				if strings.HasPrefix(strings.TrimSpace(originChannel.Key), "[") {
					// JSON数组格式
					var arr []json.RawMessage
					if err := json.Unmarshal([]byte(strings.TrimSpace(originChannel.Key)), &arr); err == nil {
						existingKeys = make([]string, len(arr))
						for i, v := range arr {
							existingKeys[i] = string(v)
						}
					}
				} else {
					// 换行分隔格式
					existingKeys = strings.Split(strings.Trim(originChannel.Key, "\n"), "\n")
				}

				// 处理 Vertex AI 的特殊情况
				if channel.Type == constant.ChannelTypeVertexAi && channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
					// 尝试解析新密钥为JSON数组
					if strings.HasPrefix(strings.TrimSpace(channel.Key), "[") {
						array, err := getVertexArrayKeys(channel.Key)
						if err != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": "追加密钥解析失败: " + err.Error(),
							})
							return
						}
						newKeys = array
					} else {
						// 单个JSON密钥
						newKeys = []string{channel.Key}
					}
				} else if channel.Type == constant.ChannelTypeChatGPTImage {
					trimmed := strings.TrimSpace(channel.Key)
					if strings.HasPrefix(trimmed, "[") {
						newKeys, err = chatgptimg.GetOAuthBatchKeys(channel.Key)
						if err != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": "追加密钥解析失败: " + err.Error(),
							})
							return
						}
					} else if strings.Contains(trimmed, "\n") {
						inputKeys := strings.Split(trimmed, "\n")
						for _, key := range inputKeys {
							key = strings.TrimSpace(key)
							if key == "" {
								continue
							}
							normalized, normalizeErr := chatgptimg.NormalizeOAuthKey(key)
							if normalizeErr != nil {
								c.JSON(http.StatusOK, gin.H{
									"success": false,
									"message": "追加密钥解析失败: " + normalizeErr.Error(),
								})
								return
							}
							newKeys = append(newKeys, normalized)
						}
					} else {
						normalized, normalizeErr := chatgptimg.NormalizeOAuthKey(channel.Key)
						if normalizeErr != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": "追加密钥解析失败: " + normalizeErr.Error(),
							})
							return
						}
						newKeys = []string{normalized}
					}
				} else {
					// 普通渠道的处理
					inputKeys := strings.Split(channel.Key, "\n")
					for _, key := range inputKeys {
						key = strings.TrimSpace(key)
						if key != "" {
							newKeys = append(newKeys, key)
						}
					}
				}

				seen := make(map[string]struct{}, len(existingKeys)+len(newKeys))
				for _, key := range existingKeys {
					normalized := strings.TrimSpace(key)
					if normalized == "" {
						continue
					}
					seen[normalized] = struct{}{}
				}
				dedupedNewKeys := make([]string, 0, len(newKeys))
				for _, key := range newKeys {
					normalized := strings.TrimSpace(key)
					if normalized == "" {
						continue
					}
					if _, ok := seen[normalized]; ok {
						continue
					}
					seen[normalized] = struct{}{}
					dedupedNewKeys = append(dedupedNewKeys, normalized)
				}

				allKeys := append(existingKeys, dedupedNewKeys...)
				channel.Key = strings.Join(allKeys, "\n")
			}
		case "replace":
			// 覆盖模式：直接使用新密钥（默认行为，不需要特殊处理）
		}
	}
	ensureCodexServiceTierPassthrough(&channel.Channel)
	err = channel.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	channel.Key = ""
	clearChannelInfo(&channel.Channel)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
	return
}

func FetchModels(c *gin.Context) {
	var req struct {
		BaseURL string `json:"base_url"`
		Type    int    `json:"type"`
		Key     string `json:"key"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
		})
		return
	}

	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[req.Type]
	}

	// remove line breaks and extra spaces.
	key := strings.TrimSpace(req.Key)
	key = strings.Split(key, "\n")[0]

	if req.Type == constant.ChannelTypeOllama {
		models, err := ollama.FetchOllamaModels(baseURL, key)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("获取Ollama模型失败: %s", err.Error()),
			})
			return
		}

		names := make([]string, 0, len(models))
		for _, modelInfo := range models {
			names = append(names, modelInfo.Name)
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    names,
		})
		return
	}

	if req.Type == constant.ChannelTypeGemini {
		models, err := gemini.FetchGeminiModels(baseURL, key, "")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("获取Gemini模型失败: %s", err.Error()),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    models,
		})
		return
	}
	if req.Type == constant.ChannelTypeChatGPTImage {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    chatgptimg.ModelList,
		})
		return
	}

	client := &http.Client{}
	url := fmt.Sprintf("%s/v1/models", baseURL)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	request.Header.Set("Authorization", "Bearer "+key)

	response, err := client.Do(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	//check status code
	if response.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch models",
		})
		return
	}
	defer response.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var models []string
	for _, model := range result.Data {
		models = append(models, model.ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    models,
	})
}

func BatchSetChannelTag(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.BatchSetChannelTag(channelBatch.Ids, channelBatch.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(channelBatch.Ids),
	})
	return
}

func GetTagModels(c *gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}

	channels, err := model.GetChannelsByTag(tag, false, false) // idSort=false, selectAll=false
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var longestModels string
	maxLength := 0

	// Find the longest models string among all channels with the given tag
	for _, channel := range channels {
		if channel.Models != "" {
			currentModels := strings.Split(channel.Models, ",")
			if len(currentModels) > maxLength {
				maxLength = len(currentModels)
				longestModels = channel.Models
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    longestModels,
	})
	return
}

// CopyChannel handles cloning an existing channel with its key.
// POST /api/channel/copy/:id
// Optional query params:
//
//	suffix         - string appended to the original name (default "_复制")
//	reset_balance  - bool, when true will reset balance & used_quota to 0 (default true)
func CopyChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid id"})
		return
	}

	suffix := c.DefaultQuery("suffix", "_复制")
	resetBalance := true
	if rbStr := c.DefaultQuery("reset_balance", "true"); rbStr != "" {
		if v, err := strconv.ParseBool(rbStr); err == nil {
			resetBalance = v
		}
	}

	// fetch original channel with key
	origin, err := model.GetChannelById(id, true)
	if err != nil {
		common.SysError("failed to get channel by id: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道信息失败，请稍后重试"})
		return
	}

	// clone channel
	clone := *origin // shallow copy is sufficient as we will overwrite primitives
	clone.Id = 0     // let DB auto-generate
	clone.CreatedTime = common.GetTimestamp()
	clone.Name = origin.Name + suffix
	clone.TestTime = 0
	clone.ResponseTime = 0
	if resetBalance {
		clone.Balance = 0
		clone.UsedQuota = 0
	}

	// insert
	if err := clone.Insert(); err != nil {
		common.SysError("failed to clone channel: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "复制渠道失败，请稍后重试"})
		return
	}
	model.InitChannelCache()
	// success
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": clone.Id}})
}

// MultiKeyManageRequest represents the request for multi-key management operations
type MultiKeyManageRequest struct {
	ChannelId int    `json:"channel_id"`
	Action    string `json:"action"`              // "disable_key", "enable_key", "delete_key", "delete_disabled_keys", "get_key_status"
	KeyIndex  *int   `json:"key_index,omitempty"` // for disable_key, enable_key, and delete_key actions
	Page      int    `json:"page,omitempty"`      // for get_key_status pagination
	PageSize  int    `json:"page_size,omitempty"` // for get_key_status pagination
	Status    *int   `json:"status,omitempty"`    // for get_key_status filtering: 1=enabled, 2=manual_disabled, 3=auto_disabled, nil=all
}

// MultiKeyStatusResponse represents the response for key status query
type MultiKeyStatusResponse struct {
	Keys       []KeyStatus `json:"keys"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
	// Statistics
	EnabledCount        int `json:"enabled_count"`
	ManualDisabledCount int `json:"manual_disabled_count"`
	AutoDisabledCount   int `json:"auto_disabled_count"`
}

type KeyStatus struct {
	Index        int    `json:"index"`
	Status       int    `json:"status"` // 1: enabled, 2: disabled
	DisabledTime int64  `json:"disabled_time,omitempty"`
	Reason       string `json:"reason,omitempty"`
	KeyPreview   string `json:"key_preview"` // first 10 chars of key for identification
}

// ManageMultiKeys handles multi-key management operations
func ManageMultiKeys(c *gin.Context) {
	request := MultiKeyManageRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(request.ChannelId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "渠道不存在",
		})
		return
	}

	if !channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该渠道不是多密钥模式",
		})
		return
	}

	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	switch request.Action {
	case "get_key_status":
		keys := channel.GetKeys()

		// Default pagination parameters
		page := request.Page
		pageSize := request.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 50 // Default page size
		}

		// Statistics for all keys (unchanged by filtering)
		var enabledCount, manualDisabledCount, autoDisabledCount int

		// Build all key status data first
		var allKeyStatusList []KeyStatus
		for i, key := range keys {
			status := 1 // default enabled
			var disabledTime int64
			var reason string

			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// Count for statistics (all keys)
			switch status {
			case 1:
				enabledCount++
			case 2:
				manualDisabledCount++
			case 3:
				autoDisabledCount++
			}

			if status != 1 {
				if channel.ChannelInfo.MultiKeyDisabledTime != nil {
					disabledTime = channel.ChannelInfo.MultiKeyDisabledTime[i]
				}
				if channel.ChannelInfo.MultiKeyDisabledReason != nil {
					reason = channel.ChannelInfo.MultiKeyDisabledReason[i]
				}
			}

			// Create key preview (first 10 chars)
			keyPreview := key
			if len(key) > 10 {
				keyPreview = key[:10] + "..."
			}

			allKeyStatusList = append(allKeyStatusList, KeyStatus{
				Index:        i,
				Status:       status,
				DisabledTime: disabledTime,
				Reason:       reason,
				KeyPreview:   keyPreview,
			})
		}

		// Apply status filter if specified
		var filteredKeyStatusList []KeyStatus
		if request.Status != nil {
			for _, keyStatus := range allKeyStatusList {
				if keyStatus.Status == *request.Status {
					filteredKeyStatusList = append(filteredKeyStatusList, keyStatus)
				}
			}
		} else {
			filteredKeyStatusList = allKeyStatusList
		}

		// Calculate pagination based on filtered results
		filteredTotal := len(filteredKeyStatusList)
		totalPages := (filteredTotal + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}

		// Calculate range for current page
		start := (page - 1) * pageSize
		end := start + pageSize
		if end > filteredTotal {
			end = filteredTotal
		}

		// Get the page data
		var pageKeyStatusList []KeyStatus
		if start < filteredTotal {
			pageKeyStatusList = filteredKeyStatusList[start:end]
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": MultiKeyStatusResponse{
				Keys:                pageKeyStatusList,
				Total:               filteredTotal, // Total of filtered results
				Page:                page,
				PageSize:            pageSize,
				TotalPages:          totalPages,
				EnabledCount:        enabledCount,        // Overall statistics
				ManualDisabledCount: manualDisabledCount, // Overall statistics
				AutoDisabledCount:   autoDisabledCount,   // Overall statistics
			},
		})
		return

	case "disable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要禁用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		channel.ChannelInfo.MultiKeyStatusList[keyIndex] = 2 // disabled

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已禁用",
		})
		return

	case "enable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要启用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		// 从状态列表中删除该密钥的记录，使其回到默认启用状态
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			delete(channel.ChannelInfo.MultiKeyStatusList, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledTime, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledReason, keyIndex)
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已启用",
		})
		return

	case "enable_all_keys":
		// 清空所有禁用状态，使所有密钥回到默认启用状态
		var enabledCount int
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			enabledCount = len(channel.ChannelInfo.MultiKeyStatusList)
		}

		channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已启用 %d 个密钥", enabledCount),
		})
		return

	case "disable_all_keys":
		// 禁用所有启用的密钥
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		var disabledCount int
		for i := 0; i < channel.ChannelInfo.MultiKeySize; i++ {
			status := 1 // default enabled
			if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
				status = s
			}

			// 只禁用当前启用的密钥
			if status == 1 {
				channel.ChannelInfo.MultiKeyStatusList[i] = 2 // disabled
				disabledCount++
			}
		}

		if disabledCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有可禁用的密钥",
			})
			return
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已禁用 %d 个密钥", disabledCount),
		})
		return

	case "delete_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要删除的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		keys := channel.GetKeys()
		var remainingKeys []string
		var newStatusList = make(map[int]int)
		var newDisabledTime = make(map[int]int64)
		var newDisabledReason = make(map[int]string)

		newIndex := 0
		for i, key := range keys {
			// 跳过要删除的密钥
			if i == keyIndex {
				continue
			}

			remainingKeys = append(remainingKeys, key)

			// 保留其他密钥的状态信息，重新索引
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if status, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists && status != 1 {
					newStatusList[newIndex] = status
				}
			}
			if channel.ChannelInfo.MultiKeyDisabledTime != nil {
				if t, exists := channel.ChannelInfo.MultiKeyDisabledTime[i]; exists {
					newDisabledTime[newIndex] = t
				}
			}
			if channel.ChannelInfo.MultiKeyDisabledReason != nil {
				if r, exists := channel.ChannelInfo.MultiKeyDisabledReason[i]; exists {
					newDisabledReason[newIndex] = r
				}
			}
			newIndex++
		}

		if len(remainingKeys) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "不能删除最后一个密钥",
			})
			return
		}

		// Update channel with remaining keys
		channel.Key = strings.Join(remainingKeys, "\n")
		channel.ChannelInfo.MultiKeySize = len(remainingKeys)
		channel.ChannelInfo.MultiKeyStatusList = newStatusList
		channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
		channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已删除",
		})
		return

	case "delete_disabled_keys":
		keys := channel.GetKeys()
		var remainingKeys []string
		var deletedCount int
		var newStatusList = make(map[int]int)
		var newDisabledTime = make(map[int]int64)
		var newDisabledReason = make(map[int]string)

		newIndex := 0
		for i, key := range keys {
			status := 1 // default enabled
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// 只删除自动禁用（status == 3）的密钥，保留启用（status == 1）和手动禁用（status == 2）的密钥
			if status == 3 {
				deletedCount++
			} else {
				remainingKeys = append(remainingKeys, key)
				// 保留非自动禁用密钥的状态信息，重新索引
				if status != 1 {
					newStatusList[newIndex] = status
					if channel.ChannelInfo.MultiKeyDisabledTime != nil {
						if t, exists := channel.ChannelInfo.MultiKeyDisabledTime[i]; exists {
							newDisabledTime[newIndex] = t
						}
					}
					if channel.ChannelInfo.MultiKeyDisabledReason != nil {
						if r, exists := channel.ChannelInfo.MultiKeyDisabledReason[i]; exists {
							newDisabledReason[newIndex] = r
						}
					}
				}
				newIndex++
			}
		}

		if deletedCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有需要删除的自动禁用密钥",
			})
			return
		}

		// Update channel with remaining keys
		channel.Key = strings.Join(remainingKeys, "\n")
		channel.ChannelInfo.MultiKeySize = len(remainingKeys)
		channel.ChannelInfo.MultiKeyStatusList = newStatusList
		channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
		channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已删除 %d 个自动禁用的密钥", deletedCount),
			"data":    deletedCount,
		})
		return

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的操作",
		})
		return
	}
}

// OllamaPullModel 拉取 Ollama 模型
func OllamaPullModel(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	err = ollama.PullOllamaModel(baseURL, key, req.ModelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to pull model: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Model %s pulled successfully", req.ModelName),
	})
}

// OllamaPullModelStream 流式拉取 Ollama 模型
func OllamaPullModelStream(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	// 设置 SSE 头部
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	key := strings.Split(channel.Key, "\n")[0]

	// 创建进度回调函数
	progressCallback := func(progress ollama.OllamaPullResponse) {
		data, _ := json.Marshal(progress)
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
		c.Writer.Flush()
	}

	// 执行拉取
	err = ollama.PullOllamaModelStream(baseURL, key, req.ModelName, progressCallback)

	if err != nil {
		errorData, _ := json.Marshal(gin.H{
			"error": err.Error(),
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(errorData))
	} else {
		successData, _ := json.Marshal(gin.H{
			"message": fmt.Sprintf("Model %s pulled successfully", req.ModelName),
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(successData))
	}

	// 发送结束标志
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}

// OllamaDeleteModel 删除 Ollama 模型
func OllamaDeleteModel(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	err = ollama.DeleteOllamaModel(baseURL, key, req.ModelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to delete model: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Model %s deleted successfully", req.ModelName),
	})
}

// OllamaVersion 获取 Ollama 服务版本信息
func OllamaVersion(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid channel id",
		})
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	version, err := ollama.FetchOllamaVersion(baseURL, key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取Ollama版本失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version": version,
		},
	})
}
