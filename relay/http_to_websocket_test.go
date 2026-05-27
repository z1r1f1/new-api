package relay

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/codex"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type httpToWebsocketTestAdaptor struct {
	called bool
}

func (a *httpToWebsocketTestAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *httpToWebsocketTestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return "", nil
}
func (a *httpToWebsocketTestAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}
func (a *httpToWebsocketTestAdaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error) {
	return nil, errors.New("fallback http request should not be used")
}
func (a *httpToWebsocketTestAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) GetModelList() []string { return nil }
func (a *httpToWebsocketTestAdaptor) GetChannelName() string { return "test" }
func (a *httpToWebsocketTestAdaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketTestAdaptor) DoHTTPToWebsocketRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (*http.Response, error) {
	a.called = true
	return &http.Response{StatusCode: http.StatusOK}, nil
}

type httpToWebsocketDisabledAdaptor struct {
	httpToWebsocketTestAdaptor
	fallbackCalled bool
}

func (a *httpToWebsocketDisabledAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error) {
	a.fallbackCalled = true
	return &http.Response{StatusCode: http.StatusOK}, nil
}

type httpToWebsocketUnsupportedAdaptor struct {
	fallbackCalled bool
}

func (a *httpToWebsocketUnsupportedAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *httpToWebsocketUnsupportedAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return "", nil
}
func (a *httpToWebsocketUnsupportedAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}
func (a *httpToWebsocketUnsupportedAdaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error) {
	a.fallbackCalled = true
	return &http.Response{StatusCode: http.StatusOK}, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) GetModelList() []string { return nil }
func (a *httpToWebsocketUnsupportedAdaptor) GetChannelName() string { return "unsupported-test" }
func (a *httpToWebsocketUnsupportedAdaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, nil
}
func (a *httpToWebsocketUnsupportedAdaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, nil
}

func withHTTPToWebsocketConversionSetting(t *testing.T, enabled bool) {
	t.Helper()
	settings := model_setting.GetGlobalSettings()
	original := settings.HTTPToWebsocketConversionEnabled
	settings.HTTPToWebsocketConversionEnabled = enabled
	t.Cleanup(func() { settings.HTTPToWebsocketConversionEnabled = original })
}

func TestShouldUseHTTPToWebsocketConversionOnlyForTextHTTPRelays(t *testing.T) {
	withHTTPToWebsocketConversionSetting(t, true)

	if !shouldUseHTTPToWebsocketConversion(&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}) {
		t.Fatal("expected chat completions to be eligible for websocket conversion")
	}
	if !shouldUseHTTPToWebsocketConversion(&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}) {
		t.Fatal("expected responses to be eligible for websocket conversion")
	}
	if shouldUseHTTPToWebsocketConversion(&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}) {
		t.Fatal("expected image generation to be excluded from websocket conversion")
	}
	if shouldUseHTTPToWebsocketConversion(&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeRealtime}) {
		t.Fatal("expected native realtime websocket relay to be excluded")
	}
}

func TestDoRequestWithOptionalHTTPToWebsocketUsesSupportedAdaptor(t *testing.T) {
	withHTTPToWebsocketConversionSetting(t, true)
	adaptor := &httpToWebsocketTestAdaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}

	resp, err := doRequestWithOptionalHTTPToWebsocket(nil, info, adaptor, nil)
	if err != nil {
		t.Fatalf("doRequestWithOptionalHTTPToWebsocket returned error: %v", err)
	}
	if !adaptor.called {
		t.Fatal("expected websocket converter to be called")
	}
	if _, ok := resp.(*http.Response); !ok {
		t.Fatalf("expected *http.Response, got %T", resp)
	}
}

func TestDoRequestWithOptionalHTTPToWebsocketSkipsWhenDisabled(t *testing.T) {
	withHTTPToWebsocketConversionSetting(t, false)
	adaptor := &httpToWebsocketDisabledAdaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}

	_, err := doRequestWithOptionalHTTPToWebsocket(nil, info, adaptor, nil)
	if err != nil {
		t.Fatalf("doRequestWithOptionalHTTPToWebsocket returned error: %v", err)
	}
	if adaptor.called {
		t.Fatal("expected websocket converter not to be called while setting is disabled")
	}
	if !adaptor.fallbackCalled {
		t.Fatal("expected normal HTTP request path while setting is disabled")
	}
}

