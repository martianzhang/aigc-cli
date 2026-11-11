package docredact

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// findSoffice and runSoffice are variables so tests can simulate LibreOffice
// without requiring a real installation.
var findSoffice = func() (string, error) {
	for _, name := range []string{"soffice", "libreoffice"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("legacy .doc requires LibreOffice (soffice) to convert; install it or convert to .docx manually")
}

var runSoffice = func(bin string, args ...string) ([]byte, error) {
	return exec.Command(bin, args...).CombinedOutput()
}

// PrepareInput converts a legacy .doc to a temporary .docx. It returns the path
// to use, the extension a redacted output should have, and a cleanup function.
// Non-.doc inputs are returned unchanged.
func PrepareInput(path string) (string, string, func(), error) {
	ext := filepath.Ext(path)
	if !strings.EqualFold(ext, ".doc") {
		return path, ext, func() {}, nil
	}
	converted, cleanup, err := convertLegacyDOC(path)
	if err != nil {
		return "", "", func() {}, err
	}
	return converted, ".docx", cleanup, nil
}

func convertLegacyDOC(path string) (string, func(), error) {
	soffice, err := findSoffice()
	if err != nil {
		return "", func() {}, err
	}
	tmp, err := os.MkdirTemp("", "aigc-doc-convert-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	out, err := runSoffice(soffice, "--headless", "--convert-to", "docx", "--outdir", tmp, path)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("convert .doc with LibreOffice: %w: %s", err, strings.TrimSpace(string(out)))
	}
	target := filepath.Join(tmp, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".docx")
	if _, err := os.Stat(target); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("LibreOffice did not produce %s: %w", filepath.Base(target), err)
	}
	return target, cleanup, nil
}
