package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func newHeartbeatSvcForTest(enabled bool, interval, ttl time.Duration) *TelemetryHeartbeatService {
	cfg := &config.Config{}
	cfg.Telemetry.Heartbeat = config.TelemetryHeartbeatConfig{
		Enabled:         enabled,
		IntervalSeconds: int(interval / time.Second),
		TTLSeconds:      int(ttl / time.Second),
	}
	return &TelemetryHeartbeatService{
		cfg:      cfg,
		sessions: make(map[string]*heartbeatSession),
	}
}

func TestTelemetryHeartbeat_DisabledTouchNoop(t *testing.T) {
	svc := newHeartbeatSvcForTest(false, 10*time.Second, 10*time.Minute)
	svc.Touch("sess-A", 1)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.sessions) != 0 {
		t.Fatalf("expected no sessions when disabled, got %d", len(svc.sessions))
	}
}

func TestTelemetryHeartbeat_CaptureSampleStoresCopy(t *testing.T) {
	svc := newHeartbeatSvcForTest(true, 10*time.Second, 10*time.Minute)
	body := []byte(`{"events":[]}`)
	svc.CaptureSample(42, body, "application/json", "/api/event_logging/batch")

	// 修改原 body 不应影响缓存
	body[0] = 'X'

	raw, ok := svc.samples.Load(int64(42))
	if !ok {
		t.Fatal("sample not stored")
	}
	sample := raw.(*heartbeatSample)
	if sample.body[0] != '{' {
		t.Errorf("expected deep-copied body to start with '{', got %q", sample.body[0])
	}
	if sample.contentType != "application/json" {
		t.Errorf("unexpected content type %q", sample.contentType)
	}
	if sample.path != "/api/event_logging/batch" {
		t.Errorf("unexpected path %q", sample.path)
	}
}

func TestTelemetryHeartbeat_TouchAccountSwitchReplacesSession(t *testing.T) {
	svc := newHeartbeatSvcForTest(true, 10*time.Second, 10*time.Minute)
	svc.Touch("sess-B", 100)
	svc.mu.Lock()
	sess1, ok := svc.sessions["sess-B"]
	svc.mu.Unlock()
	if !ok || sess1.accountID != 100 {
		t.Fatalf("expected session with accountID=100, got %+v", sess1)
	}

	// 切换到另一账号
	svc.Touch("sess-B", 200)
	svc.mu.Lock()
	sess2, ok := svc.sessions["sess-B"]
	svc.mu.Unlock()
	if !ok || sess2.accountID != 200 {
		t.Fatalf("expected session with accountID=200 after switch, got %+v", sess2)
	}

	// 清理
	svc.Stop(context.Background())
}

func TestTelemetryHeartbeat_StopIsIdempotent(t *testing.T) {
	svc := newHeartbeatSvcForTest(true, 10*time.Second, 10*time.Minute)
	svc.Touch("sess-X", 1)
	svc.Stop(context.Background())
	// 第二次 Stop 不应 panic
	svc.Stop(context.Background())
	// Stop 后的 Touch 应被忽略
	svc.Touch("sess-Y", 2)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.sessions) != 0 {
		t.Errorf("expected no active sessions after Stop, got %d", len(svc.sessions))
	}
}
