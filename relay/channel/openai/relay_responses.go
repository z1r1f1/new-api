package openai

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responseBody = normalizeResponsesResponseBodyOutput(responseBody, &responsesResponse)
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	responseBody = rewriteSGLangResponsesCreatedAt(info, responseBody, "created_at", responsesResponse.CreatedAt)

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, responsesResponse.Usage)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	accumulator := service.NewResponsesUsageAccumulator(info)
	responseCompleted := false
	var streamErr *types.NewAPIError

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if streamResponse.Response != nil {
			data = string(rewriteSGLangResponsesCreatedAt(info, []byte(data), "response.created_at", streamResponse.Response.CreatedAt))
		}
		if streamResponse.Type == "response.completed" {
			data = normalizeResponsesStreamResponseOutput(data, &streamResponse)
		}
		sendResponsesStreamData(c, streamResponse, data)
		accumulator.Observe(&streamResponse)
		switch streamResponse.Type {
		case "response.completed", "response.done":
			responseCompleted = true
			sr.Done()
		case "response.error", "response.failed":
			streamErr = newResponsesStreamAPIError(streamResponse, data)
			sr.Stop(streamErr)
		}
	})

	if responseCompleted {
		markResponsesStreamCompleted(info)
	}
	if streamErr != nil {
		return nil, streamErr
	}

	return accumulator.Finish(), nil
}

func markResponsesStreamCompleted(info *relaycommon.RelayInfo) {
	if info == nil || info.StreamStatus == nil {
		return
	}
	info.StreamStatus.EndReason = relaycommon.StreamEndReasonDone
	info.StreamStatus.EndError = nil
}

func normalizeResponsesResponseBodyOutput(data []byte, resp *dto.OpenAIResponsesResponse) []byte {
	if resp == nil || resp.Output != nil {
		return data
	}
	resp.Output = []dto.ResponsesOutput{}

	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return data
	}
	if value, exists := payload["output"]; exists && value != nil {
		return data
	}
	payload["output"] = []any{}
	normalized, err := common.Marshal(payload)
	if err != nil {
		return data
	}
	return normalized
}

func normalizeResponsesStreamResponseOutput(data string, streamResp *dto.ResponsesStreamResponse) string {
	if streamResp == nil || streamResp.Response == nil || streamResp.Response.Output != nil {
		return data
	}
	streamResp.Response.Output = []dto.ResponsesOutput{}

	var payload map[string]any
	if err := common.UnmarshalJsonStr(data, &payload); err != nil {
		return data
	}
	response, ok := payload["response"].(map[string]any)
	if !ok || response == nil {
		return data
	}
	if value, exists := response["output"]; exists && value != nil {
		return data
	}
	response["output"] = []any{}
	normalized, err := common.Marshal(payload)
	if err != nil {
		return data
	}
	return string(normalized)
}

func rewriteSGLangResponsesCreatedAt(info *relaycommon.RelayInfo, payload []byte, path string, createdAt dto.IntValue) []byte {
	if info == nil || info.GetChannelType() != constant.ChannelTypeSGLang {
		return payload
	}
	if !gjson.GetBytes(payload, path).Exists() {
		return payload
	}
	patched, err := sjson.SetBytes(payload, path, int(createdAt))
	if err != nil {
		return payload
	}
	return patched
}
