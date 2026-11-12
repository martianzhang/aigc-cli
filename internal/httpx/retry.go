// Package httpx provides a small retry wrapper for idempotent HTTP requests.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// DefaultMaxBodyBytes caps a buffered HTTP response body (32 MiB). API
// responses are small; this only guards against a misbehaving endpoint.
const DefaultMaxBodyBytes int64 = 32 << 20

// ReadAllLimited reads at most max bytes and reports an error when the body is
// larger, instead of silently truncating it.
func ReadAllLimited(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = DefaultMaxBodyBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("response exceeds %d MB", max>>20)
	}
	return data, nil
}

// Policy controls retry behavior for one request.
type Policy struct {
	MaxAttempts int           // total attempts, including the first; <=0 means 1
	BaseDelay   time.Duration // delay before the first retry
	MaxDelay    time.Duration // cap for exponential backoff
}

// DefaultPolicy retries a safe request up to three times with jittered
// exponential backoff.
var DefaultPolicy = Policy{
	MaxAttempts: 3,
	BaseDelay:   300 * time.Millisecond,
	MaxDelay:    5 * time.Second,
}

// Do sends req and retries transient failures. Only GET/HEAD are retried by
// default: retrying a POST generation request could submit (and bill) twice.
func Do(ctx context.Context, client *http.Client, req *http.Request, policy Policy) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = DefaultPolicy.BaseDelay
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = DefaultPolicy.MaxDelay
	}
	safe := req.Method == http.MethodGet || req.Method == http.MethodHead

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		attemptReq := req.Clone(ctx)
		resp, err := client.Do(attemptReq)
		if err == nil {
			if !shouldRetryStatus(resp.StatusCode) || !safe || attempt == policy.MaxAttempts {
				return resp, nil
			}
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			_ = resp.Body.Close()
			if err := sleepBackoff(ctx, policy, attempt, retryAfter); err != nil {
				return nil, err
			}
			continue
		}
		lastErr = err
		if !safe || attempt == policy.MaxAttempts || ctx.Err() != nil {
			return nil, err
		}
		if err := sleepBackoff(ctx, policy, attempt, 0); err != nil {
			return nil, err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("request failed")
	}
	return nil, lastErr
}

func shouldRetryStatus(code int) bool {
	return code == http.StatusRequestTimeout ||
		code == http.StatusTooManyRequests ||
		code >= 500
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

func sleepBackoff(ctx context.Context, policy Policy, attempt int, retryAfter time.Duration) error {
	delay := retryAfter
	if delay <= 0 {
		delay = policy.BaseDelay << (attempt - 1)
		if delay > policy.MaxDelay {
			delay = policy.MaxDelay
		}
		// Full jitter in [50%, 100%] keeps concurrent pollers from syncing.
		half := delay / 2
		delay = half + time.Duration(rand.Int64N(int64(half)+1))
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
