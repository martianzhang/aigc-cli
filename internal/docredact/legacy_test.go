package docredact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareInputLegacyDOCMissingConverter(t *testing.T) {
	oldFind, oldRun := findSoffice, runSoffice
	defer func() { findSoffice, runSoffice = oldFind, oldRun }()
	findSoffice = func() (string, error) { return "", errNoConverter{} }
	_, _, _, err := PrepareInput(filepath.Join(t.TempDir(), "old.doc"))
	if err == nil || !strings.Contains(err.Error(), "LibreOffice") {
		t.Fatalf("error = %v, want LibreOffice guidance", err)
	}
}

type errNoConverter struct{}

func (errNoConverter) Error() string { return "legacy .doc requires LibreOffice (soffice)" }

func TestPrepareInputLegacyDOCConversion(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "old.doc")
	if err := os.WriteFile(input, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldFind, oldRun := findSoffice, runSoffice
	defer func() { findSoffice, runSoffice = oldFind, oldRun }()
	findSoffice = func() (string, error) { return "soffice", nil }
	runSoffice = func(_ string, args ...string) ([]byte, error) {
		var outdir string
		for i, arg := range args {
			if arg == "--outdir" && i+1 < len(args) {
				outdir = args[i+1]
			}
		}
		if outdir == "" {
			return nil, os.ErrInvalid
		}
		return nil, os.WriteFile(filepath.Join(outdir, "old.docx"), []byte("converted"), 0o600)
	}

	path, outExt, cleanup, err := PrepareInput(input)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if outExt != ".docx" || filepath.Ext(path) != ".docx" {
		t.Fatalf("path/ext = %s/%s, want .docx", path, outExt)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "converted" {
		t.Fatalf("converted = %q, %v", data, err)
	}
}
