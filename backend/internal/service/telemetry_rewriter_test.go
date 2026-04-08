package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newTestTelemetryConfig() *config.Config {
	return &config.Config{
		Telemetry: config.TelemetryConfig{
			Enabled:       true,
			ForwardEvents: true,
			UpstreamURL:   "https://api.anthropic.com",
			Identity: config.TelemetryIdentityConfig{
				DeviceID: "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888",
				Email:    "canonical@example.com",
			},
			CanonicalEnv: config.TelemetryCanonicalEnvConfig{
				Platform:              "linux",
				Arch:                  "x64",
				NodeVersion:           "v24.13.0",
				Terminal:              "xterm-256color",
				PackageManagers:       "npm,pnpm",
				Runtimes:              "node,bun",
				IsClaudeAIAuth:        true,
				Version:               "2.1.22",
				BuildTime:             "2025-06-01T00:00:00Z",
				DeploymentEnvironment: "production",
				VCS:                   "git",
			},
			Process: config.TelemetryProcessConfig{
				ConstrainedMemory: 16 * 1024 * 1024 * 1024,
				RssMin:            80 * 1024 * 1024,
				RssMax:            200 * 1024 * 1024,
				HeapTotalMin:      60 * 1024 * 1024,
				HeapTotalMax:      150 * 1024 * 1024,
				HeapUsedMin:       40 * 1024 * 1024,
				HeapUsedMax:       120 * 1024 * 1024,
			},
			LeakFields: []string{"baseUrl", "base_url", "gateway"},
		},
	}
}

func TestRewriteEventBatch_MultipleEvents(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	body := []byte(`{
		"events": [
			{
				"event_data": {
					"device_id": "real_device_id_1",
					"email": "real@user.com",
					"env": {"platform": "darwin", "arch": "arm64", "is_ci": true},
					"baseUrl": "https://my-gateway.com",
					"base_url": "https://my-gateway.com",
					"gateway": "my-proxy"
				}
			},
			{
				"event_data": {
					"device_id": "real_device_id_2",
					"email": "another@user.com",
					"gateway": "another-proxy"
				}
			}
		]
	}`)

	result, err := svc.RewriteEventBatch(body)
	require.NoError(t, err)

	// Event 0: check device_id and email rewritten
	require.Equal(t, cfg.Telemetry.Identity.DeviceID, gjson.GetBytes(result, "events.0.event_data.device_id").String())
	require.Equal(t, cfg.Telemetry.Identity.Email, gjson.GetBytes(result, "events.0.event_data.email").String())

	// Event 0: check env replaced with canonical
	require.Equal(t, "linux", gjson.GetBytes(result, "events.0.event_data.env.platform").String())
	require.Equal(t, "x64", gjson.GetBytes(result, "events.0.event_data.env.arch").String())
	require.Equal(t, false, gjson.GetBytes(result, "events.0.event_data.env.is_ci").Bool())

	// Event 0: check leak fields deleted
	require.False(t, gjson.GetBytes(result, "events.0.event_data.baseUrl").Exists())
	require.False(t, gjson.GetBytes(result, "events.0.event_data.base_url").Exists())
	require.False(t, gjson.GetBytes(result, "events.0.event_data.gateway").Exists())

	// Event 1: check device_id rewritten
	require.Equal(t, cfg.Telemetry.Identity.DeviceID, gjson.GetBytes(result, "events.1.event_data.device_id").String())
	require.Equal(t, cfg.Telemetry.Identity.Email, gjson.GetBytes(result, "events.1.event_data.email").String())
	require.False(t, gjson.GetBytes(result, "events.1.event_data.gateway").Exists())
}

