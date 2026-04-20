package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

func TestCreateFingerprintFromHeaders_UsesLatestClaudeDefaults(t *testing.T) {
	svc := NewIdentityService(nil)

	fp := svc.createFingerprintFromHeaders(http.Header{})

	if got := fp.UserAgent; got != "claude-cli/"+claude.DefaultCLIVersion+" (external, cli)" {
		t.Fatalf("unexpected default user-agent: %q", got)
	}
	if got := fp.StainlessPackageVersion; got != claude.DefaultStainlessPackageVersion {
		t.Fatalf("unexpected default x-stainless-package-version: %q", got)
	}
}
