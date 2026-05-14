package chatgptimg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/google/uuid"
	"golang.org/x/net/publicsuffix"
)

const (
	defaultUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	defaultClientVersion  = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
	defaultClientBuildNum = "5955942"
	defaultLanguage       = "zh-CN"
	defaultBaseURL        = "https://chatgpt.com"

	defaultOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
)

type ClientOptions struct {
	BaseURL       string
	AuthToken     string
	DeviceID      string
	SessionID     string
	ProxyURL      string
	Timeout       time.Duration
	SSETimeout    time.Duration
	UserAgent     string
	ClientVersion string
	Language      string
}

type Client struct {
	opts ClientOptions
	hc   *http.Client
}

func NewClient(opt ClientOptions) (*Client, error) {
	if strings.TrimSpace(opt.AuthToken) == "" {
		return nil, errors.New("chatgpt web channel: auth_token required")
	}
	if strings.TrimSpace(opt.DeviceID) == "" {
		opt.DeviceID = uuid.NewString()
	}
	if opt.BaseURL == "" {
		opt.BaseURL = defaultBaseURL
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 120 * time.Second
	}
	if opt.SSETimeout <= 0 {
		opt.SSETimeout = 120 * time.Second
	}
	if opt.UserAgent == "" {
		opt.UserAgent = defaultUserAgent
	}
	if opt.ClientVersion == "" {
		opt.ClientVersion = defaultClientVersion
	}
	if opt.Language == "" {
		opt.Language = defaultLanguage
	}
	opt.ProxyURL = effectiveProxyURL(opt.ProxyURL, opt.BaseURL)
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	var httpClient http.Client
	transport, err := NewUTLSTransport(strings.TrimSpace(opt.ProxyURL), 30*time.Second)
	if err == nil {
		httpClient = http.Client{
			Transport: transport,
			Timeout:   opt.Timeout,
			Jar:       jar,
		}
	} else {
		baseClient, baseErr := service.GetHttpClientWithProxy(strings.TrimSpace(opt.ProxyURL))
		if baseErr != nil {
			return nil, fmt.Errorf("chatgpt web channel: init http client failed: utls=%v fallback=%w", err, baseErr)
		}
		if baseClient == nil {
			baseClient = &http.Client{}
		}
		httpClient = *baseClient
		httpClient.Timeout = opt.Timeout
		httpClient.Jar = jar
		if httpClient.Transport == nil {
			httpClient.Transport = &http.Transport{
				ForceAttemptHTTP2:   true,
				MaxIdleConns:        32,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 15 * time.Second,
			}
		}
	}
	return &Client{
		opts: opt,
		hc:   &httpClient,
	}, nil
}

func effectiveProxyURL(configuredProxyURL, baseURL string) string {
	configuredProxyURL = strings.TrimSpace(configuredProxyURL)
	if configuredProxyURL != "" {
		return configuredProxyURL
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	req, err := http.NewRequest(http.MethodGet, baseURL, nil)
	if err != nil {
		return ""
	}
	proxyURL, err := http.ProxyFromEnvironment(req)
	if err != nil || proxyURL == nil {
		return ""
	}
	return proxyURL.String()
}

func ResolveAccessToken(ctx context.Context, key *OAuthKey, proxyURL string) (string, error) {
	if key == nil {
		return "", errors.New("chatgpt web channel: oauth key is nil")
	}
	if accessTokenUsable(key.AccessToken) {
		return strings.TrimSpace(key.AccessToken), nil
	}
	client := buildExchangeHTTPClient(proxyURL)
	clientID := strings.TrimSpace(key.ClientID)
	if clientID == "" {
		clientID = defaultOAuthClientID
	}
	if strings.TrimSpace(key.RefreshToken) != "" {
		at, _, _, err := rtExchange(ctx, client, strings.TrimSpace(key.RefreshToken), clientID)
		if err == nil && strings.TrimSpace(at) != "" {
			return at, nil
		}
	}
	if strings.TrimSpace(key.SessionToken) != "" {
		at, _, err := stExchange(ctx, client, strings.TrimSpace(key.SessionToken))
		if err == nil && strings.TrimSpace(at) != "" {
			return at, nil
		}
	}
	if strings.TrimSpace(key.AccessToken) != "" {
		return strings.TrimSpace(key.AccessToken), nil
	}
	return "", errors.New("chatgpt web channel: no usable access token, please provide access_token or refresh/session token")
}

func accessTokenUsable(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	exp := parseJWTExp(token)
	if exp.IsZero() {
		return true
	}
	return time.Until(exp) > time.Minute
}

func buildExchangeHTTPClient(proxyURL string) *http.Client {
	client, err := service.GetHttpClientWithProxy(strings.TrimSpace(proxyURL))
	if err == nil && client != nil {
		copyClient := *client
		copyClient.Timeout = 30 * time.Second
		return &copyClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func parseJWTExp(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		raw, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := common.Unmarshal(raw, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func rtExchange(ctx context.Context, httpc *http.Client, refreshToken, clientID string) (newAT, newRT string, expAt time.Time, err error) {
	body := map[string]string{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"redirect_uri":  "com.openai.chat://auth0.openai.com/ios/com.openai.chat/callback",
		"refresh_token": refreshToken,
	}
	buf, _ := common.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://auth.openai.com/oauth/token", bytes.NewReader(buf))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ChatGPT/1.2025.122 (iOS 18.2; iPhone15,2; build 15096)")
	resp, err := httpc.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("rt exchange http=%d body=%s", resp.StatusCode, truncateString(string(data), 200))
		return
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err = common.Unmarshal(data, &out); err != nil {
		return
	}
	if out.AccessToken == "" {
		err = errors.New("rt exchange missing access_token")
		return
	}
	newAT = out.AccessToken
	newRT = out.RefreshToken
	if out.ExpiresIn > 0 {
		expAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	} else {
		expAt = parseJWTExp(newAT)
	}
	return
}

func stExchange(ctx context.Context, httpc *http.Client, sessionToken string) (newAT string, expAt time.Time, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/api/auth/session", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.AddCookie(&http.Cookie{Name: "__Secure-next-auth.session-token", Value: sessionToken})

	resp, err := httpc.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("st exchange http=%d body=%s", resp.StatusCode, truncateString(string(data), 200))
		return
	}
	if strings.TrimSpace(string(data)) == "" || strings.TrimSpace(string(data)) == "{}" {
		err = errors.New("session token is expired or invalid")
		return
	}
	var out struct {
		AccessToken string `json:"accessToken"`
		Expires     string `json:"expires"`
	}
	if err = common.Unmarshal(data, &out); err != nil {
		return
	}
	if out.AccessToken == "" {
		err = errors.New("session exchange missing accessToken")
		return
	}
	newAT = out.AccessToken
	if out.Expires != "" {
		if t, parseErr := time.Parse(time.RFC3339, out.Expires); parseErr == nil {
			expAt = t
		}
	}
	if expAt.IsZero() {
		expAt = parseJWTExp(newAT)
	}
	return
}

func (c *Client) commonHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.opts.AuthToken)
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Origin", c.opts.BaseURL)
	req.Header.Set("Referer", c.opts.BaseURL+"/")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6")
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Arch", `"x86"`)
	req.Header.Set("Sec-Ch-Ua-Bitness", `"64"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version", `"143.0.3650.96"`)
	req.Header.Set("Sec-Ch-Ua-Full-Version-List", `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Model", `""`)
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Ch-Ua-Platform-Version", `"19.0.0"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Oai-Device-Id", c.opts.DeviceID)
	if c.opts.SessionID != "" {
		req.Header.Set("Oai-Session-Id", c.opts.SessionID)
	}
	req.Header.Set("Oai-Language", c.opts.Language)
	req.Header.Set("Oai-Client-Version", c.opts.ClientVersion)
	req.Header.Set("Oai-Client-Build-Number", defaultClientBuildNum)
	if p := req.URL.Path; p != "" {
		req.Header.Set("X-Openai-Target-Path", p)
		req.Header.Set("X-Openai-Target-Route", p)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
}

type UpstreamError struct {
	Status  int
	Message string
	Body    string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("chatgpt upstream %d: %s", e.Status, e.Message)
}

func (e *UpstreamError) IsRateLimited() bool {
	return e != nil && e.Status == http.StatusTooManyRequests
}
func (e *UpstreamError) IsUnauthorized() bool {
	return e != nil && (e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden)
}

type ChatRequirementsResp struct {
	Token       string `json:"token"`
	Persona     string `json:"persona"`
	Proofofwork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
	Turnstile struct {
		Required bool `json:"required"`
	} `json:"turnstile"`
}

func (c *Client) Bootstrap(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opts.BaseURL+"/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode >= 400 {
		return &UpstreamError{Status: res.StatusCode, Message: "bootstrap failed"}
	}
	return nil
}

func (c *Client) ChatRequirements(ctx context.Context) (*ChatRequirementsResp, error) {
	_ = c.Bootstrap(ctx)
	reqToken := NewPOWConfig(c.opts.UserAgent).RequirementsToken()
	body, _ := common.Marshal(map[string]string{"p": reqToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/sentinel/chat-requirements", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res.StatusCode, Message: "chat-requirements failed", Body: string(buf)}
	}
	var out ChatRequirementsResp
	if err := common.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode chat-requirements: %w", err)
	}
	return &out, nil
}

type ChatRequirementsPrepareResp struct {
	Persona      string `json:"persona"`
	PrepareToken string `json:"prepare_token"`
	Turnstile    struct {
		Required bool   `json:"required"`
		DX       string `json:"dx"`
	} `json:"turnstile"`
	Proofofwork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
}

func (c *Client) ChatRequirementsPrepare(ctx context.Context) (*ChatRequirementsPrepareResp, error) {
	body, _ := common.Marshal(map[string]string{"p": NewPOWConfig(c.opts.UserAgent).RequirementsToken()})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/sentinel/chat-requirements/prepare", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res.StatusCode, Message: "chat-requirements/prepare failed", Body: string(buf)}
	}
	var out ChatRequirementsPrepareResp
	if err := common.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode chat-requirements/prepare: %w", err)
	}
	return &out, nil
}

