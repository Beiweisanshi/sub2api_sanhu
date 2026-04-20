package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ccVersionInBillingRe matches the full cc_version token inside billing
// headers, including any existing 3-hex suffix (e.g. cc_version=2.1.81.a1b).
// The suffix is rewritten together with the semver, not preserved — the
// fingerprint is re-derived from the request body per forward.
var ccVersionInBillingRe = regexp.MustCompile(`cc_version=\d+\.\d+\.\d+(?:\.[0-9a-f]{3})?`)

// cchPlaceholderRe matches the cch=00000 placeholder in billing header text,
// scoped to x-anthropic-billing-header to avoid touching user content.
var cchPlaceholderRe = regexp.MustCompile(`(x-anthropic-billing-header:[^"]*?\bcch=)(00000)(;)`)

const cchSeed uint64 = 0x6E52736AC806831E

// syncBillingHeaderVersion rewrites cc_version in x-anthropic-billing-header
// system text blocks to match the version extracted from userAgent and the
// supplied 3-hex fingerprint. Empty fingerprint produces a random suffix so
// legacy callers retain defined behavior.
func syncBillingHeaderVersion(body []byte, userAgent, fingerprint string) []byte {
	version := ExtractCLIVersion(userAgent)
	if version == "" {
		return body
	}

	systemResult := gjson.GetBytes(body, "system")
	if !systemResult.Exists() || !systemResult.IsArray() {
		return body
	}

	suffix := normalizeCCFingerprint(fingerprint)
	replacement := "cc_version=" + version + "." + suffix
	idx := 0
	systemResult.ForEach(func(_, item gjson.Result) bool {
		text := item.Get("text")
		if text.Exists() && text.Type == gjson.String &&
			strings.HasPrefix(text.String(), "x-anthropic-billing-header") {
			newText := ccVersionInBillingRe.ReplaceAllString(text.String(), replacement)
			if newText != text.String() {
				if updated, err := sjson.SetBytes(body, fmt.Sprintf("system.%d.text", idx), newText); err == nil {
					body = updated
				}
			}
		}
		idx++
		return true
	})

	return body
}

// hasBillingHeader returns true when body.system contains at least one text
// block whose (trimmed) prefix is x-anthropic-billing-header. Handles both
// array-of-blocks and plain-string forms of body.system.
func hasBillingHeader(body []byte) bool {
	system := gjson.GetBytes(body, "system")
	if !system.Exists() {
		return false
	}
	if system.Type == gjson.String {
		return strings.HasPrefix(strings.TrimLeft(system.String(), " \t\r\n"), "x-anthropic-billing-header")
	}
	if !system.IsArray() {
		return false
	}
	found := false
	system.ForEach(func(_, item gjson.Result) bool {
		text := item.Get("text")
		if !text.Exists() || text.Type != gjson.String {
			if item.Type == gjson.String {
				text = item
			} else {
				return true
			}
		}
		if strings.HasPrefix(strings.TrimLeft(text.String(), " \t\r\n"), "x-anthropic-billing-header") {
			found = true
			return false
		}
		return true
	})
	return found
}

// injectBillingHeaderIfMissing prepends a canonical x-anthropic-billing-header
// text block to body.system when none is present. The injected block has
// cch=00000 as a placeholder so signBillingHeaderCCH fills it in afterwards,
// matching the regular rewrite pipeline. Safe to call multiple times — no-op
// once any billing block exists. Returns body unchanged when version is empty.
//
// Empty fingerprint is treated as "stable zero" ("000") rather than random:
// the caller passes "" precisely when CCFingerprintV2 is off and deterministic
// output is desired.
func injectBillingHeaderIfMissing(body []byte, version, fingerprint string) []byte {
	if version == "" || len(body) == 0 {
		return body
	}
	if hasBillingHeader(body) {
		return body
	}
	suffix := fingerprint
	if suffix == "" {
		suffix = "000"
	} else {
		suffix = normalizeCCFingerprint(fingerprint)
	}
	text := "x-anthropic-billing-header: cc_version=" + version + "." + suffix + "; cc_entrypoint=cli; cch=00000;"
	block := map[string]any{"type": "text", "text": text}

	system := gjson.GetBytes(body, "system")
	if !system.Exists() {
		if next, err := sjson.SetBytes(body, "system", []any{block}); err == nil {
			return next
		}
		return body
	}
	if system.Type == gjson.String {
		existing := system.String()
		next, err := sjson.SetBytes(body, "system", []any{
			block,
			map[string]any{"type": "text", "text": existing},
		})
		if err == nil {
			return next
		}
		return body
	}
	if system.IsArray() {
		// Rebuild system[] with the billing block first.
		items := make([]any, 0)
		items = append(items, block)
		system.ForEach(func(_, item gjson.Result) bool {
			var decoded any
			if err := decodeJSONResult(item, &decoded); err == nil {
				items = append(items, decoded)
			}
			return true
		})
		if next, err := sjson.SetBytes(body, "system", items); err == nil {
			return next
		}
	}
	return body
}

// decodeJSONResult unmarshals a gjson.Result into the provided value. Kept
// tiny on purpose — gjson does not expose a direct map/slice extraction API.
func decodeJSONResult(r gjson.Result, out any) error {
	return json.Unmarshal([]byte(r.Raw), out)
}

// signBillingHeaderCCH computes the xxHash64-based CCH signature for the request
// body and replaces the cch=00000 placeholder with the computed 5-hex-char hash.
// The body must contain the placeholder when this function is called.
func signBillingHeaderCCH(body []byte) []byte {
	if !cchPlaceholderRe.Match(body) {
		return body
	}
	cch := fmt.Sprintf("%05x", xxHash64Seeded(body, cchSeed)&0xFFFFF)
	return cchPlaceholderRe.ReplaceAll(body, []byte("${1}"+cch+"${3}"))
}

// xxHash64Seeded computes xxHash64 of data with a custom seed.
func xxHash64Seeded(data []byte, seed uint64) uint64 {
	d := xxhash.NewWithSeed(seed)
	_, _ = d.Write(data)
	return d.Sum64()
}
