// mkx: 新增模型广场聚合服务，2026-04-24
package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service/plazamodels"
)

const perMillionTokens = 1_000_000

type PlazaModelPrice struct {
	InputPerMTok             float64 `json:"input_per_mtok"`
	OutputPerMTok            float64 `json:"output_per_mtok"`
	CacheReadPerMTok         float64 `json:"cache_read_per_mtok"`
	InputPriorityPerMTok     float64 `json:"input_priority_per_mtok,omitempty"`
	OutputPriorityPerMTok    float64 `json:"output_priority_per_mtok,omitempty"`
	CacheReadPriorityPerMTok float64 `json:"cache_read_priority_per_mtok,omitempty"`
	OutputImagePerImage      float64 `json:"output_image_per_image,omitempty"`
}

type PlazaModel struct {
	Name     string          `json:"name"`
	Mode     string          `json:"mode"`
	Original PlazaModelPrice `json:"original"`
	Actual   PlazaModelPrice `json:"actual"`
}

type PlazaGroup struct {
	ID                  int64        `json:"id"`
	Name                string       `json:"name"`
	Description         string       `json:"description,omitempty"`
	Platform            string       `json:"platform"`
	DefaultMultiplier   float64      `json:"default_multiplier"`
	EffectiveMultiplier float64      `json:"effective_multiplier"`
	HasPersonalRate     bool         `json:"has_personal_rate"`
	SortOrder           int          `json:"sort_order"`
	SupportedScopes     []string     `json:"supported_scopes"`
	Models              []PlazaModel `json:"models"`
}

type PlazaResponse struct {
	Currency string       `json:"currency"`
	Groups   []PlazaGroup `json:"groups"`
}

type plazaGroupLister interface {
	GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error)
	GetUserGroupRates(ctx context.Context, userID int64) (map[int64]float64, error)
}

type plazaPricingGetter interface {
	GetModelPricing(modelName string) *LiteLLMModelPricing
}

type plazaAccountLister interface {
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error)
}

type ModelPlazaService struct {
	apiKeyService  plazaGroupLister
	accountRepo    plazaAccountLister
	pricingService plazaPricingGetter
}

func NewModelPlazaService(apiKeySvc *APIKeyService, accountRepo AccountRepository, pricing *PricingService) *ModelPlazaService {
	return newModelPlazaService(apiKeySvc, accountRepo, pricing)
}

func newModelPlazaService(apiKeySvc plazaGroupLister, accountRepo plazaAccountLister, pricing plazaPricingGetter) *ModelPlazaService {
	return &ModelPlazaService{apiKeyService: apiKeySvc, accountRepo: accountRepo, pricingService: pricing}
}

func (s *ModelPlazaService) BuildUserPlaza(ctx context.Context, userID int64) (*PlazaResponse, error) {
	groups, err := s.apiKeyService.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	overrides, err := s.apiKeyService.GetUserGroupRates(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]PlazaGroup, 0, len(groups))
	for i := range groups {
		group := groups[i]
		effective, hasOverride := overrides[group.ID]
		if !hasOverride {
			effective = group.RateMultiplier
		}

		models, err := s.buildModels(ctx, group, effective)
		if err != nil {
			return nil, err
		}
		out = append(out, PlazaGroup{
			ID:                  group.ID,
			Name:                group.Name,
			Description:         group.Description,
			Platform:            group.Platform,
			DefaultMultiplier:   group.RateMultiplier,
			EffectiveMultiplier: effective,
			HasPersonalRate:     hasOverride && effective != group.RateMultiplier,
			SortOrder:           group.SortOrder,
			SupportedScopes:     append([]string(nil), group.SupportedModelScopes...),
			Models:              models,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})

	return &PlazaResponse{Currency: "USD", Groups: out}, nil
}

