package chatgptimg

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var ModelList = []string{
	"gpt-image-2",
	"gpt-image-1",
	"chatgpt-image-latest",
}

const ChannelName = "chatgpt-image"

type Adaptor struct{}

type generationRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	N               int      `json:"n,omitempty"`
	Size            string   `json:"size,omitempty"`
	Quality         string   `json:"quality,omitempty"`
	Style           string   `json:"style,omitempty"`
	ResponseFormat  string   `json:"response_format,omitempty"`
	ReferenceImages []string `json:"reference_images,omitempty"`
}

type generationResponse struct {
	Created int64           `json:"created"`
	Data    []dto.ImageData `json:"data"`
	Usage   dto.Usage       `json:"usage"`
}

type imageRunResult struct {
	ConversationID string
	FileRefs       []string
	SignedURLs     []string
	IsPreview      bool
	TurnsInConv    int
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("chatgpt image channel: /v1beta/models endpoint not supported")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("chatgpt image channel: /v1/messages endpoint not supported")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("chatgpt image channel: audio endpoint not supported")
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("chatgpt image channel: /v1/chat/completions endpoint not supported")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("chatgpt image channel: /v1/rerank endpoint not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("chatgpt image channel: /v1/embeddings endpoint not supported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("chatgpt image channel: /v1/responses endpoint not supported")
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
	if converted.Model == "" && info != nil {
		converted.Model = strings.TrimSpace(info.UpstreamModelName)
	}
	if converted.Model == "" {
		converted.Model = ModelList[0]
	}

	refs, err := extractReferenceImagesFromRequest(c, info, request)
	if err != nil {
		return nil, err
	}
	converted.ReferenceImages = refs
	return converted, nil
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
		return nil, fmt.Errorf("chatgpt image channel: read request body failed: %w", err)
	}
	var req generationRequest
	if err := common.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("chatgpt image channel: invalid image request json: %w", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("chatgpt image channel: prompt is required")
	}

	oauthKey, err := ParseOAuthKey(info.ApiKey)
	if err != nil {
		return nil, err
	}
	accessToken, err := ResolveAccessToken(c.Request.Context(), oauthKey, info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(ClientOptions{
		BaseURL:    chooseBaseURL(info),
		AuthToken:  accessToken,
		DeviceID:   strings.TrimSpace(oauthKey.DeviceID),
		SessionID:  strings.TrimSpace(oauthKey.SessionID),
		ProxyURL:   strings.TrimSpace(info.ChannelSetting.Proxy),
		Timeout:    150 * time.Second,
		SSETimeout: 120 * time.Second,
	})
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

	respPayload, err := buildGenerationResponse(c.Request.Context(), client, req, res, testMode)
	if err != nil {
		return nil, err
	}
	recordGenerationDrawingLog(info, req, res, respPayload)
	payloadBytes, err := common.Marshal(respPayload)
	if err != nil {
		return nil, fmt.Errorf("chatgpt image channel: marshal synthetic response failed: %w", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payloadBytes)),
	}, nil
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
		return nil, types.NewError(errors.New("chatgpt image channel: nil response"), types.ErrorCodeBadResponse)
	}
	return openai.OpenaiHandlerWithUsage(c, info, resp)
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
			return nil, fmt.Errorf("chatgpt image channel: invalid reference_images: %w", err)
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
			return nil, fmt.Errorf("chatgpt image channel: parse multipart form failed: %w", err)
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
			return nil, fmt.Errorf("chatgpt image channel: open multipart image failed: %w", err)
		}
		data, readErr := io.ReadAll(file)
		_ = file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("chatgpt image channel: read multipart image failed: %w", readErr)
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
	maxAttempts := 2
	if len(refs) > 0 || testMode {
		maxAttempts = 1
	}
	pollMaxWait := 300 * time.Second
	sameConvMax := 3
	if testMode {
		pollMaxWait = 45 * time.Second
		sameConvMax = 1
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
	var baselineTools = map[string]struct{}{}
	var lastPreviewFids []string
	var lastPreviewSids []string
	var fileRefs []string

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
		convID = ""
		parentID = uuid.NewString()
		messageID = uuid.NewString()
		baselineTools = map[string]struct{}{}
		lastPreviewFids = nil
		lastPreviewSids = nil
		fileRefs = nil
		result.IsPreview = false

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
				Prompt:        req.Prompt,
				UpstreamModel: "auto",
				ConvID:        convID,
				ParentMsgID:   parentID,
				MessageID:     messageID,
				ChatToken:     cr.Token,
				ProofToken:    proofToken,
				References:    refs,
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
			if sseResult.ConversationID != "" {
				convID = sseResult.ConversationID
				result.ConversationID = convID
			}
			if testMode && convID != "" {
				return result, nil
			}
			if len(sseResult.FileIDs) > 0 {
				fileRefs = append(fileRefs, sseResult.FileIDs...)
				for _, sid := range sseResult.SedimentIDs {
					fileRefs = append(fileRefs, "sed:"+sid)
				}
				break
			}
			if convID == "" {
				return nil, errors.New("chatgpt image channel: missing conversation id from SSE")
			}
			pollStatus, fids, sids := client.PollConversationForImages(ctx, convID, PollOpts{MaxWait: pollMaxWait, BaselineToolIDs: baselineTools})
			switch pollStatus {
			case PollStatusIMG2:
				fileRefs = append(fileRefs, fids...)
				for _, sid := range sids {
					fileRefs = append(fileRefs, "sed:"+sid)
				}
			case PollStatusPreviewOnly:
				lastPreviewFids = fids
				lastPreviewSids = sids
				if testMode {
					result.IsPreview = true
					fileRefs = append(fileRefs, fids...)
					for _, sid := range sids {
						fileRefs = append(fileRefs, "sed:"+sid)
					}
				}
				if len(fileRefs) == 0 && turn < sameConvMax {
					if mapping, mappingErr := client.GetConversationMapping(ctx, convID); mappingErr == nil {
						if rawMapping, ok := mapping["mapping"].(map[string]any); ok {
							baselineTools = buildToolBaseline(rawMapping)
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
				return nil, errors.New("chatgpt image channel: poll timeout")
			default:
				if attempt < maxAttempts {
					continue attemptLoop
				}
				return nil, errors.New("chatgpt image channel: poll failed")
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
		return nil, errors.New("chatgpt image channel: no image result returned")
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
		return nil, errors.New("chatgpt image channel: no downloadable image url returned")
	}
	return result, nil
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

func buildGenerationResponse(ctx context.Context, client *Client, req generationRequest, run *imageRunResult, testMode bool) (*generationResponse, error) {
	data := make([]dto.ImageData, 0, len(run.SignedURLs))
	for _, signedURL := range run.SignedURLs {
		if testMode {
			data = append(data, dto.ImageData{Url: signedURL})
			continue
		}
		imageBytes, contentType, err := client.FetchImage(ctx, signedURL, 20*1024*1024)
		if err != nil {
			return nil, err
		}
		if contentType == "" {
			contentType = http.DetectContentType(imageBytes)
		}
		b64 := base64.StdEncoding.EncodeToString(imageBytes)
		item := dto.ImageData{B64Json: b64}
		if req.ResponseFormat != "b64_json" {
			item.Url = fmt.Sprintf("data:%s;base64,%s", contentType, b64)
		}
		data = append(data, item)
	}
	return &generationResponse{
		Created: time.Now().Unix(),
		Data:    data,
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
