package wait

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSleepReturnsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := Sleep(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Sleep() took %v after cancellation", elapsed)
	}
}

func TestSleepZeroIsNoop(t *testing.T) {
	if err := Sleep(context.Background(), 0); err != nil {
		t.Fatalf("Sleep(0) error = %v", err)
	}
}
