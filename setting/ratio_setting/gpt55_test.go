package ratio_setting

import "testing"

func TestGPT55DefaultRatios(t *testing.T) {
	InitRatioSettings()

	modelRatio, hasRatio, _ := GetModelRatio("gpt-5.5")
	if !hasRatio {
		t.Fatal("expected model ratio for gpt-5.5")
	}
	if modelRatio != 0.625 {
		t.Fatalf("expected gpt-5.5 model ratio 0.625, got %v", modelRatio)
	}

	cacheRatio, ok := GetCacheRatio("gpt-5.5")
	if !ok {
		t.Fatal("expected cache ratio for gpt-5.5")
	}
	if cacheRatio != 0.1 {
		t.Fatalf("expected gpt-5.5 cache ratio 0.1, got %v", cacheRatio)
	}
}
