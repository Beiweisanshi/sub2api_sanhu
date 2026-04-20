package service

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// TelemetryHeartbeatService 对活跃 session 的绑定账号周期性重放最近一次的 event_logging/batch。
// 目的：模拟真实 Claude CLI 的长连接心跳行为，降低 "按请求单点触发"
// 的流量特征被识别为代理流量的概率。
//
// 触发：Touch(sessionKey, accountID) 在 /v1/messages 成功响应后调用。
// 重放：goroutine 每 IntervalSeconds 秒重放一次；若 TTLSeconds 内未再 Touch，则退出。
// 样本：来自 telemetry_handler 捕获到的最近一次真实请求 body（按 accountID 存）。
// 若无样本则跳过该次心跳（不发送合成流量）。
type TelemetryHeartbeatService struct {
	cfg            *config.Config
	rewriter       *TelemetryRewriterService
	tokenProvider  *ClaudeTokenProvider
	accountRepo    AccountRepository
	httpUpstream   HTTPUpstream
	settingService *SettingService

	mu       sync.Mutex
	sessions map[string]*heartbeatSession
	samples  sync.Map // key: accountID(int64) -> *heartbeatSample
	stopped  atomic.Bool
	wg       sync.WaitGroup
}

type heartbeatSession struct {
	sessionKey string
	accountID  int64
	lastTouch  atomic.Int64 // unix seconds
	cancel     context.CancelFunc
}

type heartbeatSample struct {
	body        []byte
	contentType string
	path        string
	capturedAt  time.Time
}

// NewTelemetryHeartbeatService 构造心跳服务。可 Start() 启动后台清理，Stop() 停止全部心跳。
// settingService 可选：传入后心跳会同时尊重运行时开关
// SettingKeyEnableTelemetryHeartbeat（默认 false），方便管理员热切换而无需重启。
func NewTelemetryHeartbeatService(
	cfg *config.Config,
	rewriter *TelemetryRewriterService,
	tokenProvider *ClaudeTokenProvider,
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
) *TelemetryHeartbeatService {
	return &TelemetryHeartbeatService{
		cfg:            cfg,
		rewriter:       rewriter,
		tokenProvider:  tokenProvider,
		accountRepo:    accountRepo,
		httpUpstream:   httpUpstream,
		settingService: settingService,
		sessions:       make(map[string]*heartbeatSession),
	}
}

// runtimeEnabled combines the config-level capability flag with the runtime
// admin toggle. Returns false if either is off. Safe on nil settingService —
// callers such as tests may inject the struct directly without a settings
// backend, in which case only the config flag applies.
func (s *TelemetryHeartbeatService) runtimeEnabled(ctx context.Context) bool {
	if s == nil || s.cfg == nil || !s.cfg.Telemetry.Heartbeat.Enabled {
		return false
	}
	if s.settingService == nil {
		return true
	}
	return s.settingService.GetGatewayForwardingSettings(ctx).TelemetryHeartbeat
}

