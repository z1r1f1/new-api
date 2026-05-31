package chatgptimg

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/samber/hot"
)

var ModelList = []string{
	"gpt-image-2",
	"gpt-5.5-pro",
	"gpt-5.5-thinking",
	"gpt-5.4-thinking",
	"gpt-5.4-pro",
	"gpt-5.4-instant",
}

const ChannelName = "chatgpt-web"

type Adaptor struct{}

type generationRequest struct {
	Model                   string   `json:"model"`
	Prompt                  string   `json:"prompt"`
	N                       int      `json:"n,omitempty"`
	Size                    string   `json:"size,omitempty"`
	Quality                 string   `json:"quality,omitempty"`
	Style                   string   `json:"style,omitempty"`
	ResponseFormat          string   `json:"response_format,omitempty"`
	ReferenceImages         []string `json:"reference_images,omitempty"`
	FallbackPrompt          string   `json:"fallback_prompt,omitempty"`
	FallbackReferenceImages []string `json:"fallback_reference_images,omitempty"`
	ConversationID          string   `json:"conversation_id,omitempty"`
}

type generationResponse struct {
	Created        int64           `json:"created"`
	Data           []dto.ImageData `json:"data"`
	Usage          dto.Usage       `json:"usage"`
	ConversationID string          `json:"conversation_id,omitempty"`
}

type imageRunResult struct {
	ConversationID string
	FileRefs       []string
	SignedURLs     []string
	IsPreview      bool
	TurnsInConv    int
}

type chatRequest struct {
	Model          string                `json:"model,omitempty"`
	Messages       []dto.Message         `json:"messages,omitempty"`
	Stream         *bool                 `json:"stream,omitempty"`
	Tools          []dto.ToolCallRequest `json:"tools,omitempty"`
	ToolChoice     any                   `json:"tool_choice,omitempty"`
	ThinkingEffort string                `json:"thinking_effort,omitempty"`
	DeepResearch   bool                  `json:"chatgpt_web_deep_research,omitempty"`
	ResponseFormat *dto.ResponseFormat   `json:"response_format,omitempty"`
	FallbackPrompt string                `json:"fallback_prompt,omitempty"`
	ConversationID string                `json:"conversation_id,omitempty"`
}

type chatResponse struct {
	Id             string                         `json:"id"`
	Object         string                         `json:"object"`
	Created        int64                          `json:"created"`
	Model          string                         `json:"model"`
	Choices        []dto.OpenAITextResponseChoice `json:"choices"`
	Usage          dto.Usage                      `json:"usage"`
	ConversationID string                         `json:"conversation_id,omitempty"`
}

const chatImageGenerationInstruction = "System: The user is requesting image generation. Use ChatGPT image generation capability to actually create and return image(s). Do not only return JSON, parameters, or textual instructions."
const chatGPTWebToolPromptMaxBytes = 60_000
const chatGPTWebDeepResearchPendingMessage = "深度研究任务已启动，上游暂未在当前流中返回最终报告；请稍后在同一会话继续查看结果。"
const chatGPTWebDeepResearchEmbeddedUIMessage = "ChatGPT Web 已返回深度研究嵌入式界面事件，但当前 API 无法读取该界面内的最终报告；请关闭深度研究后重试，或在 ChatGPT Web 页面中查看该任务。"
const chatGPTWebDeepResearchRecoverMaxWait = 3 * time.Minute
const chatGPTWebDeepResearchRecoverInterval = 5 * time.Second
const chatGPTWebHandoffRecoverMaxWait = 45 * time.Second
const chatGPTWebHandoffRecoverInterval = time.Second
const chatGPTWebChatImagePollMaxWait = 30 * time.Second
const chatGPTWebImagePollDefaultMaxWait = 10 * time.Minute
const chatGPTWebImagePollTestMaxWait = 45 * time.Second
const chatGPTWebImageRunDefaultTimeout = 20 * time.Minute
const chatGPTWebHTTPDefaultTimeout = 10 * time.Minute
const chatGPTWebImageOperationGrace = 2 * time.Minute
const chatGPTWebImageDownloadURLDefaultMaxWait = 90 * time.Second
const chatGPTWebImageDownloadURLTestMaxWait = 5 * time.Second

const (
	chatGPTWebSessionRouteCacheNamespace = "new-api:chatgpt_web_session_route:v1"
	chatGPTWebSessionRouteTTL            = 6 * time.Hour
	chatGPTWebSessionRouteCapacity       = 100_000
)

var (
	chatGPTWebSessionRouteCacheOnce sync.Once
	chatGPTWebSessionRouteCache     *cachex.HybridCache[chatGPTWebSessionRouteState]
)

type chatGPTWebSessionRouteState struct {
	ConversationID string `json:"conversation_id"`
}

type chatGPTWebSessionRoute struct {
	Enabled              bool
	Key                  string
	SessionHash          string
	SessionSource        string
	CachedConversationID string
	Reused               bool
	Incremental          bool
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("chatgpt web channel: /v1beta/models endpoint not supported")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("chatgpt web channel: messages are required")
	}
	openAIRequest, err := service.ClaudeToOpenAIRequest(*request, info)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, openAIRequest)
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("chatgpt web channel: audio endpoint not supported")
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil || len(request.Messages) == 0 {
		return nil, errors.New("chatgpt web channel: messages are required")
	}
	model := strings.TrimSpace(request.Model)
	if model == "" && info != nil {
		model = strings.TrimSpace(info.UpstreamModelName)
	}
	if model == "" {
		model = "auto"
	}
	return chatRequest{
		Model:          model,
		Messages:       request.Messages,
		Stream:         request.Stream,
		Tools:          request.Tools,
		ToolChoice:     request.ToolChoice,
		ThinkingEffort: chatGPTWebThinkingEffort(model, chatReasoningEffort(request.ReasoningEffort, request.Reasoning)),
		DeepResearch:   extractChatGPTWebDeepResearchFromRawBody(c),
		ResponseFormat: request.ResponseFormat,
		FallbackPrompt: extractFallbackPromptFromRawBody(c),
		ConversationID: extractConversationIDFromRawBody(c),
	}, nil
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("chatgpt web channel: /v1/rerank endpoint not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("chatgpt web channel: /v1/embeddings endpoint not supported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	messages := messagesFromResponsesRequest(request)
	if len(messages) == 0 {
		return nil, errors.New("chatgpt web channel: responses input is required")
	}
	model := strings.TrimSpace(request.Model)
	if model == "" && info != nil {
		model = strings.TrimSpace(info.UpstreamModelName)
	}
	if model == "" {
		model = "auto"
	}
	return chatRequest{
		Model:          model,
		Messages:       messages,
		Stream:         request.Stream,
		Tools:          chatGPTWebToolsFromResponsesRaw(request.Tools),
		ToolChoice:     chatGPTWebToolChoiceFromResponsesRaw(request.ToolChoice),
		ThinkingEffort: chatGPTWebThinkingEffort(model, responsesReasoningEffort(request.Reasoning)),
		DeepResearch:   extractChatGPTWebDeepResearchFromRawBody(c),
		ResponseFormat: responseFormatFromResponsesText(request.Text),
		FallbackPrompt: extractFallbackPromptFromRawBody(c),
		ConversationID: extractConversationIDFromResponsesRequest(c, request),
	}, nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	converted := generationRequest{
		Model:          strings.TrimSpace(request.Model),
		Prompt:         strings.TrimSpace(request.Prompt),
		Size:           strings.TrimSpace(request.Size),
		Quality:        strings.TrimSpace(request.Quality),
		ResponseFormat: strings.TrimSpace(request.ResponseFormat),
	}
	if request.N != nil {
		converted.N = int(*request.N)
	}
	if converted.N <= 0 {
		converted.N = 1
	}
	normalizeGenerationRequestModelAndResponseFormat(&converted, info)
	converted.ConversationID = extractConversationIDFromImageRequest(request)
	converted.FallbackPrompt = extractStringExtraField(request, "fallback_prompt")
	converted.FallbackReferenceImages = extractStringSliceExtraField(request, "fallback_reference_images")

	refs, err := extractReferenceImagesFromRequest(c, info, request)
	if err != nil {
		return nil, err
	}
	converted.ReferenceImages = refs
	return converted, nil
}

func normalizeGenerationRequestModelAndResponseFormat(req *generationRequest, info *relaycommon.RelayInfo) {
	if req == nil {
		return
	}
	upstreamModel := ""
	if info != nil && info.ChannelMeta != nil {
		upstreamModel = strings.TrimSpace(info.ChannelMeta.UpstreamModelName)
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = upstreamModel
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = ModelList[0]
	}
	if shouldForceB64JSONImageResponse(req.Model) || shouldForceB64JSONImageResponse(upstreamModel) {
		req.ResponseFormat = "b64_json"
	} else if strings.TrimSpace(req.ResponseFormat) == "" {
		req.ResponseFormat = "b64_json"
	}
}

func chatGPTWebImageConversationModel(req generationRequest) string {
	model := strings.TrimSpace(req.Model)
	if model == "" || strings.EqualFold(model, "auto") {
		return ModelList[0]
	}
	return model
}

func shouldForceB64JSONImageResponse(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gpt-image-2", "chatgpt-image-2":
		return true
	default:
		return false
	}
}

func extractConversationIDFromImageRequest(request dto.ImageRequest) string {
	return extractStringExtraField(request, "conversation_id")
}

func extractStringExtraField(request dto.ImageRequest, field string) string {
	if raw, ok := request.Extra[field]; ok && len(raw) > 0 {
		var value string
		if err := common.Unmarshal(raw, &value); err == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractStringSliceExtraField(request dto.ImageRequest, field string) []string {
	if raw, ok := request.Extra[field]; ok && len(raw) > 0 {
		var values []string
		if err := common.Unmarshal(raw, &values); err == nil {
			out := make([]string, 0, len(values))
			for _, value := range values {
				if value = strings.TrimSpace(value); value != "" {
					out = append(out, value)
				}
			}
			return out
		}
		var single string
		if err := common.Unmarshal(raw, &single); err == nil && strings.TrimSpace(single) != "" {
			return []string{strings.TrimSpace(single)}
		}
	}
	return nil
}

func extractConversationIDFromRawBody(c *gin.Context) string {
	return extractStringFromRawBody(c, "conversation_id")
}

func extractFallbackPromptFromRawBody(c *gin.Context) string {
	return extractStringFromRawBody(c, "fallback_prompt")
}

func extractChatGPTWebDeepResearchFromRawBody(c *gin.Context) bool {
	body := rawRequestBodyBytes(c)
	if len(body) == 0 {
		return false
	}
	var probe map[string]any
	if err := common.Unmarshal(body, &probe); err != nil {
		return false
	}
	return chatGPTWebDeepResearchEnabledInMap(probe)
}

func chatGPTWebDeepResearchEnabledInMap(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	for _, field := range []string{"chatgpt_web_deep_research", "deep_research"} {
		if boolLikeTrue(payload[field]) {
			return true
		}
	}
	for _, field := range []string{"chatgpt_web_deep_research_version", "deep_research_version"} {
		if stringLikePresent(payload[field]) {
			return true
		}
	}
	if chatGPTWebHintsIncludeDeepResearch(payload["system_hints"]) {
		return true
	}
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		if chatGPTWebDeepResearchEnabledInMap(metadata) {
			return true
		}
		if chatGPTWebHintsIncludeDeepResearch(metadata["system_hints"]) {
			return true
		}
	}
	return false
}

func boolLikeTrue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "on", "enabled":
			return true
		}
	}
	return false
}

func stringLikePresent(value any) bool {
	v, ok := value.(string)
	return ok && strings.TrimSpace(v) != ""
}

func chatGPTWebHintsIncludeDeepResearch(value any) bool {
	switch hints := value.(type) {
	case []any:
		for _, hint := range hints {
			if strings.TrimSpace(fmt.Sprint(hint)) == chatGPTWebDeepResearchConnector {
				return true
			}
		}
	case []string:
		for _, hint := range hints {
			if strings.TrimSpace(hint) == chatGPTWebDeepResearchConnector {
				return true
			}
		}
	case string:
		return strings.TrimSpace(hints) == chatGPTWebDeepResearchConnector
	}
	return false
}

func extractStringFromRawBody(c *gin.Context, field string) string {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return ""
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return ""
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return ""
	}
	var probe map[string]any
	if err := common.Unmarshal(body, &probe); err != nil {
		return ""
	}
	value, ok := probe[field].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func rawRequestBodyBytes(c *gin.Context) []byte {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return nil
	}
	return body
}

func chatGPTWebToolsFromResponsesRaw(raw json.RawMessage) []dto.ToolCallRequest {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var chatTools []dto.ToolCallRequest
	if err := common.Unmarshal(raw, &chatTools); err == nil {
		hasNamedFunction := false
		for _, tool := range chatTools {
			if strings.TrimSpace(tool.Function.Name) != "" {
				hasNamedFunction = true
				break
			}
		}
		if hasNamedFunction {
			return chatTools
		}
	}
	var tools []map[string]json.RawMessage
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		toolType := stringFromRawJSON(tool["type"])
		if toolType == "" {
			toolType = "function"
		}
		switch toolType {
		case "function":
			name := stringFromRawJSON(tool["name"])
			description := stringFromRawJSON(tool["description"])
			parametersRaw := tool["parameters"]
			if name == "" && len(tool["function"]) > 0 {
				var fn map[string]json.RawMessage
				if err := common.Unmarshal(tool["function"], &fn); err == nil {
					name = stringFromRawJSON(fn["name"])
					if description == "" {
						description = stringFromRawJSON(fn["description"])
					}
					if len(parametersRaw) == 0 {
						parametersRaw = fn["parameters"]
					}
				}
			}
			if name == "" {
				continue
			}
			var parameters any
			if len(parametersRaw) > 0 {
				_ = common.Unmarshal(parametersRaw, &parameters)
			}
			out = append(out, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        name,
					Description: description,
					Parameters:  parameters,
				},
			})
		default:
			var preserved dto.ToolCallRequest
			if b, err := common.Marshal(tool); err == nil && common.Unmarshal(b, &preserved) == nil && strings.TrimSpace(preserved.Type) != "" {
				out = append(out, preserved)
			}
		}
	}
	return out
}

func chatGPTWebToolChoiceFromResponsesRaw(raw json.RawMessage) any {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func buildChatGPTWebPreemptiveLocalToolResponse(req chatRequest, info *relaycommon.RelayInfo, timings ...*service.ChatGPTWebTiming) (*http.Response, bool, error) {
	timing := firstChatGPTWebTiming(timings...)
	if info == nil || len(req.Tools) == 0 {
		return nil, false, nil
	}
	if info.RelayFormat != types.RelayFormatClaude && !isResponsesRelay(info) {
		return nil, false, nil
	}
	if chatGPTWebHasToolResultAfterLatestUser(req) {
		if timing != nil {
			timing.Set("tool_bridge_preemptive", false)
			timing.Set("tool_bridge_preemptive_reason", "tool_result_present")
		}
		return nil, false, nil
	}
	toolCall, ok := buildChatGPTWebLocalToolCall(req)
	if !ok {
		if timing != nil {
			timing.Set("tool_bridge_preemptive", false)
		}
		return nil, false, nil
	}
	if timing != nil {
		timing.Set("tool_bridge_preemptive", true)
		timing.Set("tool_bridge_call_detected", true)
		timing.Set("tool_bridge_call_name", toolCall.Function.Name)
	}
	usage := buildChatUsage(latestChatGPTWebUserText(req), toolCall.Function.Name+toolCall.Function.Arguments, req.Model)
	payload, err := buildChatGPTWebSyntheticToolHTTPResponse(req, info, toolCall, usage)
	if err != nil {
		return nil, true, err
	}
	return payload, true, nil
}

func buildChatGPTWebSyntheticToolHTTPResponse(req chatRequest, info *relaycommon.RelayInfo, toolCall dto.ToolCallResponse, usage dto.Usage) (*http.Response, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "auto"
	}
	created := time.Now().Unix()
	if req.Stream != nil && *req.Stream {
		var b bytes.Buffer
		if isResponsesRelay(info) {
			responseID := buildTransientResponsesResponseID()
			writeResponsesToolCall(&b, responseID, created, model, toolCall, usage)
		} else {
			id := buildTransientChatCompletionID()
			writeChatStreamChunk(&b, id, created, model, "assistant", "", nil, nil)
			writeChatToolCallChunk(&b, id, created, model, toolCall)
			finish := "tool_calls"
			writeChatStreamChunk(&b, id, created, model, "", "", &finish, &usage)
		}
		writeChatDone(&b)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewReader(b.Bytes())),
		}, nil
	}
	if isResponsesRelay(info) {
		responseID := buildTransientResponsesResponseID()
		itemID := buildResponsesFunctionCallItemID(responseID, 0)
		respPayload := buildResponsesToolCallResponse(responseID, itemID, created, model, toolCall, usage)
		payloadBytes, err := common.Marshal(respPayload)
		if err != nil {
			return nil, fmt.Errorf("chatgpt web channel: marshal preemptive responses tool call failed: %w", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
		}, nil
	}
	respPayload := buildChatToolCallResponse(buildTransientChatCompletionID(), created, model, toolCall, usage)
	payloadBytes, err := common.Marshal(respPayload)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web channel: marshal preemptive chat tool call failed: %w", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
	}, nil
}

func buildChatGPTWebLocalToolCall(req chatRequest) (dto.ToolCallResponse, bool) {
	text := latestChatGPTWebUserText(req)
	intent, target, ok := classifyChatGPTWebLocalToolIntent(text)
	if !ok {
		return dto.ToolCallResponse{}, false
	}
	tools := chatGPTWebToolNameSet(req.Tools)
	if canonical, ok := tools["exec_command"]; ok {
		args := buildExecCommandArguments(intent, target)
		return newChatGPTWebToolCall(canonical, args), true
	}
	if intent == "read" {
		if canonical, ok := tools["read"]; ok {
			return newChatGPTWebToolCall(canonical, map[string]any{"file_path": target}), true
		}
	}
	if canonical, ok := tools["ls"]; ok {
		path := target
		if intent == "read" && looksLikeFilePath(target) {
			path = filepath.Dir(target)
			if path == "." && !strings.HasPrefix(target, ".") {
				path = "."
			}
		}
		return newChatGPTWebToolCall(canonical, map[string]any{"path": path}), true
	}
	return dto.ToolCallResponse{}, false
}

func latestChatGPTWebUserText(req chatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "" && role != "user" {
			continue
		}
		if text := strings.TrimSpace(messageTextContent(msg)); text != "" {
			return text
		}
	}
	return ""
}

func chatGPTWebToolNameSet(tools []dto.ToolCallRequest) map[string]string {
	out := make(map[string]string, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			continue
		}
		out[strings.ToLower(name)] = name
	}
	return out
}

func classifyChatGPTWebLocalToolIntent(text string) (intent string, target string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", "", false
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "为什么") || strings.Contains(lower, "why ") {
		return "", "", false
	}
	target = extractChatGPTWebLocalPath(trimmed)
	if target == "" {
		return "", "", false
	}
	readIntent := containsAnyFold(lower, []string{"读取", "读一下", "读 ", "查看", "打开", "看一下", "cat ", "read ", "show "})
	listIntent := containsAnyFold(lower, []string{"列出", "目录", "文件列表", "ls ", "list ", "tree "})
	if !readIntent && !listIntent {
		return "", "", false
	}
	if readIntent || looksLikeFilePath(target) {
		return "read", target, true
	}
	return "list", target, true
}

