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

const redactedRequestHeaderValue = "[REDACTED]"

// CleanRequestHeadersForLog returns a copy of the request headers map with
// authentication-related values redacted. Header names are retained so admins
// can see the complete request-header shape without storing usable secrets.
// Returns nil when the input is empty or contains no valid header names.
func CleanRequestHeadersForLog(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	filtered := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		if isSensitiveRequestHeaderForLog(key) {
			value = redactedRequestHeaderValue
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
	if ctx == nil || ctx.Request == nil {
		return nil
	}
	headers := make(map[string]string, len(ctx.Request.Header)+1)
	if host := strings.TrimSpace(ctx.Request.Host); host != "" {
		headers["Host"] = host
	}
	for key, values := range ctx.Request.Header {
		cleanValues := make([]string, 0, len(values))
		for _, value := range values {
			cleanValues = append(cleanValues, strings.TrimSpace(value))
		}
		headers[key] = strings.Join(cleanValues, ", ")
	}
	return CleanRequestHeadersForLog(headers)
}

// AppendRequestHeadersAdminInfo records request headers in the admin-only log
// payload. Authentication headers, cookies, token-like headers, and API-key or
// secret headers retain their names but use a redacted value.
func AppendRequestHeadersAdminInfo(ctx *gin.Context, adminInfo map[string]any) {
	if adminInfo == nil {
		return
	}
	if headers := safeRequestHeadersFromContext(ctx); headers != nil {
		adminInfo["request_headers"] = headers
	}
}
