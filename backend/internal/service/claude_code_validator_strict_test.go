package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelDisallowsAssistantPrefill(t *testing.T) {
	cases := map[string]bool{
		"claude-sonnet-4-6-20250514": true,
		"claude-opus-4-6":            true,
		"claude-haiku-4-7":           true,
		"claude-sonnet-5-0":          true,
		"claude-sonnet-4-5-20250514": false, // 4.5 still allows prefill
		"claude-sonnet-3-7":          false,
		"claude-sonnet-4-5":          false,
		"mythos-preview-001":         true,
		"CLAUDE-MYTHOS-EX":           true, // case-insensitive
		"gpt-4o":                     false,
		"":                           false,
	}
	for model, want := range cases {
		if got := ModelDisallowsAssistantPrefill(model); got != want {
			t.Fatalf("ModelDisallowsAssistantPrefill(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestValidateMessagesStrict_AssistantPrefillRejected(t *testing.T) {
	body := map[string]any{
		"model": "claude-sonnet-4-6-20250514",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": "Let me think…"},
		},
	}
	err := ValidateMessagesStrict(body)
	require.NotNil(t, err)
	assert.Equal(t, "invalid_request_error", err.ErrorType)
	assert.Contains(t, err.Detail, "assistant message prefill")
}

func TestValidateMessagesStrict_AssistantPrefillAllowedFor45(t *testing.T) {
	body := map[string]any{
		"model": "claude-sonnet-4-5-20250514",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": "prefill ok"},
		},
	}
	require.Nil(t, ValidateMessagesStrict(body))
}

func TestValidateMessagesStrict_ThinkingSignatureTooShort(t *testing.T) {
	body := map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "thinking", "thinking": "reasoning", "signature": "abc"},
				map[string]any{"type": "text", "text": "done"},
			}},
		},
	}
	err := ValidateMessagesStrict(body)
	require.NotNil(t, err)
	assert.Contains(t, err.Detail, "invalid thinking.signature")
	assert.Contains(t, err.Detail, "messages.1.content.0")
}

func TestValidateMessagesStrict_ThinkingSignatureBadChars(t *testing.T) {
	body := map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{
					"type":      "thinking",
					"thinking":  "r",
					"signature": strings.Repeat("!", 40), // non-base64 chars
				},
			}},
		},
	}
	err := ValidateMessagesStrict(body)
	require.NotNil(t, err)
	assert.Contains(t, err.Detail, "invalid thinking.signature format")
}

func TestValidateMessagesStrict_ThinkingOnUserRoleRejected(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "thinking", "thinking": "r", "signature": strings.Repeat("A", 40)},
			}},
		},
	}
	err := ValidateMessagesStrict(body)
	require.NotNil(t, err)
	assert.Contains(t, err.Detail, "thinking block is only valid in assistant")
}

func TestValidateMessagesStrict_RedactedThinkingEmptyData(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "redacted_thinking", "data": ""},
			}},
		},
	}
	err := ValidateMessagesStrict(body)
	require.NotNil(t, err)
	assert.Contains(t, err.Detail, "redacted_thinking block must include non-empty")
}

func TestValidateMessagesStrict_ValidThinkingAccepted(t *testing.T) {
	body := map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{
					"type":      "thinking",
					"thinking":  "Long reasoning here",
					"signature": "ABCdef1234567890_-+/=abcdefghijklmnopqrstuvwxyz", // ≥32, base64-url chars
				},
				map[string]any{"type": "text", "text": "ok"},
			}},
			map[string]any{"role": "user", "content": "continue"},
		},
	}
	require.Nil(t, ValidateMessagesStrict(body))
}

func TestValidateMessagesStrict_NoMessagesNoOp(t *testing.T) {
	require.Nil(t, ValidateMessagesStrict(nil))
	require.Nil(t, ValidateMessagesStrict(map[string]any{}))
	require.Nil(t, ValidateMessagesStrict(map[string]any{"messages": "wrong shape"}))
}
