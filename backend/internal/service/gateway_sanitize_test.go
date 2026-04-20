package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

func TestSanitizeOpenCodeText_RewritesCanonicalSentence(t *testing.T) {
	in := "You are OpenCode, the best coding agent on the planet."
	got := sanitizeSystemText(in)
	require.Equal(t, strings.TrimSpace(claudeCodeSystemPrompt), got)
}

func TestRewritePromptEnvBlock(t *testing.T) {
	promptEnv := config.TelemetryPromptEnvConfig{
		Platform:   "linux",
		Shell:      "/bin/bash",
		OSVersion:  "Linux 6.1.0",
		WorkingDir: "/home/user/project",
	}

	input := `Some text before
Platform: darwin
Shell: /bin/zsh
OS Version: Darwin 25.3.0
Primary working directory: /Users/kaixin/workspace/sub2api
More text after`

	got := rewritePromptEnvBlock(input, promptEnv)

	require.Contains(t, got, "Platform: linux")
	require.Contains(t, got, "Shell: /bin/bash")
	require.Contains(t, got, "OS Version: Linux 6.1.0")
	require.Contains(t, got, "Primary working directory: /home/user/project")
	require.NotContains(t, got, "darwin")
	require.NotContains(t, got, "/bin/zsh")
	require.NotContains(t, got, "Darwin 25.3.0")
}

func TestScrubHomePaths(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		workingDir string
		wantNot    string
		want       string
	}{
		{
			name:       "replaces /Users/xxx/",
			input:      "File at /Users/kaixin/workspace/project/main.go",
			workingDir: "/home/user/project",
			wantNot:    "/Users/kaixin/",
			want:       "/home/user/",
		},
		{
			name:       "replaces /home/xxx/",
			input:      "File at /home/john/code/main.go",
			workingDir: "/home/user/project",
			wantNot:    "/home/john/",
			want:       "/home/user/",
		},
		{
			name:       "no match no change",
			input:      "No home paths here",
			workingDir: "/home/user/project",
			wantNot:    "",
			want:       "No home paths here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scrubHomePaths(tt.input, tt.workingDir)
			if tt.wantNot != "" {
				require.NotContains(t, got, tt.wantNot)
			}
			require.Contains(t, got, tt.want)
		})
	}
}

func TestRewriteCCVersion(t *testing.T) {
	input := "some text cc_version=2.1.22.abc and cc_version=1.0.5.def more"
	got := rewriteCCVersion(input, "2.1.22", "f0e")
	require.Equal(t, "some text cc_version=2.1.22.f0e and cc_version=2.1.22.f0e more", got)
}

func TestRewriteCCVersion_EmptyFingerprintRandom(t *testing.T) {
	input := "cc_version=2.1.22.abc"
	got := rewriteCCVersion(input, "2.1.99", "")
	require.Regexp(t, `^cc_version=2\.1\.99\.[0-9a-f]{3}$`, got)
}

func TestRewriteBillingHeaderValue(t *testing.T) {
	input := "cc_version=2.1.22.abc; cc_entrypoint=cli"
	got := RewriteBillingHeaderValue(input, "2.1.99", "a7d")
	require.Equal(t, "cc_version=2.1.99.a7d; cc_entrypoint=cli", got)
	// Empty version returns value unchanged.
	require.Equal(t, input, RewriteBillingHeaderValue(input, "", "a7d"))
}

func TestCanonicalClaudeCLIUserAgent(t *testing.T) {
	require.Equal(t, "claude-cli/2.1.99 (external, cli)", CanonicalClaudeCLIUserAgent("2.1.99"))
	require.Equal(t, "claude-cli/"+claude.DefaultCLIVersion+" (external, cli)", CanonicalClaudeCLIUserAgent(""))
}

func TestExtractHomePrefix(t *testing.T) {
	require.Equal(t, "/Users/alice/", extractHomePrefix("/Users/alice/workspace"))
	require.Equal(t, "/home/user/", extractHomePrefix("/home/user/project"))
	require.Equal(t, "", extractHomePrefix("relative/path"))
	require.Equal(t, "", extractHomePrefix("/single"))
}