func TestRewriteEventBatch_ProcessPlainObject(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	body := []byte(`{
		"events": [{
			"event_data": {
				"process": {
					"constrainedMemory": 8589934592,
					"rss": 50000000,
					"heapTotal": 30000000,
					"heapUsed": 20000000,
					"uptime": 1234.5,
					"cpuUsage": {"user": 100, "system": 50}
				}
			}
		}]
	}`)

	result, err := svc.RewriteEventBatch(body)
	require.NoError(t, err)

	proc := gjson.GetBytes(result, "events.0.event_data.process")
	require.True(t, proc.Exists())

	// constrainedMemory should be canonical value
	require.Equal(t, cfg.Telemetry.Process.ConstrainedMemory, proc.Get("constrainedMemory").Int())

	// rss should be in range
	rss := proc.Get("rss").Int()
	require.GreaterOrEqual(t, rss, cfg.Telemetry.Process.RssMin)
	require.Less(t, rss, cfg.Telemetry.Process.RssMax)

	// heapTotal should be in range
	heapTotal := proc.Get("heapTotal").Int()
	require.GreaterOrEqual(t, heapTotal, cfg.Telemetry.Process.HeapTotalMin)
	require.Less(t, heapTotal, cfg.Telemetry.Process.HeapTotalMax)

	// heapUsed should be in range
	heapUsed := proc.Get("heapUsed").Int()
	require.GreaterOrEqual(t, heapUsed, cfg.Telemetry.Process.HeapUsedMin)
	require.Less(t, heapUsed, cfg.Telemetry.Process.HeapUsedMax)

	// uptime and cpuUsage should be preserved
	require.InDelta(t, 1234.5, proc.Get("uptime").Float(), 0.01)
	require.Equal(t, int64(100), proc.Get("cpuUsage.user").Int())
}

func TestRewriteEventBatch_ProcessBase64(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	// Create a base64-encoded process object
	procObj := map[string]any{
		"constrainedMemory": 8589934592,
		"rss":               50000000,
		"heapTotal":         30000000,
		"heapUsed":          20000000,
		"uptime":            999.9,
	}
	procJSON, _ := json.Marshal(procObj)
	procB64 := base64.StdEncoding.EncodeToString(procJSON)

	body := []byte(`{"events": [{"event_data": {"process": "` + procB64 + `"}}]}`)

	result, err := svc.RewriteEventBatch(body)
	require.NoError(t, err)

	// Process should still be a base64 string
	procResult := gjson.GetBytes(result, "events.0.event_data.process")
	require.Equal(t, gjson.String, procResult.Type)

	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(procResult.String())
	require.NoError(t, err)

	var rewrittenProc map[string]any
	err = json.Unmarshal(decoded, &rewrittenProc)
	require.NoError(t, err)

	// constrainedMemory should be overwritten
	cm, ok := rewrittenProc["constrainedMemory"].(float64)
	require.True(t, ok)
	require.Equal(t, float64(cfg.Telemetry.Process.ConstrainedMemory), cm)

	// uptime should be preserved
	uptime, ok := rewrittenProc["uptime"].(float64)
	require.True(t, ok)
	require.InDelta(t, 999.9, uptime, 0.01)
}

func TestRewriteEventBatch_AdditionalMetadata(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	// Create base64-encoded additional_metadata with leak fields
	meta := map[string]any{
		"repo_hash": "abc123",
		"baseUrl":   "https://my-gateway.com",
		"base_url":  "https://my-gateway.com",
		"gateway":   "my-proxy",
		"other":     "keep-this",
	}
	metaJSON, _ := json.Marshal(meta)
	metaB64 := base64.StdEncoding.EncodeToString(metaJSON)

	body := []byte(`{"events": [{"event_data": {"additional_metadata": "` + metaB64 + `"}}]}`)

	result, err := svc.RewriteEventBatch(body)
	require.NoError(t, err)

	// Decode the rewritten additional_metadata
	rewrittenB64 := gjson.GetBytes(result, "events.0.event_data.additional_metadata").String()
	decoded, err := base64.StdEncoding.DecodeString(rewrittenB64)
	require.NoError(t, err)

	var rewrittenMeta map[string]any
	err = json.Unmarshal(decoded, &rewrittenMeta)
	require.NoError(t, err)

	// Leak fields should be removed
	_, hasBaseUrl := rewrittenMeta["baseUrl"]
	require.False(t, hasBaseUrl)
	_, hasBaseUrlSnake := rewrittenMeta["base_url"]
	require.False(t, hasBaseUrlSnake)
	_, hasGateway := rewrittenMeta["gateway"]
	require.False(t, hasGateway)

	// Other fields should be preserved
	require.Equal(t, "abc123", rewrittenMeta["repo_hash"])
	require.Equal(t, "keep-this", rewrittenMeta["other"])
}

