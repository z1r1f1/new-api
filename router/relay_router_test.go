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

func TestSetRelayRouterRegistersResponsesWebSocketCompatibilityRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetRelayRouter(r)

	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	if !routes["POST /v1/responses"] {
		t.Fatal("expected canonical OpenAI Responses route to be registered")
	}
	if !routes["GET /v1/responses"] {
		t.Fatal("expected OpenAI Responses websocket compatibility route to be registered")
	}
}

func TestSetRelayRouterRegistersPlaygroundImageEditRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetRelayRouter(r)

	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	if !routes["POST /pg/images/generations"] {
		t.Fatal("expected Playground image generation route to be registered")
	}
	if !routes["POST /pg/images/edits"] {
		t.Fatal("expected Playground image edit route to be registered")
	}
}
