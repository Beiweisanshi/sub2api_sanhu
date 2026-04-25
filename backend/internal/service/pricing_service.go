// mkx: 改为启动时从嵌入 JSON 加载定价，删除远程同步/哈希/定时器全套逻辑 (2026-04-24)
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	modelpricing "github.com/Wei-Shaw/sub2api/resources/model-pricing"
	"go.uber.org/zap"
)

var (
	openAIModelDatePattern     = regexp.MustCompile(`-\d{8}$`)
	openAIModelBasePattern     = regexp.MustCompile(`^(gpt-\d+(?:\.\d+)?)(?:-|$)`)
	openAIGPT54FallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:               2.5e-06, // $2.5 per MTok
		OutputCostPerToken:              1.5e-05, // $15 per MTok
		CacheReadInputTokenCost:         2.5e-07, // $0.25 per MTok
		LongContextInputTokenThreshold:  272000,
		LongContextInputCostMultiplier:  2.0,
		LongContextOutputCostMultiplier: 1.5,
		LiteLLMProvider:                 "openai",
		Mode:                            "chat",
		SupportsPromptCaching:           true,
	}
	openAIGPT55FallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:               5e-06, // $5 per MTok
		InputCostPerTokenPriority:       1e-05, // $10 per MTok (2x)
		OutputCostPerToken:              3e-05, // $30 per MTok
		OutputCostPerTokenPriority:      6e-05, // $60 per MTok (2x)
		CacheReadInputTokenCost:         5e-07, // $0.5 per MTok
		CacheReadInputTokenCostPriority: 1e-06, // $1 per MTok (2x)
		FastPriceMultiplier:             2.0,
		LongContextInputTokenThreshold:  272000,
		LongContextInputCostMultiplier:  2.0,
		LongContextOutputCostMultiplier: 1.5,
		LiteLLMProvider:                 "openai",
		Mode:                            "chat",
		SupportsPromptCaching:           true,
	}
	openAIGPT54MiniFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:       7.5e-07,
		OutputCostPerToken:      4.5e-06,
		CacheReadInputTokenCost: 7.5e-08,
		LiteLLMProvider:         "openai",
		Mode:                    "chat",
		SupportsPromptCaching:   true,
	}
	openAIGPT54NanoFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:       2e-07,
		OutputCostPerToken:      1.25e-06,
		CacheReadInputTokenCost: 2e-08,
		LiteLLMProvider:         "openai",
		Mode:                    "chat",
		SupportsPromptCaching:   true,
	}
)

// LiteLLMModelPricing LiteLLM价格数据结构
// 只保留我们需要的字段，使用指针来处理可能缺失的值
type LiteLLMModelPricing struct {
	InputCostPerToken                   float64 `json:"input_cost_per_token"`
	InputCostPerTokenPriority           float64 `json:"input_cost_per_token_priority"`
	OutputCostPerToken                  float64 `json:"output_cost_per_token"`
	OutputCostPerTokenPriority          float64 `json:"output_cost_per_token_priority"`
	CacheCreationInputTokenCost         float64 `json:"cache_creation_input_token_cost"`
	CacheCreationInputTokenCostAbove1hr float64 `json:"cache_creation_input_token_cost_above_1hr"`
	CacheReadInputTokenCost             float64 `json:"cache_read_input_token_cost"`
	CacheReadInputTokenCostPriority     float64 `json:"cache_read_input_token_cost_priority"`
	FastPriceMultiplier                 float64 `json:"fast_price_multiplier,omitempty"`
	LongContextInputTokenThreshold      int     `json:"long_context_input_token_threshold,omitempty"`
	LongContextInputCostMultiplier      float64 `json:"long_context_input_cost_multiplier,omitempty"`
	LongContextOutputCostMultiplier     float64 `json:"long_context_output_cost_multiplier,omitempty"`
	SupportsServiceTier                 bool    `json:"supports_service_tier"`
	LiteLLMProvider                     string  `json:"litellm_provider"`
	Mode                                string  `json:"mode"`
	SupportsPromptCaching               bool    `json:"supports_prompt_caching"`
	OutputCostPerImage                  float64 `json:"output_cost_per_image"`       // 图片生成模型每张图片价格
	OutputCostPerImageToken             float64 `json:"output_cost_per_image_token"` // 图片输出 token 价格
}

