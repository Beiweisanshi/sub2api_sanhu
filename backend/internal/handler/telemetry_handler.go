package handler

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// TelemetryHandler handles Claude Code telemetry event proxying.
// It rewrites identity, environment, and process metrics in telemetry payloads
// before forwarding them to the upstream Anthropic API.
type TelemetryHandler struct {
	rewriterService     *service.TelemetryRewriterService
	claudeTokenProvider *service.ClaudeTokenProvider
	accountRepo         service.AccountRepository
	httpUpstream        service.HTTPUpstream
	heartbeatService    *service.TelemetryHeartbeatService
	cfg                 *config.Config
}

// NewTelemetryHandler creates a new TelemetryHandler.
func NewTelemetryHandler(
	rewriterService *service.TelemetryRewriterService,
	claudeTokenProvider *service.ClaudeTokenProvider,
	accountRepo service.AccountRepository,
	httpUpstream service.HTTPUpstream,
	heartbeatService *service.TelemetryHeartbeatService,
	cfg *config.Config,
) *TelemetryHandler {
	return &TelemetryHandler{
		rewriterService:     rewriterService,
		claudeTokenProvider: claudeTokenProvider,
		accountRepo:         accountRepo,
		httpUpstream:        httpUpstream,
		heartbeatService:    heartbeatService,
		cfg:                 cfg,
	}
}

// EvalFeatures handles GET/POST /api/eval/features. Returns a realistic
// GrowthBook feature payload so Claude Code treats the gateway's response as
// a normal CLI session. Returning empty {} (the naive pass-through outcome)
// is itself a signal — real CLI sessions always get populated flags. Values
// mirror zhima2api cc-gateway telemetry.ts buildRealisticFeatures().
func (h *TelemetryHandler) EvalFeatures(c *gin.Context) {
	if h.cfg == nil || !h.cfg.Telemetry.Enabled {
		c.Status(http.StatusOK)
		return
	}
	version := strings.TrimSpace(h.cfg.Telemetry.CanonicalEnv.Version)
	if version == "" {
		version = "2.1.22"
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"features": gin.H{
			"tengu_version_config": gin.H{
				"defaultValue": gin.H{
					"minimumSupportedVersion": "1.0.0",
					"latestVersion":           version,
				},
			},
			"tengu_data_collection_sampling_config": gin.H{
				"defaultValue": gin.H{
					"sessionSamplingRate": 0.01,
					"eventSamplingRate":   0.1,
				},
			},
			"tengu_enable_prompt_caching":    gin.H{"defaultValue": true},
			"tengu_enable_extended_thinking": gin.H{"defaultValue": true},
			"tengu_enable_mcp":               gin.H{"defaultValue": true},
			"tengu_enable_computer_use":      gin.H{"defaultValue": false},
			"tengu_max_output_tokens_config": gin.H{
				"defaultValue": gin.H{
					"maxOutputTokens":                  16000,
					"maxOutputTokensExtendedThinking":  32000,
				},
			},
			"tengu_haiku_model_config":  gin.H{"defaultValue": gin.H{"modelId": "claude-haiku-4-5-20251001"}},
			"tengu_sonnet_model_config": gin.H{"defaultValue": gin.H{"modelId": "claude-sonnet-4-5-20250514"}},
			"tengu_opus_model_config":   gin.H{"defaultValue": gin.H{"modelId": "claude-opus-4-5-20250514"}},
		},
		"dateUpdated": time.Now().UTC().Format(time.RFC3339),
	})
}

// MetricsEnabled handles GET/POST /api/metrics_enabled. CLI requires this
// endpoint to respond {enabled:true}; absence / 404 suppresses all other
// telemetry channels and makes sessions look inert. Kept separate from the
// forwarding path because there is no rewriting needed.
func (h *TelemetryHandler) MetricsEnabled(c *gin.Context) {
	if h.cfg == nil || !h.cfg.Telemetry.Enabled {
		c.Status(http.StatusOK)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"enabled": true})
}

// EventLoggingBatch handles POST /api/event_logging/batch
func (h *TelemetryHandler) EventLoggingBatch(c *gin.Context) {
	h.proxyTelemetry(c, func(body []byte, identity config.TelemetryIdentityConfig) ([]byte, error) {
		return h.rewriterService.RewriteEventBatchWithIdentity(body, identity)
	})
}

// EventLogging handles POST /api/event_logging
func (h *TelemetryHandler) EventLogging(c *gin.Context) {
	h.proxyTelemetry(c, func(body []byte, identity config.TelemetryIdentityConfig) ([]byte, error) {
		return h.rewriterService.RewriteEventBatchWithIdentity(body, identity)
	})
}

