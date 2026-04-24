package chatgptimg

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// forceH1 avoids HTTP/2 JA4H fingerprint mismatches by only negotiating HTTP/1.1.
var forceH1 = true

func NewUTLSTransport(proxyURL string, idleTimeout time.Duration) (http.RoundTripper, error) {
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Second
	}
	rt := &utlsRoundTripper{
		dialer:      &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second},
		idleTimeout: idleTimeout,
	}
	if strings.TrimSpace(proxyURL) != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy url: %w", err)
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			rt.proxyURL = u
		case "socks5", "socks5h":
			return nil, fmt.Errorf("socks5 proxy is not supported by utls transport yet")
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
		}
	}
	rt.h1 = &http.Transport{
		DialTLSContext:        rt.dialTLS,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       idleTimeout,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     false,
	}
	rt.h2 = &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return rt.dialTLS(ctx, network, addr)
		},
		ReadIdleTimeout: idleTimeout,
		AllowHTTP:       false,
	}
	return rt, nil
}

type utlsRoundTripper struct {
	proxyURL    *url.URL
	dialer      *net.Dialer
	idleTimeout time.Duration

	mu sync.Mutex
	h1 *http.Transport
	h2 *http2.Transport
}

func (rt *utlsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if forceH1 {
		return rt.h1.RoundTrip(req)
	}
	resp, err := rt.h2.RoundTrip(req)
	if err == nil {
		return resp, nil
	}
	if isH2Retryable(err) {
		return rt.h1.RoundTrip(req)
	}
	return nil, err
}

func (rt *utlsRoundTripper) CloseIdleConnections() {
	rt.mu.Lock()
	h1, h2 := rt.h1, rt.h2
	rt.mu.Unlock()
	if h1 != nil {
		h1.CloseIdleConnections()
	}
	if h2 != nil {
		h2.CloseIdleConnections()
	}
}

func (rt *utlsRoundTripper) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	raw, err := rt.dialRaw(ctx, addr)
	if err != nil {
		return nil, err
	}
	alpn := []string{"h2", "http/1.1"}
	if forceH1 {
		alpn = []string{"http/1.1"}
	}
	uconn := utls.UClient(raw, &utls.Config{
		ServerName: host,
		NextProtos: alpn,
		MinVersion: tls.VersionTLS12,
	}, utls.HelloChrome_131)
	if forceH1 {
		if err := uconn.BuildHandshakeState(); err != nil {
			_ = raw.Close()
			return nil, fmt.Errorf("utls build state: %w", err)
		}
		for _, ext := range uconn.Extensions {
			if alpnExt, ok := ext.(*utls.ALPNExtension); ok {
				alpnExt.AlpnProtocols = []string{"http/1.1"}
			}
		}
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("utls handshake %s: %w", host, err)
	}
	if forceH1 {
		np := uconn.ConnectionState().NegotiatedProtocol
		if np != "" && np != "http/1.1" {
			_ = uconn.Close()
			return nil, fmt.Errorf("alpn negotiated %q, expected http/1.1", np)
		}
	}
	return uconn, nil
}

func (rt *utlsRoundTripper) dialRaw(ctx context.Context, addr string) (net.Conn, error) {
	if rt.proxyURL == nil {
		return rt.dialer.DialContext(ctx, "tcp", addr)
	}
	proxyAddr := rt.proxyURL.Host
	if !strings.Contains(proxyAddr, ":") {
		if strings.EqualFold(rt.proxyURL.Scheme, "https") {
			proxyAddr += ":443"
		} else {
			proxyAddr += ":80"
		}
	}
	conn, err := rt.dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dial proxy %s: %w", proxyAddr, err)
	}
	if strings.EqualFold(rt.proxyURL.Scheme, "https") {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: rt.proxyURL.Hostname()})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("tls handshake to https proxy: %w", err)
		}
		conn = tlsConn
	}
	connectReq := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: addr}, Host: addr, Header: make(http.Header)}
	connectReq.Header.Set("User-Agent", defaultUserAgent)
	if u := rt.proxyURL.User; u != nil {
		pw, _ := u.Password()
		connectReq.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(u.Username()+":"+pw)))
	}
	if err := connectReq.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write CONNECT: %w", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, connectReq)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy CONNECT %s → %s", addr, resp.Status)
	}
	if n := br.Buffered(); n > 0 {
		peeked, _ := br.Peek(n)
		return &bufConn{Conn: conn, rd: bufio.NewReaderSize(io.MultiReader(peeked2Reader(peeked), conn), 4096)}, nil
	}
	return conn, nil
}

func isH2Retryable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP_1_1_REQUIRED") ||
		strings.Contains(s, "http2: unsupported scheme") ||
		strings.Contains(s, "bad protocol") ||
		strings.Contains(s, "remote error: tls: no application protocol") ||
		strings.Contains(s, "http2: server sent GOAWAY") ||
		errors.Is(err, http2.ErrNoCachedConn)
}

type bufConn struct {
	net.Conn
	rd *bufio.Reader
}

func (b *bufConn) Read(p []byte) (int, error) { return b.rd.Read(p) }

func peeked2Reader(peeked []byte) io.Reader {
	return &readOnceBuf{buf: peeked}
}

type readOnceBuf struct {
	buf []byte
	off int
}

func (r *readOnceBuf) Read(p []byte) (int, error) {
	if r.off >= len(r.buf) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}
