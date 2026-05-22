package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func recordImageGenerationDrawingLog(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest) {
	if c == nil || info == nil || request == nil || info.IsChannelTest {
		return
	}
	channelType := 0
	channelID := 0
	upstreamModel := ""
	if info.ChannelMeta != nil {
		channelType = info.ChannelMeta.ChannelType
		channelID = info.ChannelMeta.ChannelId
		upstreamModel = strings.TrimSpace(info.ChannelMeta.UpstreamModelName)
	}
	// ChatGPT Web image requests are logged by the adaptor with conversation
	// metadata; skip here to avoid duplicate drawing-log rows.
	if channelType == constant.ChannelTypeChatGPTImage {
		return
	}

	imageResp, ok := common.GetContextKeyType[*dto.ImageResponse](c, constant.ContextKeyImageGenerationResponse)
	if !ok || imageResp == nil || len(imageResp.Data) == 0 {
		return
	}

	submitTime := time.Now().UnixMilli()
	if !info.StartTime.IsZero() {
		submitTime = info.StartTime.UnixMilli()
	}
	finishTime := time.Now().UnixMilli()
	modelName := strings.TrimSpace(info.OriginModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(request.Model)
	}
	if upstreamModel == "" {
		upstreamModel = strings.TrimSpace(request.Model)
	}
	description := strings.TrimSpace(constant.GetChannelTypeName(channelType))
	if description == "" || description == "Unknown" {
		description = "image-generation"
	}
	taskIDPrefix := model.GenerateTaskID()
	propertiesBytes, _ := common.Marshal(map[string]any{
		"source":               "openai-image",
		"endpoint":             strings.TrimSpace(info.RequestURLPath),
		"channel_type":         channelType,
		"channel_type_name":    description,
		"model":                modelName,
		"upstream_model":       upstreamModel,
		"response_created":     imageResp.Created,
		"response_format":      strings.TrimSpace(request.ResponseFormat),
		"size":                 strings.TrimSpace(request.Size),
		"quality":              strings.TrimSpace(request.Quality),
		"image_response_count": len(imageResp.Data),
	})
	properties := string(propertiesBytes)

	for index, item := range imageResp.Data {
		imageURL := imageDataDrawingLogURL(item)
		if imageURL == "" {
			continue
		}
		taskID := taskIDPrefix
		if len(imageResp.Data) > 1 {
			taskID = fmt.Sprintf("%s-%d", taskIDPrefix, index+1)
		}
		prompt := strings.TrimSpace(request.Prompt)
		if revisedPrompt := strings.TrimSpace(item.RevisedPrompt); revisedPrompt != "" {
			prompt = revisedPrompt
		}
		if err := (&model.Midjourney{
			Code:        1,
			UserId:      info.UserId,
			Action:      "IMAGINE",
			MjId:        taskID,
			Prompt:      prompt,
			Description: description,
			State:       modelName,
			SubmitTime:  submitTime,
			StartTime:   submitTime,
			FinishTime:  finishTime,
			ImageUrl:    imageURL,
			Status:      string(model.TaskStatusSuccess),
			Progress:    "100%",
			ChannelId:   channelID,
			Quota:       info.FinalPreConsumedQuota,
			Properties:  properties,
		}).Insert(); err != nil {
			logger.LogError(c, fmt.Sprintf("record image generation drawing log failed: channel_id=%d model=%s err=%v", channelID, modelName, err))
		}
	}
}

func imageDataDrawingLogURL(item dto.ImageData) string {
	if imageURL := strings.TrimSpace(item.Url); imageURL != "" {
		return imageURL
	}
	if b64 := strings.TrimSpace(item.B64Json); b64 != "" {
		if strings.HasPrefix(b64, "data:") {
			return b64
		}
		return "data:image/png;base64," + b64
	}
	return ""
}

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(imageReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ImageRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.ReaderOnly(storage)
	} else {
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return newAPIErrorFromParamOverride(err)
				}
			}

			logger.LogDebug(c, "image request body: %s", sanitizedRequestBodyForLog(jsonData))

			requestBody = bytes.NewBuffer(jsonData)
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		statusCode := http.StatusInternalServerError
		opts := make([]types.NewAPIErrorOptions, 0, 1)
		var noRetryErr interface {
			SkipRelayRetry() bool
			RelayStatusCode() int
		}
		if errors.As(err, &noRetryErr) {
			if noRetryErr.SkipRelayRetry() {
				opts = append(opts, types.ErrOptionWithSkipRetry())
			}
			if noRetryErr.RelayStatusCode() > 0 {
				statusCode = noRetryErr.RelayStatusCode()
			}
		}
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, statusCode, opts...)
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode == http.StatusCreated && info.ApiType == constant.APITypeReplicate {
				// replicate channel returns 201 Created when using Prefer: wait, treat it as success.
				httpResp.StatusCode = http.StatusOK
			} else {
				newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
				// reset status code 重置状态码
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return newAPIError
			}
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}
	recordImageGenerationDrawingLog(c, info, request)

	imageN := uint(1)
	if request.N != nil {
		imageN = *request.N
	}

	// n is handled via OtherRatio so it is applied exactly once in quota
	// calculation (both price-based and ratio-based paths).
	// Adaptors may have already set a more accurate count from the
	// upstream response; only set the default when they haven't.
	if info.PriceData.UsePrice { // only price model use N ratio
		if _, hasN := info.PriceData.OtherRatios["n"]; !hasN {
			info.PriceData.AddOtherRatio("n", float64(imageN))
		}
	}

	if usage.(*dto.Usage).TotalTokens == 0 {
		usage.(*dto.Usage).TotalTokens = 1
	}
	if usage.(*dto.Usage).PromptTokens == 0 {
		usage.(*dto.Usage).PromptTokens = 1
	}

	quality := "standard"
	if request.Quality == "hd" {
		quality = "hd"
	}

	var logContent []string

	if len(request.Size) > 0 {
		logContent = append(logContent, fmt.Sprintf("大小 %s", request.Size))
	}
	if len(quality) > 0 {
		logContent = append(logContent, fmt.Sprintf("品质 %s", quality))
	}
	if imageN > 0 {
		logContent = append(logContent, fmt.Sprintf("生成数量 %d", imageN))
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), logContent)
	return nil
}
