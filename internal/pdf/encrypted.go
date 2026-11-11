package pdf

import (
	"bytes"
	"io"
	"os"
)

// IsEncrypted reports whether path has an /Encrypt dictionary in its trailer.
// Encrypted PDFs cannot be parsed or rendered without a password, so callers
// should return a clear error instead of a low-level parser failure.
func IsEncrypted(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	const tail = 16 << 10
	start := st.Size() - tail
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(f, tail))
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("/Encrypt"))
}
