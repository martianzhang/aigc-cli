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

// Install downloads url to dest. An existing destination with a matching (or
// unspecified) checksum is reused. Partial downloads resume via Range.
func Install(ctx context.Context, client *http.Client, url, dest string, opts Options) error {
	if client == nil {
		client = http.DefaultClient
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}
	if !opts.Force {
		if _, err := os.Stat(dest); err == nil {
			if opts.SHA256 == "" {
				return nil
			}
			if verify(dest, opts.SHA256) == nil {
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
	if opts.SHA256 != "" {
		if err := verify(part, opts.SHA256); err != nil {
			_ = os.Remove(part)
			return err
		}
	}
	if err := os.Rename(part, dest); err != nil {
		// Windows does not replace an existing target.
		_ = os.Remove(dest)
		if retryErr := os.Rename(part, dest); retryErr != nil {
			_ = os.Remove(part)
			return fmt.Errorf("install model: %w", retryErr)
		}
	}
	return nil
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
