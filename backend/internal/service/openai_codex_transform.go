package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// UnsupportedUpstreamModelError 作者: mkx  变更: 2026/04/24
// 当 OAuth 账号的 model_mapping 不包含 gpt-5.4，且请求模型在 Codex 归一表里无法
// 显式命中（否则会被兜底重写为 gpt-5.4）时，normalizeOpenAIModelForUpstream
// 返回此错误。调用方应避免把请求发往上游——典型场景是客户端把 gpt-image-*
// 发到 /v1/chat/completions，而选中的账号只有 image 权限，若放行会被 ChatGPT
// 后端以 401 {"detail":"Unauthorized"} 拒绝并牵连到账号禁用逻辑。
type UnsupportedUpstreamModelError struct {
	RequestedModel string
	FallbackModel  string
	AccountID      int64
	AccountName    string
}

func (e *UnsupportedUpstreamModelError) Error() string {
	return fmt.Sprintf(
		"model %q is not supported by account %d (%s): codex normalizer would fall back to %q but the account has no model_mapping entry for it",
		e.RequestedModel, e.AccountID, e.AccountName, e.FallbackModel,
	)
}

// IsUnsupportedUpstreamModelError 便于调用方识别此错误类型，无需 errors.As 到具体指针。
func IsUnsupportedUpstreamModelError(err error) bool {
	var target *UnsupportedUpstreamModelError
	return errors.As(err, &target)
}

// resolveOpenAIUpstreamModelOrFailover 作者: mkx  变更: 2026/04/24
// HTTP 路径统一用这个 helper：UnsupportedUpstreamModelError 被翻译成
// UpstreamFailoverError{StatusCode:400}，让 handler 的 failover 循环把这个账号
// 跳过去继续找别的（注意：HandleUpstreamError 不会被调用，所以账号零副作用）。
// 如果整组都不支持，failover 耗尽后客户端拿到 400，诊断信息在 ResponseBody 里。
func resolveOpenAIUpstreamModelOrFailover(account *Account, model string) (string, error) {
	upstream, err := normalizeOpenAIModelForUpstream(account, model)
	if err == nil {
		return upstream, nil
	}
	var unsupported *UnsupportedUpstreamModelError
	if errors.As(err, &unsupported) {
		body := fmt.Sprintf(
			`{"error":{"type":"invalid_request_error","code":"model_not_supported","message":%q}}`,
			unsupported.Error(),
		)
		return "", &UpstreamFailoverError{
			StatusCode:   http.StatusBadRequest,
			ResponseBody: []byte(body),
		}
	}
	return "", err
}

var codexModelMap = map[string]string{
	"gpt-5.5":                    "gpt-5.5",
	"gpt-5.5-none":               "gpt-5.5",
	"gpt-5.5-low":                "gpt-5.5",
	"gpt-5.5-medium":             "gpt-5.5",
	"gpt-5.5-high":               "gpt-5.5",
	"gpt-5.5-xhigh":              "gpt-5.5",
	"gpt-5.4":                    "gpt-5.4",
	"gpt-5.4-mini":               "gpt-5.4-mini",
	"gpt-5.4-none":               "gpt-5.4",
	"gpt-5.4-low":                "gpt-5.4",
	"gpt-5.4-medium":             "gpt-5.4",
	"gpt-5.4-high":               "gpt-5.4",
	"gpt-5.4-xhigh":              "gpt-5.4",
	"gpt-5.4-chat-latest":        "gpt-5.4",
	"gpt-5.3":                    "gpt-5.3-codex",
	"gpt-5.3-none":               "gpt-5.3-codex",
	"gpt-5.3-low":                "gpt-5.3-codex",
	"gpt-5.3-medium":             "gpt-5.3-codex",
	"gpt-5.3-high":               "gpt-5.3-codex",
	"gpt-5.3-xhigh":              "gpt-5.3-codex",
	"gpt-5.3-codex":              "gpt-5.3-codex",
	"gpt-5.3-codex-spark":        "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-low":    "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-medium": "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-high":   "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-xhigh":  "gpt-5.3-codex-spark",
	"gpt-5.3-codex-low":          "gpt-5.3-codex",
	"gpt-5.3-codex-medium":       "gpt-5.3-codex",
	"gpt-5.3-codex-high":         "gpt-5.3-codex",
	"gpt-5.3-codex-xhigh":        "gpt-5.3-codex",
	"gpt-5.2":                    "gpt-5.2",
	"gpt-5.2-none":               "gpt-5.2",
	"gpt-5.2-low":                "gpt-5.2",
	"gpt-5.2-medium":             "gpt-5.2",
	"gpt-5.2-high":               "gpt-5.2",
	"gpt-5.2-xhigh":              "gpt-5.2",
}

