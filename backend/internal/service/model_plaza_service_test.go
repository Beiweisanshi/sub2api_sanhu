// mkx: 覆盖模型广场倍率和价格计算，2026-04-24
package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakePlazaGroupLister struct {
	groups []Group
	rates  map[int64]float64
}

func (f fakePlazaGroupLister) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return f.groups, nil
}

func (f fakePlazaGroupLister) GetUserGroupRates(context.Context, int64) (map[int64]float64, error) {
	return f.rates, nil
}

type fakePlazaPricingGetter struct {
	prices map[string]*LiteLLMModelPricing
}

func (f fakePlazaPricingGetter) GetModelPricing(modelName string) *LiteLLMModelPricing {
	return f.prices[modelName]
}

type fakePlazaAccountLister struct {
	accounts map[int64][]Account
}

func (f fakePlazaAccountLister) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	out := make([]Account, 0)
	for _, account := range f.accounts[groupID] {
		if account.Platform == platform {
			out = append(out, account)
		}
	}
	return out, nil
}

func TestModelPlazaBuildUserPlaza_MultiplierAndPriceMath(t *testing.T) {
	svc := newModelPlazaService(
		fakePlazaGroupLister{
			groups: []Group{{
				ID:                   11,
				Name:                 "Gemini Images",
				Platform:             domain.PlatformGemini,
				RateMultiplier:       1.5,
				SupportedModelScopes: []string{"gemini_image"},
				SortOrder:            2,
			}},
			rates: map[int64]float64{11: 2},
		},
		nil,
		fakePlazaPricingGetter{prices: map[string]*LiteLLMModelPricing{
			"gemini-2.5-flash-image": {
				InputCostPerToken:               1e-6,
				OutputCostPerToken:              2e-6,
				CacheReadInputTokenCost:         0.5e-6,
				InputCostPerTokenPriority:       3e-6,
				OutputCostPerTokenPriority:      4e-6,
				CacheReadInputTokenCostPriority: 0.75e-6,
				OutputCostPerImage:              0.04,
				Mode:                            "image_generation",
			},
		}},
	)

	got, err := svc.BuildUserPlaza(context.Background(), 1001)
	require.NoError(t, err)
	require.Equal(t, "USD", got.Currency)
	require.Len(t, got.Groups, 1)

	group := got.Groups[0]
	require.Equal(t, 1.5, group.DefaultMultiplier)
	require.Equal(t, 2.0, group.EffectiveMultiplier)
	require.True(t, group.HasPersonalRate)
	require.Len(t, group.Models, 1)

	model := group.Models[0]
	require.Equal(t, "gemini-2.5-flash-image", model.Name)
	require.Equal(t, "image_generation", model.Mode)
	require.Equal(t, PlazaModelPrice{
		InputPerMTok:             1,
		OutputPerMTok:            2,
		CacheReadPerMTok:         0.5,
		InputPriorityPerMTok:     3,
		OutputPriorityPerMTok:    4,
		CacheReadPriorityPerMTok: 0.75,
		OutputImagePerImage:      0.04,
	}, model.Original)
	require.Equal(t, PlazaModelPrice{
		InputPerMTok:             2,
		OutputPerMTok:            4,
		CacheReadPerMTok:         1,
		InputPriorityPerMTok:     6,
		OutputPriorityPerMTok:    8,
		CacheReadPriorityPerMTok: 1.5,
		OutputImagePerImage:      0.08,
	}, model.Actual)
}

func TestModelPlazaBuildUserPlaza_UsesSchedulableAccountModelWhitelist(t *testing.T) {
	svc := newModelPlazaService(
		fakePlazaGroupLister{
			groups: []Group{{
				ID:             21,
				Name:           "max",
				Platform:       domain.PlatformOpenAI,
				RateMultiplier: 1,
			}},
			rates: map[int64]float64{},
		},
		fakePlazaAccountLister{accounts: map[int64][]Account{
			21: {{
				ID:       101,
				Platform: domain.PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4":       "gpt-5.4",
						"gpt-5.4-mini":  "gpt-5.4-mini",
						"gpt-5.3-codex": "gpt-5.3-codex",
						"gpt-5.2":       "gpt-5.2",
					},
				},
			}},
		}},
		fakePlazaPricingGetter{prices: map[string]*LiteLLMModelPricing{
			"gpt-5.4":       {InputCostPerToken: 1e-6},
			"gpt-5.4-mini":  {InputCostPerToken: 1e-6},
			"gpt-5.3-codex": {InputCostPerToken: 1e-6},
			"gpt-5.2":       {InputCostPerToken: 1e-6},
			"gpt-4o":        {InputCostPerToken: 1e-6},
		}},
	)

	got, err := svc.BuildUserPlaza(context.Background(), 1001)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Models, 4)
	require.Equal(t, []string{"gpt-5.2", "gpt-5.3-codex", "gpt-5.4", "gpt-5.4-mini"}, plazaModelNames(got.Groups[0].Models))
}

func plazaModelNames(models []PlazaModel) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		out = append(out, model.Name)
	}
	return out
}
