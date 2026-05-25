package perfmetrics

import "testing"

func TestBuildSummaryModelsIncludesBucketSeries(t *testing.T) {
	models := buildSummaryModels(map[bucketKey]counters{
		{model: "gpt-b", bucketTs: 200}: {
			requestCount:   4,
			successCount:   2,
			totalLatencyMs: 400,
			outputTokens:   80,
			generationMs:   2000,
		},
		{model: "gpt-a", bucketTs: 200}: {
			requestCount:   1,
			successCount:   1,
			totalLatencyMs: 100,
			outputTokens:   10,
			generationMs:   1000,
		},
		{model: "gpt-b", bucketTs: 100}: {
			requestCount:   2,
			successCount:   2,
			totalLatencyMs: 300,
			outputTokens:   20,
			generationMs:   1000,
		},
	})

	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}
	if models[0].ModelName != "gpt-b" {
		t.Fatalf("first model = %q, want highest request-count model gpt-b", models[0].ModelName)
	}
	if models[0].SuccessRate != 66.67 {
		t.Fatalf("gpt-b success rate = %v, want 66.67", models[0].SuccessRate)
	}
	if len(models[0].Series) != 2 {
		t.Fatalf("gpt-b series len = %d, want 2", len(models[0].Series))
	}
	if models[0].Series[0].Ts != 100 || models[0].Series[1].Ts != 200 {
		t.Fatalf("gpt-b series timestamps = %#v, want sorted ascending", models[0].Series)
	}
	if models[0].Series[0].SuccessRate != 100 {
		t.Fatalf("first bucket success rate = %v, want 100", models[0].Series[0].SuccessRate)
	}
	if models[0].Series[1].SuccessRate != 50 {
		t.Fatalf("second bucket success rate = %v, want 50", models[0].Series[1].SuccessRate)
	}
}
