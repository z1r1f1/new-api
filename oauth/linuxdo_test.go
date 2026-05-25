package oauth

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
)

func newLinuxDOTestContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}

func withLinuxDOGlobals(t *testing.T) {
	t.Helper()
	oldClientID := common.LinuxDOClientId
	oldClientSecret := common.LinuxDOClientSecret
	oldMinimumTrustLevel := common.LinuxDOMinimumTrustLevel
	common.LinuxDOClientId = "linuxdo-client"
	common.LinuxDOClientSecret = "linuxdo-secret"
	common.LinuxDOMinimumTrustLevel = 0
	t.Cleanup(func() {
		common.LinuxDOClientId = oldClientID
		common.LinuxDOClientSecret = oldClientSecret
		common.LinuxDOMinimumTrustLevel = oldMinimumTrustLevel
	})
}

func TestLinuxDOExchangeTokenUsesForwardedFrontendRedirectURIAndBasicAuth(t *testing.T) {
	withLinuxDOGlobals(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Fatalf("unexpected token path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("linuxdo-client:linuxdo-secret"))
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			t.Fatalf("Authorization header mismatch: got %q want %q", got, expectedAuth)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "authorization_code" {
			t.Fatalf("grant_type mismatch: got %q", got)
		}
		if got := r.Form.Get("code"); got != "auth-code" {
			t.Fatalf("code mismatch: got %q", got)
		}
		if got := r.Form.Get("redirect_uri"); got != "https://public.example.com/oauth/linuxdo" {
			t.Fatalf("redirect_uri mismatch: got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"access-token","token_type":"Bearer","refresh_token":"refresh-token","expires_in":3600,"scope":"user:profile"}`)
	}))
	defer server.Close()
	t.Setenv("LINUX_DO_TOKEN_ENDPOINT", server.URL+"/oauth2/token")

	c := newLinuxDOTestContext("http://internal.local/api/oauth/linuxdo")
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", "public.example.com")

	token, err := (&LinuxDOProvider{}).ExchangeToken(context.Background(), "auth-code", c)
	if err != nil {
		t.Fatalf("ExchangeToken returned error: %v", err)
	}
	if token.AccessToken != "access-token" || token.TokenType != "Bearer" || token.RefreshToken != "refresh-token" || token.ExpiresIn != 3600 || token.Scope != "user:profile" {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestLinuxDOExchangeTokenRejectsNonOKTokenResponse(t *testing.T) {
	withLinuxDOGlobals(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"invalid_client","error_description":"bad client"}`)
	}))
	defer server.Close()
	t.Setenv("LINUX_DO_TOKEN_ENDPOINT", server.URL)

	_, err := (&LinuxDOProvider{}).ExchangeToken(context.Background(), "auth-code", newLinuxDOTestContext("http://example.com/api/oauth/linuxdo"))
	if err == nil {
		t.Fatal("expected token exchange error")
	}
	oauthErr, ok := err.(*OAuthError)
	if !ok {
		t.Fatalf("expected OAuthError, got %T", err)
	}
	if oauthErr.MsgKey != i18n.MsgOAuthTokenFailed {
		t.Fatalf("unexpected message key: %s", oauthErr.MsgKey)
	}
	if !strings.Contains(oauthErr.RawError, "bad client") {
		t.Fatalf("expected raw error to include upstream description, got %q", oauthErr.RawError)
	}
}

func TestLinuxDOGetUserInfoMapsProfileAndHonorsTrustLevel(t *testing.T) {
	withLinuxDOGlobals(t)
	common.LinuxDOMinimumTrustLevel = 2

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user" {
			t.Fatalf("unexpected user path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization header mismatch: got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":12345,"username":"linuxdo-user","name":"Linux DO User","active":true,"trust_level":3,"silenced":false}`)
	}))
	defer server.Close()
	t.Setenv("LINUX_DO_USER_ENDPOINT", server.URL+"/api/user")

	user, err := (&LinuxDOProvider{}).GetUserInfo(context.Background(), &OAuthToken{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("GetUserInfo returned error: %v", err)
	}
	if user.ProviderUserID != "12345" || user.Username != "linuxdo-user" || user.DisplayName != "Linux DO User" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if got := user.Extra["trust_level"]; got != 3 {
		t.Fatalf("unexpected trust_level extra: %#v", got)
	}
}

func TestLinuxDOGetUserInfoRejectsNonOKResponse(t *testing.T) {
	withLinuxDOGlobals(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"message":"access denied"}`)
	}))
	defer server.Close()
	t.Setenv("LINUX_DO_USER_ENDPOINT", server.URL)

	_, err := (&LinuxDOProvider{}).GetUserInfo(context.Background(), &OAuthToken{AccessToken: "access-token"})
	if err == nil {
		t.Fatal("expected userinfo error")
	}
	oauthErr, ok := err.(*OAuthError)
	if !ok {
		t.Fatalf("expected OAuthError, got %T", err)
	}
	if oauthErr.MsgKey != i18n.MsgOAuthGetUserErr {
		t.Fatalf("unexpected message key: %s", oauthErr.MsgKey)
	}
}

func TestLinuxDOGetUserInfoRejectsInactiveOrSilencedAccount(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		msgKey string
	}{
		{
			name:   "inactive",
			body:   `{"id":12345,"username":"linuxdo-user","name":"Linux DO User","active":false,"trust_level":3,"silenced":false}`,
			msgKey: i18n.MsgOAuthLinuxDOInactive,
		},
		{
			name:   "silenced",
			body:   `{"id":12345,"username":"linuxdo-user","name":"Linux DO User","active":true,"trust_level":3,"silenced":true}`,
			msgKey: i18n.MsgOAuthLinuxDOSilenced,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLinuxDOGlobals(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			t.Setenv("LINUX_DO_USER_ENDPOINT", server.URL)

			_, err := (&LinuxDOProvider{}).GetUserInfo(context.Background(), &OAuthToken{AccessToken: "access-token"})
			if err == nil {
				t.Fatal("expected access denied error")
			}
			oauthErr, ok := err.(*OAuthError)
			if !ok {
				t.Fatalf("expected OAuthError, got %T", err)
			}
			if oauthErr.MsgKey != tt.msgKey {
				t.Fatalf("unexpected message key: %s", oauthErr.MsgKey)
			}
		})
	}
}
