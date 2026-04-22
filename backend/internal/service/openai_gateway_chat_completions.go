package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// cursorResponsesUnsupportedFields are top-level Responses API parameters that
// Codex upstreams reject with "Unsupported parameter: ...". They must be
// stripped when forwarding a raw client body through the Responses-shape
// short-circuit in ForwardAsChatCompletions (see isResponsesShape branch).
// The normal Chat Completions → Responses conversion path is unaffected
// because ChatCompletionsRequest has no fields for these parameters — unknown
// fields are dropped naturally by json.Unmarshal. Kept semantically in sync
// with the list in openai_gateway_service.go:2034 used by the /v1/responses
// passthrough path.
var cursorResponsesUnsupportedFields = []string{
	"prompt_cache_retention",
	"safety_identifier",
	"metadata",
	"stream_options",
}

// ForwardAsChatCompletions accepts a Chat Completions request body, converts it
// to OpenAI Responses API format, forwards to the OpenAI upstream, and converts
// the response back to Chat Completions format. All account types (OAuth and API
// Key) go through the Responses API conversion path since the upstream only
// exposes the /v1/responses endpoint.
func (s *OpenAIGatewayService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// 作者：mkx  变更：2026-04-22
	// 账号开启 Chat Completions 原生直通时，绕过 Responses API 转换，
	// 直接将请求体转发到 <base>/v1/chat/completions，用于 GLM / DeepSeek /
	// Kimi / new-api 等仅实现 /v1/chat/completions 的 OpenAI 兼容上游。
	if account.IsOpenAIChatCompletionsNativeEnabled() {
		return s.forwardChatCompletionsNative(ctx, c, account, body, defaultMappedModel, startTime)
	}

	// 1. Parse Chat Completions request
	var chatReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		return nil, fmt.Errorf("parse chat completions request: %w", err)
	}
	originalModel := chatReq.Model
	clientStream := chatReq.Stream
	includeUsage := chatReq.StreamOptions != nil && chatReq.StreamOptions.IncludeUsage

	// 2. Resolve model mapping early so compat prompt_cache_key injection can
	// derive a stable seed from the final upstream model family.
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)

	promptCacheKey = strings.TrimSpace(promptCacheKey)
	compatPromptCacheInjected := false
	if promptCacheKey == "" && account.Type == AccountTypeOAuth && shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
		promptCacheKey = deriveCompatPromptCacheKey(&chatReq, upstreamModel)
		compatPromptCacheInjected = promptCacheKey != ""
	}

	// 3. Build the upstream (Responses API) body.
	//
	// Cursor compatibility: some clients (notably Cursor cloud) send Responses
	// API shaped bodies — `input: [...]` with no `messages` field — to the
	// /v1/chat/completions URL. Running those through ChatCompletionsToResponses
	// would silently drop Cursor's `input` array (the struct has no Input field)
	// and produce `input: null`, which Codex upstreams reject with
	// "Invalid type for 'input': expected a string, but got an object".
	//
	// Detect that shape and forward the raw body as-is, only rewriting `model`
	// to the resolved upstream model. The downstream codex OAuth transform will
	// still normalize store/stream/instructions/etc.
	isResponsesShape := !gjson.GetBytes(body, "messages").Exists() && gjson.GetBytes(body, "input").Exists()

	var (
		responsesReq  *apicompat.ResponsesRequest
		responsesBody []byte
		err           error
	)
	if isResponsesShape {
		responsesBody, err = sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite model in responses-shape body: %w", err)
		}
		// Strip Responses API parameters that no Codex upstream accepts.
		// Because this branch forwards the raw body (the normal path rebuilds
		// it from ChatCompletionsRequest and drops unknown fields naturally),
		// we must filter these fields explicitly here — otherwise the upstream
		// rejects the request with "Unsupported parameter: ...".
		for _, field := range cursorResponsesUnsupportedFields {
			if stripped, derr := sjson.DeleteBytes(responsesBody, field); derr == nil {
				responsesBody = stripped
			}
		}
		// Minimal stub populated from the raw body so downstream billing
		// propagation (ServiceTier, ReasoningEffort) keeps working.
		responsesReq = &apicompat.ResponsesRequest{
			Model:       upstreamModel,
			ServiceTier: gjson.GetBytes(responsesBody, "service_tier").String(),
		}
		if effort := gjson.GetBytes(responsesBody, "reasoning.effort").String(); effort != "" {
			responsesReq.Reasoning = &apicompat.ResponsesReasoning{Effort: effort}
		}
	} else {
		// Normal path: convert Chat Completions → Responses.
		// ChatCompletionsToResponses always sets Stream=true (upstream always streams).
		responsesReq, err = apicompat.ChatCompletionsToResponses(&chatReq)
		if err != nil {
			return nil, fmt.Errorf("convert chat completions to responses: %w", err)
		}
		responsesReq.Model = upstreamModel
		responsesBody, err = json.Marshal(responsesReq)
		if err != nil {
			return nil, fmt.Errorf("marshal responses request: %w", err)
		}
	}

	logFields := []zap.Field{
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
		zap.Bool("responses_shape", isResponsesShape),
	}
	if compatPromptCacheInjected {
		logFields = append(logFields,
			zap.Bool("compat_prompt_cache_key_injected", true),
			zap.String("compat_prompt_cache_key_sha256", hashSensitiveValueForLog(promptCacheKey)),
		)
	}
	logger.L().Debug("openai chat_completions: model mapping applied", logFields...)

	if account.Type == AccountTypeOAuth {
		var reqBody map[string]any
		if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
			return nil, fmt.Errorf("unmarshal for codex transform: %w", err)
		}
		codexResult := applyCodexOAuthTransform(reqBody, false, false)
		if codexResult.NormalizedModel != "" {
			upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			promptCacheKey = codexResult.PromptCacheKey
		} else if promptCacheKey != "" {
			reqBody["prompt_cache_key"] = promptCacheKey
		}
		responsesBody, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("remarshal after codex transform: %w", err)
		}
	}

	// 5. Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	// 6. Build upstream request
	upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, responsesBody, token, true, promptCacheKey, false)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	if promptCacheKey != "" {
		upstreamReq.Header.Set("session_id", generateSessionUUID(promptCacheKey))
	}

	// 7. Send request
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// 8. Handle error response with failover
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleChatCompletionsErrorResponse(resp, c, account)
	}

	// 9. Handle normal response
	var result *OpenAIForwardResult
	var handleErr error
	if clientStream {
		result, handleErr = s.handleChatStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, includeUsage, startTime)
	} else {
		result, handleErr = s.handleChatBufferedStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
	}

	// Propagate ServiceTier and ReasoningEffort to result for billing
	if handleErr == nil && result != nil {
		if responsesReq.ServiceTier != "" {
			st := responsesReq.ServiceTier
			result.ServiceTier = &st
		}
		if responsesReq.Reasoning != nil && responsesReq.Reasoning.Effort != "" {
			re := responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}

	// Extract and save Codex usage snapshot from response headers (for OAuth accounts)
	if handleErr == nil && account.Type == AccountTypeOAuth {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}

	return result, handleErr
}

