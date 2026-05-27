package relay

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

const (
	httpToWebsocketConversionStatusKey = "http_to_websocket_conversion_status"
	httpToWebsocketConversionUsedKey   = "http_to_websocket_conversion_used"
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

	if c != nil {
		c.Set(httpToWebsocketConversionStatusKey, "converted")
		c.Set(httpToWebsocketConversionUsedKey, true)
	}
	return converter.DoHTTPToWebsocketRequest(c, info, requestBody)
}
