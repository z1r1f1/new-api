package deepseek

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	deepSeekResponsesRelayContextKey = "deepseek_responses_relay_state"
	deepSeekResponsesMaxSessions     = 256
	deepSeekResponsesSessionTTL      = 7 * 24 * time.Hour
)

type deepSeekResponsesRelayState struct {
	ResponseID       string
	RequestMessages  []dto.Message
	Model            string
	AccumulatedUsage dto.Usage
}

type deepSeekResponsesSessionEntry struct {
	Messages   []dto.Message
	LastUsedAt time.Time
}

var deepSeekResponsesSessionStore = struct {
	sync.Mutex
	Entries map[string]deepSeekResponsesSessionEntry
	Order   []string
}{Entries: make(map[string]deepSeekResponsesSessionEntry)}

type deepSeekStreamToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func convertDeepSeekResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	history := getDeepSeekResponsesHistory(request.PreviousResponseID)
	chatReq, requestMessages, err := deepSeekResponsesToChatRequest(request, history)
	if err != nil {
		return nil, err
	}
	if err := applyDeepSeekV4OpenAIThinkingSuffix(info, chatReq); err != nil {
		return nil, err
	}

	state := &deepSeekResponsesRelayState{
		ResponseID:      newDeepSeekResponseID(),
		RequestMessages: requestMessages,
		Model:           request.Model,
	}
	if c != nil {
		c.Set(deepSeekResponsesRelayContextKey, state)
	}
	return chatReq, nil
}

