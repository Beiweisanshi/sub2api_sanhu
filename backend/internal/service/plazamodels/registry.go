// mkx: 迁移模型广场平台白名单，2026-04-24
package plazamodels

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

var openaiModels = []string{
	"gpt-3.5-turbo", "gpt-3.5-turbo-0125", "gpt-3.5-turbo-1106", "gpt-3.5-turbo-16k",
	"gpt-4", "gpt-4-turbo", "gpt-4-turbo-preview",
	"gpt-4o", "gpt-4o-2024-08-06", "gpt-4o-2024-11-20",
	"gpt-4o-mini", "gpt-4o-mini-2024-07-18",
	"gpt-4.5-preview", "gpt-4.1", "gpt-4.1-mini", "gpt-4.1-nano",
	"o1", "o1-preview", "o1-mini", "o1-pro",
	"o3", "o3-mini", "o3-pro", "o4-mini",
	"gpt-5.2", "gpt-5.2-2025-12-11", "gpt-5.2-chat-latest", "gpt-5.2-pro", "gpt-5.2-pro-2025-12-11",
	"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.4-2026-03-05",
	"gpt-5.3-codex", "gpt-5.3-codex-spark", "chatgpt-4o-latest",
	"gpt-4o-audio-preview", "gpt-4o-realtime-preview",
	"gpt-image-1", "gpt-image-1.5", "gpt-image-2",
}

var claudeModels = []string{
	"claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20240620",
	"claude-3-5-haiku-20241022",
	"claude-3-opus-20240229", "claude-3-sonnet-20240229", "claude-3-haiku-20240307",
	"claude-3-7-sonnet-20250219", "claude-sonnet-4-20250514", "claude-opus-4-20250514",
	"claude-opus-4-1-20250805", "claude-sonnet-4-5-20250929", "claude-haiku-4-5-20251001",
	"claude-opus-4-5-20251101", "claude-opus-4-6", "claude-opus-4-7",
	"claude-sonnet-4-6", "claude-2.1", "claude-2.0", "claude-instant-1.2",
}

var geminiModels = []string{
	"gemini-3.1-flash-image", "gemini-2.5-flash-image", "gemini-2.0-flash",
	"gemini-2.5-flash", "gemini-2.5-pro", "gemini-3-flash-preview", "gemini-3-pro-preview",
}

var antigravityModels = []string{
	"claude-opus-4-6", "claude-opus-4-6-thinking", "claude-opus-4-7", "claude-opus-4-5-thinking",
	"claude-sonnet-4-6", "claude-sonnet-4-5", "claude-sonnet-4-5-thinking",
	"gemini-3.1-flash-image", "gemini-2.5-flash-image", "gemini-2.5-flash", "gemini-2.5-flash-lite",
	"gemini-2.5-flash-thinking", "gemini-2.5-pro", "gemini-3-flash", "gemini-3-pro-high", "gemini-3-pro-low",
	"gemini-3.1-pro-high", "gemini-3.1-pro-low", "gemini-3-pro-image",
	"gpt-oss-120b-medium", "tab_flash_lite_preview",
}

// ModelsByPlatform 返回平台支持模型列表副本，避免调用方修改注册表。
func ModelsByPlatform(platform string) []string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case domain.PlatformOpenAI:
		return cloneModels(openaiModels)
	case domain.PlatformAnthropic, "claude":
		return cloneModels(claudeModels)
	case domain.PlatformGemini:
		return cloneModels(geminiModels)
	case domain.PlatformAntigravity:
		return cloneModels(antigravityModels)
	default:
		return nil
	}
}

// ModelsForScope 返回指定模型范围在平台下的候选模型。
func ModelsForScope(platform, scope string) []string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "claude":
		return cloneModels(claudeModels)
	case "gemini_text":
		return filterGeminiModels(false)
	case "gemini_image":
		return filterGeminiModels(true)
	default:
		return ModelsByPlatform(platform)
	}
}

func filterGeminiModels(wantImage bool) []string {
	out := make([]string, 0, len(geminiModels))
	for _, model := range geminiModels {
		hasImage := strings.Contains(strings.ToLower(model), "image")
		if hasImage == wantImage {
			out = append(out, model)
		}
	}
	return out
}

func cloneModels(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}
