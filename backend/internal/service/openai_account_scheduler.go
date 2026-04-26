package service

import (
	"container/heap"
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	openAIAccountScheduleLayerPreviousResponse = "previous_response_id"
	openAIAccountScheduleLayerSessionSticky    = "session_hash"
	openAIAccountScheduleLayerLoadBalance      = "load_balance"
	openAIAdvancedSchedulerSettingKey          = "openai_advanced_scheduler_enabled"
)

const (
	openAIAdvancedSchedulerSettingCacheTTL  = 5 * time.Second
	openAIAdvancedSchedulerSettingDBTimeout = 2 * time.Second
)

type cachedOpenAIAdvancedSchedulerSetting struct {
	enabled   bool
	expiresAt int64
}

var openAIAdvancedSchedulerSettingCache atomic.Value // *cachedOpenAIAdvancedSchedulerSetting
var openAIAdvancedSchedulerSettingSF singleflight.Group

// 作者: mkx  变更: 2026/04/23 - 新增 PreferredImageCapability 字段
// 用途: 把 capability 从"硬门槛"改造成"组内偏好"。RequiredImageCapability 仍
// 作为候选池过滤下限（Basic 通过 OAuth+apikey 都可选）；PreferredImageCapability
// 作为同优先级组内的排序偏好（支持该能力的账号优先，比如 apikey 在前、OAuth 降级兜底）。
// 只有同组 apikey 全部 fresh/recheck 失败时，才在同组降级到 OAuth，而不是跨优先级找更低 priority 的 apikey。
type OpenAIAccountScheduleRequest struct {
	GroupID                   *int64
	SessionHash               string
	StickyAccountID           int64
	PreviousResponseID        string
	RequestedModel            string
	RequiredTransport         OpenAIUpstreamTransport
	RequiredImageCapability   OpenAIImagesCapability
	PreferredImageCapability  OpenAIImagesCapability
	RequireCompact            bool
	ExcludeFullAccountsNoWait bool
	ExcludedIDs               map[int64]struct{}
}

type OpenAIAccountScheduleDecision struct {
	Layer               string
	StickyPreviousHit   bool
	StickySessionHit    bool
	CandidateCount      int
	TopK                int
	LatencyMs           int64
	LoadSkew            float64
	SelectedAccountID   int64
	SelectedAccountType string
}

type OpenAIAccountSchedulerMetricsSnapshot struct {
	SelectTotal              int64
	StickyPreviousHitTotal   int64
	StickySessionHitTotal    int64
	LoadBalanceSelectTotal   int64
	AccountSwitchTotal       int64
	SchedulerLatencyMsTotal  int64
	SchedulerLatencyMsAvg    float64
	StickyHitRatio           float64
	AccountSwitchRate        float64
	LoadSkewAvg              float64
	RuntimeStatsAccountCount int
}

type OpenAIAccountScheduler interface {
	Select(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)
	ReportResult(accountID int64, success bool, firstTokenMs *int)
	ReportSwitch()
	SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot
}

type openAIAccountSchedulerMetrics struct {
	selectTotal            atomic.Int64
	stickyPreviousHitTotal atomic.Int64
	stickySessionHitTotal  atomic.Int64
	loadBalanceSelectTotal atomic.Int64
	accountSwitchTotal     atomic.Int64
	latencyMsTotal         atomic.Int64
	loadSkewMilliTotal     atomic.Int64
}

func (m *openAIAccountSchedulerMetrics) recordSelect(decision OpenAIAccountScheduleDecision) {
	if m == nil {
		return
	}
	m.selectTotal.Add(1)
	m.latencyMsTotal.Add(decision.LatencyMs)
	m.loadSkewMilliTotal.Add(int64(math.Round(decision.LoadSkew * 1000)))
	if decision.StickyPreviousHit {
		m.stickyPreviousHitTotal.Add(1)
	}
	if decision.StickySessionHit {
		m.stickySessionHitTotal.Add(1)
	}
	if decision.Layer == openAIAccountScheduleLayerLoadBalance {
		m.loadBalanceSelectTotal.Add(1)
	}
}

func (m *openAIAccountSchedulerMetrics) recordSwitch() {
	if m == nil {
		return
	}
	m.accountSwitchTotal.Add(1)
}

type openAIAccountRuntimeStats struct {
	accounts     sync.Map
	accountCount atomic.Int64
}

type openAIAccountRuntimeStat struct {
	errorRateEWMABits atomic.Uint64
	ttftEWMABits      atomic.Uint64
}

func newOpenAIAccountRuntimeStats() *openAIAccountRuntimeStats {
	return &openAIAccountRuntimeStats{}
}

func (s *openAIAccountRuntimeStats) loadOrCreate(accountID int64) *openAIAccountRuntimeStat {
	if value, ok := s.accounts.Load(accountID); ok {
		stat, _ := value.(*openAIAccountRuntimeStat)
		if stat != nil {
			return stat
		}
	}

	stat := &openAIAccountRuntimeStat{}
	stat.ttftEWMABits.Store(math.Float64bits(math.NaN()))
	actual, loaded := s.accounts.LoadOrStore(accountID, stat)
	if !loaded {
		s.accountCount.Add(1)
		return stat
	}
	existing, _ := actual.(*openAIAccountRuntimeStat)
	if existing != nil {
		return existing
	}
	return stat
}

