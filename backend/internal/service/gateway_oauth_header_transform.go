package service

import (
	"net/http"

	"github.com/google/uuid"
)

// applyClaudeCodeHeaderRewrites runs the header-level hardening pass that
// complements applyClaudeCodeBodyRewrites. Call after the upstream *http.Request
// has been populated with client-passthrough + fingerprint headers.
//
// Currently responsible for:
//   - stripping `x-claude-remote-container-id` / `x-claude-remote-session-id`
//     (defense-in-depth — they already sit outside the whitelist, but passthrough
//     mode and future whitelist edits could re-expose them)
//   - regenerating `x-client-request-id` so the same UUID never appears across
//     two accounts / two retries of the same logical request
//
// Runs only when fw.RemoteHeaderStrip is true, so admins can flip it off
// instantly via the settings table without a redeploy.
func applyClaudeCodeHeaderRewrites(req *http.Request, fw GatewayForwardingSettings) {
	if req == nil || !fw.RemoteHeaderStrip {
		return
	}

	// HTTP header names are case-insensitive; http.Header.Del handles that for
	// us, but we cover both forms that gin passes along to be safe.
	req.Header.Del("x-claude-remote-container-id")
	req.Header.Del("X-Claude-Remote-Container-Id")
	req.Header.Del("x-claude-remote-session-id")
	req.Header.Del("X-Claude-Remote-Session-Id")

	// Always emit a freshly-generated request id so cross-account requests
	// never share the same UUID (otherwise Datadog-style correlation from the
	// upstream can link two accounts running behind this gateway).
	setHeaderRaw(req.Header, "x-client-request-id", uuid.NewString())
}