// LiteLLMRawEntry 用于解析原始JSON数据
type LiteLLMRawEntry struct {
	InputCostPerToken                   *float64 `json:"input_cost_per_token"`
	InputCostPerTokenPriority           *float64 `json:"input_cost_per_token_priority"`
	OutputCostPerToken                  *float64 `json:"output_cost_per_token"`
	OutputCostPerTokenPriority          *float64 `json:"output_cost_per_token_priority"`
	CacheCreationInputTokenCost         *float64 `json:"cache_creation_input_token_cost"`
	CacheCreationInputTokenCostAbove1hr *float64 `json:"cache_creation_input_token_cost_above_1hr"`
	CacheReadInputTokenCost             *float64 `json:"cache_read_input_token_cost"`
	CacheReadInputTokenCostPriority     *float64 `json:"cache_read_input_token_cost_priority"`
	FastPriceMultiplier                 *float64 `json:"fast_price_multiplier,omitempty"`
	SupportsServiceTier                 bool     `json:"supports_service_tier"`
	LiteLLMProvider                     string   `json:"litellm_provider"`
	Mode                                string   `json:"mode"`
	SupportsPromptCaching               bool     `json:"supports_prompt_caching"`
	OutputCostPerImage                  *float64 `json:"output_cost_per_image"`
	OutputCostPerImageToken             *float64 `json:"output_cost_per_image_token"`
}

