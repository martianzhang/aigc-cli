// Package fsutil provides small filesystem helpers shared across packages.
package fsutil

import "os"

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
