package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// auth-related header names that should not be stored in logs.
var sensitiveHeaderNames = map[string]bool{
	"authorization":          true,
	"proxy-authorization":    true,
	"api-key":                true,
	"new-api-user":           true,
	"x-api-key":              true,
	"x-goog-api-key":         true,
	"mj-api-secret":          true,
	"sec-websocket-protocol": true,
	"cookie":                 true,
	"set-cookie":             true,
}

// CleanRequestHeadersForLog returns a copy of the request headers map with
// authentication-related headers removed. Returns nil when the input is empty
// or when all headers are filtered out.
func CleanRequestHeadersForLog(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	filtered := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		if key == "" || value == "" || isSensitiveRequestHeaderForLog(key) {
			continue
		}
		filtered[key] = value
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func isSensitiveRequestHeaderForLog(header string) bool {
	name := strings.ToLower(strings.TrimSpace(header))
	if sensitiveHeaderNames[name] {
		return true
	}
	return strings.Contains(name, "authorization") ||
		strings.Contains(name, "api-key") ||
		strings.Contains(name, "secret") ||
		strings.Contains(name, "token")
}

func safeRequestHeadersFromContext(ctx *gin.Context) map[string]string {
	if ctx == nil || ctx.Request == nil || len(ctx.Request.Header) == 0 {
		return nil
	}
	headers := make(map[string]string, len(ctx.Request.Header))
	for key := range ctx.Request.Header {
		value := strings.TrimSpace(ctx.Request.Header.Get(key))
		if value == "" {
			continue
		}
		headers[key] = value
	}
	return CleanRequestHeadersForLog(headers)
}

// AppendRequestHeadersAdminInfo records non-sensitive request headers in the
// admin-only log payload. Authentication headers, cookies, token-like headers,
// and API-key/secret headers are intentionally excluded.
func AppendRequestHeadersAdminInfo(ctx *gin.Context, adminInfo map[string]interface{}) {
	if adminInfo == nil {
		return
	}
	if headers := safeRequestHeadersFromContext(ctx); headers != nil {
		adminInfo["request_headers"] = headers
	}
}
