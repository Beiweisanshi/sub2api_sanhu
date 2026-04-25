package admin

import (
	"context"
	"net/url"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelPricingHandler 处理模型定价管理请求。
//
// mkx 2026-04-24：仅暴露 DB 薄覆写层，不修改嵌入 JSON 默认定价。
type ModelPricingHandler struct {
	pricingService *service.PricingService
}

// NewModelPricingHandler 创建模型定价管理处理器。
func NewModelPricingHandler(pricingService *service.PricingService) *ModelPricingHandler {
	return &ModelPricingHandler{pricingService: pricingService}
}

type modelPricingPriceFieldsResponse struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
	FastPriceMultiplier         *float64 `json:"fast_price_multiplier"`
}

type modelPricingItemResponse struct {
	ModelName   string                           `json:"model_name"`
	Provider    string                           `json:"provider"`
	Mode        string                           `json:"mode"`
	IsCustom    bool                             `json:"is_custom"`
	HasOverride bool                             `json:"has_override"`
	Effective   modelPricingPriceFieldsResponse  `json:"effective"`
	Base        *modelPricingPriceFieldsResponse `json:"base"`
	Override    *modelPricingPriceFieldsResponse `json:"override"`
	Note        string                           `json:"note"`
	UpdatedAt   string                           `json:"updated_at,omitempty"`
}

