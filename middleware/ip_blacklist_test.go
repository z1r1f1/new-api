package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func configureIPBlacklistAutoBanForTest(t *testing.T, scope string, pathPrefixes string, rpm int) *system_setting.IPBlacklistSetting {
	t.Helper()

	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	originalAutoBanEnabled := setting.AutoBanEnabled
	originalAutoBanRpm := setting.AutoBanRpm
	originalAutoBanWhitelist := setting.AutoBanWhitelist
	originalAutoBanScope := setting.AutoBanScope
	originalAutoBanPathPrefixes := setting.AutoBanPathPrefixes
	originalUpdate := updateIPBlacklistList
	originalNow := ipAutoBanNow
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
		setting.AutoBanEnabled = originalAutoBanEnabled
		setting.AutoBanRpm = originalAutoBanRpm
		setting.AutoBanWhitelist = originalAutoBanWhitelist
		setting.AutoBanScope = originalAutoBanScope
		setting.AutoBanPathPrefixes = originalAutoBanPathPrefixes
		updateIPBlacklistList = originalUpdate
		ipAutoBanNow = originalNow
		ipAutoBanCounters = make(map[string]ipAutoBanCounter)
	})

	setting.Enabled = false
	setting.List = ""
	setting.AutoBanEnabled = true
	setting.AutoBanRpm = rpm
	setting.AutoBanWhitelist = ""
	setting.AutoBanScope = scope
	setting.AutoBanPathPrefixes = pathPrefixes
	updateIPBlacklistList = func(key string, value string) error {
		if key != "ip_blacklist_setting.list" {
			t.Fatalf("key = %s, want ip_blacklist_setting.list", key)
		}
		setting.List = value
		return nil
	}
	ipAutoBanNow = func() time.Time {
		return time.Unix(300, 0)
	}
	ipAutoBanCounters = make(map[string]ipAutoBanCounter)

	return setting
}

func TestIPBlacklistBlocksMatchingClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
	})

	setting.Enabled = true
	setting.List = "203.0.113.0/24"

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.8:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestIPBlacklistAutoBansHighRpmClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	originalAutoBanEnabled := setting.AutoBanEnabled
	originalAutoBanRpm := setting.AutoBanRpm
	originalAutoBanWhitelist := setting.AutoBanWhitelist
	originalAutoBanScope := setting.AutoBanScope
	originalAutoBanPathPrefixes := setting.AutoBanPathPrefixes
	originalUpdate := updateIPBlacklistList
	originalNow := ipAutoBanNow
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
		setting.AutoBanEnabled = originalAutoBanEnabled
		setting.AutoBanRpm = originalAutoBanRpm
		setting.AutoBanWhitelist = originalAutoBanWhitelist
		setting.AutoBanScope = originalAutoBanScope
		setting.AutoBanPathPrefixes = originalAutoBanPathPrefixes
		updateIPBlacklistList = originalUpdate
		ipAutoBanNow = originalNow
		ipAutoBanCounters = make(map[string]ipAutoBanCounter)
	})

	setting.Enabled = false
	setting.List = ""
	setting.AutoBanEnabled = true
	setting.AutoBanRpm = 2
	setting.AutoBanWhitelist = ""
	setting.AutoBanScope = system_setting.IPAutoBanScopeAll
	setting.AutoBanPathPrefixes = ""
	updateIPBlacklistList = func(key string, value string) error {
		if key != "ip_blacklist_setting.list" {
			t.Fatalf("key = %s, want ip_blacklist_setting.list", key)
		}
		setting.List = value
		return nil
	}
	ipAutoBanNow = func() time.Time {
		return time.Unix(120, 0)
	}
	ipAutoBanCounters = make(map[string]ipAutoBanCounter)

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req1 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req1.RemoteAddr = "203.0.113.9:12345"
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d; body=%s", w1.Code, http.StatusOK, w1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req2.RemoteAddr = "203.0.113.9:12345"
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("second status = %d, want %d; body=%s", w2.Code, http.StatusForbidden, w2.Body.String())
	}
	if setting.List != "203.0.113.9" {
		t.Fatalf("setting.List = %q, want auto-banned IP", setting.List)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req3.RemoteAddr = "203.0.113.9:12345"
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("third status = %d, want %d; body=%s", w3.Code, http.StatusForbidden, w3.Body.String())
	}
}

func TestIPBlacklistAutoBanDefaultScopeSkipsDashboardAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := configureIPBlacklistAutoBanForTest(t, "", "", 1)

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/api/status", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "203.0.113.11:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if setting.List != "" {
		t.Fatalf("setting.List = %q, want empty", setting.List)
	}
}

func TestIPBlacklistAutoBanRelayScopeCountsModelAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := configureIPBlacklistAutoBanForTest(t, system_setting.IPAutoBanScopeRelay, "", 1)

	router := gin.New()
	router.Use(IPBlacklist())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.12:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	if setting.List != "203.0.113.12" {
		t.Fatalf("setting.List = %q, want auto-banned IP", setting.List)
	}
}

func TestIPBlacklistAutoBanAllScopeCountsDashboardAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := configureIPBlacklistAutoBanForTest(t, system_setting.IPAutoBanScopeAll, "", 1)

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/api/status", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "203.0.113.13:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	if setting.List != "203.0.113.13" {
		t.Fatalf("setting.List = %q, want auto-banned IP", setting.List)
	}
}

