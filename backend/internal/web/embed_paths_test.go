package web

import "testing"

func TestShouldBypassEmbeddedFrontend(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/api/v1/settings/public", want: true},
		{path: "/v1/messages", want: true},
		{path: "/sora/tasks", want: true},
		{path: "/antigravity/jobs", want: true},
		{path: "/policy_limits", want: true},
		{path: "/settings", want: true},
		{path: "/responses", want: true},
		{path: "/responses/123", want: true},
		{path: "/dashboard", want: false},
		{path: "/users/123", want: false},
	}

	for _, tt := range tests {
		if got := shouldBypassEmbeddedFrontend(tt.path); got != tt.want {
			t.Fatalf("shouldBypassEmbeddedFrontend(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