// handleChatCompletionsErrorResponse reads an upstream error and returns it in
// OpenAI Chat Completions error format.
func (s *OpenAIGatewayService) handleChatCompletionsErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*OpenAIForwardResult, error) {
	return s.handleCompatErrorResponse(resp, c, account, writeChatCompletionsError)
}

// handleChatBufferedStreamingResponse reads all Responses SSE events from the
// upstream, finds the terminal event, converts to a Chat Completions JSON
// response, and writes it to the client.
func (s *OpenAIGatewayService) handleChatBufferedStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var finalResponse *apicompat.ResponsesResponse
	var usage OpenAIUsage
	acc := apicompat.NewBufferedResponseAccumulator()

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		payload := line[6:]

		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("openai chat_completions buffered: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			continue
		}

		// Accumulate delta content for fallback when terminal output is empty.
		acc.ProcessEvent(&event)

		if (event.Type == "response.completed" || event.Type == "response.done" ||
			event.Type == "response.incomplete" || event.Type == "response.failed") &&
			event.Response != nil {
			finalResponse = event.Response
			if event.Response.Usage != nil {
				usage = OpenAIUsage{
					InputTokens:  event.Response.Usage.InputTokens,
					OutputTokens: event.Response.Usage.OutputTokens,
				}
				if event.Response.Usage.InputTokensDetails != nil {
					usage.CacheReadInputTokens = event.Response.Usage.InputTokensDetails.CachedTokens
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai chat_completions buffered: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	if finalResponse == nil {
		writeChatCompletionsError(c, http.StatusBadGateway, "api_error", "Upstream stream ended without a terminal response event")
		return nil, fmt.Errorf("upstream stream ended without terminal event")
	}

	// When the terminal event has an empty output array, reconstruct from
	// accumulated delta events so the client receives the full content.
	acc.SupplementResponseOutput(finalResponse)

	chatResp := apicompat.ResponsesToChatCompletions(finalResponse, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, chatResp)

	return &OpenAIForwardResult{
		RequestID:     requestID,
		Usage:         usage,
		Model:         originalModel,
		BillingModel:  billingModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

// handleChatStreamingResponse reads Responses SSE events from upstream,
// converts each to Chat Completions SSE chunks, and writes them to the client.
func (s *OpenAIGatewayService) handleChatStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	includeUsage bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	state := apicompat.NewResponsesEventToChatState()
	state.Model = originalModel
	state.IncludeUsage = includeUsage

	var usage OpenAIUsage
	var firstTokenMs *int
	firstChunk := true

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	resultWithUsage := func() *OpenAIForwardResult {
		return &OpenAIForwardResult{
			RequestID:     requestID,
			Usage:         usage,
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
			Stream:        true,
			Duration:      time.Since(startTime),
			FirstTokenMs:  firstTokenMs,
		}
	}

	processDataLine := func(payload string) bool {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("openai chat_completions stream: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return false
		}

		// Extract usage from completion events
		if (event.Type == "response.completed" || event.Type == "response.incomplete" || event.Type == "response.failed") &&
			event.Response != nil && event.Response.Usage != nil {
			usage = OpenAIUsage{
				InputTokens:  event.Response.Usage.InputTokens,
				OutputTokens: event.Response.Usage.OutputTokens,
			}
			if event.Response.Usage.InputTokensDetails != nil {
				usage.CacheReadInputTokens = event.Response.Usage.InputTokensDetails.CachedTokens
			}
		}

		chunks := apicompat.ResponsesEventToChatChunks(&event, state)
		for _, chunk := range chunks {
			sse, err := apicompat.ChatChunkToSSE(chunk)
			if err != nil {
				logger.L().Warn("openai chat_completions stream: failed to marshal chunk",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				logger.L().Info("openai chat_completions stream: client disconnected",
					zap.String("request_id", requestID),
				)
				return true
			}
		}
		if len(chunks) > 0 {
			c.Writer.Flush()
		}
		return false
	}

	finalizeStream := func() (*OpenAIForwardResult, error) {
		if finalChunks := apicompat.FinalizeResponsesChatStream(state); len(finalChunks) > 0 {
			for _, chunk := range finalChunks {
				sse, err := apicompat.ChatChunkToSSE(chunk)
				if err != nil {
					continue
				}
				fmt.Fprint(c.Writer, sse) //nolint:errcheck
			}
		}
		// Send [DONE] sentinel
		fmt.Fprint(c.Writer, "data: [DONE]\n\n") //nolint:errcheck
		c.Writer.Flush()
		return resultWithUsage(), nil
	}

	handleScanErr := func(err error) {
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai chat_completions stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	// Determine keepalive interval
	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}

	// No keepalive: fast synchronous path
	if keepaliveInterval <= 0 {
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
				continue
			}
			if processDataLine(line[6:]) {
				return resultWithUsage(), nil
			}
		}
		handleScanErr(scanner.Err())
		return finalizeStream()
	}

	// With keepalive: goroutine + channel + select
	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	go func() {
		defer close(events)
		for scanner.Scan() {
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}()
	defer close(done)

	keepaliveTicker := time.NewTicker(keepaliveInterval)
	defer keepaliveTicker.Stop()
	lastDataAt := time.Now()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return finalizeStream()
			}
			if ev.err != nil {
				handleScanErr(ev.err)
				return finalizeStream()
			}
			lastDataAt = time.Now()
			line := ev.line
			if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
				continue
			}
			if processDataLine(line[6:]) {
				return resultWithUsage(), nil
			}

		case <-keepaliveTicker.C:
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
			}
			// Send SSE comment as keepalive
			if _, err := fmt.Fprint(c.Writer, ":\n\n"); err != nil {
				logger.L().Info("openai chat_completions stream: client disconnected during keepalive",
					zap.String("request_id", requestID),
				)
				return resultWithUsage(), nil
			}
			c.Writer.Flush()
		}
	}
}

