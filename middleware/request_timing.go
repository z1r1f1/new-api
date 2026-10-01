package middleware

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

// RequestTiming logs one timing summary for requests marked by the 8999 proxy.
func RequestTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		nginxID := c.GetHeader(common.RequestTimingHeader)
		if !validNginxRequestID(nginxID) {
			c.Next()
			return
		}
		timing := common.NewRequestTiming(nginxID, time.Now())
		common.SetRequestTiming(c, timing)
		c.Next()

		payload := struct {
			common.RequestTimingSnapshot
			AppID         string `json:"app_id"`
			Method        string `json:"method"`
			Path          string `json:"path"`
			Status        int    `json:"status"`
			ContentLength int64  `json:"content_length"`
		}{timing.Snapshot(time.Now()), c.GetString(common.RequestIdKey), c.Request.Method, c.FullPath(), c.Writer.Status(), c.Request.ContentLength}
		if data, err := common.Marshal(payload); err == nil {
			logger.LogInfo(c, "perf8999 "+string(data))
		}
	}
}

func validNginxRequestID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for i := range id {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
