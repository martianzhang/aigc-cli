package secret

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// reset installs a fresh in-memory keyring, a temp lock dir, and clears the
// process cache and lock timings.
func reset(t *testing.T) {
	t.Helper()
	t.Setenv(EnvVar, "")
	t.Setenv(DisableEnvVar, "")
	keyring.MockInit()
	keyringGet = keyring.Get
	keyringSet = keyring.Set
	timeout = time.Second
	dir := t.TempDir()
	lockDir = func() string { return dir }
	lockPollInterval = 25 * time.Millisecond
	lockWaitTimeout = 3 * time.Second
	lockStaleAfter = 10 * time.Second
	Reset()
	t.Cleanup(func() {
		keyring.MockInit()
		keyringGet = keyring.Get
		keyringSet = keyring.Set
		timeout = time.Second
		lockDir = defaultLockDir
		lockPollInterval = 25 * time.Millisecond
		lockWaitTimeout = 3 * time.Second
		lockStaleAfter = 10 * time.Second
		Reset()
	})
}

func TestLoadEnvOverride(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "  AGE-SECRET-KEY-TEST  ")
	// Env wins even when the keyring would fail.
	keyringGet = func(string, string) (string, error) { return "", errors.New("boom") }

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "AGE-SECRET-KEY-TEST" {
		t.Errorf("Load() = %q, want trimmed env value", got)
	}
}

func TestLoadNotFound(t *testing.T) {
	reset(t)
	_, err := Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
}

func TestEnsureGeneratesAndStores(t *testing.T) {
	reset(t)
	calls := 0
	generate := func() (string, error) { calls++; return "gen-1", nil }

	got, created, err := Ensure(generate)
	if err != nil || !created || got != "gen-1" {
		t.Fatalf("Ensure() = (%q, %v, %v), want (gen-1, true, nil)", got, created, err)
	}

	// Fresh process view resolves the stored value without regenerating.
	Reset()
	got, created, err = Ensure(generate)
	if err != nil || created || got != "gen-1" {
		t.Fatalf("second Ensure() = (%q, %v, %v), want (gen-1, false, nil)", got, created, err)
	}
	if calls != 1 {
		t.Errorf("generate called %d times, want 1", calls)
	}
}

func TestEnsureEnvSkipsGenerate(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "from-env")
	generate := func() (string, error) { t.Fatal("generate must not run when env is set"); return "", nil }

	got, created, err := Ensure(generate)
	if err != nil || created || got != "from-env" {
		t.Fatalf("Ensure() = (%q, %v, %v), want (from-env, false, nil)", got, created, err)
	}
}

func TestEnsureWaitsForConcurrentGeneration(t *testing.T) {
	reset(t)
	// Simulate another process holding the lock and publishing the key shortly
	// after we start waiting.
	lock := filepath.Join(lockDir(), ".master-key.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	lockPollInterval = 10 * time.Millisecond
	lockWaitTimeout = 2 * time.Second
	go func() {
		time.Sleep(3 * lockPollInterval)
		_ = keyringSet(Service, Account, "raced")
	}()

	got, created, err := Ensure(func() (string, error) {
		t.Error("generate must not run while another process is publishing the key")
		return "", nil
	})
	if err != nil || created || got != "raced" {
		t.Fatalf("Ensure() = (%q, %v, %v), want (raced, false, nil)", got, created, err)
	}
}

func TestEnsureBreaksStaleLock(t *testing.T) {
	reset(t)
	lock := filepath.Join(lockDir(), ".master-key.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatalf("age lock: %v", err)
	}

	got, created, err := Ensure(func() (string, error) { return "fresh", nil })
	if err != nil || !created || got != "fresh" {
		t.Fatalf("Ensure() = (%q, %v, %v), want (fresh, true, nil)", got, created, err)
	}
}

func TestEnsureUnavailable(t *testing.T) {
	reset(t)
	keyringGet = func(string, string) (string, error) { return "", errors.New("dbus down") }
	generate := func() (string, error) { t.Fatal("generate must not run when keyring is unavailable"); return "", nil }

	_, _, err := Ensure(generate)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Ensure() error = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), EnvVar) {
		t.Errorf("error should suggest %s: %v", EnvVar, err)
	}
}

func TestDisableEnvVar(t *testing.T) {
	reset(t)
	t.Setenv(DisableEnvVar, "1")
	_, err := Load()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Load() error = %v, want ErrUnavailable", err)
	}
}

func TestDeriveKey(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "master-secret")

	k1, err := DeriveKey("config")
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}
	k2, err := DeriveKey("config")
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}
	kOther, err := DeriveKey("vault")
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}
	if len(k1) != 32 {
		t.Errorf("len(key) = %d, want 32", len(k1))
	}
	if string(k1) != string(k2) {
		t.Error("DeriveKey is not deterministic for the same purpose")
	}
	if string(k1) == string(kOther) {
		t.Error("different purposes must yield different keys")
	}
}

func TestGetTimeout(t *testing.T) {
	reset(t)
	block := make(chan struct{})
	defer close(block)
	keyringGet = func(string, string) (string, error) { <-block; return "", nil }
	timeout = 20 * time.Millisecond

	start := time.Now()
	_, err := Load()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Load() error = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should mention timeout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("timeout took %v, want under 1s", elapsed)
	}
}