func updateEWMAAtomic(target *atomic.Uint64, sample float64, alpha float64) {
	for {
		oldBits := target.Load()
		oldValue := math.Float64frombits(oldBits)
		newValue := alpha*sample + (1-alpha)*oldValue
		if target.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
			return
		}
	}
}

func (s *openAIAccountRuntimeStats) report(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || accountID <= 0 {
		return
	}
	const alpha = 0.2
	stat := s.loadOrCreate(accountID)

	errorSample := 1.0
	if success {
		errorSample = 0.0
	}
	updateEWMAAtomic(&stat.errorRateEWMABits, errorSample, alpha)

	if firstTokenMs != nil && *firstTokenMs > 0 {
		ttft := float64(*firstTokenMs)
		ttftBits := math.Float64bits(ttft)
		for {
			oldBits := stat.ttftEWMABits.Load()
			oldValue := math.Float64frombits(oldBits)
			if math.IsNaN(oldValue) {
				if stat.ttftEWMABits.CompareAndSwap(oldBits, ttftBits) {
					break
				}
				continue
			}
			newValue := alpha*ttft + (1-alpha)*oldValue
			if stat.ttftEWMABits.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
				break
			}
		}
	}
}

func (s *openAIAccountRuntimeStats) snapshot(accountID int64) (errorRate float64, ttft float64, hasTTFT bool) {
	if s == nil || accountID <= 0 {
		return 0, 0, false
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false
	}
	stat, _ := value.(*openAIAccountRuntimeStat)
	if stat == nil {
		return 0, 0, false
	}
	errorRate = clamp01(math.Float64frombits(stat.errorRateEWMABits.Load()))
	ttftValue := math.Float64frombits(stat.ttftEWMABits.Load())
	if math.IsNaN(ttftValue) {
		return errorRate, 0, false
	}
	return errorRate, ttftValue, true
}

func (s *openAIAccountRuntimeStats) size() int {
	if s == nil {
		return 0
	}
	return int(s.accountCount.Load())
}

type defaultOpenAIAccountScheduler struct {
	service *OpenAIGatewayService
	metrics openAIAccountSchedulerMetrics
	stats   *openAIAccountRuntimeStats
}

func newDefaultOpenAIAccountScheduler(service *OpenAIGatewayService, stats *openAIAccountRuntimeStats) OpenAIAccountScheduler {
	if stats == nil {
		stats = newOpenAIAccountRuntimeStats()
	}
	return &defaultOpenAIAccountScheduler{
		service: service,
		stats:   stats,
	}
}

func (s *defaultOpenAIAccountScheduler) Select(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{}
	start := time.Now()
	defer func() {
		decision.LatencyMs = time.Since(start).Milliseconds()
		s.metrics.recordSelect(decision)
	}()

	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID != "" {
		selection, err := s.service.SelectAccountByPreviousResponseID(
			ctx,
			req.GroupID,
			previousResponseID,
			req.RequestedModel,
			req.ExcludedIDs,
			req.RequireCompact,
		)
		if err != nil {
			return nil, decision, err
		}
		if selection != nil && selection.Account != nil {
			if !s.isAccountTransportCompatible(selection.Account, req.RequiredTransport) {
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				selection = nil
			}
		}
		if selection != nil && selection.Account != nil {
			decision.Layer = openAIAccountScheduleLayerPreviousResponse
			decision.StickyPreviousHit = true
			decision.SelectedAccountID = selection.Account.ID
			decision.SelectedAccountType = selection.Account.Type
			if req.SessionHash != "" {
				_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, selection.Account.ID)
			}
			return selection, decision, nil
		}
	}

	selection, err := s.selectBySessionHash(ctx, req)
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.Layer = openAIAccountScheduleLayerSessionSticky
		decision.StickySessionHit = true
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
		return selection, decision, nil
	}

	selection, candidateCount, topK, loadSkew, err := s.selectByLoadBalance(ctx, req)
	decision.Layer = openAIAccountScheduleLayerLoadBalance
	decision.CandidateCount = candidateCount
	decision.TopK = topK
	decision.LoadSkew = loadSkew
	if err != nil {
		return nil, decision, err
	}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
	}
	return selection, decision, nil
}

func (s *defaultOpenAIAccountScheduler) selectBySessionHash(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, error) {
	sessionHash := strings.TrimSpace(req.SessionHash)
	if sessionHash == "" || s == nil || s.service == nil || s.service.cache == nil {
		return nil, nil
	}

	accountID := req.StickyAccountID
	if accountID <= 0 {
		var err error
		accountID, err = s.service.getStickySessionAccountID(ctx, req.GroupID, sessionHash)
		if err != nil || accountID <= 0 {
			return nil, nil
		}
	}
	if accountID <= 0 {
		return nil, nil
	}
	if req.ExcludedIDs != nil {
		if _, excluded := req.ExcludedIDs[accountID]; excluded {
			return nil, nil
		}
	}

	account, err := s.service.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	if shouldClearStickySession(account, req.RequestedModel) || !account.IsOpenAI() || !account.IsSchedulable() {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	if !s.isAccountRequestCompatible(account, req) {
		return nil, nil
	}
	if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, nil
	}
	account = s.service.recheckSelectedOpenAIAccountFromDB(ctx, account, req.RequestedModel, req.RequireCompact)
	if account == nil || !s.isAccountTransportCompatible(account, req.RequiredTransport) {
		_ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
		return nil, nil
	}

	result, acquireErr := s.service.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
	if acquireErr == nil && result.Acquired {
		_ = s.service.refreshStickySessionTTL(ctx, req.GroupID, sessionHash, s.service.openAIWSSessionStickyTTL())
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}, nil
	}

	cfg := s.service.schedulingConfig()
	// WaitPlan.MaxConcurrency 使用 Concurrency（非 EffectiveLoadFactor），因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
	if s.service.concurrencyService != nil {
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      accountID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.StickySessionWaitTimeout,
				MaxWaiting:     cfg.StickySessionMaxWaiting,
			},
		}, nil
	}
	return nil, nil
}