func (c *Client) ChatRequirementsFinalize(ctx context.Context, prepareToken, proofToken string) (string, string, error) {
	payload := map[string]any{"prepare_token": prepareToken}
	if proofToken != "" {
		payload["proofofwork"] = proofToken
	}
	body, _ := common.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/sentinel/chat-requirements/finalize", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", "", &UpstreamError{Status: res.StatusCode, Message: "chat-requirements/finalize failed", Body: string(buf)}
	}
	var out struct {
		Persona string `json:"persona"`
		Token   string `json:"token"`
	}
	if err := common.Unmarshal(buf, &out); err != nil {
		return "", "", fmt.Errorf("decode chat-requirements/finalize: %w", err)
	}
	return out.Token, out.Persona, nil
}

func (c *Client) ChatRequirementsV2(ctx context.Context) (*ChatRequirementsResp, error) {
	prep, err := c.ChatRequirementsPrepare(ctx)
	if err != nil {
		return c.ChatRequirements(ctx)
	}
	if prep.Turnstile.Required {
		return c.ChatRequirements(ctx)
	}
	resp := &ChatRequirementsResp{Persona: prep.Persona}
	resp.Turnstile.Required = prep.Turnstile.Required
	resp.Proofofwork.Required = prep.Proofofwork.Required
	resp.Proofofwork.Seed = prep.Proofofwork.Seed
	resp.Proofofwork.Difficulty = prep.Proofofwork.Difficulty
	proofToken := ""
	if prep.Proofofwork.Required {
		proofToken = SolveProofToken(prep.Proofofwork.Seed, prep.Proofofwork.Difficulty, c.opts.UserAgent)
	}
	token, persona, err := c.ChatRequirementsFinalize(ctx, prep.PrepareToken, proofToken)
	if err != nil {
		return c.ChatRequirements(ctx)
	}
	resp.Token = token
	if persona != "" {
		resp.Persona = persona
	}
	return resp, nil
}

type ImageConvOpts struct {
	Prompt         string
	UpstreamModel  string
	ConvID         string
	ParentMsgID    string
	MessageID      string
	ChatToken      string
	ProofToken     string
	ConduitToken   string
	TimezoneOffset int
	SSETimeout     time.Duration
	References     []*UploadedFile
}

type ChatConvOpts struct {
	Prompt         string
	UpstreamModel  string
	ConvID         string
	ParentMsgID    string
	MessageID      string
	ChatToken      string
	ProofToken     string
	ConduitToken   string
	TimezoneOffset int
	SSETimeout     time.Duration
}

func (c *Client) PrepareFConversation(ctx context.Context, opt ImageConvOpts) (string, error) {
	if opt.UpstreamModel == "" {
		opt.UpstreamModel = "auto"
	}
	if opt.MessageID == "" {
		opt.MessageID = uuid.NewString()
	}
	payload := map[string]any{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     opt.ParentMsgID,
		"model":                 opt.UpstreamModel,
		"client_prepare_state":  "success",
		"timezone_offset_min":   -480,
		"timezone":              "Asia/Shanghai",
		"conversation_mode":     map[string]string{"kind": "primary_assistant"},
		"system_hints":          []string{"picture_v2"},
		"partial_query": map[string]any{
			"id":     uuid.NewString(),
			"author": map[string]string{"role": "user"},
			"content": map[string]any{
				"content_type": "text",
				"parts":        []string{opt.Prompt},
			},
		},
		"supports_buffering":  true,
		"supported_encodings": []string{"v1"},
		"client_contextual_info": map[string]any{
			"app_name": "chatgpt.com",
		},
	}
	if opt.ConvID != "" {
		payload["conversation_id"] = opt.ConvID
	}
	body, _ := common.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/f/conversation/prepare", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Openai-Sentinel-Chat-Requirements-Token", opt.ChatToken)
	if opt.ProofToken != "" {
		req.Header.Set("Openai-Sentinel-Proof-Token", opt.ProofToken)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", &UpstreamError{Status: res.StatusCode, Message: "f/conversation/prepare failed", Body: string(buf)}
	}
	var out struct {
		ConduitToken string `json:"conduit_token"`
	}
	_ = common.Unmarshal(buf, &out)
	return out.ConduitToken, nil
}

