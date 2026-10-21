package webfetch

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return u
}

func TestParseCookieString(t *testing.T) {
	cookies, err := ParseCookieString("session=abc; theme=dark; eq=a=b")
	if err != nil {
		t.Fatalf("ParseCookieString: %v", err)
	}
	if len(cookies) != 3 || cookies[0].Name != "session" || cookies[0].Value != "abc" || cookies[2].Value != "a=b" {
		t.Errorf("cookies = %+v, want session/theme/eq", cookies)
	}

	if _, err := ParseCookieString("broken"); err == nil {
		t.Error("ParseCookieString(broken) = nil error, want error")
	}
}

func TestCookieStoreMatchesDomainPathSecure(t *testing.T) {
	store := NewCookieStore()
	store.AddCookie(&http.Cookie{Name: "domain", Value: "1", Domain: ".example.com", Path: "/"})
	store.AddCookie(&http.Cookie{Name: "host", Value: "2", Domain: "api.example.com", Path: "/app"})
	store.AddCookie(&http.Cookie{Name: "secure", Value: "3", Domain: ".example.com", Path: "/", Secure: true})
	store.AddCookie(&http.Cookie{Name: "other", Value: "4", Domain: ".other.com", Path: "/"})

	tests := []struct {
		url  string
		want []string
	}{
		{url: "http://api.example.com/", want: []string{"domain"}},
		{url: "http://api.example.com/app/x", want: []string{"host", "domain"}},
		{url: "https://api.example.com/app", want: []string{"host", "domain", "secure"}},
		{url: "http://example.com/", want: []string{"domain"}},
		{url: "http://other.com/", want: []string{"other"}},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			cookies := store.Cookies(mustURL(t, tc.url))
			got := make([]string, len(cookies))
			for i, c := range cookies {
				got[i] = c.Name
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("cookies = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCookieStoreHonorsExpiry(t *testing.T) {
	store := NewCookieStore()
	store.AddCookie(&http.Cookie{Name: "expired", Value: "x", Domain: "example.com", Expires: time.Now().Add(-time.Hour)})
	store.AddCookie(&http.Cookie{Name: "session", Value: "y", Domain: "example.com"})
	store.AddCookie(&http.Cookie{Name: "future", Value: "z", Domain: "example.com", Expires: time.Now().Add(time.Hour)})

	cookies := store.Cookies(mustURL(t, "https://example.com/"))
	got := make([]string, len(cookies))
	for i, c := range cookies {
		got[i] = c.Name
	}
	if strings.Join(got, ",") != "future,session" {
		t.Errorf("cookies = %v, want future,session", got)
	}
	if all := store.All(); len(all) != 2 {
		t.Errorf("All() = %d cookies, want 2", len(all))
	}
}

func TestCookieStoreLaterSourceWins(t *testing.T) {
	store := NewCookieStore()
	store.AddCookie(&http.Cookie{Name: "session", Value: "old", Domain: "example.com", Path: "/"})
	store.AddCookie(&http.Cookie{Name: "session", Value: "new", Domain: "example.com", Path: "/"})

	cookies := store.Cookies(mustURL(t, "https://example.com/"))
	if len(cookies) != 1 || cookies[0].Value != "new" {
		t.Errorf("cookies = %+v, want the later value", cookies)
	}
}

func TestNetscapeFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "cookies.txt")
	content := "# Netscape HTTP Cookie File\n" +
		".example.com\tTRUE\t/\tFALSE\t0\tsession\tabc\n" +
		"#HttpOnly_.example.com\tTRUE\t/app\tTRUE\t4102444800\tsecure\ttoken\n"
	if err := os.WriteFile(src, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cookies, err := ParseNetscapeFile(src)
	if err != nil {
		t.Fatalf("ParseNetscapeFile: %v", err)
	}
	if len(cookies) != 2 {
		t.Fatalf("cookies = %d, want 2", len(cookies))
	}
	if !cookies[1].HttpOnly || !cookies[1].Secure || cookies[1].Name != "secure" {
		t.Errorf("secure cookie = %+v, want HttpOnly+Secure", cookies[1])
	}

	store := NewCookieStore()
	for _, c := range cookies {
		store.AddCookie(c)
	}
	out := filepath.Join(dir, "out.txt")
	if err := store.WriteNetscapeFile(out); err != nil {
		t.Fatalf("WriteNetscapeFile: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if !strings.Contains(string(data), "#HttpOnly_.example.com") || !strings.Contains(string(data), "session\tabc") {
		t.Errorf("written cookie file missing expected entries:\n%s", data)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestParseNetscapeFileRejectsBadLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(path, []byte("only-one-field\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ParseNetscapeFile(path); err == nil {
		t.Fatal("ParseNetscapeFile accepted a malformed line")
	}
}

func TestCookieStoreClear(t *testing.T) {
	store := NewCookieStore()
	store.AddCookie(&http.Cookie{Name: "a", Value: "b", Domain: "example.com"})
	store.Clear()
	if got := store.Cookies(mustURL(t, "https://example.com/")); len(got) != 0 {
		t.Errorf("cookies = %v, want none after Clear", got)
	}
}
