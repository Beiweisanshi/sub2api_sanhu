package service

import (
	"fmt"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSyncBillingHeaderVersion(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		userAgent   string
		fingerprint string
		wantSub     string // substring expected in result
		unchanged   bool   // expect body to remain the same
	}{
		{
			name:        "rewrites cc_version and suffix together",
			body:        `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.df2; cc_entrypoint=cli; cch=00000;"},{"type":"text","text":"You are Claude Code.","cache_control":{"type":"ephemeral"}}],"messages":[]}`,
			userAgent:   "claude-cli/2.1.22 (external, cli)",
			fingerprint: "a1b",
			wantSub:     "cc_version=2.1.22.a1b",
		},
		{
			name:        "adds suffix when original has none",
			body:        `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent:   "claude-cli/2.1.22",
			fingerprint: "c0d",
			wantSub:     "cc_version=2.1.22.c0d",
		},
		{
			name:      "no billing header in system",
			body:      `{"system":[{"type":"text","text":"You are Claude Code."}],"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
		{
			name:      "no system field",
			body:      `{"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
		{
			name:      "user-agent without version",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "Mozilla/5.0",
			unchanged: true,
		},
		{
			name:      "empty user-agent",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "",
			unchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncBillingHeaderVersion([]byte(tt.body), tt.userAgent, tt.fingerprint)
			if tt.unchanged {
				assert.Equal(t, tt.body, string(result), "body should remain unchanged")
			} else {
				assert.Contains(t, string(result), tt.wantSub)
				// Ensure old semver is gone
				assert.NotContains(t, string(result), "cc_version=2.1.81")
			}
		})
	}
}

func TestSyncBillingHeaderVersion_EmptyFingerprintRandomSuffix(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
	result := syncBillingHeaderVersion(body, "claude-cli/2.1.22", "")
	billing := gjson.GetBytes(result, "system.0.text").String()
	require.Regexp(t, `cc_version=2\.1\.22\.[0-9a-f]{3};`, billing)
}

func TestInjectBillingHeaderIfMissing(t *testing.T) {
	t.Run("inject prepends block when system is array without billing", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"You are Claude Code."}],"messages":[]}`)
		out := injectBillingHeaderIfMissing(body, "2.1.22", "a1b")

		first := gjson.GetBytes(out, "system.0.text").String()
		require.Contains(t, first, "x-anthropic-billing-header:")
		require.Contains(t, first, "cc_version=2.1.22.a1b")
		require.Contains(t, first, "cch=00000")
		// Original block preserved at index 1
		second := gjson.GetBytes(out, "system.1.text").String()
		require.Equal(t, "You are Claude Code.", second)
	})

	t.Run("no-op when billing header already present", func(t *testing.T) {
		original := `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.22.abc; cch=00000;"}],"messages":[]}`
		out := injectBillingHeaderIfMissing([]byte(original), "2.1.22", "abc")
		require.Equal(t, original, string(out))
	})

	t.Run("string system: wrapped into array with billing first", func(t *testing.T) {
		body := []byte(`{"system":"You are Claude Code.","messages":[]}`)
		out := injectBillingHeaderIfMissing(body, "2.1.22", "a1b")

		first := gjson.GetBytes(out, "system.0.text").String()
		require.Contains(t, first, "x-anthropic-billing-header:")
		second := gjson.GetBytes(out, "system.1.text").String()
		require.Equal(t, "You are Claude Code.", second)
	})

	t.Run("missing system field: creates new array", func(t *testing.T) {
		body := []byte(`{"messages":[]}`)
		out := injectBillingHeaderIfMissing(body, "2.1.22", "a1b")
		require.Contains(t, gjson.GetBytes(out, "system.0.text").String(), "x-anthropic-billing-header:")
	})

	t.Run("empty version: no-op", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"hi"}]}`)
		out := injectBillingHeaderIfMissing(body, "", "a1b")
		require.Equal(t, string(body), string(out))
	})

	t.Run("empty fingerprint produces stable 000 suffix", func(t *testing.T) {
		body := []byte(`{"messages":[]}`)
		out := injectBillingHeaderIfMissing(body, "2.1.22", "")
		text := gjson.GetBytes(out, "system.0.text").String()
		// Deterministic: same input ⇒ same output, no randomness. Caller is
		// expected to pass "" exactly when CCFingerprintV2 is off.
		require.Contains(t, text, "cc_version=2.1.22.000;")
	})
}

func TestHasBillingHeader(t *testing.T) {
	cases := map[string]struct {
		body string
		want bool
	}{
		"array with billing":   {`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.22.abc;"}]}`, true},
		"array without":        {`{"system":[{"type":"text","text":"You are Claude."}]}`, false},
		"string billing":       {`{"system":"x-anthropic-billing-header: cc_version=2.1.22.abc;"}`, true},
		"string plain":         {`{"system":"hello"}`, false},
		"array of raw strings": {`{"system":["x-anthropic-billing-header: cc_version=2.1.22.abc;","tail"]}`, true},
		"missing system":       {`{}`, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasBillingHeader([]byte(tc.body)))
		})
	}
}

