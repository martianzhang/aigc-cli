package preview

import (
	"errors"
	"strings"
	"testing"
)

// stubPagerSize pins the terminal grid the pager decision sees.
func stubPagerSize(t *testing.T, cols, rows int) {
	t.Helper()
	old := pagerTerminalSize
	pagerTerminalSize = func() (int, int, bool) { return cols, rows, true }
	t.Cleanup(func() { pagerTerminalSize = old })
}

// stubPager replaces pager lookup and execution.
func stubPager(t *testing.T, lookupErr error, run func([]string, string) (bool, error)) {
	t.Helper()
	oldLookup, oldRun := pagerLookPath, pagerRun
	pagerLookPath = func(string) (string, error) {
		if lookupErr != nil {
			return "", lookupErr
		}
		return "/usr/bin/less", nil
	}
	pagerRun = run
	t.Cleanup(func() {
		pagerLookPath = oldLookup
		pagerRun = oldRun
	})
}

func TestVisualLines(t *testing.T) {
	cases := []struct {
		name    string
		content string
		cols    int
		want    int
	}{
		{"short lines", "a\nb\n", 80, 2},
		{"no trailing newline", "abc", 80, 1},
		{"soft wrap", strings.Repeat("x", 100), 40, 3},
		{"ansi ignored in width", "\x1b[31m" + strings.Repeat("x", 80) + "\x1b[0m", 40, 2},
		{"blank lines count", "a\n\nb\n", 80, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := visualLines(tc.content, tc.cols); got != tc.want {
				t.Errorf("visualLines(%q, %d) = %d, want %d", tc.content, tc.cols, got, tc.want)
			}
		})
	}
}

func TestPagerNeeded(t *testing.T) {
	stubPagerSize(t, 80, 3)
	if pagerNeeded("a\nb\nc\n") {
		t.Error("three lines should fit a three-row terminal")
	}
	if !pagerNeeded("a\nb\nc\nd\n") {
		t.Error("four lines should need a pager")
	}

	stubPagerSize(t, 10, 2)
	if !pagerNeeded(strings.Repeat("x", 25) + "\n") {
		t.Error("a soft-wrapped long line should need a pager")
	}
}

func TestWritePagedText(t *testing.T) {
	long := "1\n2\n3\n4\n5\n"

	t.Run("short prints directly", func(t *testing.T) {
		stubPagerSize(t, 80, 40)
		stubPager(t, nil, func([]string, string) (bool, error) {
			t.Error("pager ran for short content")
			return true, nil
		})
		buf := captureOutput(t)
		writePagedText("1\n2\n", false)
		if got, want := buf.String(), "1\n2\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("long runs pager", func(t *testing.T) {
		stubPagerSize(t, 80, 3)
		t.Setenv("PAGER", "")
		var argv []string
		var fed string
		stubPager(t, nil, func(a []string, content string) (bool, error) {
			argv, fed = a, content
			return true, nil
		})
		buf := captureOutput(t)
		writePagedText(long, false)
		if buf.Len() != 0 {
			t.Errorf("content was printed as well as paged: %q", buf.String())
		}
		if fed != long {
			t.Errorf("pager input = %q, want %q", fed, long)
		}
		if len(argv) == 0 || argv[len(argv)-1] != "-R" {
			t.Errorf("pager argv = %v, want less -R", argv)
		}
	})

	t.Run("no-pager flag prints directly", func(t *testing.T) {
		stubPagerSize(t, 80, 3)
		stubPager(t, nil, func([]string, string) (bool, error) {
			t.Error("pager ran despite --no-pager")
			return true, nil
		})
		buf := captureOutput(t)
		writePagedText(long, true)
		if buf.String() != long {
			t.Errorf("output = %q, want %q", buf.String(), long)
		}
	})

	t.Run("missing pager falls back", func(t *testing.T) {
		stubPagerSize(t, 80, 3)
		t.Setenv("PAGER", "")
		stubPager(t, errors.New("not found"), nil)
		buf := captureOutput(t)
		writePagedText(long, false)
		if buf.String() != long {
			t.Errorf("output = %q, want %q", buf.String(), long)
		}
	})

	t.Run("start failure falls back", func(t *testing.T) {
		stubPagerSize(t, 80, 3)
		stubPager(t, nil, func([]string, string) (bool, error) { return false, errors.New("boom") })
		buf := captureOutput(t)
		writePagedText(long, false)
		if buf.String() != long {
			t.Errorf("output = %q, want %q", buf.String(), long)
		}
	})

	t.Run("pager quit does not reprint", func(t *testing.T) {
		stubPagerSize(t, 80, 3)
		stubPager(t, nil, func([]string, string) (bool, error) { return true, errors.New("quit") })
		buf := captureOutput(t)
		writePagedText(long, false)
		if buf.Len() != 0 {
			t.Errorf("content reprinted after the pager ran: %q", buf.String())
		}
	})

	t.Run("adds trailing newline", func(t *testing.T) {
		stubPagerSize(t, 80, 40)
		buf := captureOutput(t)
		writePagedText("no newline", false)
		if got, want := buf.String(), "no newline\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})
}

func TestPagerCommandPrefersPagerEnv(t *testing.T) {
	t.Setenv("PAGER", "my-pager --flag")
	stubPager(t, errors.New("lookup should not be used"), nil)

	argv, ok := pagerCommand()
	if !ok {
		t.Fatal("pagerCommand() not ok")
	}
	if got, want := strings.Join(argv, "|"), "my-pager|--flag"; got != want {
		t.Errorf("pagerCommand() = %v, want %q", argv, want)
	}
}
