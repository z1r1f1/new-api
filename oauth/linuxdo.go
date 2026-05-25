package oauth

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func init() {
	Register("linuxdo", &LinuxDOProvider{})
}

// LinuxDOProvider implements OAuth for Linux DO
type LinuxDOProvider struct{}

type linuxdoUser struct {
	Id         int    `json:"id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Active     *bool  `json:"active"`
	TrustLevel int    `json:"trust_level"`
	Silenced   *bool  `json:"silenced"`
}

type linuxdoTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Message          string `json:"message"`
}

func (p *LinuxDOProvider) GetName() string {
	return "Linux DO"
}

func (p *LinuxDOProvider) IsEnabled() bool {
	return common.LinuxDOOAuthEnabled
}

func (p *LinuxDOProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	logger.LogDebug(ctx, "[OAuth-LinuxDO] ExchangeToken: code=%s...", code[:min(len(code), 10)])

	// Get access token using Basic auth
	tokenEndpoint := common.GetEnvOrDefaultString("LINUX_DO_TOKEN_ENDPOINT", "https://connect.linux.do/oauth2/token")
	credentials := common.LinuxDOClientId + ":" + common.LinuxDOClientSecret
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))

	redirectURI := linuxdoOAuthRedirectURI(c)

	logger.LogDebug(ctx, "[OAuth-LinuxDO] ExchangeToken: token_endpoint=%s, redirect_uri=%s", tokenEndpoint, redirectURI)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Linux DO"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-LinuxDO] ExchangeToken response status: %d", res.StatusCode)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] ExchangeToken read body error: %s", err.Error()))
		return nil, err
	}

	var tokenRes linuxdoTokenResponse
	if len(body) > 0 {
		if err := common.Unmarshal(body, &tokenRes); err != nil {
			if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
				rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, body)
				logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] ExchangeToken failed: %s", rawMessage))
				return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Linux DO"}, rawMessage)
			}
			logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] ExchangeToken decode error: %s", err.Error()))
			return nil, err
		}
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, body, tokenRes.ErrorDescription, tokenRes.Message, tokenRes.Error)
		logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] ExchangeToken failed: %s", rawMessage))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Linux DO"}, rawMessage)
	}

	if tokenRes.AccessToken == "" {
		rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, body, tokenRes.ErrorDescription, tokenRes.Message, tokenRes.Error)
		logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] ExchangeToken failed: %s", rawMessage))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Linux DO"}, rawMessage)
	}

	logger.LogDebug(ctx, "[OAuth-LinuxDO] ExchangeToken success: scope=%s", tokenRes.Scope)

	return &OAuthToken{
		AccessToken:  tokenRes.AccessToken,
		TokenType:    tokenRes.TokenType,
		RefreshToken: tokenRes.RefreshToken,
		ExpiresIn:    tokenRes.ExpiresIn,
		Scope:        tokenRes.Scope,
	}, nil
}

func linuxdoOAuthRedirectURI(c *gin.Context) string {
	origin := linuxdoRequestOrigin(c)
	return strings.TrimRight(origin, "/") + "/oauth/linuxdo"
}

func linuxdoRequestOrigin(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return "http://localhost:3000"
	}

	forwarded := c.GetHeader("Forwarded")
	proto := linuxdoFirstHeaderValue(
		linuxdoForwardedHeaderValue(forwarded, "proto"),
		c.GetHeader("X-Forwarded-Proto"),
		c.GetHeader("X-Forwarded-Scheme"),
		c.GetHeader("X-Scheme"),
	)
	host := linuxdoFirstHeaderValue(
		linuxdoForwardedHeaderValue(forwarded, "host"),
		c.GetHeader("X-Forwarded-Host"),
		c.GetHeader("X-Host"),
		c.Request.Host,
	)

	if proto == "" {
		proto = "http"
		if c.Request.TLS != nil {
			proto = "https"
		}
	}
	if host == "" {
		host = "localhost:3000"
	}
	return proto + "://" + host
}

func linuxdoForwardedHeaderValue(header string, key string) string {
	firstEntry := linuxdoFirstHeaderValue(header)
	for _, part := range strings.Split(firstEntry, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !strings.EqualFold(name, key) {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`)
	}
	return ""
}

func linuxdoFirstHeaderValue(values ...string) string {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				return part
			}
		}
	}
	return ""
}

