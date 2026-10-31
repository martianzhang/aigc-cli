package provider

import (
	"testing"
	"time"
)

func TestRequestTimeout(t *testing.T) {
	tests := []struct {
		name     string
		provider *EffectiveProvider
		fallback time.Duration
		want     time.Duration
	}{
		{
			name:     "explicit timeout wins",
			provider: &EffectiveProvider{Timeout: 30 * time.Second},
			fallback: 5 * time.Minute,
			want:     30 * time.Second,
		},
		{
			name:     "unset keeps provider default",
			provider: &EffectiveProvider{},
			fallback: 15 * time.Minute,
			want:     15 * time.Minute,
		},
		{
			name:     "nil provider keeps provider default",
			provider: nil,
			fallback: 120 * time.Second,
			want:     120 * time.Second,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := requestTimeout(tc.provider, tc.fallback); got != tc.want {
				t.Errorf("requestTimeout() = %v, want %v", got, tc.want)
			}
		})
	}
}
