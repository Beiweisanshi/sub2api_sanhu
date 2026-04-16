package antigravity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"strings"
)

// BlockType 内容块类型
type BlockType int

const (
	BlockTypeNone BlockType = iota
	BlockTypeText
	BlockTypeThinking
	BlockTypeFunction
)

// UsageMapHook is a callback that can modify usage data before it's emitted in SSE events.
type UsageMapHook func(usageMap map[string]any)

// StreamingProcessor 流式响应处理器
type StreamingProcessor struct {
	blockType         BlockType
	blockIndex        int
	messageStartSent  bool
	messageStopSent   bool
	usedTool          bool
	pendingSignature  string
	trailingSignature string
	originalModel     string
	webSearchQueries  []string
	groundingChunks   []GeminiGroundingChunk
	usageMapHook      UsageMapHook

	// 累计 usage
	inputTokens         int
	outputTokens        int
	cacheReadTokens     int
	cacheCreationTokens int
	imageOutputTokens   int

	// 模拟缓存比例
	simulateCacheRatio float64

	// 模拟缓存随机决策（每个 streaming 会话只做一次）
	cacheDecisionMade     bool
	isCacheCreationEvent  bool
	cacheCreationSubRatio float64
	cacheRatioJitter      float64 // [-0.02, +0.02] 抖动，使比例更自然
}

// NewStreamingProcessor 创建流式响应处理器
func NewStreamingProcessor(originalModel string, simulateCacheRatio float64) *StreamingProcessor {
	return &StreamingProcessor{
		blockType:          BlockTypeNone,
		originalModel:      originalModel,
		simulateCacheRatio: simulateCacheRatio,
	}
}

// SetUsageMapHook sets an optional hook that modifies usage maps before they are emitted.
func (p *StreamingProcessor) SetUsageMapHook(fn UsageMapHook) {
	p.usageMapHook = fn
}

func usageToMap(u ClaudeUsage) map[string]any {
	m := map[string]any{
		"input_tokens":  u.InputTokens,
		"output_tokens": u.OutputTokens,
	}
	if u.CacheCreationInputTokens > 0 {
		m["cache_creation_input_tokens"] = u.CacheCreationInputTokens
	}
	if u.CacheReadInputTokens > 0 {
		m["cache_read_input_tokens"] = u.CacheReadInputTokens
	}
	if u.ImageOutputTokens > 0 {
		m["image_output_tokens"] = u.ImageOutputTokens
	}
	return m
}

// simulateCacheCreationRate 模拟缓存创建事件的概率（2%）
const simulateCacheCreationRate = 0.02

// applySimulateCache 基于客户端可见的 prompt token 总量按比例模拟缓存，适用于非流式场景（每个请求调用一次）。
// ratio 表示目标可见缓存占比：(cache_read + cache_creation) / (input + cache_read + cache_creation)。
// 实际比例仅允许在 ratio ~ ratio+2% 范围内上浮，不会低于配置值。
func applySimulateCache(inputTokens, cacheReadTokens int, ratio float64) (newInput, newCacheRead, newCacheCreation int) {
	isCacheCreation := rand.Float64() < simulateCacheCreationRate
	var creationSubRatio float64
	if isCacheCreation {
		creationSubRatio = 0.20 + rand.Float64()*0.30
	}
	jitter := rand.Float64() * 0.02 // [0, +0.02]
	return applySimulateCacheDeterministic(inputTokens, cacheReadTokens, ratio, isCacheCreation, creationSubRatio, jitter)
}

