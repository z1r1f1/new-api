package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func responsesStreamIndexKey(itemID string, idx *int) string {
	if itemID == "" {
		return ""
	}
	if idx == nil {
		return itemID
	}
	return fmt.Sprintf("%s:%d", itemID, *idx)
}

func stringDeltaFromPrefix(prev string, next string) string {
	if next == "" {
		return ""
	}
	if prev != "" && strings.HasPrefix(next, prev) {
		return next[len(prev):]
	}
	return next
}

type responsesToolCallTextPayload struct {
	ToolCall  *responsesToolCallTextSpec  `json:"tool_call"`
	ToolCalls []responsesToolCallTextSpec `json:"tool_calls"`
	Name      string                      `json:"name"`
	Arguments json.RawMessage             `json:"arguments"`
	Function  *responsesToolCallFunction  `json:"function"`
}

type responsesToolCallTextSpec struct {
	ID        string                     `json:"id"`
	Name      string                     `json:"name"`
	Arguments json.RawMessage            `json:"arguments"`
	Function  *responsesToolCallFunction `json:"function"`
}

type responsesToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func claudeAllowedToolNames(info *relaycommon.RelayInfo) map[string]string {
	if info == nil || info.Request == nil {
		return nil
	}
	claudeReq, ok := info.Request.(*dto.ClaudeRequest)
	if !ok || claudeReq == nil || claudeReq.Tools == nil {
		return nil
	}
	tools, err := common.Any2Type[[]dto.Tool](claudeReq.Tools)
	if err != nil || len(tools) == 0 {
		return nil
	}
	allowed := make(map[string]string, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		allowed[strings.ToLower(name)] = name
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func parseResponsesToolCallJSONText(content string, allowed map[string]string) (dto.ToolCallResponse, bool) {
	if len(allowed) == 0 {
		return dto.ToolCallResponse{}, false
	}
	for _, candidate := range responsesToolCallJSONTextCandidates(content) {
		var payload responsesToolCallTextPayload
		if err := common.Unmarshal(common.StringToByteSlice(candidate), &payload); err != nil {
			continue
		}
		if call, ok := responsesToolCallFromTextPayload(payload, allowed); ok {
			return call, true
		}
	}
	return dto.ToolCallResponse{}, false
}

func responsesToolCallFromTextPayload(payload responsesToolCallTextPayload, allowed map[string]string) (dto.ToolCallResponse, bool) {
	if payload.ToolCall != nil {
		return responsesToolCallFromTextSpec(*payload.ToolCall, allowed)
	}
	for _, spec := range payload.ToolCalls {
		if call, ok := responsesToolCallFromTextSpec(spec, allowed); ok {
			return call, true
		}
	}
	return responsesToolCallFromTextSpec(responsesToolCallTextSpec{
		Name:      payload.Name,
		Arguments: payload.Arguments,
		Function:  payload.Function,
	}, allowed)
}

func responsesToolCallFromTextSpec(spec responsesToolCallTextSpec, allowed map[string]string) (dto.ToolCallResponse, bool) {
	name := strings.TrimSpace(spec.Name)
	args := spec.Arguments
	if spec.Function != nil {
		if name == "" {
			name = strings.TrimSpace(spec.Function.Name)
		}
		if len(args) == 0 {
			args = spec.Function.Arguments
		}
	}
	canonicalName, ok := allowed[strings.ToLower(name)]
	if !ok || canonicalName == "" {
		return dto.ToolCallResponse{}, false
	}
	callID := strings.TrimSpace(spec.ID)
	if callID == "" {
		callID = "call_" + strings.ReplaceAll(common.GetUUID(), "-", "")
	}
	return dto.ToolCallResponse{
		ID:   callID,
		Type: "function",
		Function: dto.FunctionResponse{
			Name:      canonicalName,
			Arguments: normalizeResponsesToolCallTextArguments(args),
		},
	}, true
}

func normalizeResponsesToolCallTextArguments(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "{}"
	}
	if common.GetJsonType(raw) == "string" {
		var value string
		if err := common.Unmarshal(raw, &value); err != nil {
			return "{}"
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return "{}"
		}
		var decoded any
		if err := common.Unmarshal(common.StringToByteSlice(value), &decoded); err == nil {
			return value
		}
		data, err := common.Marshal(map[string]string{"value": value})
		if err != nil {
			return "{}"
		}
		return string(data)
	}
	var decoded any
	if err := common.Unmarshal(raw, &decoded); err != nil {
		return "{}"
	}
	return string(raw)
}

func responsesToolCallJSONTextCandidates(content string) []string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}
	seen := make(map[string]bool)
	add := func(out *[]string, candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		*out = append(*out, candidate)
	}
	candidates := make([]string, 0, 4)
	add(&candidates, trimmed)
	unfenced := stripResponsesToolCallJSONFence(trimmed)
	add(&candidates, unfenced)
	for _, candidate := range balancedResponsesToolCallJSONObjects(unfenced) {
		add(&candidates, candidate)
	}
	return candidates
}

