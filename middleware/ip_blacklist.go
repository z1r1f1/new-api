package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type ipAutoBanCounter struct {
	windowMinute int64
	count        int
}

var (
	ipAutoBanMu           sync.Mutex
	ipAutoBanCounters     = make(map[string]ipAutoBanCounter)
	ipAutoBanNow          = time.Now
	updateIPBlacklistList = model.UpdateOption
)

func IPBlacklist() gin.HandlerFunc {
	return func(c *gin.Context) {
		setting := system_setting.GetIPBlacklistSetting()
		if !setting.Enabled && !setting.AutoBanEnabled {
			c.Next()
			return
		}

		clientIPs := blacklistClientIPCandidates(c)
		blacklist := common.SplitIPList(setting.List)
		if setting.Enabled && len(blacklist) > 0 && len(clientIPs) == 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "无法解析客户端 IP 地址",
			})
			c.Abort()
			return
		}

		if len(blacklist) > 0 {
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
		}

		if autoBanIPIfNeeded(c, setting, clientIPs) {
			return
		}

		c.Next()
	}
}

type blacklistClientIP struct {
	source string
	raw    string
	ip     net.IP
}

func blacklistClientIPCandidates(c *gin.Context) []blacklistClientIP {
	candidates := make([]blacklistClientIP, 0, 8)
	seen := make(map[string]struct{})

	add := func(source string, value string) {
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
			source: source,
			raw:    value,
			ip:     ip,
		})
	}

	add("client_ip", c.ClientIP())
	add("remote_addr", c.Request.RemoteAddr)
	add("cf_connecting_ip", c.GetHeader("CF-Connecting-IP"))
	add("true_client_ip", c.GetHeader("True-Client-IP"))
	add("x_real_ip", c.GetHeader("X-Real-IP"))

	for _, value := range strings.Split(c.GetHeader("X-Forwarded-For"), ",") {
		add("x_forwarded_for", value)
	}

	return candidates
}

func autoBanIPIfNeeded(c *gin.Context, setting *system_setting.IPBlacklistSetting, candidates []blacklistClientIP) bool {
	if !setting.AutoBanEnabled || setting.AutoBanRpm <= 0 || len(candidates) == 0 {
		return false
	}

	clientIP, ok := selectAutoBanClientIP(candidates, common.SplitIPList(setting.AutoBanWhitelist))
	if !ok {
		return false
	}

	if !recordAutoBanHit(clientIP.ip.String(), setting.AutoBanRpm) {
		return false
	}

	if addAutoBannedIP(setting, clientIP.ip.String()) {
		logger.LogWarn(c, "auto-banned high RPM IP "+clientIP.raw+" from "+clientIP.source)
	}
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "当前 IP 请求频率过高，已被自动封禁",
	})
	c.Abort()
	return true
}

func selectAutoBanClientIP(candidates []blacklistClientIP, whitelist []string) (blacklistClientIP, bool) {
	for _, candidate := range candidates {
		if common.IsIpInCIDRList(candidate.ip, whitelist) {
			continue
		}
		return candidate, true
	}
	return blacklistClientIP{}, false
}

func recordAutoBanHit(ip string, threshold int) bool {
	nowMinute := ipAutoBanNow().Unix() / 60

	ipAutoBanMu.Lock()
	defer ipAutoBanMu.Unlock()

	counter := ipAutoBanCounters[ip]
	if counter.windowMinute != nowMinute {
		counter = ipAutoBanCounter{windowMinute: nowMinute}
	}
	counter.count++
	ipAutoBanCounters[ip] = counter

	if counter.count%256 == 0 {
		for key, value := range ipAutoBanCounters {
			if value.windowMinute < nowMinute-1 {
				delete(ipAutoBanCounters, key)
			}
		}
	}

	return counter.count >= threshold
}

func addAutoBannedIP(setting *system_setting.IPBlacklistSetting, ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	ipAutoBanMu.Lock()
	defer ipAutoBanMu.Unlock()

	blacklist := common.SplitIPList(setting.List)
	if common.IsIpInCIDRList(parsedIP, blacklist) {
		return false
	}

	normalizedIP := parsedIP.String()
	if strings.TrimSpace(setting.List) == "" {
		setting.List = normalizedIP
	} else {
		setting.List = strings.TrimRight(setting.List, " \t\r\n,;") + "\n" + normalizedIP
	}

	if err := updateIPBlacklistList("ip_blacklist_setting.list", setting.List); err != nil {
		common.SysError("failed to persist auto-banned IP: " + err.Error())
	}
	return true
}
