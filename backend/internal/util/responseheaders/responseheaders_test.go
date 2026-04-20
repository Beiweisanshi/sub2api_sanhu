package responseheaders

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestFilterHeadersDisabledUsesDefaultAllowlist(t *testing.T) {
	src := http.Header{}
	src.Add("Content-Type", "application/json")
	src.Add("X-Request-Id", "req-123")
	src.Add("X-Test", "ok")
	src.Add("Connection", "keep-alive")
	src.Add("Content-Length", "123")

	cfg := config.ResponseHeaderConfig{
		Enabled:     false,
		ForceRemove: []string{"x-request-id"},
	}

	filtered := FilterHeaders(src, CompileHeaderFilter(cfg))
	if filtered.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type passthrough, got %q", filtered.Get("Content-Type"))
	}
	if filtered.Get("X-Request-Id") != "req-123" {
		t.Fatalf("expected X-Request-Id allowed, got %q", filtered.Get("X-Request-Id"))
	}
	if filtered.Get("X-Test") != "" {
		t.Fatalf("expected X-Test removed, got %q", filtered.Get("X-Test"))
	}
	if filtered.Get("Connection") != "" {
		t.Fatalf("expected Connection to be removed, got %q", filtered.Get("Connection"))
	}
	if filtered.Get("Content-Length") != "" {
		t.Fatalf("expected Content-Length to be removed, got %q", filtered.Get("Content-Length"))
	}
}

func TestFilterHeadersEnabledUsesAllowlist(t *testing.T) {
	src := http.Header{}
	src.Add("Content-Type", "application/json")
	src.Add("X-Extra", "ok")
	src.Add("X-Remove", "nope")
	src.Add("X-Blocked", "nope")

	cfg := config.ResponseHeaderConfig{
		Enabled:           true,
		AdditionalAllowed: []string{"x-extra"},
		ForceRemove:       []string{"x-remove"},
	}

	filtered := FilterHeaders(src, CompileHeaderFilter(cfg))
	if filtered.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type allowed, got %q", filtered.Get("Content-Type"))
	}
	if filtered.Get("X-Extra") != "ok" {
		t.Fatalf("expected X-Extra allowed, got %q", filtered.Get("X-Extra"))
	}
	if filtered.Get("X-Remove") != "" {
		t.Fatalf("expected X-Remove removed, got %q", filtered.Get("X-Remove"))
	}
	if filtered.Get("X-Blocked") != "" {
		t.Fatalf("expected X-Blocked removed, got %q", filtered.Get("X-Blocked"))
	}
}

func TestHasGatewayFingerprintPrefix(t *testing.T) {
	cases := map[string]bool{
		"x-litellm-request-id":  true,
		"helicone-cache":        true,
		"x-portkey-something":   true,
		"cf-aig-cache-status":   true,
		"x-kong-upstream-uri":   true,
		"x-bt-trace-id":         true,
		"X-LITELLM-DEBUG":       true, // case-insensitive
		"content-type":          false,
		"x-ratelimit-remaining": false,
		"":                      false,
	}
	for key, want := range cases {
		if got := HasGatewayFingerprintPrefix(key); got != want {
			t.Fatalf("HasGatewayFingerprintPrefix(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestFilterHeaders_StripsGatewayPrefix(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "application/json")
	src.Set("x-litellm-model", "gpt-4o")
	src.Set("Helicone-Cache", "HIT")

	// Even if the allowlist accidentally permits one of these, the prefix
	// filter should still strip it.
	filter := CompileHeaderFilter(config.ResponseHeaderConfig{
		Enabled:           true,
		AdditionalAllowed: []string{"x-litellm-model", "helicone-cache"},
	})
	filtered := FilterHeaders(src, filter)

	if filtered.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type kept, got %q", filtered.Get("Content-Type"))
	}
	if filtered.Get("x-litellm-model") != "" {
		t.Fatalf("expected x-litellm-model stripped, got %q", filtered.Get("x-litellm-model"))
	}
	if filtered.Get("Helicone-Cache") != "" {
		t.Fatalf("expected Helicone-Cache stripped, got %q", filtered.Get("Helicone-Cache"))
	}
}
