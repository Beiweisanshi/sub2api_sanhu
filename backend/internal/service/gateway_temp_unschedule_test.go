package service

// 作者: mkx  变更: 2026/04/26
// 覆盖 tempUnscheduleRetryableError 的状态码分发逻辑，保证 401/502/504 触发账号级冷却，
// 而 400 仍然只在 RetryableOnSameAccount=true 时下架（避免把客户端错误算到账号头上）。

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type tempUnschedRepoStub struct {
	AccountRepository
	calls []tempUnschedRepoCall
}

type tempUnschedRepoCall struct {
	accountID int64
	until     time.Time
	reason    string
}

func (r *tempUnschedRepoStub) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.calls = append(r.calls, tempUnschedRepoCall{accountID: id, until: until, reason: reason})
	return nil
}

func TestTempUnscheduleRetryableError_StatusDispatch(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		retryable  bool
		wantCalled bool
		wantReason string // 子串匹配
	}{
		{"401_retryable_5min", http.StatusUnauthorized, true, true, "401: upstream authentication failed"},
		{"401_non_retryable_仍下架_5min", http.StatusUnauthorized, false, true, "401: upstream authentication failed"},
		{"502_retryable_1min", http.StatusBadGateway, true, true, "empty stream response"},
		{"502_non_retryable_仍下架_1min", http.StatusBadGateway, false, true, "empty stream response"},
		{"504_non_retryable_仍下架_1min", http.StatusGatewayTimeout, false, true, "empty stream response"},
		{"400_retryable_下架_1min", http.StatusBadRequest, true, true, "invalid project resource name"},
		{"400_non_retryable_不下架_客户端错误", http.StatusBadRequest, false, false, ""},
		{"500_不在白名单_不下架", http.StatusInternalServerError, false, false, ""},
		{"429_不在白名单_由限流路径处理", http.StatusTooManyRequests, true, false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &tempUnschedRepoStub{}
			err := &UpstreamFailoverError{StatusCode: tc.status, RetryableOnSameAccount: tc.retryable}

			tempUnscheduleRetryableError(context.Background(), repo, 42, err, "[test]")

			if !tc.wantCalled {
				require.Empty(t, repo.calls, "不应触发 SetTempUnschedulable")
				return
			}
			require.Len(t, repo.calls, 1)
			require.Equal(t, int64(42), repo.calls[0].accountID)
			require.Contains(t, repo.calls[0].reason, tc.wantReason)
		})
	}
}

func TestTempUnscheduleRetryableError_NilGuard(t *testing.T) {
	repo := &tempUnschedRepoStub{}
	tempUnscheduleRetryableError(context.Background(), repo, 1, nil, "[test]")
	require.Empty(t, repo.calls, "failoverErr=nil 时不应触发任何下架")

	tempUnscheduleRetryableError(context.Background(), nil, 1, &UpstreamFailoverError{StatusCode: 401}, "[test]")
	// repo 为 nil 直接返回，不 panic 即为通过。
}

func TestTempUnscheduleRetryableError_AuthCooldownDuration(t *testing.T) {
	repo := &tempUnschedRepoStub{}
	err := &UpstreamFailoverError{StatusCode: http.StatusUnauthorized}

	before := time.Now()
	tempUnscheduleRetryableError(context.Background(), repo, 7, err, "[test]")
	after := time.Now()

	require.Len(t, repo.calls, 1)
	until := repo.calls[0].until
	// 5min ± 调用窗口
	require.WithinDuration(t, before.Add(authErrorCooldown), until, after.Sub(before)+time.Second)
}