func (c *Client) PrepareChatConversation(ctx context.Context, opt ChatConvOpts) (string, error) {
	if opt.UpstreamModel == "" {
		opt.UpstreamModel = "auto"
	}
	if opt.MessageID == "" {
		opt.MessageID = uuid.NewString()
	}
	payload := map[string]any{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     opt.ParentMsgID,
		"model":                 opt.UpstreamModel,
		"client_prepare_state":  "success",
		"timezone_offset_min":   -480,
		"timezone":              "Asia/Shanghai",
		"conversation_mode":     map[string]string{"kind": "primary_assistant"},
		"partial_query": map[string]any{
			"id":     uuid.NewString(),
			"author": map[string]string{"role": "user"},
			"content": map[string]any{
				"content_type": "text",
				"parts":        []string{opt.Prompt},
			},
		},
		"supports_buffering":  true,
		"supported_encodings": []string{"v1"},
		"client_contextual_info": map[string]any{
			"app_name": "chatgpt.com",
		},
	}
	if opt.ConvID != "" {
		payload["conversation_id"] = opt.ConvID
	}
	body, _ := common.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/f/conversation/prepare", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Openai-Sentinel-Chat-Requirements-Token", opt.ChatToken)
	if opt.ProofToken != "" {
		req.Header.Set("Openai-Sentinel-Proof-Token", opt.ProofToken)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", &UpstreamError{Status: res.StatusCode, Message: "f/conversation/prepare chat failed", Body: string(buf)}
	}
	var out struct {
		ConduitToken string `json:"conduit_token"`
	}
	_ = common.Unmarshal(buf, &out)
	return out.ConduitToken, nil
}

func (c *Client) StreamFConversation(ctx context.Context, opt ImageConvOpts) (<-chan SSEEvent, error) {
	if opt.UpstreamModel == "" {
		opt.UpstreamModel = "auto"
	}
	if opt.MessageID == "" {
		opt.MessageID = uuid.NewString()
	}
	if opt.ParentMsgID == "" {
		opt.ParentMsgID = uuid.NewString()
	}
	if opt.TimezoneOffset == 0 {
		opt.TimezoneOffset = -480
	}
	if opt.SSETimeout == 0 {
		opt.SSETimeout = 180 * time.Second
	}

	msgContent := map[string]any{"content_type": "text", "parts": []string{opt.Prompt}}
	msgMeta := map[string]any{
		"developer_mode_connector_ids": []any{},
		"selected_github_repos":        []any{},
		"selected_all_github_repos":    false,
		"system_hints":                 []string{"picture_v2"},
		"serialization_metadata": map[string]any{
			"custom_symbol_offsets": []any{},
		},
	}
	if len(opt.References) > 0 {
		parts := make([]any, 0, len(opt.References)+1)
		attachments := make([]Attachment, 0, len(opt.References))
		for _, ref := range opt.References {
			if ref == nil || ref.FileID == "" {
				continue
			}
			parts = append(parts, ref.ToAssetPointerPart())
			attachments = append(attachments, ref.ToAttachment())
		}
		parts = append(parts, opt.Prompt)
		msgContent = map[string]any{
			"content_type": "multimodal_text",
			"parts":        parts,
		}
		msgMeta["attachments"] = attachments
	}

	payload := map[string]any{
		"action": "next",
		"messages": []map[string]any{{
			"id":          opt.MessageID,
			"author":      map[string]string{"role": "user"},
			"create_time": float64(time.Now().UnixMilli()) / 1000.0,
			"content":     msgContent,
			"metadata":    msgMeta,
		}},
		"parent_message_id":        opt.ParentMsgID,
		"model":                    opt.UpstreamModel,
		"client_prepare_state":     "sent",
		"timezone_offset_min":      opt.TimezoneOffset,
		"timezone":                 "Asia/Shanghai",
		"conversation_mode":        map[string]string{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":             []string{"picture_v2"},
		"supports_buffering":       true,
		"supported_encodings":      []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 1200,
			"page_height":       1072,
			"page_width":        1724,
			"pixel_ratio":       1.2,
			"screen_height":     1440,
			"screen_width":      2560,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}
	if opt.ConvID != "" {
		payload["conversation_id"] = opt.ConvID
	}
	body, _ := common.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/f/conversation", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	req.Header.Set("Openai-Sentinel-Chat-Requirements-Token", opt.ChatToken)
	if opt.ProofToken != "" {
		req.Header.Set("Openai-Sentinel-Proof-Token", opt.ProofToken)
	}
	if opt.ConduitToken != "" {
		req.Header.Set("X-Conduit-Token", opt.ConduitToken)
	}
	local := *c.hc
	local.Timeout = 0
	res, err := local.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		buf, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, &UpstreamError{Status: res.StatusCode, Message: "f/conversation failed", Body: string(buf)}
	}
	out := make(chan SSEEvent, 64)
	go parseSSE(res.Body, out)
	return out, nil
}

func (c *Client) StreamChatConversation(ctx context.Context, opt ChatConvOpts) (<-chan SSEEvent, error) {
	if opt.UpstreamModel == "" {
		opt.UpstreamModel = "auto"
	}
	if opt.MessageID == "" {
		opt.MessageID = uuid.NewString()
	}
	if opt.ParentMsgID == "" {
		opt.ParentMsgID = uuid.NewString()
	}
	if opt.TimezoneOffset == 0 {
		opt.TimezoneOffset = -480
	}
	if opt.SSETimeout == 0 {
		opt.SSETimeout = 300 * time.Second
	}

	payload := map[string]any{
		"action": "next",
		"messages": []map[string]any{{
			"id":          opt.MessageID,
			"author":      map[string]string{"role": "user"},
			"create_time": float64(time.Now().UnixMilli()) / 1000.0,
			"content": map[string]any{
				"content_type": "text",
				"parts":        []string{opt.Prompt},
			},
			"metadata": map[string]any{
				"developer_mode_connector_ids": []any{},
				"selected_github_repos":        []any{},
				"selected_all_github_repos":    false,
				"serialization_metadata": map[string]any{
					"custom_symbol_offsets": []any{},
				},
			},
		}},
		"parent_message_id":        opt.ParentMsgID,
		"model":                    opt.UpstreamModel,
		"client_prepare_state":     "sent",
		"timezone_offset_min":      opt.TimezoneOffset,
		"timezone":                 "Asia/Shanghai",
		"conversation_mode":        map[string]string{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"supports_buffering":       true,
		"supported_encodings":      []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 1200,
			"page_height":       1072,
			"page_width":        1724,
			"pixel_ratio":       1.2,
			"screen_height":     1440,
			"screen_width":      2560,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}
	if opt.ConvID != "" {
		payload["conversation_id"] = opt.ConvID
	}
	body, _ := common.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/f/conversation", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	req.Header.Set("Openai-Sentinel-Chat-Requirements-Token", opt.ChatToken)
	if opt.ProofToken != "" {
		req.Header.Set("Openai-Sentinel-Proof-Token", opt.ProofToken)
	}
	if opt.ConduitToken != "" {
		req.Header.Set("X-Conduit-Token", opt.ConduitToken)
	}
	local := *c.hc
	local.Timeout = 0
	res, err := local.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		buf, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, &UpstreamError{Status: res.StatusCode, Message: "f/conversation chat failed", Body: string(buf)}
	}
	out := make(chan SSEEvent, 64)
	go parseSSE(res.Body, out)
	return out, nil
}