// openAIAccountCandidate 调度候选账号，weight 为按比例随机选择的权重。
type openAIAccountCandidate struct {
	account   *Account
	loadInfo  *AccountLoadInfo
	weight    float64
	score     float64
	errorRate float64
	ttft      float64
	hasTTFT   bool
}

type openAIAccountCandidateHeap []openAIAccountCandidate

func (h openAIAccountCandidateHeap) Len() int {
	return len(h)
}

func (h openAIAccountCandidateHeap) Less(i, j int) bool {
	// 最小堆根节点保存“最差”候选，便于 O(log k) 维护 topK。
	return isOpenAIAccountCandidateBetter(h[j], h[i])
}

func (h openAIAccountCandidateHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *openAIAccountCandidateHeap) Push(x any) {
	candidate, ok := x.(openAIAccountCandidate)
	if !ok {
		panic("openAIAccountCandidateHeap: invalid element type")
	}
	*h = append(*h, candidate)
}

func (h *openAIAccountCandidateHeap) Pop() any {
	old := *h
	n := len(old)
	last := old[n-1]
	*h = old[:n-1]
	return last
}

func isOpenAIAccountCandidateBetter(left openAIAccountCandidate, right openAIAccountCandidate) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	if left.account.Priority != right.account.Priority {
		return left.account.Priority < right.account.Priority
	}
	if left.loadInfo.LoadRate != right.loadInfo.LoadRate {
		return left.loadInfo.LoadRate < right.loadInfo.LoadRate
	}
	if left.loadInfo.WaitingCount != right.loadInfo.WaitingCount {
		return left.loadInfo.WaitingCount < right.loadInfo.WaitingCount
	}
	return left.account.ID < right.account.ID
}

func selectTopKOpenAICandidates(candidates []openAIAccountCandidate, topK int) []openAIAccountCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 1
	}
	if topK >= len(candidates) {
		ranked := append([]openAIAccountCandidate(nil), candidates...)
		sort.Slice(ranked, func(i, j int) bool {
			return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
		})
		return ranked
	}

	best := make(openAIAccountCandidateHeap, 0, topK)
	for _, candidate := range candidates {
		if len(best) < topK {
			heap.Push(&best, candidate)
			continue
		}
		if isOpenAIAccountCandidateBetter(candidate, best[0]) {
			best[0] = candidate
			heap.Fix(&best, 0)
		}
	}

	ranked := make([]openAIAccountCandidate, len(best))
	copy(ranked, best)
	sort.Slice(ranked, func(i, j int) bool {
		return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
	})
	return ranked
}

type openAISelectionRNG struct {
	state uint64
}

func newOpenAISelectionRNG(seed uint64) openAISelectionRNG {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return openAISelectionRNG{state: seed}
}

func (r *openAISelectionRNG) nextUint64() uint64 {
	// xorshift64*
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 2685821657736338717
}

func (r *openAISelectionRNG) nextFloat64() float64 {
	// [0,1)
	return float64(r.nextUint64()>>11) / (1 << 53)
}

func deriveOpenAISelectionSeed(req OpenAIAccountScheduleRequest) uint64 {
	hasher := fnv.New64a()
	writeValue := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		_, _ = hasher.Write([]byte(trimmed))
		_, _ = hasher.Write([]byte{0})
	}

	writeValue(req.SessionHash)
	writeValue(req.PreviousResponseID)
	writeValue(req.RequestedModel)
	if req.GroupID != nil {
		_, _ = hasher.Write([]byte(strconv.FormatInt(*req.GroupID, 10)))
	}

	seed := hasher.Sum64()
	// 对“无会话锚点”的纯负载均衡请求引入时间熵，避免固定命中同一账号。
	if strings.TrimSpace(req.SessionHash) == "" && strings.TrimSpace(req.PreviousResponseID) == "" {
		seed ^= uint64(time.Now().UnixNano())
	}
	if seed == 0 {
		seed = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
	}
	return seed
}

// buildOpenAIWeightedSelectionOrder 按 weight 比例生成随机排列，weight 无效时回退为 1.0。
func buildOpenAIWeightedSelectionOrder(
	candidates []openAIAccountCandidate,
	req OpenAIAccountScheduleRequest,
) []openAIAccountCandidate {
	if len(candidates) <= 1 {
		return append([]openAIAccountCandidate(nil), candidates...)
	}

	pool := append([]openAIAccountCandidate(nil), candidates...)
	weights := make([]float64, len(pool))
	for i := range pool {
		w := pool[i].weight
		if math.IsNaN(w) || math.IsInf(w, 0) || w <= 0 {
			w = 1.0
		}
		weights[i] = w
	}

	order := make([]openAIAccountCandidate, 0, len(pool))
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
	for len(pool) > 0 {
		total := 0.0
		for _, w := range weights {
			total += w
		}

		selectedIdx := 0
		if total > 0 {
			r := rng.nextFloat64() * total
			acc := 0.0
			for i, w := range weights {
				acc += w
				if r <= acc {
					selectedIdx = i
					break
				}
			}
		} else {
			selectedIdx = int(rng.nextUint64() % uint64(len(pool)))
		}

		order = append(order, pool[selectedIdx])
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
		weights = append(weights[:selectedIdx], weights[selectedIdx+1:]...)
	}
	return order
}

