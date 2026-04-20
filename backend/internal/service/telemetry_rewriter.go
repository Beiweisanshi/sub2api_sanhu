package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TelemetryRewriterService rewrites Claude Code telemetry event payloads
// to normalize identity, environment, and process metrics before forwarding
// to the upstream Anthropic API.
type TelemetryRewriterService struct {
	cfg *config.Config
}

// NewTelemetryRewriterService creates a new TelemetryRewriterService.
func NewTelemetryRewriterService(cfg *config.Config) *TelemetryRewriterService {
	return &TelemetryRewriterService{cfg: cfg}
}

// CanonicalTelemetryIdentityForAccount derives a stable telemetry identity for an account.
// Priority:
// 1. account credential email / email_address
// 2. account_uuid
// 3. org_uuid
// 4. stable account tuple (platform/type/id/name)
func CanonicalTelemetryIdentityForAccount(account *Account, fallback config.TelemetryIdentityConfig) config.TelemetryIdentityConfig {
	identity := fallback
	if account == nil {
		return identity
	}

	if email := telemetryAccountEmail(account); email != "" {
		identity.Email = email
		identity.DeviceID = deriveTelemetryDeviceID("email:" + email)
		return identity
	}
	if accountUUID := normalizeTelemetryIdentityComponent(account.GetCredential("account_uuid")); accountUUID != "" {
		identity.DeviceID = deriveTelemetryDeviceID("account_uuid:" + accountUUID)
		return identity
	}
	if orgUUID := normalizeTelemetryIdentityComponent(account.GetCredential("org_uuid")); orgUUID != "" {
		identity.DeviceID = deriveTelemetryDeviceID("org_uuid:" + orgUUID)
		return identity
	}

	seed := fmt.Sprintf(
		"platform:%s|type:%s|id:%d|name:%s",
		normalizeTelemetryIdentityComponent(account.Platform),
		normalizeTelemetryIdentityComponent(account.Type),
		account.ID,
		normalizeTelemetryIdentityComponent(account.Name),
	)
	identity.DeviceID = deriveTelemetryDeviceID(seed)
	return identity
}

func deriveTelemetryDeviceID(seed string) string {
	normalized := strings.TrimSpace(seed)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("sub2api:telemetry-device:" + normalized))
	return hex.EncodeToString(sum[:])
}

func telemetryAccountEmail(account *Account) string {
	if account == nil {
		return ""
	}
	for _, key := range []string{"email", "email_address"} {
		if value := normalizeTelemetryIdentityComponent(account.GetCredential(key)); value != "" {
			return value
		}
	}
	return ""
}