func TestIPBlacklistAutoBanCustomScopeCountsOnlyConfiguredPrefixes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := configureIPBlacklistAutoBanForTest(t, system_setting.IPAutoBanScopeCustom, "/api/token\ncustom", 1)

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/api/status", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.POST("/api/token/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req1.RemoteAddr = "203.0.113.14:12345"
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("non-matching status = %d, want %d; body=%s", w1.Code, http.StatusOK, w1.Body.String())
	}
	if setting.List != "" {
		t.Fatalf("setting.List after non-matching request = %q, want empty", setting.List)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/token/", nil)
	req2.RemoteAddr = "203.0.113.14:12345"
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("matching status = %d, want %d; body=%s", w2.Code, http.StatusForbidden, w2.Body.String())
	}
	if setting.List != "203.0.113.14" {
		t.Fatalf("setting.List = %q, want auto-banned IP", setting.List)
	}
}

func TestIPBlacklistAutoBanWhitelistSkipsClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	originalAutoBanEnabled := setting.AutoBanEnabled
	originalAutoBanRpm := setting.AutoBanRpm
	originalAutoBanWhitelist := setting.AutoBanWhitelist
	originalAutoBanScope := setting.AutoBanScope
	originalAutoBanPathPrefixes := setting.AutoBanPathPrefixes
	originalNow := ipAutoBanNow
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
		setting.AutoBanEnabled = originalAutoBanEnabled
		setting.AutoBanRpm = originalAutoBanRpm
		setting.AutoBanWhitelist = originalAutoBanWhitelist
		setting.AutoBanScope = originalAutoBanScope
		setting.AutoBanPathPrefixes = originalAutoBanPathPrefixes
		ipAutoBanNow = originalNow
		ipAutoBanCounters = make(map[string]ipAutoBanCounter)
	})

	setting.Enabled = false
	setting.List = ""
	setting.AutoBanEnabled = true
	setting.AutoBanRpm = 1
	setting.AutoBanWhitelist = "203.0.113.0/24"
	setting.AutoBanScope = system_setting.IPAutoBanScopeAll
	setting.AutoBanPathPrefixes = ""
	ipAutoBanNow = func() time.Time {
		return time.Unix(180, 0)
	}
	ipAutoBanCounters = make(map[string]ipAutoBanCounter)

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if setting.List != "" {
		t.Fatalf("setting.List = %q, want empty", setting.List)
	}
}

func TestIPBlacklistAutoBanUsesForwardedClientWhenProxyIsWhitelisted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	originalAutoBanEnabled := setting.AutoBanEnabled
	originalAutoBanRpm := setting.AutoBanRpm
	originalAutoBanWhitelist := setting.AutoBanWhitelist
	originalAutoBanScope := setting.AutoBanScope
	originalAutoBanPathPrefixes := setting.AutoBanPathPrefixes
	originalUpdate := updateIPBlacklistList
	originalNow := ipAutoBanNow
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
		setting.AutoBanEnabled = originalAutoBanEnabled
		setting.AutoBanRpm = originalAutoBanRpm
		setting.AutoBanWhitelist = originalAutoBanWhitelist
		setting.AutoBanScope = originalAutoBanScope
		setting.AutoBanPathPrefixes = originalAutoBanPathPrefixes
		updateIPBlacklistList = originalUpdate
		ipAutoBanNow = originalNow
		ipAutoBanCounters = make(map[string]ipAutoBanCounter)
	})

	setting.Enabled = false
	setting.List = ""
	setting.AutoBanEnabled = true
	setting.AutoBanRpm = 2
	setting.AutoBanWhitelist = "127.0.0.1"
	setting.AutoBanScope = system_setting.IPAutoBanScopeAll
	setting.AutoBanPathPrefixes = ""
	updateIPBlacklistList = func(key string, value string) error {
		if key != "ip_blacklist_setting.list" {
			t.Fatalf("key = %s, want ip_blacklist_setting.list", key)
		}
		setting.List = value
		return nil
	}
	ipAutoBanNow = func() time.Time {
		return time.Unix(240, 0)
	}
	ipAutoBanCounters = make(map[string]ipAutoBanCounter)

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Real-IP", "203.0.113.44")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if i == 1 && w.Code != http.StatusOK {
			t.Fatalf("first status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		if i == 2 && w.Code != http.StatusForbidden {
			t.Fatalf("second status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
		}
	}

	if setting.List != "203.0.113.44" {
		t.Fatalf("setting.List = %q, want forwarded client IP", setting.List)
	}
}

func TestIPBlacklistAllowsNonMatchingClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
	})

	setting.Enabled = true
	setting.List = "203.0.113.0/24"

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "198.51.100.10:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestIPBlacklistBlocksCloudflareConnectingIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
	})

	setting.Enabled = true
	setting.List = "203.0.113.8"

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("CF-Connecting-IP", "203.0.113.8")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestIPBlacklistBlocksRemoteAddrWhenForwardedForIsSpoofed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
	})

	setting.Enabled = true
	setting.List = "203.0.113.8"

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "203.0.113.8:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.10")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestIPBlacklistBlocksAnyForwardedForCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := system_setting.GetIPBlacklistSetting()
	originalEnabled := setting.Enabled
	originalList := setting.List
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.List = originalList
	})

	setting.Enabled = true
	setting.List = "203.0.113.8"

	router := gin.New()
	router.Use(IPBlacklist())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 203.0.113.8")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
