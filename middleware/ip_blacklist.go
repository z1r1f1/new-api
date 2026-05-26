package middleware

import (
	"net"
	"net/http"
	"strings"

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

		clientIPs := blacklistClientIPCandidates(c)
		if len(clientIPs) == 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "无法解析客户端 IP 地址",
			})
			c.Abort()
			return
		}

		for _, clientIP := range clientIPs {
			if common.IsIpInCIDRList(clientIP.ip, blacklist) {
				logger.LogWarn(c, "blocked request from blacklisted IP "+clientIP.raw)
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"message": "当前 IP 已被禁止访问",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

type blacklistClientIP struct {
	raw string
	ip  net.IP
}

func blacklistClientIPCandidates(c *gin.Context) []blacklistClientIP {
	candidates := make([]blacklistClientIP, 0, 8)
	seen := make(map[string]struct{})

	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = host
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return
		}
		key := ip.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, blacklistClientIP{
			raw: value,
			ip:  ip,
		})
	}

	add(c.ClientIP())
	add(c.Request.RemoteAddr)
	add(c.GetHeader("CF-Connecting-IP"))
	add(c.GetHeader("True-Client-IP"))
	add(c.GetHeader("X-Real-IP"))

	for _, value := range strings.Split(c.GetHeader("X-Forwarded-For"), ",") {
		add(value)
	}

	return candidates
}
