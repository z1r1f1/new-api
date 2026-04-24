package codex

import "testing"

func TestModelListIncludesGPT55(t *testing.T) {
	found := false
	for _, model := range ModelList {
		if model == "gpt-5.5" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected codex ModelList to include gpt-5.5")
	}
}