// buildOpenAISelectionOrderWithPreference
// 作者: mkx  变更: 2026/04/23
// 在同优先级组内，把 PreferredImageCapability 当作"二级排序键"：
//   - 支持 preferred 的账号（如 apikey 支持 native）放到前面，组成 tier-A
//   - 不支持 preferred 但满足 Required 的账号（如 oauth 只支持 basic）放到后面，组成 tier-B
//
// 每个 tier 内部仍使用权重随机；两个 tier 按 A→B 顺序拼接。
// 当 req.PreferredImageCapability 为空或全组都支持/都不支持时，退化为单层权重随机。
func buildOpenAISelectionOrderWithPreference(
	candidates []openAIAccountCandidate,
	req OpenAIAccountScheduleRequest,
) []openAIAccountCandidate {
	if len(candidates) <= 1 || req.PreferredImageCapability == "" {
		return buildOpenAIWeightedSelectionOrder(candidates, req)
	}

	preferred := make([]openAIAccountCandidate, 0, len(candidates))
	rest := make([]openAIAccountCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.account != nil && c.account.SupportsOpenAIImageCapability(req.PreferredImageCapability) {
			preferred = append(preferred, c)
		} else {
			rest = append(rest, c)
		}
	}
	if len(preferred) == 0 || len(rest) == 0 {
		return buildOpenAIWeightedSelectionOrder(candidates, req)
	}

	order := buildOpenAIWeightedSelectionOrder(preferred, req)
	return append(order, buildOpenAIWeightedSelectionOrder(rest, req)...)
}

