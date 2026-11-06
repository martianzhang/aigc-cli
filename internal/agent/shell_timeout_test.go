package agent

import (
	"runtime"
	"testing"
	"time"
)

func TestParseShellTimeout(t *testing.T) {
	cases := []struct {
		name    string
		seconds float64
		want    time.Duration
		wantErr bool
	}{
		{"omitted", 0, 0, false},
		{"whole seconds", 30, 30 * time.Second, false},
		{"fractional seconds", 0.25, 250 * time.Millisecond, false},
		{"negative", -1, 0, true},
		{"above the cap", MaxShellTimeout.Seconds() + 1, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseShellTimeout(tc.seconds)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseShellTimeout(%v) error = nil, want an error", tc.seconds)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseShellTimeout(%v) error = %v", tc.seconds, err)
			}
			if got != tc.want {
				t.Errorf("parseShellTimeout(%v) = %v, want %v", tc.seconds, got, tc.want)
			}
		})
	}
}

// TestKillActiveShellsStopsRegisteredCommand guards the SIGINT cleanup path:
// a command running in its own process group must be reachable from the CLI.
func TestKillActiveShellsStopsRegisteredCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep is not available on windows")
	}
	done := make(chan string, 1)
	go func() { done <- ShellCommand("sleep 30") }()

	deadline := time.Now().Add(2 * time.Second)
	for activeShellCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if activeShellCount() == 0 {
		t.Fatal("shell command was not registered as active")
	}

	KillActiveShells()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("KillActiveShells did not stop the running command")
	}
	if n := activeShellCount(); n != 0 {
		t.Errorf("active shells = %d after the command exited, want 0", n)
	}
}

func TestResolveShellTimeout(t *testing.T) {
	prev := shellDefaultTimeout()
	t.Cleanup(func() { SetShellTimeout(prev) })

	SetShellTimeout(0)
	if got := resolveShellTimeout(0); got != DefaultShellTimeout {
		t.Errorf("resolveShellTimeout(0) = %v, want built-in %v", got, DefaultShellTimeout)
	}

	SetShellTimeout(45 * time.Second)
	if got := resolveShellTimeout(0); got != 45*time.Second {
		t.Errorf("resolveShellTimeout(0) = %v, want configured 45s", got)
	}
	if got := resolveShellTimeout(10 * time.Second); got != 10*time.Second {
		t.Errorf("resolveShellTimeout(10s) = %v, want the per-call 10s", got)
	}
}
