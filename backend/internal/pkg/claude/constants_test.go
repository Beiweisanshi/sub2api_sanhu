package claude

import "testing"

func TestBuildDefaultHeaders_UsesLatest112Defaults(t *testing.T) {
	headers := BuildDefaultHeaders(ClientHeaderConfig{})

	if got := headers["User-Agent"]; got != "claude-cli/"+DefaultCLIVersion+" (external, cli)" {
		t.Fatalf("unexpected user-agent: %q", got)
	}
	if got := headers["X-Stainless-Package-Version"]; got != DefaultStainlessPackageVersion {
		t.Fatalf("unexpected x-stainless-package-version: %q", got)
	}
}

func TestBuildDefaultHeaders_AllowsOverrides(t *testing.T) {
	headers := BuildDefaultHeaders(ClientHeaderConfig{
		CLIVersion:              "9.9.9",
		StainlessPackageVersion: "1.2.3",
	})

	if got := headers["User-Agent"]; got != "claude-cli/9.9.9 (external, cli)" {
		t.Fatalf("unexpected overridden user-agent: %q", got)
	}
	if got := headers["X-Stainless-Package-Version"]; got != "1.2.3" {
		t.Fatalf("unexpected overridden x-stainless-package-version: %q", got)
	}
}
