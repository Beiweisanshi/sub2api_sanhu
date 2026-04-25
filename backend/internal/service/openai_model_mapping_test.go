package service

import (
	"errors"
	"testing"
)

func TestResolveOpenAIForwardModel(t *testing.T) {
	tests := []struct {
		name               string
		account            *Account
		requestedModel     string
		defaultMappedModel string
		expectedModel      string
	}{
		{
			name: "falls back to group default when account has no mapping",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "claude-opus-4-6",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-4o-mini",
		},
		{
			name: "preserves explicit gpt-5.4 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "preserves exact passthrough mapping instead of group default",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "preserves wildcard passthrough mapping instead of group default",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-*": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "uses account remap when explicit target differs",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "preserves codex spark instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.3-codex-spark",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt-5.3-codex-spark",
		},
		{
			name: "preserves gpt-5.5 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.5",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt-5.5",
		},
		{
			name: "preserves openai namespaced gpt-5.5 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "openai/gpt-5.5",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "openai/gpt-5.5",
		},
		{
			name: "preserves compact gpt-5.5 instead of group default",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.5-openai-compact",
			defaultMappedModel: "gpt-5.4",
			expectedModel:      "gpt-5.5-openai-compact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAIForwardModel(tt.account, tt.requestedModel, tt.defaultMappedModel); got != tt.expectedModel {
				t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", got, tt.expectedModel)
			}
		})
	}
}

func TestResolveOpenAIForwardModel_PreventsClaudeModelFromFallingBackToGpt54(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{},
	}

	withoutDefault := normalizeCodexModel(resolveOpenAIForwardModel(account, "claude-opus-4-6", ""))
	if withoutDefault != "gpt-5.4" {
		t.Fatalf("normalizeCodexModel(...) = %q, want %q", withoutDefault, "gpt-5.4")
	}

	withDefault := normalizeCodexModel(resolveOpenAIForwardModel(account, "claude-opus-4-6", "gpt-5.4"))
	if withDefault != "gpt-5.4" {
		t.Fatalf("normalizeCodexModel(...) = %q, want %q", withDefault, "gpt-5.4")
	}
}

