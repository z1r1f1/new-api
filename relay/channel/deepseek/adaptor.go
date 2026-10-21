package deepseek

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitreasoning "github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	// DeepSeek-compatible channels use Chat Completions, not Anthropic's
	// native thinking renderer. Apply DeepSeek's controls after conversion.
	openAIRequest, err := service.ClaudeToOpenAIRequest(*req, info)
	if err != nil {
		return nil, err
	}
	if info.SupportStreamOptions && info.IsStream {
		openAIRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return a.ConvertOpenAIRequest(c, info, openAIRequest)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// DeepSeek runs Claude Code and Codex searches and page fetches
	// server-side and bills them through its Search and Fetch APIs, whatever
	// model name the client sent.
	info.WebSearchBillingKey = SearchToolPriceKey
	info.WebFetchBillingKey = FetchToolPriceKey
	if info.RelayMode == constant.RelayModeResponses {
		info.VendorToolUsage = responsesToolUsage
	}
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// Tool endpoints are independent of the client's conversation format.
	switch info.RelayMode {
	case constant.RelayModeToolsSearch:
		return fmt.Sprintf("%s/v1/tools/search", info.ChannelBaseUrl), nil
	case constant.RelayModeToolsFetch:
		return fmt.Sprintf("%s/v1/tools/fetch", info.ChannelBaseUrl), nil
	}
	if info.RelayFormat != types.RelayFormatClaude && info.RelayMode == constant.RelayModeCompletions {
		fimBaseUrl := info.ChannelBaseUrl
		if !strings.HasSuffix(fimBaseUrl, "/beta") {
			fimBaseUrl += "/beta"
		}
		return fmt.Sprintf("%s/completions", fimBaseUrl), nil
	}
	// Both Claude Messages and Responses are bridged through Chat Completions.
	return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if err := applyDeepSeekV4OpenAIThinkingSuffix(info, request); err != nil {
		return nil, err
	}

	return request, nil
}

func applyDeepSeekV4OpenAIThinkingSuffix(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) error {
	modelName := request.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	baseModel, thinkingType, effort, ok := deepSeekThinking(info, modelName)
	if !ok {
		return nil
	}
	thinking, err := common.Marshal(map[string]string{
		"type": thinkingType,
	})
	if err != nil {
		return fmt.Errorf("error marshalling thinking: %w", err)
	}
	request.Model = baseModel
	request.THINKING = thinking
	request.ReasoningEffort = effort
	if info.ReasoningState() != nil {
		// DeepSeek Chat has no reasoning object; a budget modifier adds one.
		request.Reasoning = nil
	}
	if info != nil {
		if info.ChannelMeta != nil {
			info.UpstreamModelName = baseModel
		}
		info.SetReasoningEffort(deepSeekRunEffort(thinkingType, effort))
	}
	return nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return convertDeepSeekResponsesRequest(c, info, request)
}

// deepSeekThinking returns the model name to send with DeepSeek's thinking
// switch and effort. An explicit @ modifier (the reasoning state) wins over a
// -none/-low/-max tail, which is still stripped from the name. A budget maps
// to an effort as on other effort-only targets; thinking on or adaptive leaves
// the effort to DeepSeek's default.
func deepSeekThinking(info *relaycommon.RelayInfo, modelName string) (baseModel string, thinkingType string, effort string, ok bool) {
	baseModel = modelName
	if !model_setting.ShouldPreserveThinkingSuffix(modelName) && (info == nil || !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName)) {
		baseModel, thinkingType, effort, ok = reasoning.ParseDeepSeekV4ThinkingSuffix(modelName)
	}
	state := info.ReasoningState()
	if state == nil {
		return baseModel, thinkingType, effort, ok
	}
	intent := kitreasoning.IntentFromState(state)
	switch runs := kitreasoning.EffectiveEffort(intent); {
	case runs == "":
		return baseModel, thinkingType, effort, ok
	case runs == kitreasoning.EffortNone:
		return baseModel, "disabled", "", true
	case intent.Effort == "" && intent.BudgetTokens == nil:
		return baseModel, "enabled", "", true
	default:
		return baseModel, "enabled", string(runs), true
	}
}

// deepSeekRunEffort is the effort DeepSeek runs: none with thinking off, its
// default high when no effort is sent, and the levels it does not implement
// mapped as DeepSeek maps them (minimal to low, medium and xhigh to high).
func deepSeekRunEffort(thinkingType string, effort string) string {
	switch {
	case thinkingType == "disabled":
		return string(kitreasoning.EffortNone)
	case effort == "" || effort == "medium" || effort == "xhigh":
		return "high"
	case effort == "minimal":
		return "low"
	default:
		return effort
	}
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		if isDeepSeekResponsesRelay(info) {
			if info.IsStream {
				return handleDeepSeekResponsesStream(c, info, resp)
			}
			return handleDeepSeekResponsesBlocking(c, info, resp)
		}
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