// PricingService 定价服务，定价数据在编译期嵌入二进制。
// ModelPricingOverride 是数据库中的模型定价覆写 DTO。
//
// mkx 2026-04-24：接口放在 service 包，仓储实现反向依赖该 DTO，避免 service 依赖 repository。
type ModelPricingOverride struct {
	ModelName                   string
	InputCostPerToken           *float64
	OutputCostPerToken          *float64
	CacheReadInputTokenCost     *float64
	CacheCreationInputTokenCost *float64
	IsCustom                    bool
	Note                        string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

// ModelPricingRepository 定义模型定价覆写仓储接口。
type ModelPricingRepository interface {
	List(ctx context.Context) ([]*ModelPricingOverride, error)
	Upsert(ctx context.Context, e *ModelPricingOverride) error
	DeleteByName(ctx context.Context, name string) error
}

// ModelPricingListItem 是管理页展示用的定价聚合行。
type ModelPricingListItem struct {
	ModelName   string
	Base        *LiteLLMModelPricing
	Override    *ModelPricingOverride
	Effective   *LiteLLMModelPricing
	IsCustom    bool
	HasOverride bool
	Note        string
}

// PricingService 定价服务，定价数据在编译期嵌入二进制。
type PricingService struct {
	cfg          *config.Config
	overrideRepo ModelPricingRepository
	mu           sync.RWMutex
	baseData     map[string]*LiteLLMModelPricing
	pricingData  map[string]*LiteLLMModelPricing
	overrides    map[string]*ModelPricingOverride
	loadedAt     time.Time
}

// NewPricingService 创建价格服务
func NewPricingService(cfg *config.Config, overrideRepo ModelPricingRepository) *PricingService {
	return &PricingService{
		cfg:          cfg,
		overrideRepo: overrideRepo,
		baseData:     make(map[string]*LiteLLMModelPricing),
		pricingData:  make(map[string]*LiteLLMModelPricing),
		overrides:    make(map[string]*ModelPricingOverride),
	}
}

// Initialize 从嵌入 JSON 一次性加载定价数据
func (s *PricingService) Initialize() error {
	data, err := s.parsePricingData(modelpricing.JSON)
	if err != nil {
		return fmt.Errorf("parse embedded pricing data: %w", err)
	}

	s.mu.Lock()
	s.baseData = data
	s.pricingData = clonePricingMap(data)
	s.overrides = make(map[string]*ModelPricingOverride)
	s.loadedAt = time.Now()
	s.mu.Unlock()

	if s.overrideRepo != nil {
		if err := s.loadOverrides(context.Background()); err != nil {
			logger.LegacyPrintf("service.pricing", "[Pricing] Failed to load DB pricing overrides: %v", err)
		}
	}

	logger.LegacyPrintf("service.pricing", "[Pricing] Loaded %d models from embedded JSON", len(data))
	return nil
}

// parsePricingData 解析价格数据（处理各种格式）
func (s *PricingService) parsePricingData(body []byte) (map[string]*LiteLLMModelPricing, error) {
	// 首先解析为 map[string]json.RawMessage
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("parse raw JSON: %w", err)
	}

	result := make(map[string]*LiteLLMModelPricing)
	skipped := 0

	for modelName, rawEntry := range rawData {
		// 跳过 sample_spec 等文档条目
		if modelName == "sample_spec" {
			continue
		}

		// 尝试解析每个条目
		var entry LiteLLMRawEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			skipped++
			continue
		}

		// 只保留有有效价格的条目
		if entry.InputCostPerToken == nil && entry.OutputCostPerToken == nil {
			continue
		}

		pricing := &LiteLLMModelPricing{
			LiteLLMProvider:       entry.LiteLLMProvider,
			Mode:                  entry.Mode,
			SupportsPromptCaching: entry.SupportsPromptCaching,
			SupportsServiceTier:   entry.SupportsServiceTier,
		}

		if entry.InputCostPerToken != nil {
			pricing.InputCostPerToken = *entry.InputCostPerToken
		}
		if entry.InputCostPerTokenPriority != nil {
			pricing.InputCostPerTokenPriority = *entry.InputCostPerTokenPriority
		}
		if entry.OutputCostPerToken != nil {
			pricing.OutputCostPerToken = *entry.OutputCostPerToken
		}
		if entry.OutputCostPerTokenPriority != nil {
			pricing.OutputCostPerTokenPriority = *entry.OutputCostPerTokenPriority
		}
		if entry.CacheCreationInputTokenCost != nil {
			pricing.CacheCreationInputTokenCost = *entry.CacheCreationInputTokenCost
		}
		if entry.CacheCreationInputTokenCostAbove1hr != nil {
			pricing.CacheCreationInputTokenCostAbove1hr = *entry.CacheCreationInputTokenCostAbove1hr
		}
		if entry.CacheReadInputTokenCost != nil {
			pricing.CacheReadInputTokenCost = *entry.CacheReadInputTokenCost
		}
		if entry.CacheReadInputTokenCostPriority != nil {
			pricing.CacheReadInputTokenCostPriority = *entry.CacheReadInputTokenCostPriority
		}
		if entry.OutputCostPerImage != nil {
			pricing.OutputCostPerImage = *entry.OutputCostPerImage
		}
		if entry.OutputCostPerImageToken != nil {
			pricing.OutputCostPerImageToken = *entry.OutputCostPerImageToken
		}

		result[modelName] = pricing
	}

	if skipped > 0 {
		logger.LegacyPrintf("service.pricing", "[Pricing] Skipped %d invalid entries", skipped)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid pricing entries found")
	}

	return result, nil
}

// ReloadOverrides 重新加载数据库覆写并热刷新内存定价。
//
// mkx 2026-04-24：写入 DB 后立即调用，保证管理页变更不需要重启服务。
func (s *PricingService) ReloadOverrides(ctx context.Context) error {
	return s.loadOverrides(ctx)
}