func TestResolveOpenAICompactForwardModel(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		model         string
		expectedModel string
	}{
		{
			name:          "nil account keeps original model",
			account:       nil,
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
		{
			name: "missing compact mapping keeps original model",
			account: &Account{
				Credentials: map[string]any{},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
		{
			name: "exact compact mapping overrides model",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4-openai-compact",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4-openai-compact",
		},
		{
			name: "wildcard compact mapping overrides model",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.*": "gpt-5-openai-compact",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5-openai-compact",
		},
		{
			name: "passthrough compact mapping remains unchanged",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAICompactForwardModel(tt.account, tt.model); got != tt.expectedModel {
				t.Fatalf("resolveOpenAICompactForwardModel(...) = %q, want %q", got, tt.expectedModel)
			}
		})
	}
}

func TestNormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.3-codex-spark":       "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-high":  "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex-spark",
		"gpt-5.3":                   "gpt-5.3-codex",
	}

	for input, expected := range cases {
		if got := normalizeCodexModel(input); got != expected {
			t.Fatalf("normalizeCodexModel(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeOpenAIModelForUpstream(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		model   string
		want    string
	}{
		{
			name:    "oauth keeps codex normalization behavior",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gemini-3-flash-preview",
			want:    "gpt-5.4",
		},
		{
			name:    "apikey preserves custom compatible model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
		},
		{
			name:    "apikey preserves official non codex model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gpt-4.1",
			want:    "gpt-4.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeOpenAIModelForUpstream(tt.account, tt.model)
			if err != nil {
				t.Fatalf("normalizeOpenAIModelForUpstream(...) unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeOpenAIModelForUpstream(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNormalizeOpenAIModelForUpstream_RejectsFallbackWhenAccountLacksGpt54
// 作者: mkx  变更: 2026/04/24
// image-only free 账号只在 model_mapping 里配了 gpt-image-*，没有 gpt-5.4。
// 客户端误把 gpt-image-2 发到 /v1/chat/completions 时，旧版会静默兜底到 gpt-5.4，
// 再被 ChatGPT 后端以 401 拒绝，级联烧号。新版必须直接返回
// *UnsupportedUpstreamModelError 让调用方提前拒绝。
func TestNormalizeOpenAIModelForUpstream_RejectsFallbackWhenAccountLacksGpt54(t *testing.T) {
	imageOnlyOAuth := &Account{
		ID:       2665,
		Name:     "image-only-free",
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-image-2": "gpt-image-2",
			},
		},
	}

	t.Run("unknown model falls through to fallback and is rejected", func(t *testing.T) {
		got, err := normalizeOpenAIModelForUpstream(imageOnlyOAuth, "gpt-image-2")
		if err == nil {
			t.Fatalf("expected UnsupportedUpstreamModelError, got %q with nil err", got)
		}
		if !IsUnsupportedUpstreamModelError(err) {
			t.Fatalf("expected UnsupportedUpstreamModelError, got %T: %v", err, err)
		}
		if got != "" {
			t.Fatalf("expected empty upstream model on reject, got %q", got)
		}
	})

	t.Run("explicit codex model still resolves even without gpt-5.4 mapping", func(t *testing.T) {
		// gpt-5.3-codex 是显式命中，不走兜底，应该放行即使账号没有 gpt-5.4 mapping。
		got, err := normalizeOpenAIModelForUpstream(imageOnlyOAuth, "gpt-5.3-codex")
		if err != nil {
			t.Fatalf("unexpected err for explicit codex model: %v", err)
		}
		if got != "gpt-5.3-codex" {
			t.Fatalf("got %q want %q", got, "gpt-5.3-codex")
		}
	})

	t.Run("plus account with gpt-5.4 mapping keeps legacy fallback", func(t *testing.T) {
		plusOAuth := &Account{
			ID:       100,
			Name:     "plus-chat",
			Type:     AccountTypeOAuth,
			Platform: PlatformOpenAI,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.4": "gpt-5.4",
				},
			},
		}
		got, err := normalizeOpenAIModelForUpstream(plusOAuth, "claude-opus-4-6")
		if err != nil {
			t.Fatalf("unexpected err for plus account fallback: %v", err)
		}
		if got != "gpt-5.4" {
			t.Fatalf("got %q want %q", got, "gpt-5.4")
		}
	})

	t.Run("account with no mapping keeps legacy fallback", func(t *testing.T) {
		// 无 mapping == allow-all，保持旧版兜底行为，避免破坏现有部署。
		noMapping := &Account{ID: 200, Type: AccountTypeOAuth, Platform: PlatformOpenAI}
		got, err := normalizeOpenAIModelForUpstream(noMapping, "gpt-image-2")
		if err != nil {
			t.Fatalf("unexpected err for no-mapping account: %v", err)
		}
		if got != "gpt-5.4" {
			t.Fatalf("got %q want %q", got, "gpt-5.4")
		}
	})

	t.Run("apikey account is unaffected", func(t *testing.T) {
		apikey := &Account{ID: 300, Type: AccountTypeAPIKey, Platform: PlatformOpenAI}
		got, err := normalizeOpenAIModelForUpstream(apikey, "gpt-image-2")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "gpt-image-2" {
			t.Fatalf("got %q want %q", got, "gpt-image-2")
		}
	})
}

// TestResolveOpenAIUpstreamModelOrFailover_ConvertsToFailoverError
// 作者: mkx  变更: 2026/04/24
// HTTP 路径的封装器：把 UnsupportedUpstreamModelError 翻成
// UpstreamFailoverError{StatusCode:400}，让 handler 的 failover 循环跳过此账号。
func TestResolveOpenAIUpstreamModelOrFailover_ConvertsToFailoverError(t *testing.T) {
	imageOnlyOAuth := &Account{
		ID:       2665,
		Name:     "image-only-free",
		Type:     AccountTypeOAuth,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-image-2": "gpt-image-2",
			},
		},
	}

	got, err := resolveOpenAIUpstreamModelOrFailover(imageOnlyOAuth, "gpt-image-2")
	if err == nil {
		t.Fatalf("expected UpstreamFailoverError, got %q with nil err", got)
	}
	var failover *UpstreamFailoverError
	if !errors.As(err, &failover) {
		t.Fatalf("expected *UpstreamFailoverError, got %T: %v", err, err)
	}
	if failover.StatusCode != 400 {
		t.Fatalf("got StatusCode %d want 400", failover.StatusCode)
	}
	if len(failover.ResponseBody) == 0 {
		t.Fatalf("expected non-empty ResponseBody with diagnostic message")
	}
}
