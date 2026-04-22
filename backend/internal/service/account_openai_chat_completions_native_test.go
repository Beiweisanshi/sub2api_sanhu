// 作者：mkx  变更：2026-04-22 新增
// 覆盖 Account.IsOpenAIChatCompletionsNativeEnabled 开关的各种守卫场景。
package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsOpenAIChatCompletionsNativeEnabled(t *testing.T) {
	t.Run("API Key 账号开启", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_chat_completions_mode_enabled": true,
			},
		}
		require.True(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})

	t.Run("API Key 账号显式关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_chat_completions_mode_enabled": false,
			},
		}
		require.False(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})

	t.Run("字段缺失默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{},
		}
		require.False(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})

	t.Run("Extra 为 nil 默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
		}
		require.False(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})

	t.Run("类型非法默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_chat_completions_mode_enabled": "true",
			},
		}
		require.False(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})

	t.Run("OAuth 账号始终关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_chat_completions_mode_enabled": true,
			},
		}
		require.False(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})

	t.Run("非 OpenAI 平台始终关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_chat_completions_mode_enabled": true,
			},
		}
		require.False(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})

	t.Run("nil 账号安全返回 false", func(t *testing.T) {
		var account *Account
		require.False(t, account.IsOpenAIChatCompletionsNativeEnabled())
	})
}

func TestBuildOpenAIChatCompletionsURL(t *testing.T) {
	t.Run("base 已是完整 chat/completions 路径", func(t *testing.T) {
		require.Equal(t,
			"https://example.com/v1/chat/completions",
			buildOpenAIChatCompletionsURL("https://example.com/v1/chat/completions"))
	})
	t.Run("base 以 /v1 结尾", func(t *testing.T) {
		require.Equal(t,
			"https://example.com/v1/chat/completions",
			buildOpenAIChatCompletionsURL("https://example.com/v1"))
	})
	t.Run("base 只有域名", func(t *testing.T) {
		require.Equal(t,
			"https://example.com/v1/chat/completions",
			buildOpenAIChatCompletionsURL("https://example.com"))
	})
	t.Run("带尾部斜杠", func(t *testing.T) {
		require.Equal(t,
			"https://example.com/v1/chat/completions",
			buildOpenAIChatCompletionsURL("https://example.com/v1/"))
	})
	t.Run("base 以 /v1/responses 结尾（从老配置迁移）", func(t *testing.T) {
		require.Equal(t,
			"https://example.com/v1/chat/completions",
			buildOpenAIChatCompletionsURL("https://example.com/v1/responses"))
	})
	t.Run("base 以 /responses 结尾", func(t *testing.T) {
		require.Equal(t,
			"https://example.com/v1/chat/completions",
			buildOpenAIChatCompletionsURL("https://example.com/responses"))
	})
	t.Run("火山 Ark v3 根路径", func(t *testing.T) {
		require.Equal(t,
			"https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions",
			buildOpenAIChatCompletionsURL("https://ark.cn-beijing.volces.com/api/coding/v3"))
	})
	t.Run("火山 Ark v3 根路径带尾斜杠", func(t *testing.T) {
		require.Equal(t,
			"https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions",
			buildOpenAIChatCompletionsURL("https://ark.cn-beijing.volces.com/api/coding/v3/"))
	})
	t.Run("泛化识别 v2 版本根", func(t *testing.T) {
		require.Equal(t,
			"https://example.com/v2/chat/completions",
			buildOpenAIChatCompletionsURL("https://example.com/v2"))
	})
	t.Run("火山 Ark v3 responses 路径", func(t *testing.T) {
		require.Equal(t,
			"https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions",
			buildOpenAIChatCompletionsURL("https://ark.cn-beijing.volces.com/api/coding/v3/responses"))
	})
}

func TestParseChatCompletionsUsageFromChunk(t *testing.T) {
	t.Run("标准 usage", func(t *testing.T) {
		u := parseChatCompletionsUsageFromChunk(`{"usage":{"prompt_tokens":12,"completion_tokens":34}}`)
		require.NotNil(t, u)
		require.Equal(t, 12, u.InputTokens)
		require.Equal(t, 34, u.OutputTokens)
	})
	t.Run("含 prompt_tokens_details.cached_tokens", func(t *testing.T) {
		u := parseChatCompletionsUsageFromChunk(`{"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":80}}}`)
		require.NotNil(t, u)
		require.Equal(t, 80, u.CacheReadInputTokens)
	})
	t.Run("空 usage 对象返回 nil", func(t *testing.T) {
		require.Nil(t, parseChatCompletionsUsageFromChunk(`{"usage":{}}`))
	})
	t.Run("全 0 usage 视为无效", func(t *testing.T) {
		require.Nil(t, parseChatCompletionsUsageFromChunk(`{"usage":{"prompt_tokens":0,"completion_tokens":0}}`))
	})
	t.Run("无 usage 字段", func(t *testing.T) {
		require.Nil(t, parseChatCompletionsUsageFromChunk(`{"choices":[{"delta":{"content":"hi"}}]}`))
	})
	t.Run("非法 JSON", func(t *testing.T) {
		require.Nil(t, parseChatCompletionsUsageFromChunk(`not json`))
	})
}