// PolicyLimits handles POST /policy_limits
func (h *TelemetryHandler) PolicyLimits(c *gin.Context) {
	h.proxyTelemetry(c, func(body []byte, identity config.TelemetryIdentityConfig) ([]byte, error) {
		return h.rewriterService.RewriteGenericIdentityWithIdentity(body, identity)
	})
}

// Settings handles GET/POST /settings
func (h *TelemetryHandler) Settings(c *gin.Context) {
	h.proxyTelemetry(c, func(body []byte, identity config.TelemetryIdentityConfig) ([]byte, error) {
		return h.rewriterService.RewriteGenericIdentityWithIdentity(body, identity)
	})
}

// proxyTelemetry is the shared logic: read body, rewrite, forward to upstream.
func (h *TelemetryHandler) proxyTelemetry(c *gin.Context, rewrite func([]byte, config.TelemetryIdentityConfig) ([]byte, error)) {
	if h.cfg == nil || !h.cfg.Telemetry.Enabled {
		c.Status(http.StatusOK)
		return
	}

	identity := h.cfg.Telemetry.Identity
	var account *service.Account
	var token string

	if h.cfg.Telemetry.ForwardEvents {
		// Select any available Anthropic OAuth account for forwarding
		var err error
		account, token, err = h.getForwardingCredentials(c.Request.Context())
		if err != nil {
			slog.Warn("telemetry: no account available for forwarding", "error", err)
			c.Status(http.StatusOK) // fail silently
			return
		}
		identity = service.CanonicalTelemetryIdentityForAccount(account, identity)
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		slog.Warn("telemetry: failed to read body", "error", err)
		c.Status(http.StatusOK) // fail silently — telemetry should not block client
		return
	}

	// Rewrite the body
	if len(body) > 0 {
		body, err = rewrite(body, identity)
		if err != nil {
			slog.Warn("telemetry: rewrite failed", "error", err)
			c.Status(http.StatusOK)
			return
		}
	}

	// If forwarding is disabled, just return 200
	if !h.cfg.Telemetry.ForwardEvents {
		c.Status(http.StatusOK)
		return
	}

	// Build upstream request
	upstreamURL := strings.TrimRight(h.cfg.Telemetry.UpstreamURL, "/") + c.Request.URL.Path
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("telemetry: failed to build upstream request", "error", err)
		c.Status(http.StatusOK)
		return
	}

	// Set headers
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+token)

	// Passthrough select headers from the original request
	for _, hdr := range []string{"User-Agent", "Accept", "Accept-Encoding", "Accept-Language", "X-Anthropic-Billing-Header"} {
		if v := c.Request.Header.Get(hdr); v != "" {
			upstreamReq.Header.Set(hdr, v)
		}
	}
	if version := strings.TrimSpace(h.cfg.Telemetry.CanonicalEnv.Version); version != "" {
		upstreamReq.Header.Set("User-Agent", service.CanonicalClaudeCLIUserAgent(version))
		if billingHeader := upstreamReq.Header.Get("X-Anthropic-Billing-Header"); billingHeader != "" {
			// Telemetry paths have no messages[] to derive a real fingerprint from;
			// pass empty so RewriteBillingHeaderValue emits a random 3-hex suffix.
			upstreamReq.Header.Set("X-Anthropic-Billing-Header", service.RewriteBillingHeaderValue(billingHeader, version, ""))
		}
	}

	// Forward request
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := h.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		slog.Warn("telemetry: upstream request failed", "error", err)
		c.Status(http.StatusOK)
		return
	}
	defer resp.Body.Close()

	// 捕获 event_logging/batch 样本，供心跳服务重放（仅在成功转发时）
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && h.heartbeatService != nil {
		if p := c.Request.URL.Path; strings.Contains(p, "event_logging") {
			h.heartbeatService.CaptureSample(account.ID, body, upstreamReq.Header.Get("Content-Type"), p)
		}
	}

	// Stream upstream response back to client
	for key, vals := range resp.Header {
		for _, v := range vals {
			c.Writer.Header().Add(key, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// getForwardingCredentials selects an available Anthropic OAuth account and
// retrieves its access token for telemetry forwarding.
func (h *TelemetryHandler) getForwardingCredentials(ctx context.Context) (*service.Account, string, error) {
	accounts, err := h.accountRepo.ListSchedulableByPlatform(ctx, service.PlatformAnthropic)
	if err != nil {
		return nil, "", err
	}

	// Try each schedulable Anthropic OAuth account
	for i := range accounts {
		acc := &accounts[i]
		if acc.Type != service.AccountTypeOAuth {
			continue
		}
		token, err := h.claudeTokenProvider.GetAccessToken(ctx, acc)
		if err != nil {
			continue
		}
		return acc, token, nil
	}

	return nil, "", io.ErrUnexpectedEOF
}
