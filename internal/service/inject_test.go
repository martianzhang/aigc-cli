package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadInjectJSON(t *testing.T) {
	t.Run("empty input yields nil", func(t *testing.T) {
		got, err := ReadInjectJSON("")
		if err != nil {
			t.Fatalf("ReadInjectJSON(\"\") error = %v", err)
		}
		if got != nil {
			t.Errorf("ReadInjectJSON(\"\") = %s, want nil", got)
		}
	})

	t.Run("inline object is returned", func(t *testing.T) {
		got, err := ReadInjectJSON(`{"enable_search":true}`)
		if err != nil {
			t.Fatalf("ReadInjectJSON() error = %v", err)
		}
		if string(got) != `{"enable_search":true}` {
			t.Errorf("ReadInjectJSON() = %s", got)
		}
	})

	t.Run("file path is read", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "inject.json")
		if err := os.WriteFile(path, []byte(`{"a":1}`), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		got, err := ReadInjectJSON(path)
		if err != nil {
			t.Fatalf("ReadInjectJSON(file) error = %v", err)
		}
		if string(got) != `{"a":1}` {
			t.Errorf("ReadInjectJSON(file) = %s", got)
		}
	})

	t.Run("non-object is rejected", func(t *testing.T) {
		for _, in := range []string{`[1,2]`, `"x"`, `1`, `null`} {
			if _, err := ReadInjectJSON(in); err == nil {
				t.Errorf("ReadInjectJSON(%q) = nil error, want error", in)
			}
		}
	})

	t.Run("missing file is reported", func(t *testing.T) {
		if _, err := ReadInjectJSON("does-not-exist.json"); err == nil {
			t.Error("ReadInjectJSON(missing) = nil error, want error")
		}
	})
}
