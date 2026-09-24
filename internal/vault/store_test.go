package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/fsutil"
)

// testDoc builds a VaultDoc with deterministic fields for assertions.
func testDoc(id, title string) *VaultDoc {
	return &VaultDoc{
		ID:        id,
		URL:       "https://example.com/" + id,
		FilePath:  "/tmp/" + id + ".txt",
		Title:     title,
		Size:      int64(len(title)),
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

// newTestVault opens a vault rooted at a fresh temp directory.
func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return v
}

func docEqual(a, b VaultDoc) bool {
	return a.ID == b.ID && a.URL == b.URL && a.FilePath == b.FilePath &&
		a.Title == b.Title && a.Size == b.Size && a.CreatedAt.Equal(b.CreatedAt)
}

func assertDocsEqual(t *testing.T, got, want []VaultDoc) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(docs) = %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if !docEqual(got[i], want[i]) {
			t.Errorf("docs[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestOpen(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nested", "vault")
	v, err := Open(base)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", base, err)
	}
	if v.baseDir != base {
		t.Errorf("baseDir = %q, want %q", v.baseDir, base)
	}

	docsDir := filepath.Join(base, "docs")
	info, err := os.Stat(docsDir)
	if err != nil {
		t.Fatalf("stat %s: %v", docsDir, err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", docsDir)
	}
}

func TestBaseDir(t *testing.T) {
	base := t.TempDir()
	v, err := Open(base)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got := v.BaseDir(); got != base {
		t.Errorf("BaseDir() = %q, want %q", got, base)
	}
}

func TestDocPath(t *testing.T) {
	v := newTestVault(t)
	got := v.docPath("abc123")
	want := filepath.Join(v.BaseDir(), "docs", "abc123.age")
	if got != want {
		t.Errorf("docPath(abc123) = %q, want %q", got, want)
	}
}

func TestMetadataPath(t *testing.T) {
	v := newTestVault(t)
	got := v.metadataPath()
	want := filepath.Join(v.BaseDir(), "metadata.json")
	if got != want {
		t.Errorf("metadataPath() = %q, want %q", got, want)
	}
}

func TestOpenCreatesOwnerOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not enforced on Windows")
	}
	base := filepath.Join(t.TempDir(), "vault")
	if _, err := Open(base); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(base, "docs"))
	if err != nil {
		t.Fatalf("stat docs dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("docs dir mode = %04o, want 0700", got)
	}
}

func TestWriteMetadataOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not enforced on Windows")
	}
	v := newTestVault(t)
	if err := v.writeMetadata([]VaultDoc{*testDoc("id-1", "first")}); err != nil {
		t.Fatalf("writeMetadata() error = %v", err)
	}
	info, err := os.Stat(v.metadataPath())
	if err != nil {
		t.Fatalf("stat metadata: %v", err)
	}
	if got := info.Mode().Perm(); got != fsutil.PrivateFileMode {
		t.Errorf("metadata mode = %04o, want %04o", got, fsutil.PrivateFileMode)
	}
}

