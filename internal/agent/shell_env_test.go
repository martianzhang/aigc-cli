package agent

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// exactEnvValue returns the value for an exact key (case-sensitive), matching
// how Unix environments store HTTP_PROXY and http_proxy separately.
func exactEnvValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if envKey(kv) == key {
			return strings.TrimPrefix(kv, key+"="), true
		}
	}
	return "", false
}

func TestBuildShellEnv(t *testing.T) {
	base := []string{"PATH=/bin", "HTTP_PROXY=http://old:1", "no_proxy=localhost", "HOME=/tmp"}

	t.Run("empty proxy inherits base unchanged", func(t *testing.T) {
		got := buildShellEnv(base, "", false)
		if !reflect.DeepEqual(got, base) {
			t.Errorf("buildShellEnv(base, \"\", false) = %v, want %v", got, base)
		}
	})

	t.Run("direct strips proxy vars but keeps the rest", func(t *testing.T) {
		got := buildShellEnv(base, "", true)

		for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
			if v, ok := exactEnvValue(got, key); ok {
				t.Errorf("direct env still contains %s=%q", key, v)
			}
		}
		if v, ok := exactEnvValue(got, "no_proxy"); !ok || v != "localhost" {
			t.Errorf("direct env should preserve NO_PROXY, got %q (present=%v)", v, ok)
		}
		if v, ok := exactEnvValue(got, "PATH"); !ok || v != "/bin" {
			t.Errorf("direct env should preserve PATH, got %q (present=%v)", v, ok)
		}
	})

	t.Run("proxy replaces existing values", func(t *testing.T) {
		got := buildShellEnv(base, "http://new:2", false)

		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
			if v, ok := exactEnvValue(got, key); !ok || v != "http://new:2" {
				t.Errorf("%s = %q (present=%v), want http://new:2", key, v, ok)
			}
		}
		// The old value must not survive under either case.
		if v, ok := exactEnvValue(got, "HTTP_PROXY"); !ok || v != "http://new:2" {
			t.Errorf("HTTP_PROXY = %q (present=%v), want the new proxy", v, ok)
		}
		if v, ok := exactEnvValue(got, "http_proxy"); !ok || v != "http://new:2" {
			t.Errorf("http_proxy = %q (present=%v), want the new proxy", v, ok)
		}
	})

	t.Run("proxy preserves a user NO_PROXY", func(t *testing.T) {
		got := buildShellEnv(base, "http://new:2", false)
		if v, ok := exactEnvValue(got, "no_proxy"); !ok || v != "localhost" {
			t.Errorf("no_proxy = %q (present=%v), want the user value preserved", v, ok)
		}
		if _, ok := exactEnvValue(got, "NO_PROXY"); ok {
			t.Error("NO_PROXY default should not be added when no_proxy is already set")
		}
	})

	t.Run("proxy adds loopback NO_PROXY defaults", func(t *testing.T) {
		got := buildShellEnv([]string{"PATH=/bin"}, "http://new:2", false)
		for _, key := range []string{"NO_PROXY", "no_proxy"} {
			v, ok := exactEnvValue(got, key)
			if !ok {
				t.Fatalf("%s missing", key)
			}
			for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
				if !strings.Contains(v, host) {
					t.Errorf("%s = %q, want %q", key, v, host)
				}
			}
		}
	})
}

// TestShellCommandHonorsProxy is the regression guard for the bash tool: a
// proxy resolved from config/--http-proxy must reach the child shell even
// though it was never in the parent's environment.
func TestShellCommandHonorsProxy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX parameter expansion is not portable to the Windows shells")
	}
	SetShellProxy("http://127.0.0.1:9999", false)
	t.Cleanup(func() { SetShellProxy("", false) })

	got := ShellCommand(`printf '%s' "$http_proxy"`)
	if strings.TrimSpace(got) != "http://127.0.0.1:9999" {
		t.Errorf("bash tool http_proxy = %q, want http://127.0.0.1:9999", got)
	}
}

// TestShellCommandNoProxy guards that --no-proxy also applies to child shells
// instead of only the in-process Go transport.
func TestShellCommandNoProxy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX parameter expansion is not portable to the Windows shells")
	}
	t.Setenv("HTTP_PROXY", "http://env-proxy:8080")
	t.Setenv("http_proxy", "http://env-proxy:8080")
	SetShellProxy("", true)
	t.Cleanup(func() { SetShellProxy("", false) })

	got := ShellCommand(`printf '%s' "${http_proxy:-unset}"`)
	if strings.TrimSpace(got) != "unset" {
		t.Errorf("bash tool http_proxy = %q, want it stripped by --no-proxy", got)
	}
}
