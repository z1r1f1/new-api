package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func TestNormalizePlaygroundDebugID(t *testing.T) {
	if got := NormalizePlaygroundDebugID(" abc-123_DEF "); got != "abc-123_DEF" {
		t.Fatalf("unexpected normalized debug id: %q", got)
	}
	if got := NormalizePlaygroundDebugID("bad/id"); got != "" {
		t.Fatalf("expected invalid debug id to be rejected, got %q", got)
	}
	if got := NormalizePlaygroundDebugID(strings.Repeat("a", maxPlaygroundDebugIDLength+1)); got != "" {
		t.Fatalf("expected overlong debug id to be rejected, got %q", got)
	}
}

func TestRecordPlaygroundUpstreamRequestDebug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &gin.Context{}
	c.Set("id", 42)
	common.SetContextKey(c, constant.ContextKeyPlaygroundDebugId, "debug-1")

	body := []byte(`{"model":"demo","image":"data:image/png;base64,AAAA"}`)
	RecordPlaygroundUpstreamRequestDebug(c, body)

	debug, ok := GetPlaygroundUpstreamRequestDebug(42, "debug-1")
	if !ok {
		t.Fatal("expected playground upstream debug to be stored")
	}
	if debug.BodyBytes != len(body) {
		t.Fatalf("unexpected body bytes: got %d want %d", debug.BodyBytes, len(body))
	}
	request, ok := debug.UpstreamRequest.(map[string]any)
	if !ok {
		t.Fatalf("expected parsed upstream request map, got %T", debug.UpstreamRequest)
	}
	if request["model"] != "demo" {
		t.Fatalf("unexpected model: %v", request["model"])
	}
	image, _ := request["image"].(string)
	if !strings.Contains(image, "data:image/png;base64,[redacted") {
		t.Fatalf("expected data url to be redacted, got %q", image)
	}
}
