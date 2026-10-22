package web

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{in: 0, want: "0 B"},
		{in: 512, want: "512 B"},
		{in: 1536, want: "1.5 KB"},
		{in: 2 * 1024 * 1024, want: "2.0 MB"},
		{in: 3 * 1024 * 1024 * 1024, want: "3.00 GB"},
	}
	for _, tc := range tests {
		if got := formatByteSize(tc.in); got != tc.want {
			t.Errorf("formatByteSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatETA(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: -time.Second, want: "--:--"},
		{in: 9 * time.Second, want: "00:09"},
		{in: 75 * time.Second, want: "01:15"},
		{in: 3661 * time.Second, want: "1:01:01"},
	}
	for _, tc := range tests {
		if got := formatETA(tc.in); got != tc.want {
			t.Errorf("formatETA(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMediaProgressTTYLineAndFinish(t *testing.T) {
	var buf bytes.Buffer
	p := &mediaProgress{w: &buf, tty: true, start: time.Now().Add(-time.Second)}
	p.Update(50, 100)
	line := buf.String()
	for _, want := range []string{"50 B", "/ 100 B", "50%", "/s", "ETA"} {
		if !strings.Contains(line, want) {
			t.Errorf("progress line = %q, want %q", line, want)
		}
	}
	buf.Reset()
	p.Finish(100)
	out := buf.String()
	if !strings.Contains(out, "downloaded 100 B") || !strings.Contains(out, "in ") {
		t.Errorf("finish = %q, want a size/duration summary", out)
	}
	if !strings.Contains(out, "\rdownloaded 100 B") {
		t.Errorf("finish = %q, want the live line cleared before the summary", out)
	}
}

func TestMediaProgressNonTTYOnlyFinishes(t *testing.T) {
	var buf bytes.Buffer
	p := &mediaProgress{w: &buf, tty: false, start: time.Now()}
	p.Update(50, 100)
	if buf.Len() != 0 {
		t.Errorf("non-TTY update wrote %q, want nothing", buf.String())
	}
	p.Finish(100)
	if !strings.Contains(buf.String(), "downloaded 100 B") {
		t.Errorf("finish = %q, want the final size", buf.String())
	}
}
