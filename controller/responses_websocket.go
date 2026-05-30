package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/helper"

	"github.com/gin-gonic/gin"
)

func ResponsesWebSocket(c *gin.Context) {
	requestId := c.GetString(common.RequestIdKey)
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	if newAPIError := relay.ResponsesWebSocketHelper(c, ws); newAPIError != nil {
		errorPreview := common.LocalLogPreview(newAPIError.Error())
		logger.LogError(c, fmt.Sprintf("responses websocket relay error: %s", errorPreview))
		newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
		helper.WssError(c, ws, newAPIError.ToOpenAIError())
	}
}
