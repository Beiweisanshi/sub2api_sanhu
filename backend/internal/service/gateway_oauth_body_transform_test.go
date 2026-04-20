package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func scrubTestConfig() *config.Config {
	return &config.Config{
		Telemetry: config.TelemetryConfig{
			PromptEnv: config.TelemetryPromptEnvConfig{
				Platform:   "darwin",
				Shell:      "zsh",
				OSVersion:  "Darwin 24.4.0",
				WorkingDir: "/Users/user/projects",
			},
		},
	}
}

// Realistic fingerprint for a Claude Code CLI 2.1.22 client.
func scrubTestFingerprint() *Fingerprint {
	return &Fingerprint{
		ClientID:  "abc-123",
		UserAgent: "claude-cli/2.1.22 (external, cli)",
	}
}

func TestApplyClaudeCodeBodyRewrites_FullPipeline(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"Platform: linux\nShell: bash\nWorking directory: /home/bob/code"}],
		"messages":[{"role":"user","content":"Hello from /home/bob project"}]
	}`)
	fw := defaultGatewayForwardingSettings()
	fw.CCHSigning = true // verify CCH path too

	out := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)

	// Env block scrubbed.
	envText := gjson.GetBytes(out, "system.#(text%\"Platform:*\").text").String()
	require.NotEmpty(t, envText)
	require.Contains(t, envText, "Platform: darwin")
	require.Contains(t, envText, "Shell: zsh")
	require.Contains(t, envText, "Working directory: /Users/user/projects")

	// Billing block injected at the top with filled CCH (5 hex chars) and real fingerprint suffix.
	billing := gjson.GetBytes(out, "system.0.text").String()
	require.Contains(t, billing, "x-anthropic-billing-header:")
	require.Regexp(t, `cc_version=2\.1\.22\.[0-9a-f]{3};`, billing)
	require.Regexp(t, `cch=[0-9a-f]{5};`, billing)
	require.NotContains(t, billing, "cch=00000")
}

func TestApplyClaudeCodeBodyRewrites_BillingInjectOffLeavesMissing(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"You are Claude."}],"messages":[{"role":"user","content":"hi"}]}`)
	fw := defaultGatewayForwardingSettings()
	fw.BillingInject = false

	out := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)

	assert.False(t, hasBillingHeader(out))
}

func TestApplyClaudeCodeBodyRewrites_NoFingerprintSkipsEverything(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"Platform: linux"}],"messages":[{"role":"user","content":"hi"}]}`)
	fw := defaultGatewayForwardingSettings()

	out := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), nil, fw)

	// Non-OAuth path: body must pass through byte-for-byte — neither prompt
	// scrubbing nor billing injection applies when there's no fingerprint to
	// impersonate (API Key / Bedrock / third-party relay callers).
	assert.JSONEq(t, string(body), string(out))
	assert.False(t, hasBillingHeader(out))
}

func TestApplyClaudeCodeBodyRewrites_V2OffPreservesUpstreamCCVersion(t *testing.T) {
	// Body carries an existing billing header with a specific cc_version suffix.
	// Disabling V2 must NOT randomize or touch it.
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.22.abc; cc_entrypoint=cli; cch=00000;"}],"messages":[{"role":"user","content":"hi"}]}`)
	fw := defaultGatewayForwardingSettings()
	fw.CCFingerprintV2 = false

	a := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)
	b := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)

	// cc_version suffix on the existing billing header is left untouched, and
	// two identical inputs produce identical outputs (no randomness).
	assert.Equal(t, string(a), string(b))
	require.Contains(t, gjson.GetBytes(a, "system.0.text").String(), "cc_version=2.1.22.abc;")
}

func TestApplyClaudeCodeBodyRewrites_V2OffStableInjectedSuffix(t *testing.T) {
	// No existing billing header, V2 off but inject on: suffix must be the
	// stable "000" rather than a random 3-hex value.
	body := []byte(`{"system":[],"messages":[{"role":"user","content":"hi"}]}`)
	fw := defaultGatewayForwardingSettings()
	fw.CCFingerprintV2 = false
	fw.CCHSigning = false

	out := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)
	billing := gjson.GetBytes(out, "system.0.text").String()
	require.Contains(t, billing, "cc_version=2.1.22.000;")
}

func TestApplyClaudeCodeBodyRewrites_AllOffPassesThrough(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"Platform: linux"}],"messages":[{"role":"user","content":"hi"}]}`)
	fw := GatewayForwardingSettings{} // every knob off

	out := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)

	assert.JSONEq(t, string(body), string(out))
}

func TestApplyClaudeCodeBodyRewrites_FingerprintV2DeterministicAcrossCalls(t *testing.T) {
	body := []byte(`{"system":[],"messages":[{"role":"user","content":"consistent first message for fingerprint"}]}`)
	fw := defaultGatewayForwardingSettings()

	a := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)
	b := applyClaudeCodeBodyRewrites(body, scrubTestConfig(), scrubTestFingerprint(), fw)

	// Billing was injected; suffix must be stable across identical inputs.
	billA := gjson.GetBytes(a, "system.0.text").String()
	billB := gjson.GetBytes(b, "system.0.text").String()
	assert.Equal(t, billA, billB)
}

func TestApplyClaudeCodeBodyRewrites_EmptyBody(t *testing.T) {
	got := applyClaudeCodeBodyRewrites(nil, scrubTestConfig(), scrubTestFingerprint(), defaultGatewayForwardingSettings())
	assert.Nil(t, got)
}