// applySimulateCacheDeterministic 确定性版本的模拟缓存转换，用于流式场景
// 随机决策由调用方提前做出并缓存，确保同一 streaming 会话内的一致性
//
// 目标可见缓存量（cache_read + cache_creation）精确等于 round((ratio + jitter) × totalPrompt)，
// 其中 totalPrompt 是客户端可见的 prompt 总量。
// cache_creation 仅从本次新增的模拟缓存中切分，不能重分类上游真实的 cache_read。
func applySimulateCacheDeterministic(inputTokens, cacheReadTokens int, ratio float64, isCacheCreation bool, creationSubRatio float64, jitter float64) (newInput, newCacheRead, newCacheCreation int) {
	totalPrompt := inputTokens + cacheReadTokens
	if ratio <= 0 || totalPrompt <= 0 {
		return inputTokens, cacheReadTokens, 0
	}

	// 应用上浮抖动并钳位到 [0, 1]
	effectiveRatio := ratio + jitter
	if effectiveRatio < 0 {
		effectiveRatio = 0
	}
	if effectiveRatio > 1 {
		effectiveRatio = 1
	}

	// 基于客户端可见的 prompt 总量计算目标缓存量（cache_read + cache_creation）
	targetCacheTotal := int(math.Round(float64(totalPrompt) * effectiveRatio))

	// Gemini 真实缓存已达到或超过目标，无需模拟
	if cacheReadTokens >= targetCacheTotal {
		return inputTokens, cacheReadTokens, 0
	}

	// 从 inputTokens 中转换到缓存
	additionalCache := targetCacheTotal - cacheReadTokens
	remaining := inputTokens - additionalCache
	// 始终保留至少 1 个 input token
	if remaining < 1 && inputTokens >= 1 {
		remaining = 1
		additionalCache = inputTokens - 1
	}

	if additionalCache <= 0 {
		return remaining, cacheReadTokens, 0
	}

	newCacheRead = cacheReadTokens + additionalCache

	// cache creation 事件：仅从新增的模拟缓存里切分，避免改写上游真实的 cache_read。
	if isCacheCreation && additionalCache > 1 {
		creationTokens := int(math.Round(float64(additionalCache) * creationSubRatio))
		if creationTokens < 1 {
			creationTokens = 1
		}
		// 确保新增的模拟 cache_read 至少保留 1
		if creationTokens >= additionalCache {
			creationTokens = additionalCache - 1
		}
		return remaining, newCacheRead - creationTokens, creationTokens
	}

	return remaining, newCacheRead, 0
}

// ProcessLine 处理 SSE 行，返回 Claude SSE 事件
func (p *StreamingProcessor) ProcessLine(line string) []byte {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "data:") {
		return nil
	}

	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" || data == "[DONE]" {
		return nil
	}

	// 解包 v1internal 响应
	var v1Resp V1InternalResponse
	if err := json.Unmarshal([]byte(data), &v1Resp); err != nil {
		// 尝试直接解析为 GeminiResponse
		var directResp GeminiResponse
		if err2 := json.Unmarshal([]byte(data), &directResp); err2 != nil {
			return nil
		}
		v1Resp.Response = directResp
		v1Resp.ResponseID = directResp.ResponseID
		v1Resp.ModelVersion = directResp.ModelVersion
	}

	geminiResp := &v1Resp.Response

	var result bytes.Buffer

	// 更新 usage（在 emitMessageStart 之前，确保 message_start 和后续事件使用一致的模拟缓存决策）
	// 注意：Gemini 的 promptTokenCount 包含 cachedContentTokenCount，
	// 但 Claude 的 input_tokens 不包含 cache_read_input_tokens，需要减去
	if geminiResp.UsageMetadata != nil {
		cached := geminiResp.UsageMetadata.CachedContentTokenCount
		p.inputTokens = geminiResp.UsageMetadata.PromptTokenCount - cached
		p.outputTokens = geminiResp.UsageMetadata.CandidatesTokenCount + geminiResp.UsageMetadata.ThoughtsTokenCount
		p.cacheReadTokens = cached
		p.cacheCreationTokens = 0
		p.imageOutputTokens = geminiResp.UsageMetadata.ImageOutputTokens()

		// 应用模拟缓存（仅在收到新的 UsageMetadata 时，避免对已模拟值重复应用）
		if p.simulateCacheRatio > 0 {
			// 随机决策只做一次，后续 chunk 复用，确保 message_start 和 message_delta 一致
			if !p.cacheDecisionMade {
				p.isCacheCreationEvent = rand.Float64() < simulateCacheCreationRate
				if p.isCacheCreationEvent {
					p.cacheCreationSubRatio = 0.20 + rand.Float64()*0.30
				}
				p.cacheRatioJitter = rand.Float64() * 0.02 // [0, +0.02]
				p.cacheDecisionMade = true
			}
			p.inputTokens, p.cacheReadTokens, p.cacheCreationTokens = applySimulateCacheDeterministic(
				p.inputTokens, p.cacheReadTokens, p.simulateCacheRatio, p.isCacheCreationEvent, p.cacheCreationSubRatio, p.cacheRatioJitter)
		}
	}

	// 发送 message_start（在 usage 更新之后，确保 message_start 包含模拟缓存后的值）
	if !p.messageStartSent {
		_, _ = result.Write(p.emitMessageStart(&v1Resp))
	}

	// 处理 parts
	if len(geminiResp.Candidates) > 0 && geminiResp.Candidates[0].Content != nil {
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			_, _ = result.Write(p.processPart(&part))
		}
	}

	if len(geminiResp.Candidates) > 0 {
		p.captureGrounding(geminiResp.Candidates[0].GroundingMetadata)
	}

	// 检查是否结束
	if len(geminiResp.Candidates) > 0 {
		finishReason := geminiResp.Candidates[0].FinishReason
		if finishReason == "MALFORMED_FUNCTION_CALL" {
			log.Printf("[Antigravity] MALFORMED_FUNCTION_CALL detected in stream for model %s", p.originalModel)
			if geminiResp.Candidates[0].Content != nil {
				if b, err := json.Marshal(geminiResp.Candidates[0].Content); err == nil {
					log.Printf("[Antigravity] Malformed content: %s", string(b))
				}
			}
		}
		if finishReason != "" {
			_, _ = result.Write(p.emitFinish(finishReason))
		}
	}

	return result.Bytes()
}