// writeChatCompletionsError writes an error response in OpenAI Chat Completions format.
func writeChatCompletionsError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// forwardChatCompletionsNative 将 Chat Completions 请求原样（仅做模型映射）
// 转发到 <base>/v1/chat/completions，并把上游响应透传回客户端。
//
// 作者：mkx  变更：2026-04-22 新增
// 适用于 GLM / DeepSeek / Kimi / new-api 等仅实现 /v1/chat/completions 的
// OpenAI 兼容上游。不做 Responses API 转换，不注入 Codex/OAuth 专属头。
func (s *OpenAIGatewayService) forwardChatCompletionsNative(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	// 1. 解析请求元信息
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	clientStream := gjson.GetBytes(body, "stream").Bool()

	// 2. 模型映射（账号级 mapping > 分组 default mapping > 原模型）
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := billingModel
	// 仅在映射后模型与原模型不同时改写 body，避免无谓的序列化。
	var upstreamBody []byte
	if upstreamModel != "" && upstreamModel != originalModel {
		rewritten, err := sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite model in cc native body: %w", err)
		}
		upstreamBody = rewritten
	} else {
		upstreamBody = body
	}

	logger.L().Debug("openai chat_completions native: forwarding",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
	)

	// 3. 获取上游认证（API Key 账号为 Bearer token）
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	// 4. 组装上游请求
	upstreamReq, err := s.buildUpstreamRequestChatCompletionsNative(ctx, c, account, upstreamBody, token)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	// 5. 发起上游请求
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// 6. 错误响应：复用标准失败切换状态码规则
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:   resp.StatusCode,
				ResponseBody: respBody,
				// 对齐老路径的同账号重试判定：pool 模式下兼容 429/5xx 以及
				// OpenAI "an error occurred while processing" 这类 400 transient。
				RetryableOnSameAccount: account.IsPoolMode() &&
					(isPoolModeRetryableStatus(resp.StatusCode) ||
						isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleChatCompletionsErrorResponse(resp, c, account)
	}

	// 7. 正常响应
	requestID := resp.Header.Get("x-request-id")
	// 透出计费需要的服务等级 / 推理强度字段（老路径由 ResponsesRequest 回填）
	serviceTier := extractOpenAIServiceTierFromBody(body)
	reasoningEffort := extractCCReasoningEffortFromBody(body)
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	var usage OpenAIUsage
	var firstTokenMs *int

	if clientStream {
		// 流式：按行读上游 SSE，原样回写；同时嗅探 usage chunk
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)

		scanner := bufio.NewScanner(resp.Body)
		maxLineSize := defaultMaxLineSize
		if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
			maxLineSize = s.cfg.Gateway.MaxLineSize
		}
		scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

		first := true
		clientDisconnected := false
		sawDone := false
		for scanner.Scan() {
			line := scanner.Text()
			if first && strings.HasPrefix(line, "data: ") {
				first = false
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			// 客户端断连后仍继续 drain 上游，以便捕获最后一帧 usage 做计费记录
			if !clientDisconnected {
				if _, werr := fmt.Fprintln(c.Writer, line); werr != nil {
					clientDisconnected = true
				}
				if line == "" {
					c.Writer.Flush()
				}
			}
			if line == "data: [DONE]" {
				sawDone = true
			}
			// 从 data 行抓取 usage（OpenAI CC 在 include_usage=true 时最后一帧含 usage）
			if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
				payload := line[6:]
				if u := parseChatCompletionsUsageFromChunk(payload); u != nil {
					usage = *u
				}
			}
			if !clientDisconnected && line != "" {
				c.Writer.Flush()
			}
		}
		if err := scanner.Err(); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logger.L().Warn("openai chat_completions native: stream read error",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
			}
		} else if !sawDone && !clientDisconnected {
			logger.L().Warn("openai chat_completions native: stream ended without [DONE] marker",
				zap.String("request_id", requestID),
				zap.Int64("account_id", account.ID),
			)
		}
	} else {
		// 非流式：复用统一的上游响应体大小限制工具
		respBody, rerr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
		if rerr != nil {
			logger.L().Warn("openai chat_completions native: read body error",
				zap.Error(rerr),
				zap.String("request_id", requestID),
			)
			return nil, rerr
		}
		// 某些 OpenAI 兼容上游即使 stream=false 仍会返回 SSE；此时直接透传，
		// 客户端按 CC 协议可容忍（不走 Responses→JSON 的精细转换，避免引入复杂度）。
		if isEventStreamResponse(resp.Header) {
			logger.L().Warn("openai chat_completions native: upstream returned SSE for non-streaming request, forwarding as-is",
				zap.String("request_id", requestID),
				zap.Int64("account_id", account.ID),
			)
			// 从 SSE 各 data 行扫一下 usage
			for _, line := range strings.Split(string(respBody), "\n") {
				if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
					if u := parseChatCompletionsUsageFromChunk(line[6:]); u != nil {
						usage = *u
					}
				}
			}
		} else if u := parseChatCompletionsUsageFromChunk(string(respBody)); u != nil {
			usage = *u
		}
		ms := int(time.Since(startTime).Milliseconds())
		firstTokenMs = &ms
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		c.Data(http.StatusOK, contentType, respBody)
	}

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		Stream:          clientStream,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
		ServiceTier:     serviceTier,
		ReasoningEffort: reasoningEffort,
	}, nil
}

