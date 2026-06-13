package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCacheRequiresStaticAssetsToRevalidate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(Cache())
	router.GET("/static/js/app.js", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/js/app.js", nil)
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-cache")
	}
	if got := w.Header().Get("Cache-Version"); got == "" {
		t.Fatal("Cache-Version header is empty")
	}
}

func TestCacheKeepsIndexUncached(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(Cache())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-cache")
	}
}