type codexTransformResult struct {
	Modified        bool
	NormalizedModel string
	PromptCacheKey  string
}

func applyCodexOAuthTransform(reqBody map[string]any, isCodexCLI bool, isCompact bool) codexTransformResult {
	result := codexTransformResult{}
	// 工具续链需求会影响存储策略与 input 过滤逻辑。
	needsToolContinuation := NeedsToolContinuation(reqBody)

	model := ""
	if v, ok := reqBody["model"].(string); ok {
		model = v
	}
	normalizedModel := strings.TrimSpace(model)
	if normalizedModel != "" {
		if model != normalizedModel {
			reqBody["model"] = normalizedModel
			result.Modified = true
		}
		result.NormalizedModel = normalizedModel
	}

	if isCompact {
		if _, ok := reqBody["store"]; ok {
			delete(reqBody, "store")
			result.Modified = true
		}
		if _, ok := reqBody["stream"]; ok {
			delete(reqBody, "stream")
			result.Modified = true
		}
	} else {
		// OAuth 走 ChatGPT internal API 时，store 必须为 false；显式 true 也会强制覆盖。
		// 避免上游返回 "Store must be set to false"。
		if v, ok := reqBody["store"].(bool); !ok || v {
			reqBody["store"] = false
			result.Modified = true
		}
		if v, ok := reqBody["stream"].(bool); !ok || !v {
			reqBody["stream"] = true
			result.Modified = true
		}
	}

	// Strip parameters unsupported by codex models via the Responses API.
	for _, key := range []string{
		"max_output_tokens",
		"max_completion_tokens",
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
		// prompt_cache_retention is a newer Responses API parameter (cache TTL).
		// The ChatGPT internal Codex endpoint rejects it with
		// "Unsupported parameter: prompt_cache_retention". Defense-in-depth
		// for any OAuth path that reaches this transform — the Cursor
		// Responses-shape short-circuit in ForwardAsChatCompletions strips
		// it earlier too, but we keep this line so other OAuth callers are
		// equally protected.
		"prompt_cache_retention",
	} {
		if _, ok := reqBody[key]; ok {
			delete(reqBody, key)
			result.Modified = true
		}
	}

	// 兼容遗留的 functions 和 function_call，转换为 tools 和 tool_choice
	if functionsRaw, ok := reqBody["functions"]; ok {
		if functions, k := functionsRaw.([]any); k {
			tools := make([]any, 0, len(functions))
			for _, f := range functions {
				tools = append(tools, map[string]any{
					"type":     "function",
					"function": f,
				})
			}
			reqBody["tools"] = tools
		}
		delete(reqBody, "functions")
		result.Modified = true
	}

	if fcRaw, ok := reqBody["function_call"]; ok {
		if fcStr, ok := fcRaw.(string); ok {
			// e.g. "auto", "none"
			reqBody["tool_choice"] = fcStr
		} else if fcObj, ok := fcRaw.(map[string]any); ok {
			// e.g. {"name": "my_func"}
			if name, ok := fcObj["name"].(string); ok && strings.TrimSpace(name) != "" {
				reqBody["tool_choice"] = map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": name,
					},
				}
			}
		}
		delete(reqBody, "function_call")
		result.Modified = true
	}

	if normalizeCodexTools(reqBody) {
		result.Modified = true
	}

	if v, ok := reqBody["prompt_cache_key"].(string); ok {
		result.PromptCacheKey = strings.TrimSpace(v)
	}

	// 提取 input 中 role:"system" 消息至 instructions（OAuth 上游不支持 system role）。
	if extractSystemMessagesFromInput(reqBody) {
		result.Modified = true
	}

	// instructions 处理逻辑：根据是否是 Codex CLI 分别调用不同方法
	if applyInstructions(reqBody, isCodexCLI) {
		result.Modified = true
	}

	// 续链场景保留 item_reference 与 id，避免 call_id 上下文丢失。
	if input, ok := reqBody["input"].([]any); ok {
		input = filterCodexInput(input, needsToolContinuation)
		reqBody["input"] = input
		result.Modified = true
	} else if inputStr, ok := reqBody["input"].(string); ok {
		// ChatGPT codex endpoint requires input to be a list, not a string.
		// Convert string input to the expected message array format.
		trimmed := strings.TrimSpace(inputStr)
		if trimmed != "" {
			reqBody["input"] = []any{
				map[string]any{
					"type":    "message",
					"role":    "user",
					"content": inputStr,
				},
			}
		} else {
			reqBody["input"] = []any{}
		}
		result.Modified = true
	}

	return result
}

