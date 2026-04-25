//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// swapMonitorHTTPClient 临时替换 monitorHTTPClient 为不带 SSRF 校验的普通 client，
// 让 httptest (127.0.0.1) 能连通。测试结束后恢复。
func swapMonitorHTTPClient(t *testing.T) {
	t.Helper()
	orig := monitorHTTPClient
	monitorHTTPClient = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() { monitorHTTPClient = orig })
}

// captureHandler 把每次收到的请求 body 和 headers 存起来，测试断言用。
type captureHandler struct {
	lastBody    map[string]any
	lastHeaders http.Header
	respondText string // 写到 Anthropic content[0].text 里（校验用）
	status      int
}

func (h *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lastHeaders = r.Header.Clone()
	defer func() { _ = r.Body.Close() }()
	var parsed map[string]any
	_ = json.NewDecoder(r.Body).Decode(&parsed)
	h.lastBody = parsed

	if h.status == 0 {
		h.status = 200
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.status)
	// 构造 Anthropic 格式的响应：content[0].text = h.respondText
	_ = json.NewEncoder(w).Encode(map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": h.respondText},
		},
	})
}

func setupFakeAnthropic(t *testing.T, handler http.Handler) string {
	t.Helper()
	swapMonitorHTTPClient(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRunCheckForModel_OffMode_PreservesDefaultBody(t *testing.T) {
	h := &captureHandler{respondText: "the answer is 42"}
	endpoint := setupFakeAnthropic(t, h)

	// 跑一次 off 模式（opts=nil），确认 Anthropic 默认请求保持 Claude Code 模拟形态。
	_ = runCheckForModel(context.Background(), MonitorProviderAnthropic, endpoint, "sk-fake", "claude-x", nil)

	if h.lastBody["model"] != "claude-x" {
		t.Errorf("default body should contain model=claude-x, got %v", h.lastBody["model"])
	}
	if _, ok := h.lastBody["messages"]; !ok {
		t.Error("default body should contain messages")
	}
	if stream, ok := h.lastBody["stream"].(bool); !ok || !stream {
		t.Errorf("default Anthropic body should mimic Claude Code stream=true, got %v", h.lastBody["stream"])
	}
	if temperature, ok := h.lastBody["temperature"].(float64); !ok || temperature != 1 {
		t.Errorf("default Anthropic body should set temperature=1, got %v", h.lastBody["temperature"])
	}
	if mt, ok := h.lastBody["max_tokens"].(float64); !ok || mt != monitorChallengeMaxTokens {
		t.Errorf("default Anthropic body should set max_tokens=%d, got %v", monitorChallengeMaxTokens, h.lastBody["max_tokens"])
	}
	systemBlocks, _ := h.lastBody["system"].([]any)
	if len(systemBlocks) == 0 {
		t.Fatal("default Anthropic body should contain Claude Code system block")
	}
	systemBlock, _ := systemBlocks[0].(map[string]any)
	if systemBlock["text"] != claudeCodeSystemPrompt {
		t.Errorf("default Anthropic system prompt mismatch: %v", systemBlock["text"])
	}
	if _, ok := h.lastBody["metadata"].(map[string]any); !ok {
		t.Errorf("default Anthropic body should contain metadata.user_id, got %v", h.lastBody["metadata"])
	}
	if h.lastHeaders.Get("x-api-key") != "sk-fake" {
		t.Errorf("expected adapter's x-api-key header, got %q", h.lastHeaders.Get("x-api-key"))
	}
	if h.lastHeaders.Get("User-Agent") != claude.DefaultHeaders["User-Agent"] {
		t.Errorf("expected Claude Code User-Agent, got %q", h.lastHeaders.Get("User-Agent"))
	}
	if h.lastHeaders.Get("anthropic-beta") != claude.APIKeyBetaHeader {
		t.Errorf("expected API-key Claude Code beta header, got %q", h.lastHeaders.Get("anthropic-beta"))
	}
	if h.lastHeaders.Get("x-client-request-id") == "" {
		t.Error("expected x-client-request-id to be generated")
	}
}

func TestRunCheckForModel_AnthropicStreamResponseChallenge(t *testing.T) {
	h := &anthropicStreamChallengeHandler{}
	endpoint := setupFakeAnthropic(t, h)

	res := runCheckForModel(context.Background(), MonitorProviderAnthropic, endpoint, "sk-fake", "claude-x", nil)

	if res.Status != MonitorStatusOperational {
		t.Fatalf("stream response should pass challenge, got status=%s message=%q", res.Status, res.Message)
	}
	if h.lastPath != "/v1/messages" {
		t.Errorf("expected messages path, got %q", h.lastPath)
	}
	if h.lastQuery != "beta=true" {
		t.Errorf("expected beta=true query, got %q", h.lastQuery)
	}
}

func TestRunCheckForModel_Anthropic2xxMismatchIsOperational(t *testing.T) {
	h := &captureHandler{respondText: "hello"}
	endpoint := setupFakeAnthropic(t, h)

	res := runCheckForModel(context.Background(), MonitorProviderAnthropic, endpoint, "sk-fake", "claude-x", nil)

	if res.Status != MonitorStatusOperational {
		t.Fatalf("2xx response should be operational even when challenge text mismatches, got status=%s message=%q", res.Status, res.Message)
	}
}

func TestRunCheckForModel_MergeMode_UserFieldsWinButDenyListProtects(t *testing.T) {
	h := &captureHandler{respondText: "the answer is 42"}
	endpoint := setupFakeAnthropic(t, h)

	opts := &CheckOptions{
		BodyOverrideMode: MonitorBodyOverrideModeMerge,
		BodyOverride: map[string]any{
			"system":     "You are Claude Code...",
			"max_tokens": float64(999),   // 应该覆盖默认 50
			"model":      "hacked-model", // 应该被黑名单挡住，保留原 model
			"messages":   []any{},        // 同上，被挡
		},
		ExtraHeaders: map[string]string{
			"User-Agent":     "claude-cli/1.0",
			"Content-Length": "999", // 黑名单
			"x-custom":       "ok",
		},
	}
	_ = runCheckForModel(context.Background(), MonitorProviderAnthropic, endpoint, "sk-fake", "claude-x", opts)

	if h.lastBody["system"] != "You are Claude Code..." {
		t.Errorf("merge mode should inject system, got %v", h.lastBody["system"])
	}
	// max_tokens 覆盖生效
	if mt, ok := h.lastBody["max_tokens"].(float64); !ok || mt != 999 {
		t.Errorf("merge mode should override max_tokens to 999, got %v", h.lastBody["max_tokens"])
	}
	// model 在黑名单 — 应该保留默认值
	if h.lastBody["model"] != "claude-x" {
		t.Errorf("model should be protected by deny list, got %v", h.lastBody["model"])
	}
	// messages 在黑名单 — 应该保留默认值（非空）
	msgs, _ := h.lastBody["messages"].([]any)
	if len(msgs) == 0 {
		t.Error("messages should be protected by deny list (kept default, non-empty)")
	}
	// header 合并
	if h.lastHeaders.Get("User-Agent") != "claude-cli/1.0" {
		t.Errorf("extra User-Agent should override, got %q", h.lastHeaders.Get("User-Agent"))
	}
	if h.lastHeaders.Get("x-custom") != "ok" {
		t.Errorf("extra custom header should be present, got %q", h.lastHeaders.Get("x-custom"))
	}
	// Content-Length 黑名单：会被 net/http 自动重算，但不应由用户的 "999" 决定。
	// 我们无法直接断言丢弃（http.Client 总会填上），只断言请求成功即可。
}

func TestRunCheckForModel_ReplaceMode_FullBodyUsedAndChallengeSkipped(t *testing.T) {
	// replace 模式下我们的 body 完全自定义，challenge 数学题不会出现在请求里，
	// 上游也不会回正确答案 — 但只要返回 2xx，就算 operational
	h := &captureHandler{respondText: "any non-empty text"}
	endpoint := setupFakeAnthropic(t, h)

	userBody := map[string]any{
		"model":      "user-forced-model",
		"messages":   []any{map[string]any{"role": "user", "content": "hi"}},
		"max_tokens": float64(10),
		"system":     "You are someone else",
	}
	opts := &CheckOptions{
		BodyOverrideMode: MonitorBodyOverrideModeReplace,
		BodyOverride:     userBody,
	}
	res := runCheckForModel(context.Background(), MonitorProviderAnthropic, endpoint, "sk-fake", "claude-x", opts)

	// 请求 body = 用户提供的原样
	if h.lastBody["model"] != "user-forced-model" {
		t.Errorf("replace mode should use user's model, got %v", h.lastBody["model"])
	}
	if h.lastBody["system"] != "You are someone else" {
		t.Errorf("replace mode should use user's system, got %v", h.lastBody["system"])
	}
	// challenge 虽然没命中，但 2xx 即视为可用。
	if res.Status != MonitorStatusOperational {
		t.Errorf("replace mode with 2xx should be operational, got status=%s message=%q",
			res.Status, res.Message)
	}
}

func TestRunCheckForModel_ReplaceMode_Empty2xxResponseIsOperational(t *testing.T) {
	h := &captureHandler{respondText: ""} // 上游 200 但 content[0].text 为空
	endpoint := setupFakeAnthropic(t, h)

	opts := &CheckOptions{
		BodyOverrideMode: MonitorBodyOverrideModeReplace,
		BodyOverride:     map[string]any{"model": "x", "messages": []any{}},
	}
	res := runCheckForModel(context.Background(), MonitorProviderAnthropic, endpoint, "sk-fake", "claude-x", opts)

	if res.Status != MonitorStatusOperational {
		t.Errorf("replace mode with empty 2xx response should be operational, got status=%s message=%q", res.Status, res.Message)
	}
}

type anthropicStreamChallengeHandler struct {
	lastBody  map[string]any
	lastPath  string
	lastQuery string
}

func (h *anthropicStreamChallengeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lastPath = r.URL.Path
	h.lastQuery = r.URL.RawQuery
	defer func() { _ = r.Body.Close() }()
	_ = json.NewDecoder(r.Body).Decode(&h.lastBody)

	answer, err := solveMonitorChallengeFromBody(h.lastBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	writeSSEData(w, map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{
			"type": "text_delta",
			"text": answer,
		},
	})
}

var monitorChallengeQuestionRegex = regexp.MustCompile(`Q:\s*(\d+)\s*([+-])\s*(\d+)\s*=\s*\?\s*A:`)

func solveMonitorChallengeFromBody(body map[string]any) (string, error) {
	prompt, err := monitorPromptFromBody(body)
	if err != nil {
		return "", err
	}
	matches := monitorChallengeQuestionRegex.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("challenge prompt not found")
	}
	last := matches[len(matches)-1]
	left, _ := strconv.Atoi(last[1])
	right, _ := strconv.Atoi(last[3])
	switch last[2] {
	case "+":
		return strconv.Itoa(left + right), nil
	case "-":
		return strconv.Itoa(left - right), nil
	default:
		return "", fmt.Errorf("unsupported operator %q", last[2])
	}
}

func monitorPromptFromBody(body map[string]any) (string, error) {
	messages, _ := body["messages"].([]any)
	if len(messages) == 0 {
		return "", fmt.Errorf("messages missing")
	}
	first, _ := messages[0].(map[string]any)
	content, _ := first["content"].([]any)
	if len(content) == 0 {
		return "", fmt.Errorf("message content missing")
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	if text == "" {
		return "", fmt.Errorf("message text missing")
	}
	return text, nil
}

func writeSSEData(w http.ResponseWriter, payload map[string]any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "event: %s\n", payload["type"])
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
