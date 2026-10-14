package oauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "JSON with credentials", body: `{"error":"invalid_client","error_description":"bad client access-token-sensitive","access_token":"access-token-sensitive","refresh_token":"refresh-token-sensitive"}`},
		{name: "non-JSON with credentials", body: `<html>access-token-sensitive refresh-token-sensitive</html>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withLinuxDOGlobals(t)
			var logs bytes.Buffer
			oldWriter := gin.DefaultWriter
			gin.DefaultWriter = &logs
			t.Cleanup(func() { gin.DefaultWriter = oldWriter })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			t.Setenv("LINUX_DO_TOKEN_ENDPOINT", server.URL)

			token, err := (&LinuxDOProvider{}).ExchangeToken(context.Background(), "auth-code", newLinuxDOTestContext("http://example.com/api/oauth/linuxdo"))
			require.Error(t, err)
			assert.Nil(t, token)
			var oauthErr *OAuthError
			require.ErrorAs(t, err, &oauthErr)
			assert.Contains(t, oauthErr.RawError, "status=401")
			for _, secret := range []string{"access-token-sensitive", "refresh-token-sensitive"} {
				assert.NotContains(t, oauthErr.Error(), secret)
				assert.NotContains(t, logs.String(), secret)
			}
		})
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

	user, err := (&LinuxDOProvider{}).GetUserInfo(context.Background(), &OAuthToken{AccessToken: "access-token"})
	require.Error(t, err)
	assert.Nil(t, user)
	var oauthErr *OAuthError
	require.ErrorAs(t, err, &oauthErr)
}

func TestLinuxDOGetUserInfoRejectsInactiveOrSilencedAccount(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "inactive",
			body: `{"id":12345,"username":"linuxdo-user","name":"Linux DO User","active":false,"trust_level":3,"silenced":false}`,
		},
		{
			name: "silenced",
			body: `{"id":12345,"username":"linuxdo-user","name":"Linux DO User","active":true,"trust_level":3,"silenced":true}`,
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

			user, err := (&LinuxDOProvider{}).GetUserInfo(context.Background(), &OAuthToken{AccessToken: "access-token"})
			require.Error(t, err)
			assert.Nil(t, user)
			var oauthErr *OAuthError
			require.ErrorAs(t, err, &oauthErr)
		})
	}
}