func linuxdoOAuthResponseMessage(statusCode int, body []byte, messages ...string) string {
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message != "" {
			return fmt.Sprintf("status=%d, %s", statusCode, message)
		}
	}
	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		return fmt.Sprintf("status=%d", statusCode)
	}
	const maxBodyLogLength = 500
	if len(bodyText) > maxBodyLogLength {
		bodyText = bodyText[:maxBodyLogLength] + "..."
	}
	return fmt.Sprintf("status=%d, %s", statusCode, bodyText)
}

func (p *LinuxDOProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	userEndpoint := common.GetEnvOrDefaultString("LINUX_DO_USER_ENDPOINT", "https://connect.linux.do/api/user")

	logger.LogDebug(ctx, "[OAuth-LinuxDO] GetUserInfo: user_endpoint=%s", userEndpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", userEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] GetUserInfo error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Linux DO"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-LinuxDO] GetUserInfo response status: %d", res.StatusCode)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] GetUserInfo read body error: %s", err.Error()))
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, body)
		logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] GetUserInfo failed: %s", rawMessage))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "Linux DO"}, rawMessage)
	}

	var linuxdoUser linuxdoUser
	if len(body) > 0 {
		if err := common.Unmarshal(body, &linuxdoUser); err != nil {
			logger.LogError(ctx, fmt.Sprintf("[OAuth-LinuxDO] GetUserInfo decode error: %s", err.Error()))
			return nil, err
		}
	}

	if linuxdoUser.Id == 0 {
		logger.LogError(ctx, "[OAuth-LinuxDO] GetUserInfo failed: invalid user id")
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "Linux DO"})
	}

	active := true
	if linuxdoUser.Active != nil {
		active = *linuxdoUser.Active
	}
	silenced := false
	if linuxdoUser.Silenced != nil {
		silenced = *linuxdoUser.Silenced
	}
	if !active {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-LinuxDO] GetUserInfo: inactive account id=%d", linuxdoUser.Id))
		return nil, NewOAuthError(i18n.MsgOAuthLinuxDOInactive, nil)
	}
	if silenced {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-LinuxDO] GetUserInfo: silenced account id=%d", linuxdoUser.Id))
		return nil, NewOAuthError(i18n.MsgOAuthLinuxDOSilenced, nil)
	}

	logger.LogDebug(ctx, "[OAuth-LinuxDO] GetUserInfo: id=%d, username=%s, name=%s, trust_level=%d, active=%v, silenced=%v",
		linuxdoUser.Id, linuxdoUser.Username, linuxdoUser.Name, linuxdoUser.TrustLevel, active, silenced)

	// Check trust level
	if linuxdoUser.TrustLevel < common.LinuxDOMinimumTrustLevel {
		logger.LogWarn(ctx, fmt.Sprintf("[OAuth-LinuxDO] GetUserInfo: trust level too low (required=%d, current=%d)",
			common.LinuxDOMinimumTrustLevel, linuxdoUser.TrustLevel))
		return nil, &TrustLevelError{
			Required: common.LinuxDOMinimumTrustLevel,
			Current:  linuxdoUser.TrustLevel,
		}
	}

	logger.LogDebug(ctx, "[OAuth-LinuxDO] GetUserInfo success: id=%d, username=%s", linuxdoUser.Id, linuxdoUser.Username)

	return &OAuthUser{
		ProviderUserID: strconv.Itoa(linuxdoUser.Id),
		Username:       linuxdoUser.Username,
		DisplayName:    linuxdoUser.Name,
		Extra: map[string]any{
			"trust_level": linuxdoUser.TrustLevel,
			"active":      active,
			"silenced":    silenced,
		},
	}, nil
}

func (p *LinuxDOProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsLinuxDOIdAlreadyTaken(providerUserID)
}

func (p *LinuxDOProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.LinuxDOId = providerUserID
	return user.FillUserByLinuxDOId()
}

func (p *LinuxDOProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.LinuxDOId = providerUserID
}

func (p *LinuxDOProvider) GetProviderPrefix() string {
	return "linuxdo_"
}

// TrustLevelError indicates the user's trust level is too low
type TrustLevelError struct {
	Required int
	Current  int
}

func (e *TrustLevelError) Error() string {
	return "trust level too low"
}
