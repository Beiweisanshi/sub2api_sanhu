package service

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// Precompiled regex patterns for prompt sanitization
var (
	// Matches "Platform: <value>" in system prompt <env> blocks
	promptPlatformRegex = regexp.MustCompile(`(?m)Platform:\s*\S+`)
	// Matches "Shell: <value>" in system prompt <env> blocks
	promptShellRegex = regexp.MustCompile(`(?m)Shell:\s*\S+`)
	// Matches "OS Version: <rest-of-line>" (stops at newline or <)
	promptOSVersionRegex = regexp.MustCompile(`(?m)OS Version:\s*[^\n<]+`)
	// Matches "(Primary )?Working directory: /path/..." or "working directory: /path/..."
	promptWorkingDirRegex = regexp.MustCompile(`(?m)((?:Primary )?[Ww]orking directory:\s*)/\S+`)
	// Matches home directory paths like /Users/xxx/ or /home/xxx/
	homePathRegex = regexp.MustCompile(`/(?:Users|home)/[^/\s]+/`)
	// Matches cc_version billing fingerprint: cc_version=X.Y.Z with an optional
	// 3-hex suffix (cc_version=X.Y.Z.abc). Anchors on whole semver triplet so
	// partial suffixes (e.g. stale .000) are replaced cleanly.
	ccVersionRegex = regexp.MustCompile(`cc_version=\d+\.\d+\.\d+(?:\.[0-9a-f]{3})?`)
)

// rewritePromptEnvBlock rewrites the <env> block fields in system prompt text.
// Replaces Platform:, Shell:, OS Version:, and Working directory: lines with
// canonical values from config.
func rewritePromptEnvBlock(text string, promptEnv config.TelemetryPromptEnvConfig) string {
	if promptEnv.Platform != "" {
		text = promptPlatformRegex.ReplaceAllString(text, "Platform: "+promptEnv.Platform)
	}
	if promptEnv.Shell != "" {
		text = promptShellRegex.ReplaceAllString(text, "Shell: "+promptEnv.Shell)
	}
	if promptEnv.OSVersion != "" {
		text = promptOSVersionRegex.ReplaceAllString(text, "OS Version: "+promptEnv.OSVersion)
	}
	if promptEnv.WorkingDir != "" {
		text = promptWorkingDirRegex.ReplaceAllStringFunc(text, func(match string) string {
			// Find the label prefix (e.g., "Primary working directory: ")
			idx := strings.LastIndex(match, "/")
			if idx < 0 {
				return match
			}
			// Extract the label part up to (but not including) the path
			submatch := promptWorkingDirRegex.FindStringSubmatch(match)
			if len(submatch) >= 2 {
				return submatch[1] + promptEnv.WorkingDir
			}
			return match
		})
	}
	return text
}

// scrubHomePaths replaces real home directory paths (e.g., /Users/john/, /home/john/)
// with a canonical home prefix derived from the configured working directory.
func scrubHomePaths(text string, workingDir string) string {
	if workingDir == "" {
		return text
	}

	// Extract canonical home prefix from working dir (first two path segments)
	// e.g., "/home/user/project" -> "/home/user/"
	canonicalHome := extractHomePrefix(workingDir)
	if canonicalHome == "" {
		canonicalHome = "/Users/user/"
	}

	return homePathRegex.ReplaceAllString(text, canonicalHome)
}

// extractHomePrefix extracts the first two path segments from a path.
// e.g., "/home/user/project" -> "/home/user/"
// e.g., "/Users/alice/workspace" -> "/Users/alice/"
func extractHomePrefix(path string) string {
	if !strings.HasPrefix(path, "/") {
		return ""
	}
	parts := strings.SplitN(path, "/", 4) // ["", "Users", "alice", "workspace"]
	if len(parts) < 3 {
		return ""
	}
	return "/" + parts[1] + "/" + parts[2] + "/"
}

// rewriteCCVersion replaces cc_version billing fingerprint strings in prompt
// text with the supplied version and 3-hex fingerprint suffix. When
// fingerprint is empty it falls back to a random 3-hex value so multiple
// rewrites do not collapse into a single across-account fingerprint.
func rewriteCCVersion(text, version, fingerprint string) string {
	suffix := normalizeCCFingerprint(fingerprint)
	return ccVersionRegex.ReplaceAllString(text, "cc_version="+version+"."+suffix)
}

// RewriteBillingHeaderValue rewrites cc_version in x-anthropic-billing-header.
// Empty canonical version means "leave the original value untouched".
// Empty fingerprint causes a random 3-hex suffix to be generated.
func RewriteBillingHeaderValue(value, version, fingerprint string) string {
	version = strings.TrimSpace(version)
	if version == "" || value == "" {
		return value
	}
	return rewriteCCVersion(value, version, fingerprint)
}

// normalizeCCFingerprint returns a valid 3-hex lowercase suffix. If the input
// already matches that shape it is returned untouched; otherwise a random one
// is generated. Keeps output deterministic when the caller passes a real
// fingerprint computed from the request body.
func normalizeCCFingerprint(fp string) string {
	fp = strings.ToLower(strings.TrimSpace(fp))
	if len(fp) == 3 {
		valid := true
		for _, r := range fp {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				valid = false
				break
			}
		}
		if valid {
			return fp
		}
	}
	return randomCCFingerprint()
}

// randomCCFingerprint returns a random 3-hex-char suffix. Used as a fallback
// when no message-derived fingerprint is available (e.g. telemetry forwarding
// paths that have no messages[] array).
func randomCCFingerprint() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "000"
	}
	return hex.EncodeToString(b[:])[:3]
}

// CanonicalClaudeCLIUserAgent returns the canonical Claude CLI user-agent used
// for telemetry forwarding. When version is empty, it falls back to the
// project's default Claude CLI fingerprint.
func CanonicalClaudeCLIUserAgent(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return claude.DefaultHeaders["User-Agent"]
	}
	return "claude-cli/" + version + " (external, cli)"
}