func extractChatGPTWebLocalPath(text string) string {
	for _, quote := range []string{"`", "\"", "'"} {
		if path := extractQuotedChatGPTWebPath(text, quote); path != "" {
			return path
		}
	}
	normalized := strings.NewReplacer(
		"\n", " ",
		"\t", " ",
		"，", " ",
		"。", " ",
		"；", " ",
		";", " ",
		",", " ",
		"：", " ",
		":", " ",
		"）", " ",
		")", " ",
		"（", " ",
		"(", " ",
	).Replace(text)
	for _, field := range strings.Fields(normalized) {
		candidate := strings.Trim(field, " \t\r\n`\"'[]{}<>")
		candidate = strings.TrimRight(candidate, "。.,，;；:：!?！？")
		if isChatGPTWebLocalPathCandidate(candidate) {
			return candidate
		}
	}
	if containsAnyFold(text, []string{"当前目录", "current directory", "working directory"}) {
		return "."
	}
	return ""
}

func extractQuotedChatGPTWebPath(text string, quote string) string {
	start := strings.Index(text, quote)
	for start >= 0 {
		rest := text[start+len(quote):]
		end := strings.Index(rest, quote)
		if end < 0 {
			return ""
		}
		candidate := strings.TrimSpace(rest[:end])
		if isChatGPTWebLocalPathCandidate(candidate) {
			return candidate
		}
		next := strings.Index(rest[end+len(quote):], quote)
		if next < 0 {
			return ""
		}
		start += len(quote) + end + len(quote) + next
	}
	return ""
}

func isChatGPTWebLocalPathCandidate(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || value == "." {
		return true
	}
	return lower == "agents.md" || strings.HasSuffix(lower, "/agents.md") || strings.Contains(lower, ".")
}

func looksLikeFilePath(path string) bool {
	base := strings.TrimSpace(filepath.Base(path))
	if base == "." || base == "/" || base == "" {
		return false
	}
	return strings.Contains(base, ".")
}

func buildExecCommandArguments(intent string, target string) map[string]any {
	target = strings.TrimSpace(target)
	if target == "" {
		target = "."
	}
	commandTarget := target
	workdir := ""
	if intent == "read" && filepath.IsAbs(target) && looksLikeFilePath(target) {
		workdir = filepath.Dir(target)
		commandTarget = filepath.Base(target)
	}
	cmd := "ls -la " + shellQuote(commandTarget)
	if intent == "read" {
		cmd = "cat " + shellQuote(commandTarget)
	}
	args := map[string]any{
		"cmd":               cmd,
		"max_output_tokens": 20000,
	}
	if workdir != "" && workdir != "." {
		args["workdir"] = workdir
	}
	return args
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func newChatGPTWebToolCall(name string, args map[string]any) dto.ToolCallResponse {
	data, err := common.Marshal(args)
	if err != nil || len(data) == 0 {
		data = []byte(`{}`)
	}
	return dto.ToolCallResponse{
		ID:   "call_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Type: "function",
		Function: dto.FunctionResponse{
			Name:      name,
			Arguments: string(data),
		},
	}
}

func containsAnyFold(text string, needles []string) bool {
	lower := strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(lower, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func getChatGPTWebSessionRouteCache() *cachex.HybridCache[chatGPTWebSessionRouteState] {
	chatGPTWebSessionRouteCacheOnce.Do(func() {
		chatGPTWebSessionRouteCache = cachex.NewHybridCache[chatGPTWebSessionRouteState](cachex.HybridCacheConfig[chatGPTWebSessionRouteState]{
			Namespace:  cachex.Namespace(chatGPTWebSessionRouteCacheNamespace),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[chatGPTWebSessionRouteState]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, chatGPTWebSessionRouteState] {
				return hot.NewHotCache[string, chatGPTWebSessionRouteState](hot.LRU, chatGPTWebSessionRouteCapacity).
					WithTTL(chatGPTWebSessionRouteTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return chatGPTWebSessionRouteCache
}

func resetChatGPTWebSessionRouteCacheForTest() {
	chatGPTWebSessionRouteCacheOnce = sync.Once{}
	chatGPTWebSessionRouteCache = nil
}

func chatGPTWebSessionRouteKey(info *relaycommon.RelayInfo, sessionKey string) string {
	sessionKey = strings.TrimSpace(sessionKey)
	if info == nil || info.ChannelMeta == nil || sessionKey == "" {
		return ""
	}
	raw := strings.Join([]string{
		"user", strconv.Itoa(info.UserId),
		"token", strconv.Itoa(info.TokenId),
		"channel", strconv.Itoa(info.ChannelId),
		"multi_key", strconv.Itoa(info.ChannelMultiKeyIndex),
		"account", hashCachePart(info.ApiKey),
		"session", sessionKey,
	}, "\x00")
	return hashCachePart(raw)
}

func chatGPTWebShortHash(value string) string {
	sum := hashCachePart(value)
	if len(sum) > 12 {
		return sum[:12]
	}
	return sum
}

func resolveChatGPTWebSessionRoute(info *relaycommon.RelayInfo, req chatRequest, rawBody []byte, timings ...*service.ChatGPTWebTiming) chatGPTWebSessionRoute {
	timing := firstChatGPTWebTiming(timings...)
	if info == nil || info.ChannelMeta == nil || info.IsChannelTest {
		if timing != nil {
			timing.Set("session_route_enabled", false)
		}
		return chatGPTWebSessionRoute{}
	}
	headers := info.RequestHeaders
	sessionKey := service.ExtractOpenAICompatPromptCacheKeyFromRawBody(rawBody, headers)
	sessionSource := "explicit"
	if sessionKey == "" {
		sessionKey = deriveChatGPTWebSessionKeyFromMessages(req)
		sessionSource = "message_seed"
	}
	if sessionKey == "" {
		if timing != nil {
			timing.Set("session_route_enabled", false)
			timing.Set("session_route_reason", "missing_session_key")
		}
		return chatGPTWebSessionRoute{}
	}
	routeKey := chatGPTWebSessionRouteKey(info, sessionKey)
	if routeKey == "" {
		if timing != nil {
			timing.Set("session_route_enabled", false)
			timing.Set("session_route_reason", "missing_route_key")
		}
		return chatGPTWebSessionRoute{}
	}
	route := chatGPTWebSessionRoute{
		Enabled:       true,
		Key:           routeKey,
		SessionHash:   chatGPTWebShortHash(sessionKey),
		SessionSource: sessionSource,
	}
	if timing != nil {
		timing.Set("session_route_enabled", true)
		timing.Set("session_route_session_hash", route.SessionHash)
		timing.Set("session_route_source", sessionSource)
		timing.Set("session_route_hit", false)
	}
	if strings.TrimSpace(req.ConversationID) != "" {
		if timing != nil {
			timing.Set("session_route_explicit_conversation", true)
		}
		return route
	}
	state, found, err := getChatGPTWebSessionRouteCache().Get(routeKey)
	if err != nil {
		if timing != nil {
			timing.Set("session_route_cache_error", common.MaskSensitiveInfo(err.Error()))
		}
		return route
	}
	conversationID := normalizeChatGPTWebConversationID(state.ConversationID)
	if found && conversationID != "" {
		route.Reused = true
		route.CachedConversationID = conversationID
		if timing != nil {
			timing.Set("session_route_hit", true)
			timing.Set("session_route_conversation_hash", chatGPTWebShortHash(conversationID))
		}
	}
	return route
}

func deriveChatGPTWebSessionKeyFromMessages(req chatRequest) string {
	firstUser := ""
	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "" && role != "user" {
			continue
		}
		if text := strings.TrimSpace(messageTextContent(msg)); text != "" {
			firstUser = normalizeChatGPTWebSessionSeedText(text)
			break
		}
	}
	if firstUser == "" {
		return ""
	}
	return "message_seed:" + hashCachePart(firstUser)
}

func normalizeChatGPTWebSessionSeedText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func applyChatGPTWebSessionRoute(req *chatRequest, route *chatGPTWebSessionRoute, fullPrompt string, info *relaycommon.RelayInfo, timings ...*service.ChatGPTWebTiming) string {
	timing := firstChatGPTWebTiming(timings...)
	fullPrompt = strings.TrimSpace(fullPrompt)
	if req == nil || route == nil || !route.Reused || strings.TrimSpace(route.CachedConversationID) == "" {
		if timing != nil {
			timing.Set("session_route_incremental", false)
		}
		return fullPrompt
	}
	latestPrompt := buildLatestChatGPTWebMessagePromptForRelay(*req, info)
	if strings.TrimSpace(latestPrompt) == "" {
		if timing != nil {
			timing.Set("session_route_incremental", false)
			timing.Set("session_route_reason", "missing_latest_message")
		}
		return fullPrompt
	}
	req.ConversationID = route.CachedConversationID
	if strings.TrimSpace(req.FallbackPrompt) == "" {
		req.FallbackPrompt = fullPrompt
	}
	route.Incremental = true
	if timing != nil {
		timing.Set("session_route_incremental", true)
		timing.Set("session_route_fallback_ready", strings.TrimSpace(req.FallbackPrompt) != "")
		timing.Set("session_route_incremental_tool_result", chatGPTWebHasToolResultAfterLatestUser(*req))
	}
	return strings.TrimSpace(latestPrompt)
}

func recordChatGPTWebSessionRoute(route chatGPTWebSessionRoute, conversationID string, timings ...*service.ChatGPTWebTiming) {
	timing := firstChatGPTWebTiming(timings...)
	conversationID = normalizeChatGPTWebConversationID(conversationID)
	if !route.Enabled || strings.TrimSpace(route.Key) == "" || conversationID == "" {
		return
	}
	err := getChatGPTWebSessionRouteCache().SetWithTTL(route.Key, chatGPTWebSessionRouteState{
		ConversationID: conversationID,
	}, chatGPTWebSessionRouteTTL)
	if timing != nil {
		if err != nil {
			timing.Set("session_route_record_error", common.MaskSensitiveInfo(err.Error()))
			return
		}
		timing.Set("session_route_recorded", true)
		timing.Set("session_route_recorded_conversation_hash", chatGPTWebShortHash(conversationID))
	}
}

func messagesFromResponsesRequest(request dto.OpenAIResponsesRequest) []dto.Message {
	messages := make([]dto.Message, 0)
	if instruction := stringFromRawJSON(request.Instructions); instruction != "" {
		messages = append(messages, dto.Message{Role: "system", Content: instruction})
	}
	messages = append(messages, messagesFromResponsesInput(request.Input)...)
	return messages
}

func messagesFromResponsesInput(raw json.RawMessage) []dto.Message {
	if len(raw) == 0 {
		return nil
	}
	switch common.GetJsonType(raw) {
	case "string":
		if text := stringFromRawJSON(raw); text != "" {
			return []dto.Message{{Role: "user", Content: text}}
		}
	case "object":
		if msg, ok := messageFromResponsesInputObject(raw); ok {
			return []dto.Message{msg}
		}
	case "array":
		var items []json.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil
		}
		messages := make([]dto.Message, 0, len(items))
		for _, item := range items {
			if msg, ok := messageFromResponsesInputObject(item); ok {
				messages = append(messages, msg)
			}
		}
		return messages
	}
	return nil
}

func messageFromResponsesInputObject(raw json.RawMessage) (dto.Message, bool) {
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(raw, &obj); err != nil {
		return dto.Message{}, false
	}
	itemType := stringFromRawJSON(obj["type"])
	role := stringFromRawJSON(obj["role"])
	if role == "" {
		role = "user"
	}

	switch itemType {
	case "input_text":
		text := stringFromRawJSON(obj["text"])
		if text == "" {
			return dto.Message{}, false
		}
		return dto.Message{Role: role, Content: text}, true
	case "input_image":
		imageURL := imageURLFromResponsesInput(obj["image_url"])
		if imageURL == "" {
			return dto.Message{}, false
		}
		return dto.Message{Role: role, Content: []any{map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": imageURL,
			},
		}}}, true
	case "input_file":
		fileURL := imageURLFromResponsesInput(obj["file_url"])
		if fileURL == "" {
			return dto.Message{}, false
		}
		return dto.Message{Role: role, Content: fmt.Sprintf("[file_url: %s]", fileURL)}, true
	case "function_call_output":
		output := stringFromRawJSON(obj["output"])
		if output == "" {
			output = string(obj["output"])
		}
		return dto.Message{Role: "tool", Content: output, ToolCallId: stringFromRawJSON(obj["call_id"])}, output != ""
	case "function_call":
		name := stringFromRawJSON(obj["name"])
		args := stringFromRawJSON(obj["arguments"])
		if args == "" && len(obj["arguments"]) > 0 {
			args = string(obj["arguments"])
		}
		text := strings.TrimSpace(fmt.Sprintf("[function_call] %s %s", name, args))
		return dto.Message{Role: "assistant", Content: text}, text != "[function_call]"
	}

	if contentRaw, ok := obj["content"]; ok && len(contentRaw) > 0 {
		content := responsesContentToMessageContent(contentRaw, role)
		if content == nil {
			return dto.Message{}, false
		}
		return dto.Message{Role: role, Content: content}, true
	}
	return dto.Message{}, false
}

func responsesContentToMessageContent(raw json.RawMessage, role string) any {
	switch common.GetJsonType(raw) {
	case "string":
		return stringFromRawJSON(raw)
	case "array":
		var parts []json.RawMessage
		if err := common.Unmarshal(raw, &parts); err != nil {
			return nil
		}
		converted := make([]any, 0, len(parts))
		for _, partRaw := range parts {
			var part map[string]json.RawMessage
			if err := common.Unmarshal(partRaw, &part); err != nil {
				continue
			}
			partType := stringFromRawJSON(part["type"])
			switch partType {
			case "input_text", "output_text", "text":
				if text := stringFromRawJSON(part["text"]); text != "" {
					converted = append(converted, map[string]any{"type": "text", "text": text})
				}
			case "input_image", "image_url":
				if imageURL := imageURLFromResponsesInput(part["image_url"]); imageURL != "" {
					converted = append(converted, map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": imageURL,
						},
					})
				}
			case "input_file":
				if fileURL := imageURLFromResponsesInput(part["file_url"]); fileURL != "" {
					converted = append(converted, map[string]any{"type": "text", "text": fmt.Sprintf("[file_url: %s]", fileURL)})
				}
			}
		}
		if len(converted) == 0 {
			return nil
		}
		if role == "assistant" {
			var b strings.Builder
			for _, part := range converted {
				partMap, _ := part.(map[string]any)
				if text, _ := partMap["text"].(string); text != "" {
					if b.Len() > 0 {
						b.WriteByte('\n')
					}
					b.WriteString(text)
				}
			}
			if b.Len() > 0 {
				return b.String()
			}
		}
		return converted
	default:
		if len(raw) > 0 {
			return string(raw)
		}
	}
	return nil
}

func imageURLFromResponsesInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if common.GetJsonType(raw) == "string" {
		return stringFromRawJSON(raw)
	}
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return stringFromRawJSON(obj["url"])
}

func stringFromRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	if common.GetJsonType(raw) == "object" || common.GetJsonType(raw) == "array" {
		return strings.TrimSpace(string(raw))
	}
	return ""
}

func responseFormatFromResponsesText(raw json.RawMessage) *dto.ResponseFormat {
	if len(raw) == 0 || common.GetJsonType(raw) != "object" {
		return nil
	}
	var textObj map[string]json.RawMessage
	if err := common.Unmarshal(raw, &textObj); err != nil {
		return nil
	}
	formatRaw := textObj["format"]
	if len(formatRaw) == 0 || common.GetJsonType(formatRaw) != "object" {
		return nil
	}
	var formatObj map[string]json.RawMessage
	if err := common.Unmarshal(formatRaw, &formatObj); err != nil {
		return nil
	}
	formatType := stringFromRawJSON(formatObj["type"])
	switch formatType {
	case "json_object", "json_schema":
		return &dto.ResponseFormat{Type: formatType, JsonSchema: formatRaw}
	default:
		return nil
	}
}

func extractConversationIDFromResponsesRequest(c *gin.Context, request dto.OpenAIResponsesRequest) string {
	if conversationID := extractConversationIDFromRawBody(c); conversationID != "" {
		return normalizeChatGPTWebConversationID(conversationID)
	}
	if conversationID := conversationIDFromResponsesConversation(request.Conversation); conversationID != "" {
		return conversationID
	}
	if conversationID := conversationIDFromResponsesMetadata(request.Metadata); conversationID != "" {
		return conversationID
	}
	return normalizeChatGPTWebConversationID(request.PreviousResponseID)
}

func conversationIDFromResponsesConversation(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if common.GetJsonType(raw) == "string" {
		value := stringFromRawJSON(raw)
		if strings.EqualFold(value, "auto") {
			return ""
		}
		return normalizeChatGPTWebConversationID(value)
	}
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"id", "conversation_id", "conversationId"} {
		if value := stringFromRawJSON(obj[key]); value != "" {
			return normalizeChatGPTWebConversationID(value)
		}
	}
	return ""
}

func conversationIDFromResponsesMetadata(raw json.RawMessage) string {
	if len(raw) == 0 || common.GetJsonType(raw) != "object" {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"conversation_id", "conversationId", "previous_response_id"} {
		if value := stringFromRawJSON(obj[key]); value != "" {
			return normalizeChatGPTWebConversationID(value)
		}
	}
	return ""
}

func normalizeChatGPTWebConversationID(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{
		"resp_chatgptimg-",
		"resp_chatgptimg_",
		"chatcmpl-chatgptimg-",
	} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	return value
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := defaultBaseURL
	if info != nil && strings.TrimSpace(info.ChannelBaseUrl) != "" {
		baseURL = strings.TrimSpace(info.ChannelBaseUrl)
	}
	return baseURL + "/backend-api/f/conversation", nil
}

func (a *Adaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web channel: read request body failed: %w", err)
	}
	var probe struct {
		Messages []dto.Message `json:"messages"`
	}
	if err := common.Unmarshal(body, &probe); err == nil && len(probe.Messages) > 0 {
		return a.doChatRequest(c, info, body)
	}
	return a.doImageRequest(c, info, body)
}

func (a *Adaptor) doImageRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) (any, error) {
	timing := service.NewChatGPTWebTiming()
	service.SetChatGPTWebTiming(c, timing)
	timing.Set("request_kind", "image")
	var req generationRequest
	if err := common.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("chatgpt web channel: invalid image request json: %w", err)
	}
	normalizeGenerationRequestModelAndResponseFormat(&req, info)
	timing.Set("model", strings.TrimSpace(req.Model))
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("chatgpt web channel: prompt is required")
	}

	testMode := info != nil && info.IsChannelTest
	requestCtx := context.Background()
	if c != nil && c.Request != nil {
		requestCtx = c.Request.Context()
	}
	imageCtx, cancelImageCtx := chatGPTWebImageOperationContext(requestCtx, testMode, timing)
	defer cancelImageCtx()

	client, err := newClientFromRelayInfo(imageCtx, info, timing)
	if err != nil {
		return nil, err
	}
	uploadStart := time.Now()
	refs, err := uploadReferenceImages(imageCtx, client, req.ReferenceImages)
	timing.ObserveSince("reference_upload_ms", uploadStart)
	timing.Set("reference_count", len(refs))
	if err != nil {
		return nil, err
	}
	res, err := runImageGeneration(imageCtx, client, req, refs, testMode, playgroundDebugCapture(c), timing)
	if err != nil {
		return nil, err
	}
	if info != nil {
		actualCount := len(res.SignedURLs)
		if actualCount == 0 {
			actualCount = len(res.FileRefs)
		}
		if actualCount <= 0 {
			actualCount = req.N
		}
		if actualCount > 0 {
			info.PriceData.AddOtherRatio("n", float64(actualCount))
		}
	}

	respPayload, err := buildGenerationResponse(imageCtx, client, req, res, testMode, info, requestPublicBaseURLForImages(c, info), timing)
	if err != nil {
		return nil, err
	}
	recordGenerationDrawingLog(info, req, res, respPayload)
	payloadBytes, err := common.Marshal(respPayload)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web channel: marshal synthetic response failed: %w", err)
	}
	if info != nil && info.IsStream {
		payloadBytes = buildImageStreamPayload(payloadBytes)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
	}, nil
}