type SSEEvent struct {
	Event string
	Data  []byte
	Err   error
}

func parseSSE(r io.ReadCloser, out chan<- SSEEvent) {
	defer r.Close()
	defer close(out)

	rd := bufio.NewReaderSize(r, 32*1024)
	var event string
	var dataBuf strings.Builder
	flush := func() {
		if dataBuf.Len() == 0 {
			event = ""
			return
		}
		data := strings.TrimRight(dataBuf.String(), "\n")
		dataBuf.Reset()
		out <- SSEEvent{Event: event, Data: []byte(data)}
		event = ""
	}
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				out <- SSEEvent{Err: fmt.Errorf("sse read: %w", err)}
			} else {
				flush()
			}
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			s := strings.TrimPrefix(line, "data:")
			if len(s) > 0 && s[0] == ' ' {
				s = s[1:]
			}
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(s)
		}
	}
}

type ImageSSEResult struct {
	ConversationID string
	FileIDs        []string
	SedimentIDs    []string
	FinishType     string
	ImageGenTaskID string
	Content        string
	Err            error
}

type ChatSSEResult struct {
	ConversationID     string
	Content            string
	FinishType         string
	HasImageGeneration bool
	HasInlineImage     bool
	Err                error
}

type ChatSSEState struct {
	ConversationID     string
	Content            string
	FinishType         string
	IsAppendingText    bool
	HasImageGeneration bool
	HasInlineImage     bool
}

var (
	reFileRef           = regexp.MustCompile(`file-service://([A-Za-z0-9_-]+)`)
	reSedRef            = regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
	reMarkdownDataImage = regexp.MustCompile(`!\[([^\]]*)]\((data:image/[A-Za-z0-9.+-]+;base64,[A-Za-z0-9+/=\r\n]+)\)`)
)

const imageGenerationUpstreamErrorText = "We experienced an error when generating images"

func containsImageGenerationUpstreamErrorText(text string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(imageGenerationUpstreamErrorText))
}

func imageGenerationUpstreamError() error {
	return fmt.Errorf("chatgpt web channel: upstream image generation failed: %s", imageGenerationUpstreamErrorText)
}

type noRelayRetryError struct {
	err        error
	statusCode int
	skipRetry  bool
}

func (e *noRelayRetryError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	if e.statusCode > 0 {
		return fmt.Sprintf("HTTP %d: %s", e.statusCode, e.err.Error())
	}
	return e.err.Error()
}

func (e *noRelayRetryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *noRelayRetryError) SkipRelayRetry() bool {
	return e != nil && e.skipRetry
}

func (e *noRelayRetryError) RelayStatusCode() int {
	if e == nil || e.statusCode <= 0 {
		return http.StatusInternalServerError
	}
	return e.statusCode
}

func noRelayRetry(err error, statusCode int) error {
	if err == nil {
		return nil
	}
	return &noRelayRetryError{err: err, statusCode: statusCode, skipRetry: true}
}

func relayStatusError(err error, statusCode int) error {
	if err == nil {
		return nil
	}
	return &noRelayRetryError{err: err, statusCode: statusCode}
}

func ParseChatSSE(stream <-chan SSEEvent) ChatSSEResult {
	state := &ChatSSEState{}
	for ev := range stream {
		_, done, err := CollectChatSSEEvent(ev, state)
		if err != nil {
			return ChatSSEResult{
				ConversationID: state.ConversationID,
				Content:        state.Content,
				FinishType:     state.FinishType,
				Err:            err,
			}
		}
		if done {
			break
		}
	}
	return ChatSSEResult{
		ConversationID:     state.ConversationID,
		Content:            state.Content,
		FinishType:         state.FinishType,
		HasImageGeneration: state.HasImageGeneration,
		HasInlineImage:     state.HasInlineImage,
	}
}

func ParseChatSSEUntilReady(stream <-chan SSEEvent, quietAfterReady time.Duration) ChatSSEResult {
	state := &ChatSSEState{}
	var quietTimer <-chan time.Time
	for {
		select {
		case ev, ok := <-stream:
			if !ok {
				return ChatSSEResult{
					ConversationID:     state.ConversationID,
					Content:            state.Content,
					FinishType:         state.FinishType,
					HasImageGeneration: state.HasImageGeneration,
					HasInlineImage:     state.HasInlineImage,
				}
			}
			_, done, err := CollectChatSSEEvent(ev, state)
			if err != nil {
				return ChatSSEResult{
					ConversationID:     state.ConversationID,
					Content:            state.Content,
					FinishType:         state.FinishType,
					HasImageGeneration: state.HasImageGeneration,
					HasInlineImage:     state.HasInlineImage,
					Err:                err,
				}
			}
			if done {
				return ChatSSEResult{
					ConversationID:     state.ConversationID,
					Content:            state.Content,
					FinishType:         state.FinishType,
					HasImageGeneration: state.HasImageGeneration,
					HasInlineImage:     state.HasInlineImage,
				}
			}
			if strings.TrimSpace(state.Content) != "" {
				return ChatSSEResult{
					ConversationID:     state.ConversationID,
					Content:            state.Content,
					FinishType:         state.FinishType,
					HasImageGeneration: state.HasImageGeneration,
					HasInlineImage:     state.HasInlineImage,
				}
			}
			if state.ConversationID != "" && quietAfterReady > 0 {
				quietTimer = time.After(quietAfterReady)
			}
		case <-quietTimer:
			return ChatSSEResult{
				ConversationID:     state.ConversationID,
				Content:            state.Content,
				FinishType:         state.FinishType,
				HasImageGeneration: state.HasImageGeneration,
				HasInlineImage:     state.HasInlineImage,
			}
		}
	}
}

func CollectChatSSEEvent(ev SSEEvent, state *ChatSSEState) (delta string, done bool, err error) {
	if state == nil {
		state = &ChatSSEState{}
	}
	if ev.Err != nil {
		return "", true, ev.Err
	}
	if len(ev.Data) == 0 {
		return "", false, nil
	}
	if string(ev.Data) == "[DONE]" {
		return "", true, nil
	}
	var obj map[string]any
	if err := common.Unmarshal(ev.Data, &obj); err != nil {
		return "", false, nil
	}
	if chatSSEEventHasImageGeneration(obj, ev.Data) {
		state.HasImageGeneration = true
	}
	if cid, ok := obj["conversation_id"].(string); ok && cid != "" && state.ConversationID == "" {
		state.ConversationID = cid
	}
	if typ, _ := obj["type"].(string); typ == "message_stream_complete" {
		return "", true, nil
	}
	if patchDelta, patchDone := collectChatPatchEvent(obj, state); patchDelta != "" || patchDone {
		return patchDelta, patchDone, nil
	}
	message, conversationID, finishType := extractChatMessage(obj)
	if conversationID != "" && state.ConversationID == "" {
		state.ConversationID = conversationID
	}
	if finishType != "" {
		state.FinishType = finishType
	}
	if message == nil {
		return "", false, nil
	}
	if !isAssistantMessage(message) {
		return "", false, nil
	}
	latest := extractMessageText(message)
	if latest == "" {
		return "", false, nil
	}
	var hasInlineImage bool
	latest, hasInlineImage = normalizeChatAssistantContent(latest)
	if hasInlineImage {
		state.HasInlineImage = true
	}
	if strings.HasPrefix(latest, state.Content) {
		delta = latest[len(state.Content):]
	} else if latest != state.Content {
		delta = latest
	}
	state.Content = latest
	return delta, false, nil
}