func shouldBufferResponsesToolCallText(content string) bool {
	trimmed := strings.TrimLeft(strings.TrimSpace(content), "\ufeff")
	if trimmed == "" {
		return true
	}
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "```")
}

func stripResponsesToolCallJSONFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 || !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return trimmed
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func balancedResponsesToolCallJSONObjects(content string) []string {
	out := make([]string, 0, 2)
	start := -1
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					out = append(out, content[start:i+1])
					start = -1
				}
			}
		}
	}
	return out
}

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	chatId := helper.GetResponseID(c)
	chatResp, usage, err := service.ResponsesResponseToChatCompletionsResponse(&responsesResp, chatId)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	var responseBody []byte
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(chatResp, info)
		responseBody, err = common.Marshal(claudeResp)
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(chatResp, info)
		responseBody, err = common.Marshal(geminiResp)
	default:
		responseBody, err = common.Marshal(chatResp)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	model := info.UpstreamModelName

	var (
		usage       = &dto.Usage{}
		outputText  strings.Builder
		usageText   strings.Builder
		sentStart   bool
		sentStop    bool
		sawToolCall bool
		streamErr   *types.NewAPIError
	)

	toolCallIndexByID := make(map[string]int)
	toolCallNameByID := make(map[string]string)
	toolCallArgsByID := make(map[string]string)
	toolCallNameSent := make(map[string]bool)
	toolCallCanonicalIDByItemID := make(map[string]string)
	messageTextByItemID := make(map[string]string)
	hasSentReasoningSummary := false
	needsReasoningSummarySeparator := false
	//reasoningSummaryTextByKey := make(map[string]string)

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendChatChunk := func(chunk *dto.ChatCompletionsStreamResponse) bool {
		if chunk == nil {
			return true
		}
		if info.RelayFormat == types.RelayFormatOpenAI {
			if err := helper.ObjectData(c, chunk); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		}

		chunkData, err := common.Marshal(chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := HandleStreamFormat(c, info, string(chunkData), false, false); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}

	sendStartIfNeeded := func() bool {
		if sentStart {
			return true
		}
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
			return false
		}
		sentStart = true
		return true
	}

	//sendReasoningDelta := func(delta string) bool {
	//	if delta == "" {
	//		return true
	//	}
	//	if !sendStartIfNeeded() {
	//		return false
	//	}
	//
	//	usageText.WriteString(delta)
	//	chunk := &dto.ChatCompletionsStreamResponse{
	//		Id:      responseId,
	//		Object:  "chat.completion.chunk",
	//		Created: createAt,
	//		Model:   model,
	//		Choices: []dto.ChatCompletionsStreamResponseChoice{
	//			{
	//				Index: 0,
	//				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
	//					ReasoningContent: &delta,
	//				},
	//			},
	//		},
	//	}
	//	if err := helper.ObjectData(c, chunk); err != nil {
	//		streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	//		return false
	//	}
	//	return true
	//}

	sendReasoningSummaryDelta := func(delta string) bool {
		if delta == "" {
			return true
		}
		if needsReasoningSummarySeparator {
			if strings.HasPrefix(delta, "\n\n") {
				needsReasoningSummarySeparator = false
			} else if strings.HasPrefix(delta, "\n") {
				delta = "\n" + delta
				needsReasoningSummarySeparator = false
			} else {
				delta = "\n\n" + delta
				needsReasoningSummarySeparator = false
			}
		}
		if !sendStartIfNeeded() {
			return false
		}

		usageText.WriteString(delta)
		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ReasoningContent: &delta,
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
			return false
		}
		hasSentReasoningSummary = true
		return true
	}

	sendToolCallDelta := func(callID string, name string, argsDelta string) bool {
		if callID == "" {
			return true
		}
		if !sendStartIfNeeded() {
			return false
		}

		idx, ok := toolCallIndexByID[callID]
		if !ok {
			idx = len(toolCallIndexByID)
			toolCallIndexByID[callID] = idx
		}
		if name != "" {
			toolCallNameByID[callID] = name
		}
		if toolCallNameByID[callID] != "" {
			name = toolCallNameByID[callID]
		}

		tool := dto.ToolCallResponse{
			ID:   callID,
			Type: "function",
			Function: dto.FunctionResponse{
				Arguments: argsDelta,
			},
		}
		tool.SetIndex(idx)
		if name != "" && !toolCallNameSent[callID] {
			tool.Function.Name = name
			toolCallNameSent[callID] = true
		}

		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{tool},
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
			return false
		}
		sawToolCall = true

		// Include tool call data in the local builder for fallback token estimation.
		if tool.Function.Name != "" {
			usageText.WriteString(tool.Function.Name)
		}
		if argsDelta != "" {
			usageText.WriteString(argsDelta)
		}
		return true
	}

	allowedToolNames := claudeAllowedToolNames(info)
	toolTextBridgeEnabled := info.RelayFormat == types.RelayFormatClaude && len(allowedToolNames) > 0
	var toolTextBuffer strings.Builder
	consumedToolTextSnapshot := ""

	sendOutputTextDelta := func(delta string) bool {
		if delta == "" {
			return true
		}
		if !sendStartIfNeeded() {
			return false
		}
		outputText.WriteString(delta)
		usageText.WriteString(delta)
		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						Content: &delta,
					},
				},
			},
		}
		return sendChatChunk(chunk)
	}

	sendOutputTextSnapshot := func(key string, text string) bool {
		if text == "" {
			return true
		}
		key = strings.TrimSpace(key)
		if key == "" {
			key = "__response__"
		}
		if prev := messageTextByItemID[key]; prev != "" && prev == text {
			return true
		}

		if toolTextBridgeEnabled && outputText.Len() == 0 && toolTextBuffer.Len() == 0 {
			if consumedToolTextSnapshot == text {
				messageTextByItemID[key] = text
				return true
			}
			if toolCall, ok := parseResponsesToolCallJSONText(text, allowedToolNames); ok {
				if !sendToolCallDelta(toolCall.ID, toolCall.Function.Name, "") {
					return false
				}
				if !sendToolCallDelta(toolCall.ID, "", toolCall.Function.Arguments) {
					return false
				}
				consumedToolTextSnapshot = text
				messageTextByItemID[key] = text
				return true
			}
		}

		delta := text
		sentText := outputText.String()
		if strings.HasPrefix(text, sentText) {
			delta = text[len(sentText):]
		} else if prev := messageTextByItemID[key]; prev != "" && strings.HasPrefix(text, prev) {
			delta = text[len(prev):]
		}
		if delta == "" {
			messageTextByItemID[key] = text
			return true
		}
		if !sendOutputTextDelta(delta) {
			return false
		}
		messageTextByItemID[key] = text
		return true
	}

	flushToolTextBuffer := func() bool {
		if toolTextBuffer.Len() == 0 {
			return true
		}
		text := toolTextBuffer.String()
		toolTextBuffer.Reset()
		return sendOutputTextDelta(text)
	}

	finalizeToolTextBuffer := func() bool {
		if toolTextBuffer.Len() == 0 {
			return true
		}
		text := toolTextBuffer.String()
		toolTextBuffer.Reset()
		if toolCall, ok := parseResponsesToolCallJSONText(text, allowedToolNames); ok {
			if !sendToolCallDelta(toolCall.ID, toolCall.Function.Name, "") {
				return false
			}
			return sendToolCallDelta(toolCall.ID, "", toolCall.Function.Arguments)
		}
		return sendOutputTextDelta(text)
	}

	responseCompleted := false

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			sr.Error(err)
			return
		}

		switch streamResp.Type {
		case "response.created":
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
			}

		//case "response.reasoning_text.delta":
		//if !sendReasoningDelta(streamResp.Delta) {
		//	sr.Stop(streamErr)
		//	return
		//}

		//case "response.reasoning_text.done":

		case "response.reasoning_summary_text.delta":
			if !sendReasoningSummaryDelta(streamResp.Delta) {
				sr.Stop(streamErr)
				return
			}

		case "response.reasoning_summary_text.done":
			if hasSentReasoningSummary {
				needsReasoningSummarySeparator = true
			}

		//case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		//	key := responsesStreamIndexKey(strings.TrimSpace(streamResp.ItemID), streamResp.SummaryIndex)
		//	if key == "" || streamResp.Part == nil {
		//		break
		//	}
		//	// Only handle summary text parts, ignore other part types.
		//	if streamResp.Part.Type != "" && streamResp.Part.Type != "summary_text" {
		//		break
		//	}
		//	prev := reasoningSummaryTextByKey[key]
		//	next := streamResp.Part.Text
		//	delta := stringDeltaFromPrefix(prev, next)
		//	reasoningSummaryTextByKey[key] = next
		//	if !sendReasoningSummaryDelta(delta) {
		//		sr.Stop(streamErr)
		//		return
		//	}

		case "response.output_text.delta":
			if streamResp.Delta != "" {
				delta := streamResp.Delta
				if toolTextBridgeEnabled && (toolTextBuffer.Len() > 0 || shouldBufferResponsesToolCallText(delta)) {
					toolTextBuffer.WriteString(delta)
					if shouldBufferResponsesToolCallText(toolTextBuffer.String()) {
						break
					}
					if !flushToolTextBuffer() {
						sr.Stop(streamErr)
						return
					}
					break
				}
				if !sendOutputTextDelta(delta) {
					sr.Stop(streamErr)
					return
				}
			}

		case "response.output_item.added", "response.output_item.done":
			if !finalizeToolTextBuffer() {
				sr.Stop(streamErr)
				return
			}
			if streamResp.Item == nil {
				break
			}
			if streamResp.Item.Type == "message" {
				if !sendOutputTextSnapshot(streamResp.Item.ID, responsesMessageOutputText(streamResp.Item)) {
					sr.Stop(streamErr)
					return
				}
				break
			}
			if streamResp.Item.Type != "function_call" {
				break
			}

			itemID := strings.TrimSpace(streamResp.Item.ID)
			callID := strings.TrimSpace(streamResp.Item.CallId)
			if callID == "" {
				callID = itemID
			}
			if itemID != "" && callID != "" {
				toolCallCanonicalIDByItemID[itemID] = callID
			}
			name := strings.TrimSpace(streamResp.Item.Name)
			if name != "" {
				toolCallNameByID[callID] = name
			}

			newArgs := streamResp.Item.ArgumentsString()
			prevArgs := toolCallArgsByID[callID]
			argsDelta := ""
			if newArgs != "" {
				if strings.HasPrefix(newArgs, prevArgs) {
					argsDelta = newArgs[len(prevArgs):]
				} else {
					argsDelta = newArgs
				}
				toolCallArgsByID[callID] = newArgs
			}

			if !sendToolCallDelta(callID, name, argsDelta) {
				sr.Stop(streamErr)
				return
			}

		case "response.function_call_arguments.delta":
			itemID := strings.TrimSpace(streamResp.ItemID)
			callID := toolCallCanonicalIDByItemID[itemID]
			if callID == "" {
				callID = itemID
			}
			if callID == "" {
				break
			}
			toolCallArgsByID[callID] += streamResp.Delta
			if !sendToolCallDelta(callID, "", streamResp.Delta) {
				sr.Stop(streamErr)
				return
			}

		case "response.function_call_arguments.done":

		case "response.completed":
			responseCompleted = true
			if !finalizeToolTextBuffer() {
				sr.Stop(streamErr)
				return
			}
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
				if !sendOutputTextSnapshot("__response__", service.ExtractOutputTextFromResponses(streamResp.Response)) {
					sr.Stop(streamErr)
					return
				}
				if streamResp.Response.Usage != nil {
					if streamResp.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResp.Response.Usage.InputTokens
						usage.InputTokens = streamResp.Response.Usage.InputTokens
					}
					if streamResp.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResp.Response.Usage.OutputTokens
						usage.OutputTokens = streamResp.Response.Usage.OutputTokens
					}
					if streamResp.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResp.Response.Usage.TotalTokens
					} else {
						usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
					}
					if streamResp.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResp.Response.Usage.InputTokensDetails.CachedTokens
						usage.PromptTokensDetails.ImageTokens = streamResp.Response.Usage.InputTokensDetails.ImageTokens
						usage.PromptTokensDetails.AudioTokens = streamResp.Response.Usage.InputTokensDetails.AudioTokens
					}
					if streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
						usage.CompletionTokenDetails.ReasoningTokens = streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens
					}
				}
			}

			if !sendStartIfNeeded() {
				sr.Stop(streamErr)
				return
			}
			if !sentStop {
				if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
					info.ClaudeConvertInfo.Usage = usage
				}
				finishReason := "stop"
				if sawToolCall {
					finishReason = "tool_calls"
				}
				stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
				if !sendChatChunk(stop) {
					sr.Stop(streamErr)
					return
				}
				sentStop = true
			}
			sr.Done()

		case "response.error", "response.failed":
			service.AppendChannelAffinityResponseDebug(c, []byte(data))
			streamErr = newResponsesStreamAPIError(streamResp, data)
			sr.Stop(streamErr)
			return

		default:
		}
	})
	if responseCompleted && streamErr == nil {
		markResponsesStreamCompleted(info)
	}

	if streamErr != nil {
		return nil, streamErr
	}

	if !finalizeToolTextBuffer() {
		if streamErr != nil {
			return nil, streamErr
		}
		return nil, types.NewOpenAIError(fmt.Errorf("failed to flush responses tool-call text buffer"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, usageText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}

	if !sentStart {
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
			return nil, streamErr
		}
	}
	if !sentStop {
		if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
			info.ClaudeConvertInfo.Usage = usage
		}
		finishReason := "stop"
		if sawToolCall {
			finishReason = "tool_calls"
		}
		stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
		if !sendChatChunk(stop) {
			return nil, streamErr
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}

func responsesMessageOutputText(item *dto.ResponsesOutput) string {
	if item == nil || item.Type != "message" || len(item.Content) == 0 {
		return ""
	}
	var text strings.Builder
	for _, content := range item.Content {
		if content.Type == "output_text" && content.Text != "" {
			text.WriteString(content.Text)
		}
	}
	if text.Len() > 0 {
		return text.String()
	}
	for _, content := range item.Content {
		if content.Text != "" {
			text.WriteString(content.Text)
		}
	}
	return text.String()
}
