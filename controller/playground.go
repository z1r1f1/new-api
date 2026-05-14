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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	enablePlaygroundUpstreamDebug(c)
	if err := sanitizePlaygroundChatRequest(c); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	playgroundRelay(c, types.RelayFormatOpenAI)
}

func PlaygroundImageGeneration(c *gin.Context) {
	enablePlaygroundUpstreamDebug(c)
	playgroundImageGenerationAsync(c)
}

func PlaygroundDebug(c *gin.Context) {
	debugID := service.NormalizePlaygroundDebugID(c.Param("debug_id"))
	if debugID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid debug id",
		})
		return
	}
	debug, ok := service.GetPlaygroundUpstreamRequestDebug(c.GetInt("id"), debugID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "debug data not found",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    debug,
	})
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
	servePlaygroundImageGenerationContent(c, true)
}

func PlaygroundImageGenerationPublicContent(c *gin.Context) {
	servePlaygroundImageGenerationContent(c, false)
}

func servePlaygroundImageGenerationContent(c *gin.Context, requireUser bool) {
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

	var task *model.Task
	var exist bool
	if requireUser {
		task, exist, err = model.GetByTaskId(c.GetInt("id"), taskID)
	} else {
		task, exist, err = model.GetByOnlyTaskId(taskID)
	}
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
	if !requireUser && task.Platform != constant.TaskPlatformPlaygroundImage {
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
	servePlaygroundImageItem(c, image.URL, image.B64JSON)
}

func servePlaygroundImageItem(c *gin.Context, rawURL, rawB64JSON string) {
	b64Data := strings.TrimSpace(rawB64JSON)
	if comma := strings.Index(b64Data, ","); strings.HasPrefix(b64Data, "data:") && comma >= 0 {
		b64Data = b64Data[comma+1:]
	}
	if b64Data != "" {
		imageBytes, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to decode image data",
			})
			return
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Data(http.StatusOK, http.DetectContentType(imageBytes), imageBytes)
		return
	}

	if imageURL := strings.TrimSpace(rawURL); imageURL != "" {
		if strings.HasPrefix(imageURL, "data:") {
			imageBytes, contentType, err := decodePlaygroundImageDataURL(imageURL)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "failed to decode image data",
				})
				return
			}
			c.Header("Cache-Control", "private, max-age=3600")
			c.Data(http.StatusOK, contentType, imageBytes)
			return
		}
		c.Redirect(http.StatusFound, imageURL)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error": "image data not found",
	})
}

func decodePlaygroundImageDataURL(dataURL string) ([]byte, string, error) {
	dataURL = strings.TrimSpace(dataURL)
	comma := strings.Index(dataURL, ",")
	if !strings.HasPrefix(dataURL, "data:") || comma < 0 {
		return nil, "", errors.New("invalid data url")
	}
	meta := dataURL[len("data:"):comma]
	payload := strings.TrimSpace(dataURL[comma+1:])
	if payload == "" {
		return nil, "", errors.New("empty data url payload")
	}
	imageBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(strings.Split(meta, ";")[0])
	if contentType == "" {
		contentType = http.DetectContentType(imageBytes)
	}
	return imageBytes, contentType, nil
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

func enablePlaygroundUpstreamDebug(c *gin.Context) {
	debugID := service.NormalizePlaygroundDebugID(c.GetHeader(service.PlaygroundDebugIDHeader))
	if debugID == "" {
		return
	}
	common.SetContextKey(c, constant.ContextKeyPlaygroundDebugId, debugID)
	c.Header(service.PlaygroundDebugIDHeader, debugID)
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

func sanitizePlaygroundChatRequest(c *gin.Context) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}

	body, err := storage.Bytes()
	if err != nil {
		return err
	}

	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return err
	}

	changed := false
	changed = deleteNumberIfEqual(payload, "temperature", 0.7) || changed
	changed = deleteNumberIfEqual(payload, "top_p", 1) || changed
	changed = deleteNumberIfEqual(payload, "frequency_penalty", 0) || changed
	changed = deleteNumberIfEqual(payload, "presence_penalty", 0) || changed

	if !changed {
		if _, err := storage.Seek(0, io.SeekStart); err != nil {
			return err
		}
		c.Request.Body = io.NopCloser(storage)
		return nil
	}

	sanitizedBody, err := common.Marshal(payload)
	if err != nil {
		return err
	}

	sanitizedStorage, err := common.CreateBodyStorage(sanitizedBody)
	if err != nil {
		return err
	}

	_ = storage.Close()
	c.Set(common.KeyBodyStorage, sanitizedStorage)
	c.Set(common.KeyRequestBody, sanitizedBody)
	c.Request.Body = io.NopCloser(sanitizedStorage)
	c.Request.ContentLength = int64(len(sanitizedBody))
	c.Request.Header.Set("Content-Length", strconv.Itoa(len(sanitizedBody)))
	return nil
}

func deleteNumberIfEqual(payload map[string]any, key string, expected float64) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}

	number, ok := value.(float64)
	if !ok || number != expected {
		return false
	}

	delete(payload, key)
	return true
}
