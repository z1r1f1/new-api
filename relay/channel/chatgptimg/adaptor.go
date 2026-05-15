package chatgptimg

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	Model          string              `json:"model,omitempty"`
	Messages       []dto.Message       `json:"messages,omitempty"`
	Stream         *bool               `json:"stream,omitempty"`
	ResponseFormat *dto.ResponseFormat `json:"response_format,omitempty"`
	FallbackPrompt string              `json:"fallback_prompt,omitempty"`
	ConversationID string              `json:"conversation_id,omitempty"`
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

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("chatgpt web channel: /v1beta/models endpoint not supported")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("chatgpt web channel: /v1/messages endpoint not supported")
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

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("chatgpt web channel: /v1/responses endpoint not supported")
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
	var req generationRequest
	if err := common.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("chatgpt web channel: invalid image request json: %w", err)
	}
	normalizeGenerationRequestModelAndResponseFormat(&req, info)
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("chatgpt web channel: prompt is required")
	}

	client, err := newClientFromRelayInfo(c.Request.Context(), info)
	if err != nil {
		return nil, err
	}
	refs, err := uploadReferenceImages(c.Request.Context(), client, req.ReferenceImages)
	if err != nil {
		return nil, err
	}
	testMode := info != nil && info.IsChannelTest
	res, err := runImageGeneration(c.Request.Context(), client, req, refs, testMode)
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

	respPayload, err := buildGenerationResponse(c.Request.Context(), client, req, res, testMode, info, requestPublicBaseURLForImages(c, info))
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
	prompt := buildChatPrompt(req)
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("chatgpt web channel: chat prompt is empty")
	}
	client, err := newClientFromRelayInfo(c.Request.Context(), info)
	if err != nil {
		return nil, err
	}
	if info != nil && info.IsChannelTest {
		content, conversationID, usedPrompt, err := runChatCompletionProbe(c.Request.Context(), client, req, prompt)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(content) == "" {
			content = "ok"
		}
		usage := buildChatUsage(usedPrompt, content, req.Model)
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
		started, err := startChatStream(c.Request.Context(), client, req, prompt)
		if err != nil {
			return nil, err
		}
		return buildStreamingChatResponse(c.Request.Context(), client, started.Stream, req, started.Prompt, started.Baseline, info, requestPublicBaseURLForImages(c, info)), nil
	}
	content, conversationID, usedPrompt, baseline, hasImageGeneration, err := runChatCompletion(c.Request.Context(), client, req, prompt)
	if err != nil {
		return nil, err
	}
	content = materializePlaygroundInlineDataImages(content, info, usedPrompt, req.Model)
	content = materializeChatGPTContentImageURLs(c.Request.Context(), client, content, info, usedPrompt, req.Model, requestPublicBaseURLForImages(c, info))
	textContent := content
	hasInlineDataImage := chatContentHasInlineDataImage(content)
	allowImagePoll := !hasInlineDataImage && shouldPollChatGeneratedImages(req, usedPrompt, textContent, hasImageGeneration)
	if imageMarkdown, err := collectChatGeneratedImageMarkdown(c.Request.Context(), client, conversationID, baseline, allowImagePoll, info, usedPrompt, req.Model, requestPublicBaseURLForImages(c, info)); err != nil {
		return nil, err
	} else if imageMarkdown != "" {
		content = appendMarkdownBlock(content, imageMarkdown)
	}
	usage := buildChatUsage(usedPrompt, textContent, req.Model)
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

