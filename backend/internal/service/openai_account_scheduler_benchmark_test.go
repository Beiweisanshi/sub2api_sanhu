package service

import (
	"testing"
)

func buildOpenAISchedulerBenchmarkCandidates(size int) []openAIAccountCandidate {
	if size <= 0 {
		return nil
	}
	candidates := make([]openAIAccountCandidate, 0, size)
	for i := 0; i < size; i++ {
		accountID := int64(10_000 + i)
		loadRate := (i * 17) % 100
		errorRate := float64((i*5)%100) / 100.0
		availFactor := 1.0 - float64(loadRate)/100.0
		healthFactor := 1.0 - errorRate
		if availFactor < 0.1 {
			availFactor = 0.1
		}
		if healthFactor < 0.1 {
			healthFactor = 0.1
		}
		candidates = append(candidates, openAIAccountCandidate{
			account: &Account{
				ID:       accountID,
				Priority: i % 7,
			},
			loadInfo: &AccountLoadInfo{
				AccountID:    accountID,
				LoadRate:     loadRate,
				WaitingCount: (i * 11) % 13,
			},
			weight: availFactor * healthFactor,
		})
	}
	return candidates
}

func BenchmarkOpenAIAccountSchedulerWeightedSelection(b *testing.B) {
	cases := []struct {
		name string
		size int
	}{
		{name: "n_16", size: 16},
		{name: "n_64", size: 64},
		{name: "n_256", size: 256},
	}

	req := OpenAIAccountScheduleRequest{
		SessionHash:    "bench_session",
		RequestedModel: "gpt-5.1",
	}

	for _, tc := range cases {
		candidates := buildOpenAISchedulerBenchmarkCandidates(tc.size)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result := buildOpenAIWeightedSelectionOrder(candidates, req)
				if len(result) == 0 {
					b.Fatal("unexpected empty result")
				}
			}
		})
	}
}