func (a *Adaptor) doChatRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) (any, error) {
	timing := service.NewChatGPTWebTiming()
	service.SetChatGPTWebTiming(c, timing)
	timing.Set("request_kind", "chat")
	var req chatRequest
	if err := common.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("chatgpt web channel: invalid chat request json: %w", err)
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("chatgpt web channel: messages are required")
	}
	if strings.TrimSpace(req.Model) == "" && info != nil {
		req.Model = strings.TrimSpace(info.UpstreamModelName)
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "auto"
	}
	timing.Set("model", strings.TrimSpace(req.Model))
	timing.Set("tool_bridge_enabled", len(req.Tools) > 0)
	timing.Set("tool_bridge_tool_count", len(req.Tools))
	timing.Set("tool_bridge_tool_result_present", chatGPTWebHasToolResultAfterLatestUser(req))
	if resp, ok, err := buildChatGPTWebPreemptiveLocalToolResponse(req, info, timing); ok || err != nil {
		return resp, err
	}
	fullPrompt := buildChatPromptForRelay(req, info)
	rawBody := rawRequestBodyBytes(c)
	if len(rawBody) == 0 {
		rawBody = body
	}
	route := resolveChatGPTWebSessionRoute(info, req, rawBody, timing)
	prompt := applyChatGPTWebSessionRoute(&req, &route, fullPrompt, info, timing)
	timing.Set("image_intent", shouldPollChatGeneratedImagesForRelay(info, req, prompt, "", false))
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("chatgpt web channel: chat prompt is empty")
	}
	client, err := newClientFromRelayInfo(c.Request.Context(), info, timing)
	if err != nil {
		return nil, err
	}
	if info != nil && info.IsChannelTest {
		content, conversationID, usedPrompt, err := runChatCompletionProbe(c.Request.Context(), client, req, prompt, playgroundDebugCapture(c), timing)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(content) == "" {
			content = "ok"
		}
		usage := buildChatUsage(usedPrompt, content, req.Model)
		if isResponsesRelay(info) {
			respPayload := buildResponsesResponse(
				buildResponsesResponseID(conversationID),
				buildResponsesMessageID(conversationID),
				time.Now().Unix(),
				strings.TrimSpace(req.Model),
				content,
				usage,
			)
			payloadBytes, err := common.Marshal(respPayload)
			if err != nil {
				return nil, fmt.Errorf("chatgpt web channel: marshal responses test response failed: %w", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
			}, nil
		}
		respPayload := chatResponse{
			Id:      buildChatCompletionID(conversationID),
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   strings.TrimSpace(req.Model),
			Choices: []dto.OpenAITextResponseChoice{{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			}},
			Usage:          usage,
			ConversationID: conversationID,
		}
		payloadBytes, err := common.Marshal(respPayload)
		if err != nil {
			return nil, fmt.Errorf("chatgpt web channel: marshal chat test response failed: %w", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
		}, nil
	}
	if req.Stream != nil && *req.Stream {
		started, err := startChatStream(c.Request.Context(), client, req, prompt, playgroundDebugCapture(c), timing)
		if err != nil {
			return nil, err
		}
		return buildStreamingChatResponse(c.Request.Context(), client, started.Stream, req, started.Prompt, started.Baseline, info, requestPublicBaseURLForImages(c, info), route, timing), nil
	}
	content, conversationID, usedPrompt, baseline, hasImageGeneration, err := runChatCompletion(c.Request.Context(), client, req, prompt, playgroundDebugCapture(c), timing)
	if err != nil {
		return nil, err
	}
	recordChatGPTWebSessionRoute(route, conversationID, timing)
	materializeStart := time.Now()
	content = materializePlaygroundInlineDataImages(content, info, usedPrompt, req.Model)
	content = materializeChatGPTContentImageURLs(c.Request.Context(), client, content, info, usedPrompt, req.Model, requestPublicBaseURLForImages(c, info), timing)
	timing.ObserveSince("chat_materialize_ms", materializeStart)
	if toolCall, ok := parseChatGPTWebToolCall(content, req.Tools); ok {
		timing.Set("tool_bridge_call_detected", true)
		timing.Set("tool_bridge_call_name", toolCall.Function.Name)
		usage := buildChatUsage(usedPrompt, toolCall.Function.Name+toolCall.Function.Arguments, req.Model)
		if isResponsesRelay(info) {
			respPayload := buildResponsesToolCallResponse(
				buildResponsesResponseID(conversationID),
				buildResponsesFunctionCallItemID(conversationID, 0),
				time.Now().Unix(),
				strings.TrimSpace(req.Model),
				toolCall,
				usage,
			)
			payloadBytes, err := common.Marshal(respPayload)
			if err != nil {
				return nil, fmt.Errorf("chatgpt web channel: marshal responses tool call response failed: %w", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
			}, nil
		}
		respPayload := buildChatToolCallResponse(buildChatCompletionID(conversationID), time.Now().Unix(), strings.TrimSpace(req.Model), toolCall, usage)
		payloadBytes, err := common.Marshal(respPayload)
		if err != nil {
			return nil, fmt.Errorf("chatgpt web channel: marshal chat tool call response failed: %w", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
		}, nil
	}
	timing.Set("tool_bridge_call_detected", false)
	textContent := content
	hasInlineDataImage := chatContentHasInlineDataImage(content)
	allowImagePoll := !hasInlineDataImage && shouldPollChatGeneratedImagesForRelay(info, req, usedPrompt, textContent, hasImageGeneration)
	timing.Set("allow_image_poll", allowImagePoll)
	if imageMarkdown, err := collectChatGeneratedImageMarkdown(c.Request.Context(), client, conversationID, baseline, allowImagePoll, info, usedPrompt, req.Model, requestPublicBaseURLForImages(c, info), timing); err != nil {
		return nil, err
	} else if imageMarkdown != "" {
		content = appendMarkdownBlock(content, imageMarkdown)
	}
	usage := buildChatUsage(usedPrompt, textContent, req.Model)
	if isResponsesRelay(info) {
		respPayload := buildResponsesResponse(
			buildResponsesResponseID(conversationID),
			buildResponsesMessageID(conversationID),
			time.Now().Unix(),
			strings.TrimSpace(req.Model),
			content,
			usage,
		)
		payloadBytes, err := common.Marshal(respPayload)
		if err != nil {
			return nil, fmt.Errorf("chatgpt web channel: marshal responses response failed: %w", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
		}, nil
	}
	respPayload := chatResponse{
		Id:      buildChatCompletionID(conversationID),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   strings.TrimSpace(req.Model),
		Choices: []dto.OpenAITextResponseChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: "stop",
		}},
		Usage:          usage,
		ConversationID: conversationID,
	}
	payloadBytes, err := common.Marshal(respPayload)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web channel: marshal chat response failed: %w", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
	}, nil
}

func playgroundDebugCapture(c *gin.Context) func([]byte) {
	if !service.ShouldCapturePlaygroundUpstreamRequestDebug(c) {
		return nil
	}
	return func(body []byte) {
		service.RecordPlaygroundUpstreamRequestDebug(c, body)
	}
}

func newClientFromRelayInfo(ctx context.Context, info *relaycommon.RelayInfo, timings ...*service.ChatGPTWebTiming) (*Client, error) {
	timing := firstChatGPTWebTiming(timings...)
	if info == nil {
		return nil, errors.New("chatgpt web channel: relay info is required")
	}
	oauthKey, err := ParseOAuthKey(info.ApiKey)
	if err != nil {
		return nil, err
	}
	resolveStart := time.Now()
	accessToken, err := ResolveAccessToken(ctx, oauthKey, info.ChannelSetting.Proxy)
	if timing != nil {
		timing.ObserveSince("resolve_access_token_ms", resolveStart)
	}
	if err != nil {
		return nil, err
	}
	clientStart := time.Now()
	client, cacheHit, err := getCachedClient(ClientOptions{
		BaseURL:    chooseBaseURL(info),
		AuthToken:  accessToken,
		DeviceID:   strings.TrimSpace(oauthKey.DeviceID),
		SessionID:  strings.TrimSpace(oauthKey.SessionID),
		ProxyURL:   strings.TrimSpace(info.ChannelSetting.Proxy),
		Timeout:    chatGPTWebHTTPTimeout(),
		SSETimeout: 300 * time.Second,
	})
	if timing != nil {
		timing.ObserveSince("client_init_ms", clientStart)
		timing.Set("client_cache_hit", cacheHit)
	}
	return client, err
}

func chatGPTWebHTTPTimeout() time.Duration {
	return chatGPTWebDurationFromEnv("CHATGPT_WEB_HTTP_TIMEOUT_SECONDS", chatGPTWebHTTPDefaultTimeout, time.Minute)
}

func chatGPTWebImageRunTimeout() time.Duration {
	return chatGPTWebDurationFromEnv("CHATGPT_WEB_IMAGE_RUN_TIMEOUT_SECONDS", chatGPTWebImageRunDefaultTimeout, chatGPTWebImagePollDefaultMaxWait)
}

func chatGPTWebImagePollMaxWait(testMode bool) time.Duration {
	if testMode {
		return chatGPTWebImagePollTestMaxWait
	}
	return chatGPTWebDurationFromEnv("CHATGPT_WEB_IMAGE_POLL_TIMEOUT_SECONDS", chatGPTWebImagePollDefaultMaxWait, time.Minute)
}

func chatGPTWebImageDownloadURLMaxWait(testMode bool) time.Duration {
	if testMode {
		return chatGPTWebImageDownloadURLTestMaxWait
	}
	return chatGPTWebDurationFromEnv("CHATGPT_WEB_IMAGE_DOWNLOAD_URL_TIMEOUT_SECONDS", chatGPTWebImageDownloadURLDefaultMaxWait, 5*time.Second)
}

func chatGPTWebDurationFromEnv(name string, fallback, minimum time.Duration) time.Duration {
	seconds := common.GetEnvOrDefault(name, int(fallback/time.Second))
	duration := time.Duration(seconds) * time.Second
	if duration < minimum {
		return fallback
	}
	return duration
}

func chatGPTWebImageOperationContext(parent context.Context, testMode bool, timing *service.ChatGPTWebTiming) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if testMode {
		if timing != nil {
			timing.Set("image_request_context_detached", false)
		}
		return context.WithCancel(parent)
	}
	timeout := chatGPTWebImageRunTimeout() + chatGPTWebImageOperationGrace
	if timeout <= 0 {
		timeout = chatGPTWebImageRunDefaultTimeout + chatGPTWebImageOperationGrace
	}
	if timing != nil {
		timing.Set("image_request_context_detached", true)
		timing.Set("image_operation_timeout_seconds", int(timeout/time.Second))
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

func buildChatPrompt(req chatRequest) string {
	return buildChatPromptWithTextImageIntent(req, true)
}

func buildChatPromptForRelay(req chatRequest, info *relaycommon.RelayInfo) string {
	return buildChatPromptWithTextImageIntent(req, allowTextImageIntent(info))
}

func buildLatestChatGPTWebMessagePromptForRelay(req chatRequest, info *relaycommon.RelayInfo) string {
	return buildLatestChatGPTWebMessagePromptWithTextImageIntent(req, allowTextImageIntent(info))
}

func buildLatestChatGPTWebMessagePromptWithTextImageIntent(req chatRequest, allowTextIntent bool) string {
	if prompt := buildLatestChatGPTWebToolResultPrompt(req); prompt != "" {
		return prompt
	}
	content := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "" && role != "user" {
			continue
		}
		if text := strings.TrimSpace(messageTextContent(msg)); text != "" {
			content = text
			break
		}
	}
	if content == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(content)
	appendChatGPTWebToolInstruction(&b, req)
	imageGenerationIntent := common.IsImageGenerationModel(req.Model) || (allowTextIntent && chatRequestUserRequestsImageGeneration(req))
	if req.ResponseFormat != nil && !imageGenerationIntent {
		switch req.ResponseFormat.Type {
		case "json_object":
			b.WriteString("\n\nSystem: Please respond with a valid JSON object only.")
		case "json_schema":
			schemaBytes, _ := common.Marshal(req.ResponseFormat.JsonSchema)
			b.WriteString("\n\nSystem: Please respond with valid JSON matching this JSON Schema: ")
			b.Write(schemaBytes)
		}
	}
	if imageGenerationIntent {
		b.WriteString("\n\n")
		b.WriteString(chatImageGenerationInstruction)
	}
	return strings.TrimSpace(b.String())
}

func buildChatPromptWithTextImageIntent(req chatRequest, allowTextIntent bool) string {
	var b strings.Builder
	for _, msg := range req.Messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(messageTextContent(msg))
		if content == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		switch role {
		case "system", "developer":
			b.WriteString("System: ")
		case "assistant":
			b.WriteString("Assistant: ")
		case "tool":
			b.WriteString("Tool: ")
		default:
			b.WriteString("User: ")
		}
		b.WriteString(content)
	}
	appendChatGPTWebToolInstruction(&b, req)

	imageGenerationIntent := common.IsImageGenerationModel(req.Model) || (allowTextIntent && chatRequestUserRequestsImageGeneration(req))
	if req.ResponseFormat != nil && !imageGenerationIntent {
		switch req.ResponseFormat.Type {
		case "json_object":
			b.WriteString("\n\nSystem: Please respond with a valid JSON object only.")
		case "json_schema":
			schemaBytes, _ := common.Marshal(req.ResponseFormat.JsonSchema)
			b.WriteString("\n\nSystem: Please respond with valid JSON matching this JSON Schema: ")
			b.Write(schemaBytes)
		}
	}
	if imageGenerationIntent {
		b.WriteString("\n\n")
		b.WriteString(chatImageGenerationInstruction)
	}
	return strings.TrimSpace(b.String())
}

func appendChatGPTWebToolInstruction(b *strings.Builder, req chatRequest) {
	if b == nil || len(req.Tools) == 0 {
		return
	}
	instruction := buildChatGPTWebToolInstruction(req)
	if instruction == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(instruction)
}

func buildChatGPTWebToolInstruction(req chatRequest) string {
	if len(req.Tools) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("System: TOOL BRIDGE PROTOCOL. Treat the listed tools as your filesystem, shell, and project-inspection capability. If Tool results are already present in this conversation, use those results to answer and only call another tool when the provided results are insufficient. Otherwise, if the user's request requires local files, directories, shell/project state, repository inspection, or any tool-only information, you MUST call an available tool before answering. Do not say the filesystem is unavailable, do not describe what you would try, and do not invent tool results. For a tool call, reply with exactly one JSON object and no markdown or extra text in this shape: {\"tool_call\":{\"name\":\"<tool name>\",\"arguments\":{...}}}. Examples: {\"tool_call\":{\"name\":\"Read\",\"arguments\":{\"file_path\":\"AGENTS.md\"}}}; {\"tool_call\":{\"name\":\"LS\",\"arguments\":{\"path\":\".\"}}}. Use normal prose only when no tool is needed.\nAvailable tools:")
	for i, tool := range req.Tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			continue
		}
		description := strings.Join(strings.Fields(strings.TrimSpace(tool.Function.Description)), " ")
		parameters := "{}"
		if tool.Function.Parameters != nil {
			if data, err := common.Marshal(tool.Function.Parameters); err == nil && len(data) > 0 {
				parameters = string(data)
			}
		}
		line := fmt.Sprintf("\n- %s", name)
		if description != "" {
			line += ": " + description
		}
		line += "\n  input_schema: " + parameters
		if b.Len()+len(line) > chatGPTWebToolPromptMaxBytes {
			b.WriteString(fmt.Sprintf("\n- ... %d additional tools omitted to keep the prompt within the gateway limit.", len(req.Tools)-i))
			break
		}
		b.WriteString(line)
	}
	if req.ToolChoice != nil {
		if data, err := common.Marshal(req.ToolChoice); err == nil && len(data) > 0 {
			line := "\nTool choice constraint: " + string(data)
			if b.Len()+len(line) <= chatGPTWebToolPromptMaxBytes {
				b.WriteString(line)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func buildLatestChatGPTWebToolResultPrompt(req chatRequest) string {
	latestUserIdx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "" && role != "user" {
			continue
		}
		if strings.TrimSpace(messageTextContent(msg)) != "" {
			latestUserIdx = i
			break
		}
	}

	toolResults := make([]string, 0)
	for i := latestUserIdx + 1; i < len(req.Messages); i++ {
		if i < 0 {
			continue
		}
		msg := req.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "tool" && role != "function" {
			continue
		}
		content := strings.TrimSpace(messageTextContent(msg))
		if content == "" {
			continue
		}
		label := "Tool result"
		if msg.Name != nil && strings.TrimSpace(*msg.Name) != "" {
			label += " from " + strings.TrimSpace(*msg.Name)
		} else if strings.TrimSpace(msg.ToolCallId) != "" {
			label += " " + strings.TrimSpace(msg.ToolCallId)
		}
		toolResults = append(toolResults, label+":\n"+content)
	}
	if len(toolResults) == 0 {
		return ""
	}

	var b strings.Builder
	if latestUserIdx >= 0 {
		if userText := strings.TrimSpace(messageTextContent(req.Messages[latestUserIdx])); userText != "" {
			b.WriteString("User request:\n")
			b.WriteString(userText)
			b.WriteString("\n\n")
		}
	}
	b.WriteString(strings.Join(toolResults, "\n\n"))
	b.WriteString("\n\nSystem: Use the tool result above to answer the user's request. Do not say the local filesystem, project directory, or shell is unavailable because the tool result has already been provided. Only request another tool call if the provided result is insufficient.")
	return strings.TrimSpace(b.String())
}

func chatGPTWebHasToolResultAfterLatestUser(req chatRequest) bool {
	latestUserIdx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "" && role != "user" {
			continue
		}
		if strings.TrimSpace(messageTextContent(msg)) != "" {
			latestUserIdx = i
			break
		}
	}
	for i := latestUserIdx + 1; i < len(req.Messages); i++ {
		if i < 0 {
			continue
		}
		msg := req.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if (role == "tool" || role == "function") && strings.TrimSpace(messageTextContent(msg)) != "" {
			return true
		}
	}
	return false
}

func messageTextContent(msg dto.Message) string {
	if msg.IsStringContent() {
		return msg.StringContent()
	}
	parts := msg.ParseContent()
	if len(parts) == 0 {
		return msg.StringContent()
	}
	var b strings.Builder
	for _, part := range parts {
		switch part.Type {
		case dto.ContentTypeText:
			b.WriteString(part.Text)
		case dto.ContentTypeImageURL:
			if image := part.GetImageMedia(); image != nil && image.Url != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString("[image_url: ")
				b.WriteString(image.Url)
				b.WriteString("]")
			}
		}
	}
	return b.String()
}