func chatSSEEventHasImageGeneration(obj map[string]any, raw []byte) bool {
	if bytes.Contains(raw, []byte("image_gen")) || bytes.Contains(raw, []byte("image-generation")) {
		return true
	}
	return valueHasImageGenerationMarker(obj)
}

func valueHasImageGenerationMarker(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			lowerKey := strings.ToLower(key)
			if lowerKey == "image_gen_task_id" {
				return true
			}
			if lowerKey == "async_task_type" || lowerKey == "recipient" {
				if text, ok := child.(string); ok && strings.Contains(strings.ToLower(text), "image_gen") {
					return true
				}
			}
			if valueHasImageGenerationMarker(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if valueHasImageGenerationMarker(child) {
				return true
			}
		}
	case string:
		return strings.Contains(strings.ToLower(v), "image_gen")
	}
	return false
}

func chatContentHasInlineDataImage(content string) bool {
	return reMarkdownDataImage.MatchString(content)
}

func normalizeChatAssistantContent(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", false
	}
	lines := strings.Split(content, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if isSkippedMainlineMetadataLine(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	normalized := strings.TrimSpace(strings.Join(filtered, "\n"))
	return normalized, chatContentHasInlineDataImage(normalized)
}

func isSkippedMainlineMetadataLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
		return false
	}
	var payload map[string]any
	if err := common.Unmarshal([]byte(line), &payload); err != nil {
		return false
	}
	skipped, _ := payload["skipped_mainline"].(bool)
	return skipped && len(payload) == 1
}

func appendNormalizedChatContent(state *ChatSSEState, value string) string {
	if state == nil || value == "" {
		return ""
	}
	previous := state.Content
	normalized, hasInlineImage := normalizeChatAssistantContent(previous + value)
	if hasInlineImage {
		state.HasInlineImage = true
	}
	if normalized == previous {
		state.Content = normalized
		return ""
	}
	state.Content = normalized
	if strings.HasPrefix(normalized, previous) {
		return normalized[len(previous):]
	}
	return normalized
}

func collectChatPatchEvent(obj map[string]any, state *ChatSSEState) (delta string, done bool) {
	path, _ := obj["p"].(string)
	op, _ := obj["o"].(string)
	if state.IsAppendingText && path == "" && op == "" {
		value, _ := obj["v"].(string)
		if value != "" {
			return appendNormalizedChatContent(state, value), false
		}
	}
	if strings.Contains(path, "/message/content/parts") {
		value, _ := obj["v"].(string)
		if value == "" {
			return "", false
		}
		switch op {
		case "append":
			state.IsAppendingText = true
			return appendNormalizedChatContent(state, value), false
		case "replace":
			state.IsAppendingText = false
			return replaceChatContent(value, state), false
		}
	}
	if op != "patch" {
		return "", false
	}
	patches, _ := obj["v"].([]any)
	for _, raw := range patches {
		patch, _ := raw.(map[string]any)
		if patch == nil {
			continue
		}
		patchPath, _ := patch["p"].(string)
		patchOp, _ := patch["o"].(string)
		if strings.Contains(patchPath, "/message/content/parts") {
			value, _ := patch["v"].(string)
			if value == "" {
				continue
			}
			if patchOp == "append" {
				state.IsAppendingText = true
				delta += appendNormalizedChatContent(state, value)
			} else if patchOp == "replace" {
				state.IsAppendingText = false
				replaced := replaceChatContent(value, state)
				delta += replaced
			}
			continue
		}
		switch patchPath {
		case "/message/metadata":
			meta, _ := patch["v"].(map[string]any)
			if finish, ok := meta["finish_details"].(map[string]any); ok {
				if typ, ok := finish["type"].(string); ok {
					state.FinishType = typ
				}
			}
			if complete, _ := meta["is_complete"].(bool); complete {
				done = true
			}
		}
	}
	return delta, done
}

func replaceChatContent(latest string, state *ChatSSEState) string {
	var hasInlineImage bool
	latest, hasInlineImage = normalizeChatAssistantContent(latest)
	if hasInlineImage {
		state.HasInlineImage = true
	}
	if strings.HasPrefix(latest, state.Content) {
		delta := latest[len(state.Content):]
		state.Content = latest
		return delta
	}
	if latest != state.Content {
		state.Content = latest
		return latest
	}
	return ""
}

func extractChatMessage(obj map[string]any) (message map[string]any, conversationID string, finishType string) {
	if v, ok := obj["v"].(map[string]any); ok {
		if cid, ok := v["conversation_id"].(string); ok {
			conversationID = cid
		}
		if msg, ok := v["message"].(map[string]any); ok {
			message = msg
		}
	}
	if message == nil {
		if msg, ok := obj["message"].(map[string]any); ok {
			message = msg
		}
	}
	if conversationID == "" {
		if cid, ok := obj["conversation_id"].(string); ok {
			conversationID = cid
		}
	}
	if message != nil {
		if meta, ok := message["metadata"].(map[string]any); ok {
			if finish, ok := meta["finish_details"].(map[string]any); ok {
				if typ, ok := finish["type"].(string); ok {
					finishType = typ
				}
			}
		}
	}
	return message, conversationID, finishType
}

func isAssistantMessage(message map[string]any) bool {
	author, _ := message["author"].(map[string]any)
	if author == nil {
		return false
	}
	role, _ := author["role"].(string)
	return role == "assistant"
}

