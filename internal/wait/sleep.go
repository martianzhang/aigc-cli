// Package wait provides context-aware delays for polling loops.
package wait

import (
	"context"
	"time"
)

// Sleep waits for d or until ctx is cancelled, whichever comes first.
func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
