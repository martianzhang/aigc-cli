package client

import "testing"

func TestAgnesSize(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "480p", want: "720P"},
		{in: "720p", want: "720P"},
		{in: "1080p", want: "2K"},
		{in: "2k", want: "2K"},
		{in: "4k", want: "2K"},
		{in: "960P", want: "960P"},
		{in: "720P", want: "720P"},
		{in: "", want: ""},
	}
	for _, tt := range tests {
		if got := agnesSize(tt.in); got != tt.want {
			t.Errorf("agnesSize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