func TestDoRequestWithOptionalHTTPToWebsocketFallsBackWhenUnsupported(t *testing.T) {
	withHTTPToWebsocketConversionSetting(t, true)
	adaptor := &httpToWebsocketUnsupportedAdaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}

	_, err := doRequestWithOptionalHTTPToWebsocket(nil, info, adaptor, nil)
	if err != nil {
		t.Fatalf("doRequestWithOptionalHTTPToWebsocket returned error: %v", err)
	}
	if !adaptor.fallbackCalled {
		t.Fatal("expected normal HTTP request path to be used for unsupported adaptors")
	}
}

type httpToWebsocketFailingAdaptor struct {
	httpToWebsocketTestAdaptor
	converterBody  string
	fallbackBody   string
	fallbackCalled bool
}

func (a *httpToWebsocketFailingAdaptor) DoHTTPToWebsocketRequest(_ *gin.Context, _ *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	body, _ := io.ReadAll(requestBody)
	a.converterBody = string(body)
	return nil, errors.New("websocket upstream failed")
}

func (a *httpToWebsocketFailingAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	body, _ := io.ReadAll(requestBody)
	a.fallbackBody = string(body)
	a.fallbackCalled = true
	return &http.Response{StatusCode: http.StatusOK}, nil
}

func TestDoRequestWithOptionalHTTPToWebsocketFallsBackWithReplayableBodyWhenConverterFails(t *testing.T) {
	withHTTPToWebsocketConversionSetting(t, true)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	adaptor := &httpToWebsocketFailingAdaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}

	resp, err := doRequestWithOptionalHTTPToWebsocket(ctx, info, adaptor, strings.NewReader(`{"model":"gpt-5.5"}`))
	if err != nil {
		t.Fatalf("doRequestWithOptionalHTTPToWebsocket returned error: %v", err)
	}
	if _, ok := resp.(*http.Response); !ok {
		t.Fatalf("expected fallback *http.Response, got %T", resp)
	}
	if adaptor.converterBody != `{"model":"gpt-5.5"}` {
		t.Fatalf("expected converter to receive request body, got %q", adaptor.converterBody)
	}
	if !adaptor.fallbackCalled {
		t.Fatal("expected normal HTTP fallback after websocket converter failure")
	}
	if adaptor.fallbackBody != adaptor.converterBody {
		t.Fatalf("expected fallback to replay original request body, converter=%q fallback=%q", adaptor.converterBody, adaptor.fallbackBody)
	}
	if got := ctx.GetString(httpToWebsocketConversionStatusKey); got != "fallback_error" {
		t.Fatalf("expected fallback_error status, got %q", got)
	}
	if used, _ := ctx.Get(httpToWebsocketConversionUsedKey); used != false {
		t.Fatalf("expected websocket used=false after fallback, got %#v", used)
	}
}

func TestOpenAIAndCodexAdaptorsSupportHTTPToWebsocketConversion(t *testing.T) {
	if _, ok := any(&openai.Adaptor{}).(httpToWebsocketConverter); !ok {
		t.Fatal("expected OpenAI adaptor to implement HTTP-to-websocket conversion")
	}
	if _, ok := any(&codex.Adaptor{}).(httpToWebsocketConverter); !ok {
		t.Fatal("expected Codex adaptor to implement HTTP-to-websocket conversion")
	}
}

var _ channel.Adaptor = (*httpToWebsocketTestAdaptor)(nil)
var _ channel.Adaptor = (*httpToWebsocketUnsupportedAdaptor)(nil)
var _ channel.Adaptor = (*httpToWebsocketDisabledAdaptor)(nil)
var _ channel.Adaptor = (*httpToWebsocketFailingAdaptor)(nil)