func (s *defaultOpenAIAccountScheduler) selectByLoadBalance(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
) (*AccountSelectionResult, int, int, float64, error) {
	accounts, err := s.service.listSchedulableAccounts(ctx, req.GroupID)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if len(accounts) == 0 {
		return nil, 0, 0, 0, noAvailableOpenAISelectionError(req.RequestedModel, false)
	}

	var schedGroup *Group
	if req.GroupID != nil && s.service.schedulerSnapshot != nil {
		schedGroup, _ = s.service.schedulerSnapshot.GetGroupByID(ctx, *req.GroupID)
	}

	type pendingAccount struct {
		acc *Account
		lc  AccountWithConcurrency
	}
	type priorityGroup struct {
		priority   int
		candidates []openAIAccountCandidate
	}

	// 过滤出全部合法账号，批量获取负载信息
	pending := make([]pendingAccount, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if req.ExcludedIDs != nil {
			if _, excluded := req.ExcludedIDs[account.ID]; excluded {
				continue
			}
		}
		if !account.IsSchedulable() || !account.IsOpenAI() {
			continue
		}
		if schedGroup != nil && schedGroup.RequirePrivacySet && !account.IsPrivacySet() {
			_ = s.service.accountRepo.SetError(ctx, account.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			continue
		}
		if !s.isAccountRequestCompatible(account, req) {
			continue
		}
		if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
			continue
		}
		pending = append(pending, pendingAccount{
			acc: account,
			lc:  AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.EffectiveLoadFactor()},
		})
	}
	if len(pending) == 0 {
		return nil, 0, 0, 0, noAvailableOpenAISelectionError(req.RequestedModel, false)
	}

	loadReq := make([]AccountWithConcurrency, len(pending))
	for i, p := range pending {
		loadReq[i] = p.lc
	}
	loadMap := map[int64]*AccountLoadInfo{}
	if s.service.concurrencyService != nil {
		if batchLoad, loadErr := s.service.concurrencyService.GetAccountsLoadBatch(ctx, loadReq); loadErr == nil {
			loadMap = batchLoad
		}
	}

	allCandidates := make([]openAIAccountCandidate, 0, len(pending))
	for _, item := range pending {
		loadInfo := loadMap[item.acc.ID]
		if loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: item.acc.ID}
		}
		errorRate, ttft, hasTTFT := s.stats.snapshot(item.acc.ID)
		allCandidates = append(allCandidates, openAIAccountCandidate{
			account:   item.acc,
			loadInfo:  loadInfo,
			errorRate: errorRate,
			ttft:      ttft,
			hasTTFT:   hasTTFT,
		})
	}

	// Compact 模式下把明确不支持 compact 的账号拆出，仅在 schedulerSnapshot 启用
	// 时作为最后兜底重试（snapshot 可能已陈旧）。
	candidates := allCandidates
	staleSnapshotCompactRetry := make([]openAIAccountCandidate, 0, len(allCandidates))
	compactBlocked := false
	if req.RequireCompact {
		candidates = make([]openAIAccountCandidate, 0, len(allCandidates))
		for _, candidate := range allCandidates {
			if openAICompactSupportTier(candidate.account) == 0 {
				compactBlocked = true
				if s.service.schedulerSnapshot != nil {
					staleSnapshotCompactRetry = append(staleSnapshotCompactRetry, candidate)
				}
				continue
			}
			candidates = append(candidates, candidate)
		}
		if len(candidates) == 0 && len(staleSnapshotCompactRetry) == 0 {
			return nil, 0, 0, 0, ErrNoAvailableCompactAccounts
		}
	}

	candidateCount := len(candidates)
	scoreCandidates := func(pool []openAIAccountCandidate) ([]openAIAccountCandidate, float64) {
		if len(pool) == 0 {
			return nil, 0
		}
		scored := append([]openAIAccountCandidate(nil), pool...)
		minPriority, maxPriority := scored[0].account.Priority, scored[0].account.Priority
		maxWaiting := 1
		loadRateSum := 0.0
		loadRateSumSquares := 0.0
		minTTFT, maxTTFT := 0.0, 0.0
		hasTTFTSample := false
		for _, candidate := range scored {
			if candidate.account.Priority < minPriority {
				minPriority = candidate.account.Priority
			}
			if candidate.account.Priority > maxPriority {
				maxPriority = candidate.account.Priority
			}
			if candidate.loadInfo.WaitingCount > maxWaiting {
				maxWaiting = candidate.loadInfo.WaitingCount
			}
			if candidate.hasTTFT && candidate.ttft > 0 {
				if !hasTTFTSample {
					minTTFT, maxTTFT = candidate.ttft, candidate.ttft
					hasTTFTSample = true
				} else {
					if candidate.ttft < minTTFT {
						minTTFT = candidate.ttft
					}
					if candidate.ttft > maxTTFT {
						maxTTFT = candidate.ttft
					}
				}
			}
			loadRate := float64(candidate.loadInfo.LoadRate)
			loadRateSum += loadRate
			loadRateSumSquares += loadRate * loadRate
		}
		loadSkew := calcLoadSkewByMoments(loadRateSum, loadRateSumSquares, len(scored))

		weights := s.service.openAIWSSchedulerWeights()
		for i := range scored {
			item := &scored[i]
			priorityFactor := 1.0
			if maxPriority > minPriority {
				priorityFactor = 1 - float64(item.account.Priority-minPriority)/float64(maxPriority-minPriority)
			}
			loadFactor := 1 - clamp01(float64(item.loadInfo.LoadRate)/100.0)
			queueFactor := 1 - clamp01(float64(item.loadInfo.WaitingCount)/float64(maxWaiting))
			errorFactor := 1 - clamp01(item.errorRate)
			ttftFactor := 0.5
			if item.hasTTFT && hasTTFTSample && maxTTFT > minTTFT {
				ttftFactor = 1 - clamp01((item.ttft-minTTFT)/(maxTTFT-minTTFT))
			}

			item.score = weights.Priority*priorityFactor +
				weights.Load*loadFactor +
				weights.Queue*queueFactor +
				weights.ErrorRate*errorFactor +
				weights.TTFT*ttftFactor
		}
		return scored, loadSkew
	}
	candidates, loadSkew := scoreCandidates(candidates)
	staleSnapshotCompactRetry, _ = scoreCandidates(staleSnapshotCompactRetry)

	topK := 0
	if len(candidates) > 0 {
		topK = s.service.openAIWSLBTopK()
		if topK > len(candidates) {
			topK = len(candidates)
		}
		if topK <= 0 {
			topK = 1
		}
	}

	applyScoreWeights := func(pool []openAIAccountCandidate) []openAIAccountCandidate {
		if len(pool) == 0 {
			return nil
		}
		weighted := append([]openAIAccountCandidate(nil), pool...)
		minScore := weighted[0].score
		for i := 1; i < len(weighted); i++ {
			if weighted[i].score < minScore {
				minScore = weighted[i].score
			}
		}
		for i := range weighted {
			weighted[i].weight = (weighted[i].score - minScore) + 1.0
		}
		return weighted
	}
	buildWeightedSelectionOrder := func(pool []openAIAccountCandidate) []openAIAccountCandidate {
		if len(pool) == 0 {
			return nil
		}
		groupTopK := topK
		if groupTopK <= 0 {
			groupTopK = len(pool)
		}
		if groupTopK > len(pool) {
			groupTopK = len(pool)
		}
		ranked := selectTopKOpenAICandidates(pool, groupTopK)
		return buildOpenAIWeightedSelectionOrder(applyScoreWeights(ranked), req)
	}
	buildPreferredSelectionOrder := func(pool []openAIAccountCandidate) []openAIAccountCandidate {
		if len(pool) <= 1 || req.PreferredImageCapability == "" {
			return buildWeightedSelectionOrder(pool)
		}
		preferred := make([]openAIAccountCandidate, 0, len(pool))
		rest := make([]openAIAccountCandidate, 0, len(pool))
		for _, candidate := range pool {
			if candidate.account != nil && candidate.account.SupportsOpenAIImageCapability(req.PreferredImageCapability) {
				preferred = append(preferred, candidate)
			} else {
				rest = append(rest, candidate)
			}
		}
		if len(preferred) == 0 || len(rest) == 0 {
			return buildWeightedSelectionOrder(pool)
		}
		order := buildWeightedSelectionOrder(preferred)
		return append(order, buildWeightedSelectionOrder(rest)...)
	}
	buildSelectionOrder := func(pool []openAIAccountCandidate) []openAIAccountCandidate {
		if !req.RequireCompact {
			return buildPreferredSelectionOrder(pool)
		}
		supported := make([]openAIAccountCandidate, 0, len(pool))
		unknown := make([]openAIAccountCandidate, 0, len(pool))
		for _, candidate := range pool {
			switch openAICompactSupportTier(candidate.account) {
			case 2:
				supported = append(supported, candidate)
			case 1:
				unknown = append(unknown, candidate)
			}
		}
		order := buildPreferredSelectionOrder(supported)
		return append(order, buildPreferredSelectionOrder(unknown)...)
	}
	buildPriorityGroups := func(pool []openAIAccountCandidate) []priorityGroup {
		if len(pool) == 0 {
			return nil
		}
		groupMap := map[int]*priorityGroup{}
		priOrder := make([]int, 0)
		for _, candidate := range pool {
			priority := candidate.account.Priority
			group, exists := groupMap[priority]
			if !exists {
				group = &priorityGroup{priority: priority}
				groupMap[priority] = group
				priOrder = append(priOrder, priority)
			}
			group.candidates = append(group.candidates, candidate)
		}
		sort.Ints(priOrder)
		groups := make([]priorityGroup, 0, len(priOrder))
		for _, priority := range priOrder {
			groups = append(groups, *groupMap[priority])
		}
		return groups
	}

	// 依次尝试各优先级组，组内按权重概率选择。
	// 默认语义：只要当前组有账号通过双重检查（即使并发槽已满），就在当前组返回 WaitPlan，不再降级。
	// 图片请求会设置 ExcludeFullAccountsNoWait：并发满的账号本轮直接跳过，继续尝试其他账号。
	// 作者: mkx  变更: 2026/04/23 - 组内按 PreferredImageCapability 分层
	// 分层逻辑：若请求带了 preferred capability（如 native），把组内候选拆成
	// [支持 preferred 的] + [仅支持 required 的] 两层，前者先试，后者作为组内降级。
	cfg := s.service.schedulingConfig()
	tryGroups := func(groups []priorityGroup) (*AccountSelectionResult, error) {
		for _, group := range groups {
			selectionOrder := buildSelectionOrder(group.candidates)

			anyTrulySchedulable := false
			var waitPlanCandidate *Account

			for _, candidate := range selectionOrder {
				fresh := s.service.resolveFreshSchedulableOpenAIAccount(ctx, candidate.account, req.RequestedModel, false)
				if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(fresh, req) {
					continue
				}
				fresh = s.service.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.RequestedModel, false)
				if fresh == nil || !s.isAccountTransportCompatible(fresh, req.RequiredTransport) || !s.isAccountRequestCompatible(fresh, req) {
					continue
				}
				if req.RequireCompact && openAICompactSupportTier(fresh) == 0 {
					compactBlocked = true
					continue
				}
				result, acquireErr := s.service.tryAcquireAccountSlot(ctx, fresh.ID, fresh.Concurrency)
				if acquireErr != nil {
					return nil, acquireErr
				}
				if result != nil && result.Acquired {
					if req.SessionHash != "" {
						_ = s.service.BindStickySession(ctx, req.GroupID, req.SessionHash, fresh.ID)
					}
					return &AccountSelectionResult{
						Account:     fresh,
						Acquired:    true,
						ReleaseFunc: result.ReleaseFunc,
					}, nil
				}
				if req.ExcludeFullAccountsNoWait {
					continue
				}
				// 账号通过双重检查但槽位已满，可作为非图片请求的 WaitPlan 候选。
				anyTrulySchedulable = true
				if waitPlanCandidate == nil {
					waitPlanCandidate = fresh
				}
			}

			// 当前组有可调度账号但槽位已满：返回 WaitPlan，不降级到下一优先级组。
			if anyTrulySchedulable && waitPlanCandidate != nil {
				return &AccountSelectionResult{
					Account: waitPlanCandidate,
					WaitPlan: &AccountWaitPlan{
						AccountID:      waitPlanCandidate.ID,
						MaxConcurrency: waitPlanCandidate.Concurrency,
						Timeout:        cfg.FallbackWaitTimeout,
						MaxWaiting:     cfg.FallbackMaxWaiting,
					},
				}, nil
			}
			// 当前组所有账号均不可调度（freshCheck 或 dbRecheck 失败）→ 降级到下一优先级组。
		}
		return nil, nil
	}

	if selection, err := tryGroups(buildPriorityGroups(candidates)); selection != nil || err != nil {
		return selection, candidateCount, topK, loadSkew, err
	}
	if req.RequireCompact && len(staleSnapshotCompactRetry) > 0 {
		if selection, err := tryGroups(buildPriorityGroups(staleSnapshotCompactRetry)); selection != nil || err != nil {
			return selection, candidateCount, topK, loadSkew, err
		}
	}

	return nil, candidateCount, topK, loadSkew, noAvailableOpenAISelectionError(req.RequestedModel, compactBlocked)
}