type chatGPTWebToolCallPayload struct {
	ToolCall  *chatGPTWebToolCallSpec  `json:"tool_call"`
	ToolCalls []chatGPTWebToolCallSpec `json:"tool_calls"`
	Name      string                   `json:"name"`
	Arguments json.RawMessage          `json:"arguments"`
	Function  *chatGPTWebToolFunction  `json:"function"`
}

type chatGPTWebToolCallSpec struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Arguments json.RawMessage         `json:"arguments"`
	Function  *chatGPTWebToolFunction `json:"function"`
}

type chatGPTWebToolFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func parseChatGPTWebToolCall(content string, tools []dto.ToolCallRequest) (dto.ToolCallResponse, bool) {
	allowed := make(map[string]string)
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name != "" {
			allowed[strings.ToLower(name)] = name
		}
	}
	if len(allowed) == 0 {
		return dto.ToolCallResponse{}, false
	}
	for _, candidate := range chatGPTWebToolCallJSONCandidates(content) {
		var payload chatGPTWebToolCallPayload
		if err := common.Unmarshal(common.StringToByteSlice(candidate), &payload); err != nil {
			continue
		}
		if call, ok := chatGPTWebToolCallFromPayload(payload, allowed); ok {
			return call, true
		}
	}
	return dto.ToolCallResponse{}, false
}

func chatGPTWebToolCallFromPayload(payload chatGPTWebToolCallPayload, allowed map[string]string) (dto.ToolCallResponse, bool) {
	if payload.ToolCall != nil {
		return chatGPTWebToolCallFromSpec(*payload.ToolCall, allowed)
	}
	for _, spec := range payload.ToolCalls {
		if call, ok := chatGPTWebToolCallFromSpec(spec, allowed); ok {
			return call, true
		}
	}
	spec := chatGPTWebToolCallSpec{
		Name:      payload.Name,
		Arguments: payload.Arguments,
		Function:  payload.Function,
	}
	return chatGPTWebToolCallFromSpec(spec, allowed)
}

func chatGPTWebToolCallFromSpec(spec chatGPTWebToolCallSpec, allowed map[string]string) (dto.ToolCallResponse, bool) {
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
		callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return dto.ToolCallResponse{
		ID:   callID,
		Type: "function",
		Function: dto.FunctionResponse{
			Name:      canonicalName,
			Arguments: normalizeChatGPTWebToolArguments(args),
		},
	}, true
}

func normalizeChatGPTWebToolArguments(raw json.RawMessage) string {
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

func chatGPTWebToolCallJSONCandidates(content string) []string {
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
	unfenced := stripChatGPTWebJSONCodeFence(trimmed)
	add(&candidates, unfenced)
	for _, candidate := range balancedJSONObjectCandidates(unfenced) {
		add(&candidates, candidate)
	}
	return candidates
}

func shouldContinueBufferingChatGPTWebToolCandidate(content string) bool {
	trimmed := strings.TrimLeft(strings.TrimSpace(content), "\ufeff")
	if trimmed == "" {
		return true
	}
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "```")
}

func stripChatGPTWebJSONCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 {
		return trimmed
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return trimmed
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func balancedJSONObjectCandidates(content string) []string {
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

type chatStreamStart struct {
	Stream   <-chan SSEEvent
	Baseline imageBaseline
	Prompt   string
}

func runChatCompletion(ctx context.Context, client *Client, req chatRequest, prompt string, captureRequestBody func([]byte), timings ...*service.ChatGPTWebTiming) (string, string, string, imageBaseline, bool, error) {
	timing := firstChatGPTWebTiming(timings...)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	started, err := startChatStream(ctx, client, req, prompt, captureRequestBody, timing)
	if err != nil {
		return "", "", "", imageBaseline{}, false, err
	}
	parseStart := time.Now()
	result := ParseChatSSE(started.Stream)
	if timing != nil {
		timing.ObserveSince("chat_stream_parse_ms", parseStart)
		timing.Set("has_image_generation", result.HasImageGeneration)
		timing.Set("has_inline_image", result.HasInlineImage)
		timing.Set("stream_handoff", result.HasStreamHandoff)
	}
	if result.Err != nil {
		return "", result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, result.Err
	}
	if containsImageGenerationUpstreamErrorText(result.Content) {
		return "", result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, imageGenerationUpstreamError()
	}
	if strings.TrimSpace(result.Content) == "" {
		recovered := ""
		if req.DeepResearch && result.HasDeepResearchInternalEvent {
			recovered = recoverDeepResearchTextFromConversation(ctx, client, result.ConversationID, timing)
		} else if result.HasStreamHandoff {
			recovered = recoverHandoffTextFromConversation(ctx, client, result.ConversationID, timing)
		} else {
			recovered = recoverChatCompletionTextFromConversation(ctx, client, result.ConversationID, timing)
		}
		if recovered != "" {
			result.Content = recovered
		}
	}
	if strings.TrimSpace(result.Content) == "" && req.DeepResearch && result.HasDeepResearchInternalEvent {
		result.Content = chatGPTWebDeepResearchPendingMessage
	}
	if strings.TrimSpace(result.Content) == "" {
		return "", result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, errors.New("chatgpt web channel: empty chat response")
	}
	return result.Content, result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, nil
}

func runChatCompletionProbe(ctx context.Context, client *Client, req chatRequest, prompt string, captureRequestBody func([]byte), timings ...*service.ChatGPTWebTiming) (string, string, string, error) {
	timing := firstChatGPTWebTiming(timings...)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	started, err := startChatStream(ctx, client, req, prompt, captureRequestBody, timing)
	if err != nil {
		return "", "", "", err
	}
	probeStart := time.Now()
	result := ParseChatSSEUntilReady(started.Stream, 3*time.Second)
	if timing != nil {
		timing.ObserveSince("chat_probe_parse_ms", probeStart)
		timing.Set("has_image_generation", result.HasImageGeneration)
		timing.Set("has_inline_image", result.HasInlineImage)
		timing.Set("stream_handoff", result.HasStreamHandoff)
	}
	if result.Err != nil {
		return "", result.ConversationID, started.Prompt, result.Err
	}
	if containsImageGenerationUpstreamErrorText(result.Content) {
		return "", result.ConversationID, started.Prompt, imageGenerationUpstreamError()
	}
	if strings.TrimSpace(result.Content) == "" {
		recovered := ""
		if result.HasStreamHandoff {
			recovered = recoverHandoffTextFromConversation(ctx, client, result.ConversationID, timing)
		} else {
			recovered = recoverChatCompletionTextFromConversation(ctx, client, result.ConversationID, timing)
		}
		if recovered != "" {
			result.Content = recovered
		}
	}
	if strings.TrimSpace(result.ConversationID) == "" && strings.TrimSpace(result.Content) == "" {
		return "", "", started.Prompt, errors.New("chatgpt web channel: chat test did not receive a conversation id or content")
	}
	return result.Content, result.ConversationID, started.Prompt, nil
}

func recoverChatCompletionTextFromConversation(ctx context.Context, client *Client, conversationID string, timings ...*service.ChatGPTWebTiming) string {
	return recoverChatCompletionTextFromConversationWithWait(ctx, client, conversationID, 0, 0, timings...)
}

func recoverDeepResearchTextFromConversation(ctx context.Context, client *Client, conversationID string, timings ...*service.ChatGPTWebTiming) string {
	return recoverChatCompletionTextFromConversationWithWait(ctx, client, conversationID, chatGPTWebDeepResearchRecoverMaxWait, chatGPTWebDeepResearchRecoverInterval, timings...)
}

func recoverHandoffTextFromConversation(ctx context.Context, client *Client, conversationID string, timings ...*service.ChatGPTWebTiming) string {
	return recoverChatCompletionTextFromConversationWithWait(ctx, client, conversationID, chatGPTWebHandoffRecoverMaxWait, chatGPTWebHandoffRecoverInterval, timings...)
}

func recoverChatCompletionTextFromConversationWithWait(ctx context.Context, client *Client, conversationID string, maxWait, interval time.Duration, timings ...*service.ChatGPTWebTiming) string {
	timing := firstChatGPTWebTiming(timings...)
	conversationID = strings.TrimSpace(conversationID)
	if client == nil || conversationID == "" {
		return ""
	}
	if timing != nil {
		timing.Set("chat_text_mapping_conversation_hash", chatGPTWebShortHash(conversationID))
	}
	return recoverChatCompletionTextWithFetcher(ctx, func(fetchCtx context.Context) (string, error) {
		recoverStart := time.Now()
		mapping, err := client.GetConversationMapping(fetchCtx, conversationID)
		if timing != nil {
			timing.ObserveSince("chat_text_mapping_ms", recoverStart)
		}
		if err != nil {
			return "", err
		}
		if text := ExtractLatestAssistantTextFromConversation(mapping); strings.TrimSpace(text) != "" {
			return text, nil
		}
		if text := ExtractDeepResearchReportTextFromConversation(mapping); strings.TrimSpace(text) != "" {
			return text, nil
		}
		if ChatGPTWebConversationHasDeepResearchEmbeddedUI(mapping) {
			return chatGPTWebDeepResearchEmbeddedUIMessage, nil
		}
		return "", nil
	}, maxWait, interval, timing)
}

type chatCompletionTextFetcher func(context.Context) (string, error)

func recoverChatCompletionTextWithFetcher(ctx context.Context, fetch chatCompletionTextFetcher, maxWait, interval time.Duration, timing *service.ChatGPTWebTiming) string {
	if fetch == nil {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = time.Second
	}
	start := time.Now()
	recoverCtx := ctx
	cancel := func() {}
	if maxWait > 0 {
		recoverCtx, cancel = context.WithTimeout(ctx, maxWait)
	}
	defer cancel()
	attempts := 0
	for {
		attempts++
		text, err := fetch(recoverCtx)
		if timing != nil {
			timing.Set("chat_text_mapping_attempts", attempts)
		}
		if err != nil {
			if timing != nil {
				timing.Set("chat_text_mapping_error", common.MaskSensitiveInfo(err.Error()))
			}
		} else if strings.TrimSpace(text) != "" {
			if timing != nil {
				timing.Set("chat_text_mapping_recovered", true)
				timing.ObserveSince("chat_text_recovery_ms", start)
			}
			return text
		}
		if maxWait <= 0 {
			if timing != nil {
				timing.Set("chat_text_mapping_recovered", false)
				timing.ObserveSince("chat_text_recovery_ms", start)
			}
			return ""
		}
		timer := time.NewTimer(interval)
		select {
		case <-recoverCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if timing != nil {
				timing.Set("chat_text_mapping_recovered", false)
				timing.Set("chat_text_mapping_timeout", true)
				timing.Set("chat_text_mapping_context_error", recoverCtx.Err().Error())
				timing.ObserveSince("chat_text_recovery_ms", start)
			}
			return ""
		case <-timer.C:
		}
	}
}

func collectChatGeneratedImageMarkdown(ctx context.Context, client *Client, conversationID string, baseline imageBaseline, allowPoll bool, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string, timings ...*service.ChatGPTWebTiming) (string, error) {
	timing := firstChatGPTWebTiming(timings...)
	conversationID = strings.TrimSpace(conversationID)
	if client == nil || conversationID == "" {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	mappingStart := time.Now()
	mapping, err := client.getMappingRaw(ctx, conversationID)
	if timing != nil {
		timing.ObserveSince("chat_image_mapping_ms", mappingStart)
	}
	if err != nil {
		return "", nil
	}
	if mappingContainsImageGenerationError(mapping) {
		return "", imageGenerationUpstreamError()
	}
	toolMsgs := ExtractImageToolMsgs(mapping)
	if len(baseline.ToolIDs) > 0 {
		filtered := make([]ImageToolMsg, 0, len(toolMsgs))
		for _, msg := range toolMsgs {
			if _, ok := baseline.ToolIDs[msg.MessageID]; !ok {
				filtered = append(filtered, msg)
			}
		}
		toolMsgs = filtered
	}
	fileRefs, hasFileRefs := imageRefsFromToolMsgs(toolMsgs)
	if !hasFileRefs {
		if !allowPoll && len(toolMsgs) == 0 {
			return "", nil
		}
		pollStart := time.Now()
		pollStatus, fids, sids := client.PollConversationForImages(ctx, conversationID, PollOpts{
			MaxWait:             chatGPTWebChatImagePollMaxWait,
			Interval:            2 * time.Second,
			StableRounds:        2,
			PreviewWait:         8 * time.Second,
			BaselineToolIDs:     baseline.ToolIDs,
			BaselineFileIDs:     baseline.FileIDs,
			BaselineSedimentIDs: baseline.SedimentIDs,
		})
		if timing != nil {
			timing.ObserveSince("chat_image_poll_ms", pollStart)
			timing.Set("chat_image_poll_status", string(pollStatus))
		}
		switch pollStatus {
		case PollStatusIMG2, PollStatusPreviewOnly:
			fileRefs = append(fileRefs, fids...)
			for _, sid := range sids {
				fileRefs = append(fileRefs, "sed:"+sid)
			}
		case PollStatusImageError:
			return "", imageGenerationUpstreamError()
		}
	}
	if len(fileRefs) == 0 {
		return "", nil
	}
	markdownStart := time.Now()
	markdown := imageRefsToMarkdown(ctx, client, conversationID, fileRefs, info, prompt, modelName, publicBaseURL, timing)
	if timing != nil {
		timing.ObserveSince("chat_image_markdown_ms", markdownStart)
		timing.Set("chat_image_ref_count", len(fileRefs))
	}
	return markdown, nil
}

func shouldPollChatGeneratedImages(req chatRequest, prompt, content string, hasImageGeneration bool) bool {
	return shouldPollChatGeneratedImagesWithTextIntent(req, prompt, content, hasImageGeneration, true)
}

func shouldPollChatGeneratedImagesForRelay(info *relaycommon.RelayInfo, req chatRequest, prompt, content string, hasImageGeneration bool) bool {
	return shouldPollChatGeneratedImagesWithTextIntent(req, prompt, content, hasImageGeneration, allowTextImageIntent(info))
}

func shouldPollChatGeneratedImagesWithTextIntent(req chatRequest, prompt, content string, hasImageGeneration bool, allowTextIntent bool) bool {
	if hasImageGeneration {
		return true
	}
	if common.IsImageGenerationModel(req.Model) {
		return true
	}
	if !allowTextIntent {
		return false
	}
	// Only the current user request should opt into best-effort image polling.
	// The built prompt contains assistant/history/system text, and assistant output
	// may mention image generation while explaining capabilities or latency; using
	// either as intent makes normal streams wait for the image poll timeout before
	// sending the final SSE terminator. Actual image tool execution is still
	// covered by hasImageGeneration above.
	return chatRequestUserRequestsImageGeneration(req)
}

func chatRequestUserRequestsImageGeneration(req chatRequest) bool {
	return chatTextRequestsImageGeneration(latestChatGPTWebUserText(req))
}

func allowTextImageIntent(info *relaycommon.RelayInfo) bool {
	return !isResponsesRelay(info)
}

func chatTextRequestsImageGeneration(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	chineseImageTerms := []string{"生图", "画图", "绘图", "生成图片", "生成图像", "生成照片", "图片生成", "图像生成"}
	for _, term := range chineseImageTerms {
		if strings.Contains(text, term) {
			return true
		}
	}
	chineseImageNouns := []string{"图片", "图像", "照片", "相片", "插画", "画面", "海报", "头像", "壁纸"}
	chineseActionTerms := []string{"生成", "创建", "创作", "画", "绘制", "制作", "设计", "做一张", "来一张", "出一张"}
	hasChineseImageNoun := false
	for _, term := range chineseImageNouns {
		if strings.Contains(text, term) {
			hasChineseImageNoun = true
			break
		}
	}
	if hasChineseImageNoun {
		for _, term := range chineseActionTerms {
			if strings.Contains(text, term) {
				return true
			}
		}
	}
	imageTerms := []string{"image", "picture", "photo", "illustration", "drawing"}
	actionTerms := []string{"generate", "create", "draw", "make", "render"}
	hasImageTerm := false
	for _, term := range imageTerms {
		if strings.Contains(text, term) {
			hasImageTerm = true
			break
		}
	}
	if !hasImageTerm {
		return false
	}
	for _, term := range actionTerms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func imageRefsFromToolMsgs(toolMsgs []ImageToolMsg) ([]string, bool) {
	fileRefs := make([]string, 0)
	hasFileRefs := false
	for _, msg := range toolMsgs {
		for _, fid := range msg.FileIDs {
			hasFileRefs = true
			fileRefs = append(fileRefs, fid)
		}
		for _, sid := range msg.SedimentIDs {
			fileRefs = append(fileRefs, "sed:"+sid)
		}
	}
	return dedupeStrings(fileRefs), hasFileRefs
}

func imageRefsToMarkdown(ctx context.Context, client *Client, conversationID string, fileRefs []string, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string, timings ...*service.ChatGPTWebTiming) string {
	timing := firstChatGPTWebTiming(timings...)
	refs := dedupeStrings(fileRefs)
	if len(refs) == 0 {
		return ""
	}
	publicURLs, ok := materializeImageRefsToPublicURLs(ctx, client, conversationID, refs, info, prompt, modelName, publicBaseURL, timing)
	if ok {
		return imageURLsToMarkdown(publicURLs)
	}
	var b strings.Builder
	for index, ref := range refs {
		downloadURLStart := time.Now()
		signedURL, err := client.ImageDownloadURL(ctx, conversationID, ref)
		if timing != nil {
			timing.ObserveSince("chat_image_download_url_ms", downloadURLStart)
		}
		if err != nil || strings.TrimSpace(signedURL) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("![image_%d](%s)", index+1, signedURL))
	}
	return b.String()
}

func imageURLsToMarkdown(urls []string) string {
	var b strings.Builder
	for index, imageURL := range urls {
		imageURL = strings.TrimSpace(imageURL)
		if imageURL == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("![image_%d](%s)", index+1, imageURL))
	}
	return b.String()
}

func chatContentHasChatGPTImageURL(content string) bool {
	return len(extractChatGPTImageURLs(content)) > 0
}

func materializeChatGPTContentImageURLs(ctx context.Context, client *Client, content string, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string, timings ...*service.ChatGPTWebTiming) string {
	timing := firstChatGPTWebTiming(timings...)
	urls := extractChatGPTImageURLs(content)
	if len(urls) == 0 || client == nil || info == nil || info.UserId <= 0 {
		return content
	}
	if timing != nil {
		timing.Set("content_image_url_count", len(urls))
	}
	data := make([]dto.ImageData, 0, len(urls))
	replacements := make(map[string]string, len(urls))
	for _, imageURL := range urls {
		fetchURL := strings.ReplaceAll(strings.TrimSpace(imageURL), "&amp;", "&")
		fetchStart := time.Now()
		imageBytes, _, err := client.FetchImage(ctx, fetchURL, 20*1024*1024)
		if timing != nil {
			timing.ObserveSince("content_image_fetch_ms", fetchStart)
		}
		if err != nil || len(imageBytes) == 0 {
			replacements[imageURL] = ""
			continue
		}
		index := len(data)
		data = append(data, dto.ImageData{B64Json: base64.StdEncoding.EncodeToString(imageBytes)})
		replacements[imageURL] = publicPlaygroundImageURL(publicBaseURL, "__pending__", index)
	}
	if len(data) == 0 {
		return replaceChatGPTImageURLs(content, replacements)
	}
	taskStart := time.Now()
	publicURLs, ok := createPublicPlaygroundImageTask(info, prompt, modelName, data, publicBaseURL)
	if timing != nil {
		timing.ObserveSince("content_image_task_insert_ms", taskStart)
	}
	if !ok || len(publicURLs) == 0 {
		for original := range replacements {
			replacements[original] = ""
		}
		return replaceChatGPTImageURLs(content, replacements)
	}
	for original, replacement := range replacements {
		if replacement == "" {
			continue
		}
		index := strings.LastIndex(replacement, "/")
		if index < 0 {
			continue
		}
		imageIndex, err := strconv.Atoi(replacement[index+1:])
		if err != nil || imageIndex < 0 || imageIndex >= len(publicURLs) {
			continue
		}
		replacements[original] = publicURLs[imageIndex]
	}
	return replaceChatGPTImageURLs(content, replacements)
}

func extractChatGPTImageURLs(content string) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case ' ', '\n', '\r', '\t', '(', ')', '[', ']', '<', '>', '"', '\'':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0)
	for _, field := range fields {
		field = strings.TrimSpace(strings.TrimRight(field, ".,;"))
		if isChatGPTImageURL(field) {
			out = append(out, field)
		}
	}
	return dedupeStrings(out)
}

