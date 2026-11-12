// Package modelstore downloads model files with optional SHA256 verification
// and HTTP Range resume.
package modelstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Options controls one file install.
type Options struct {
	Force    bool
	SHA256   string // lowercase hex digest; empty disables verification
	Progress func(done, total int64)
}

// Install downloads url to dest. An existing destination with a matching
// checksum is reused; partial downloads resume via Range.
//
// Checksum policy:
//   - opts.SHA256 is authoritative when set (published release digest).
//   - Otherwise a sidecar dest+".sha256" acts as a TOFU allowlist: the first
//     successful install records the digest, and later installs verify it.
//     A legacy file without a sidecar is pinned on first sight.
//
// This gives every model a stable local digest even when the upstream release
// does not publish one.
func Install(ctx context.Context, client *http.Client, url, dest string, opts Options) error {
	if client == nil {
		client = http.DefaultClient
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}
	expected := opts.SHA256
	if expected == "" {
		if pin, ok := readPin(dest); ok {
			expected = pin
		}
	}
	if !opts.Force {
		if _, err := os.Stat(dest); err == nil {
			if expected == "" {
				// Legacy install without a published digest: pin what is on disk.
				sum, err := digest(dest)
				if err == nil {
					_ = writePin(dest, sum)
					return nil
				}
			} else if verify(dest, expected) == nil {
				return nil
			}
			// Checksum mismatch: re-download instead of trusting a corrupt file.
		}
	}

	part := dest + ".part"
	var offset int64
	if info, err := os.Stat(part); err == nil && !opts.Force {
		offset = info.Size()
	}
	if opts.Force {
		offset = 0
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		if offset == 0 {
			flags |= os.O_TRUNC
		} else {
			flags |= os.O_APPEND
		}
	case http.StatusOK:
		offset = 0
		flags |= os.O_TRUNC
	default:
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return fmt.Errorf("open partial model: %w", err)
	}
	total := resp.ContentLength
	if total > 0 {
		total += offset
	}
	if _, copyErr := io.Copy(f, &progressReader{r: resp.Body, done: offset, total: total, progress: opts.Progress}); copyErr != nil {
		_ = f.Close()
		return fmt.Errorf("write partial model: %w", copyErr)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync partial model: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close partial model: %w", err)
	}
	if expected != "" {
		if err := verify(part, expected); err != nil {
			_ = os.Remove(part)
			return err
		}
	} else {
		sum, err := digest(part)
		if err != nil {
			_ = os.Remove(part)
			return fmt.Errorf("hash partial model: %w", err)
		}
		expected = sum
	}
	if err := os.Rename(part, dest); err != nil {
		// Windows does not replace an existing target.
		_ = os.Remove(dest)
		if retryErr := os.Rename(part, dest); retryErr != nil {
			_ = os.Remove(part)
			return fmt.Errorf("install model: %w", retryErr)
		}
	}
	if err := writePin(dest, expected); err != nil {
		return fmt.Errorf("record model checksum: %w", err)
	}
	return nil
}

// pinPath is the sidecar TOFU allowlist for one model file.
func pinPath(dest string) string { return dest + ".sha256" }

func readPin(dest string) (string, bool) {
	data, err := os.ReadFile(pinPath(dest))
	if err != nil {
		return "", false
	}
	sum := strings.TrimSpace(string(data))
	if len(sum) != 64 {
		return "", false
	}
	return strings.ToLower(sum), true
}

func writePin(dest, sum string) error {
	return os.WriteFile(pinPath(dest), []byte(strings.ToLower(sum)+"\n"), 0o644)
}

func digest(path string) (string, error) {
	h := sha256.New()
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type progressReader struct {
	r        io.Reader
	done     int64
	total    int64
	progress func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.progress != nil {
		p.progress(p.done, p.total)
	}
	return n, err
}

func verify(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}