func newClientFromRelayInfo(ctx context.Context, info *relaycommon.RelayInfo) (*Client, error) {
	if info == nil {
		return nil, errors.New("chatgpt web channel: relay info is required")
	}
	oauthKey, err := ParseOAuthKey(info.ApiKey)
	if err != nil {
		return nil, err
	}
	accessToken, err := ResolveAccessToken(ctx, oauthKey, info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	return NewClient(ClientOptions{
		BaseURL:    chooseBaseURL(info),
		AuthToken:  accessToken,
		DeviceID:   strings.TrimSpace(oauthKey.DeviceID),
		SessionID:  strings.TrimSpace(oauthKey.SessionID),
		ProxyURL:   strings.TrimSpace(info.ChannelSetting.Proxy),
		Timeout:    150 * time.Second,
		SSETimeout: 300 * time.Second,
	})
}

func buildChatPrompt(req chatRequest) string {
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

	imageGenerationIntent := common.IsImageGenerationModel(req.Model) || chatTextRequestsImageGeneration(b.String())
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

type chatStreamStart struct {
	Stream   <-chan SSEEvent
	Baseline imageBaseline
	Prompt   string
}

func runChatCompletion(ctx context.Context, client *Client, req chatRequest, prompt string) (string, string, string, imageBaseline, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	started, err := startChatStream(ctx, client, req, prompt)
	if err != nil {
		return "", "", "", imageBaseline{}, false, err
	}
	result := ParseChatSSE(started.Stream)
	if result.Err != nil {
		return "", result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, result.Err
	}
	if containsImageGenerationUpstreamErrorText(result.Content) {
		return "", result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, imageGenerationUpstreamError()
	}
	if strings.TrimSpace(result.Content) == "" {
		return "", result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, errors.New("chatgpt web channel: empty chat response")
	}
	return result.Content, result.ConversationID, started.Prompt, started.Baseline, result.HasImageGeneration, nil
}

func runChatCompletionProbe(ctx context.Context, client *Client, req chatRequest, prompt string) (string, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	started, err := startChatStream(ctx, client, req, prompt)
	if err != nil {
		return "", "", "", err
	}
	result := ParseChatSSEUntilReady(started.Stream, 3*time.Second)
	if result.Err != nil {
		return "", result.ConversationID, started.Prompt, result.Err
	}
	if containsImageGenerationUpstreamErrorText(result.Content) {
		return "", result.ConversationID, started.Prompt, imageGenerationUpstreamError()
	}
	if strings.TrimSpace(result.ConversationID) == "" && strings.TrimSpace(result.Content) == "" {
		return "", "", started.Prompt, errors.New("chatgpt web channel: chat test did not receive a conversation id or content")
	}
	return result.Content, result.ConversationID, started.Prompt, nil
}

func collectChatGeneratedImageMarkdown(ctx context.Context, client *Client, conversationID string, baseline imageBaseline, allowPoll bool, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string) (string, error) {
	conversationID = strings.TrimSpace(conversationID)
	if client == nil || conversationID == "" {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	mapping, err := client.getMappingRaw(ctx, conversationID)
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
		pollStatus, fids, sids := client.PollConversationForImages(ctx, conversationID, PollOpts{
			MaxWait:             60 * time.Second,
			Interval:            2 * time.Second,
			StableRounds:        2,
			PreviewWait:         8 * time.Second,
			BaselineToolIDs:     baseline.ToolIDs,
			BaselineFileIDs:     baseline.FileIDs,
			BaselineSedimentIDs: baseline.SedimentIDs,
		})
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
	return imageRefsToMarkdown(ctx, client, conversationID, fileRefs, info, prompt, modelName, publicBaseURL), nil
}

func shouldPollChatGeneratedImages(req chatRequest, prompt, content string, hasImageGeneration bool) bool {
	if hasImageGeneration {
		return true
	}
	if common.IsImageGenerationModel(req.Model) {
		return true
	}
	return chatTextRequestsImageGeneration(prompt) || chatTextRequestsImageGeneration(content)
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

func imageRefsToMarkdown(ctx context.Context, client *Client, conversationID string, fileRefs []string, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string) string {
	refs := dedupeStrings(fileRefs)
	if len(refs) == 0 {
		return ""
	}
	publicURLs, ok := materializeImageRefsToPublicURLs(ctx, client, conversationID, refs, info, prompt, modelName, publicBaseURL)
	if ok {
		return imageURLsToMarkdown(publicURLs)
	}
	var b strings.Builder
	for index, ref := range refs {
		signedURL, err := client.ImageDownloadURL(ctx, conversationID, ref)
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

func materializeChatGPTContentImageURLs(ctx context.Context, client *Client, content string, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string) string {
	urls := extractChatGPTImageURLs(content)
	if len(urls) == 0 || client == nil || info == nil || info.UserId <= 0 {
		return content
	}
	data := make([]dto.ImageData, 0, len(urls))
	replacements := make(map[string]string, len(urls))
	for _, imageURL := range urls {
		fetchURL := strings.ReplaceAll(strings.TrimSpace(imageURL), "&amp;", "&")
		imageBytes, _, err := client.FetchImage(ctx, fetchURL, 20*1024*1024)
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
	publicURLs, ok := createPublicPlaygroundImageTask(info, prompt, modelName, data, publicBaseURL)
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

func materializeImageRefsToPublicURLs(ctx context.Context, client *Client, conversationID string, refs []string, info *relaycommon.RelayInfo, prompt, modelName, publicBaseURL string) ([]string, bool) {
	if client == nil || info == nil || info.UserId <= 0 || len(refs) == 0 {
		return nil, false
	}
	data := make([]dto.ImageData, 0, len(refs))
	for _, ref := range refs {
		signedURL, err := client.ImageDownloadURL(ctx, conversationID, ref)
		if err != nil || strings.TrimSpace(signedURL) == "" {
			continue
		}
		imageBytes, _, err := client.FetchImage(ctx, signedURL, 20*1024*1024)
		if err != nil || len(imageBytes) == 0 {
			continue
		}
		data = append(data, dto.ImageData{B64Json: base64.StdEncoding.EncodeToString(imageBytes)})
	}
	if len(data) == 0 {
		return nil, false
	}
	return createPublicPlaygroundImageTask(info, prompt, modelName, data, publicBaseURL)
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

func startChatStream(ctx context.Context, client *Client, req chatRequest, prompt string) (*chatStreamStart, error) {
	cr, err := client.ChatRequirementsV2(ctx)
	if err != nil {
		return nil, err
	}
	proofToken := ""
	if cr.Proofofwork.Required {
		proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
	}
	continuation := prepareConversationContinuation(ctx, client, req.ConversationID)
	convID := ""
	if continuation.Available {
		convID = continuation.ConvID
	}
	actualPrompt := prompt
	if !continuation.Available && strings.TrimSpace(req.ConversationID) != "" && strings.TrimSpace(req.FallbackPrompt) != "" {
		actualPrompt = strings.TrimSpace(req.FallbackPrompt)
	}
	convOpt := ChatConvOpts{
		Prompt:        actualPrompt,
		UpstreamModel: chatModelForWeb(req.Model),
		ConvID:        convID,
		ParentMsgID:   continuation.ParentID,
		MessageID:     uuid.NewString(),
		ChatToken:     cr.Token,
		ProofToken:    proofToken,
		SSETimeout:    300 * time.Second,
	}
	if conduitToken, conduitErr := client.PrepareChatConversation(ctx, convOpt); conduitErr == nil {
		convOpt.ConduitToken = conduitToken
	}
	stream, err := client.StreamChatConversation(ctx, convOpt)
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
	return model
}

func buildStreamingChatResponse(ctx context.Context, client *Client, stream <-chan SSEEvent, req chatRequest, prompt string, baseline imageBaseline, info *relaycommon.RelayInfo, publicBaseURL string) *http.Response {
	pr, pw := io.Pipe()
	go streamChatCompletion(ctx, client, stream, req, prompt, baseline, info, publicBaseURL, pw)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}
}

func streamChatCompletion(ctx context.Context, client *Client, stream <-chan SSEEvent, req chatRequest, prompt string, baseline imageBaseline, info *relaycommon.RelayInfo, publicBaseURL string, pw *io.PipeWriter) {
	defer pw.Close()
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
	for ev := range stream {
		delta, done, collectErr := CollectChatSSEEvent(ev, state)
		if state.ConversationID != "" {
			id = buildChatCompletionID(state.ConversationID)
		}
		if collectErr != nil {
			_ = pw.CloseWithError(collectErr)
			return
		}
		if delta != "" {
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
	finalContent := materializePlaygroundInlineDataImages(state.Content, info, prompt, model)
	finalContent = materializeChatGPTContentImageURLs(ctx, client, finalContent, info, prompt, model, publicBaseURL)
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
	if imageMarkdown, err := collectChatGeneratedImageMarkdown(ctx, client, state.ConversationID, baseline, allowImagePoll, info, prompt, model, publicBaseURL); err != nil {
		_ = pw.CloseWithError(err)
		return
	} else if imageMarkdown != "" {
		if strings.TrimSpace(state.Content) != "" {
			imageMarkdown = "\n\n" + imageMarkdown
		}
		writeChatStreamChunk(pw, id, created, model, "", imageMarkdown, nil, nil)
	}
	finish := "stop"
	usage := buildChatUsage(prompt, streamedContent, model)
	writeChatStreamChunk(pw, id, created, model, "", "", &finish, &usage)
	writeChatDone(pw)
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

func runImageGeneration(ctx context.Context, client *Client, req generationRequest, refs []*UploadedFile, testMode bool) (*imageRunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()

	result := &imageRunResult{}
	maxAttempts := 1
	pollMaxWait := 300 * time.Second
	sameConvMax := 1
	if testMode {
		pollMaxWait = 45 * time.Second
	}

	cr, err := client.ChatRequirementsV2(ctx)
	if err != nil {
		return nil, err
	}
	proofToken := ""
	if cr.Proofofwork.Required {
		proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
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
		if attempt > 1 {
			cr, err = client.ChatRequirementsV2(ctx)
			if err != nil {
				return nil, err
			}
			proofToken = ""
			if cr.Proofofwork.Required {
				proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
			}
		}
		continuation := prepareConversationContinuation(ctx, client, req.ConversationID)
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
					fallbackRefs, err = uploadReferenceImages(ctx, client, req.FallbackReferenceImages)
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
			if turn > 1 {
				cr, err = client.ChatRequirementsV2(ctx)
				if err != nil {
					return nil, err
				}
				proofToken = ""
				if cr.Proofofwork.Required {
					proofToken = SolveProofToken(cr.Proofofwork.Seed, cr.Proofofwork.Difficulty, defaultUserAgent)
				}
			}
			convOpt := ImageConvOpts{
				Prompt:        prompt,
				UpstreamModel: "auto",
				ConvID:        convID,
				ParentMsgID:   parentID,
				MessageID:     messageID,
				ChatToken:     cr.Token,
				ProofToken:    proofToken,
				References:    activeRefs,
			}
			if turn > 1 {
				convOpt.MessageID = uuid.NewString()
			}
			if conduitToken, conduitErr := client.PrepareFConversation(ctx, convOpt); conduitErr == nil {
				convOpt.ConduitToken = conduitToken
			}
			streamCtx, cancelStream := context.WithCancel(ctx)
			stream, err := client.StreamFConversation(streamCtx, convOpt)
			if err != nil {
				cancelStream()
				if ue, ok := err.(*UpstreamError); ok && ue.IsRateLimited() && attempt < maxAttempts {
					break
				}
				return nil, err
			}
			var sseResult ImageSSEResult
			if testMode {
				sseResult = ParseImageSSEUntilConversationReady(stream, 3*time.Second)
			} else {
				sseResult = ParseImageSSE(stream)
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
			if err := imageSSETextWithoutImageError(sseResult); err != nil {
				return nil, noRelayRetry(err, http.StatusUnprocessableEntity)
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
				return nil, noRelayRetry(errors.New("chatgpt web channel: poll timeout"), http.StatusGatewayTimeout)
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
	for _, ref := range fileRefs {
		signedURL, err := client.ImageDownloadURL(ctx, convID, ref)
		if err != nil {
			continue
		}
		result.SignedURLs = append(result.SignedURLs, signedURL)
	}
	if len(result.SignedURLs) == 0 {
		return nil, errors.New("chatgpt web channel: no downloadable image url returned")
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
		if file == nil || strings.TrimSpace(file.FileID) == "" {
			continue
		}
		out[strings.TrimSpace(file.FileID)] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildGenerationResponse(ctx context.Context, client *Client, req generationRequest, run *imageRunResult, testMode bool, info *relaycommon.RelayInfo, publicBaseURL string) (*generationResponse, error) {
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
		imageBytes, _, err := client.FetchImage(ctx, signedURL, 20*1024*1024)
		if err != nil {
			return nil, err
		}
		b64 := base64.StdEncoding.EncodeToString(imageBytes)
		item := dto.ImageData{B64Json: b64}
		if responseFormat != "b64_json" {
			item.Url = signedURL
		}
		data = append(data, item)
	}
	if !testMode && responseFormat != "b64_json" {
		if publicURLs, ok := createPublicPlaygroundImageTask(info, req.Prompt, req.Model, data, publicBaseURL); ok {
			for index := range data {
				if index < len(publicURLs) {
					data[index].Url = publicURLs[index]
				}
			}
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