func isChatGPTImageURL(rawURL string) bool {
	rawURL = strings.ReplaceAll(strings.TrimSpace(rawURL), "&amp;", "&")
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	path := parsed.EscapedPath()
	if !strings.Contains(path, "/backend-api/estuary/content") &&
		!strings.Contains(path, "/backend-api/conversation/") &&
		!strings.Contains(path, "/backend-api/files/") {
		return false
	}
	host := strings.ToLower(parsed.Host)
	return host == "chatgpt.com" ||
		strings.HasSuffix(host, ".chatgpt.com") ||
		strings.HasPrefix(host, "127.0.0.1:") ||
		strings.HasPrefix(host, "localhost:")
}

func replaceChatGPTImageURLs(content string, replacements map[string]string) string {
	out := content
	imageIndex := 1
	for original, replacement := range replacements {
		if strings.TrimSpace(replacement) == "" {
			replacement = "[image unavailable]"
		}
		out = replaceChatGPTImageURLVariant(out, original, replacement, &imageIndex)
		out = replaceChatGPTImageURLVariant(out, strings.ReplaceAll(original, "&", "&amp;"), replacement, &imageIndex)
	}
	return out
}

func replaceChatGPTImageURLVariant(content, original, replacement string, imageIndex *int) string {
	original = strings.TrimSpace(original)
	if original == "" || !strings.Contains(content, original) {
		return content
	}
	var b strings.Builder
	start := 0
	for {
		index := strings.Index(content[start:], original)
		if index < 0 {
			b.WriteString(content[start:])
			break
		}
		index += start
		end := index + len(original)
		b.WriteString(content[start:index])
		if replacement == "[image unavailable]" || isMarkdownDestination(content, index, end) {
			b.WriteString(replacement)
		} else {
			b.WriteString(fmt.Sprintf("![image_%d](%s)", *imageIndex, replacement))
			*imageIndex++
		}
		start = end
	}
	return b.String()
}

func isMarkdownDestination(content string, start, end int) bool {
	return start >= 2 && content[start-2:start] == "](" && end < len(content) && content[end] == ')'
}

func materializeImageRefsToPublicURLs(ctx context.Context, client *Client, conversationID string, refs []string, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string, timings ...*service.ChatGPTWebTiming) ([]string, bool) {
	timing := firstChatGPTWebTiming(timings...)
	if client == nil || info == nil || info.UserId <= 0 || len(refs) == 0 {
		return nil, false
	}
	data := make([]dto.ImageData, 0, len(refs))
	for _, ref := range refs {
		downloadURLStart := time.Now()
		signedURL, err := client.ImageDownloadURL(ctx, conversationID, ref)
		if timing != nil {
			timing.ObserveSince("image_ref_download_url_ms", downloadURLStart)
		}
		if err != nil || strings.TrimSpace(signedURL) == "" {
			continue
		}
		fetchStart := time.Now()
		imageBytes, _, err := client.FetchImage(ctx, signedURL, 20*1024*1024)
		if timing != nil {
			timing.ObserveSince("image_ref_fetch_ms", fetchStart)
		}
		if err != nil || len(imageBytes) == 0 {
			continue
		}
		data = append(data, dto.ImageData{B64Json: base64.StdEncoding.EncodeToString(imageBytes)})
	}
	if len(data) == 0 {
		return nil, false
	}
	taskStart := time.Now()
	urls, ok := createPublicPlaygroundImageTask(info, prompt, modelName, data, publicBaseURL)
	if timing != nil {
		timing.ObserveSince("image_ref_task_insert_ms", taskStart)
	}
	return urls, ok
}

func createPublicPlaygroundImageTask(info *relaycommon.RelayInfo, prompt, modelName string, data []dto.ImageData, publicBaseURL string) ([]string, bool) {
	if info == nil || info.UserId <= 0 || len(data) == 0 {
		return nil, false
	}
	taskID := model.GenerateTaskID()
	now := time.Now().Unix()
	group := strings.TrimSpace(info.UsingGroup)
	if group == "" {
		group = strings.TrimSpace(info.UserGroup)
	}
	channelID := 0
	if info.ChannelMeta != nil {
		channelID = info.ChannelMeta.ChannelId
	}
	task := &model.Task{
		TaskID:     taskID,
		UserId:     info.UserId,
		Group:      group,
		SubmitTime: now,
		StartTime:  now,
		FinishTime: now,
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		ChannelId:  channelID,
		Platform:   constant.TaskPlatformPlaygroundImage,
		Action:     constant.TaskActionGenerate,
		Properties: model.Properties{
			Input:             strings.TrimSpace(prompt),
			UpstreamModelName: strings.TrimSpace(modelName),
			OriginModelName:   strings.TrimSpace(modelName),
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: taskID,
			ResultURL:      publicPlaygroundImageURL(publicBaseURL, taskID, 0),
		},
	}
	task.SetData(dto.ImageResponse{
		Created: now,
		Data:    data,
	})
	if err := task.Insert(); err != nil {
		return nil, false
	}
	urls := make([]string, 0, len(data))
	for index := range data {
		urls = append(urls, publicPlaygroundImageURL(publicBaseURL, taskID, index))
	}
	return urls, true
}

func publicPlaygroundImageURL(baseURL, taskID string, index int) string {
	path := fmt.Sprintf("/pg/public/images/generations/%s/image/%d", url.PathEscape(strings.TrimSpace(taskID)), index)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return path
	}
	return baseURL + path
}

func requestPublicBaseURLForImages(c *gin.Context, info *relaycommon.RelayInfo) string {
	if info != nil && info.IsPlayground {
		return configuredPublicBaseURL()
	}
	return requestPublicBaseURL(c)
}

func requestPublicBaseURL(c *gin.Context) string {
	if baseURL := configuredPublicBaseURL(); baseURL != "" {
		return baseURL
	}
	if c == nil || c.Request == nil {
		return ""
	}
	forwarded := firstForwardedValue(c.GetHeader("Forwarded"))
	host := forwardedHeaderParam(forwarded, "host")
	if host == "" {
		host = firstForwardedValue(c.GetHeader("X-Forwarded-Host"))
	}
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	host = normalizeForwardedHost(host)
	if host == "" {
		return ""
	}

	proto := forwardedHeaderParam(forwarded, "proto")
	if proto == "" {
		proto = firstForwardedValue(c.GetHeader("X-Forwarded-Proto"))
	}
	if proto == "" {
		proto = firstForwardedValue(c.GetHeader("X-Forwarded-Scheme"))
	}
	if proto == "" {
		if c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	if !hostHasPort(host) {
		port := requestForwardedPort(c)
		if !isDefaultPortForProto(port, proto) {
			host = appendPortToHost(host, port)
		}
	}
	return proto + "://" + host
}

func configuredPublicBaseURL() string {
	return normalizePublicBaseURL(system_setting.ServerAddress)
}

func normalizePublicBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return ""
	}
	authority := parsed.Host
	if port := normalizeForwardedPort(parsed.Port()); isDefaultPortForProto(port, scheme) {
		authority = parsed.Hostname()
		if strings.Contains(authority, ":") {
			authority = "[" + strings.Trim(authority, "[]") + "]"
		}
	}
	return scheme + "://" + authority
}

func forwardedHeaderParam(forwarded, key string) string {
	forwarded = strings.TrimSpace(forwarded)
	if forwarded == "" || key == "" {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	for _, part := range strings.Split(forwarded, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), key) {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`)
	}
	return ""
}

func normalizeForwardedHost(host string) string {
	host = strings.Trim(strings.TrimSpace(host), `"`)
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
			host = parsed.Host
		}
	}
	if slash := strings.IndexByte(host, '/'); slash >= 0 {
		host = host[:slash]
	}
	return strings.TrimSpace(host)
}

func requestForwardedPort(c *gin.Context) string {
	if c == nil {
		return ""
	}
	for _, header := range []string{"X-Forwarded-Port", "X-Real-Port", "X-Original-Port"} {
		if port := normalizeForwardedPort(firstForwardedValue(c.GetHeader(header))); port != "" {
			return port
		}
	}
	return ""
}

func normalizeForwardedPort(port string) string {
	port = strings.Trim(strings.TrimSpace(port), `"`)
	if port == "" {
		return ""
	}
	if n, err := strconv.Atoi(port); err != nil || n <= 0 || n > 65535 {
		return ""
	}
	return port
}

func isDefaultPortForProto(port, proto string) bool {
	port = normalizeForwardedPort(port)
	proto = strings.ToLower(strings.TrimSpace(proto))
	return (proto == "http" && port == "80") || (proto == "https" && port == "443")
}

func hostHasPort(host string) bool {
	host = normalizeForwardedHost(host)
	if host == "" {
		return false
	}
	_, port, err := net.SplitHostPort(host)
	return err == nil && port != ""
}

func appendPortToHost(host, port string) string {
	host = normalizeForwardedHost(host)
	port = normalizeForwardedPort(port)
	if host == "" || port == "" || hostHasPort(host) {
		return host
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	return net.JoinHostPort(host, port)
}

func firstForwardedValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if comma := strings.Index(value, ","); comma >= 0 {
		value = value[:comma]
	}
	return strings.TrimSpace(value)
}

func appendMarkdownBlock(content, markdown string) string {
	content = strings.TrimSpace(content)
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return content
	}
	if content == "" {
		return markdown
	}
	return content + "\n\n" + markdown
}

func materializePlaygroundInlineDataImages(content string, info *relaycommon.RelayInfo, prompt, modelName string) string {
	if info == nil || !info.IsPlayground || !chatContentHasInlineDataImage(content) {
		return content
	}
	matches := reMarkdownDataImage.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	taskID := model.GenerateTaskID()
	data := make([]dto.ImageData, 0, len(matches))
	var b strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 6 || match[0] < last {
			continue
		}
		alt := strings.TrimSpace(content[match[2]:match[3]])
		dataURL := content[match[4]:match[5]]
		_, b64, ok := splitInlineDataImageURL(dataURL)
		if !ok {
			continue
		}
		b.WriteString(content[last:match[0]])
		imageIndex := len(data)
		if alt == "" {
			alt = fmt.Sprintf("image_%d", imageIndex+1)
		}
		b.WriteString(fmt.Sprintf("![%s](/pg/images/generations/%s/image/%d)", alt, taskID, imageIndex))
		data = append(data, dto.ImageData{B64Json: b64})
		last = match[1]
	}
	if len(data) == 0 {
		return content
	}
	b.WriteString(content[last:])

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     taskID,
		UserId:     info.UserId,
		Group:      strings.TrimSpace(info.UsingGroup),
		SubmitTime: now,
		StartTime:  now,
		FinishTime: now,
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		ChannelId:  0,
		Platform:   constant.TaskPlatformPlaygroundImage,
		Action:     constant.TaskActionGenerate,
		Properties: model.Properties{
			Input:             strings.TrimSpace(prompt),
			UpstreamModelName: strings.TrimSpace(modelName),
			OriginModelName:   strings.TrimSpace(modelName),
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: taskID,
			ResultURL:      fmt.Sprintf("/pg/images/generations/%s/image/0", taskID),
		},
	}
	if info.ChannelMeta != nil {
		task.ChannelId = info.ChannelMeta.ChannelId
	}
	task.SetData(dto.ImageResponse{
		Created: now,
		Data:    data,
	})
	if err := task.Insert(); err != nil {
		return content
	}
	recordInlineChatDrawingLog(info, strings.TrimSpace(prompt), strings.TrimSpace(modelName), taskID, len(data))
	return strings.TrimSpace(b.String())
}

func recordInlineChatDrawingLog(info *relaycommon.RelayInfo, prompt, modelName, taskID string, imageCount int) {
	if info == nil || info.IsChannelTest || taskID == "" || imageCount <= 0 {
		return
	}
	submitTime := time.Now().UnixMilli()
	if !info.StartTime.IsZero() {
		submitTime = info.StartTime.UnixMilli()
	}
	finishTime := time.Now().UnixMilli()
	propertiesBytes, _ := common.Marshal(map[string]any{
		"source":     ChannelName,
		"model":      modelName,
		"task_id":    taskID,
		"inline":     true,
		"playground": info.IsPlayground,
	})
	properties := string(propertiesBytes)
	channelID := 0
	if info.ChannelMeta != nil {
		channelID = info.ChannelMeta.ChannelId
	}
	for index := 0; index < imageCount; index++ {
		mjID := taskID
		if imageCount > 1 {
			mjID = fmt.Sprintf("%s-%d", taskID, index+1)
		}
		_ = (&model.Midjourney{
			Code:        1,
			UserId:      info.UserId,
			Action:      "IMAGINE",
			MjId:        mjID,
			Prompt:      prompt,
			Description: ChannelName,
			State:       modelName,
			SubmitTime:  submitTime,
			StartTime:   submitTime,
			FinishTime:  finishTime,
			ImageUrl:    fmt.Sprintf("/pg/images/generations/%s/image/%d", taskID, index),
			Status:      string(model.TaskStatusSuccess),
			Progress:    "100%",
			ChannelId:   channelID,
			Quota:       info.FinalPreConsumedQuota,
			Properties:  properties,
		}).Insert()
	}
}

func splitInlineDataImageURL(dataURL string) (contentType, b64 string, ok bool) {
	dataURL = strings.TrimSpace(dataURL)
	comma := strings.Index(dataURL, ",")
	if !strings.HasPrefix(dataURL, "data:image/") || comma < 0 {
		return "", "", false
	}
	contentType = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(dataURL[:comma], "data:"), ";base64"))
	b64 = strings.NewReplacer("\r", "", "\n", "", " ", "").Replace(strings.TrimSpace(dataURL[comma+1:]))
	if contentType == "" || b64 == "" {
		return "", "", false
	}
	return contentType, b64, true
}

func startChatStream(ctx context.Context, client *Client, req chatRequest, prompt string, captureRequestBody func([]byte), timings ...*service.ChatGPTWebTiming) (*chatStreamStart, error) {
	timing := firstChatGPTWebTiming(timings...)
	cr, err := client.ChatRequirementsV2(ctx, timing)
	if err != nil {
		return nil, err
	}
	proofToken := strings.TrimSpace(cr.ProofToken)
	if cr.Proofofwork.Required {
		if proofToken == "" {
			proofStart := time.Now()
			proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
			if timing != nil {
				timing.ObserveSince("conversation_proof_ms", proofStart)
			}
		} else if timing != nil {
			timing.Set("conversation_proof_reused", true)
		}
	}
	continuationStart := time.Now()
	continuation := prepareConversationContinuation(ctx, client, req.ConversationID)
	if timing != nil {
		timing.ObserveSince("conversation_mapping_ms", continuationStart)
		timing.Set("conversation_continuation", continuation.Available)
	}
	convID := ""
	if continuation.Available {
		convID = continuation.ConvID
	}
	actualPrompt := prompt
	if !continuation.Available && strings.TrimSpace(req.ConversationID) != "" && strings.TrimSpace(req.FallbackPrompt) != "" {
		actualPrompt = strings.TrimSpace(req.FallbackPrompt)
	}
	if timing != nil && strings.TrimSpace(req.ConversationID) != "" && strings.TrimSpace(req.FallbackPrompt) != "" {
		timing.Set("session_route_fallback_full_context", !continuation.Available)
	}
	webModel := chatModelForWeb(req.Model)
	thinkingEffort := chatGPTWebThinkingEffort(webModel, req.ThinkingEffort)
	if timing != nil {
		timing.Set("upstream_web_model", webModel)
		if thinkingEffort != "" {
			timing.Set("upstream_thinking_effort", thinkingEffort)
		}
		timing.Set("deep_research", req.DeepResearch)
	}
	convOpt := ChatConvOpts{
		Prompt:             actualPrompt,
		UpstreamModel:      webModel,
		ThinkingEffort:     thinkingEffort,
		DeepResearch:       req.DeepResearch,
		ConvID:             convID,
		ParentMsgID:        continuation.ParentID,
		MessageID:          uuid.NewString(),
		ChatToken:          cr.Token,
		ProofToken:         proofToken,
		SSETimeout:         300 * time.Second,
		CaptureRequestBody: captureRequestBody,
	}
	prepareStart := time.Now()
	if conduitToken, conduitErr := client.PrepareChatConversation(ctx, convOpt); conduitErr == nil {
		convOpt.ConduitToken = conduitToken
	}
	if timing != nil {
		timing.ObserveSince("prepare_conversation_ms", prepareStart)
	}
	streamOpenStart := time.Now()
	stream, err := client.StreamChatConversation(ctx, convOpt)
	if timing != nil {
		timing.ObserveSince("stream_open_ms", streamOpenStart)
	}
	if err != nil {
		return nil, err
	}
	return &chatStreamStart{Stream: stream, Baseline: continuation.Baseline, Prompt: actualPrompt}, nil
}

func chatModelForWeb(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || common.IsImageGenerationModel(model) {
		return "auto"
	}
	if slug, ok := chatGPTWebModelSlugs[strings.ToLower(model)]; ok {
		return slug
	}
	return model
}

var chatGPTWebModelSlugs = map[string]string{
	"gpt-5.5-thinking": "gpt-5-5-thinking",
	"gpt-5.5-pro":      "gpt-5-5-pro",
	"gpt-5.4-thinking": "gpt-5-4-thinking",
	"gpt-5.4-pro":      "gpt-5-4-pro",
	"gpt-5.4-instant":  "gpt-5-4-instant",
}

func chatReasoningEffort(reasoningEffort string, reasoning json.RawMessage) string {
	if effort := strings.TrimSpace(reasoningEffort); effort != "" {
		return effort
	}
	if len(reasoning) == 0 {
		return ""
	}
	var payload struct {
		Effort string `json:"effort"`
	}
	if err := common.Unmarshal(reasoning, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Effort)
}

func responsesReasoningEffort(reasoning *dto.Reasoning) string {
	if reasoning == nil {
		return ""
	}
	return strings.TrimSpace(reasoning.Effort)
}

