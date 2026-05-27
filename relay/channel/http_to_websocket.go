package channel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// DoHTTPToWebsocketRequest adapts the normal HTTP relay request shape into a
// simple websocket upstream transport:
//   - the upstream URL is derived from the adaptor request URL by rewriting
//     http/https to ws/wss;
//   - the converted upstream JSON body is sent as one websocket text message;
//   - stream responses are exposed back to the existing relay response handlers
//     as SSE data frames, while non-stream responses return the first upstream
//     websocket message as the HTTP response body.
func DoHTTPToWebsocketRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	fullRequestURL, err = httpURLToWebsocketURL(fullRequestURL)
	if err != nil {
		return nil, err
	}

	requestBody, _, err = capturePlaygroundUpstreamRequestBody(c, requestBody)
	if err != nil {
		return nil, err
	}

	targetHeader := http.Header{}
	if err := a.SetupRequestHeader(c, &targetHeader, info); err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	headerOverride, err := processHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		targetHeader.Set(key, value)
	}
	if c != nil && c.Request != nil {
		if contentType := strings.TrimSpace(c.Request.Header.Get("Content-Type")); contentType != "" {
			targetHeader.Set("Content-Type", contentType)
		}
	}

	dialer := *websocket.DefaultDialer
	if info != nil && strings.TrimSpace(info.ChannelSetting.Proxy) != "" {
		proxyURL, err := url.Parse(strings.TrimSpace(info.ChannelSetting.Proxy))
		if err != nil {
			return nil, fmt.Errorf("parse websocket proxy failed: %w", err)
		}
		dialer.Proxy = http.ProxyURL(proxyURL)
	}
	targetConn, handshakeResp, err := dialer.DialContext(ginRequestContext(c), fullRequestURL, targetHeader)
	if err != nil {
		if handshakeResp != nil && handshakeResp.Body != nil {
			_ = handshakeResp.Body.Close()
		}
		return nil, fmt.Errorf("dial websocket failed to %s: %w", fullRequestURL, err)
	}

	if requestBody != nil {
		if err := writeWebsocketRequestBody(targetConn, requestBody); err != nil {
			_ = targetConn.Close()
			return nil, err
		}
	}

	responseHeader := http.Header{}
	if info != nil && info.IsStream {
		responseHeader.Set("Content-Type", "text/event-stream")
		responseHeader.Set("Cache-Control", "no-cache")
	} else {
		responseHeader.Set("Content-Type", "application/json")
	}

	pr, pw := io.Pipe()
	go pipeWebsocketUpstreamResponse(targetConn, pw, info != nil && info.IsStream)

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
		Header:     responseHeader,
		Body:       pr,
	}, nil
}

func httpURLToWebsocketURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse websocket request url failed: %w", err)
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported websocket conversion url scheme %q", parsed.Scheme)
	}
	return parsed.String(), nil
}

func writeWebsocketRequestBody(conn *websocket.Conn, requestBody io.Reader) error {
	writer, err := conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return fmt.Errorf("open websocket request writer failed: %w", err)
	}
	if _, err := io.Copy(writer, requestBody); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write websocket request body failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("flush websocket request body failed: %w", err)
	}
	return nil
}

func pipeWebsocketUpstreamResponse(conn *websocket.Conn, writer *io.PipeWriter, stream bool) {
	defer conn.Close()

	closeWithError := func(err error) {
		if err == nil {
			_ = writer.Close()
			return
		}
		_ = writer.CloseWithError(err)
	}

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if errors.Is(err, io.EOF) || websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				closeWithError(nil)
				return
			}
			closeWithError(fmt.Errorf("read websocket upstream response failed: %w", err))
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		var outbound []byte
		if stream {
			outbound = websocketPayloadToSSEFrame(payload)
		} else {
			outbound = payload
		}
		if len(outbound) > 0 {
			if _, err := writer.Write(outbound); err != nil {
				closeWithError(err)
				return
			}
		}
		if !stream || isWebsocketDonePayload(payload) {
			closeWithError(nil)
			return
		}
	}
}

func websocketPayloadToSSEFrame(payload []byte) []byte {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) ||
		bytes.HasPrefix(trimmed, []byte("event:")) ||
		bytes.HasPrefix(trimmed, []byte(":")) {
		return ensureDoubleNewline(bytes.TrimRight(payload, "\r\n"))
	}
	return []byte("data: " + string(bytes.TrimRight(payload, "\r\n")) + "\n\n")
}

func ensureDoubleNewline(payload []byte) []byte {
	if bytes.HasSuffix(payload, []byte("\n\n")) {
		return payload
	}
	if bytes.HasSuffix(payload, []byte("\n")) {
		return append(payload, '\n')
	}
	return append(payload, '\n', '\n')
}

func isWebsocketDonePayload(payload []byte) bool {
	trimmed := strings.TrimSpace(string(payload))
	return trimmed == "[DONE]" || trimmed == "data: [DONE]"
}