// Finish 结束处理，返回最终事件和用量。
// 若整个流未收到任何可解析的上游数据（messageStartSent == false），
// 则不补发任何结束事件，防止客户端收到没有 message_start 的残缺流。
func (p *StreamingProcessor) Finish() ([]byte, *ClaudeUsage) {
	usage := &ClaudeUsage{
		InputTokens:              p.inputTokens,
		OutputTokens:             p.outputTokens,
		CacheReadInputTokens:     p.cacheReadTokens,
		CacheCreationInputTokens: p.cacheCreationTokens,
		ImageOutputTokens:        p.imageOutputTokens,
	}

	if !p.messageStartSent {
		return nil, usage
	}

	var result bytes.Buffer
	if !p.messageStopSent {
		_, _ = result.Write(p.emitFinish(""))
	}

	return result.Bytes(), usage
}

// MessageStartSent 报告流中是否已发出过 message_start 事件（即是否收到过有效的上游数据）
func (p *StreamingProcessor) MessageStartSent() bool {
	return p.messageStartSent
}

// emitMessageStart 发送 message_start 事件
// 注意：调用前 ProcessLine 已完成 UsageMetadata 处理和模拟缓存，
// 直接读取 processor 字段以确保与后续事件的一致性
func (p *StreamingProcessor) emitMessageStart(v1Resp *V1InternalResponse) []byte {
	if p.messageStartSent {
		return nil
	}

	usage := ClaudeUsage{}
	if v1Resp.Response.UsageMetadata != nil {
		cached := v1Resp.Response.UsageMetadata.CachedContentTokenCount
		usage.InputTokens = v1Resp.Response.UsageMetadata.PromptTokenCount - cached
		usage.OutputTokens = v1Resp.Response.UsageMetadata.CandidatesTokenCount + v1Resp.Response.UsageMetadata.ThoughtsTokenCount
		usage.CacheReadInputTokens = cached
		usage.CacheCreationInputTokens = 0
		usage.ImageOutputTokens = v1Resp.Response.UsageMetadata.ImageOutputTokens()
	}

	responseID := v1Resp.ResponseID
	if responseID == "" {
		responseID = v1Resp.Response.ResponseID
	}
	if responseID == "" {
		responseID = "msg_" + generateRandomID()
	}

	var usageValue any = usage
	if p.usageMapHook != nil {
		usageMap := usageToMap(usage)
		p.usageMapHook(usageMap)
		usageValue = usageMap
	}

	message := map[string]any{
		"id":            responseID,
		"type":          "message",
		"role":          "assistant",
		"content":       []any{},
		"model":         p.originalModel,
		"stop_reason":   nil,
		"stop_sequence": nil,
		"usage":         usageValue,
	}

	event := map[string]any{
		"type":    "message_start",
		"message": message,
	}

	p.messageStartSent = true
	return p.formatSSE("message_start", event)
}

// processPart 处理单个 part
func (p *StreamingProcessor) processPart(part *GeminiPart) []byte {
	var result bytes.Buffer
	signature := part.ThoughtSignature

	// 1. FunctionCall 处理
	if part.FunctionCall != nil {
		// 先处理 trailingSignature
		if p.trailingSignature != "" {
			_, _ = result.Write(p.endBlock())
			_, _ = result.Write(p.emitEmptyThinkingWithSignature(p.trailingSignature))
			p.trailingSignature = ""
		}

		_, _ = result.Write(p.processFunctionCall(part.FunctionCall, signature))
		return result.Bytes()
	}

	// 2. Text 处理
	if part.Text != "" || part.Thought {
		if part.Thought {
			_, _ = result.Write(p.processThinking(part.Text, signature))
		} else {
			_, _ = result.Write(p.processText(part.Text, signature))
		}
	}

	// 3. InlineData (Image) 处理
	if part.InlineData != nil && part.InlineData.Data != "" {
		markdownImg := fmt.Sprintf("![image](data:%s;base64,%s)",
			part.InlineData.MimeType, part.InlineData.Data)
		_, _ = result.Write(p.processText(markdownImg, ""))
	}

	return result.Bytes()
}

