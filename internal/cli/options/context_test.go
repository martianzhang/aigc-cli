package options

import (
	"context"
	"testing"
)

func TestContextRoundTrip(t *testing.T) {
	t.Cleanup(func() { SetContext(context.Background()) })

	if Context() == nil {
		t.Fatal("Context() must never return nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	SetContext(ctx)
	if got := Context(); got != ctx {
		t.Fatal("Context() did not return the installed context")
	}
	SetContext(nil)
	if got := Context(); got != ctx {
		t.Fatal("SetContext(nil) must not clear the installed context")
	}
}