func chatGPTWebThinkingEffort(model string, requested string) string {
	if !chatGPTWebModelUsesThinking(model) {
		return ""
	}
	requested = strings.ToLower(strings.TrimSpace(requested))
	switch requested {
	case "", "auto", "none", "minimal", "low", "medium", "standard", "normal", "default":
		return "standard"
	case "high", "xhigh", "x-high", "x_high", "extra_high", "very_high", "extended", "max", "maximum", "ultra":
		return "extended"
	default:
		if strings.Contains(requested, "high") || strings.Contains(requested, "extended") {
			return "extended"
		}
		return "standard"
	}
}

func chatGPTWebModelUsesThinking(model string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(model)), "thinking")
}

func buildStreamingChatResponse(ctx context.Context, client *Client, stream <-chan SSEEvent, req chatRequest, prompt string, baseline imageBaseline, info *relaycommon.RelayInfo, publicBaseURL string, route chatGPTWebSessionRoute, timings ...*service.ChatGPTWebTiming) *http.Response {
	timing := firstChatGPTWebTiming(timings...)
	pr, pw := io.Pipe()
	if isResponsesRelay(info) {
		go streamResponsesCompletion(ctx, client, stream, req, prompt, baseline, info, publicBaseURL, route, pw, timing)
	} else {
		go streamChatCompletion(ctx, client, stream, req, prompt, baseline, info, publicBaseURL, route, pw, timing)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}
}

func streamChatCompletion(ctx context.Context, client *Client, stream <-chan SSEEvent, req chatRequest, prompt string, baseline imageBaseline, info *relaycommon.RelayInfo, publicBaseURL string, route chatGPTWebSessionRoute, pw *io.PipeWriter, timings ...*service.ChatGPTWebTiming) {
	timing := firstChatGPTWebTiming(timings...)
	defer pw.Close()
	streamStart := time.Now()
	firstEventSeen := false
	streamEventCount := 0
	streamDeltaCount := 0
	id := buildTransientChatCompletionID()
	created := time.Now().Unix()
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "auto"
	}
	writeChatStreamChunk(pw, id, created, model, "assistant", "", nil, nil)
	state := &ChatSSEState{}
	streamedContent := ""
	bufferImageResponse := shouldPollChatGeneratedImages(req, prompt, "", false)
	toolDecisionPending := len(req.Tools) > 0
	toolCandidateBuffer := ""
	if timing != nil {
		timing.Set("stream_buffer_image_response", bufferImageResponse)
		timing.Set("stream_tool_bridge_candidate", toolDecisionPending)
	}
	for ev := range stream {
		streamEventCount++
		if !firstEventSeen {
			firstEventSeen = true
			if timing != nil {
				timing.ObserveSince("stream_first_event_ms", streamStart)
			}
		}
		delta, done, collectErr := CollectChatSSEEvent(ev, state)
		if state.ConversationID != "" {
			id = buildChatCompletionID(state.ConversationID)
		}
		if collectErr != nil {
			_ = pw.CloseWithError(collectErr)
			return
		}
		if delta != "" {
			streamDeltaCount++
			if toolDecisionPending {
				toolCandidateBuffer += delta
				if shouldContinueBufferingChatGPTWebToolCandidate(toolCandidateBuffer) {
					if done {
						break
					}
					continue
				}
				delta = toolCandidateBuffer
				toolCandidateBuffer = ""
				toolDecisionPending = false
			}
			if !bufferImageResponse && !state.HasImageGeneration && !chatContentHasChatGPTImageURL(delta) && !chatContentHasChatGPTImageURL(state.Content) && !state.HasInlineImage && !chatContentHasInlineDataImage(delta) && !chatContentHasInlineDataImage(state.Content) {
				streamedContent += delta
				writeChatStreamChunk(pw, id, created, model, "", delta, nil, nil)
			}
		}
		if done {
			break
		}
	}
	if containsImageGenerationUpstreamErrorText(state.Content) {
		_ = pw.CloseWithError(imageGenerationUpstreamError())
		return
	}
	if strings.TrimSpace(state.Content) == "" {
		recovered := ""
		if req.DeepResearch && state.HasDeepResearchInternalEvent {
			recovered = recoverDeepResearchTextFromConversation(ctx, client, state.ConversationID, timing)
		} else if state.HasStreamHandoff {
			recovered = recoverHandoffTextFromConversation(ctx, client, state.ConversationID, timing)
		} else {
			recovered = recoverChatCompletionTextFromConversation(ctx, client, state.ConversationID, timing)
		}
		if recovered != "" {
			state.Content = recovered
		}
	}
	if strings.TrimSpace(state.Content) == "" && req.DeepResearch && state.HasDeepResearchInternalEvent {
		state.Content = chatGPTWebDeepResearchPendingMessage
	}
	finalContent := materializePlaygroundInlineDataImages(state.Content, info, prompt, model)
	finalContent = materializeChatGPTContentImageURLs(ctx, client, finalContent, info, prompt, model, publicBaseURL, timing)
	if toolCall, ok := parseChatGPTWebToolCall(finalContent, req.Tools); ok {
		if timing != nil {
			timing.Set("tool_bridge_call_detected", true)
			timing.Set("tool_bridge_call_name", toolCall.Function.Name)
		}
		usage := buildChatUsage(prompt, toolCall.Function.Name+toolCall.Function.Arguments, model)
		writeChatToolCallChunk(pw, id, created, model, toolCall)
		finish := "tool_calls"
		writeChatStreamChunk(pw, id, created, model, "", "", &finish, &usage)
		writeChatDone(pw)
		recordChatGPTWebSessionRoute(route, state.ConversationID, timing)
		if timing != nil {
			timing.ObserveSince("stream_total_ms", streamStart)
		}
		return
	}
	if timing != nil && len(req.Tools) > 0 {
		timing.Set("tool_bridge_call_detected", false)
	}
	if finalContent != "" && finalContent != streamedContent {
		delta := finalContent
		if strings.HasPrefix(finalContent, streamedContent) {
			delta = finalContent[len(streamedContent):]
		}
		if delta != "" {
			writeChatStreamChunk(pw, id, created, model, "", delta, nil, nil)
			streamedContent = finalContent
		}
	}
	allowImagePoll := !state.HasInlineImage && !chatContentHasInlineDataImage(state.Content) && shouldPollChatGeneratedImages(req, prompt, state.Content, state.HasImageGeneration)
	if timing != nil {
		timing.Set("allow_image_poll", allowImagePoll)
		timing.Set("has_image_generation", state.HasImageGeneration)
		timing.Set("has_inline_image", state.HasInlineImage)
	}
	if imageMarkdown, err := collectChatGeneratedImageMarkdown(ctx, client, state.ConversationID, baseline, allowImagePoll, info, prompt, model, publicBaseURL, timing); err != nil {
		_ = pw.CloseWithError(err)
		return
	} else if imageMarkdown != "" {
		if strings.TrimSpace(state.Content) != "" {
			imageMarkdown = "\n\n" + imageMarkdown
		}
		writeChatStreamChunk(pw, id, created, model, "", imageMarkdown, nil, nil)
	}
	if timing != nil {
		timing.Set("stream_event_count", streamEventCount)
		timing.Set("stream_delta_count", streamDeltaCount)
		timing.Set("stream_handoff", state.HasStreamHandoff)
		timing.Set("stream_state_content_chars", len(state.Content))
		timing.Set("stream_output_text_chars", len(streamedContent))
		timing.Set("stream_empty_output", strings.TrimSpace(streamedContent) == "")
	}
	finish := "stop"
	usage := buildChatUsage(prompt, streamedContent, model)
	writeChatStreamChunk(pw, id, created, model, "", "", &finish, &usage)
	writeChatDone(pw)
	recordChatGPTWebSessionRoute(route, state.ConversationID, timing)
	if timing != nil {
		timing.ObserveSince("stream_total_ms", streamStart)
	}
}

func streamResponsesCompletion(ctx context.Context, client *Client, stream <-chan SSEEvent, req chatRequest, prompt string, baseline imageBaseline, info *relaycommon.RelayInfo, publicBaseURL string, route chatGPTWebSessionRoute, pw *io.PipeWriter, timings ...*service.ChatGPTWebTiming) {
	timing := firstChatGPTWebTiming(timings...)
	defer pw.Close()
	streamStart := time.Now()
	firstEventSeen := false
	streamEventCount := 0
	streamDeltaCount := 0
	responseID := buildTransientResponsesResponseID()
	messageID := buildResponsesMessageID("")
	created := time.Now().Unix()
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "auto"
	}

	outputText := ""
	writeResponsesStreamEvent(pw, "response.created", &dto.ResponsesStreamResponse{
		Type:     "response.created",
		Response: buildResponsesStatusResponse(responseID, created, model, "in_progress", nil),
	})
	writeResponsesStreamEvent(pw, "response.output_item.added", &dto.ResponsesStreamResponse{
		Type:        "response.output_item.added",
		OutputIndex: common.GetPointer(0),
		Item:        buildResponsesMessageItem(messageID, "", "in_progress"),
	})

	state := &ChatSSEState{}
	bufferImageResponse := shouldPollChatGeneratedImagesForRelay(info, req, prompt, "", false)
	toolDecisionPending := len(req.Tools) > 0
	toolCandidateBuffer := ""
	if timing != nil {
		timing.Set("stream_buffer_image_response", bufferImageResponse)
		timing.Set("stream_tool_bridge_candidate", toolDecisionPending)
	}
	for ev := range stream {
		streamEventCount++
		if !firstEventSeen {
			firstEventSeen = true
			if timing != nil {
				timing.ObserveSince("stream_first_event_ms", streamStart)
			}
		}
		delta, done, collectErr := CollectChatSSEEvent(ev, state)
		if state.ConversationID != "" {
			responseID = buildResponsesResponseID(state.ConversationID)
			messageID = buildResponsesMessageID(state.ConversationID)
		}
		if collectErr != nil {
			_ = pw.CloseWithError(collectErr)
			return
		}
		if delta != "" {
			streamDeltaCount++
			if toolDecisionPending {
				toolCandidateBuffer += delta
				if shouldContinueBufferingChatGPTWebToolCandidate(toolCandidateBuffer) {
					if done {
						break
					}
					continue
				}
				delta = toolCandidateBuffer
				toolCandidateBuffer = ""
				toolDecisionPending = false
			}
			if !bufferImageResponse && !state.HasImageGeneration && !chatContentHasChatGPTImageURL(delta) && !chatContentHasChatGPTImageURL(state.Content) && !state.HasInlineImage && !chatContentHasInlineDataImage(delta) && !chatContentHasInlineDataImage(state.Content) {
				outputText += delta
				writeResponsesTextDelta(pw, delta)
			}
		}
		if done {
			break
		}
	}
	if containsImageGenerationUpstreamErrorText(state.Content) {
		_ = pw.CloseWithError(imageGenerationUpstreamError())
		return
	}
	if strings.TrimSpace(state.Content) == "" {
		recovered := ""
		if req.DeepResearch && state.HasDeepResearchInternalEvent {
			recovered = recoverDeepResearchTextFromConversation(ctx, client, state.ConversationID, timing)
		} else if state.HasStreamHandoff {
			recovered = recoverHandoffTextFromConversation(ctx, client, state.ConversationID, timing)
		} else {
			recovered = recoverChatCompletionTextFromConversation(ctx, client, state.ConversationID, timing)
		}
		if recovered != "" {
			state.Content = recovered
		}
	}
	if strings.TrimSpace(state.Content) == "" && req.DeepResearch && state.HasDeepResearchInternalEvent {
		state.Content = chatGPTWebDeepResearchPendingMessage
	}
	finalContent := materializePlaygroundInlineDataImages(state.Content, info, prompt, model)
	finalContent = materializeChatGPTContentImageURLs(ctx, client, finalContent, info, prompt, model, publicBaseURL, timing)
	if toolCall, ok := parseChatGPTWebToolCall(finalContent, req.Tools); ok {
		if timing != nil {
			timing.Set("tool_bridge_call_detected", true)
			timing.Set("tool_bridge_call_name", toolCall.Function.Name)
		}
		usage := buildChatUsage(prompt, toolCall.Function.Name+toolCall.Function.Arguments, model)
		writeResponsesToolCall(pw, responseID, created, model, toolCall, usage)
		writeChatDone(pw)
		recordChatGPTWebSessionRoute(route, state.ConversationID, timing)
		if timing != nil {
			timing.ObserveSince("stream_total_ms", streamStart)
		}
		return
	}
	if timing != nil && len(req.Tools) > 0 {
		timing.Set("tool_bridge_call_detected", false)
	}
	if finalContent != "" && finalContent != outputText {
		delta := finalContent
		if strings.HasPrefix(finalContent, outputText) {
			delta = finalContent[len(outputText):]
		}
		if delta != "" {
			outputText += delta
			writeResponsesTextDelta(pw, delta)
		}
	}
	allowImagePoll := !state.HasInlineImage && !chatContentHasInlineDataImage(state.Content) && shouldPollChatGeneratedImagesForRelay(info, req, prompt, state.Content, state.HasImageGeneration)
	if timing != nil {
		timing.Set("allow_image_poll", allowImagePoll)
		timing.Set("has_image_generation", state.HasImageGeneration)
		timing.Set("has_inline_image", state.HasInlineImage)
	}
	if imageMarkdown, err := collectChatGeneratedImageMarkdown(ctx, client, state.ConversationID, baseline, allowImagePoll, info, prompt, model, publicBaseURL, timing); err != nil {
		_ = pw.CloseWithError(err)
		return
	} else if imageMarkdown != "" {
		if strings.TrimSpace(outputText) != "" {
			imageMarkdown = "\n\n" + imageMarkdown
		}
		outputText += imageMarkdown
		writeResponsesTextDelta(pw, imageMarkdown)
	}
	if timing != nil {
		timing.Set("stream_event_count", streamEventCount)
		timing.Set("stream_delta_count", streamDeltaCount)
		timing.Set("stream_handoff", state.HasStreamHandoff)
		timing.Set("stream_state_content_chars", len(state.Content))
		timing.Set("stream_output_text_chars", len(outputText))
		timing.Set("stream_empty_output", strings.TrimSpace(outputText) == "")
	}

	usage := buildChatUsage(prompt, outputText, model)
	writeResponsesStreamEvent(pw, "response.output_text.done", &dto.ResponsesStreamResponse{
		Type:         "response.output_text.done",
		OutputIndex:  common.GetPointer(0),
		ContentIndex: common.GetPointer(0),
		ItemID:       messageID,
	})
	writeResponsesStreamEvent(pw, dto.ResponsesOutputTypeItemDone, &dto.ResponsesStreamResponse{
		Type:        dto.ResponsesOutputTypeItemDone,
		OutputIndex: common.GetPointer(0),
		Item:        buildResponsesMessageItem(messageID, outputText, "completed"),
	})
	writeResponsesStreamEvent(pw, "response.completed", &dto.ResponsesStreamResponse{
		Type:     "response.completed",
		Response: buildResponsesResponse(responseID, messageID, created, model, outputText, usage),
	})
	writeChatDone(pw)
	recordChatGPTWebSessionRoute(route, state.ConversationID, timing)
	if timing != nil {
		timing.ObserveSince("stream_total_ms", streamStart)
	}
}

func writeChatStreamChunk(w io.Writer, id string, created int64, model, role, content string, finishReason *string, usage *dto.Usage) {
	chunk := dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index:        0,
			FinishReason: finishReason,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
				Role: role,
			},
		}},
		Usage: usage,
	}
	if content != "" {
		chunk.Choices[0].Delta.SetContentString(content)
	}
	data, _ := common.Marshal(chunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func writeChatToolCallChunk(w io.Writer, id string, created int64, model string, toolCall dto.ToolCallResponse) {
	toolCall.SetIndex(0)
	chunk := dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
				ToolCalls: []dto.ToolCallResponse{toolCall},
			},
		}},
	}
	data, _ := common.Marshal(chunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func writeResponsesTextDelta(w io.Writer, delta string) {
	if delta == "" {
		return
	}
	writeResponsesStreamEvent(w, "response.output_text.delta", &dto.ResponsesStreamResponse{
		Type:         "response.output_text.delta",
		Delta:        delta,
		OutputIndex:  common.GetPointer(0),
		ContentIndex: common.GetPointer(0),
	})
}

func writeResponsesToolCall(w io.Writer, responseID string, created int64, model string, toolCall dto.ToolCallResponse, usage dto.Usage) {
	itemID := buildResponsesFunctionCallItemID(responseID, 0)
	item := buildResponsesFunctionCallItem(itemID, toolCall, "in_progress")
	item.Arguments = nil
	writeResponsesStreamEvent(w, dto.ResponsesOutputTypeItemAdded, &dto.ResponsesStreamResponse{
		Type:        dto.ResponsesOutputTypeItemAdded,
		OutputIndex: common.GetPointer(0),
		Item:        item,
	})
	writeResponsesStreamEvent(w, "response.function_call_arguments.delta", &dto.ResponsesStreamResponse{
		Type:        "response.function_call_arguments.delta",
		Delta:       toolCall.Function.Arguments,
		OutputIndex: common.GetPointer(0),
		ItemID:      itemID,
	})
	writeResponsesStreamEvent(w, "response.function_call_arguments.done", &dto.ResponsesStreamResponse{
		Type:        "response.function_call_arguments.done",
		OutputIndex: common.GetPointer(0),
		ItemID:      itemID,
	})
	writeResponsesStreamEvent(w, dto.ResponsesOutputTypeItemDone, &dto.ResponsesStreamResponse{
		Type:        dto.ResponsesOutputTypeItemDone,
		OutputIndex: common.GetPointer(0),
		Item:        buildResponsesFunctionCallItem(itemID, toolCall, "completed"),
	})
	writeResponsesStreamEvent(w, "response.completed", &dto.ResponsesStreamResponse{
		Type:     "response.completed",
		Response: buildResponsesToolCallResponse(responseID, itemID, created, model, toolCall, usage),
	})
}

func writeResponsesStreamEvent(w io.Writer, eventName string, event *dto.ResponsesStreamResponse) {
	if event == nil {
		return
	}
	if event.Type == "" {
		event.Type = eventName
	}
	data, _ := common.Marshal(event)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, data)
}

func writeChatDone(w io.Writer) {
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func buildChatUsage(prompt, content, model string) dto.Usage {
	promptTokens := service.CountTextToken(prompt, model)
	completionTokens := service.CountTextToken(content, model)
	if promptTokens == 0 && strings.TrimSpace(prompt) != "" {
		promptTokens = 1
	}
	return dto.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
}

func buildChatToolCallResponse(id string, created int64, model string, toolCall dto.ToolCallResponse, usage dto.Usage) chatResponse {
	message := dto.Message{Role: "assistant"}
	message.SetNullContent()
	message.SetToolCalls([]dto.ToolCallRequest{chatToolCallResponseToRequest(toolCall)})
	return chatResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []dto.OpenAITextResponseChoice{{
			Index:        0,
			Message:      message,
			FinishReason: "tool_calls",
		}},
		Usage: usage,
	}
}

func chatToolCallResponseToRequest(toolCall dto.ToolCallResponse) dto.ToolCallRequest {
	return dto.ToolCallRequest{
		ID:   toolCall.ID,
		Type: "function",
		Function: dto.FunctionRequest{
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		},
	}
}

func isResponsesRelay(info *relaycommon.RelayInfo) bool {
	return info != nil && info.RelayMode == relayconstant.RelayModeResponses
}

