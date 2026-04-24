package constant

import "testing"

func TestPath2RelayModePlaygroundImagesGenerations(t *testing.T) {
	got := Path2RelayMode("/pg/images/generations")
	if got != RelayModeImagesGenerations {
		t.Fatalf("expected playground image generation path to use image relay mode, got %d", got)
	}
}