func TestWriteMetadata(t *testing.T) {
	v := newTestVault(t)
	docs := []VaultDoc{*testDoc("id-1", "first"), *testDoc("id-2", "second")}

	if err := v.writeMetadata(docs); err != nil {
		t.Fatalf("writeMetadata() error = %v", err)
	}

	data, err := os.ReadFile(v.metadataPath())
	if err != nil {
		t.Fatalf("read metadata file: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("metadata file is not valid JSON: %s", data)
	}

	var got []VaultDoc
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	assertDocsEqual(t, got, docs)
}

func TestReadMetadata(t *testing.T) {
	v := newTestVault(t)
	docs := []VaultDoc{*testDoc("id-1", "first"), *testDoc("id-2", "second")}

	data, err := json.Marshal(docs)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(v.metadataPath(), data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := v.readMetadata()
	if err != nil {
		t.Fatalf("readMetadata() error = %v", err)
	}
	assertDocsEqual(t, got, docs)
}

func TestReadMetadata_notExist(t *testing.T) {
	v := newTestVault(t)

	got, err := v.readMetadata()
	if err != nil {
		t.Fatalf("readMetadata() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("readMetadata() = %+v, want nil", got)
	}
}

func TestAddMetadata(t *testing.T) {
	v := newTestVault(t)

	first := testDoc("id-1", "first")
	if err := v.addMetadata(first); err != nil {
		t.Fatalf("addMetadata(first) error = %v", err)
	}
	got, err := v.readMetadata()
	if err != nil {
		t.Fatalf("readMetadata() error = %v", err)
	}
	assertDocsEqual(t, got, []VaultDoc{*first})

	second := testDoc("id-2", "second")
	if err := v.addMetadata(second); err != nil {
		t.Fatalf("addMetadata(second) error = %v", err)
	}
	got, err = v.readMetadata()
	if err != nil {
		t.Fatalf("readMetadata() error = %v", err)
	}
	assertDocsEqual(t, got, []VaultDoc{*first, *second})
}

func TestAddMetadata_replace(t *testing.T) {
	v := newTestVault(t)

	if err := v.addMetadata(testDoc("id-1", "original")); err != nil {
		t.Fatalf("addMetadata(original) error = %v", err)
	}

	replacement := testDoc("id-1", "replaced")
	replacement.Size = 42
	if err := v.addMetadata(replacement); err != nil {
		t.Fatalf("addMetadata(replacement) error = %v", err)
	}

	got, err := v.readMetadata()
	if err != nil {
		t.Fatalf("readMetadata() error = %v", err)
	}
	assertDocsEqual(t, got, []VaultDoc{*replacement})
}

func TestRemoveMetadata(t *testing.T) {
	v := newTestVault(t)
	dropped := testDoc("id-1", "drop")
	kept := testDoc("id-2", "keep")
	third := testDoc("id-3", "drop")
	for _, d := range []*VaultDoc{dropped, kept, third} {
		if err := v.addMetadata(d); err != nil {
			t.Fatalf("addMetadata(%s) error = %v", d.ID, err)
		}
	}

	if err := v.removeMetadata("id-1"); err != nil {
		t.Fatalf("removeMetadata(id-1) error = %v", err)
	}
	got, err := v.readMetadata()
	if err != nil {
		t.Fatalf("readMetadata() error = %v", err)
	}
	assertDocsEqual(t, got, []VaultDoc{*kept, *third})

	// Removing an unknown ID is a no-op.
	if err := v.removeMetadata("missing"); err != nil {
		t.Fatalf("removeMetadata(missing) error = %v", err)
	}
	got, err = v.readMetadata()
	if err != nil {
		t.Fatalf("readMetadata() error = %v", err)
	}
	assertDocsEqual(t, got, []VaultDoc{*kept, *third})
}

func TestGetMetadata(t *testing.T) {
	v := newTestVault(t)
	for _, d := range []*VaultDoc{testDoc("id-1", "first"), testDoc("id-2", "second")} {
		if err := v.addMetadata(d); err != nil {
			t.Fatalf("addMetadata(%s) error = %v", d.ID, err)
		}
	}

	got, err := v.getMetadata("id-2")
	if err != nil {
		t.Fatalf("getMetadata(id-2) error = %v", err)
	}
	want := testDoc("id-2", "second")
	if !docEqual(*got, *want) {
		t.Errorf("getMetadata(id-2) = %+v, want %+v", *got, *want)
	}
}

func TestGetMetadata_notFound(t *testing.T) {
	v := newTestVault(t)
	if err := v.addMetadata(testDoc("id-1", "first")); err != nil {
		t.Fatalf("addMetadata(id-1) error = %v", err)
	}

	got, err := v.getMetadata("missing")
	if err == nil {
		t.Fatal("getMetadata(missing) error = nil, want error")
	}
	if got != nil {
		t.Errorf("getMetadata(missing) = %+v, want nil", got)
	}
}

func TestVaultDocJSONTags(t *testing.T) {
	want := map[string]string{
		"ID":        "id",
		"URL":       "url,omitempty",
		"FilePath":  "filepath,omitempty",
		"Title":     "title,omitempty",
		"Size":      "size",
		"CreatedAt": "created_at",
	}

	typ := reflect.TypeOf(VaultDoc{})
	if typ.NumField() != len(want) {
		t.Errorf("VaultDoc has %d fields, want %d", typ.NumField(), len(want))
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		expected, ok := want[f.Name]
		if !ok {
			t.Errorf("unexpected field %q", f.Name)
			continue
		}
		if got := f.Tag.Get("json"); got != expected {
			t.Errorf("field %s json tag = %q, want %q", f.Name, got, expected)
		}
	}
}

func TestVaultDocJSONRoundTrip(t *testing.T) {
	want := *testDoc("id-1", "first")

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal VaultDoc: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal into map: %v", err)
	}
	for _, key := range []string{"id", "url", "filepath", "title", "size", "created_at"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("marshaled JSON missing key %q: %s", key, data)
		}
	}

	var got VaultDoc
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal VaultDoc: %v", err)
	}
	if !docEqual(got, want) {
		t.Errorf("round-tripped doc = %+v, want %+v", got, want)
	}
}

func TestVaultDocJSONOmitEmpty(t *testing.T) {
	doc := VaultDoc{ID: "id-1", Size: 7, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal VaultDoc: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal into map: %v", err)
	}
	for _, key := range []string{"url", "filepath", "title"} {
		if _, ok := raw[key]; ok {
			t.Errorf("empty field should be omitted, but key %q present: %s", key, data)
		}
	}
}