// buildUpstreamRequestChatCompletionsNative 为原生 Chat Completions 直通模式
// 构造精简上游请求：仅注入 Bearer 认证、Content-Type，透传白名单请求头。
// 作者：mkx  变更：2026-04-22 新增
func (s *OpenAIGatewayService) buildUpstreamRequestChatCompletionsNative(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	// 组装目标 URL：优先账号 base_url + /v1/chat/completions
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		return nil, fmt.Errorf("openai chat_completions native: account base_url is empty")
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	targetURL := buildOpenAIChatCompletionsURL(validatedURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 透传客户端请求头（沿用 passthrough 白名单逻辑；不包含任何 Codex/OAuth 专属头）
	allowTimeoutHeaders := s.isOpenAIPassthroughTimeoutHeadersAllowed()
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if !isOpenAIPassthroughAllowedRequestHeader(lower, allowTimeoutHeaders) {
				continue
			}
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}

	// 覆盖入站鉴权残留，注入上游 API Key
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Set("authorization", "Bearer "+token)

	// 账号自定义 UA（若有）
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("user-agent", customUA)
	}
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	return req, nil
}

// parseChatCompletionsUsageFromChunk 从一段 OpenAI CC 格式的 JSON 文本
// （可能是流式 chunk，也可能是非流式完整响应）中提取 usage。
// 作者：mkx  变更：2026-04-22 新增
// 字段约定：
//   - usage.prompt_tokens         → InputTokens
//   - usage.completion_tokens     → OutputTokens
//   - usage.prompt_tokens_details.cached_tokens → CacheReadInputTokens（可选）
//
// 未命中返回 nil，交由调用方保留已有值（支持多帧情况下的覆盖语义）。
func parseChatCompletionsUsageFromChunk(payload string) *OpenAIUsage {
	usageResult := gjson.Get(payload, "usage")
	if !usageResult.Exists() || !usageResult.IsObject() {
		return nil
	}
	prompt := usageResult.Get("prompt_tokens").Int()
	completion := usageResult.Get("completion_tokens").Int()
	cached := usageResult.Get("prompt_tokens_details.cached_tokens").Int()
	// 全 0 视为未携带 usage（某些上游会把空 usage 对象放在 chunk 里）
	if prompt == 0 && completion == 0 && cached == 0 {
		return nil
	}
	return &OpenAIUsage{
		InputTokens:          int(prompt),
		OutputTokens:         int(completion),
		CacheReadInputTokens: int(cached),
	}
}