func (s *PricingService) loadOverrides(ctx context.Context) error {
	if s.overrideRepo == nil {
		s.mu.Lock()
		s.pricingData = clonePricingMap(s.baseData)
		s.overrides = make(map[string]*ModelPricingOverride)
		s.loadedAt = time.Now()
		s.mu.Unlock()
		return nil
	}

	overrides, err := s.overrideRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("list pricing overrides: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	merged := clonePricingMap(s.baseData)
	indexed := make(map[string]*ModelPricingOverride, len(overrides))
	for _, override := range overrides {
		if override == nil {
			continue
		}
		modelName := strings.TrimSpace(override.ModelName)
		if modelName == "" {
			continue
		}

		copyOverride := cloneModelPricingOverride(override)
		copyOverride.ModelName = modelName
		indexed[modelName] = copyOverride

		base, hasBase := merged[modelName]
		if !hasBase {
			if !override.IsCustom {
				continue
			}
			base = &LiteLLMModelPricing{LiteLLMProvider: "custom", Mode: "custom"}
		}

		merged[modelName] = applyModelPricingOverride(base, copyOverride)
	}

	s.pricingData = merged
	s.overrides = indexed
	s.loadedAt = time.Now()
	logger.LegacyPrintf("service.pricing", "[Pricing] Applied %d model pricing overrides", len(indexed))
	return nil
}

// ListAllPricing 返回管理页可见的全部模型定价。
func (s *PricingService) ListAllPricing(ctx context.Context) ([]ModelPricingListItem, error) {
	if err := s.ReloadOverrides(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]ModelPricingListItem, 0, len(s.pricingData))
	for modelName, effective := range s.pricingData {
		base := clonePricing(s.baseData[modelName])
		override := cloneModelPricingOverride(s.overrides[modelName])
		items = append(items, ModelPricingListItem{
			ModelName:   modelName,
			Base:        base,
			Override:    override,
			Effective:   clonePricing(effective),
			IsCustom:    override != nil && override.IsCustom && base == nil,
			HasOverride: override != nil,
			Note:        noteFromOverride(override),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].ModelName) < strings.ToLower(items[j].ModelName)
	})
	return items, nil
}

// UpsertOverride 保存模型定价覆写并刷新内存定价。
func (s *PricingService) UpsertOverride(ctx context.Context, override *ModelPricingOverride) error {
	if s.overrideRepo == nil {
		return fmt.Errorf("model pricing repository is not configured")
	}
	if override == nil || strings.TrimSpace(override.ModelName) == "" {
		return fmt.Errorf("model name is required")
	}
	override.ModelName = strings.TrimSpace(override.ModelName)
	if err := s.overrideRepo.Upsert(ctx, override); err != nil {
		return err
	}
	return s.ReloadOverrides(ctx)
}