func TestRewriteEventBatch_EmptyEvents(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	body := []byte(`{"events": []}`)
	result, err := svc.RewriteEventBatch(body)
	require.NoError(t, err)
	require.JSONEq(t, `{"events": []}`, string(result))
}

func TestRewriteEventBatch_NoEventsField(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	body := []byte(`{"something": "else"}`)
	result, err := svc.RewriteEventBatch(body)
	require.NoError(t, err)
	require.JSONEq(t, `{"something": "else"}`, string(result))
}

func TestRewriteGenericIdentity(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	body := []byte(`{"device_id": "real_id", "email": "real@user.com", "other": "keep"}`)
	result, err := svc.RewriteGenericIdentity(body)
	require.NoError(t, err)

	require.Equal(t, cfg.Telemetry.Identity.DeviceID, gjson.GetBytes(result, "device_id").String())
	require.Equal(t, cfg.Telemetry.Identity.Email, gjson.GetBytes(result, "email").String())
	require.Equal(t, "keep", gjson.GetBytes(result, "other").String())
}

func TestCanonicalTelemetryIdentityForAccount_UsesEmailAsStableSeed(t *testing.T) {
	fallback := config.TelemetryIdentityConfig{
		DeviceID: strings.Repeat("a", 64),
		Email:    "fallback@example.com",
	}
	account := &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Name:     "test-account",
		Credentials: map[string]any{
			"email_address": "User@example.com",
		},
	}

	identity := CanonicalTelemetryIdentityForAccount(account, fallback)
	require.Equal(t, "user@example.com", identity.Email)
	require.Equal(t, deriveTelemetryDeviceID("email:user@example.com"), identity.DeviceID)
}

func TestCanonicalTelemetryIdentityForAccount_FallsBackToStableAccountTuple(t *testing.T) {
	fallback := config.TelemetryIdentityConfig{
		DeviceID: strings.Repeat("a", 64),
		Email:    "fallback@example.com",
	}
	account := &Account{
		ID:       7,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Name:     "stable-account",
	}

	identity := CanonicalTelemetryIdentityForAccount(account, fallback)
	require.Equal(t, fallback.Email, identity.Email)
	require.Equal(
		t,
		deriveTelemetryDeviceID("platform:anthropic|type:oauth|id:7|name:stable-account"),
		identity.DeviceID,
	)
}

func TestRewriteEventBatch_CanonicalEnvFields(t *testing.T) {
	cfg := newTestTelemetryConfig()
	svc := NewTelemetryRewriterService(cfg)

	body := []byte(`{
		"events": [{
			"event_data": {
				"env": {
					"platform": "darwin",
					"arch": "arm64",
					"is_ci": true,
					"is_claude_code_remote": true,
					"extra_field": "should_be_gone"
				}
			}
		}]
	}`)

	result, err := svc.RewriteEventBatch(body)
	require.NoError(t, err)

	env := gjson.GetBytes(result, "events.0.event_data.env")

	// Canonical values
	require.Equal(t, "linux", env.Get("platform").String())
	require.Equal(t, "x64", env.Get("arch").String())
	require.Equal(t, "v24.13.0", env.Get("node_version").String())
	require.Equal(t, "2.1.22", env.Get("version").String())

	// Boolean flags hardcoded to false
	require.False(t, env.Get("is_ci").Bool())
	require.False(t, env.Get("is_claude_code_remote").Bool())
	require.False(t, env.Get("is_conductor").Bool())

	// is_claude_ai_auth should be true
	require.True(t, env.Get("is_claude_ai_auth").Bool())

	// extra_field from original should be gone (full replacement)
	require.False(t, env.Get("extra_field").Exists())
}

func TestRandomInRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		v := randomInRange(10, 20)
		require.GreaterOrEqual(t, v, int64(10))
		require.Less(t, v, int64(20))
	}

	// Edge case: min == max
	require.Equal(t, int64(5), randomInRange(5, 5))

	// Edge case: max < min
	require.Equal(t, int64(10), randomInRange(10, 5))
}