func (p *StreamingProcessor) captureGrounding(grounding *GeminiGroundingMetadata) {
	if grounding == nil {
		return
	}

	if len(grounding.WebSearchQueries) > 0 && len(p.webSearchQueries) == 0 {
		p.webSearchQueries = append([]string(nil), grounding.WebSearchQueries...)
	}

	if len(grounding.GroundingChunks) > 0 && len(p.groundingChunks) == 0 {
		p.groundingChunks = append([]GeminiGroundingChunk(nil), grounding.GroundingChunks...)
	}
}

// processThinking 处理 thinking
func (p *StreamingProcessor) processThinking(text, signature string) []byte {
	var result bytes.Buffer

	// 处理之前的 trailingSignature
	if p.trailingSignature != "" {
		_, _ = result.Write(p.endBlock())
		_, _ = result.Write(p.emitEmptyThinkingWithSignature(p.trailingSignature))
		p.trailingSignature = ""
	}

	// 开始或继续 thinking 块
	if p.blockType != BlockTypeThinking {
		_, _ = result.Write(p.startBlock(BlockTypeThinking, map[string]any{
			"type":     "thinking",
			"thinking": "",
		}))
	}

	if text != "" {
		_, _ = result.Write(p.emitDelta("thinking_delta", map[string]any{
			"thinking": text,
		}))
	}

	// 暂存签名
	if signature != "" {
		p.pendingSignature = signature
	}

	return result.Bytes()
}

// processText 处理普通 text
func (p *StreamingProcessor) processText(text, signature string) []byte {
	var result bytes.Buffer

	// 空 text 带签名 - 暂存
	if text == "" {
		if signature != "" {
			p.trailingSignature = signature
		}
		return nil
	}

	// 处理之前的 trailingSignature
	if p.trailingSignature != "" {
		_, _ = result.Write(p.endBlock())
		_, _ = result.Write(p.emitEmptyThinkingWithSignature(p.trailingSignature))
		p.trailingSignature = ""
	}

	// 非空 text 带签名 - 特殊处理
	if signature != "" {
		_, _ = result.Write(p.startBlock(BlockTypeText, map[string]any{
			"type": "text",
			"text": "",
		}))
		_, _ = result.Write(p.emitDelta("text_delta", map[string]any{
			"text": text,
		}))
		_, _ = result.Write(p.endBlock())
		_, _ = result.Write(p.emitEmptyThinkingWithSignature(signature))
		return result.Bytes()
	}

	// 普通 text (无签名)
	if p.blockType != BlockTypeText {
		_, _ = result.Write(p.startBlock(BlockTypeText, map[string]any{
			"type": "text",
			"text": "",
		}))
	}

	_, _ = result.Write(p.emitDelta("text_delta", map[string]any{
		"text": text,
	}))

	return result.Bytes()
}

// processFunctionCall 处理 function call
func (p *StreamingProcessor) processFunctionCall(fc *GeminiFunctionCall, signature string) []byte {
	var result bytes.Buffer

	p.usedTool = true

	toolID := fc.ID
	if toolID == "" {
		toolID = fmt.Sprintf("%s-%s", fc.Name, generateRandomID())
	}

	toolUse := map[string]any{
		"type":  "tool_use",
		"id":    toolID,
		"name":  fc.Name,
		"input": map[string]any{},
	}

	if signature != "" {
		toolUse["signature"] = signature
	}

	_, _ = result.Write(p.startBlock(BlockTypeFunction, toolUse))

	// 发送 input_json_delta
	if fc.Args != nil {
		argsJSON, _ := json.Marshal(fc.Args)
		_, _ = result.Write(p.emitDelta("input_json_delta", map[string]any{
			"partial_json": string(argsJSON),
		}))
	}

	_, _ = result.Write(p.endBlock())

	return result.Bytes()
}

