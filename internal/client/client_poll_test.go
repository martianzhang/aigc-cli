package client

import (
	"testing"
	"time"
)

func TestPollWait(t *testing.T) {
	tests := []struct {
		name     string
		maxWait  time.Duration
		fallback time.Duration
		want     time.Duration
	}{
		{"caller value wins", 30 * time.Minute, 3 * time.Minute, 30 * time.Minute},
		{"zero uses fallback", 0, 3 * time.Minute, 3 * time.Minute},
		{"negative uses fallback", -time.Second, 3 * time.Minute, 3 * time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pollWait(tc.maxWait, tc.fallback); got != tc.want {
				t.Errorf("pollWait(%v, %v) = %v, want %v", tc.maxWait, tc.fallback, got, tc.want)
			}
		})
	}
}