func buildResponsesToolCallResponse(id, itemID string, created int64, model string, toolCall dto.ToolCallResponse, usage dto.Usage) *dto.OpenAIResponsesResponse {
	resp := buildResponsesStatusResponse(id, created, model, "completed", buildResponsesFunctionCallItem(itemID, toolCall, "completed"))
	resp.Usage = responsesUsageFromChatUsage(usage)
	return resp
}

func buildResponsesResponse(id, messageID string, created int64, model, content string, usage dto.Usage) *dto.OpenAIResponsesResponse {
	resp := buildResponsesStatusResponse(id, created, model, "completed", &dto.ResponsesOutput{
		Type:    "message",
		ID:      messageID,
		Status:  "completed",
		Role:    "assistant",
		Content: []dto.ResponsesOutputContent{buildResponsesOutputText(content)},
		Quality: "",
		Size:    "",
	})
	resp.Usage = responsesUsageFromChatUsage(usage)
	return resp
}

func buildResponsesStatusResponse(id string, created int64, model string, status string, output *dto.ResponsesOutput) *dto.OpenAIResponsesResponse {
	resp := &dto.OpenAIResponsesResponse{
		ID:                 id,
		Object:             "response",
		CreatedAt:          int(created),
		Status:             rawJSONString(status),
		Error:              nil,
		IncompleteDetails:  nil,
		Instructions:       rawJSONNull(),
		Model:              model,
		Output:             []dto.ResponsesOutput{},
		ParallelToolCalls:  false,
		PreviousResponseID: rawJSONNull(),
		Reasoning:          nil,
		Store:              false,
		ToolChoice:         rawJSONString("auto"),
		Tools:              []map[string]any{},
		Truncation:         rawJSONString("disabled"),
		Usage:              nil,
		User:               rawJSONNull(),
		Metadata:           rawJSONNull(),
	}
	if output != nil {
		resp.Output = []dto.ResponsesOutput{*output}
	}
	return resp
}

func responsesUsageFromChatUsage(usage dto.Usage) *dto.Usage {
	out := usage
	if out.InputTokens == 0 {
		out.InputTokens = out.PromptTokens
	}
	if out.OutputTokens == 0 {
		out.OutputTokens = out.CompletionTokens
	}
	if out.PromptTokens == 0 {
		out.PromptTokens = out.InputTokens
	}
	if out.CompletionTokens == 0 {
		out.CompletionTokens = out.OutputTokens
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	if out.InputTokensDetails == nil {
		out.InputTokensDetails = &dto.InputTokenDetails{
			CachedTokens: out.PromptTokensDetails.CachedTokens,
			TextTokens:   out.PromptTokens,
			ImageTokens:  out.PromptTokensDetails.ImageTokens,
			AudioTokens:  out.PromptTokensDetails.AudioTokens,
		}
	}
	return &out
}

func buildResponsesFunctionCallItemID(conversationID string, index int) string {
	base := normalizeChatGPTWebConversationID(conversationID)
	if base == "" {
		base = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return fmt.Sprintf("fc_chatgptimg-%s-%d", base, index)
}

func buildResponsesFunctionCallItem(itemID string, toolCall dto.ToolCallResponse, status string) *dto.ResponsesOutput {
	if strings.TrimSpace(itemID) == "" {
		itemID = buildResponsesFunctionCallItemID("", 0)
	}
	callID := strings.TrimSpace(toolCall.ID)
	if callID == "" {
		callID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	arguments := json.RawMessage(strings.TrimSpace(toolCall.Function.Arguments))
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	return &dto.ResponsesOutput{
		Type:      "function_call",
		ID:        itemID,
		Status:    status,
		CallId:    callID,
		Name:      toolCall.Function.Name,
		Arguments: arguments,
	}
}

func buildResponsesMessageItem(messageID string, content string, status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type:    "message",
		ID:      messageID,
		Status:  status,
		Role:    "assistant",
		Content: []dto.ResponsesOutputContent{buildResponsesOutputText(content)},
	}
}

func buildResponsesOutputText(content string) dto.ResponsesOutputContent {
	return dto.ResponsesOutputContent{
		Type:        "output_text",
		Text:        content,
		Annotations: []interface{}{},
	}
}

func rawJSONString(value string) json.RawMessage {
	data, err := common.Marshal(value)
	if err != nil {
		return rawJSONNull()
	}
	return data
}

func rawJSONNull() json.RawMessage {
	return json.RawMessage("null")
}

func buildTransientChatCompletionID() string {
	return "chatcmpl-" + uuid.NewString()
}

func buildChatCompletionID(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		conversationID = uuid.NewString()
	}
	return "chatcmpl-chatgptimg-" + conversationID
}

func buildTransientResponsesResponseID() string {
	return "resp_chatgptimg-" + uuid.NewString()
}

func buildResponsesResponseID(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		conversationID = uuid.NewString()
	}
	return "resp_chatgptimg-" + conversationID
}

func buildResponsesMessageID(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		conversationID = uuid.NewString()
	}
	return "msg_chatgptimg-" + conversationID
}

type conversationContinuation struct {
	ConvID    string
	ParentID  string
	Baseline  imageBaseline
	Available bool
}

type imageBaseline struct {
	ToolIDs     map[string]struct{}
	FileIDs     map[string]struct{}
	SedimentIDs map[string]struct{}
}

func prepareConversationContinuation(ctx context.Context, client *Client, conversationID string) conversationContinuation {
	conversationID = strings.TrimSpace(conversationID)
	if client == nil || conversationID == "" {
		return conversationContinuation{ParentID: uuid.NewString()}
	}
	mapping, err := client.GetConversationMapping(ctx, conversationID)
	if err != nil {
		return conversationContinuation{ParentID: uuid.NewString()}
	}
	parentID, _ := mapping["current_node"].(string)
	if strings.TrimSpace(parentID) == "" {
		return conversationContinuation{ParentID: uuid.NewString()}
	}
	rawMapping, _ := mapping["mapping"].(map[string]any)
	return conversationContinuation{
		ConvID:    conversationID,
		ParentID:  parentID,
		Baseline:  buildImageBaseline(rawMapping),
		Available: true,
	}
}

func recordGenerationDrawingLog(info *relaycommon.RelayInfo, req generationRequest, run *imageRunResult, resp *generationResponse) {
	if info == nil || info.IsChannelTest || resp == nil || len(resp.Data) == 0 {
		return
	}

	submitTime := time.Now().UnixMilli()
	if !info.StartTime.IsZero() {
		submitTime = info.StartTime.UnixMilli()
	}
	finishTime := time.Now().UnixMilli()
	taskIDPrefix := strings.TrimSpace(run.ConversationID)
	if taskIDPrefix == "" {
		taskIDPrefix = "chatgptimg-" + uuid.NewString()
	}

	propertiesBytes, _ := common.Marshal(map[string]any{
		"source":          ChannelName,
		"model":           strings.TrimSpace(req.Model),
		"conversation_id": strings.TrimSpace(run.ConversationID),
		"preview":         run.IsPreview,
	})
	properties := string(propertiesBytes)

	for index, item := range resp.Data {
		imageURL := getGenerationLogImageURL(item)
		if imageURL == "" {
			continue
		}

		taskID := taskIDPrefix
		if len(resp.Data) > 1 {
			taskID = fmt.Sprintf("%s-%d", taskIDPrefix, index+1)
		}

		_ = (&model.Midjourney{
			Code:        1,
			UserId:      info.UserId,
			Action:      "IMAGINE",
			MjId:        taskID,
			Prompt:      req.Prompt,
			Description: ChannelName,
			State:       strings.TrimSpace(req.Model),
			SubmitTime:  submitTime,
			StartTime:   submitTime,
			FinishTime:  finishTime,
			ImageUrl:    imageURL,
			Status:      string(model.TaskStatusSuccess),
			Progress:    "100%",
			ChannelId:   info.ChannelId,
			Quota:       info.FinalPreConsumedQuota,
			Properties:  properties,
		}).Insert()
	}
}

func getGenerationLogImageURL(item dto.ImageData) string {
	if url := strings.TrimSpace(item.Url); url != "" {
		return url
	}
	if b64 := strings.TrimSpace(item.B64Json); b64 != "" {
		if strings.HasPrefix(b64, "data:") {
			return b64
		}
		return "data:image/png;base64," + b64
	}
	return ""
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if resp == nil {
		return nil, types.NewError(errors.New("chatgpt web channel: nil response"), types.ErrorCodeBadResponse)
	}
	if info != nil && info.IsStream && (info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits) {
		return streamImageResponse(c, resp)
	}
	if info != nil && (info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits) {
		return openai.OpenaiHandlerWithUsage(c, info, resp)
	}
	if isResponsesRelay(info) {
		if info.IsStream {
			return openai.OaiResponsesStreamHandler(c, info, resp)
		}
		return openai.OaiResponsesHandler(c, info, resp)
	}
	if info != nil && info.IsStream {
		return openai.OaiStreamHandler(c, info, resp)
	}
	return openai.OpenaiHandler(c, info, resp)
}

func buildImageStreamPayload(payloadBytes []byte) []byte {
	payloadBytes = bytes.TrimSpace(payloadBytes)
	if len(payloadBytes) == 0 {
		return []byte("data: [DONE]\n\n")
	}
	var b strings.Builder
	b.Grow(len(payloadBytes) + 16)
	b.WriteString("data: ")
	b.Write(payloadBytes)
	b.WriteString("\n\n")
	b.WriteString("data: [DONE]\n\n")
	return []byte(b.String())
}

func streamImageResponse(c *gin.Context, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var payload generationResponse
	if err := decodeImageStreamPayload(responseBody, &payload); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if len(payload.Data) > 0 {
		common.SetContextKey(c, constant.ContextKeyImageGenerationResponse, &dto.ImageResponse{
			Created: payload.Created,
			Data:    payload.Data,
		})
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return &payload.Usage, nil
}

func decodeImageStreamPayload(body []byte, payload *generationResponse) error {
	if payload == nil {
		return errors.New("chatgpt web channel: nil image payload target")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return errors.New("chatgpt web channel: empty image stream body")
	}
	parts := bytes.Split(trimmed, []byte("\n\n"))
	for _, part := range parts {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		if bytes.Equal(part, []byte("data: [DONE]")) {
			continue
		}
		if !bytes.HasPrefix(part, []byte("data: ")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(part, []byte("data: ")))
		if len(data) == 0 {
			continue
		}
		if err := common.Unmarshal(data, payload); err != nil {
			return fmt.Errorf("chatgpt web channel: decode image stream payload failed: %w", err)
		}
		return nil
	}
	return errors.New("chatgpt web channel: image stream payload not found")
}

func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func chooseBaseURL(info *relaycommon.RelayInfo) string {
	if info != nil && strings.TrimSpace(info.ChannelBaseUrl) != "" {
		return strings.TrimSpace(info.ChannelBaseUrl)
	}
	return defaultBaseURL
}

func extractReferenceImagesFromRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) ([]string, error) {
	refs := make([]string, 0)
	if raw, ok := request.Extra["reference_images"]; ok && len(raw) > 0 {
		parsed, err := parseStringOrStringArray(raw)
		if err != nil {
			return nil, fmt.Errorf("chatgpt web channel: invalid reference_images: %w", err)
		}
		refs = append(refs, parsed...)
	}
	if len(request.Image) > 0 {
		parsed, err := parseStringOrStringArray(request.Image)
		if err == nil {
			refs = append(refs, parsed...)
		}
	}
	if info != nil && info.RelayMode == relayconstant.RelayModeImagesEdits {
		multipartRefs, err := extractMultipartReferenceImages(c)
		if err != nil {
			return nil, err
		}
		refs = append(refs, multipartRefs...)
	}
	return dedupeStrings(refs), nil
}

func parseStringOrStringArray(raw []byte) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var single string
	if err := common.Unmarshal(raw, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			return nil, nil
		}
		return []string{strings.TrimSpace(single)}, nil
	}
	var arr []string
	if err := common.Unmarshal(raw, &arr); err == nil {
		cleaned := make([]string, 0, len(arr))
		for _, item := range arr {
			item = strings.TrimSpace(item)
			if item != "" {
				cleaned = append(cleaned, item)
			}
		}
		return cleaned, nil
	}
	return nil, errors.New("must be a string or string array")
}

func extractMultipartReferenceImages(c *gin.Context) ([]string, error) {
	if c == nil || c.Request == nil {
		return nil, nil
	}
	if c.Request.MultipartForm == nil {
		if _, err := c.MultipartForm(); err != nil && !errors.Is(err, http.ErrNotMultipart) {
			return nil, fmt.Errorf("chatgpt web channel: parse multipart form failed: %w", err)
		}
	}
	if c.Request.MultipartForm == nil {
		return nil, nil
	}
	fileHeaders := make([]*multipart.FileHeader, 0)
	if images, ok := c.Request.MultipartForm.File["image"]; ok {
		fileHeaders = append(fileHeaders, images...)
	}
	if images, ok := c.Request.MultipartForm.File["image[]"]; ok {
		fileHeaders = append(fileHeaders, images...)
	}
	for fieldName, files := range c.Request.MultipartForm.File {
		if strings.HasPrefix(fieldName, "image[") {
			fileHeaders = append(fileHeaders, files...)
		}
	}
	refs := make([]string, 0, len(fileHeaders))
	for _, fileHeader := range fileHeaders {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("chatgpt web channel: open multipart image failed: %w", err)
		}
		data, readErr := io.ReadAll(file)
		_ = file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("chatgpt web channel: read multipart image failed: %w", readErr)
		}
		mimeType := http.DetectContentType(data)
		refs = append(refs, fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)))
	}
	return refs, nil
}

func uploadReferenceImages(ctx context.Context, client *Client, refs []string) ([]*UploadedFile, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	uploaded := make([]*UploadedFile, 0, len(refs))
	for idx, ref := range refs {
		data, fileName, err := decodeReferenceInput(ref, idx)
		if err != nil {
			return nil, err
		}
		up, err := client.UploadFile(ctx, data, fileName)
		if err != nil {
			return nil, err
		}
		uploaded = append(uploaded, up)
	}
	return uploaded, nil
}

func decodeReferenceInput(ref string, index int) ([]byte, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", errors.New("empty reference image")
	}
	if data, fileName, ok, err := decodePlaygroundImageReference(ref, index); ok {
		return data, fileName, err
	}
	if strings.HasPrefix(ref, "data:") {
		comma := strings.Index(ref, ",")
		if comma < 0 {
			return nil, "", errors.New("invalid data url")
		}
		meta := ref[:comma]
		payload := ref[comma+1:]
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, "", fmt.Errorf("decode data url failed: %w", err)
		}
		ext := guessExtensionFromDataURLMeta(meta)
		return decoded, fmt.Sprintf("reference-%d%s", index+1, ext), nil
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		resp, err := http.Get(ref) //nolint:gosec // user provided image url fetch for image-edit compatibility
		if err != nil {
			return nil, "", fmt.Errorf("download reference image failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, "", fmt.Errorf("download reference image failed: http %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024+1))
		if err != nil {
			return nil, "", fmt.Errorf("read reference image failed: %w", err)
		}
		if len(data) > 20*1024*1024 {
			return nil, "", errors.New("reference image exceeds 20MB")
		}
		ext := filepath.Ext(ref)
		if ext == "" {
			ext = ".png"
		}
		return data, fmt.Sprintf("reference-%d%s", index+1, ext), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(ref)
	if err != nil {
		return nil, "", fmt.Errorf("decode reference image base64 failed: %w", err)
	}
	return decoded, fmt.Sprintf("reference-%d.png", index+1), nil
}

func decodePlaygroundImageReference(ref string, index int) ([]byte, string, bool, error) {
	taskID, imageIndex, ok := parsePlaygroundImageReference(ref)
	if !ok {
		return nil, "", false, nil
	}

	task, exist, err := model.GetByOnlyTaskId(taskID)
	if err != nil {
		return nil, "", true, fmt.Errorf("load playground image reference failed: %w", err)
	}
	if !exist || task == nil || len(task.Data) == 0 {
		return nil, "", true, fmt.Errorf("playground image reference not found: %s", taskID)
	}

	var payload struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return nil, "", true, fmt.Errorf("parse playground image reference failed: %w", err)
	}
	if imageIndex < 0 || imageIndex >= len(payload.Data) {
		return nil, "", true, fmt.Errorf("playground image reference index out of range: %d", imageIndex)
	}

	item := payload.Data[imageIndex]
	if strings.TrimSpace(item.B64JSON) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(item.B64JSON))
		if err != nil {
			return nil, "", true, fmt.Errorf("decode playground image b64_json failed: %w", err)
		}
		return decoded, fmt.Sprintf("reference-%d.png", index+1), true, nil
	}
	if imageURL := strings.TrimSpace(item.URL); imageURL != "" {
		data, fileName, err := decodeReferenceInput(imageURL, index)
		return data, fileName, true, err
	}

	return nil, "", true, errors.New("playground image reference has no image data")
}

func parsePlaygroundImageReference(ref string) (string, int, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", 0, false
	}
	path := ref
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		parsed, err := url.Parse(ref)
		if err != nil {
			return "", 0, false
		}
		path = parsed.Path
	}

	var payloadPath string
	for _, prefix := range []string{"/pg/images/generations/", "/pg/public/images/generations/"} {
		if strings.HasPrefix(path, prefix) {
			payloadPath = strings.TrimPrefix(path, prefix)
			break
		}
	}
	if payloadPath == "" {
		return "", 0, false
	}
	parts := strings.Split(payloadPath, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "image" {
		return "", 0, false
	}
	imageIndex, err := strconv.Atoi(parts[2])
	if err != nil || imageIndex < 0 {
		return "", 0, false
	}
	return parts[0], imageIndex, true
}

func guessExtensionFromDataURLMeta(meta string) string {
	switch {
	case strings.Contains(meta, "image/png"):
		return ".png"
	case strings.Contains(meta, "image/jpeg"):
		return ".jpg"
	case strings.Contains(meta, "image/gif"):
		return ".gif"
	case strings.Contains(meta, "image/webp"):
		return ".webp"
	default:
		return ".png"
	}
}

func resolveImageDownloadURLs(ctx context.Context, client *Client, convID string, fileRefs []string, testMode bool, timing *service.ChatGPTWebTiming) []string {
	return resolveImageDownloadURLsWithWait(ctx, client, convID, fileRefs, chatGPTWebImageDownloadURLMaxWait(testMode), 2*time.Second, timing)
}

