package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func TestShouldAutoTestUseStream(t *testing.T) {
	t.Run("codex channels use stream", func(t *testing.T) {
		channel := &model.Channel{Type: constant.ChannelTypeCodex}
		if !shouldAutoTestUseStream(channel) {
			t.Fatalf("expected codex channel to use stream=true during auto test")
		}
	})

	t.Run("non codex channels do not use stream", func(t *testing.T) {
		channel := &model.Channel{Type: 1}
		if shouldAutoTestUseStream(channel) {
			t.Fatalf("expected non-codex channel to keep stream=false during auto test")
		}
	})
}
