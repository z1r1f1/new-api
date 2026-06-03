package deepseek

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
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
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	fimBaseUrl := info.ChannelBaseUrl
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
	default:
		if !strings.HasSuffix(info.ChannelBaseUrl, "/beta") {
			fimBaseUrl += "/beta"
		}
		switch info.RelayMode {
		case constant.RelayModeCompletions:
			return fmt.Sprintf("%s/completions", fimBaseUrl), nil
		default:
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		}
	}
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

	// Capture reasoning_effort from request body for logging when not already set by V4 thinking suffix
	if info.ReasoningEffort == "" && request.ReasoningEffort != "" {
		info.ReasoningEffort = request.ReasoningEffort
	}

	return request, nil
}

func applyDeepSeekV4OpenAIThinkingSuffix(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) error {
	modelName := request.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	baseModel, thinkingType, effort, ok := reasoning.ParseDeepSeekV4ThinkingSuffix(modelName)
	if ok {
		thinking, err := common.Marshal(map[string]string{
			"type": thinkingType,
		})
		if err != nil {
			return fmt.Errorf("error marshalling thinking: %w", err)
		}
		request.Model = baseModel
		request.THINKING = thinking
		request.ReasoningEffort = effort
		if info != nil {
			if info.ChannelMeta != nil {
				info.UpstreamModelName = baseModel
			}
			info.ReasoningEffort = effort
		}
		return nil
	}

	// No V4 suffix — normalize explicit thinking / reasoning params for DeepSeek upstream and log display.
	normalizeDeepSeekExplicitThinking(info, request)
	return nil
}

// normalizeDeepSeekExplicitThinking consumes enable_thinking and normalises
// thinking / reasoning_effort into the shape that DeepSeek upstream expects.
// It also records the effective effort into info.ReasoningEffort for the log UI.
func normalizeDeepSeekExplicitThinking(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	// 1. enable_thinking (Ali Qwen / generic compat) → thinking.type
	if request.EnableThinking != nil {
		var raw interface{}
		if err := common.Unmarshal(request.EnableThinking, &raw); err == nil {
			switch v := raw.(type) {
			case bool:
				if v {
					setDeepSeekThinkingType(request, "enabled")
				} else {
					setDeepSeekThinkingType(request, "disabled")
				}
			case string:
				setDeepSeekThinkingType(request, v)
			}
		}
		request.EnableThinking = nil // consumed — upstream does not expect this key
	}

	// 2. Existing thinking object — capture type for logging
	if request.THINKING != nil && info.ReasoningEffort == "" {
		var tm map[string]interface{}
		if err := common.Unmarshal(request.THINKING, &tm); err == nil {
			if t, ok := tm["type"].(string); ok {
				info.ReasoningEffort = t
			}
		}
	}

	// 3. Top-level reasoning_effort — capture for logging
	if info.ReasoningEffort == "" && request.ReasoningEffort != "" {
		info.ReasoningEffort = request.ReasoningEffort
	}
}

// setDeepSeekThinkingType marshals a {"type": t} value into request.THINKING.
func setDeepSeekThinkingType(request *dto.GeneralOpenAIRequest, t string) {
	raw, _ := common.Marshal(map[string]string{"type": t})
	request.THINKING = raw
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