func resolveImageDownloadURLsWithWait(ctx context.Context, client *Client, convID string, fileRefs []string, maxWait, interval time.Duration, timing *service.ChatGPTWebTiming) []string {
	if ctx == nil {
		ctx = context.Background()
	}
	refs := dedupeStrings(fileRefs)
	if client == nil || len(refs) == 0 {
		return nil
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	deadline := time.Now().Add(maxWait)
	pending := append([]string(nil), refs...)
	signedURLs := make([]string, 0, len(refs))
	seenURLs := map[string]struct{}{}
	attempts := 0
	errorCount := 0
	lastErrorLabel := ""

	for {
		nextPending := pending[:0]
		for _, ref := range pending {
			if ctx.Err() != nil {
				lastErrorLabel = imageDownloadURLErrorLabel(ctx.Err())
				pending = nextPending
				break
			}
			attempts++
			downloadURLStart := time.Now()
			signedURL, err := client.ImageDownloadURL(ctx, convID, ref)
			if timing != nil {
				timing.ObserveSince("image_download_url_ms", downloadURLStart)
			}
			signedURL = strings.TrimSpace(signedURL)
			if err != nil || signedURL == "" {
				errorCount++
				lastErrorLabel = imageDownloadURLErrorLabel(err)
				if signedURL == "" && err == nil {
					lastErrorLabel = "empty_download_url"
				}
				nextPending = append(nextPending, ref)
				continue
			}
			if _, ok := seenURLs[signedURL]; ok {
				continue
			}
			seenURLs[signedURL] = struct{}{}
			signedURLs = append(signedURLs, signedURL)
		}
		pending = nextPending
		if len(signedURLs) > 0 || len(pending) == 0 || maxWait <= 0 || !time.Now().Before(deadline) || ctx.Err() != nil {
			break
		}
		sleepFor := interval
		if remaining := time.Until(deadline); remaining > 0 && remaining < sleepFor {
			sleepFor = remaining
		}
		sleepContext(ctx, sleepFor)
	}

	if timing != nil {
		timing.Set("image_download_url_attempts", attempts)
		timing.Set("image_download_url_error_count", errorCount)
		timing.Set("image_download_url_pending_count", len(pending))
		if lastErrorLabel != "" {
			timing.Set("image_download_url_last_error", lastErrorLabel)
		}
	}
	return signedURLs
}

func imageDownloadURLErrorLabel(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline_exceeded"
	}
	var upstreamErr *UpstreamError
	if errors.As(err, &upstreamErr) && upstreamErr != nil {
		return fmt.Sprintf("upstream_status_%d", upstreamErr.Status)
	}
	return "request_failed"
}

func runImageGeneration(ctx context.Context, client *Client, req generationRequest, refs []*UploadedFile, testMode bool, captureRequestBody func([]byte), timings ...*service.ChatGPTWebTiming) (*imageRunResult, error) {
	timing := firstChatGPTWebTiming(timings...)
	runStart := time.Now()
	defer func() {
		if timing != nil {
			timing.ObserveSince("image_run_total_ms", runStart)
		}
	}()
	imageRunTimeout := chatGPTWebImageRunTimeout()
	ctx, cancel := context.WithTimeout(ctx, imageRunTimeout)
	defer cancel()

	result := &imageRunResult{}
	maxAttempts := 1
	pollMaxWait := chatGPTWebImagePollMaxWait(testMode)
	sameConvMax := 1
	if pollMaxWait >= imageRunTimeout {
		pollMaxWait = imageRunTimeout - time.Minute
		if pollMaxWait < time.Minute {
			pollMaxWait = imageRunTimeout
		}
	}
	if timing != nil {
		timing.Set("image_run_timeout_seconds", int(imageRunTimeout/time.Second))
		timing.Set("image_poll_max_wait_seconds", int(pollMaxWait/time.Second))
	}

	cr, err := client.ChatRequirementsV2(ctx, timing)
	if err != nil {
		return nil, err
	}
	proofToken := strings.TrimSpace(cr.ProofToken)
	if cr.Proofofwork.Required {
		if proofToken == "" {
			proofStart := time.Now()
			proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
			if timing != nil {
				timing.ObserveSince("conversation_proof_ms", proofStart)
			}
		} else if timing != nil {
			timing.Set("conversation_proof_reused", true)
		}
	}
	var convID string
	parentID := uuid.NewString()
	messageID := uuid.NewString()
	var baseline imageBaseline
	var lastPreviewFids []string
	var lastPreviewSids []string
	var fileRefs []string
	var fallbackRefs []*UploadedFile
	var fallbackRefsLoaded bool

attemptLoop:
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if timing != nil {
			timing.Set("image_attempt", attempt)
		}
		if attempt > 1 {
			cr, err = client.ChatRequirementsV2(ctx, timing)
			if err != nil {
				return nil, err
			}
			proofToken = strings.TrimSpace(cr.ProofToken)
			if cr.Proofofwork.Required {
				if proofToken == "" {
					proofStart := time.Now()
					proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
					if timing != nil {
						timing.ObserveSince("conversation_proof_ms", proofStart)
					}
				} else if timing != nil {
					timing.Set("conversation_proof_reused", true)
				}
			}
		}
		continuationStart := time.Now()
		continuation := prepareConversationContinuation(ctx, client, req.ConversationID)
		if timing != nil {
			timing.ObserveSince("conversation_mapping_ms", continuationStart)
			timing.Set("conversation_continuation", continuation.Available)
		}
		convID = ""
		if continuation.Available {
			convID = continuation.ConvID
		}
		parentID = continuation.ParentID
		baseline = continuation.Baseline
		messageID = uuid.NewString()
		lastPreviewFids = nil
		lastPreviewSids = nil
		fileRefs = nil
		result.IsPreview = false
		prompt := req.Prompt
		activeRefs := refs
		if !continuation.Available && strings.TrimSpace(req.ConversationID) != "" {
			if fallbackPrompt := strings.TrimSpace(req.FallbackPrompt); fallbackPrompt != "" {
				prompt = fallbackPrompt
			}
			if len(req.FallbackReferenceImages) > 0 {
				if !fallbackRefsLoaded {
					fallbackUploadStart := time.Now()
					fallbackRefs, err = uploadReferenceImages(ctx, client, req.FallbackReferenceImages)
					if timing != nil {
						timing.ObserveSince("fallback_reference_upload_ms", fallbackUploadStart)
						timing.Set("fallback_reference_count", len(fallbackRefs))
					}
					if err != nil {
						return nil, err
					}
					fallbackRefsLoaded = true
				}
				if len(fallbackRefs) > 0 {
					activeRefs = append(append([]*UploadedFile{}, fallbackRefs...), refs...)
				}
			}
		}

		for turn := 1; turn <= sameConvMax; turn++ {
			result.TurnsInConv = turn
			if timing != nil {
				timing.Set("image_turn", turn)
			}
			if turn > 1 {
				cr, err = client.ChatRequirementsV2(ctx, timing)
				if err != nil {
					return nil, err
				}
				proofToken = strings.TrimSpace(cr.ProofToken)
				if cr.Proofofwork.Required {
					if proofToken == "" {
						proofStart := time.Now()
						proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
						if timing != nil {
							timing.ObserveSince("conversation_proof_ms", proofStart)
						}
					} else if timing != nil {
						timing.Set("conversation_proof_reused", true)
					}
				}
			}
			convOpt := ImageConvOpts{
				Prompt:             prompt,
				UpstreamModel:      chatGPTWebImageConversationModel(req),
				ConvID:             convID,
				ParentMsgID:        parentID,
				MessageID:          messageID,
				ChatToken:          cr.Token,
				ProofToken:         proofToken,
				References:         activeRefs,
				CaptureRequestBody: captureRequestBody,
			}
			if turn > 1 {
				convOpt.MessageID = uuid.NewString()
			}
			prepareStart := time.Now()
			if conduitToken, conduitErr := client.PrepareFConversation(ctx, convOpt); conduitErr == nil {
				convOpt.ConduitToken = conduitToken
			}
			if timing != nil {
				timing.ObserveSince("prepare_conversation_ms", prepareStart)
			}
			streamCtx, cancelStream := context.WithCancel(ctx)
			streamOpenStart := time.Now()
			stream, err := client.StreamFConversation(streamCtx, convOpt)
			if timing != nil {
				timing.ObserveSince("stream_open_ms", streamOpenStart)
			}
			if err != nil {
				cancelStream()
				if ue, ok := err.(*UpstreamError); ok && ue.IsRateLimited() && attempt < maxAttempts {
					break
				}
				return nil, err
			}
			var sseResult ImageSSEResult
			parseStart := time.Now()
			if testMode {
				sseResult = ParseImageSSEUntilConversationReady(stream, 3*time.Second)
			} else {
				sseResult = ParseImageSSE(stream)
			}
			if timing != nil {
				timing.ObserveSince("image_sse_parse_ms", parseStart)
			}
			cancelStream()
			if sseResult.Err != nil {
				return nil, sseResult.Err
			}
			if sseResult.ConversationID != "" {
				convID = sseResult.ConversationID
				result.ConversationID = convID
			}
			if testMode && convID != "" {
				return result, nil
			}
			excludedFileIDs := uploadedFileIDSet(activeRefs)
			sseResult.FileIDs = filterExcludedFileIDs(sseResult.FileIDs, excludedFileIDs)
			textOnlyErr := imageSSETextWithoutImageError(sseResult)
			if textOnlyErr != nil {
				if !shouldPollTextOnlyImageSSE(sseResult) {
					return nil, noRelayRetry(textOnlyErr, http.StatusUnprocessableEntity)
				}
				if timing != nil {
					timing.Set("image_text_only_poll_fallback", true)
				}
			}
			if len(sseResult.FileIDs) > 0 || len(sseResult.SedimentIDs) > 0 {
				fileRefs = append(fileRefs, sseResult.FileIDs...)
				for _, sid := range sseResult.SedimentIDs {
					fileRefs = append(fileRefs, "sed:"+sid)
				}
				if len(sseResult.FileIDs) == 0 {
					result.IsPreview = true
				}
				break
			}
			if convID == "" {
				return nil, errors.New("chatgpt web channel: missing conversation id from SSE")
			}
			pollStart := time.Now()
			pollStatus, fids, sids := client.PollConversationForImages(ctx, convID, PollOpts{
				MaxWait:             pollMaxWait,
				Interval:            2 * time.Second,
				StableRounds:        2,
				PreviewWait:         8 * time.Second,
				BaselineToolIDs:     baseline.ToolIDs,
				BaselineFileIDs:     baseline.FileIDs,
				BaselineSedimentIDs: baseline.SedimentIDs,
				ExcludedFileIDs:     excludedFileIDs,
			})
			if timing != nil {
				timing.ObserveSince("image_poll_ms", pollStart)
				timing.Set("image_poll_status", string(pollStatus))
			}
			switch pollStatus {
			case PollStatusIMG2:
				fileRefs = append(fileRefs, fids...)
				for _, sid := range sids {
					fileRefs = append(fileRefs, "sed:"+sid)
				}
			case PollStatusPreviewOnly:
				lastPreviewFids = fids
				lastPreviewSids = sids
				if len(fids) > 0 || len(sids) > 0 {
					result.IsPreview = true
					fileRefs = append(fileRefs, fids...)
					for _, sid := range sids {
						fileRefs = append(fileRefs, "sed:"+sid)
					}
				}
				if len(fileRefs) == 0 && turn < sameConvMax {
					if mapping, mappingErr := client.GetConversationMapping(ctx, convID); mappingErr == nil {
						if rawMapping, ok := mapping["mapping"].(map[string]any); ok {
							baseline = buildImageBaseline(rawMapping)
						}
						if head, ok := mapping["current_node"].(string); ok && head != "" {
							parentID = head
						}
					}
				}
			case PollStatusTimeout:
				if attempt < maxAttempts {
					continue attemptLoop
				}
				if textOnlyErr != nil {
					return nil, noRelayRetry(textOnlyErr, http.StatusUnprocessableEntity)
				}
				return nil, noRelayRetry(errors.New("chatgpt web channel: poll timeout"), http.StatusGatewayTimeout)
			case PollStatusCanceled:
				ctxErr := ctx.Err()
				if ctxErr == nil {
					ctxErr = context.Canceled
				}
				return nil, noRelayRetry(fmt.Errorf("chatgpt web channel: image request canceled: %w", ctxErr), http.StatusRequestTimeout)
			default:
				if attempt < maxAttempts {
					continue attemptLoop
				}
				if pollStatus == PollStatusImageError {
					return nil, noRelayRetry(imageGenerationUpstreamError(), http.StatusBadGateway)
				}
				if pollStatus == PollStatusRateLimited {
					if len(lastPreviewFids) > 0 || len(lastPreviewSids) > 0 {
						result.IsPreview = true
						fileRefs = append(fileRefs, lastPreviewFids...)
						for _, sid := range lastPreviewSids {
							fileRefs = append(fileRefs, "sed:"+sid)
						}
						break
					}
					return nil, relayStatusError(errors.New("chatgpt web channel: upstream rate limited while polling image result"), http.StatusTooManyRequests)
				}
				if textOnlyErr != nil {
					return nil, noRelayRetry(textOnlyErr, http.StatusUnprocessableEntity)
				}
				return nil, noRelayRetry(errors.New("chatgpt web channel: poll failed"), http.StatusBadGateway)
			}
			if len(fileRefs) > 0 {
				break
			}
		}
		if len(fileRefs) == 0 && (len(lastPreviewFids) > 0 || len(lastPreviewSids) > 0) {
			result.IsPreview = true
			fileRefs = append(fileRefs, lastPreviewFids...)
			for _, sid := range lastPreviewSids {
				fileRefs = append(fileRefs, "sed:"+sid)
			}
		}
		if len(fileRefs) > 0 {
			break
		}
	}
	if len(fileRefs) == 0 {
		return nil, errors.New("chatgpt web channel: no image result returned")
	}
	result.FileRefs = fileRefs
	if timing != nil {
		timing.Set("image_ref_count", len(fileRefs))
		timing.Set("image_preview", result.IsPreview)
	}
	result.SignedURLs = resolveImageDownloadURLs(ctx, client, convID, fileRefs, testMode, timing)
	if len(result.SignedURLs) == 0 && ctx.Err() == nil && convID != "" {
		repollStart := time.Now()
		pollStatus, fids, sids := client.PollConversationForImages(ctx, convID, PollOpts{
			MaxWait:      chatGPTWebImageDownloadURLMaxWait(testMode),
			Interval:     2 * time.Second,
			StableRounds: 1,
			PreviewWait:  15 * time.Second,
		})
		if timing != nil {
			timing.ObserveSince("image_download_repoll_ms", repollStart)
			timing.Set("image_download_repoll_status", string(pollStatus))
		}
		if pollStatus == PollStatusIMG2 || pollStatus == PollStatusPreviewOnly {
			extraRefs := append([]string{}, fids...)
			for _, sid := range sids {
				extraRefs = append(extraRefs, "sed:"+sid)
			}
			if len(extraRefs) > 0 {
				fileRefs = dedupeStrings(append(fileRefs, extraRefs...))
				result.FileRefs = fileRefs
				if timing != nil {
					timing.Set("image_ref_count", len(fileRefs))
				}
				result.SignedURLs = resolveImageDownloadURLs(ctx, client, convID, fileRefs, testMode, timing)
			}
		}
	}
	if len(result.SignedURLs) == 0 {
		return nil, errors.New("chatgpt web channel: no downloadable image url returned")
	}
	if timing != nil {
		timing.Set("signed_url_count", len(result.SignedURLs))
	}
	return result, nil
}

func imageSSETextWithoutImageError(result ImageSSEResult) error {
	if len(result.FileIDs) > 0 || len(result.SedimentIDs) > 0 || strings.TrimSpace(result.ImageGenTaskID) != "" {
		return nil
	}
	content := strings.TrimSpace(result.Content)
	if content == "" {
		return nil
	}
	const maxLen = 300
	runes := []rune(content)
	if len(runes) > maxLen {
		content = string(runes[:maxLen]) + "..."
	}
	return fmt.Errorf("chatgpt web channel: no image generated: %s", content)
}

func shouldPollTextOnlyImageSSE(result ImageSSEResult) bool {
	if strings.TrimSpace(result.ConversationID) == "" {
		return false
	}
	if len(result.FileIDs) > 0 || len(result.SedimentIDs) > 0 || strings.TrimSpace(result.ImageGenTaskID) != "" {
		return false
	}
	content := strings.TrimSpace(result.Content)
	if content == "" {
		return false
	}
	return !looksLikeImageGenerationRefusal(content)
}

func looksLikeImageGenerationRefusal(content string) bool {
	content = strings.ToLower(strings.TrimSpace(content))
	if content == "" {
		return false
	}
	refusalMarkers := []string{
		"i can't",
		"i cannot",
		"i’m unable",
		"i am unable",
		"unable to generate",
		"can't generate",
		"cannot generate",
		"sorry",
		"对不起",
		"抱歉",
		"不能生成",
		"无法生成",
		"不能帮",
		"无法帮",
		"不支持生成",
	}
	for _, marker := range refusalMarkers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func buildToolBaseline(mapping map[string]any) map[string]struct{} {
	tools := ExtractImageToolMsgs(mapping)
	if len(tools) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		out[tool.MessageID] = struct{}{}
	}
	return out
}

func buildImageBaseline(mapping map[string]any) imageBaseline {
	fileIDs, sedimentIDs := ExtractImageRefsFromMapping(mapping)
	return imageBaseline{
		ToolIDs:     buildToolBaseline(mapping),
		FileIDs:     stringSliceSet(fileIDs),
		SedimentIDs: stringSliceSet(sedimentIDs),
	}
}

func stringSliceSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out[item] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func uploadedFileIDSet(files []*UploadedFile) map[string]struct{} {
	if len(files) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		if fileID := strings.TrimSpace(file.FileID); fileID != "" {
			out[fileID] = struct{}{}
		}
		if libraryFileID := strings.TrimSpace(file.LibraryFileID); libraryFileID != "" {
			out[libraryFileID] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildGenerationResponse(ctx context.Context, client *Client, req generationRequest, run *imageRunResult, testMode bool, info *relaycommon.RelayInfo, publicBaseURL string, timings ...*service.ChatGPTWebTiming) (*generationResponse, error) {
	timing := firstChatGPTWebTiming(timings...)
	responseFormat := strings.TrimSpace(req.ResponseFormat)
	forceB64JSON := shouldForceB64JSONImageResponse(req.Model)
	if !forceB64JSON && info != nil && info.ChannelMeta != nil {
		forceB64JSON = shouldForceB64JSONImageResponse(info.ChannelMeta.UpstreamModelName)
	}
	if forceB64JSON {
		responseFormat = "b64_json"
	}

	data := make([]dto.ImageData, 0, len(run.SignedURLs))
	for _, signedURL := range run.SignedURLs {
		if testMode {
			data = append(data, dto.ImageData{Url: signedURL})
			continue
		}
		fetchStart := time.Now()
		imageBytes, _, err := client.FetchImage(ctx, signedURL, 20*1024*1024)
		if timing != nil {
			timing.ObserveSince("image_fetch_ms", fetchStart)
		}
		if err != nil {
			return nil, err
		}
		base64Start := time.Now()
		b64 := base64.StdEncoding.EncodeToString(imageBytes)
		if timing != nil {
			timing.ObserveSince("image_base64_ms", base64Start)
		}
		item := dto.ImageData{B64Json: b64}
		if responseFormat != "b64_json" {
			item.Url = signedURL
		}
		data = append(data, item)
	}
	if !testMode && responseFormat != "b64_json" {
		taskStart := time.Now()
		if publicURLs, ok := createPublicPlaygroundImageTask(info, req.Prompt, req.Model, data, publicBaseURL); ok {
			for index := range data {
				if index < len(publicURLs) {
					data[index].Url = publicURLs[index]
				}
			}
		}
		if timing != nil {
			timing.ObserveSince("image_task_insert_ms", taskStart)
		}
	}
	return &generationResponse{
		Created:        time.Now().Unix(),
		Data:           data,
		ConversationID: strings.TrimSpace(run.ConversationID),
		Usage: dto.Usage{
			PromptTokens:     1,
			CompletionTokens: len(data),
			TotalTokens:      1 + len(data),
		},
	}, nil
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
