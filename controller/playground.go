package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	playgroundRelay(c, types.RelayFormatOpenAI)
}

func PlaygroundImageGeneration(c *gin.Context) {
	playgroundImageGenerationAsync(c)
}

func PlaygroundImageGenerationTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "task_id is required",
		})
		return
	}
	userID := c.GetInt("id")
	task, exist, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !exist {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "task not found",
		})
		return
	}

	resp := gin.H{
		"task_id":    task.TaskID,
		"status":     mapPlaygroundTaskStatus(task.Status),
		"raw_status": string(task.Status),
		"progress":   task.Progress,
		"channel_id": task.ChannelId,
		"model":      task.Properties.OriginModelName,
	}
	if task.FailReason != "" {
		resp["fail_reason"] = task.FailReason
	}
	if len(task.Data) > 0 {
		var data any
		if err := common.Unmarshal(task.Data, &data); err == nil {
			resp["data"] = data
		}
	}
	c.JSON(http.StatusOK, resp)
}

func PlaygroundImageGenerationContent(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "task_id is required",
		})
		return
	}

	imageIndex, err := strconv.Atoi(strings.TrimSpace(c.Param("index")))
	if err != nil || imageIndex < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid image index",
		})
		return
	}

	userID := c.GetInt("id")
	task, exist, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !exist || task == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "task not found",
		})
		return
	}
	if len(task.Data) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "task image data not found",
		})
		return
	}

	var payload struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to parse task image data",
		})
		return
	}
	if imageIndex >= len(payload.Data) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "image not found",
		})
		return
	}

	image := payload.Data[imageIndex]
	if imageURL := strings.TrimSpace(image.URL); imageURL != "" {
		c.Redirect(http.StatusFound, imageURL)
		return
	}

	b64Data := strings.TrimSpace(image.B64JSON)
	if comma := strings.Index(b64Data, ","); strings.HasPrefix(b64Data, "data:") && comma >= 0 {
		b64Data = b64Data[comma+1:]
	}
	if b64Data == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "image data not found",
		})
		return
	}

	imageBytes, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to decode image data",
		})
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Data(http.StatusOK, http.DetectContentType(imageBytes), imageBytes)
}

func playgroundImageGenerationAsync(c *gin.Context) {
	imageRequest, err := helper.GetAndValidateRequest(c, types.RelayFormatOpenAIImage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	imageReq, _ := imageRequest.(*dto.ImageRequest)

	relayInfo, err := preparePlaygroundRelay(c, types.RelayFormatOpenAIImage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if relayInfo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to prepare playground relay info",
		})
		return
	}

	taskID := model.GenerateTaskID()
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if usingGroup == "" {
		usingGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	if usingGroup == "" {
		usingGroup = relayInfo.UsingGroup
	}
	imageModel := ""
	imagePrompt := ""
	if imageReq != nil {
		imageModel = strings.TrimSpace(imageReq.Model)
		imagePrompt = imageReq.Prompt
	}
	channelID := 0
	if relayInfo.ChannelMeta != nil {
		channelID = relayInfo.ChannelMeta.ChannelId
	}
	task := &model.Task{
		TaskID:     taskID,
		UserId:     c.GetInt("id"),
		Group:      usingGroup,
		SubmitTime: time.Now().Unix(),
		Status:     model.TaskStatusSubmitted,
		Progress:   "0%",
		ChannelId:  channelID,
		Platform:   constant.TaskPlatformPlaygroundImage,
		Properties: model.Properties{
			Input:             imagePrompt,
			UpstreamModelName: imageModel,
			OriginModelName:   imageModel,
		},
		PrivateData: model.TaskPrivateData{},
	}
	task.Status = model.TaskStatusSubmitted
	task.Progress = "0%"
	task.Action = constant.TaskActionGenerate
	task.PrivateData.UpstreamTaskID = taskID
	if err := task.Insert(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	bodyStorage, err := common.GetBodyStorage(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	rawBody, err := bodyStorage.Bytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	rawBody = append([]byte(nil), rawBody...)
	reqHeaders := c.Request.Header.Clone()
	reqURI := c.Request.URL.RequestURI()
	ctxKeys := clonePlaygroundContextKeys(c)

	go runPlaygroundImageTask(taskID, reqURI, reqHeaders, rawBody, ctxKeys)

	c.JSON(http.StatusAccepted, gin.H{
		"task_id":  taskID,
		"status":   "submitted",
		"poll_url": fmt.Sprintf("/pg/images/generations/%s", taskID),
	})
}

func preparePlaygroundRelay(c *gin.Context, relayFormat types.RelayFormat) (*relaycommon.RelayInfo, error) {
	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		return nil, errors.New("暂不支持使用 access token")
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, nil, nil)
	if err != nil {
		return nil, err
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return nil, err
	}
	userCache.WriteContext(c)

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", relayInfo.UsingGroup),
		Group:  relayInfo.UsingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	return relayInfo, nil
}

func clonePlaygroundContextKeys(c *gin.Context) map[string]any {
	keys := make(map[string]any, len(c.Keys))
	for k, v := range c.Keys {
		switch k {
		case common.KeyBodyStorage, common.KeyRequestBody:
			continue
		}
		keys[k] = v
	}
	return keys
}

func runPlaygroundImageTask(taskID, requestURI string, headers http.Header, body []byte, keys map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			updatePlaygroundImageTaskFailure(taskID, fmt.Sprintf("playground image worker panicked: %v", r), nil)
		}
	}()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, requestURI, bytes.NewReader(body))
	if err != nil {
		updatePlaygroundImageTaskFailure(taskID, fmt.Sprintf("build request failed: %v", err), nil)
		return
	}
	req.Header = headers.Clone()
	req.ContentLength = int64(len(body))
	ctx.Request = req
	ctx.Keys = keys

	markPlaygroundImageTaskInProgress(taskID)
	Relay(ctx, types.RelayFormatOpenAIImage)

	resp := recorder.Result()
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	updatePlaygroundImageTaskResult(taskID, resp.StatusCode, respBody)
}