func deepSeekResponsesToChatRequest(request dto.OpenAIResponsesRequest, history []dto.Message) (*dto.GeneralOpenAIRequest, []dto.Message, error) {
	messages := cloneDeepSeekMessages(history)
	if instructions := rawStringLike(request.Instructions); instructions != "" {
		systemMessage := dto.Message{Role: "system", Content: instructions}
		if len(messages) > 0 && messages[0].Role == "system" {
			messages[0] = systemMessage
		} else {
			messages = append([]dto.Message{systemMessage}, messages...)
		}
	}

	messages, err := deepSeekResponsesInputToChatMessages(request.Input, messages)
	if err != nil {
		return nil, nil, err
	}

	tools, err := deepSeekResponsesToolsToChatTools(request.Tools)
	if err != nil {
		return nil, nil, err
	}

	chatReq := &dto.GeneralOpenAIRequest{
		Model:       request.Model,
		Messages:    messages,
		Stream:      request.Stream,
		MaxTokens:   request.MaxOutputTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Tools:       tools,
	}
	if len(request.ToolChoice) > 0 {
		chatReq.ToolChoice = deepSeekResponsesToolChoiceToChat(request.ToolChoice)
	}
	if request.Stream != nil && *request.Stream {
		chatReq.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return chatReq, cloneDeepSeekMessages(messages), nil
}

func deepSeekResponsesInputToChatMessages(rawInput []byte, existing []dto.Message) ([]dto.Message, error) {
	messages := cloneDeepSeekMessages(existing)
	if len(rawInput) == 0 {
		return messages, nil
	}
	if common.GetJsonType(rawInput) == "string" {
		var text string
		if err := common.Unmarshal(rawInput, &text); err != nil {
			return nil, err
		}
		return append(messages, dto.Message{Role: "user", Content: text}), nil
	}
	if common.GetJsonType(rawInput) != "array" {
		return append(messages, dto.Message{Role: "user", Content: string(rawInput)}), nil
	}

	var items []map[string]any
	if err := common.Unmarshal(rawInput, &items); err != nil {
		return nil, err
	}

	existingCallIDs := collectDeepSeekToolCallIDs(existing)
	existingToolResponses := collectDeepSeekToolResponseIDs(existing)
	for i := 0; i < len(items); {
		item := items[i]
		itemType := common.Interface2String(item["type"])
		switch itemType {
		case "function_call":
			callID := common.Interface2String(item["call_id"])
			if existingCallIDs[callID] {
				i++
				continue
			}
			grouped := make([]map[string]any, 0, 1)
			for i < len(items) {
				cur := items[i]
				if common.Interface2String(cur["type"]) != "function_call" {
					break
				}
				curCallID := common.Interface2String(cur["call_id"])
				if !existingCallIDs[curCallID] {
					grouped = append(grouped, map[string]any{
						"id":   curCallID,
						"type": "function",
						"function": map[string]any{
							"name":      responseFunctionNameForDeepSeekChat(cur),
							"arguments": responseArgumentsForDeepSeekChat(cur["arguments"]),
						},
					})
				}
				i++
			}
			if len(grouped) > 0 {
				rawToolCalls, err := common.Marshal(grouped)
				if err != nil {
					return nil, err
				}
				messages = append(messages, dto.Message{Role: "assistant", Content: nil, ToolCalls: rawToolCalls})
			}
		case "function_call_output":
			callID := common.Interface2String(item["call_id"])
			if !existingToolResponses[callID] {
				messages = append(messages, dto.Message{Role: "tool", Content: common.Interface2String(item["output"]), ToolCallId: callID})
			}
			i++
		case "reasoning":
			i++
		default:
			role := common.Interface2String(item["role"])
			if role == "" {
				role = "user"
			}
			if role == "developer" {
				role = "system"
			}
			msg := dto.Message{Role: role, Content: deepSeekResponsesContentToChatContent(item["content"])}
			if role == "system" {
				if len(messages) > 0 && messages[0].Role == "system" {
					messages[0] = msg
				} else {
					messages = append([]dto.Message{msg}, messages...)
				}
			} else {
				messages = append(messages, msg)
			}
			i++
		}
	}
	return messages, nil
}

func deepSeekResponsesContentToChatContent(content any) any {
	switch value := content.(type) {
	case nil:
		return nil
	case string:
		return value
	case []any:
		hasNonText := false
		for _, partAny := range value {
			part, ok := partAny.(map[string]any)
			if !ok {
				hasNonText = true
				break
			}
			switch common.Interface2String(part["type"]) {
			case "input_text", "text", "output_text":
			default:
				hasNonText = true
			}
		}
		if !hasNonText {
			var b strings.Builder
			for _, partAny := range value {
				if part, ok := partAny.(map[string]any); ok {
					b.WriteString(common.Interface2String(part["text"]))
				}
			}
			return b.String()
		}
		mapped := make([]any, 0, len(value))
		for _, partAny := range value {
			part, ok := partAny.(map[string]any)
			if !ok {
				mapped = append(mapped, partAny)
				continue
			}
			switch common.Interface2String(part["type"]) {
			case "input_text", "text", "output_text":
				mapped = append(mapped, map[string]any{"type": "text", "text": common.Interface2String(part["text"])})
			case "input_image":
				mapped = append(mapped, map[string]any{"type": "image_url", "image_url": map[string]any{"url": common.Interface2String(part["image_url"])}})
			case "image_url":
				imageURL := part["image_url"]
				if url, ok := imageURL.(string); ok {
					imageURL = map[string]any{"url": url}
				}
				mapped = append(mapped, map[string]any{"type": "image_url", "image_url": imageURL})
			default:
				mapped = append(mapped, part)
			}
		}
		return mapped
	default:
		bytes, err := common.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(bytes)
	}
}

func deepSeekResponsesToolsToChatTools(rawTools []byte) ([]dto.ToolCallRequest, error) {
	if len(rawTools) == 0 || common.GetJsonType(rawTools) != "array" {
		return nil, nil
	}
	var tools []map[string]any
	if err := common.Unmarshal(rawTools, &tools); err != nil {
		return nil, err
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		switch common.Interface2String(tool["type"]) {
		case "function":
			converted, ok := deepSeekResponsesFunctionToolToChatTool(tool, "")
			if ok {
				out = append(out, converted)
			}
		case "namespace":
			namespace := common.Interface2String(tool["name"])
			subs, _ := tool["tools"].([]any)
			for _, subAny := range subs {
				sub, ok := subAny.(map[string]any)
				if !ok || common.Interface2String(sub["type"]) != "function" {
					continue
				}
				converted, ok := deepSeekResponsesFunctionToolToChatTool(sub, namespace+common.Interface2String(sub["name"]))
				if ok {
					out = append(out, converted)
				}
			}
		}
	}
	return out, nil
}

func deepSeekResponsesFunctionToolToChatTool(tool map[string]any, overrideName string) (dto.ToolCallRequest, bool) {
	if nested, ok := tool["function"].(map[string]any); ok {
		if overrideName != "" {
			nested["name"] = overrideName
		}
		return toolCallRequestFromMap(map[string]any{"type": "function", "function": nested}), true
	}
	name := overrideName
	if name == "" {
		name = common.Interface2String(tool["name"])
	}
	if strings.TrimSpace(name) == "" {
		return dto.ToolCallRequest{}, false
	}
	return dto.ToolCallRequest{
		Type: "function",
		Function: dto.FunctionRequest{
			Name:        name,
			Description: common.Interface2String(tool["description"]),
			Parameters:  tool["parameters"],
		},
	}, true
}

func toolCallRequestFromMap(tool map[string]any) dto.ToolCallRequest {
	var out dto.ToolCallRequest
	data, err := common.Marshal(tool)
	if err == nil {
		_ = common.Unmarshal(data, &out)
	}
	return out
}

func deepSeekResponsesToolChoiceToChat(raw []byte) any {
	var choice map[string]any
	if err := common.Unmarshal(raw, &choice); err != nil {
		return raw
	}
	if common.Interface2String(choice["type"]) != "function" {
		return choice
	}
	name := common.Interface2String(choice["name"])
	if name == "" {
		return choice
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": name}}
}

func handleDeepSeekResponsesBlocking(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.AppendChannelAffinityResponseDebug(c, responseBody)
	logger.LogDebug(c, "deepseek responses upstream chat body: %s", responseBody)

	var chatResp dto.OpenAITextResponse
	if err := common.Unmarshal(responseBody, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	state := getDeepSeekResponsesRelayState(c, info)
	usage := normalizeDeepSeekResponsesUsage(chatResp.Usage, info, chatResp.Choices)
	assistant := firstDeepSeekAssistantMessage(chatResp.Choices)
	fullHistory := append(cloneDeepSeekMessages(state.RequestMessages), assistant)
	saveDeepSeekResponsesHistory(state.ResponseID, fullHistory)

	payload := buildDeepSeekResponsesResponse(state.ResponseID, state.Model, chatResp.Choices, usage)
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, payloadBytes)
	return usage, nil
}

func handleDeepSeekResponsesStream(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	state := getDeepSeekResponsesRelayState(c, info)
	responseID := state.ResponseID
	model := state.Model
	created := time.Now().Unix()
	messageID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	usage := &dto.Usage{}
	var text strings.Builder
	var reasoning strings.Builder
	toolCalls := make(map[int]*deepSeekStreamToolCall)
	emittedMessage := false
	streamErr := (*types.NewAPIError)(nil)

	writeDeepSeekResponsesStreamEvent(c, "response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"created_at": created,
			"status":     "in_progress",
			"model":      model,
			"output":     []any{},
		},
	})

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			logger.LogError(c, "failed to unmarshal deepseek chat stream chunk: "+err.Error())
			sr.Error(err)
			return
		}
		if chunk.Usage != nil {
			usage = normalizeDeepSeekResponsesUsage(*chunk.Usage, info, nil)
		}
		for _, choice := range chunk.Choices {
			if rc := choice.Delta.GetReasoningContent(); rc != "" {
				reasoning.WriteString(rc)
			}
			if content := choice.Delta.GetContentString(); content != "" {
				if !emittedMessage {
					writeDeepSeekResponsesStreamEvent(c, dto.ResponsesOutputTypeItemAdded, map[string]any{
						"type":         dto.ResponsesOutputTypeItemAdded,
						"output_index": 0,
						"item": map[string]any{
							"type":    "message",
							"id":      messageID,
							"role":    "assistant",
							"status":  "in_progress",
							"content": []any{},
						},
					})
					emittedMessage = true
				}
				text.WriteString(content)
				writeDeepSeekResponsesStreamEvent(c, "response.output_text.delta", map[string]any{
					"type":         "response.output_text.delta",
					"item_id":      messageID,
					"output_index": 0,
					"delta":        content,
				})
			}
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				acc := toolCalls[idx]
				if acc == nil {
					acc = &deepSeekStreamToolCall{}
					toolCalls[idx] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Function.Name != "" {
					acc.Name += tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.Arguments += tc.Function.Arguments
				}
			}
		}
	})

	if info.StreamStatus != nil && info.StreamStatus.EndError != nil {
		return nil, types.NewOpenAIError(info.StreamStatus.EndError, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, text.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}

	assistant := dto.Message{Role: "assistant", Content: text.String()}
	if reasoning.String() != "" {
		rc := reasoning.String()
		assistant.ReasoningContent = &rc
	}
	if len(toolCalls) > 0 {
		assistant.SetNullContent()
		assistant.ToolCalls = deepSeekStreamToolCallsRaw(toolCalls)
	}
	fullHistory := append(cloneDeepSeekMessages(state.RequestMessages), assistant)
	saveDeepSeekResponsesHistory(responseID, fullHistory)

	if emittedMessage {
		writeDeepSeekResponsesStreamEvent(c, dto.ResponsesOutputTypeItemDone, map[string]any{
			"type":         dto.ResponsesOutputTypeItemDone,
			"output_index": 0,
			"item": map[string]any{
				"type":   "message",
				"id":     messageID,
				"role":   "assistant",
				"status": "completed",
				"content": []any{map[string]any{
					"type": "output_text",
					"text": text.String(),
				}},
			},
		})
	}
	for idx := 0; idx < len(toolCalls); idx++ {
		acc := toolCalls[idx]
		if acc == nil {
			continue
		}
		itemID := "fc_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		callID := acc.ID
		if callID == "" {
			callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		_, name := splitDeepSeekMCPFunctionName(acc.Name)
		writeDeepSeekResponsesStreamEvent(c, dto.ResponsesOutputTypeItemAdded, map[string]any{
			"type":         dto.ResponsesOutputTypeItemAdded,
			"output_index": idx,
			"item": map[string]any{
				"type":    "function_call",
				"id":      itemID,
				"call_id": callID,
				"name":    name,
				"status":  "in_progress",
			},
		})
		writeDeepSeekResponsesStreamEvent(c, "response.function_call_arguments.delta", map[string]any{
			"type":         "response.function_call_arguments.delta",
			"output_index": idx,
			"item_id":      itemID,
			"delta":        acc.Arguments,
		})
		writeDeepSeekResponsesStreamEvent(c, "response.function_call_arguments.done", map[string]any{
			"type":         "response.function_call_arguments.done",
			"output_index": idx,
			"item_id":      itemID,
		})
		writeDeepSeekResponsesStreamEvent(c, dto.ResponsesOutputTypeItemDone, map[string]any{
			"type":         dto.ResponsesOutputTypeItemDone,
			"output_index": idx,
			"item":         buildDeepSeekResponsesFunctionCallItem(itemID, callID, acc.Name, acc.Arguments, "completed"),
		})
	}
	writeDeepSeekResponsesStreamEvent(c, "response.completed", map[string]any{
		"type":     "response.completed",
		"response": buildDeepSeekResponsesResponseFromParts(responseID, model, created, text.String(), toolCalls, usage),
	})
	return usage, nil
}

func writeDeepSeekResponsesStreamEvent(c *gin.Context, eventType string, payload any) {
	data, err := common.Marshal(payload)
	if err != nil {
		logger.LogError(c, "failed to marshal deepseek responses stream event: "+err.Error())
		return
	}
	helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType}, string(data))
}

