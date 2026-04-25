package relay

import (
	"fmt"
	"strings"
)

const maxRequestBodyLogBytes = 32 * 1024

func sanitizedRequestBodyForLog(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(min(len(body), maxRequestBodyLogBytes))
	truncated := false
	for i := 0; i < len(body); {
		if b.Len() >= maxRequestBodyLogBytes {
			truncated = true
			break
		}
		if hasDataURLPrefix(body, i) {
			next, replacement := redactDataURLForLog(body, i)
			b.WriteString(replacement)
			i = next
			continue
		}
		b.WriteByte(body[i])
		i++
	}
	if truncated {
		b.WriteString(fmt.Sprintf("...[truncated request body: %d bytes]", len(body)))
	}
	return b.String()
}

func hasDataURLPrefix(body []byte, index int) bool {
	if index+5 > len(body) {
		return false
	}
	return string(body[index:index+5]) == "data:"
}

func redactDataURLForLog(body []byte, start int) (int, string) {
	end := start
	for end < len(body) && body[end] != '"' && body[end] != '\\' && body[end] != '\n' && body[end] != '\r' {
		end++
	}
	token := string(body[start:end])
	if !strings.Contains(token, ";base64,") {
		return start + 5, "data:"
	}
	mime := "image"
	if semi := strings.Index(token, ";"); semi > len("data:") {
		mime = token[len("data:"):semi]
	}
	return end, fmt.Sprintf("data:%s;base64,[redacted %d bytes]", mime, end-start)
}
