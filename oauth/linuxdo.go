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
		return nil, NewOAuthError(common.NewMessage(msgInvalidCode))
	}

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
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgConnectFailed, map[string]any{"provider": "Linux DO"}), err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-LinuxDO] ExchangeToken response status: %d", res.StatusCode)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] ExchangeToken read body error: %s", err.Error()))
		return nil, err
	}

	var tokenRes linuxdoTokenResponse
	if len(body) > 0 {
		if err := common.Unmarshal(body, &tokenRes); err != nil {
			if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
				rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, "")
				logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] ExchangeToken failed: %s", rawMessage))
				return nil, NewOAuthErrorWithRaw(common.NewMessage(msgTokenFailed, map[string]any{"provider": "Linux DO"}), rawMessage)
			}
			logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] ExchangeToken decode error: %s", err.Error()))
			return nil, err
		}
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, tokenRes.Error)
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] ExchangeToken failed: %s", rawMessage))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgTokenFailed, map[string]any{"provider": "Linux DO"}), rawMessage)
	}

	if tokenRes.AccessToken == "" {
		rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, tokenRes.Error)
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] ExchangeToken failed: %s", rawMessage))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgTokenFailed, map[string]any{"provider": "Linux DO"}), rawMessage)
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

// OAuth responses can contain usable credentials even on failures. Record only
// the HTTP status and standard OAuth error code, never the response body or an
// arbitrary provider description.
func linuxdoOAuthResponseMessage(statusCode int, errorCode string) string {
	switch errorCode {
	case "invalid_request", "invalid_client", "invalid_grant", "unauthorized_client",
		"unsupported_grant_type", "invalid_scope", "server_error", "temporarily_unavailable", "access_denied":
		return fmt.Sprintf("status=%d, %s", statusCode, errorCode)
	default:
		return fmt.Sprintf("status=%d", statusCode)
	}
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
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgConnectFailed, map[string]any{"provider": "Linux DO"}), err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-LinuxDO] GetUserInfo response status: %d", res.StatusCode)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo read body error: %s", err.Error()))
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		rawMessage := linuxdoOAuthResponseMessage(res.StatusCode, "")
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo failed: %s", rawMessage))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgGetUserFailed), rawMessage)
	}

	var linuxdoUser linuxdoUser
	if len(body) > 0 {
		if err := common.Unmarshal(body, &linuxdoUser); err != nil {
			logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo decode error: %s", err.Error()))
			return nil, err
		}
	}

	if linuxdoUser.Id == 0 {
		logger.LogError(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo failed: invalid user id"))
		return nil, NewOAuthError(common.NewMessage(msgUserInfoEmpty, map[string]any{"provider": "Linux DO"}))
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
		logger.LogWarn(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo: inactive account id=%d", linuxdoUser.Id))
		return nil, NewOAuthError(common.NewMessage("Linux DO account is not active"))
	}
	if silenced {
		logger.LogWarn(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo: silenced account id=%d", linuxdoUser.Id))
		return nil, NewOAuthError(common.NewMessage("Linux DO account is silenced"))
	}

	logger.LogDebug(ctx, "[OAuth-LinuxDO] GetUserInfo: id=%d, trust_level=%d, active=%v, silenced=%v",
		linuxdoUser.Id, linuxdoUser.TrustLevel, active, silenced)

	// Check trust level
	if linuxdoUser.TrustLevel < common.LinuxDOMinimumTrustLevel {
		logger.LogWarn(ctx, common.LogText("[OAuth-LinuxDO] GetUserInfo: trust level too low (required=%d, current=%d)",
			common.LinuxDOMinimumTrustLevel, linuxdoUser.TrustLevel))
		return nil, &TrustLevelError{
			Required: common.LinuxDOMinimumTrustLevel,
			Current:  linuxdoUser.TrustLevel,
		}
	}

	logger.LogDebug(ctx, "[OAuth-LinuxDO] GetUserInfo success: id=%d", linuxdoUser.Id)

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

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *LinuxDOProvider) ProviderUserIDColumn() string {
	return "linux_do_id"
}

// TrustLevelError indicates the user's trust level is too low
type TrustLevelError struct {
	Required int
	Current  int
}

func (e *TrustLevelError) Error() string {
	return "trust level too low"
}
