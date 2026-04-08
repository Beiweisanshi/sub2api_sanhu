package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterTelemetryRoutes_DoesNotConflictWithCommonRoutesWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterCommonRoutes(router)

	cfg := &config.Config{}
	cfg.Gateway.MaxBodySize = 1024

	require.NotPanics(t, func() {
		RegisterTelemetryRoutes(
			router,
			&handler.Handlers{Telemetry: &handler.TelemetryHandler{}},
			nil,
			cfg,
		)
	})
}