func (s *ModelPlazaService) buildModels(ctx context.Context, group Group, multiplier float64) ([]PlazaModel, error) {
	modelNames, err := s.resolveGroupModelNames(ctx, group)
	if err != nil {
		return nil, err
	}
	models := make([]PlazaModel, 0, len(modelNames))
	for _, name := range modelNames {
		pricing := s.pricingService.GetModelPricing(name)
		if pricing == nil {
			continue
		}
		original := plazaPriceFromPricing(pricing)
		models = append(models, PlazaModel{
			Name:     name,
			Mode:     pricing.Mode,
			Original: original,
			Actual:   multiplyPlazaPrice(original, multiplier),
		})
	}
	sort.SliceStable(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return models, nil
}

func (s *ModelPlazaService) resolveGroupModelNames(ctx context.Context, group Group) ([]string, error) {
	if s.accountRepo == nil {
		return resolvePlazaModelNames(group.Platform, group.SupportedModelScopes), nil
	}

	accounts, err := s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, group.ID, group.Platform)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	modelNames := make([]string, 0)
	hasMapping := false
	for i := range accounts {
		mapping := accounts[i].GetModelMapping()
		if len(mapping) == 0 {
			continue
		}
		hasMapping = true
		for model := range mapping {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			modelNames = append(modelNames, model)
		}
	}
	if hasMapping {
		return modelNames, nil
	}
	return resolvePlazaModelNames(group.Platform, group.SupportedModelScopes), nil
}

func resolvePlazaModelNames(platform string, scopes []string) []string {
	if len(scopes) == 0 {
		return plazamodels.ModelsByPlatform(platform)
	}

	platformNorm := strings.ToLower(strings.TrimSpace(platform))
	seen := make(map[string]struct{})
	out := make([]string, 0)
	knownScope := false
	for _, scope := range scopes {
		scopeNorm := strings.ToLower(strings.TrimSpace(scope))
		var models []string
		switch scopeNorm {
		case "claude":
			knownScope = true
			if platformNorm == domain.PlatformOpenAI {
				models = plazamodels.ModelsByPlatform(platform)
			} else {
				models = plazamodels.ModelsForScope(platform, scopeNorm)
			}
		case "gemini_text", "gemini_image":
			knownScope = true
			models = plazamodels.ModelsForScope(platform, scopeNorm)
		default:
			continue
		}
		for _, model := range models {
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			out = append(out, model)
		}
	}
	if !knownScope || len(out) == 0 {
		return plazamodels.ModelsByPlatform(platform)
	}
	return out
}

func plazaPriceFromPricing(pricing *LiteLLMModelPricing) PlazaModelPrice {
	return PlazaModelPrice{
		InputPerMTok:             pricing.InputCostPerToken * perMillionTokens,
		OutputPerMTok:            pricing.OutputCostPerToken * perMillionTokens,
		CacheReadPerMTok:         pricing.CacheReadInputTokenCost * perMillionTokens,
		InputPriorityPerMTok:     pricing.InputCostPerTokenPriority * perMillionTokens,
		OutputPriorityPerMTok:    pricing.OutputCostPerTokenPriority * perMillionTokens,
		CacheReadPriorityPerMTok: pricing.CacheReadInputTokenCostPriority * perMillionTokens,
		OutputImagePerImage:      pricing.OutputCostPerImage,
	}
}

func multiplyPlazaPrice(price PlazaModelPrice, multiplier float64) PlazaModelPrice {
	return PlazaModelPrice{
		InputPerMTok:             price.InputPerMTok * multiplier,
		OutputPerMTok:            price.OutputPerMTok * multiplier,
		CacheReadPerMTok:         price.CacheReadPerMTok * multiplier,
		InputPriorityPerMTok:     price.InputPriorityPerMTok * multiplier,
		OutputPriorityPerMTok:    price.OutputPriorityPerMTok * multiplier,
		CacheReadPriorityPerMTok: price.CacheReadPriorityPerMTok * multiplier,
		OutputImagePerImage:      price.OutputImagePerImage * multiplier,
	}
}
