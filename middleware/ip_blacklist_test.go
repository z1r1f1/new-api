package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

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
