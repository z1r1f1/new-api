package relay

import (
	"io"
	"net/http"

	rootcommon "github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

const (
	httpToWebsocketConversionStatusKey = string(appconstant.ContextKeyHTTPToWebsocketConversionStatus)
	httpToWebsocketConversionUsedKey   = string(appconstant.ContextKeyHTTPToWebsocketConversionUsed)
)

type httpToWebsocketConverter interface {
	DoHTTPToWebsocketRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error)
}

func shouldUseHTTPToWebsocketConversion(info *relaycommon.RelayInfo) bool {
	if info == nil || !model_setting.GetGlobalSettings().HTTPToWebsocketConversionEnabled {
		return false
	}
	if info.IsChannelTest {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeCompletions,
		relayconstant.RelayModeResponses:
		return true
	default:
		return false
	}
}

func doRequestWithOptionalHTTPToWebsocket(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, requestBody io.Reader) (any, error) {
	if !shouldUseHTTPToWebsocketConversion(info) {
		return adaptor.DoRequest(c, info, requestBody)
	}

	converter, ok := adaptor.(httpToWebsocketConverter)
	if !ok {
		if c != nil {
			c.Set(httpToWebsocketConversionStatusKey, "unsupported")
			c.Set(httpToWebsocketConversionUsedKey, false)
		}
		return adaptor.DoRequest(c, info, requestBody)
	}

	storage, replayableBody, err := makeHTTPToWebsocketReplayableBody(requestBody, info)
	if err != nil {
		if c != nil {
			c.Set(httpToWebsocketConversionStatusKey, "body_replay_failed")
			c.Set(httpToWebsocketConversionUsedKey, false)
		}
		return nil, err
	}
	if storage != nil {
		defer storage.Close()
		requestBody = replayableBody
	}

	resp, err := converter.DoHTTPToWebsocketRequest(c, info, requestBody)
	if err != nil {
		if c != nil {
			c.Set(httpToWebsocketConversionStatusKey, "fallback_error")
			c.Set(httpToWebsocketConversionUsedKey, false)
		}
		if storage != nil {
			if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
				return nil, seekErr
			}
			return adaptor.DoRequest(c, info, rootcommon.ReaderOnly(storage))
		}
		return adaptor.DoRequest(c, info, requestBody)
	}

	if c != nil {
		c.Set(httpToWebsocketConversionStatusKey, "converted")
		c.Set(httpToWebsocketConversionUsedKey, true)
	}
	return resp, nil
}

func makeHTTPToWebsocketReplayableBody(requestBody io.Reader, info *relaycommon.RelayInfo) (rootcommon.BodyStorage, io.Reader, error) {
	if requestBody == nil {
		return nil, nil, nil
	}
	maxMB := appconstant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	contentLength := int64(-1)
	if info != nil && info.UpstreamRequestBodySize > 0 {
		contentLength = info.UpstreamRequestBodySize
	}
	storage, err := rootcommon.CreateBodyStorageFromReader(requestBody, contentLength, int64(maxMB)<<20)
	if err != nil {
		return nil, nil, err
	}
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		_ = storage.Close()
		return nil, nil, err
	}
	return storage, rootcommon.ReaderOnly(storage), nil
}
