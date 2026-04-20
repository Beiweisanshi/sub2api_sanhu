package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// systemReminderBlockRegex captures everything inside a <system-reminder> tag
// (non-greedy, multi-line). Mirrors zhima2api/rewriter.ts:460. Injected
// reminder blocks commonly leak real paths / environment fields even though
// the surrounding user text is authored content we must not touch.
var systemReminderBlockRegex = regexp.MustCompile(`(?s)(<system-reminder>)(.*?)(</system-reminder>)`)

// ScrubOptions toggles the individual passes of ScrubClaudeCodePrompt.
type ScrubOptions struct {
	// ScrubEnv rewrites Platform/Shell/OS Version/Working directory lines and
	// home paths inside body.system[] text blocks.
	ScrubEnv bool
	// ScrubSystemReminder re-runs env/path scrubbing inside any
	// <system-reminder>…</system-reminder> segments in messages[] text blocks.
	ScrubSystemReminder bool
}

// ScrubClaudeCodePrompt applies identity-hiding rewrites to the Claude Code
// request body in-place (returns a new buffer). Safe on non-matching bodies.
//
// Controls:
//   - opts.ScrubEnv → sanitize system[] non-billing text blocks.
//   - opts.ScrubSystemReminder → sanitize <system-reminder> blocks in
//     messages[].content[] text entries only.
//
// The billing-header system block is identified by prefix and left to
// gateway_billing_header.go. User-authored content outside <system-reminder>
// tags is never mutated.
func ScrubClaudeCodePrompt(body []byte, cfg *config.Config, opts ScrubOptions) []byte {
	if cfg == nil || len(body) == 0 {
		return body
	}

	pe := cfg.Telemetry.PromptEnv
	if !hasPromptEnvValues(pe) {
		return body
	}

	if opts.ScrubEnv {
		body = scrubSystemBlocks(body, pe)
	}
	if opts.ScrubSystemReminder {
		body = scrubMessageReminders(body, pe)
	}
	return body
}

// hasPromptEnvValues returns true when at least one canonical replacement is
// configured — otherwise scrubbing would be a no-op that still pays the cost
// of walking the JSON tree.
func hasPromptEnvValues(pe config.TelemetryPromptEnvConfig) bool {
	return pe.Platform != "" || pe.Shell != "" || pe.OSVersion != "" || pe.WorkingDir != ""
}

func scrubSystemBlocks(body []byte, pe config.TelemetryPromptEnvConfig) []byte {
	system := gjson.GetBytes(body, "system")
	if !system.Exists() {
		return body
	}

	if system.Type == gjson.String {
		original := system.String()
		if isBillingHeaderText(original) {
			return body
		}
		updated := applyPromptScrubs(original, pe)
		if updated != original {
			if next, err := sjson.SetBytes(body, "system", updated); err == nil {
				body = next
			}
		}
		return body
	}

	if !system.IsArray() {
		return body
	}

	idx := 0
	system.ForEach(func(_, item gjson.Result) bool {
		path := fmt.Sprintf("system.%d", idx)
		idx++

		var text string
		var isObject bool
		switch {
		case item.Type == gjson.String:
			text = item.String()
		case item.Type == gjson.JSON && item.IsObject():
			t := item.Get("text")
			if t.Exists() && t.Type == gjson.String {
				text = t.String()
				isObject = true
			}
		}

		if text == "" || isBillingHeaderText(text) {
			return true
		}

		updated := applyPromptScrubs(text, pe)
		if updated == text {
			return true
		}

		target := path
		if isObject {
			target = path + ".text"
		}
		if next, err := sjson.SetBytes(body, target, updated); err == nil {
			body = next
		}
		return true
	})

	return body
}

func scrubMessageReminders(body []byte, pe config.TelemetryPromptEnvConfig) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}

	mIdx := 0
	messages.ForEach(func(_, msg gjson.Result) bool {
		mi := mIdx
		mIdx++

		content := msg.Get("content")
		if !content.Exists() {
			return true
		}

		// content: plain string
		if content.Type == gjson.String {
			original := content.String()
			updated := rewriteSystemReminderSegments(original, pe)
			if updated != original {
				if next, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", mi), updated); err == nil {
					body = next
				}
			}
			return true
		}

		if !content.IsArray() {
			return true
		}

		cIdx := 0
		content.ForEach(func(_, block gjson.Result) bool {
			ci := cIdx
			cIdx++

			switch {
			case block.Type == gjson.String:
				original := block.String()
				updated := rewriteSystemReminderSegments(original, pe)
				if updated != original {
					if next, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content.%d", mi, ci), updated); err == nil {
						body = next
					}
				}
			case block.Type == gjson.JSON && block.IsObject():
				t := block.Get("text")
				if !t.Exists() || t.Type != gjson.String {
					return true
				}
				original := t.String()
				updated := rewriteSystemReminderSegments(original, pe)
				if updated != original {
					if next, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content.%d.text", mi, ci), updated); err == nil {
						body = next
					}
				}
			}
			return true
		})
		return true
	})

	return body
}

// rewriteSystemReminderSegments rewrites only the contents of
// <system-reminder>…</system-reminder> spans. The surrounding text — which is
// authored by the user — is left byte-for-byte identical.
func rewriteSystemReminderSegments(text string, pe config.TelemetryPromptEnvConfig) string {
	if !strings.Contains(text, "<system-reminder>") {
		return text
	}
	return systemReminderBlockRegex.ReplaceAllStringFunc(text, func(match string) string {
		sub := systemReminderBlockRegex.FindStringSubmatch(match)
		if len(sub) != 4 {
			return match
		}
		return sub[1] + applyPromptScrubs(sub[2], pe) + sub[3]
	})
}

func applyPromptScrubs(text string, pe config.TelemetryPromptEnvConfig) string {
	text = rewritePromptEnvBlock(text, pe)
	text = scrubHomePaths(text, pe.WorkingDir)
	return text
}

// isBillingHeaderText recognizes the x-anthropic-billing-header text block.
// The billing block has its own pipeline (signing + version sync) and must
// not be touched by env scrubbing.
func isBillingHeaderText(text string) bool {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	return strings.HasPrefix(trimmed, "x-anthropic-billing-header")
}