func extractMessageText(message map[string]any) string {
	content, _ := message["content"].(map[string]any)
	if content == nil {
		return ""
	}
	parts, _ := content["parts"].([]any)
	if len(parts) == 0 {
		if text, ok := content["text"].(string); ok {
			return text
		}
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		switch v := part.(type) {
		case string:
			b.WriteString(v)
		case map[string]any:
			if text, ok := v["text"].(string); ok {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

func ParseImageSSE(stream <-chan SSEEvent) ImageSSEResult {
	var result ImageSSEResult
	seenFile := map[string]struct{}{}
	seenSed := map[string]struct{}{}
	for ev := range stream {
		if !collectImageSSEEvent(ev, &result, seenFile, seenSed) {
			return result
		}
	}
	return result
}

func ParseImageSSEUntilConversationReady(stream <-chan SSEEvent, quietAfterConversation time.Duration) ImageSSEResult {
	var result ImageSSEResult
	seenFile := map[string]struct{}{}
	seenSed := map[string]struct{}{}
	var quietTimer <-chan time.Time
	for {
		select {
		case ev, ok := <-stream:
			if !ok {
				return result
			}
			if !collectImageSSEEvent(ev, &result, seenFile, seenSed) {
				return result
			}
			if len(result.FileIDs) > 0 || len(result.SedimentIDs) > 0 {
				return result
			}
			if result.ConversationID != "" && quietAfterConversation > 0 {
				quietTimer = time.After(quietAfterConversation)
			}
		case <-quietTimer:
			return result
		}
	}
}

func collectImageSSEEvent(ev SSEEvent, result *ImageSSEResult, seenFile, seenSed map[string]struct{}) bool {
	if ev.Err != nil {
		result.Err = ev.Err
		return false
	}
	if len(ev.Data) == 0 {
		return true
	}
	if string(ev.Data) == "[DONE]" {
		return false
	}
	if containsImageGenerationUpstreamErrorText(string(ev.Data)) {
		result.Err = imageGenerationUpstreamError()
		return false
	}

	var obj map[string]any
	hasJSON := common.Unmarshal(ev.Data, &obj) == nil
	if hasJSON {
		if v, ok := obj["v"].(map[string]any); ok {
			if cid, ok := v["conversation_id"].(string); ok && cid != "" && result.ConversationID == "" {
				result.ConversationID = cid
			}
			if msg, ok := v["message"].(map[string]any); ok {
				if meta, ok := msg["metadata"].(map[string]any); ok {
					if tid, ok := meta["image_gen_task_id"].(string); ok {
						result.ImageGenTaskID = tid
					}
					if finish, ok := meta["finish_details"].(map[string]any); ok {
						if finishType, ok := finish["type"].(string); ok {
							result.FinishType = finishType
						}
					}
				}
				collectImageAssistantTextFromMessage(msg, result)
				collectImageRefsFromMessage(msg, result, seenFile, seenSed)
				return true
			}
		}
		if msg, ok := obj["message"].(map[string]any); ok {
			collectImageAssistantTextFromMessage(msg, result)
			collectImageRefsFromMessage(msg, result, seenFile, seenSed)
			return true
		}
	}

	collectImageRefsFromBytes(ev.Data, result, seenFile, seenSed)
	return true
}

func collectImageAssistantTextFromMessage(message map[string]any, result *ImageSSEResult) {
	if result == nil || message == nil || isUserAuthoredMessage(message) {
		return
	}
	text := strings.TrimSpace(extractMessageText(message))
	if text == "" {
		return
	}
	result.Content = text
}

func collectImageRefsFromMessage(message map[string]any, result *ImageSSEResult, seenFile, seenSed map[string]struct{}) {
	if isUserAuthoredMessage(message) {
		return
	}
	data, err := common.Marshal(message)
	if err != nil {
		return
	}
	collectImageRefsFromBytes(data, result, seenFile, seenSed)
}

func collectImageRefsFromBytes(data []byte, result *ImageSSEResult, seenFile, seenSed map[string]struct{}) {
	for _, m := range reFileRef.FindAllSubmatch(data, -1) {
		fid := string(m[1])
		if _, ok := seenFile[fid]; !ok {
			seenFile[fid] = struct{}{}
			result.FileIDs = append(result.FileIDs, fid)
		}
	}
	for _, m := range reSedRef.FindAllSubmatch(data, -1) {
		sid := string(m[1])
		if _, ok := seenSed[sid]; !ok {
			seenSed[sid] = struct{}{}
			result.SedimentIDs = append(result.SedimentIDs, sid)
		}
	}
}

func (c *Client) GetConversationMapping(ctx context.Context, convID string) (map[string]any, error) {
	if convID == "" {
		return nil, errors.New("conv_id required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opts.BaseURL+"/backend-api/conversation/"+convID, nil)
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req)
	req.Header.Set("Accept", "*/*")
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res.StatusCode, Message: "conversation get failed", Body: string(buf)}
	}
	var out map[string]any
	if err := common.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode conversation: %w", err)
	}
	return out, nil
}

type ImageToolMsg struct {
	MessageID     string
	CreateTime    float64
	ModelSlug     string
	Recipient     string
	AuthorName    string
	ImageGenTitle string
	FileIDs       []string
	SedimentIDs   []string
}

func ExtractImageToolMsgs(mapping map[string]any) []ImageToolMsg {
	out := make([]ImageToolMsg, 0, 4)
	for mid, raw := range mapping {
		node, _ := raw.(map[string]any)
		if node == nil {
			continue
		}
		msg, _ := node["message"].(map[string]any)
		author, _ := msg["author"].(map[string]any)
		meta, _ := msg["metadata"].(map[string]any)
		content, _ := msg["content"].(map[string]any)
		if msg == nil || author == nil || meta == nil || content == nil {
			continue
		}
		if role, _ := author["role"].(string); role != "tool" {
			continue
		}
		if asyncTask, _ := meta["async_task_type"].(string); asyncTask != "image_gen" {
			continue
		}
		if contentType, _ := content["content_type"].(string); contentType != "multimodal_text" {
			continue
		}
		toolMsg := ImageToolMsg{MessageID: mid}
		if v, ok := msg["create_time"].(float64); ok {
			toolMsg.CreateTime = v
		}
		if v, ok := meta["model_slug"].(string); ok {
			toolMsg.ModelSlug = v
		}
		if v, ok := msg["recipient"].(string); ok {
			toolMsg.Recipient = v
		}
		if v, ok := author["name"].(string); ok {
			toolMsg.AuthorName = v
		}
		if v, ok := meta["image_gen_title"].(string); ok {
			toolMsg.ImageGenTitle = v
		}
		parts, _ := content["parts"].([]any)
		seenF := map[string]struct{}{}
		seenS := map[string]struct{}{}
		extractAsset := func(text string) {
			for _, m := range reFileRef.FindAllStringSubmatch(text, -1) {
				if _, ok := seenF[m[1]]; !ok {
					seenF[m[1]] = struct{}{}
					toolMsg.FileIDs = append(toolMsg.FileIDs, m[1])
				}
			}
			for _, m := range reSedRef.FindAllStringSubmatch(text, -1) {
				if _, ok := seenS[m[1]]; !ok {
					seenS[m[1]] = struct{}{}
					toolMsg.SedimentIDs = append(toolMsg.SedimentIDs, m[1])
				}
			}
		}
		for _, part := range parts {
			switch v := part.(type) {
			case map[string]any:
				if assetPointer, _ := v["asset_pointer"].(string); assetPointer != "" {
					extractAsset(assetPointer)
				}
			case string:
				extractAsset(v)
			}
		}
		out = append(out, toolMsg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreateTime < out[j].CreateTime })
	return out
}

func ExtractImageRefsFromMapping(mapping map[string]any) ([]string, []string) {
	if len(mapping) == 0 {
		return nil, nil
	}
	seenFile := map[string]struct{}{}
	seenSed := map[string]struct{}{}
	fileIDs := make([]string, 0)
	sedimentIDs := make([]string, 0)

	collect := func(data []byte) {
		for _, m := range reFileRef.FindAllSubmatch(data, -1) {
			fid := string(m[1])
			if _, ok := seenFile[fid]; ok {
				continue
			}
			seenFile[fid] = struct{}{}
			fileIDs = append(fileIDs, fid)
		}
		for _, m := range reSedRef.FindAllSubmatch(data, -1) {
			sid := string(m[1])
			if _, ok := seenSed[sid]; ok {
				continue
			}
			seenSed[sid] = struct{}{}
			sedimentIDs = append(sedimentIDs, sid)
		}
	}

	for _, raw := range mapping {
		node, _ := raw.(map[string]any)
		msg, _ := node["message"].(map[string]any)
		if msg != nil {
			if isUserAuthoredMessage(msg) {
				continue
			}
			data, err := common.Marshal(msg)
			if err == nil {
				collect(data)
			}
			continue
		}
		data, err := common.Marshal(raw)
		if err == nil {
			collect(data)
		}
	}
	return fileIDs, sedimentIDs
}

func isUserAuthoredMessage(message map[string]any) bool {
	if message == nil {
		return false
	}
	author, _ := message["author"].(map[string]any)
	if author == nil {
		return false
	}
	role, _ := author["role"].(string)
	return role == "user"
}

type PollOpts struct {
	BaselineToolIDs     map[string]struct{}
	BaselineFileIDs     map[string]struct{}
	BaselineSedimentIDs map[string]struct{}
	ExcludedFileIDs     map[string]struct{}
	MaxWait             time.Duration
	Interval            time.Duration
	StableRounds        int
	PreviewWait         time.Duration
}

type PollStatus string

const (
	PollStatusIMG2        PollStatus = "img2"
	PollStatusPreviewOnly PollStatus = "preview_only"
	PollStatusTimeout     PollStatus = "timeout"
	PollStatusError       PollStatus = "error"
	PollStatusRateLimited PollStatus = "rate_limited"
	PollStatusImageError  PollStatus = "image_error"
)

func (c *Client) PollConversationForImages(ctx context.Context, convID string, opt PollOpts) (PollStatus, []string, []string) {
	if opt.MaxWait == 0 {
		opt.MaxWait = 300 * time.Second
	}
	if opt.Interval == 0 {
		opt.Interval = 2 * time.Second
	}
	if opt.StableRounds == 0 {
		opt.StableRounds = 2
	}
	if opt.PreviewWait == 0 {
		opt.PreviewWait = 8 * time.Second
	}
	baseline := opt.BaselineToolIDs
	excludedFiles := mergeStringSets(opt.BaselineFileIDs, opt.ExcludedFileIDs)
	excludedSediments := opt.BaselineSedimentIDs
	deadline := time.Now().Add(opt.MaxWait)
	var stableCount int
	var lastSedSig string
	var firstToolTs time.Time
	var firstAnyRefTs time.Time
	var lastBroadSed []string
	var consecutive429 int

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return PollStatusTimeout, nil, nil
		default:
		}
		mapping, err := c.getMappingRaw(ctx, convID)
		if err != nil {
			if ue, ok := err.(*UpstreamError); ok && ue.Status == http.StatusTooManyRequests {
				consecutive429++
				if consecutive429 >= 3 {
					return PollStatusRateLimited, nil, nil
				}
				sleepContext(ctx, 10*time.Second)
				continue
			}
			sleepContext(ctx, opt.Interval)
			continue
		}
		consecutive429 = 0
		if mappingContainsImageGenerationError(mapping) {
			return PollStatusImageError, nil, nil
		}
		mappingFileIDs, mappingSedimentIDs := ExtractImageRefsFromMapping(mapping)
		mappingFileIDs = filterExcludedFileIDs(mappingFileIDs, excludedFiles)
		mappingSedimentIDs = filterExcludedFileIDs(mappingSedimentIDs, excludedSediments)
		if len(mappingFileIDs) > 0 {
			return PollStatusIMG2, mappingFileIDs, mappingSedimentIDs
		}
		if len(mappingSedimentIDs) > 0 {
			lastBroadSed = mappingSedimentIDs
			if firstAnyRefTs.IsZero() {
				firstAnyRefTs = time.Now()
			}
			if time.Since(firstAnyRefTs) >= opt.PreviewWait {
				return PollStatusPreviewOnly, nil, mappingSedimentIDs
			}
		}
		msgs := ExtractImageToolMsgs(mapping)
		var newMsgs []ImageToolMsg
		if len(baseline) > 0 {
			for _, msg := range msgs {
				if _, ok := baseline[msg.MessageID]; !ok {
					newMsgs = append(newMsgs, msg)
				}
			}
		} else {
			newMsgs = msgs
		}
		var allSed []string
		var allFile []string
		seenFile := map[string]struct{}{}
		seenSed := map[string]struct{}{}
		for _, msg := range newMsgs {
			for _, fid := range msg.FileIDs {
				if _, excluded := excludedFiles[fid]; excluded {
					continue
				}
				if _, ok := seenFile[fid]; !ok {
					seenFile[fid] = struct{}{}
					allFile = append(allFile, fid)
				}
			}
			for _, sid := range msg.SedimentIDs {
				if _, excluded := excludedSediments[sid]; excluded {
					continue
				}
				if _, ok := seenSed[sid]; !ok {
					seenSed[sid] = struct{}{}
					allSed = append(allSed, sid)
				}
			}
		}
		if len(allFile) > 0 {
			return PollStatusIMG2, allFile, allSed
		}
		if len(newMsgs) == 0 {
			sleepContext(ctx, opt.Interval)
			continue
		}
		if firstToolTs.IsZero() && len(newMsgs) >= 1 {
			firstToolTs = time.Now()
		}
		if len(newMsgs) >= 2 {
			sortedSed := append([]string(nil), allSed...)
			sort.Strings(sortedSed)
			sig := strings.Join(sortedSed, ",")
			if sig == lastSedSig && sig != "" {
				stableCount++
				if stableCount >= opt.StableRounds {
					return PollStatusIMG2, allFile, allSed
				}
			} else {
				stableCount = 0
				lastSedSig = sig
			}
		} else if !firstToolTs.IsZero() && time.Since(firstToolTs) >= opt.PreviewWait {
			return PollStatusPreviewOnly, allFile, allSed
		}
		sleepContext(ctx, opt.Interval)
	}
	if len(lastBroadSed) > 0 {
		return PollStatusPreviewOnly, nil, lastBroadSed
	}
	return PollStatusTimeout, nil, nil
}

