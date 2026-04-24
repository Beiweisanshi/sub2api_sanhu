package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/modelpricing"
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
	rows, err := r.client.ModelPricing.Query().
		Order(modelpricing.ByModelName()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model pricing overrides: %w", err)
	}

	out := make([]*service.ModelPricingOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, modelPricingToService(row))
	}
	return out, nil
}

// Upsert 新增或更新模型定价覆写。
func (r *modelPricingRepository) Upsert(ctx context.Context, e *service.ModelPricingOverride) error {
	if e == nil {
		return fmt.Errorf("model pricing override is nil")
	}
	modelName := strings.TrimSpace(e.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	now := time.Now()

	create := r.client.ModelPricing.Create().
		SetModelName(modelName).
		SetIsCustom(e.IsCustom).
		SetNote(e.Note).
		SetUpdatedAt(now)
	setCreateFloat(create.SetInputCostPerToken, create.SetNillableInputCostPerToken, e.InputCostPerToken)
	setCreateFloat(create.SetOutputCostPerToken, create.SetNillableOutputCostPerToken, e.OutputCostPerToken)
	setCreateFloat(create.SetCacheReadInputTokenCost, create.SetNillableCacheReadInputTokenCost, e.CacheReadInputTokenCost)
	setCreateFloat(create.SetCacheCreationInputTokenCost, create.SetNillableCacheCreationInputTokenCost, e.CacheCreationInputTokenCost)

	upsert := create.OnConflict(sql.ConflictColumns(modelpricing.FieldModelName)).UpdateNewValues().
		SetModelName(modelName).
		SetIsCustom(e.IsCustom).
		SetNote(e.Note).
		SetUpdatedAt(now)
	setUpsertFloat(upsert.SetInputCostPerToken, upsert.ClearInputCostPerToken, e.InputCostPerToken)
	setUpsertFloat(upsert.SetOutputCostPerToken, upsert.ClearOutputCostPerToken, e.OutputCostPerToken)
	setUpsertFloat(upsert.SetCacheReadInputTokenCost, upsert.ClearCacheReadInputTokenCost, e.CacheReadInputTokenCost)
	setUpsertFloat(upsert.SetCacheCreationInputTokenCost, upsert.ClearCacheCreationInputTokenCost, e.CacheCreationInputTokenCost)

	if err := upsert.Exec(ctx); err != nil {
		return fmt.Errorf("upsert model pricing override: %w", err)
	}
	return nil
}

// DeleteByName 按模型名删除覆写。
func (r *modelPricingRepository) DeleteByName(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	_, err := r.client.ModelPricing.Delete().Where(modelpricing.ModelNameEQ(name)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete model pricing override: %w", err)
	}
	return nil
}

func modelPricingToService(row *ent.ModelPricing) *service.ModelPricingOverride {
	if row == nil {
		return nil
	}
	return &service.ModelPricingOverride{
		ModelName:                   row.ModelName,
		InputCostPerToken:           row.InputCostPerToken,
		OutputCostPerToken:          row.OutputCostPerToken,
		CacheReadInputTokenCost:     row.CacheReadInputTokenCost,
		CacheCreationInputTokenCost: row.CacheCreationInputTokenCost,
		IsCustom:                    row.IsCustom,
		Note:                        row.Note,
		CreatedAt:                   row.CreatedAt,
		UpdatedAt:                   row.UpdatedAt,
	}
}

func setCreateFloat(set func(float64) *ent.ModelPricingCreate, setNillable func(*float64) *ent.ModelPricingCreate, value *float64) {
	if value == nil {
		setNillable(nil)
		return
	}
	set(*value)
}

func setUpsertFloat(set func(float64) *ent.ModelPricingUpsertOne, clear func() *ent.ModelPricingUpsertOne, value *float64) {
	if value == nil {
		clear()
		return
	}
	set(*value)
}
