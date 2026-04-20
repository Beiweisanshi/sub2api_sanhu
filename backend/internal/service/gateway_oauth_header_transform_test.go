package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHeaderRewriteReq(h http.Header) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	for k, vs := range h {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return req
}

func TestApplyClaudeCodeHeaderRewrites_StripsRemoteAndRegeneratesID(t *testing.T) {
	h := http.Header{}
	h.Set("X-Claude-Remote-Container-Id", "container-123")
	h.Set("X-Claude-Remote-Session-Id", "sess-abc")
	h.Set("x-client-request-id", "real-client-uuid")
	req := newHeaderRewriteReq(h)

	fw := defaultGatewayForwardingSettings()
	fw.RemoteHeaderStrip = true
	applyClaudeCodeHeaderRewrites(req, fw)

	assert.Empty(t, req.Header.Get("X-Claude-Remote-Container-Id"))
	assert.Empty(t, req.Header.Get("X-Claude-Remote-Session-Id"))

	got := getHeaderRaw(req.Header, "x-client-request-id")
	require.NotEqual(t, "real-client-uuid", got, "request id should be regenerated")
	require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, got)
}

func TestApplyClaudeCodeHeaderRewrites_Disabled_PassesThrough(t *testing.T) {
	h := http.Header{}
	h.Set("X-Claude-Remote-Container-Id", "container-123")
	h.Set("x-client-request-id", "keep-me")
	req := newHeaderRewriteReq(h)

	fw := defaultGatewayForwardingSettings()
	fw.RemoteHeaderStrip = false
	applyClaudeCodeHeaderRewrites(req, fw)

	assert.Equal(t, "container-123", req.Header.Get("X-Claude-Remote-Container-Id"))
	assert.Equal(t, "keep-me", getHeaderRaw(req.Header, "x-client-request-id"))
}

func TestApplyClaudeCodeHeaderRewrites_NilReq_Safe(t *testing.T) {
	fw := defaultGatewayForwardingSettings()
	fw.RemoteHeaderStrip = true
	// Must not panic.
	applyClaudeCodeHeaderRewrites(nil, fw)
}

func TestApplyClaudeCodeHeaderRewrites_RegeneratesUUIDEachCall(t *testing.T) {
	fw := defaultGatewayForwardingSettings()
	fw.RemoteHeaderStrip = true

	req1 := newHeaderRewriteReq(http.Header{})
	req2 := newHeaderRewriteReq(http.Header{})
	applyClaudeCodeHeaderRewrites(req1, fw)
	applyClaudeCodeHeaderRewrites(req2, fw)

	a := getHeaderRaw(req1.Header, "x-client-request-id")
	b := getHeaderRaw(req2.Header, "x-client-request-id")
	require.NotEmpty(t, a)
	require.NotEmpty(t, b)
	require.NotEqual(t, a, b, "UUIDs must differ across calls")
}
