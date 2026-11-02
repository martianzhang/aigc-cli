package agent

import (
	"os"
	"strings"
	"sync"
)

// shellProxyConfig is the effective proxy for commands run by the bash tool.
//
// The in-process HTTP client honours config `http_proxy` / `--http-proxy` via
// client.ConfigureDefaultClient, but a child shell (curl, wget, git, …) only
// understands proxy environment variables. SetShellProxy is called once at
// startup so bash commands see the same proxy as the CLI's own HTTP requests.
var shellProxyConfig struct {
	mu     sync.RWMutex
	url    string
	direct bool
}

// SetShellProxy configures the proxy environment inherited by bash tool
// commands. proxyURL is the effective proxy resolved from --http-proxy, config
// `http_proxy`, or the standard environment variables. direct=true (--no-proxy)
// strips proxy variables from the child environment instead.
func SetShellProxy(proxyURL string, direct bool) {
	shellProxyConfig.mu.Lock()
	shellProxyConfig.url = strings.TrimSpace(proxyURL)
	shellProxyConfig.direct = direct
	shellProxyConfig.mu.Unlock()
}

// shellCommandEnv returns the environment for a child shell: the current
// process environment plus the resolved proxy configuration.
func shellCommandEnv() []string {
	shellProxyConfig.mu.RLock()
	proxyURL, direct := shellProxyConfig.url, shellProxyConfig.direct
	shellProxyConfig.mu.RUnlock()
	return buildShellEnv(os.Environ(), proxyURL, direct)
}

// buildShellEnv applies proxyURL/direct to base and returns the child
// environment. It is a pure function so the rules are unit-testable.
//
//   - direct: remove HTTP(S)/ALL proxy variables, leaving everything else
//     (including NO_PROXY) untouched.
//   - proxyURL set: replace existing proxy variables with proxyURL and add
//     loopback defaults to NO_PROXY when the user did not define one.
//   - proxyURL empty: inherit base unchanged.
func buildShellEnv(base []string, proxyURL string, direct bool) []string {
	if direct {
		return withoutProxyEnv(base)
	}
	if proxyURL == "" {
		return base
	}

	out := withoutProxyEnv(base)
	out = append(out,
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"ALL_PROXY="+proxyURL,
		"http_proxy="+proxyURL,
		"https_proxy="+proxyURL,
		"all_proxy="+proxyURL,
	)
	// Match the in-process transport, which always bypasses loopback so local
	// services (Ollama, local MCP servers) are not sent through the proxy.
	if !hasEnvKey(out, "NO_PROXY") && !hasEnvKey(out, "no_proxy") {
		out = append(out, "NO_PROXY=localhost,127.0.0.1,::1", "no_proxy=localhost,127.0.0.1,::1")
	}
	return out
}

func withoutProxyEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if isProxyEnvKey(envKey(kv)) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func envKey(kv string) string {
	if i := strings.IndexByte(kv, '='); i >= 0 {
		return kv[:i]
	}
	return kv
}

// isProxyEnvKey matches the proxy variables curl/wget/git understand. Windows
// environment variables are case-insensitive, so both cases are handled.
func isProxyEnvKey(key string) bool {
	switch strings.ToLower(key) {
	case "http_proxy", "https_proxy", "all_proxy":
		return true
	}
	return false
}

func hasEnvKey(env []string, key string) bool {
	for _, kv := range env {
		if strings.EqualFold(envKey(kv), key) {
			return true
		}
	}
	return false
}
