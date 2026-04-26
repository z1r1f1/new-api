package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

func TestShouldAutoTestUseStream(t *testing.T) {
	t.Run("codex channels use stream", func(t *testing.T) {
		channel := &model.Channel{Type: constant.ChannelTypeCodex}
		if !shouldUseStreamForAutomaticChannelTest(channel) {
			t.Fatalf("expected codex channel to use stream=true during auto test")
		}
	})

	t.Run("non codex channels do not use stream", func(t *testing.T) {
		channel := &model.Channel{Type: 1}
		if shouldUseStreamForAutomaticChannelTest(channel) {
			t.Fatalf("expected non-codex channel to keep stream=false during auto test")
		}
	})
}

func TestShouldDeleteChannelAfterTest(t *testing.T) {
	t.Run("matches local error", func(t *testing.T) {
		result := testResult{
			localErr: errors.New(`bad response status code 402, body: {"detail":{"code":"deactivated_workspace"}}`),
		}
		if !shouldDeleteChannelAfterTest(result) {
			t.Fatalf("expected deactivated_workspace local error to trigger deletion")
		}
	})

	t.Run("matches new api error", func(t *testing.T) {
		result := testResult{
			newAPIError: types.NewOpenAIError(
				errors.New(`bad response status code 402, body: {"detail":{"code":"deactivated_workspace"}}`),
				types.ErrorCodeBadResponse,
				402,
			),
		}
		if !shouldDeleteChannelAfterTest(result) {
			t.Fatalf("expected deactivated_workspace new api error to trigger deletion")
		}
	})

	t.Run("ignores other failures", func(t *testing.T) {
		result := testResult{
			localErr: errors.New("bad response status code 429"),
		}
		if shouldDeleteChannelAfterTest(result) {
			t.Fatalf("expected non-deactivated_workspace failure to keep channel")
		}
	})

	t.Run("manual batch can delete unauthorized", func(t *testing.T) {
		result := testResult{
			newAPIError: types.NewOpenAIError(
				errors.New("bad response status code 401"),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusUnauthorized,
			),
		}
		if channelDeletionReasonAfterTest(result, false) != "" {
			t.Fatalf("expected scheduled/non-manual batch behavior to keep 401 channel")
		}
		if got := channelDeletionReasonAfterTest(result, true); got != "status_code_401" {
			t.Fatalf("expected manual batch 401 to trigger deletion, got %q", got)
		}
	})

	t.Run("bad response 402 deletes channel", func(t *testing.T) {
		result := testResult{
			newAPIError: types.NewOpenAIError(
				errors.New("bad response status code 402"),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusPaymentRequired,
			),
		}
		if got := channelDeletionReasonAfterTest(result, false); got != "status_code_402" {
			t.Fatalf("expected automatic batch 402 to trigger deletion, got %q", got)
		}
		if got := channelDeletionReasonAfterTest(result, true); got != "status_code_402" {
			t.Fatalf("expected manual batch 402 to trigger deletion, got %q", got)
		}
	})
}
