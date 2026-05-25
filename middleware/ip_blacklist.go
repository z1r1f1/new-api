package middleware

import (
	"net"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func IPBlacklist() gin.HandlerFunc {
	return func(c *gin.Context) {
		setting := system_setting.GetIPBlacklistSetting()
		if !setting.Enabled {
			c.Next()
			return
		}

		blacklist := common.SplitIPList(setting.List)
		if len(blacklist) == 0 {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		ip := net.ParseIP(clientIP)
		if ip == nil {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "无法解析客户端 IP 地址",
			})
			c.Abort()
			return
		}

		if common.IsIpInCIDRList(ip, blacklist) {
			logger.LogWarn(c, "blocked request from blacklisted IP "+clientIP)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "当前 IP 已被禁止访问",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
