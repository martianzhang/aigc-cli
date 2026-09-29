// Package secret resolves the aigc-cli master secret used for local
// encryption (vault documents today, encrypted config values next).
//
// Resolution order:
//  1. AIGC_CLI_MASTER_KEY environment variable — for CI, containers and
//     headless hosts where no system keyring exists.
//  2. The system keyring (macOS Keychain / Windows Credential Manager /
//     Linux Secret Service), accessed with a 1s timeout so a locked or missing
//     Secret Service can never hang the CLI.
//
// The value is cached for the lifetime of the process, so at most one keyring
// round-trip happens per command.
//
// The stored value is the vault age identity (AGE-SECRET-KEY-1…), kept under
// the original service/account for backward compatibility with existing
// vaults. Use DeriveKey to obtain a symmetric key from it.
package secret

import (
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	// EnvVar overrides the keyring. Set it to the AGE-SECRET-KEY-1… value.
	EnvVar = "AIGC_CLI_MASTER_KEY"

	// Service/account of the master secret in the system keyring. The names
	// predate the generic "master secret" role; they are kept so existing
	// vaults keep decrypting without migration.
	Service = "aigc-cli-vault"
	Account = "identity"

	// DisableEnvVar skips the keyring entirely (no read, no write). Useful for
	// headless runs that must not touch the keyring; pair with EnvVar.
	DisableEnvVar = "AIGC_CLI_NO_KEYRING"
)

// timeout bounds each keyring operation. A variable so tests can shorten it.
var timeout = time.Second

// First-use cross-process locking. Two simultaneous first runs could otherwise
// each generate a key and race to store it; the loser's encrypted data would
// then be unreadable.
var (
	lockPollInterval = 25 * time.Millisecond
	lockWaitTimeout  = 3 * time.Second
	lockStaleAfter   = 10 * time.Second
	// lockDir resolves the directory holding the lock file. Overridable in tests.
	lockDir = defaultLockDir
)

// defaultLockDir mirrors options.ConfigDir without importing the CLI layer.
func defaultLockDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "aigc-cli")
}

// ErrNotFound means neither the env var nor the keyring holds the secret.
var ErrNotFound = errors.New("master secret not found")

// ErrUnavailable means the keyring could not be used (unsupported platform,
// timed out, or otherwise failed). The caller should set EnvVar instead.
var ErrUnavailable = errors.New("system keyring unavailable, set " + EnvVar)

// keyringGet/keyringSet are variables so tests can substitute blocking or
// failing implementations.
var (
	keyringGet = keyring.Get
	keyringSet = keyring.Set
)

var (
	mu        sync.Mutex
	cached    string
	cachedSet bool
)

// Load returns the master secret without generating one.
func Load() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	return loadLocked()
}

func loadLocked() (string, error) {
	if cachedSet {
		return cached, nil
	}
	if v := strings.TrimSpace(os.Getenv(EnvVar)); v != "" {
		cached, cachedSet = v, true
		return v, nil
	}
	if os.Getenv(DisableEnvVar) != "" {
		return "", fmt.Errorf("%w (keyring disabled by %s)", ErrUnavailable, DisableEnvVar)
	}
	value, err := getWithTimeout()
	if err != nil {
		return "", err
	}
	cached, cachedSet = value, true
	return value, nil
}

// Ensure returns the master secret, generating and persisting one with
// generate when neither the env var nor the keyring holds a value. created
// reports whether generate ran.
//
// First-use creation is serialized across processes with a lock file so
// concurrent first runs converge on a single key.
func Ensure(generate func() (string, error)) (value string, created bool, err error) {
	mu.Lock()
	defer mu.Unlock()

	if v, err := loadLocked(); err == nil {
		return v, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", false, err
	}

	if release, ok := tryLock(); ok {
		defer release()
		// Re-check: another process may have generated while we waited.
		if v, err := loadLocked(); err == nil {
			return v, false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return "", false, err
		}
		return generateAndStore(generate)
	}

	// Another process holds the lock; wait for it to publish the key.
	if v, err := waitForKey(); err == nil {
		return v, false, nil
	} else if errors.Is(err, ErrUnavailable) {
		return "", false, err
	}

	// Holder died or the lock is unusable: fall back to best-effort creation.
	return generateAndStore(generate)
}

// generateAndStore runs generate, stores the result and refreshes the cache.
func generateAndStore(generate func() (string, error)) (string, bool, error) {
	generated, err := generate()
	if err != nil {
		return "", false, fmt.Errorf("generate master secret: %w", err)
	}
	if err := setWithTimeout(generated); err != nil {
		return "", false, err
	}
	cached, cachedSet = generated, true
	return generated, true, nil
}

// tryLock creates the lock file exclusively. release is non-nil only when the
// lock was acquired.
func tryLock() (release func(), ok bool) {
	dir := lockDir()
	if dir == "" {
		return nil, false
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false
	}
	path := filepath.Join(dir, ".master-key.lock")
	// Break a lock left behind by a dead process.
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > lockStaleAfter {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, false
	}
	_ = f.Close()
	return func() { _ = os.Remove(path) }, true
}

// waitForKey polls the keyring until another process publishes the key.
func waitForKey() (string, error) {
	deadline := time.Now().Add(lockWaitTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(lockPollInterval)
		v, err := getWithTimeout()
		if err == nil {
			cached, cachedSet = v, true
			return v, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", err
		}
	}
	return "", ErrNotFound
}

// Store persists value to the keyring and refreshes the process cache.
func Store(value string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := setWithTimeout(value); err != nil {
		return err
	}
	cached, cachedSet = value, true
	return nil
}

// DeriveKey derives a 32-byte symmetric key for purpose using HKDF-SHA256 over
// the master secret. Distinct purposes yield independent keys.
func DeriveKey(purpose string) ([]byte, error) {
	master, err := Load()
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Key(sha256.New, []byte(master), nil, "aigc-cli/"+purpose, 32)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}

// Reset clears the process cache. Tests only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	cached, cachedSet = "", false
}

// getWithTimeout runs the (possibly blocking) keyring read in a goroutine and
// gives up after timeout. A blocked goroutine is left behind deliberately: the
// OS call owns the thread, and the process usually exits shortly after.
func getWithTimeout() (string, error) {
	type result struct {
		value string
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := keyringGet(Service, Account)
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		if errors.Is(r.err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		if r.err != nil {
			return "", fmt.Errorf("%w: %v", ErrUnavailable, r.err)
		}
		return r.value, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("%w: read timed out after %s", ErrUnavailable, timeout)
	}
}

// setWithTimeout is getWithTimeout's write counterpart.
func setWithTimeout(value string) error {
	ch := make(chan error, 1)
	go func() { ch <- keyringSet(Service, Account, value) }()
	select {
	case err := <-ch:
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("%w: write timed out after %s", ErrUnavailable, timeout)
	}
}
