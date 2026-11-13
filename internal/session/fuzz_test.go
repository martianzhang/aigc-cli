package session

import "testing"

func FuzzResolve(f *testing.F) {
	f.Add("latest", "/tmp")
	f.Add("abc123", "")
	f.Add("../escape", "/tmp/sessions")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, value, dir string) {
		_, _ = Resolve(value, dir)
		_, _ = ResolveExisting(value, dir)
	})
}
