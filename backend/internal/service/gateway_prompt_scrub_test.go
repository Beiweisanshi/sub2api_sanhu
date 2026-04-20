package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func buildScrubConfig() *config.Config {
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

func TestScrubClaudeCodePrompt_EnvBlockReplaced(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"Platform: linux\nShell: bash\nOS Version: Linux 6.5.0-generic\nWorking directory: /home/bob/code"}]}`)
	got := ScrubClaudeCodePrompt(body, buildScrubConfig(), ScrubOptions{ScrubEnv: true})

	text := gjson.GetBytes(got, "system.0.text").String()
	require.Contains(t, text, "Platform: darwin")
	require.Contains(t, text, "Shell: zsh")
	require.Contains(t, text, "OS Version: Darwin 24.4.0")
	require.Contains(t, text, "Working directory: /Users/user/projects")
	require.NotContains(t, text, "/home/bob/")
}

func TestScrubClaudeCodePrompt_HomePathsInNarrativeReplaced(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"See /Users/alice/secrets.txt and /home/bob/notes for context."}]}`)
	got := ScrubClaudeCodePrompt(body, buildScrubConfig(), ScrubOptions{ScrubEnv: true})

	text := gjson.GetBytes(got, "system.0.text").String()
	require.NotContains(t, text, "/Users/alice/")
	require.NotContains(t, text, "/home/bob/")
	require.Contains(t, text, "/Users/user/")
}

func TestScrubClaudeCodePrompt_BillingBlockUntouched(t *testing.T) {
	billing := "x-anthropic-billing-header: cc_version=2.1.81.abc; cc_entrypoint=cli; cch=00000;"
	body := []byte(`{"system":[{"type":"text","text":"` + billing + `"},{"type":"text","text":"Platform: linux"}]}`)
	got := ScrubClaudeCodePrompt(body, buildScrubConfig(), ScrubOptions{ScrubEnv: true})

	require.Equal(t, billing, gjson.GetBytes(got, "system.0.text").String(), "billing block must be left intact")
	require.Contains(t, gjson.GetBytes(got, "system.1.text").String(), "Platform: darwin")
}

func TestScrubClaudeCodePrompt_SystemReminderBlockScrubbedNotUserText(t *testing.T) {
	cfg := buildScrubConfig()
	userText := "Please explain /home/bob/script.sh "
	reminder := "<system-reminder>\nWorking directory: /home/bob/code\nPlatform: linux\n</system-reminder>"
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"` + userText + reminder + `"}]}]}`)

	got := ScrubClaudeCodePrompt(body, cfg, ScrubOptions{ScrubSystemReminder: true})
	text := gjson.GetBytes(got, "messages.0.content.0.text").String()

	// User-authored path is untouched.
	require.Contains(t, text, "/home/bob/script.sh")
	// Reminder content is scrubbed.
	require.Contains(t, text, "<system-reminder>")
	require.Contains(t, text, "Working directory: /Users/user/projects")
	require.Contains(t, text, "Platform: darwin")
	require.NotContains(t, text, "/home/bob/code")
}

func TestScrubClaudeCodePrompt_BothOptsCombined(t *testing.T) {
	cfg := buildScrubConfig()
	body := []byte(`{"system":[{"type":"text","text":"Platform: linux"}],"messages":[{"role":"user","content":"<system-reminder>Shell: bash</system-reminder> plain"}]}`)
	got := ScrubClaudeCodePrompt(body, cfg, ScrubOptions{ScrubEnv: true, ScrubSystemReminder: true})

	require.Contains(t, gjson.GetBytes(got, "system.0.text").String(), "Platform: darwin")
	require.Contains(t, gjson.GetBytes(got, "messages.0.content").String(), "Shell: zsh")
}

func TestScrubClaudeCodePrompt_StringSystemField(t *testing.T) {
	body := []byte(`{"system":"Platform: linux\nWorking directory: /home/bob"}`)
	got := ScrubClaudeCodePrompt(body, buildScrubConfig(), ScrubOptions{ScrubEnv: true})

	require.Contains(t, gjson.GetBytes(got, "system").String(), "Platform: darwin")
	require.NotContains(t, gjson.GetBytes(got, "system").String(), "/home/bob")
}

func TestScrubClaudeCodePrompt_StringSystemField_BillingUntouched(t *testing.T) {
	billing := "x-anthropic-billing-header: cc_version=2.1.81.abc; cch=00000;"
	body := []byte(`{"system":"` + billing + `"}`)
	got := ScrubClaudeCodePrompt(body, buildScrubConfig(), ScrubOptions{ScrubEnv: true})
	require.Equal(t, billing, gjson.GetBytes(got, "system").String())
}

func TestScrubClaudeCodePrompt_EmptyConfigShortCircuits(t *testing.T) {
	cfg := &config.Config{Telemetry: config.TelemetryConfig{PromptEnv: config.TelemetryPromptEnvConfig{}}}
	body := []byte(`{"system":[{"type":"text","text":"Platform: linux"}]}`)
	got := ScrubClaudeCodePrompt(body, cfg, ScrubOptions{ScrubEnv: true, ScrubSystemReminder: true})
	require.Equal(t, string(body), string(got))
}

func TestScrubClaudeCodePrompt_NoOptsNoop(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"Platform: linux"}],"messages":[{"role":"user","content":"<system-reminder>Shell: bash</system-reminder>"}]}`)
	got := ScrubClaudeCodePrompt(body, buildScrubConfig(), ScrubOptions{})
	require.Equal(t, string(body), string(got))
}

func TestScrubClaudeCodePrompt_MessagesStringContentReminder(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"keep me <system-reminder>Shell: bash</system-reminder> intact"}]}`)
	got := ScrubClaudeCodePrompt(body, buildScrubConfig(), ScrubOptions{ScrubSystemReminder: true})
	val := gjson.GetBytes(got, "messages.0.content").String()
	require.Contains(t, val, "Shell: zsh")
	require.Contains(t, val, "keep me ")
	require.Contains(t, val, " intact")
}
