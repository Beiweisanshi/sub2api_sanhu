package repository

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type modelPricingRepository struct {
	client *ent.Client
}

// NewModelPricingRepository 创建模型定价覆写仓储。
//
// mkx 2026-04-24：返回 service 包接口，保持 handler→service→repository→ent 分层方向。
func NewModelPricingRepository(client *ent.Client) service.ModelPricingRepository {
	return &modelPricingRepository{client: client}
}

// List 获取全部模型定价覆写。
func (r *modelPricingRepository) List(ctx context.Context) ([]*service.ModelPricingOverride, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("model pricing repository is not initialized")
	}

	var rows entsql.Rows
	if err := r.client.Driver().Query(ctx, `
SELECT model_name, input_cost_per_token, output_cost_per_token,
       cache_read_input_token_cost, cache_creation_input_token_cost,
       fast_price_multiplier, is_custom, note, created_at, updated_at
FROM model_pricings
ORDER BY model_name`, []any{}, &rows); err != nil {
		return nil, fmt.Errorf("list model pricing overrides: %w", err)
	}
	defer rows.Close()

	out := make([]*service.ModelPricingOverride, 0)
	for rows.Next() {
		item, err := scanModelPricingOverride(&rows)
		if err != nil {
			return nil, fmt.Errorf("scan model pricing override: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model pricing overrides: %w", err)
	}
	return out, nil
}

// Upsert 新增或更新模型定价覆写。
func (r *modelPricingRepository) Upsert(ctx context.Context, e *service.ModelPricingOverride) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("model pricing repository is not initialized")
	}
	if e == nil {
		return fmt.Errorf("model pricing override is nil")
	}
	modelName := strings.TrimSpace(e.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	now := time.Now()

	var result entsql.Result
	if err := r.client.Driver().Exec(ctx, `
INSERT INTO model_pricings (
    model_name, input_cost_per_token, output_cost_per_token,
    cache_read_input_token_cost, cache_creation_input_token_cost,
    fast_price_multiplier, is_custom, note, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (model_name) DO UPDATE SET
    input_cost_per_token = EXCLUDED.input_cost_per_token,
    output_cost_per_token = EXCLUDED.output_cost_per_token,
    cache_read_input_token_cost = EXCLUDED.cache_read_input_token_cost,
    cache_creation_input_token_cost = EXCLUDED.cache_creation_input_token_cost,
    fast_price_multiplier = EXCLUDED.fast_price_multiplier,
    is_custom = EXCLUDED.is_custom,
    note = EXCLUDED.note,
    updated_at = EXCLUDED.updated_at`,
		[]any{
			modelName,
			nullableFloat(e.InputCostPerToken),
			nullableFloat(e.OutputCostPerToken),
			nullableFloat(e.CacheReadInputTokenCost),
			nullableFloat(e.CacheCreationInputTokenCost),
			nullableFloat(e.FastPriceMultiplier),
			e.IsCustom,
			e.Note,
			now,
		},
		&result,
	); err != nil {
		return fmt.Errorf("upsert model pricing override: %w", err)
	}
	return nil
}

// DeleteByName 按模型名删除覆写。
func (r *modelPricingRepository) DeleteByName(ctx context.Context, name string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("model pricing repository is not initialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	var result entsql.Result
	if err := r.client.Driver().Exec(ctx, `DELETE FROM model_pricings WHERE model_name = $1`, []any{name}, &result); err != nil {
		return fmt.Errorf("delete model pricing override: %w", err)
	}
	return nil
}

func scanModelPricingOverride(rows *entsql.Rows) (*service.ModelPricingOverride, error) {
	var (
		modelName      string
		inputPrice     stdsql.NullFloat64
		outputPrice    stdsql.NullFloat64
		cacheRead      stdsql.NullFloat64
		cacheCreation  stdsql.NullFloat64
		fastMultiplier stdsql.NullFloat64
		isCustom       bool
		note           string
		createdAt      time.Time
		updatedAt      time.Time
	)
	if err := rows.Scan(
		&modelName,
		&inputPrice,
		&outputPrice,
		&cacheRead,
		&cacheCreation,
		&fastMultiplier,
		&isCustom,
		&note,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	return &service.ModelPricingOverride{
		ModelName:                   modelName,
		InputCostPerToken:           nullFloatPtr(inputPrice),
		OutputCostPerToken:          nullFloatPtr(outputPrice),
		CacheReadInputTokenCost:     nullFloatPtr(cacheRead),
		CacheCreationInputTokenCost: nullFloatPtr(cacheCreation),
		FastPriceMultiplier:         nullFloatPtr(fastMultiplier),
		IsCustom:                    isCustom,
		Note:                        note,
		CreatedAt:                   createdAt,
		UpdatedAt:                   updatedAt,
	}, nil
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullFloatPtr(value stdsql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	v := value.Float64
	return &v
}