func buildDeepSeekResponsesResponse(responseID string, model string, choices []dto.OpenAITextResponseChoice, usage *dto.Usage) map[string]any {
	created := time.Now().Unix()
	outputs := make([]any, 0)
	if len(choices) == 0 {
		outputs = append(outputs, buildDeepSeekResponsesMessageItem("msg_"+strings.ReplaceAll(uuid.NewString(), "-", ""), "", "completed"))
	} else {
		message := choices[0].Message
		text := message.StringContent()
		var toolCalls []dto.ToolCallResponse
		if len(message.ToolCalls) > 0 {
			_ = common.Unmarshal(message.ToolCalls, &toolCalls)
		}
		if text != "" || len(toolCalls) == 0 {
			outputs = append(outputs, buildDeepSeekResponsesMessageItem("msg_"+strings.ReplaceAll(uuid.NewString(), "-", ""), text, "completed"))
		}
		for _, tc := range toolCalls {
			outputs = append(outputs, buildDeepSeekResponsesFunctionCallItem("fc_"+strings.ReplaceAll(uuid.NewString(), "-", ""), tc.ID, tc.Function.Name, tc.Function.Arguments, "completed"))
		}
	}
	return map[string]any{
		"id":                  responseID,
		"object":              "response",
		"created_at":          created,
		"status":              "completed",
		"model":               model,
		"output":              outputs,
		"usage":               responsesUsageForDeepSeek(usage),
		"parallel_tool_calls": true,
		"tool_choice":         "auto",
		"tools":               []any{},
		"truncation":          "disabled",
	}
}

