package web

import "strings"

func shouldBypassEmbeddedFrontend(path string) bool {
	trimmed := strings.TrimSpace(path)
	return strings.HasPrefix(trimmed, "/api/") ||
		strings.HasPrefix(trimmed, "/v1/") ||
		strings.HasPrefix(trimmed, "/v1beta/") ||
		strings.HasPrefix(trimmed, "/sora/") ||
		strings.HasPrefix(trimmed, "/antigravity/") ||
		strings.HasPrefix(trimmed, "/setup/") ||
		trimmed == "/health" ||
		trimmed == "/policy_limits" ||
		trimmed == "/settings" ||
		trimmed == "/responses" ||
		strings.HasPrefix(trimmed, "/responses/") ||
		strings.HasPrefix(trimmed, "/images/")
}
