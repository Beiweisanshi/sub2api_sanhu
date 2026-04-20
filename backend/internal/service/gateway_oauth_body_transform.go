package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
)

// applyClaudeCodeBodyRewrites runs the Claude Code body-level identity hardening
// pipeline against a /v1/messages (or /v1/messages/count_tokens) payload.
//
// Order (mirrors zhima2api cc-gateway rewriter.ts):
//  1. Scrub `<env>`/home paths inside body.system[] text blocks.
//  2. Scrub `<system-reminder>…</system-reminder>` segments in messages[].
//  3. Compute the 3-hex cc_version fingerprint from the first user message.
//  4. Sync cc_version in any existing x-anthropic-billing-header block.
//  5. Inject a canonical x-anthropic-billing-header block when missing.
//  6. Replace cch=00000 with xxHash64(body, CCH_SEED)&0xFFFFF (signing pass).
//
// Each step is individually gated by settings, so every mutation is observable
// and reversible via the admin UI. Callers should invoke this after
// metadata.user_id rewriting so the fingerprint hashes the post-rewrite body.
func applyClaudeCodeBodyRewrites(
	body []byte,
	cfg *config.Config,
	fingerprint *Fingerprint,
	fw GatewayForwardingSettings,
) []byte {
	if len(body) == 0 {
		return body
	}

	// The entire pipeline is Claude Code identity spoofing — only meaningful
	// when we have an OAuth fingerprint to impersonate. On API Key / Bedrock /
	// third-party-relay paths fingerprint is nil: skip scrubbing too, otherwise
	// we silently rewrite user prompts containing `<system-reminder>` or
	// Platform:/Shell:/Working directory: lines.
	if fingerprint == nil {
		return body
	}

	// Steps 1-2: prompt scrubbing.
	if fw.EnvScrub || fw.SystemReminderScrub {
		body = ScrubClaudeCodePrompt(body, cfg, ScrubOptions{
			ScrubEnv:            fw.EnvScrub,
			ScrubSystemReminder: fw.SystemReminderScrub,
		})
	}

	// Step 3-4: compute fingerprint and sync billing cc_version. Only run when
	// V2 is enabled — otherwise leave upstream cc_version untouched instead of
	// rolling a random suffix each forward.
	version := ExtractCLIVersion(fingerprint.UserAgent)
	var ccFP string
	if fw.CCFingerprintV2 && version != "" {
		ccFP = ComputeCCVersionFingerprint(body, version)
		body = syncBillingHeaderVersion(body, fingerprint.UserAgent, ccFP)
	}

	// Step 5: inject missing billing header. Requires a known version — skip
	// silently when we can't derive one. When V2 is off we pass an empty
	// fingerprint and let injectBillingHeaderIfMissing fall back to a stable
	// zero suffix instead of randomizing.
	if fw.BillingInject && version != "" {
		body = injectBillingHeaderIfMissing(body, version, ccFP)
	}

	// Step 6: CCH signing (only run when both the admin opt-in is on and we
	// have a billing block to sign — injectBillingHeaderIfMissing guarantees
	// one exists when BillingInject is on).
	if fw.CCHSigning {
		body = signBillingHeaderCCH(body)
	}

	return body
}
