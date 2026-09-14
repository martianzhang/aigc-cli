package service

import "testing"

func TestMaskKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "empty", key: "", want: "***"},
		{name: "too-short", key: "abc", want: "***"},
		{name: "five-chars-reveals-last-four", key: "abcde", want: "...bcde"},
		{name: "typical-key-reveals-suffix-only", key: "sk-1234567890abcdef", want: "...cdef"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaskKey(tc.key); got != tc.want {
				t.Errorf("MaskKey(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestRedactURLSecrets(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "plain-url-unchanged",
			in:   "https://api.example.com/v1",
			want: "https://api.example.com/v1",
		},
		{
			name: "basic-auth-password-redacted",
			in:   "https://user:s3cret@api.example.com/v1",
			want: "https://user:REDACTED@api.example.com/v1",
		},
		{
			name: "query-key-redacted",
			in:   "https://api.example.com/v1?key=abc123&x=1",
			want: "https://api.example.com/v1?key=REDACTED&x=1",
		},
		{
			name: "query-token-redacted",
			in:   "https://api.example.com/v1?token=zzz",
			want: "https://api.example.com/v1?token=REDACTED",
		},
		{
			name: "non-secret-query-untouched",
			in:   "https://api.example.com/v1?model=gpt&x=1",
			want: "https://api.example.com/v1?model=gpt&x=1",
		},
		{
			name: "path-key-redacted",
			in:   "https://api.example.com/sk-abcdefgh12345678/v1",
			want: "https://api.example.com/REDACTED/v1",
		},
		{
			name: "unparseable-input-still-redacted",
			in:   "not a url ?key=abc123",
			want: "not a url ?key=REDACTED",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactURLSecrets(tc.in); got != tc.want {
				t.Errorf("RedactURLSecrets(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
