package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func TestSetupContextForTokenNormalizesModelLimitContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}
	token := &model.Token{
		ModelLimitsEnabled: true,
		ModelLimits:        " gpt-5.5, ,gpt-4o\nclaude-sonnet-4 ",
	}

	if err := SetupContextForToken(ctx, token); err != nil {
		t.Fatalf("expected token context setup to succeed: %v", err)
	}

	if !common.GetContextKeyBool(ctx, constant.ContextKeyTokenModelLimitEnabled) {
		t.Fatal("expected token model limits to be enabled in context")
	}
	rawLimits, ok := common.GetContextKey(ctx, constant.ContextKeyTokenModelLimit)
	if !ok {
		t.Fatal("expected token model limits to be set in context")
	}
	limits, ok := rawLimits.(map[string]bool)
	if !ok {
		t.Fatalf("expected token model limits map, got %T", rawLimits)
	}
	for _, modelName := range []string{"gpt-5.5", "gpt-4o", "claude-sonnet-4"} {
		if !limits[modelName] {
			t.Fatalf("expected token model limits to contain %q, got %#v", modelName, limits)
		}
	}
	if limits[""] || limits[" "] {
		t.Fatalf("expected empty token model limits to be ignored, got %#v", limits)
	}
}
