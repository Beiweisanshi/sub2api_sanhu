package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type openAIImageCompatErrorWriter func(*gin.Context, int, string, string)

const (
	openAIImageCompatSourceChatCompletions = "chat_completions"
	openAIImageCompatSourceResponses       = "responses"
	openAIImageCompatSourceMessages        = "messages"
)

// routeOpenAIImageCompat 将误打到文本入口的 gpt-image-* 请求清洗成 Images API 请求。
// 只处理纯生图语义：抽取文本 prompt，保留图片接口支持的少量参数，丢弃 messages/input
// 等文本对话字段，避免 Codex normalizer 将图片模型兜底成文本模型。
func (h *OpenAIGatewayHandler) routeOpenAIImageCompat(
	c *gin.Context,
	body []byte,
	source string,
	reqModel string,
	reqLog *zap.Logger,
	writeErr openAIImageCompatErrorWriter,
) bool {
	if !isOpenAIImageCompatModel(reqModel) {
		return false
	}

	rewritten, err := buildOpenAIImageCompatRequestBody(body, source, reqModel)
	if err != nil {
		if writeErr == nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		} else {
			writeErr(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		}
		return true
	}

	if reqLog != nil {
		reqLog.Info("openai.image_compat_route",
			zap.String("source", source),
			zap.String("target_endpoint", EndpointImagesGenerations),
			zap.Int("rewritten_body_bytes", len(rewritten)),
		)
	}

	c.Request.Body = io.NopCloser(bytes.NewReader(rewritten))
	c.Request.ContentLength = int64(len(rewritten))
	c.Request.Header.Set("Content-Type", "application/json")
	if c.Request.URL != nil {
		c.Request.URL.Path = EndpointImagesGenerations
		c.Request.URL.RawPath = ""
	}
	c.Set(ctxKeyInboundEndpoint, EndpointImagesGenerations)

	h.Images(c)
	return true
}

func isOpenAIImageCompatModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image-")
}

func buildOpenAIImageCompatRequestBody(body []byte, source string, reqModel string) ([]byte, error) {
	prompt := extractOpenAIImageCompatPrompt(body, source)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("prompt is required for gpt-image compatibility mode")
	}

	payload := map[string]any{
		"model":  strings.TrimSpace(reqModel),
		"prompt": prompt,
	}
	copyOpenAIImageCompatFields(body, payload)

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("rewrite image request body: %w", err)
	}
	return rewritten, nil
}

func copyOpenAIImageCompatFields(body []byte, payload map[string]any) {
	if n := gjson.GetBytes(body, "n"); n.Exists() && n.Type == gjson.Number {
		payload["n"] = n.Value()
	}
	if stream := gjson.GetBytes(body, "stream"); stream.Exists() && (stream.Type == gjson.True || stream.Type == gjson.False) {
		payload["stream"] = stream.Value()
	}
	if compression := gjson.GetBytes(body, "output_compression"); compression.Exists() && compression.Type == gjson.Number {
		payload["output_compression"] = compression.Value()
	}

	for _, field := range []string{"size", "background", "quality", "style", "output_format", "moderation"} {
		result := gjson.GetBytes(body, field)
		if result.Exists() && result.Type == gjson.String && strings.TrimSpace(result.String()) != "" {
			payload[field] = result.Value()
		}
	}

	if responseFormat := gjson.GetBytes(body, "response_format"); responseFormat.Exists() && responseFormat.Type == gjson.String {
		switch strings.ToLower(strings.TrimSpace(responseFormat.String())) {
		case "b64_json", "url":
			payload["response_format"] = responseFormat.String()
		}
	}
}

func extractOpenAIImageCompatPrompt(body []byte, source string) string {
	if prompt := strings.TrimSpace(gjson.GetBytes(body, "prompt").String()); prompt != "" {
		return prompt
	}

	switch source {
	case openAIImageCompatSourceChatCompletions, openAIImageCompatSourceMessages:
		return extractPromptFromCompatMessages(gjson.GetBytes(body, "messages"))
	case openAIImageCompatSourceResponses:
		return extractPromptFromCompatResponsesInput(gjson.GetBytes(body, "input"))
	default:
		return ""
	}
}

func extractPromptFromCompatMessages(messages gjson.Result) string {
	if !messages.IsArray() {
		return ""
	}
	var all []string
	var lastUser []string
	for _, msg := range messages.Array() {
		texts := collectOpenAIImageCompatContentText(msg.Get("content"))
		if len(texts) == 0 {
			continue
		}
		all = append(all, texts...)
		if strings.EqualFold(strings.TrimSpace(msg.Get("role").String()), "user") {
			lastUser = texts
		}
	}
	if len(lastUser) > 0 {
		return strings.Join(lastUser, "\n")
	}
	return strings.Join(all, "\n")
}

func extractPromptFromCompatResponsesInput(input gjson.Result) string {
	if input.Type == gjson.String {
		return strings.TrimSpace(input.String())
	}
	if !input.IsArray() {
		return ""
	}

	var all []string
	var lastUser []string
	for _, item := range input.Array() {
		texts := collectOpenAIImageCompatContentText(item)
		if len(texts) == 0 {
			continue
		}
		all = append(all, texts...)
		if strings.EqualFold(strings.TrimSpace(item.Get("role").String()), "user") {
			lastUser = texts
		}
	}
	if len(lastUser) > 0 {
		return strings.Join(lastUser, "\n")
	}
	return strings.Join(all, "\n")
}

func collectOpenAIImageCompatContentText(value gjson.Result) []string {
	switch {
	case value.Type == gjson.String:
		if text := strings.TrimSpace(value.String()); text != "" {
			return []string{text}
		}
		return nil
	case value.IsArray():
		var out []string
		for _, item := range value.Array() {
			out = append(out, collectOpenAIImageCompatContentText(item)...)
		}
		return out
	case value.IsObject():
		if text := strings.TrimSpace(value.Get("text").String()); text != "" && !isOpenAIImageCompatImagePart(value) {
			return []string{text}
		}
		if content := value.Get("content"); content.Exists() {
			return collectOpenAIImageCompatContentText(content)
		}
		if message := value.Get("message"); message.Exists() {
			return collectOpenAIImageCompatContentText(message.Get("content"))
		}
		return nil
	default:
		return nil
	}
}

func isOpenAIImageCompatImagePart(value gjson.Result) bool {
	partType := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
	return strings.Contains(partType, "image")
}