func mergeStringSets(sets ...map[string]struct{}) map[string]struct{} {
	var merged map[string]struct{}
	for _, set := range sets {
		for key := range set {
			if merged == nil {
				merged = map[string]struct{}{}
			}
			merged[key] = struct{}{}
		}
	}
	return merged
}

func filterExcludedFileIDs(fileIDs []string, excluded map[string]struct{}) []string {
	if len(fileIDs) == 0 || len(excluded) == 0 {
		return fileIDs
	}
	filtered := make([]string, 0, len(fileIDs))
	for _, fid := range fileIDs {
		if _, skip := excluded[fid]; skip {
			continue
		}
		filtered = append(filtered, fid)
	}
	return filtered
}

func mappingContainsImageGenerationError(mapping map[string]any) bool {
	if len(mapping) == 0 {
		return false
	}
	data, err := common.Marshal(mapping)
	if err != nil {
		return false
	}
	return containsImageGenerationUpstreamErrorText(string(data))
}

func (c *Client) getMappingRaw(ctx context.Context, convID string) (map[string]any, error) {
	full, err := c.GetConversationMapping(ctx, convID)
	if err != nil {
		return nil, err
	}
	mapping, _ := full["mapping"].(map[string]any)
	if mapping == nil {
		mapping = map[string]any{}
	}
	return mapping, nil
}

func (c *Client) GetConversationHead(ctx context.Context, convID string) (string, error) {
	full, err := c.GetConversationMapping(ctx, convID)
	if err != nil {
		return "", err
	}
	head, _ := full["current_node"].(string)
	if head == "" {
		return "", errors.New("current_node missing")
	}
	return head, nil
}

