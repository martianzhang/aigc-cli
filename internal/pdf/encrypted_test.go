package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsEncrypted(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.pdf")
	if err := os.WriteFile(plain, []byte("%PDF-1.7\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	encrypted := filepath.Join(dir, "enc.pdf")
	if err := os.WriteFile(encrypted, []byte("%PDF-1.7\n1 0 obj<<>>endobj\ntrailer<< /Encrypt 5 0 R >>\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if IsEncrypted(plain) {
		t.Fatal("plain PDF reported as encrypted")
	}
	if !IsEncrypted(encrypted) {
		t.Fatal("encrypted PDF not detected")
	}
	if IsEncrypted(filepath.Join(dir, "missing.pdf")) {
		t.Fatal("missing file reported as encrypted")
	}
}