func normalizeCodexModel(model string) string {
	normalized, _ := normalizeCodexModelStrict(model)
	return normalized
}

// normalizeCodexModelStrict 作者: mkx  变更: 2026/04/24
// 返回归一后的 Codex 模型名与是否"显式命中"标志：
//   - explicit=true  表示输入在 codexModelMap 或已知子串（gpt-5.3/5.4/codex 等）里匹配到；
//   - explicit=false 表示兜底到 gpt-5.4 的默认值，调用方应据此决定是否拒绝。
//
// 将此信息回传是为了解决 image-only free 账号被 /v1/chat/completions 链路
// 打穿的问题——旧的 normalizeCodexModel 在 explicit=false 时静默返回 "gpt-5.4"，
// 导致上游把 chat 请求发给无 chat 权限的账号而失败。
func normalizeCodexModelStrict(model string) (normalized string, explicit bool) {
	if model == "" {
		return "gpt-5.4", false
	}

	modelID := model
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}

	if mapped := getNormalizedCodexModel(modelID); mapped != "" {
		return mapped, true
	}

	lower := strings.ToLower(modelID)

	if strings.Contains(lower, "gpt-5.5") || strings.Contains(lower, "gpt 5.5") {
		return "gpt-5.5", true
	}
	if strings.Contains(lower, "gpt-5.4-mini") || strings.Contains(lower, "gpt 5.4 mini") {
		return "gpt-5.4-mini", true
	}
	if strings.Contains(lower, "gpt-5.4") || strings.Contains(lower, "gpt 5.4") {
		return "gpt-5.4", true
	}
	if strings.Contains(lower, "gpt-5.2") || strings.Contains(lower, "gpt 5.2") {
		return "gpt-5.2", true
	}
	if strings.Contains(lower, "gpt-5.3-codex-spark") || strings.Contains(lower, "gpt 5.3 codex spark") {
		return "gpt-5.3-codex-spark", true
	}
	if strings.Contains(lower, "gpt-5.3-codex") || strings.Contains(lower, "gpt 5.3 codex") {
		return "gpt-5.3-codex", true
	}
	if strings.Contains(lower, "gpt-5.3") || strings.Contains(lower, "gpt 5.3") {
		return "gpt-5.3-codex", true
	}
	if strings.Contains(lower, "codex") {
		return "gpt-5.3-codex", true
	}
	if strings.Contains(lower, "gpt-5") || strings.Contains(lower, "gpt 5") {
		return "gpt-5.4", true
	}

	return "gpt-5.4", false
}

