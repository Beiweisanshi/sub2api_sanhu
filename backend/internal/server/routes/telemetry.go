package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterTelemetryRoutes registers telemetry proxy routes for Claude Code.
// These routes intercept, rewrite, and forward telemetry events to the upstream
// Anthropic API with normalized identity and environment data.
// No API key authentication is required — these are transparent proxy endpoints.
func RegisterTelemetryRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	opsService *service.OpsService,
	cfg *config.Config,
) {
	if h.Telemetry == nil {
		return
	}

	bodyLimit := middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize)
	clientRequestID := middleware.ClientRequestID()
	opsErrorLogger := handler.OpsErrorLoggerMiddleware(opsService)

	// /api/event_logging group — Claude Code telemetry events
	eventLogging := r.Group("/api")
	eventLogging.Use(bodyLimit)
	eventLogging.Use(clientRequestID)
	eventLogging.Use(opsErrorLogger)
	{
		eventLogging.POST("/event_logging/batch", h.Telemetry.EventLoggingBatch)
		eventLogging.POST("/event_logging", h.Telemetry.EventLogging)
	}

	// /policy_limits and /settings — Claude Code configuration endpoints
	r.POST("/policy_limits", bodyLimit, clientRequestID, opsErrorLogger, h.Telemetry.PolicyLimits)
	r.Any("/settings", bodyLimit, clientRequestID, opsErrorLogger, h.Telemetry.Settings)
}
