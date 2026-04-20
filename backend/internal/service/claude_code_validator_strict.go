package service

import (
	"regexp"
	"strings"
)

// StrictValidationError describes a structural violation of the Claude Code
// /v1/messages payload that the gateway refuses to forward. The caller is
// expected to return it as a 400 invalid_request_error, matching upstream
// Anthropic behavior — better to fail fast than let the upstream reject the
// request and counting it against the account's error budget.
type StrictValidationError struct {
	ErrorType string // e.g. "invalid_request_error"
	Detail    string
}

var (
	// thinkingSignaturePattern: base64-url / base64 charset. Real upstream
	// signatures are opaque blobs ≥ 32 chars; anything shorter or with
	// non-base64 chars is a client that mutated the block (most commonly
	// a buggy relay stripping whitespace / re-encoding).
	thinkingSignaturePattern = regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)

	// claudeVersionModelPattern extracts major/minor from model ids like
	// "claude-sonnet-4-6-20250514" or "claude-opus-4-6".
	claudeVersionModelPattern = regexp.MustCompile(`(?i)^claude-[^-]+-(\d+)-(\d+)(?:-|$)`)
)

// ModelDisallowsAssistantPrefill reports whether `model` is a Claude 4.6+ or
// "mythos"-family release that refuses to run when messages[] ends with an
// assistant-role message (i.e. the caller is trying to prefill a response).
// Matches zhima2api cc-gateway rewriter.ts modelDisallowsAssistantPrefill.
func ModelDisallowsAssistantPrefill(model string) bool {
	if model == "" {
		return false
	}
	if strings.Contains(strings.ToLower(model), "mythos") {
		return true
	}
	m := claudeVersionModelPattern.FindStringSubmatch(model)
	if len(m) != 3 {
		return false
	}
	major := atoiSafe(m[1])
	minor := atoiSafe(m[2])
	if major > 4 {
		return true
	}
	return major == 4 && minor >= 6
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// ValidateMessagesStrict performs structural validation that is too strict
// for the soft (context-flag) Validate() path. It returns non-nil when the
// gateway should reject the request with 400 before any upstream contact:
//
//   - thinking blocks must live on assistant messages, carry a string
//     `thinking`, and carry a base64-shaped `signature` ≥ 32 chars.
//     (Upstream returns 400 for these cases anyway, but failing locally
//     avoids eating one error-budget ping per failure.)
//   - redacted_thinking blocks must live on assistant messages and carry
//     a non-empty string `data`.
//   - Claude 4.6+ / mythos refuse messages[] ending in role=assistant
//     (assistant prefill).
//
// The parsed map is expected to be the pre-decoded JSON body (same shape
// the existing Validate() accepts). Nil / non-object messages bail quietly
// so callers don't have to pre-check — this is a defense-in-depth layer.
func ValidateMessagesStrict(body map[string]any) *StrictValidationError {
	if body == nil {
		return nil
	}
	rawMessages, ok := body["messages"].([]any)
	if !ok {
		return nil
	}

	// Claude 4.6+ prefill rule — model comes from the top-level body.
	if len(rawMessages) > 0 {
		if model, _ := body["model"].(string); ModelDisallowsAssistantPrefill(model) {
			if last, _ := rawMessages[len(rawMessages)-1].(map[string]any); last != nil {
				if role, _ := last["role"].(string); role == "assistant" {
					return &StrictValidationError{
						ErrorType: "invalid_request_error",
						Detail:    "This model does not support assistant message prefill. The conversation must end with a user message.",
					}
				}
			}
		}
	}

	// Per-block thinking/redacted_thinking structural checks.
	for i, rawMsg := range rawMessages {
		msg, _ := rawMsg.(map[string]any)
		if msg == nil {
			continue
		}
		content, _ := msg["content"].([]any)
		if len(content) == 0 {
			continue
		}
		role, _ := msg["role"].(string)

		for j, rawBlock := range content {
			block, _ := rawBlock.(map[string]any)
			if block == nil {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "thinking":
				if err := validateThinkingBlock(block, role, i, j); err != nil {
					return err
				}
			case "redacted_thinking":
				if err := validateRedactedThinkingBlock(block, role, i, j); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateThinkingBlock(block map[string]any, role string, i, j int) *StrictValidationError {
	path := messagePath(i, j)
	if role != "assistant" {
		return &StrictValidationError{"invalid_request_error", path + ": thinking block is only valid in assistant messages"}
	}
	thinking, ok := block["thinking"].(string)
	_ = thinking // existence is what matters; empty string is still accepted
	if !ok {
		return &StrictValidationError{"invalid_request_error", path + ": thinking block must include string field \"thinking\""}
	}
	sig, ok := block["signature"].(string)
	if !ok || len(sig) < 32 {
		return &StrictValidationError{"invalid_request_error", path + ": invalid thinking.signature, expected original opaque signature from Claude"}
	}
	if !thinkingSignaturePattern.MatchString(sig) {
		return &StrictValidationError{"invalid_request_error", path + ": invalid thinking.signature format, thinking blocks must be passed back unmodified"}
	}
	return nil
}

func validateRedactedThinkingBlock(block map[string]any, role string, i, j int) *StrictValidationError {
	path := messagePath(i, j)
	if role != "assistant" {
		return &StrictValidationError{"invalid_request_error", path + ": redacted_thinking block is only valid in assistant messages"}
	}
	data, ok := block["data"].(string)
	if !ok || data == "" {
		return &StrictValidationError{"invalid_request_error", path + ": redacted_thinking block must include non-empty string field \"data\""}
	}
	return nil
}

func messagePath(i, j int) string {
	// Fixed-size scratch buffer — faster than fmt.Sprintf in hot path.
	return "messages." + itoaSmall(i) + ".content." + itoaSmall(j)
}

// itoaSmall is a tiny non-negative int → string. Request bodies rarely have
// more than a few hundred blocks, so this stays within 3 digits and avoids a
// strconv import in the hot path.
func itoaSmall(n int) string {
	if n < 0 {
		n = 0
	}
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
