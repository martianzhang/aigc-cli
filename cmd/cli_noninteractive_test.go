package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestCLIRejectsEmptyStdinWithoutHanging runs the real binary (not an
// in-process call) with an empty stdin. It is the H2 regression guard for the
// observed `aigc-cli </dev/null` hang and the accidental empty chat request.
func TestCLIRejectsEmptyStdinWithoutHanging(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found; cannot build the real binary")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(wd)
	bin := filepath.Join(t.TempDir(), "aigc-cli")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, goBin, "build", "-o", bin, ".")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real binary: %v\n%s", err, out)
	}

	for _, args := range [][]string{{}, {"chat"}} {
		name := "root"
		if len(args) > 0 {
			name = args[0]
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Dir = t.TempDir() // keep the run away from the repo
			cmd.Stdin = strings.NewReader("")
			out, err := cmd.CombinedOutput()

			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("real binary hung on empty stdin: %s", out)
			}
			if err == nil {
				t.Fatalf("expected non-zero exit for empty stdin, got success:\n%s", out)
			}
			if !strings.Contains(string(out), "no input") {
				t.Fatalf("expected a 'no input' error, got:\n%s", out)
			}
		})
	}
}