// startBlock 开始新的内容块
func (p *StreamingProcessor) startBlock(blockType BlockType, contentBlock map[string]any) []byte {
	var result bytes.Buffer

	if p.blockType != BlockTypeNone {
		_, _ = result.Write(p.endBlock())
	}

	event := map[string]any{
		"type":          "content_block_start",
		"index":         p.blockIndex,
		"content_block": contentBlock,
	}

	_, _ = result.Write(p.formatSSE("content_block_start", event))
	p.blockType = blockType

	return result.Bytes()
}

// endBlock 结束当前内容块
func (p *StreamingProcessor) endBlock() []byte {
	if p.blockType == BlockTypeNone {
		return nil
	}

	var result bytes.Buffer

	// Thinking 块结束时发送暂存的签名
	if p.blockType == BlockTypeThinking && p.pendingSignature != "" {
		_, _ = result.Write(p.emitDelta("signature_delta", map[string]any{
			"signature": p.pendingSignature,
		}))
		p.pendingSignature = ""
	}

	event := map[string]any{
		"type":  "content_block_stop",
		"index": p.blockIndex,
	}

	_, _ = result.Write(p.formatSSE("content_block_stop", event))

	p.blockIndex++
	p.blockType = BlockTypeNone

	return result.Bytes()
}

// emitDelta 发送 delta 事件
func (p *StreamingProcessor) emitDelta(deltaType string, deltaContent map[string]any) []byte {
	delta := map[string]any{
		"type": deltaType,
	}
	for k, v := range deltaContent {
		delta[k] = v
	}

	event := map[string]any{
		"type":  "content_block_delta",
		"index": p.blockIndex,
		"delta": delta,
	}

	return p.formatSSE("content_block_delta", event)
}

// emitEmptyThinkingWithSignature 发送空 thinking 块承载签名
func (p *StreamingProcessor) emitEmptyThinkingWithSignature(signature string) []byte {
	var result bytes.Buffer

	_, _ = result.Write(p.startBlock(BlockTypeThinking, map[string]any{
		"type":     "thinking",
		"thinking": "",
	}))
	_, _ = result.Write(p.emitDelta("thinking_delta", map[string]any{
		"thinking": "",
	}))
	_, _ = result.Write(p.emitDelta("signature_delta", map[string]any{
		"signature": signature,
	}))
	_, _ = result.Write(p.endBlock())

	return result.Bytes()
}

// emitFinish 发送结束事件
func (p *StreamingProcessor) emitFinish(finishReason string) []byte {
	var result bytes.Buffer

	// 关闭最后一个块
	_, _ = result.Write(p.endBlock())

	// 处理 trailingSignature
	if p.trailingSignature != "" {
		_, _ = result.Write(p.emitEmptyThinkingWithSignature(p.trailingSignature))
		p.trailingSignature = ""
	}

	if len(p.webSearchQueries) > 0 || len(p.groundingChunks) > 0 {
		groundingText := buildGroundingText(&GeminiGroundingMetadata{
			WebSearchQueries: p.webSearchQueries,
			GroundingChunks:  p.groundingChunks,
		})
		if groundingText != "" {
			_, _ = result.Write(p.startBlock(BlockTypeText, map[string]any{
				"type": "text",
				"text": "",
			}))
			_, _ = result.Write(p.emitDelta("text_delta", map[string]any{
				"text": groundingText,
			}))
			_, _ = result.Write(p.endBlock())
		}
	}

	// 确定 stop_reason
	stopReason := "end_turn"
	if p.usedTool {
		stopReason = "tool_use"
	} else if finishReason == "MAX_TOKENS" {
		stopReason = "max_tokens"
	}

	usage := ClaudeUsage{
		InputTokens:              p.inputTokens,
		OutputTokens:             p.outputTokens,
		CacheReadInputTokens:     p.cacheReadTokens,
		CacheCreationInputTokens: p.cacheCreationTokens,
		ImageOutputTokens:        p.imageOutputTokens,
	}

	var usageValue any = usage
	if p.usageMapHook != nil {
		usageMap := usageToMap(usage)
		p.usageMapHook(usageMap)
		usageValue = usageMap
	}

	deltaEvent := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": usageValue,
	}

	_, _ = result.Write(p.formatSSE("message_delta", deltaEvent))

	if !p.messageStopSent {
		stopEvent := map[string]any{
			"type": "message_stop",
		}
		_, _ = result.Write(p.formatSSE("message_stop", stopEvent))
		p.messageStopSent = true
	}

	return result.Bytes()
}

// formatSSE 格式化 SSE 事件
func (p *StreamingProcessor) formatSSE(eventType string, data any) []byte {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(jsonData)))
}
