package knowledge

import (
	"archive/tar"
	"bytes"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/fsutil"
)

func TestAddFileToTarUsesPrivateMode(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := addFileToTar(tw, "identity.txt", []byte("secret")); err != nil {
		t.Fatalf("addFileToTar() error = %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	hdr, err := tar.NewReader(&buf).Next()
	if err != nil {
		t.Fatalf("next header: %v", err)
	}
	if hdr.Mode != int64(fsutil.PrivateFileMode) {
		t.Errorf("tar header mode = %04o, want %04o", hdr.Mode, int64(fsutil.PrivateFileMode))
	}
}
