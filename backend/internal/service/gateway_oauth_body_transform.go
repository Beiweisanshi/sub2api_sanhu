package service

import (
	"strings"

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
// applyClaudeCodeBodyRewrites 参数说明：
//   - effectiveCLIVersion 是本次请求最终发往上游的 claude-cli 版本号（例如
//     "2.1.112"）。调用方必须传入与最终 User-Agent header 一致的值，否则
//     billing header 注入的 cc_version 会和实际 UA 漂移。空串代表无法推断
//     UA 版本（如 api-key 透传），此时跳过 billing 注入/同步，但 prompt
//     清洗仍会运行。
//
// 注意：本函数不再依赖 OAuth fingerprint —— env/system-reminder 清洗、
// billing 注入、CCH 签名属于通用 body 级硬加固，独立于指纹伪装开关。
// 是否需要指纹伪装由调用方在传入 effectiveCLIVersion 之前自行判断。
func applyClaudeCodeBodyRewrites(
	body []byte,
	cfg *config.Config,
	effectiveCLIVersion string,
	fw GatewayForwardingSettings,
) []byte {
	if len(body) == 0 {
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
	// rolling a random suffix each forward. 版本号必须用最终发出的 UA 版本，
	// 而不是 fingerprint.UserAgent：mimic 路径会用 claudeClientHeaders() 覆写
	// 请求头，若此处仍取 fingerprint.UA 会导致 billing 与实际 UA 不一致。
	version := strings.TrimSpace(effectiveCLIVersion)
	// syncBillingHeaderVersion 内部也会 ExtractCLIVersion(userAgent)，构造一个
	// 最小可用的 UA 字符串即可触发替换。
	userAgent := ""
	if version != "" {
		userAgent = "claude-cli/" + version
	}
	var ccFP string
	if fw.CCFingerprintV2 && version != "" {
		ccFP = ComputeCCVersionFingerprint(body, version)
		body = syncBillingHeaderVersion(body, userAgent, ccFP)
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