func normalizeTelemetryIdentityComponent(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// RewriteEventBatch rewrites a /api/event_logging/batch request body.
// It iterates over events[] and rewrites each event_data:
//   - device_id and email -> canonical values
//   - env -> canonical env object (full replacement)
//   - process -> randomized memory metrics (supports base64 and plain JSON)
//   - delete leak fields (baseUrl, base_url, gateway)
//   - additional_metadata -> strip leak fields from base64 JSON blob
func (s *TelemetryRewriterService) RewriteEventBatch(body []byte) ([]byte, error) {
	return s.RewriteEventBatchWithIdentity(body, s.cfg.Telemetry.Identity)
}

// RewriteEventBatchWithIdentity rewrites telemetry event batches using the provided identity.
func (s *TelemetryRewriterService) RewriteEventBatchWithIdentity(body []byte, identity config.TelemetryIdentityConfig) ([]byte, error) {
	events := gjson.GetBytes(body, "events")
	if !events.Exists() || !events.IsArray() {
		return body, nil
	}

	var err error
	events.ForEach(func(key, value gjson.Result) bool {
		idx := key.Int()
		basePath := fmt.Sprintf("events.%d.event_data", idx)
		body = s.rewriteEventData(body, basePath, identity)
		return true
	})

	return body, err
}

// RewriteGenericIdentity rewrites only device_id and email fields in the body.
// Used for /policy_limits and /settings endpoints.
func (s *TelemetryRewriterService) RewriteGenericIdentity(body []byte) ([]byte, error) {
	return s.RewriteGenericIdentityWithIdentity(body, s.cfg.Telemetry.Identity)
}

// RewriteGenericIdentityWithIdentity rewrites top-level identity fields using the provided identity.
func (s *TelemetryRewriterService) RewriteGenericIdentityWithIdentity(body []byte, identity config.TelemetryIdentityConfig) ([]byte, error) {

	if gjson.GetBytes(body, "device_id").Exists() {
		body, _ = sjson.SetBytes(body, "device_id", identity.DeviceID)
	}
	if gjson.GetBytes(body, "email").Exists() {
		body, _ = sjson.SetBytes(body, "email", identity.Email)
	}

	return body, nil
}

// rewriteEventData rewrites a single event's event_data at the given JSON path.
func (s *TelemetryRewriterService) rewriteEventData(body []byte, basePath string, identity config.TelemetryIdentityConfig) []byte {
	// 1. Rewrite device_id
	if gjson.GetBytes(body, basePath+".device_id").Exists() {
		body, _ = sjson.SetBytes(body, basePath+".device_id", identity.DeviceID)
	}

	// 2. Rewrite email
	if gjson.GetBytes(body, basePath+".email").Exists() {
		body, _ = sjson.SetBytes(body, basePath+".email", identity.Email)
	}

	// 3. Replace entire env object with canonical env
	if gjson.GetBytes(body, basePath+".env").Exists() {
		body, _ = sjson.SetBytes(body, basePath+".env", s.buildCanonicalEnv())
	}

	// 4. Rewrite process field (randomize memory metrics)
	body = s.rewriteProcessField(body, basePath+".process")

	// 5. Canonicalize cross-account linkage fields that we cannot simply delete
	// (upstream may treat their absence as anomalous). They get rewritten to
	// deterministic per-identity values so an account always presents the
	// same fingerprint across events.
	if gjson.GetBytes(body, basePath+".hostname").Exists() {
		body, _ = sjson.SetBytes(body, basePath+".hostname", canonicalHostname(s.cfg, identity))
	}
	if gjson.GetBytes(body, basePath+".session_id").Exists() {
		body, _ = sjson.SetBytes(body, basePath+".session_id", canonicalSessionID(identity))
	}

	// 6. Delete leak fields (cross-account vectors we want gone entirely)
	body = s.deleteLeakFields(body, basePath)

	// 7. Rewrite additional_metadata (base64 JSON blob)
	body = s.rewriteAdditionalMetadata(body, basePath+".additional_metadata")

	return body
}

// canonicalHostname returns a stable hostname for the given identity. Prefers
// the username segment of prompt_env.working_dir (mirrors zhima2api's
// rewriter.ts:517 behavior: "/Users/alice/..." → "alice"). Falls back to a
// generic literal when no working_dir is configured, since "empty hostname"
// itself is a signal.
func canonicalHostname(cfg *config.Config, identity config.TelemetryIdentityConfig) string {
	if cfg != nil {
		if wd := cfg.Telemetry.PromptEnv.WorkingDir; wd != "" {
			parts := strings.Split(strings.TrimPrefix(wd, "/"), "/")
			if len(parts) >= 2 && parts[1] != "" {
				return parts[1]
			}
		}
	}
	if identity.DeviceID != "" {
		// Short stable pseudo-hostname so different accounts never collide.
		// 8 hex chars gives 32 bits of entropy — enough for the hundreds of
		// accounts the gateway handles while staying human-shaped.
		return "mac-" + identity.DeviceID[:8]
	}
	return "mac"
}

// canonicalSessionID derives a deterministic UUID-shaped identifier from the
// account's canonical DeviceID. Upstream treats session_id as opaque; using
// a per-identity stable value keeps events from an account correlated under
// the gateway's canonical identity instead of the real client session.
func canonicalSessionID(identity config.TelemetryIdentityConfig) string {
	if identity.DeviceID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("sub2api:telemetry-session:" + identity.DeviceID))
	b := sum[:16]
	// UUID v4: version=0100
	b[6] = (b[6] & 0x0f) | 0x40
	// Variant: 10xx
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// buildCanonicalEnv returns the canonical env object that replaces the real env
// in telemetry events. Mirrors cc-gateway's buildCanonicalEnv().
func (s *TelemetryRewriterService) buildCanonicalEnv() map[string]any {
	env := s.cfg.Telemetry.CanonicalEnv

	platformRaw := env.PlatformRaw
	if platformRaw == "" {
		platformRaw = env.Platform
	}
	versionBase := env.VersionBase
	if versionBase == "" {
		versionBase = env.Version
	}

	return map[string]any{
		"platform":               env.Platform,
		"platform_raw":           platformRaw,
		"arch":                   env.Arch,
		"node_version":           env.NodeVersion,
		"terminal":               env.Terminal,
		"package_managers":       env.PackageManagers,
		"runtimes":               env.Runtimes,
		"is_running_with_bun":    env.IsRunningWithBun,
		"is_ci":                  false,
		"is_claubbit":            false,
		"is_claude_code_remote":  false,
		"is_local_agent_mode":    false,
		"is_conductor":           false,
		"is_github_action":       false,
		"is_claude_code_action":  false,
		"is_claude_ai_auth":      env.IsClaudeAIAuth,
		"version":                env.Version,
		"version_base":           versionBase,
		"build_time":             env.BuildTime,
		"deployment_environment": env.DeploymentEnvironment,
		"vcs":                    env.VCS,
	}
}

// rewriteProcessField handles the process field which may be base64-encoded
// or a plain JSON object. It randomizes memory metrics while preserving
// other fields like uptime and cpuUsage.
func (s *TelemetryRewriterService) rewriteProcessField(body []byte, path string) []byte {
	processResult := gjson.GetBytes(body, path)
	if !processResult.Exists() {
		return body
	}

	pc := s.cfg.Telemetry.Process

	switch processResult.Type {
	case gjson.String:
		// Base64-encoded JSON string
		decoded, err := base64.StdEncoding.DecodeString(processResult.String())
		if err != nil {
			return body
		}
		var proc map[string]any
		if err := json.Unmarshal(decoded, &proc); err != nil {
			return body
		}
		s.applyProcessMetrics(proc, pc)
		reEncoded, err := json.Marshal(proc)
		if err != nil {
			return body
		}
		encoded := base64.StdEncoding.EncodeToString(reEncoded)
		body, _ = sjson.SetBytes(body, path, encoded)

	case gjson.JSON:
		// Plain JSON object — parse into map, mutate, set back
		var proc map[string]any
		if err := json.Unmarshal([]byte(processResult.Raw), &proc); err != nil {
			return body
		}
		s.applyProcessMetrics(proc, pc)
		body, _ = sjson.SetBytes(body, path, proc)
	}

	return body
}

// applyProcessMetrics overwrites memory metric fields with canonical/randomized values.
func (s *TelemetryRewriterService) applyProcessMetrics(proc map[string]any, pc config.TelemetryProcessConfig) {
	proc["constrainedMemory"] = pc.ConstrainedMemory
	proc["rss"] = randomInRange(pc.RssMin, pc.RssMax)
	proc["heapTotal"] = randomInRange(pc.HeapTotalMin, pc.HeapTotalMax)
	proc["heapUsed"] = randomInRange(pc.HeapUsedMin, pc.HeapUsedMax)
}

// deleteLeakFields removes leak fields from event_data.
func (s *TelemetryRewriterService) deleteLeakFields(body []byte, basePath string) []byte {
	for _, field := range s.cfg.Telemetry.LeakFields {
		fieldPath := basePath + "." + field
		if gjson.GetBytes(body, fieldPath).Exists() {
			body, _ = sjson.DeleteBytes(body, fieldPath)
		}
	}
	return body
}

// rewriteAdditionalMetadata decodes a base64 JSON blob, strips leak fields,
// and re-encodes it. Returns body unchanged on any parse error.
func (s *TelemetryRewriterService) rewriteAdditionalMetadata(body []byte, path string) []byte {
	metaResult := gjson.GetBytes(body, path)
	if !metaResult.Exists() || metaResult.Type != gjson.String {
		return body
	}

	decoded, err := base64.StdEncoding.DecodeString(metaResult.String())
	if err != nil {
		return body
	}

	var meta map[string]any
	if err := json.Unmarshal(decoded, &meta); err != nil {
		return body
	}

	// Strip leak fields from decoded metadata
	for _, field := range s.cfg.Telemetry.LeakFields {
		delete(meta, field)
	}

	reEncoded, err := json.Marshal(meta)
	if err != nil {
		return body
	}

	encoded := base64.StdEncoding.EncodeToString(reEncoded)
	body, _ = sjson.SetBytes(body, path, encoded)
	return body
}

// randomInRange returns a random int64 in [min, max).
func randomInRange(min, max int64) int64 {
	if max <= min {
		return min
	}
	return min + rand.Int64N(max-min)
}