// DeleteOverride 删除模型定价覆写；自定义模型会同时从内存定价中移除。
func (s *PricingService) DeleteOverride(ctx context.Context, name string) error {
	if s.overrideRepo == nil {
		return fmt.Errorf("model pricing repository is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	if err := s.overrideRepo.DeleteByName(ctx, name); err != nil {
		return err
	}
	return s.ReloadOverrides(ctx)
}

// Stop 保留给 Wire cleanup 调用，当前定价服务无后台任务需要关闭。
func (s *PricingService) Stop() {}

func applyModelPricingOverride(base *LiteLLMModelPricing, override *ModelPricingOverride) *LiteLLMModelPricing {
	pricing := clonePricing(base)
	if pricing == nil {
		pricing = &LiteLLMModelPricing{LiteLLMProvider: "custom", Mode: "custom"}
	}
	if override == nil {
		return pricing
	}
	if override.InputCostPerToken != nil {
		pricing.InputCostPerToken = *override.InputCostPerToken
	}
	if override.OutputCostPerToken != nil {
		pricing.OutputCostPerToken = *override.OutputCostPerToken
	}
	if override.CacheReadInputTokenCost != nil {
		pricing.CacheReadInputTokenCost = *override.CacheReadInputTokenCost
	}
	if override.CacheCreationInputTokenCost != nil {
		pricing.CacheCreationInputTokenCost = *override.CacheCreationInputTokenCost
	}
	return pricing
}

func clonePricingMap(source map[string]*LiteLLMModelPricing) map[string]*LiteLLMModelPricing {
	out := make(map[string]*LiteLLMModelPricing, len(source))
	for name, pricing := range source {
		out[name] = clonePricing(pricing)
	}
	return out
}

func clonePricing(pricing *LiteLLMModelPricing) *LiteLLMModelPricing {
	if pricing == nil {
		return nil
	}
	clone := *pricing
	return &clone
}

func cloneModelPricingOverride(override *ModelPricingOverride) *ModelPricingOverride {
	if override == nil {
		return nil
	}
	clone := *override
	clone.InputCostPerToken = cloneFloat64Ptr(override.InputCostPerToken)
	clone.OutputCostPerToken = cloneFloat64Ptr(override.OutputCostPerToken)
	clone.CacheReadInputTokenCost = cloneFloat64Ptr(override.CacheReadInputTokenCost)
	clone.CacheCreationInputTokenCost = cloneFloat64Ptr(override.CacheCreationInputTokenCost)
	return &clone
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}

func noteFromOverride(override *ModelPricingOverride) string {
	if override == nil {
		return ""
	}
	return override.Note
}

// GetModelPricing 获取模型价格（带模糊匹配）
func (s *PricingService) GetModelPricing(modelName string) *LiteLLMModelPricing {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if modelName == "" {
		return nil
	}

	// 标准化模型名称（同时兼容 "models/xxx"、VertexAI 资源名等前缀）
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	lookupCandidates := s.buildModelLookupCandidates(modelLower)

	// 1. 精确匹配
	for _, candidate := range lookupCandidates {
		if candidate == "" {
			continue
		}
		if pricing, ok := s.pricingData[candidate]; ok {
			return pricing
		}
	}

	// 2. 处理常见的模型名称变体
	// claude-opus-4-5-20251101 -> claude-opus-4.5-20251101
	for _, candidate := range lookupCandidates {
		normalized := strings.ReplaceAll(candidate, "-4-5-", "-4.5-")
		if pricing, ok := s.pricingData[normalized]; ok {
			return pricing
		}
	}

	// 3. 尝试模糊匹配（去掉版本号后缀）
	// claude-opus-4-5-20251101 -> claude-opus-4.5
	baseName := s.extractBaseName(lookupCandidates[0])
	for key, pricing := range s.pricingData {
		keyBase := s.extractBaseName(strings.ToLower(key))
		if keyBase == baseName {
			return pricing
		}
	}

	// 4. 基于模型系列匹配（Claude）
	if pricing := s.matchByModelFamily(lookupCandidates[0]); pricing != nil {
		return pricing
	}

	// 5. OpenAI 模型回退策略
	if strings.HasPrefix(lookupCandidates[0], "gpt-") {
		return s.matchOpenAIModel(lookupCandidates[0])
	}

	return nil
}

func (s *PricingService) buildModelLookupCandidates(modelLower string) []string {
	// Prefer canonical model name first (this also improves billing compatibility with "models/xxx").
	candidates := []string{
		normalizeModelNameForPricing(modelLower),
		modelLower,
	}
	candidates = append(candidates,
		strings.TrimPrefix(modelLower, "models/"),
		lastSegment(modelLower),
		lastSegment(strings.TrimPrefix(modelLower, "models/")),
	)

	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	if len(out) == 0 {
		return []string{modelLower}
	}
	return out
}

func normalizeModelNameForPricing(model string) string {
	// Common Gemini/VertexAI forms:
	// - models/gemini-2.0-flash-exp
	// - publishers/google/models/gemini-2.5-pro
	// - projects/.../locations/.../publishers/google/models/gemini-2.5-pro
	model = strings.TrimSpace(model)
	model = strings.TrimLeft(model, "/")
	model = strings.TrimPrefix(model, "models/")
	model = strings.TrimPrefix(model, "publishers/google/models/")

	if idx := strings.LastIndex(model, "/publishers/google/models/"); idx != -1 {
		model = model[idx+len("/publishers/google/models/"):]
	}
	if idx := strings.LastIndex(model, "/models/"); idx != -1 {
		model = model[idx+len("/models/"):]
	}

	model = strings.TrimLeft(model, "/")
	return model
}

func lastSegment(model string) string {
	if idx := strings.LastIndex(model, "/"); idx != -1 {
		return model[idx+1:]
	}
	return model
}

// extractBaseName 提取基础模型名称（去掉日期版本号）
func (s *PricingService) extractBaseName(model string) string {
	// 移除日期后缀 (如 -20251101, -20241022)
	parts := strings.Split(model, "-")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		// 跳过看起来像日期的部分（8位数字）
		if len(part) == 8 && isNumeric(part) {
			continue
		}
		// 跳过版本号（如 v1:0）
		if strings.Contains(part, ":") {
			continue
		}
		result = append(result, part)
	}
	return strings.Join(result, "-")
}

