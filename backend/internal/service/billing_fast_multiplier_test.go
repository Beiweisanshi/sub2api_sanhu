package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBillingFastMultiplier_Gpt55PriorityIsTwoX(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 120, OutputTokens: 30, CacheCreationTokens: 12, CacheReadTokens: 8}

	baseCost, err := svc.CalculateCost("gpt-5.5", tokens, 1.0)
	require.NoError(t, err)

	priorityCost, err := svc.CalculateCostWithServiceTier("gpt-5.5", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*2, priorityCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*2, priorityCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheCreationCost*2, priorityCost.CacheCreationCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*2, priorityCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*2, priorityCost.TotalCost, 1e-10)
}

func TestBillingFastMultiplier_OverridesExplicitPriorityUnitPrices(t *testing.T) {
	svc := NewBillingService(&config.Config{}, &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"dynamic-fast-model": {
				InputCostPerToken:               1e-6,
				InputCostPerTokenPriority:       9e-6,
				OutputCostPerToken:              3e-6,
				OutputCostPerTokenPriority:      9e-6,
				CacheCreationInputTokenCost:     4e-6,
				CacheReadInputTokenCost:         7e-7,
				CacheReadInputTokenCostPriority: 9e-7,
				FastPriceMultiplier:             1.5,
			},
		},
	})
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 40, CacheReadTokens: 20}

	baseCost, err := svc.CalculateCost("dynamic-fast-model", tokens, 1.0)
	require.NoError(t, err)

	priorityCost, err := svc.CalculateCostWithServiceTier("dynamic-fast-model", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, baseCost.InputCost*1.5, priorityCost.InputCost, 1e-10)
	require.InDelta(t, baseCost.OutputCost*1.5, priorityCost.OutputCost, 1e-10)
	require.InDelta(t, baseCost.CacheCreationCost*1.5, priorityCost.CacheCreationCost, 1e-10)
	require.InDelta(t, baseCost.CacheReadCost*1.5, priorityCost.CacheReadCost, 1e-10)
	require.InDelta(t, baseCost.TotalCost*1.5, priorityCost.TotalCost, 1e-10)
}