// normalizeOpenAIModelForUpstream 作者: mkx  变更: 2026/04/24
// OAuth 账号在"未显式命中 + 账号 model_mapping 不包含 gpt-5.4"时返回
// *UnsupportedUpstreamModelError，调用方应将其视为不可重试的客户端错误（400），
// 不要把请求打到上游，也不要让账号承担任何禁用/限流副作用。
//
// 其余情况（API Key 账号、账号无 mapping 即允许全部、显式命中的模型）
// 保持与旧版相同的行为：返回归一后的上游模型名，err=nil。
func normalizeOpenAIModelForUpstream(account *Account, model string) (string, error) {
	if account == nil || account.Type != AccountTypeOAuth {
		return strings.TrimSpace(model), nil
	}

	normalized, explicit := normalizeCodexModelStrict(model)
	if explicit {
		return normalized, nil
	}
	// 账号未配置 model_mapping 等同于"允许全部"，保持旧兜底行为。
	if account.IsModelSupported("gpt-5.4") {
		return normalized, nil
	}
	return "", &UnsupportedUpstreamModelError{
		RequestedModel: model,
		FallbackModel:  normalized,
		AccountID:      account.ID,
		AccountName:    account.Name,
	}
}

func SupportsVerbosity(model string) bool {
	if !strings.HasPrefix(model, "gpt-") {
		return true
	}

	var major, minor int
	n, _ := fmt.Sscanf(model, "gpt-%d.%d", &major, &minor)

	if major > 5 {
		return true
	}
	if major < 5 {
		return false
	}

	// gpt-5
	if n == 1 {
		return true
	}

	return minor >= 3
}

func getNormalizedCodexModel(modelID string) string {
	if modelID == "" {
		return ""
	}
	if mapped, ok := codexModelMap[modelID]; ok {
		return mapped
	}
	lower := strings.ToLower(modelID)
	for key, value := range codexModelMap {
		if strings.ToLower(key) == lower {
			return value
		}
	}
	return ""
}

// extractTextFromContent extracts plain text from a content value that is either
// a Go string or a []any of content-part maps with type:"text".
func extractTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, part := range v {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t == "text" {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// extractSystemMessagesFromInput scans the input array for items with role=="system",
// removes them, and merges their content into reqBody["instructions"].
// If instructions is already non-empty, extracted content is prepended with "\n\n".
// Returns true if any system messages were extracted.
func extractSystemMessagesFromInput(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 {
		return false
	}

	var systemTexts []string
	remaining := make([]any, 0, len(input))

	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			remaining = append(remaining, item)
			continue
		}
		if role, _ := m["role"].(string); role != "system" {
			remaining = append(remaining, item)
			continue
		}
		if text := extractTextFromContent(m["content"]); text != "" {
			systemTexts = append(systemTexts, text)
		}
	}

	if len(systemTexts) == 0 {
		return false
	}

	extracted := strings.Join(systemTexts, "\n\n")
	if existing, ok := reqBody["instructions"].(string); ok && strings.TrimSpace(existing) != "" {
		reqBody["instructions"] = extracted + "\n\n" + existing
	} else {
		reqBody["instructions"] = extracted
	}
	reqBody["input"] = remaining
	return true
}

// applyInstructions 处理 instructions 字段：仅在 instructions 为空时填充默认值。
func applyInstructions(reqBody map[string]any, isCodexCLI bool) bool {
	if !isInstructionsEmpty(reqBody) {
		return false
	}
	reqBody["instructions"] = "You are a helpful coding assistant."
	return true
}

// isInstructionsEmpty 检查 instructions 字段是否为空
// 处理以下情况：字段不存在、nil、空字符串、纯空白字符串
func isInstructionsEmpty(reqBody map[string]any) bool {
	val, exists := reqBody["instructions"]
	if !exists {
		return true
	}
	if val == nil {
		return true
	}
	str, ok := val.(string)
	if !ok {
		return true
	}
	return strings.TrimSpace(str) == ""
}

