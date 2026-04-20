package service

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGetUsagePercentLimit(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  float64
	}{
		{"nil extra", nil, 0},
		{"not set", map[string]any{}, 0},
		{"float64", map[string]any{"usage_percent_limit": 80.0}, 80.0},
		{"int", map[string]any{"usage_percent_limit": 75}, 75.0},
		{"json.Number", map[string]any{"usage_percent_limit": json.Number("90")}, 90.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.GetUsagePercentLimit(); got != tt.want {
				t.Errorf("GetUsagePercentLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPassiveUsagePercent(t *testing.T) {
	futureEnd := time.Now().Add(time.Hour)
	expiredEnd := time.Now().Add(-time.Minute)

	tests := []struct {
		name             string
		extra            map[string]any
		sessionWindowEnd *time.Time
		want             float64
		wantOK           bool
	}{
		{"nil extra", nil, nil, 0, false},
		{"not set", map[string]any{}, nil, 0, false},
		{"float64 0.75 -> 75", map[string]any{"session_window_utilization": 0.75}, &futureEnd, 75.0, true},
		{"json.Number 0.5 -> 50", map[string]any{"session_window_utilization": json.Number("0.5")}, &futureEnd, 50.0, true},
		{"expired window fails open", map[string]any{"session_window_utilization": 0.95}, &expiredEnd, 0, false},
		{"unsupported type", map[string]any{"session_window_utilization": "0.8"}, &futureEnd, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra, SessionWindowEnd: tt.sessionWindowEnd}
			got, ok := a.GetPassiveUsagePercent()
			if ok != tt.wantOK {
				t.Errorf("GetPassiveUsagePercent() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("GetPassiveUsagePercent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAccountSchedulableForUsagePercent(t *testing.T) {
	svc := &GatewayService{}
	futureEnd := time.Now().Add(time.Hour)
	expiredEnd := time.Now().Add(-time.Minute)

	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "oauth account under limit",
			account: &Account{
				Platform:         PlatformAnthropic,
				Type:             AccountTypeOAuth,
				SessionWindowEnd: &futureEnd,
				Extra: map[string]any{
					"usage_percent_limit":        80.0,
					"session_window_utilization": 0.75,
				},
			},
			want: true,
		},
		{
			name: "oauth account over limit",
			account: &Account{
				Platform:         PlatformAnthropic,
				Type:             AccountTypeOAuth,
				SessionWindowEnd: &futureEnd,
				Extra: map[string]any{
					"usage_percent_limit":        80.0,
					"session_window_utilization": 0.81,
				},
			},
			want: false,
		},
		{
			name: "setup token also gated",
			account: &Account{
				Platform:         PlatformAnthropic,
				Type:             AccountTypeSetupToken,
				SessionWindowEnd: &futureEnd,
				Extra: map[string]any{
					"usage_percent_limit":        80.0,
					"session_window_utilization": 0.81,
				},
			},
			want: false,
		},
		{
			name: "expired sample fails open",
			account: &Account{
				Platform:         PlatformAnthropic,
				Type:             AccountTypeOAuth,
				SessionWindowEnd: &expiredEnd,
				Extra: map[string]any{
					"usage_percent_limit":        80.0,
					"session_window_utilization": 0.99,
				},
			},
			want: true,
		},
		{
			name: "missing passive sample fails open",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"usage_percent_limit": 80.0,
				},
			},
			want: true,
		},
		{
			name: "non oauth account ignored",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"usage_percent_limit":        1.0,
					"session_window_utilization": 1.0,
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.isAccountSchedulableForUsagePercent(t.Context(), tt.account, false); got != tt.want {
				t.Fatalf("isAccountSchedulableForUsagePercent() = %v, want %v", got, tt.want)
			}
		})
	}
}