func (s *defaultOpenAIAccountScheduler) isAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
		return true
	}
	if s == nil || s.service == nil {
		return false
	}
	return s.service.isOpenAIAccountTransportCompatible(account, requiredTransport)
}

func (s *defaultOpenAIAccountScheduler) isAccountRequestCompatible(account *Account, req OpenAIAccountScheduleRequest) bool {
	if account == nil {
		return false
	}
	if req.RequestedModel != "" && !account.IsModelSupported(req.RequestedModel) {
		return false
	}
	return account.SupportsOpenAIImageCapability(req.RequiredImageCapability)
}

func (s *defaultOpenAIAccountScheduler) ReportResult(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || s.stats == nil {
		return
	}
	s.stats.report(accountID, success, firstTokenMs)
}

func (s *defaultOpenAIAccountScheduler) ReportSwitch() {
	if s == nil {
		return
	}
	s.metrics.recordSwitch()
}

func (s *defaultOpenAIAccountScheduler) SnapshotMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	if s == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{}
	}

	selectTotal := s.metrics.selectTotal.Load()
	prevHit := s.metrics.stickyPreviousHitTotal.Load()
	sessionHit := s.metrics.stickySessionHitTotal.Load()
	switchTotal := s.metrics.accountSwitchTotal.Load()
	latencyTotal := s.metrics.latencyMsTotal.Load()
	loadSkewTotal := s.metrics.loadSkewMilliTotal.Load()

	snapshot := OpenAIAccountSchedulerMetricsSnapshot{
		SelectTotal:              selectTotal,
		StickyPreviousHitTotal:   prevHit,
		StickySessionHitTotal:    sessionHit,
		LoadBalanceSelectTotal:   s.metrics.loadBalanceSelectTotal.Load(),
		AccountSwitchTotal:       switchTotal,
		SchedulerLatencyMsTotal:  latencyTotal,
		RuntimeStatsAccountCount: s.stats.size(),
	}
	if selectTotal > 0 {
		snapshot.SchedulerLatencyMsAvg = float64(latencyTotal) / float64(selectTotal)
		snapshot.StickyHitRatio = float64(prevHit+sessionHit) / float64(selectTotal)
		snapshot.AccountSwitchRate = float64(switchTotal) / float64(selectTotal)
		snapshot.LoadSkewAvg = float64(loadSkewTotal) / 1000 / float64(selectTotal)
	}
	return snapshot
}