func buildDeepSeekResponsesResponseFromParts(responseID string, model string, created int64, text string, toolCalls map[int]*deepSeekStreamToolCall, usage *dto.Usage) map[string]any {
	outputs := make([]any, 0)
	if text != "" || len(toolCalls) == 0 {
		outputs = append(outputs, buildDeepSeekResponsesMessageItem("msg_"+strings.ReplaceAll(uuid.NewString(), "-", ""), text, "completed"))
	}
	for idx := 0; idx < len(toolCalls); idx++ {
		acc := toolCalls[idx]
		if acc == nil {
			continue
		}
		callID := acc.ID
		if callID == "" {
			callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		outputs = append(outputs, buildDeepSeekResponsesFunctionCallItem("fc_"+strings.ReplaceAll(uuid.NewString(), "-", ""), callID, acc.Name, acc.Arguments, "completed"))
	}
	return map[string]any{
		"id":                  responseID,
		"object":              "response",
		"created_at":          created,
		"status":              "completed",
		"model":               model,
		"output":              outputs,
		"usage":               responsesUsageForDeepSeek(usage),
		"parallel_tool_calls": true,
		"tool_choice":         "auto",
		"tools":               []any{},
		"truncation":          "disabled",
	}
}

func buildDeepSeekResponsesMessageItem(itemID string, text string, status string) map[string]any {
	return map[string]any{
		"type":   "message",
		"id":     itemID,
		"status": status,
		"role":   "assistant",
		"content": []any{map[string]any{
			"type": "output_text",
			"text": text,
		}},
	}
}

func buildDeepSeekResponsesFunctionCallItem(itemID string, callID string, rawName string, args string, status string) map[string]any {
	namespace, name := splitDeepSeekMCPFunctionName(rawName)
	if callID == "" {
		callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	item := map[string]any{
		"type":      "function_call",
		"id":        itemID,
		"status":    status,
		"call_id":   callID,
		"name":      name,
		"arguments": args,
	}
	if namespace != "" {
		item["namespace"] = namespace
	}
	return item
}

func normalizeDeepSeekResponsesUsage(usage dto.Usage, info *relaycommon.RelayInfo, choices []dto.OpenAITextResponseChoice) *dto.Usage {
	out := usage
	if out.PromptTokens == 0 && out.InputTokens != 0 {
		out.PromptTokens = out.InputTokens
	}
	if out.CompletionTokens == 0 && out.OutputTokens != 0 {
		out.CompletionTokens = out.OutputTokens
	}
	if out.InputTokens == 0 {
		out.InputTokens = out.PromptTokens
	}
	if out.OutputTokens == 0 {
		out.OutputTokens = out.CompletionTokens
	}
	if out.CompletionTokens == 0 && choices != nil {
		for _, choice := range choices {
			out.CompletionTokens += service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
		}
		out.OutputTokens = out.CompletionTokens
	}
	if out.PromptTokens == 0 && info != nil {
		out.PromptTokens = info.GetEstimatePromptTokens()
		out.InputTokens = out.PromptTokens
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	return &out
}

func responsesUsageForDeepSeek(usage *dto.Usage) map[string]any {
	if usage == nil {
		usage = &dto.Usage{}
	}
	inputTokens := usage.InputTokens
	if inputTokens == 0 {
		inputTokens = usage.PromptTokens
	}
	outputTokens := usage.OutputTokens
	if outputTokens == 0 {
		outputTokens = usage.CompletionTokens
	}
	totalTokens := usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	return map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"total_tokens":  totalTokens,
	}
}

func firstDeepSeekAssistantMessage(choices []dto.OpenAITextResponseChoice) dto.Message {
	if len(choices) == 0 {
		return dto.Message{Role: "assistant", Content: ""}
	}
	msg := choices[0].Message
	msg.Role = "assistant"
	return msg
}

func getDeepSeekResponsesRelayState(c *gin.Context, info *relaycommon.RelayInfo) *deepSeekResponsesRelayState {
	if c != nil {
		if value, ok := c.Get(deepSeekResponsesRelayContextKey); ok {
			if state, ok := value.(*deepSeekResponsesRelayState); ok && state != nil {
				return state
			}
		}
	}
	model := ""
	if info != nil {
		model = info.OriginModelName
		if info.ChannelMeta != nil && info.ChannelMeta.UpstreamModelName != "" {
			model = info.ChannelMeta.UpstreamModelName
		}
	}
	return &deepSeekResponsesRelayState{ResponseID: newDeepSeekResponseID(), Model: model}
}

func getDeepSeekResponsesHistory(responseID string) []dto.Message {
	if strings.TrimSpace(responseID) == "" {
		return nil
	}
	deepSeekResponsesSessionStore.Lock()
	defer deepSeekResponsesSessionStore.Unlock()
	pruneDeepSeekResponsesSessionsLocked(time.Now())
	entry, ok := deepSeekResponsesSessionStore.Entries[responseID]
	if !ok {
		return nil
	}
	entry.LastUsedAt = time.Now()
	deepSeekResponsesSessionStore.Entries[responseID] = entry
	return cloneDeepSeekMessages(entry.Messages)
}

func saveDeepSeekResponsesHistory(responseID string, messages []dto.Message) {
	if strings.TrimSpace(responseID) == "" {
		return
	}
	deepSeekResponsesSessionStore.Lock()
	defer deepSeekResponsesSessionStore.Unlock()
	now := time.Now()
	if _, exists := deepSeekResponsesSessionStore.Entries[responseID]; !exists {
		deepSeekResponsesSessionStore.Order = append(deepSeekResponsesSessionStore.Order, responseID)
	}
	deepSeekResponsesSessionStore.Entries[responseID] = deepSeekResponsesSessionEntry{Messages: cloneDeepSeekMessages(messages), LastUsedAt: now}
	pruneDeepSeekResponsesSessionsLocked(now)
}

func pruneDeepSeekResponsesSessionsLocked(now time.Time) {
	writeAt := 0
	for _, id := range deepSeekResponsesSessionStore.Order {
		entry, ok := deepSeekResponsesSessionStore.Entries[id]
		if !ok || now.Sub(entry.LastUsedAt) > deepSeekResponsesSessionTTL {
			delete(deepSeekResponsesSessionStore.Entries, id)
			continue
		}
		deepSeekResponsesSessionStore.Order[writeAt] = id
		writeAt++
	}
	deepSeekResponsesSessionStore.Order = deepSeekResponsesSessionStore.Order[:writeAt]
	for len(deepSeekResponsesSessionStore.Order) > deepSeekResponsesMaxSessions {
		oldest := deepSeekResponsesSessionStore.Order[0]
		deepSeekResponsesSessionStore.Order = deepSeekResponsesSessionStore.Order[1:]
		delete(deepSeekResponsesSessionStore.Entries, oldest)
	}
}

func cloneDeepSeekMessages(messages []dto.Message) []dto.Message {
	if len(messages) == 0 {
		return nil
	}
	data, err := common.Marshal(messages)
	if err != nil {
		cloned := make([]dto.Message, len(messages))
		copy(cloned, messages)
		return cloned
	}
	var cloned []dto.Message
	if err := common.Unmarshal(data, &cloned); err != nil {
		fallback := make([]dto.Message, len(messages))
		copy(fallback, messages)
		return fallback
	}
	return cloned
}

func newDeepSeekResponseID() string {
	return "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func collectDeepSeekToolCallIDs(messages []dto.Message) map[string]bool {
	ids := make(map[string]bool)
	for _, msg := range messages {
		var toolCalls []dto.ToolCallRequest
		if len(msg.ToolCalls) > 0 && common.Unmarshal(msg.ToolCalls, &toolCalls) == nil {
			for _, tc := range toolCalls {
				if tc.ID != "" {
					ids[tc.ID] = true
				}
			}
		}
		if msg.ToolCallId != "" {
			ids[msg.ToolCallId] = true
		}
	}
	return ids
}

func collectDeepSeekToolResponseIDs(messages []dto.Message) map[string]bool {
	ids := make(map[string]bool)
	for _, msg := range messages {
		if msg.ToolCallId != "" {
			ids[msg.ToolCallId] = true
		}
	}
	return ids
}

func responseFunctionNameForDeepSeekChat(item map[string]any) string {
	return common.Interface2String(item["namespace"]) + common.Interface2String(item["name"])
}

func responseArgumentsForDeepSeekChat(arguments any) string {
	if s, ok := arguments.(string); ok {
		if strings.TrimSpace(s) == "" {
			return "{}"
		}
		return s
	}
	if arguments == nil {
		return "{}"
	}
	data, err := common.Marshal(arguments)
	if err != nil || len(data) == 0 {
		return "{}"
	}
	return string(data)
}

func rawStringLike(raw []byte) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if common.GetJsonType(raw) == "string" {
		var s string
		if common.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	return strings.TrimSpace(string(raw))
}

func splitDeepSeekMCPFunctionName(name string) (string, string) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", name
	}
	rest := strings.TrimPrefix(name, "mcp__")
	serverEnd := strings.Index(rest, "__")
	if serverEnd < 0 {
		return "", name
	}
	splitAt := len("mcp__") + serverEnd + len("__")
	if splitAt >= len(name) {
		return "", name
	}
	return name[:splitAt], name[splitAt:]
}

func deepSeekStreamToolCallsRaw(toolCalls map[int]*deepSeekStreamToolCall) []byte {
	out := make([]map[string]any, 0, len(toolCalls))
	for idx := 0; idx < len(toolCalls); idx++ {
		acc := toolCalls[idx]
		if acc == nil {
			continue
		}
		callID := acc.ID
		if callID == "" {
			callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		out = append(out, map[string]any{
			"id":   callID,
			"type": "function",
			"function": map[string]any{
				"name":      acc.Name,
				"arguments": acc.Arguments,
			},
		})
	}
	data, _ := common.Marshal(out)
	return data
}

func isDeepSeekResponsesRelay(info *relaycommon.RelayInfo) bool {
	return info != nil && info.RelayMode == relayconstant.RelayModeResponses
}

func deepSeekFinishReasonContentFilter(choices []dto.OpenAITextResponseChoice) bool {
	for _, choice := range choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			return true
		}
	}
	return false
}
