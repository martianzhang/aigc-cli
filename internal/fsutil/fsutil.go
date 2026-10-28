// Package fsutil provides small filesystem helpers shared across packages.
package fsutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PrivateFileMode is the permission for files that may hold secrets, such as the
// config file (API keys) and the encrypted vault documents: owner read/write only.
const PrivateFileMode os.FileMode = 0o600

// WritePrivate writes data to path with PrivateFileMode. os.WriteFile only
// applies perm when it creates a file, so an explicit Chmod follows to also
// tighten a pre-existing file that had broader permissions.
func WritePrivate(path string, data []byte) error {
	if err := os.WriteFile(path, data, PrivateFileMode); err != nil {
		return err
	}
	return os.Chmod(path, PrivateFileMode)
}

// WriteFileAtomic writes data to path atomically. It writes a temporary file in
// the same directory, syncs it, then renames it over path. If any step fails the
// destination is left untouched, so an existing file is never truncated.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	_, err := WriteFileAtomicFromReader(path, bytes.NewReader(data), perm)
	return err
}

// WriteFileAtomicFromReader streams r into path atomically and returns the
// number of bytes written. The temporary file lives in the destination
// directory so the final rename stays on the same filesystem. Missing parent
// directories are created (mode 0755) so callers can write straight into a
// fresh -o/--output directory without a manual mkdir -p.
func WriteFileAtomicFromReader(path string, r io.Reader, perm os.FileMode) (int64, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return 0, fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()

	// cleanup closes and removes the temporary file after a failure. The
	// destination has not been touched at this point.
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	written, err := io.Copy(tmp, r)
	if err != nil {
		cleanup()
		return written, fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return written, fmt.Errorf("set permissions on %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return written, fmt.Errorf("sync %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return written, fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return written, fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return written, nil
}