func (s *OpenAIGatewayService) openAIAdvancedSchedulerSettingRepo() SettingRepository {
	if s == nil || s.rateLimitService == nil || s.rateLimitService.settingService == nil {
		return nil
	}
	return s.rateLimitService.settingService.settingRepo
}

func (s *OpenAIGatewayService) isOpenAIAdvancedSchedulerEnabled(ctx context.Context) bool {
	if cached, ok := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.enabled
		}
	}

	result, _, _ := openAIAdvancedSchedulerSettingSF.Do(openAIAdvancedSchedulerSettingKey, func() (any, error) {
		if cached, ok := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.enabled, nil
			}
		}

		enabled := false
		if repo := s.openAIAdvancedSchedulerSettingRepo(); repo != nil {
			dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAdvancedSchedulerSettingDBTimeout)
			defer cancel()

			value, err := repo.GetValue(dbCtx, openAIAdvancedSchedulerSettingKey)
			if err == nil {
				enabled = strings.EqualFold(strings.TrimSpace(value), "true")
			}
		}

		openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
			enabled:   enabled,
			expiresAt: time.Now().Add(openAIAdvancedSchedulerSettingCacheTTL).UnixNano(),
		})
		return enabled, nil
	})

	enabled, _ := result.(bool)
	return enabled
}

func (s *OpenAIGatewayService) getOpenAIAccountScheduler(ctx context.Context) OpenAIAccountScheduler {
	if s == nil {
		return nil
	}
	// 本地调度算法（按优先级分组+加权随机）为默认调度路径，不依赖上游的 advanced scheduler setting gate
	_ = ctx
	s.openaiSchedulerOnce.Do(func() {
		if s.openaiAccountStats == nil {
			s.openaiAccountStats = newOpenAIAccountRuntimeStats()
		}
		if s.openaiScheduler == nil {
			s.openaiScheduler = newDefaultOpenAIAccountScheduler(s, s.openaiAccountStats)
		}
	})
	return s.openaiScheduler
}

func resetOpenAIAdvancedSchedulerSettingCacheForTest() {
	openAIAdvancedSchedulerSettingCache = atomic.Value{}
	openAIAdvancedSchedulerSettingSF = singleflight.Group{}
}

func (s *OpenAIGatewayService) SelectAccountWithScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requireCompact bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	return s.selectAccountWithScheduler(ctx, groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, "", "", requireCompact)
}

// SelectAccountWithSchedulerForImages
// 作者: mkx  变更: 2026/04/23
// 策略：priority 作为主排序键，capability 降为组内偏好。
//   - 分类器判 native 的请求：下限放宽到 Basic（OAuth+apikey 都进候选池），同优先级组内 apikey 先试，
//     apikey 全部 fresh/recheck 失败才在同组降级 OAuth（以 basic 能力承接，静默丢弃 quality/style 等原生参数）。
//   - 分类器判 basic 的请求：Required=Basic，无偏好，按权重随机即可。
//
// 删除了原本的外层"native 全失败→全量 basic 重调度"兜底——priority 在新逻辑下是硬序，组内已经完成降级。
func (s *OpenAIGatewayService) SelectAccountWithSchedulerForImages(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIImagesCapability,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	// 图片生成请求不使用跨请求粘性会话，避免同 prompt/body 长时间固定到同一账号。
	// 仍允许 handler 在 pool mode 下为"同一次请求内重试"补一次性 session key。
	sessionHash = ""
	required := requiredCapability
	preferred := OpenAIImagesCapability("")
	if requiredCapability == OpenAIImagesCapabilityNative {
		required = OpenAIImagesCapabilityBasic
		preferred = OpenAIImagesCapabilityNative
	}
	return s.selectAccountWithScheduler(ctx, groupID, "", sessionHash, requestedModel, excludedIDs, OpenAIUpstreamTransportHTTPSSE, required, preferred, false)
}