// filterCodexInput 按需过滤 item_reference 与 id。
// preserveReferences 为 true 时保持引用与 id，以满足续链请求对上下文的依赖。
func filterCodexInput(input []any, preserveReferences bool) []any {
	filtered := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		typ, _ := m["type"].(string)

		// 仅修正真正的 tool/function call 标识，避免误改普通 message/reasoning id；
		// 若 item_reference 指向 legacy call_* 标识，则仅修正该引用本身。
		fixCallIDPrefix := func(id string) string {
			if id == "" || strings.HasPrefix(id, "fc") {
				return id
			}
			if strings.HasPrefix(id, "call_") {
				return "fc" + strings.TrimPrefix(id, "call_")
			}
			return "fc_" + id
		}

		if typ == "item_reference" {
			if !preserveReferences {
				continue
			}
			newItem := make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
			}
			if id, ok := newItem["id"].(string); ok && strings.HasPrefix(id, "call_") {
				newItem["id"] = fixCallIDPrefix(id)
			}
			filtered = append(filtered, newItem)
			continue
		}

		newItem := m
		copied := false
		// 仅在需要修改字段时创建副本，避免直接改写原始输入。
		ensureCopy := func() {
			if copied {
				return
			}
			newItem = make(map[string]any, len(m))
			for key, value := range m {
				newItem[key] = value
			}
			copied = true
		}

		if isCodexToolCallItemType(typ) {
			callID, ok := m["call_id"].(string)
			if !ok || strings.TrimSpace(callID) == "" {
				if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
					callID = id
					ensureCopy()
					newItem["call_id"] = callID
				}
			}

			if callID != "" {
				fixedCallID := fixCallIDPrefix(callID)
				if fixedCallID != callID {
					ensureCopy()
					newItem["call_id"] = fixedCallID
				}
			}
		}

		if !preserveReferences {
			ensureCopy()
			delete(newItem, "id")
			if !isCodexToolCallItemType(typ) {
				delete(newItem, "call_id")
			}
		}

		filtered = append(filtered, newItem)
	}
	return filtered
}

func isCodexToolCallItemType(typ string) bool {
	if typ == "" {
		return false
	}
	return strings.HasSuffix(typ, "_call") || strings.HasSuffix(typ, "_call_output")
}

func normalizeCodexTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}

	modified := false
	validTools := make([]any, 0, len(tools))

	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			// Keep unknown structure as-is to avoid breaking upstream behavior.
			validTools = append(validTools, tool)
			continue
		}

		toolType, _ := toolMap["type"].(string)
		toolType = strings.TrimSpace(toolType)
		if toolType != "function" {
			validTools = append(validTools, toolMap)
			continue
		}

		// OpenAI Responses-style tools use top-level name/parameters.
		if name, ok := toolMap["name"].(string); ok && strings.TrimSpace(name) != "" {
			validTools = append(validTools, toolMap)
			continue
		}

		// ChatCompletions-style tools use {type:"function", function:{...}}.
		functionValue, hasFunction := toolMap["function"]
		function, ok := functionValue.(map[string]any)
		if !hasFunction || functionValue == nil || !ok || function == nil {
			// Drop invalid function tools.
			modified = true
			continue
		}

		if _, ok := toolMap["name"]; !ok {
			if name, ok := function["name"].(string); ok && strings.TrimSpace(name) != "" {
				toolMap["name"] = name
				modified = true
			}
		}
		if _, ok := toolMap["description"]; !ok {
			if desc, ok := function["description"].(string); ok && strings.TrimSpace(desc) != "" {
				toolMap["description"] = desc
				modified = true
			}
		}
		if _, ok := toolMap["parameters"]; !ok {
			if params, ok := function["parameters"]; ok {
				toolMap["parameters"] = params
				modified = true
			}
		}
		if _, ok := toolMap["strict"]; !ok {
			if strict, ok := function["strict"]; ok {
				toolMap["strict"] = strict
				modified = true
			}
		}

		validTools = append(validTools, toolMap)
	}

	if modified {
		reqBody["tools"] = validTools
	}

	return modified
}
