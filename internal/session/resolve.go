package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Resolve maps a --session value to a file. A value containing a path
// separator or ending in .jsonl is a path; otherwise it is an id under dir.
// Missing ids are errors, while missing paths are allowed (they create a new
// session at that path).
func Resolve(value, dir string) (Target, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Target{}, fmt.Errorf("session id or path is empty")
	}

	if looksLikePath(value) {
		info, err := os.Stat(value)
		switch {
		case err == nil:
			if info.IsDir() {
				return Target{}, fmt.Errorf("session path %s is a directory", value)
			}
			return Target{Path: value, ID: idFromPath(value), Exists: true}, nil
		case !os.IsNotExist(err):
			return Target{}, fmt.Errorf("stat session %s: %w", value, err)
		}
		id := idFromPath(value)
		if id == "" {
			id = NewID(time.Now())
		}
		return Target{Path: value, ID: id, Exists: false}, nil
	}

	if !idPattern.MatchString(value) {
		return Target{}, fmt.Errorf("invalid session id %q (allowed: letters, digits, '.', '_', '-')", value)
	}
	path := filepath.Join(dir, value+".jsonl")
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return Target{}, fmt.Errorf("session path %s is a directory", path)
		}
		return Target{Path: path, ID: value, Exists: true}, nil
	case !os.IsNotExist(err):
		return Target{}, fmt.Errorf("stat session %s: %w", path, err)
	}
	return Target{}, fmt.Errorf("session id %q not found in %s", value, dir)
}

func looksLikePath(value string) bool {
	if filepath.IsAbs(value) {
		return true
	}
	if strings.ContainsAny(value, `/\`) {
		return true
	}
	return strings.HasSuffix(strings.ToLower(value), ".jsonl")
}

func idFromPath(path string) string {
	base := filepath.Base(path)
	if strings.HasSuffix(strings.ToLower(base), ".jsonl") {
		base = base[:len(base)-len(".jsonl")]
	}
	if base == "." || base == string(filepath.Separator) || !idPattern.MatchString(base) {
		return ""
	}
	return base
}

// NewID returns a sortable, human-typeable id: UTC timestamp plus 4 random
// bytes.
func NewID(now time.Time) string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%08x", now.UTC().Format("20060102T150405Z"), uint32(now.UnixNano()))
	}
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(buf[:])
}