// matchByModelFamily 基于模型系列匹配
func (s *PricingService) matchByModelFamily(model string) *LiteLLMModelPricing {
	// modelFamily 定义一个模型系列的匹配和定价查找规则。
	type modelFamily struct {
		name    string   // 系列名称
		match   []string // 用于将模型归类到此系列的模式（strings.Contains 匹配）
		pricing []string // 用于在定价数据中查找价格的模式（nil 则复用 match；可包含低版本 fallback）
	}

	// 按特异性降序排列：高版本号在前，避免 "claude-opus-4"（opus-4 系列）
	// 因子串关系误匹配 "claude-opus-4-7"（opus-4.7 系列）。
	// 注意：原 map 实现存在 Go map 迭代随机性导致的同类 bug，此处改为有序切片修复。
	families := []modelFamily{
		{name: "opus-4.7", match: []string{"claude-opus-4-7", "claude-opus-4.7"}, pricing: []string{"claude-opus-4-7", "claude-opus-4.7", "claude-opus-4-6"}},
		{name: "opus-4.6", match: []string{"claude-opus-4-6", "claude-opus-4.6"}},
		{name: "opus-4.5", match: []string{"claude-opus-4-5", "claude-opus-4.5"}},
		{name: "opus-4", match: []string{"claude-opus-4", "claude-3-opus"}},
		{name: "sonnet-4.5", match: []string{"claude-sonnet-4-5", "claude-sonnet-4.5"}},
		{name: "sonnet-4", match: []string{"claude-sonnet-4", "claude-3-5-sonnet"}},
		{name: "sonnet-3.5", match: []string{"claude-3-5-sonnet", "claude-3.5-sonnet"}},
		{name: "sonnet-3", match: []string{"claude-3-sonnet"}},
		{name: "haiku-3.5", match: []string{"claude-3-5-haiku", "claude-3.5-haiku"}},
		{name: "haiku-3", match: []string{"claude-3-haiku"}},
	}

	// Phase 1: 按有序切片归类（最具体的系列优先匹配）
	var matched *modelFamily
	for i := range families {
		for _, pattern := range families[i].match {
			if strings.Contains(model, pattern) || strings.Contains(model, strings.ReplaceAll(pattern, "-", "")) {
				matched = &families[i]
				break
			}
		}
		if matched != nil {
			break
		}
	}

	// Phase 2: 二次兜底——当模型 ID 不含已知模式串时，按关键字粗分
	if matched == nil {
		var fallbackName string
		switch {
		case strings.Contains(model, "opus"):
			switch {
			case strings.Contains(model, "4.7") || strings.Contains(model, "4-7"):
				fallbackName = "opus-4.7"
			case strings.Contains(model, "4.6") || strings.Contains(model, "4-6"):
				fallbackName = "opus-4.6"
			case strings.Contains(model, "4.5") || strings.Contains(model, "4-5"):
				fallbackName = "opus-4.5"
			default:
				fallbackName = "opus-4"
			}
		case strings.Contains(model, "sonnet"):
			switch {
			case strings.Contains(model, "4.5") || strings.Contains(model, "4-5"):
				fallbackName = "sonnet-4.5"
			case strings.Contains(model, "3-5") || strings.Contains(model, "3.5"):
				fallbackName = "sonnet-3.5"
			default:
				fallbackName = "sonnet-4"
			}
		case strings.Contains(model, "haiku"):
			switch {
			case strings.Contains(model, "3-5") || strings.Contains(model, "3.5"):
				fallbackName = "haiku-3.5"
			default:
				fallbackName = "haiku-3"
			}
		}
		if fallbackName != "" {
			for i := range families {
				if families[i].name == fallbackName {
					matched = &families[i]
					break
				}
			}
		}
	}

	if matched == nil {
		return nil
	}

	// Phase 3: 在定价数据中查找该系列的价格
	lookups := matched.pricing
	if lookups == nil {
		lookups = matched.match
	}
	for _, pattern := range lookups {
		for key, pricing := range s.pricingData {
			keyLower := strings.ToLower(key)
			if strings.Contains(keyLower, pattern) {
				logger.LegacyPrintf("service.pricing", "[Pricing] Fuzzy matched %s -> %s", model, key)
				return pricing
			}
		}
	}

	return nil
}