type modelPricingUpsertRequest struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token" binding:"omitempty,min=0"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token" binding:"omitempty,min=0"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost" binding:"omitempty,min=0"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost" binding:"omitempty,min=0"`
	FastPriceMultiplier         *float64 `json:"fast_price_multiplier" binding:"omitempty,gt=0"`
	Note                        string   `json:"note" binding:"omitempty,max=1000"`
}

type modelPricingCreateRequest struct {
	ModelName string `json:"model_name" binding:"required,max=200"`
	modelPricingUpsertRequest
}

// List 返回全部模型定价，前端负责搜索和分页。
// GET /api/v1/admin/model-pricing
func (h *ModelPricingHandler) List(c *gin.Context) {
	items, err := h.pricingService.ListAllPricing(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]modelPricingItemResponse, 0, len(items))
	for i := range items {
		out = append(out, modelPricingItemToResponse(items[i]))
	}
	response.Success(c, out)
}

// Create 新增自定义模型定价。
// POST /api/v1/admin/model-pricing
func (h *ModelPricingHandler) Create(c *gin.Context) {
	var req modelPricingCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	modelName, err := validateModelPricingName(req.ModelName)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	override := requestToModelPricingOverride(modelName, req.modelPricingUpsertRequest, true)
	if err := h.pricingService.UpsertOverride(c.Request.Context(), override); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"model_name": override.ModelName})
}

// Upsert 更新嵌入模型覆写或自定义模型定价。
// PUT /api/v1/admin/model-pricing/:name
func (h *ModelPricingHandler) Upsert(c *gin.Context) {
	modelName, err := modelNameParam(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var req modelPricingUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	isCustom := !h.hasEmbeddedBase(c.Request.Context(), modelName)
	override := requestToModelPricingOverride(modelName, req, isCustom)
	if err := h.pricingService.UpsertOverride(c.Request.Context(), override); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"model_name": override.ModelName})
}

// Delete 删除覆写；自定义模型会从有效定价列表移除。
// DELETE /api/v1/admin/model-pricing/:name
func (h *ModelPricingHandler) Delete(c *gin.Context) {
	modelName, err := modelNameParam(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.pricingService.DeleteOverride(c.Request.Context(), modelName); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"model_name": modelName})
}

func (h *ModelPricingHandler) hasEmbeddedBase(ctx context.Context, modelName string) bool {
	items, err := h.pricingService.ListAllPricing(ctx)
	if err != nil {
		return false
	}
	for i := range items {
		if items[i].ModelName == modelName && items[i].Base != nil {
			return true
		}
	}
	return false
}

func modelNameParam(c *gin.Context) (string, error) {
	value, err := url.PathUnescape(c.Param("name"))
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_MODEL_NAME", "Invalid model name")
	}
	return validateModelPricingName(strings.TrimPrefix(value, "/"))
}

func validateModelPricingName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", infraerrors.BadRequest("INVALID_MODEL_NAME", "Model name is required")
	}
	if utf8.RuneCountInString(value) > 200 {
		return "", infraerrors.BadRequest("INVALID_MODEL_NAME", "Model name must be at most 200 characters")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", infraerrors.BadRequest("INVALID_MODEL_NAME", "Model name contains invalid characters")
	}
	return value, nil
}

func requestToModelPricingOverride(modelName string, req modelPricingUpsertRequest, isCustom bool) *service.ModelPricingOverride {
	return &service.ModelPricingOverride{
		ModelName:                   strings.TrimSpace(modelName),
		InputCostPerToken:           cloneRequestFloat(req.InputCostPerToken),
		OutputCostPerToken:          cloneRequestFloat(req.OutputCostPerToken),
		CacheReadInputTokenCost:     cloneRequestFloat(req.CacheReadInputTokenCost),
		CacheCreationInputTokenCost: cloneRequestFloat(req.CacheCreationInputTokenCost),
		FastPriceMultiplier:         cloneRequestFloat(req.FastPriceMultiplier),
		IsCustom:                    isCustom,
		Note:                        strings.TrimSpace(req.Note),
	}
}

func modelPricingItemToResponse(item service.ModelPricingListItem) modelPricingItemResponse {
	provider, mode := "", ""
	if item.Effective != nil {
		provider = item.Effective.LiteLLMProvider
		mode = item.Effective.Mode
	}
	updatedAt := ""
	if item.Override != nil && !item.Override.UpdatedAt.IsZero() {
		updatedAt = item.Override.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return modelPricingItemResponse{
		ModelName:   item.ModelName,
		Provider:    provider,
		Mode:        mode,
		IsCustom:    item.IsCustom,
		HasOverride: item.HasOverride,
		Effective:   pricingToPriceFields(item.Effective),
		Base:        pricingToNullablePriceFields(item.Base),
		Override:    overrideToNullablePriceFields(item.Override),
		Note:        item.Note,
		UpdatedAt:   updatedAt,
	}
}

func pricingToPriceFields(pricing *service.LiteLLMModelPricing) modelPricingPriceFieldsResponse {
	if pricing == nil {
		return modelPricingPriceFieldsResponse{}
	}
	return modelPricingPriceFieldsResponse{
		InputCostPerToken:           floatPtr(pricing.InputCostPerToken),
		OutputCostPerToken:          floatPtr(pricing.OutputCostPerToken),
		CacheReadInputTokenCost:     floatPtr(pricing.CacheReadInputTokenCost),
		CacheCreationInputTokenCost: floatPtr(pricing.CacheCreationInputTokenCost),
		FastPriceMultiplier:         positiveFloatPtr(pricing.FastPriceMultiplier),
	}
}

func pricingToNullablePriceFields(pricing *service.LiteLLMModelPricing) *modelPricingPriceFieldsResponse {
	if pricing == nil {
		return nil
	}
	fields := pricingToPriceFields(pricing)
	return &fields
}

func overrideToNullablePriceFields(override *service.ModelPricingOverride) *modelPricingPriceFieldsResponse {
	if override == nil {
		return nil
	}
	return &modelPricingPriceFieldsResponse{
		InputCostPerToken:           cloneRequestFloat(override.InputCostPerToken),
		OutputCostPerToken:          cloneRequestFloat(override.OutputCostPerToken),
		CacheReadInputTokenCost:     cloneRequestFloat(override.CacheReadInputTokenCost),
		CacheCreationInputTokenCost: cloneRequestFloat(override.CacheCreationInputTokenCost),
		FastPriceMultiplier:         cloneRequestFloat(override.FastPriceMultiplier),
	}
}

func floatPtr(value float64) *float64 {
	v := value
	return &v
}

func positiveFloatPtr(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return floatPtr(value)
}

func cloneRequestFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}
