package router

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetRelayRouterRegistersClaudeMessageCompatibilityAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetRelayRouter(r)

	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	if !routes["POST /v1/messages"] {
		t.Fatal("expected canonical Claude messages route to be registered")
	}
	if !routes["POST /v1/message"] {
		t.Fatal("expected singular Claude message compatibility route to be registered")
	}
}