// matchOpenAIModel OpenAI 模型回退匹配策略
// 回退顺序：
// 1. gpt-5.3-codex-spark* -> gpt-5.1-codex（按业务要求固定计费）
// 2. gpt-5.2-codex -> gpt-5.2（去掉后缀如 -codex, -mini, -max 等）
// 3. gpt-5.2-20251222 -> gpt-5.2（去掉日期版本号）
// 4. gpt-5.3-codex -> gpt-5.2-codex
// 5. gpt-5.4* -> 业务静态兜底价
// 6. 最终回退到 DefaultTestModel (gpt-5.1-codex)
func (s *PricingService) matchOpenAIModel(model string) *LiteLLMModelPricing {
	if strings.HasPrefix(model, "gpt-5.3-codex-spark") {
		if pricing, ok := s.pricingData["gpt-5.1-codex"]; ok {
			logger.LegacyPrintf("service.pricing", "[Pricing][SparkBilling] %s -> %s billing", model, "gpt-5.1-codex")
			logger.With(zap.String("component", "service.pricing")).
				Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.1-codex"))
			return pricing
		}
	}

	// 尝试的回退变体
	variants := s.generateOpenAIModelVariants(model, openAIModelDatePattern)

	for _, variant := range variants {
		if pricing, ok := s.pricingData[variant]; ok {
			logger.With(zap.String("component", "service.pricing")).
				Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, variant))
			return pricing
		}
	}

	if strings.HasPrefix(model, "gpt-5.3-codex") {
		if pricing, ok := s.pricingData["gpt-5.2-codex"]; ok {
			logger.With(zap.String("component", "service.pricing")).
				Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.2-codex"))
			return pricing
		}
	}

	// GPT-5.5 回退到独立定价
	if strings.HasPrefix(model, "gpt-5.5") {
		logger.With(zap.String("component", "service.pricing")).
			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.5(static)"))
		return openAIGPT55FallbackPricing
	}

	if strings.HasPrefix(model, "gpt-5.4-mini") {
		logger.With(zap.String("component", "service.pricing")).
			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4-mini(static)"))
		return openAIGPT54MiniFallbackPricing
	}

	if strings.HasPrefix(model, "gpt-5.4-nano") {
		logger.With(zap.String("component", "service.pricing")).
			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4-nano(static)"))
		return openAIGPT54NanoFallbackPricing
	}

	if strings.HasPrefix(model, "gpt-5.4") {
		logger.With(zap.String("component", "service.pricing")).
			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4(static)"))
		return openAIGPT54FallbackPricing
	}

	// 最终回退到 DefaultTestModel
	defaultModel := strings.ToLower(openai.DefaultTestModel)
	if pricing, ok := s.pricingData[defaultModel]; ok {
		logger.LegacyPrintf("service.pricing", "[Pricing] OpenAI fallback to default model %s -> %s", model, defaultModel)
		return pricing
	}

	return nil
}

// generateOpenAIModelVariants 生成 OpenAI 模型的回退变体列表
func (s *PricingService) generateOpenAIModelVariants(model string, datePattern *regexp.Regexp) []string {
	seen := make(map[string]bool)
	var variants []string

	addVariant := func(v string) {
		if v != model && !seen[v] {
			seen[v] = true
			variants = append(variants, v)
		}
	}

	// 1. 去掉日期版本号: gpt-5.2-20251222 -> gpt-5.2
	withoutDate := datePattern.ReplaceAllString(model, "")
	if withoutDate != model {
		addVariant(withoutDate)
	}

	// 2. 提取基础版本号: gpt-5.2-codex -> gpt-5.2
	// 只匹配纯数字版本号格式 gpt-X 或 gpt-X.Y，不匹配 gpt-4o 这种带字母后缀的
	if matches := openAIModelBasePattern.FindStringSubmatch(model); len(matches) > 1 {
		addVariant(matches[1])
	}

	// 3. 同时去掉日期后再提取基础版本号
	if withoutDate != model {
		if matches := openAIModelBasePattern.FindStringSubmatch(withoutDate); len(matches) > 1 {
			addVariant(matches[1])
		}
	}

	return variants
}

// GetStatus 获取服务状态
func (s *PricingService) GetStatus() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]any{
		"model_count": len(s.pricingData),
		"source":      "embedded",
		"loaded_at":   s.loadedAt,
	}
}

// isNumeric 检查字符串是否为纯数字
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