func (s *OpenAIGatewayService) selectAccountWithScheduler(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredImageCapability OpenAIImagesCapability,
	preferredImageCapability OpenAIImagesCapability,
	requireCompact bool,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{}
	scheduler := s.getOpenAIAccountScheduler(ctx)
	if scheduler == nil {
		decision.Layer = openAIAccountScheduleLayerLoadBalance
		if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
			effectiveExcludedIDs := cloneExcludedAccountIDs(excludedIDs)
			for {
				selection, err := s.selectAccountWithLoadAwareness(ctx, groupID, sessionHash, requestedModel, effectiveExcludedIDs, requireCompact)
				if err != nil {
					return nil, decision, err
				}
				if selection == nil || selection.Account == nil {
					return selection, decision, nil
				}
				if selection.Account.SupportsOpenAIImageCapability(requiredImageCapability) {
					return selection, decision, nil
				}
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
				if effectiveExcludedIDs == nil {
					effectiveExcludedIDs = make(map[int64]struct{})
				}
				if _, exists := effectiveExcludedIDs[selection.Account.ID]; exists {
					return nil, decision, ErrNoAvailableAccounts
				}
				effectiveExcludedIDs[selection.Account.ID] = struct{}{}
			}
		}

		effectiveExcludedIDs := cloneExcludedAccountIDs(excludedIDs)
		for {
			selection, err := s.selectAccountWithLoadAwareness(ctx, groupID, sessionHash, requestedModel, effectiveExcludedIDs, requireCompact)
			if err != nil {
				return nil, decision, err
			}
			if selection == nil || selection.Account == nil {
				return selection, decision, nil
			}
			if s.isOpenAIAccountTransportCompatible(selection.Account, requiredTransport) {
				return selection, decision, nil
			}
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			if effectiveExcludedIDs == nil {
				effectiveExcludedIDs = make(map[int64]struct{})
			}
			if _, exists := effectiveExcludedIDs[selection.Account.ID]; exists {
				return nil, decision, ErrNoAvailableAccounts
			}
			effectiveExcludedIDs[selection.Account.ID] = struct{}{}
		}
	}

	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash); err == nil && accountID > 0 {
			stickyAccountID = accountID
		}
	}

	return scheduler.Select(ctx, OpenAIAccountScheduleRequest{
		GroupID:                   groupID,
		SessionHash:               sessionHash,
		StickyAccountID:           stickyAccountID,
		PreviousResponseID:        previousResponseID,
		RequestedModel:            requestedModel,
		RequiredTransport:         requiredTransport,
		RequiredImageCapability:   requiredImageCapability,
		PreferredImageCapability:  preferredImageCapability,
		RequireCompact:            requireCompact,
		ExcludeFullAccountsNoWait: requiredImageCapability != "",
		ExcludedIDs:               excludedIDs,
	})
}

func cloneExcludedAccountIDs(excludedIDs map[int64]struct{}) map[int64]struct{} {
	if len(excludedIDs) == 0 {
		return nil
	}
	cloned := make(map[int64]struct{}, len(excludedIDs))
	for id := range excludedIDs {
		cloned[id] = struct{}{}
	}
	return cloned
}

func (s *OpenAIGatewayService) isOpenAIAccountTransportCompatible(account *Account, requiredTransport OpenAIUpstreamTransport) bool {
	if requiredTransport == OpenAIUpstreamTransportAny || requiredTransport == OpenAIUpstreamTransportHTTPSSE {
		return true
	}
	if s == nil || account == nil {
		return false
	}
	return s.getOpenAIWSProtocolResolver().Resolve(account).Transport == requiredTransport
}

func (s *OpenAIGatewayService) ReportOpenAIAccountScheduleResult(accountID int64, success bool, firstTokenMs *int) {
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return
	}
	scheduler.ReportResult(accountID, success, firstTokenMs)
}

func (s *OpenAIGatewayService) RecordOpenAIAccountSwitch() {
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return
	}
	scheduler.ReportSwitch()
}

// TempUnscheduleRetryableError 对 OpenAI 专用 failover 路径补齐同账号重试耗尽后的临时封禁。
func (s *OpenAIGatewayService) TempUnscheduleRetryableError(ctx context.Context, accountID int64, failoverErr *UpstreamFailoverError) {
	if s == nil {
		return
	}
	tempUnscheduleRetryableError(ctx, s.accountRepo, accountID, failoverErr, "[openai_handler]")
}

// ApplyShortSwitchCooldown 在 OpenAI 专用 failover 换号前短暂摘除旧账号。
func (s *OpenAIGatewayService) ApplyShortSwitchCooldown(ctx context.Context, accountID int64) {
	if s == nil {
		return
	}
	applyShortSwitchCooldown(ctx, s.accountRepo, accountID)
}

func (s *OpenAIGatewayService) SnapshotOpenAIAccountSchedulerMetrics() OpenAIAccountSchedulerMetricsSnapshot {
	scheduler := s.getOpenAIAccountScheduler(context.Background())
	if scheduler == nil {
		return OpenAIAccountSchedulerMetricsSnapshot{}
	}
	return scheduler.SnapshotMetrics()
}

func (s *OpenAIGatewayService) openAIWSSessionStickyTTL() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		return time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	return openaiStickySessionTTL
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func calcLoadSkewByMoments(sum float64, sumSquares float64, count int) float64 {
	if count <= 1 {
		return 0
	}
	mean := sum / float64(count)
	variance := sumSquares/float64(count) - mean*mean
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}