func TestSignBillingHeaderCCH(t *testing.T) {
	t.Run("replaces placeholder with hash", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63.a43; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
		result := signBillingHeaderCCH(body)

		// Should not have the placeholder anymore
		assert.NotContains(t, string(result), "cch=00000")

		// Should have a 5 hex-char cch value
		billingText := gjson.GetBytes(result, "system.0.text").String()
		require.Contains(t, billingText, "cch=")
		assert.Regexp(t, `cch=[0-9a-f]{5};`, billingText)
	})

	t.Run("no placeholder - body unchanged", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=abcde;"}],"messages":[]}`)
		result := signBillingHeaderCCH(body)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("no billing header - body unchanged", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"You are Claude Code."}],"messages":[]}`)
		result := signBillingHeaderCCH(body)
		assert.Equal(t, string(body), string(result))
	})

	t.Run("cch=00000 in user content is not touched", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"keep literal cch=00000 in this message"}]}]}`)
		result := signBillingHeaderCCH(body)

		// Billing header should be signed
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.NotContains(t, billingText, "cch=00000")

		// User message should keep its literal cch=00000
		userText := gjson.GetBytes(result, "messages.0.content.0.text").String()
		assert.Contains(t, userText, "cch=00000")
	})

	t.Run("signing is deterministic", func(t *testing.T) {
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":"hi"}]}`)
		r1 := signBillingHeaderCCH(body)
		body2 := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":"hi"}]}`)
		r2 := signBillingHeaderCCH(body2)
		assert.Equal(t, string(r1), string(r2))
	})

	t.Run("matches reference algorithm", func(t *testing.T) {
		// Verify: signBillingHeaderCCH(body) produces cch = xxHash64(body_with_placeholder, seed) & 0xFFFFF
		body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63.a43; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
		expectedCCH := fmt.Sprintf("%05x", xxHash64Seeded(body, cchSeed)&0xFFFFF)

		result := signBillingHeaderCCH(body)
		billingText := gjson.GetBytes(result, "system.0.text").String()
		assert.Contains(t, billingText, "cch="+expectedCCH+";")
	})
}

func TestXXHash64Seeded(t *testing.T) {
	t.Run("matches cespare/xxhash for seed 0", func(t *testing.T) {
		inputs := []string{"", "a", "hello world", "The quick brown fox jumps over the lazy dog"}
		for _, s := range inputs {
			data := []byte(s)
			expected := xxhash.Sum64(data)
			got := xxHash64Seeded(data, 0)
			assert.Equal(t, expected, got, "mismatch for input %q", s)
		}
	})

	t.Run("large input matches cespare", func(t *testing.T) {
		data := make([]byte, 256)
		for i := range data {
			data[i] = byte(i)
		}
		expected := xxhash.Sum64(data)
		got := xxHash64Seeded(data, 0)
		assert.Equal(t, expected, got)
	})

	t.Run("deterministic with custom seed", func(t *testing.T) {
		data := []byte("hello world")
		h1 := xxHash64Seeded(data, cchSeed)
		h2 := xxHash64Seeded(data, cchSeed)
		assert.Equal(t, h1, h2)
	})

	t.Run("different seeds produce different results", func(t *testing.T) {
		data := []byte("test data for hashing")
		h1 := xxHash64Seeded(data, 0)
		h2 := xxHash64Seeded(data, cchSeed)
		assert.NotEqual(t, h1, h2)
	})
}
