//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type telemetryAccountRepoStub struct {
	service.AccountRepository
	accounts []service.Account
}

func (r telemetryAccountRepoStub) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	var result []service.Account
	for _, account := range r.accounts {
		if account.Platform == platform && account.IsSchedulable() {
			result = append(result, account)
		}
	}
	return result, nil
}

type telemetryHTTPUpstreamStub struct {
	lastReq  *http.Request
	lastBody []byte
	resp     *http.Response
	err      error
}

func (u *telemetryHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.lastReq = req
	if req != nil && req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		u.lastBody = b
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	if u.err != nil {
		return nil, u.err
	}
	return u.resp, nil
}

func (u *telemetryHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestTelemetryHandler_RewritesForwardedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Telemetry: config.TelemetryConfig{
			Enabled:       true,
			ForwardEvents: true,
			UpstreamURL:   "https://api.anthropic.com",
			Identity: config.TelemetryIdentityConfig{
				DeviceID: "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888",
				Email:    "canonical@example.com",
			},
			CanonicalEnv: config.TelemetryCanonicalEnvConfig{
				Version: "2.1.99",
			},
			LeakFields: []string{"baseUrl", "base_url", "gateway"},
		},
	}

	upstream := &telemetryHTTPUpstreamStub{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(`{"ok":true}`)),
		},
	}
	account := service.Account{
		ID:          9001,
		Name:        "telemetry-oauth",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"email":        "telemetry@example.com",
		},
	}
	handler := NewTelemetryHandler(
		service.NewTelemetryRewriterService(cfg),
		service.NewClaudeTokenProvider(nil, nil, nil),
		telemetryAccountRepoStub{accounts: []service.Account{account}},
		upstream,
		nil, // heartbeat service: not needed for this test
		cfg,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/event_logging/batch",
		strings.NewReader(`{"events":[{"event_data":{"device_id":"real-device","email":"real@example.com"}}]}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "claude-cli/9.9.9 (darwin; arm64)")
	c.Request.Header.Set("X-Anthropic-Billing-Header", "cc_version=9.9.9.abc; cc_entrypoint=cli")

	handler.EventLoggingBatch(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, `{"ok":true}`, rec.Body.String())
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "Bearer oauth-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "claude-cli/2.1.99 (external, cli)", upstream.lastReq.Header.Get("User-Agent"))
	// Telemetry paths can't derive a real fingerprint from a messages[] array,
	// so RewriteBillingHeaderValue emits a random 3-hex suffix. Assert the
	// shape (version rewritten + suffix present) rather than a fixed literal.
	require.Regexp(t,
		`^cc_version=2\.1\.99\.[0-9a-f]{3}; cc_entrypoint=cli$`,
		upstream.lastReq.Header.Get("X-Anthropic-Billing-Header"),
	)
	expectedIdentity := service.CanonicalTelemetryIdentityForAccount(&account, cfg.Telemetry.Identity)
	require.Equal(
		t,
		expectedIdentity.DeviceID,
		gjson.GetBytes(upstream.lastBody, "events.0.event_data.device_id").String(),
	)
	require.Equal(t, expectedIdentity.Email, gjson.GetBytes(upstream.lastBody, "events.0.event_data.email").String())
}

func TestTelemetryHandler_DisabledReturnsOKWithoutForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Telemetry: config.TelemetryConfig{
			Enabled: false,
		},
	}
	upstream := &telemetryHTTPUpstreamStub{}
	handler := NewTelemetryHandler(
		service.NewTelemetryRewriterService(cfg),
		service.NewClaudeTokenProvider(nil, nil, nil),
		telemetryAccountRepoStub{},
		upstream,
		nil, // heartbeat service: not needed for this test
		cfg,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/event_logging/batch",
		strings.NewReader(`{"events":[{"event_data":{"device_id":"real-device"}}]}`),
	)

	handler.EventLoggingBatch(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, upstream.lastReq)
}

// TestTelemetryHandler_EvalFeatures_RealisticPayload verifies the intercept
// handler returns a non-empty features map matching what a legitimate
// Claude Code CLI would see. Empty {} would itself be a risk signal.
func TestTelemetryHandler_EvalFeatures_RealisticPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{Telemetry: config.TelemetryConfig{
		Enabled:      true,
		CanonicalEnv: config.TelemetryCanonicalEnvConfig{Version: "2.1.22"},
	}}
	handler := NewTelemetryHandler(
		service.NewTelemetryRewriterService(cfg),
		service.NewClaudeTokenProvider(nil, nil, nil),
		telemetryAccountRepoStub{},
		&telemetryHTTPUpstreamStub{},
		nil,
		cfg,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/eval/features", nil)
	handler.EvalFeatures(c)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "tengu_enable_prompt_caching")
	require.Contains(t, body, "tengu_version_config")
	require.Contains(t, body, `"latestVersion":"2.1.22"`)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

// TestTelemetryHandler_MetricsEnabled_ReturnsTrue verifies the CLI probe
// receives {enabled:true}; absence triggers the CLI to suppress telemetry
// entirely, which looks inert to upstream.
func TestTelemetryHandler_MetricsEnabled_ReturnsTrue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{Telemetry: config.TelemetryConfig{Enabled: true}}
	handler := NewTelemetryHandler(
		service.NewTelemetryRewriterService(cfg),
		service.NewClaudeTokenProvider(nil, nil, nil),
		telemetryAccountRepoStub{},
		&telemetryHTTPUpstreamStub{},
		nil,
		cfg,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/metrics_enabled", nil)
	handler.MetricsEnabled(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"enabled":true}`, rec.Body.String())
}

// TestTelemetryHandler_TelemetryDisabled_ShortCircuits verifies both intercept
// endpoints still return 200 (no panic, no upstream call) when
// telemetry.enabled=false.
func TestTelemetryHandler_TelemetryDisabled_ShortCircuits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{Telemetry: config.TelemetryConfig{Enabled: false}}
	upstream := &telemetryHTTPUpstreamStub{}
	handler := NewTelemetryHandler(
		service.NewTelemetryRewriterService(cfg),
		service.NewClaudeTokenProvider(nil, nil, nil),
		telemetryAccountRepoStub{},
		upstream,
		nil,
		cfg,
	)

	for _, fn := range []func(*gin.Context){handler.EvalFeatures, handler.MetricsEnabled} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/_", nil)
		fn(c)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	require.Nil(t, upstream.lastReq, "intercept endpoints must not talk to upstream")
}