// heartbeatConfig 返回生效的心跳参数。enabled=false 时调用方应跳过。
func (s *TelemetryHeartbeatService) heartbeatConfig() (enabled bool, interval, ttl time.Duration) {
	if s == nil || s.cfg == nil {
		return false, 0, 0
	}
	h := s.cfg.Telemetry.Heartbeat
	if !h.Enabled {
		return false, 0, 0
	}
	interval = time.Duration(h.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ttl = time.Duration(h.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return true, interval, ttl
}

// CaptureSample 由 telemetry_handler 在成功代理 event_logging/batch 时调用，
// 记录该账号最近一次真实 body，用于后续心跳重放。
func (s *TelemetryHeartbeatService) CaptureSample(accountID int64, body []byte, contentType, path string) {
	if s == nil || accountID <= 0 || len(body) == 0 {
		return
	}
	// 深拷贝，避免 handler 后续复用 buffer 导致数据竞争
	copied := make([]byte, len(body))
	copy(copied, body)
	s.samples.Store(accountID, &heartbeatSample{
		body:        copied,
		contentType: contentType,
		path:        path,
		capturedAt:  time.Now(),
	})
}

// Touch 在 /v1/messages 成功响应后调用：若无心跳则启动，若已存在则刷新 lastTouch。
// sessionKey 来自 gateway 的粘性会话 hash；accountID 为本次绑定的账号。
func (s *TelemetryHeartbeatService) Touch(sessionKey string, accountID int64) {
	if s == nil || accountID <= 0 || strings.TrimSpace(sessionKey) == "" {
		return
	}
	enabled, _, _ := s.heartbeatConfig()
	if !enabled {
		return
	}
	if !s.runtimeEnabled(context.Background()) {
		return
	}
	if s.stopped.Load() {
		return
	}

	now := time.Now().Unix()

	s.mu.Lock()
	existing, ok := s.sessions[sessionKey]
	if ok {
		existing.lastTouch.Store(now)
		// 若绑定账号切换了，重启 session 以携带新 accountID
		if existing.accountID != accountID {
			existing.cancel()
			delete(s.sessions, sessionKey)
			ok = false
		}
	}
	if !ok {
		ctx, cancel := context.WithCancel(context.Background())
		sess := &heartbeatSession{
			sessionKey: sessionKey,
			accountID:  accountID,
			cancel:     cancel,
		}
		sess.lastTouch.Store(now)
		s.sessions[sessionKey] = sess
		s.wg.Add(1)
		go s.runSession(ctx, sess)
	}
	s.mu.Unlock()
}

// Stop 停止所有活跃心跳并等待 goroutine 退出。
func (s *TelemetryHeartbeatService) Stop(ctx context.Context) {
	if s == nil {
		return
	}
	if !s.stopped.CompareAndSwap(false, true) {
		return
	}
	s.mu.Lock()
	for _, sess := range s.sessions {
		sess.cancel()
	}
	s.sessions = map[string]*heartbeatSession{}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// runSession 单个 session 的心跳循环。
func (s *TelemetryHeartbeatService) runSession(ctx context.Context, sess *heartbeatSession) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		if cur, ok := s.sessions[sess.sessionKey]; ok && cur == sess {
			delete(s.sessions, sess.sessionKey)
		}
		s.mu.Unlock()
	}()

	_, interval, ttl := s.heartbeatConfig()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// TTL 超出 → 退出
			last := time.Unix(sess.lastTouch.Load(), 0)
			if time.Since(last) > ttl {
				slog.Info("telemetry_heartbeat_ttl_expired",
					"session_key", sess.sessionKey,
					"account_id", sess.accountID,
					"idle", time.Since(last).Truncate(time.Second),
				)
				return
			}
			s.sendOnce(ctx, sess)
		}
	}
}

// sendOnce 发送一次心跳，使用 samples 中缓存的真实 body 作为模板。
// 每次 tick 都重新检查运行时开关，让 /admin/settings 上的热切换立即生效。
func (s *TelemetryHeartbeatService) sendOnce(ctx context.Context, sess *heartbeatSession) {
	if !s.runtimeEnabled(ctx) {
		return
	}
	raw, ok := s.samples.Load(sess.accountID)
	if !ok {
		return // 无样本 → 不发合成流量
	}
	sample, _ := raw.(*heartbeatSample)
	if sample == nil || len(sample.body) == 0 {
		return
	}

	account, err := s.accountRepo.GetByID(ctx, sess.accountID)
	if err != nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}

	identity := CanonicalTelemetryIdentityForAccount(account, s.cfg.Telemetry.Identity)
	body, err := s.rewriter.RewriteEventBatchWithIdentity(sample.body, identity)
	if err != nil {
		slog.Warn("telemetry_heartbeat_rewrite_failed", "account_id", sess.accountID, "error", err)
		return
	}

	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return
	}

	upstreamURL := strings.TrimRight(s.cfg.Telemetry.UpstreamURL, "/") + sample.path
	if upstreamURL == sample.path {
		upstreamURL = "https://api.anthropic.com" + sample.path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	ct := sample.contentType
	if ct == "" {
		ct = "application/json"
	}
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+token)
	if version := strings.TrimSpace(s.cfg.Telemetry.CanonicalEnv.Version); version != "" {
		req.Header.Set("User-Agent", CanonicalClaudeCLIUserAgent(version))
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		slog.Warn("telemetry_heartbeat_upstream_failed", "account_id", sess.accountID, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		slog.Warn("telemetry_heartbeat_upstream_non2xx",
			"account_id", sess.accountID,
			"status", resp.StatusCode,
		)
	}
}