func markPlaygroundImageTaskInProgress(taskID string) {
	task, exist, err := model.GetByOnlyTaskId(taskID)
	if err != nil || !exist || task == nil {
		return
	}
	oldStatus := task.Status
	task.Status = model.TaskStatusInProgress
	task.Progress = "1%"
	if task.StartTime == 0 {
		task.StartTime = time.Now().Unix()
	}
	_, _ = task.UpdateWithStatus(oldStatus)
}

func updatePlaygroundImageTaskFailure(taskID, reason string, data []byte) {
	task, exist, err := model.GetByOnlyTaskId(taskID)
	if err != nil || !exist || task == nil {
		return
	}
	oldStatus := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	if task.StartTime == 0 {
		task.StartTime = time.Now().Unix()
	}
	task.FinishTime = time.Now().Unix()
	task.FailReason = reason
	if len(data) > 0 {
		setTaskJSONData(task, data)
	}
	_, _ = task.UpdateWithStatus(oldStatus)
}

func updatePlaygroundImageTaskResult(taskID string, statusCode int, data []byte) {
	task, exist, err := model.GetByOnlyTaskId(taskID)
	if err != nil || !exist || task == nil {
		return
	}
	oldStatus := task.Status
	now := time.Now().Unix()
	if statusCode >= 200 && statusCode < 300 {
		task.Status = model.TaskStatusSuccess
		task.Progress = "100%"
		task.FinishTime = now
		task.FailReason = ""
		setTaskJSONData(task, data)
		if resultURL := extractPlaygroundImageURL(data); resultURL != "" {
			task.PrivateData.ResultURL = resultURL
		}
	} else {
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		task.FailReason = extractPlaygroundImageError(data)
		if task.FailReason == "" {
			task.FailReason = fmt.Sprintf("playground image task failed with status %d", statusCode)
		}
		setTaskJSONData(task, data)
	}
	_, _ = task.UpdateWithStatus(oldStatus)
}

func setTaskJSONData(task *model.Task, data []byte) {
	var parsed any
	if len(data) > 0 && common.Unmarshal(data, &parsed) == nil {
		task.SetData(parsed)
		return
	}
	task.SetData(map[string]any{"raw": string(data)})
}

func extractPlaygroundImageURL(data []byte) string {
	var payload struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
			Revised string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := common.Unmarshal(data, &payload); err != nil || len(payload.Data) == 0 {
		return ""
	}
	first := payload.Data[0]
	if strings.TrimSpace(first.URL) != "" {
		return first.URL
	}
	if strings.TrimSpace(first.B64JSON) != "" {
		return "data:image/png;base64," + strings.TrimSpace(first.B64JSON)
	}
	return ""
}

func extractPlaygroundImageError(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(data, &payload); err == nil {
		if strings.TrimSpace(payload.Error.Message) != "" {
			return payload.Error.Message
		}
		if strings.TrimSpace(payload.Message) != "" {
			return payload.Message
		}
	}
	return strings.TrimSpace(string(data))
}

func mapPlaygroundTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusSubmitted, model.TaskStatusQueued:
		return "queued"
	case model.TaskStatusInProgress, model.TaskStatusNotStart:
		return "processing"
	default:
		return "processing"
	}
}

func playgroundRelay(c *gin.Context, relayFormat types.RelayFormat) {
	if _, err := preparePlaygroundRelay(c, relayFormat); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	Relay(c, relayFormat)
}