func (c *Client) ImageDownloadURL(ctx context.Context, convID, fileRef string) (string, error) {
	var apiURL string
	if strings.HasPrefix(fileRef, "sed:") {
		if convID == "" {
			return "", errors.New("conv_id required for sediment")
		}
		fid := strings.TrimPrefix(fileRef, "sed:")
		apiURL = fmt.Sprintf("%s/backend-api/conversation/%s/attachment/%s/download", c.opts.BaseURL, url.PathEscape(convID), url.PathEscape(fid))
	} else {
		apiURL = fmt.Sprintf("%s/backend-api/files/%s/download", c.opts.BaseURL, url.PathEscape(fileRef))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	c.commonHeaders(req)
	req.Header.Set("Accept", "*/*")
	res, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", &UpstreamError{Status: res.StatusCode, Message: "files/download failed", Body: string(buf)}
	}
	var out struct {
		DownloadURL string `json:"download_url"`
		Status      string `json:"status"`
	}
	if err := common.Unmarshal(buf, &out); err != nil {
		return "", fmt.Errorf("decode files/download: %w", err)
	}
	if out.DownloadURL == "" {
		return "", fmt.Errorf("empty download_url (status=%s)", out.Status)
	}
	return out.DownloadURL, nil
}

func (c *Client) FetchImage(ctx context.Context, signedURL string, maxBytes int64) ([]byte, string, error) {
	if maxBytes <= 0 {
		maxBytes = 16 * 1024 * 1024
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, "", err
	}
	needAuth := strings.HasPrefix(signedURL, c.opts.BaseURL+"/")
	if needAuth {
		c.commonHeaders(req)
		req.Header.Set("Accept", "image/*,*/*;q=0.8")
	} else {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, "", &UpstreamError{Status: res.StatusCode, Message: "fetch image failed"}
	}
	ct := res.Header.Get("Content-Type")
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return nil, ct, err
	}
	if int64(len(body)) > maxBytes {
		return nil, ct, fmt.Errorf("image exceeds max bytes (%d)", maxBytes)
	}
	return body, ct, nil
}

type UploadedFile struct {
	FileID      string `json:"file_id"`
	FileName    string `json:"file_name"`
	FileSize    int    `json:"file_size"`
	MimeType    string `json:"mime_type"`
	UseCase     string `json:"use_case"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	DownloadURL string `json:"download_url"`
}

func (c *Client) UploadFile(ctx context.Context, data []byte, fileName string) (*UploadedFile, error) {
	if len(data) == 0 {
		return nil, errors.New("empty file data")
	}
	mimeType, ext := sniffMime(data)
	useCase := "multimodal"
	if !strings.HasPrefix(mimeType, "image/") {
		useCase = "my_files"
	}
	if fileName == "" {
		fileName = fmt.Sprintf("file-%d%s", len(data), ext)
	}
	out := &UploadedFile{FileName: fileName, FileSize: len(data), MimeType: mimeType, UseCase: useCase}
	if strings.HasPrefix(mimeType, "image/") {
		if img, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			out.Width = img.Width
			out.Height = img.Height
		}
	}
	step1Body := map[string]any{"file_name": fileName, "file_size": len(data), "use_case": useCase}
	if out.Width > 0 && out.Height > 0 {
		step1Body["height"] = out.Height
		step1Body["width"] = out.Width
	}
	step1JSON, _ := common.Marshal(step1Body)
	req1, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/files", bytes.NewReader(step1JSON))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req1)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Accept", "application/json")
	res1, err := c.hc.Do(req1)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	defer res1.Body.Close()
	buf1, _ := io.ReadAll(res1.Body)
	if res1.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res1.StatusCode, Message: "create file failed", Body: string(buf1)}
	}
	var step1Resp struct {
		FileID    string `json:"file_id"`
		UploadURL string `json:"upload_url"`
	}
	if err := common.Unmarshal(buf1, &step1Resp); err != nil {
		return nil, fmt.Errorf("decode create-file resp: %w", err)
	}
	if step1Resp.FileID == "" || step1Resp.UploadURL == "" {
		return nil, fmt.Errorf("create-file empty: %s", truncateString(string(buf1), 200))
	}
	out.FileID = step1Resp.FileID
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	req2, err := http.NewRequestWithContext(ctx, http.MethodPut, step1Resp.UploadURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", mimeType)
	req2.Header.Set("x-ms-blob-type", "BlockBlob")
	req2.Header.Set("x-ms-version", "2020-04-08")
	req2.Header.Set("Origin", c.opts.BaseURL)
	req2.Header.Set("User-Agent", c.opts.UserAgent)
	req2.Header.Set("Accept", "application/json, text/plain, */*")
	req2.Header.Set("Accept-Language", "en-US,en;q=0.8")
	req2.Header.Set("Referer", c.opts.BaseURL+"/")
	res2, err := c.hc.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("upload PUT: %w", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode >= 400 {
		buf2, _ := io.ReadAll(res2.Body)
		return nil, &UpstreamError{Status: res2.StatusCode, Message: "upload PUT failed", Body: string(buf2)}
	}
	_, _ = io.Copy(io.Discard, res2.Body)
	req3, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/backend-api/files/"+step1Resp.FileID+"/uploaded", strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req3)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Accept", "application/json")
	res3, err := c.hc.Do(req3)
	if err != nil {
		return nil, fmt.Errorf("register uploaded: %w", err)
	}
	defer res3.Body.Close()
	buf3, _ := io.ReadAll(res3.Body)
	if res3.StatusCode >= 400 {
		return nil, &UpstreamError{Status: res3.StatusCode, Message: "register uploaded failed", Body: string(buf3)}
	}
	var step3Resp struct {
		DownloadURL string `json:"download_url"`
	}
	_ = common.Unmarshal(buf3, &step3Resp)
	out.DownloadURL = step3Resp.DownloadURL
	return out, nil
}

type Attachment struct {
	ID       string `json:"id"`
	MimeType string `json:"mimeType"`
	Name     string `json:"name"`
	Size     int    `json:"size"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

func (u *UploadedFile) ToAttachment() Attachment {
	attachment := Attachment{ID: u.FileID, MimeType: u.MimeType, Name: u.FileName, Size: u.FileSize}
	if u.UseCase == "multimodal" {
		attachment.Width = u.Width
		attachment.Height = u.Height
	}
	return attachment
}

type AssetPointerPart struct {
	ContentType  string `json:"content_type,omitempty"`
	AssetPointer string `json:"asset_pointer"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	SizeBytes    int    `json:"size_bytes,omitempty"`
}

func (u *UploadedFile) ToAssetPointerPart() AssetPointerPart {
	return AssetPointerPart{
		ContentType:  "image_asset_pointer",
		AssetPointer: "file-service://" + u.FileID,
		Width:        u.Width,
		Height:       u.Height,
		SizeBytes:    u.FileSize,
	}
}

func sniffMime(data []byte) (string, string) {
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png":
		return contentType, ".png"
	case "image/jpeg":
		return contentType, ".jpg"
	case "image/gif":
		return contentType, ".gif"
	case "image/webp":
		return contentType, ".webp"
	default:
		if strings.HasPrefix(contentType, "image/") {
			ext := filepath.Ext(contentType)
			if ext != "" {
				return contentType, ext
			}
		}
		return contentType, ""
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func sleepContext(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
