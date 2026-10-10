package decision

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/service"
)

// maxMediaBytes caps a local audio/video clip embedded as a data URL at 64 MiB.
const maxMediaBytes = 64 << 20

// mediaFormat maps a container extension to the MIME type a data URL must
// declare so the model server can pick a decoder. Kept explicit because
// mime.TypeByExtension is OS-dependent for several of these extensions.
type mediaFormat struct{ ext, mime string }

var audioFormats = []mediaFormat{
	{".wav", "audio/wav"},
	{".mp3", "audio/mpeg"},
	{".m4a", "audio/mp4"},
	{".aac", "audio/aac"},
	{".ogg", "audio/ogg"},
	{".oga", "audio/ogg"},
	{".opus", "audio/opus"},
	{".flac", "audio/flac"},
	{".webm", "audio/webm"},
}

var videoFormats = []mediaFormat{
	{".mp4", "video/mp4"},
	{".m4v", "video/mp4"},
	{".mov", "video/quicktime"},
	{".webm", "video/webm"},
	{".mkv", "video/x-matroska"},
	{".avi", "video/x-msvideo"},
}

// resolveAudio normalizes attached audio clips to base64 data URLs
// (e.g. data:audio/wav;base64,...). CLI --audio wins over bank.audio; blank
// entries are skipped.
func resolveAudio(cliAudio, bankAudio []string) ([]string, error) {
	return resolveMedia(cliAudio, bankAudio, "audio")
}

// resolveVideos normalizes attached video clips to base64 data URLs. Footage is
// sampled at 2 frames per second server-side, with its soundtrack when present.
func resolveVideos(cliVideos, bankVideos []string) ([]string, error) {
	return resolveMedia(cliVideos, bankVideos, "video")
}

// resolveMedia converts each entry (a local file or a base64 data URI) to a
// base64 data URL. Audio and video are sent as data URLs, not raw base64 like
// images, so the MIME type travels with the bytes: the System One protocol
// decodes a data: prefix and otherwise treats a bare string as a file path.
func resolveMedia(cli, bank []string, kind string) ([]string, error) {
	inputs := cli
	if len(inputs) == 0 {
		inputs = bank
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(inputs))
	for _, in := range inputs {
		in = strings.TrimSpace(in)
		if in == "" {
			continue
		}
		uri, err := mediaDataURI(in, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, uri)
	}
	return out, nil
}

// mediaDataURI resolves one entry to a base64 data URL: a validated local file
// is read and typed; an existing base64 data: URI is kept as-is. http(s) URLs
// are rejected, mirroring --image (the decision API does not fetch URLs).
func mediaDataURI(in, kind string) (string, error) {
	switch {
	case service.IsFile(in):
		info, err := os.Stat(in)
		if err != nil {
			return "", fmt.Errorf("read %s %q: %w", kind, in, err)
		}
		if info.Size() > maxMediaBytes {
			return "", fmt.Errorf("local %s exceeds %d MiB: %s", kind, maxMediaBytes>>20, in)
		}
		data, err := os.ReadFile(in)
		if err != nil {
			return "", fmt.Errorf("read %s %q: %w", kind, in, err)
		}
		mime, ok := mediaMIME(data, in, kind)
		if !ok {
			return "", fmt.Errorf("%q is not a recognized %s file (supported: %s)", in, kind, supportedMediaExts(kind))
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	case strings.HasPrefix(in, "data:"):
		if err := validateMediaDataURI(in); err != nil {
			return "", err
		}
		return in, nil
	default:
		return "", fmt.Errorf("%s %q is not a local file or data URI (the decision API does not fetch URLs; download it first)", kind, in)
	}
}

// validateMediaDataURI checks a caller-supplied entry is a base64 data: URI.
func validateMediaDataURI(uri string) error {
	comma := strings.Index(uri, ",")
	if comma < 0 {
		return fmt.Errorf("invalid data URI: missing comma")
	}
	if !strings.Contains(uri[:comma], ";base64") {
		return fmt.Errorf("data URI must be base64-encoded")
	}
	if uri[comma+1:] == "" {
		return fmt.Errorf("data URI contains no data")
	}
	return nil
}

// mediaMIME resolves the MIME type for a local clip: the known extension first,
// then a magic-byte sniff. ok is false when neither matches, so an arbitrary
// file (e.g. ~/.ssh/id_rsa) is never embedded as a media clip.
func mediaMIME(data []byte, path, kind string) (string, bool) {
	if m := mimeByExt(strings.ToLower(filepath.Ext(path)), kind); m != "" {
		return m, true
	}
	if m := http.DetectContentType(data); strings.HasPrefix(m, kind+"/") {
		return strings.TrimSpace(strings.SplitN(m, ";", 2)[0]), true
	}
	return "", false
}

func mediaFormats(kind string) []mediaFormat {
	if kind == "audio" {
		return audioFormats
	}
	return videoFormats
}

func mimeByExt(ext, kind string) string {
	for _, f := range mediaFormats(kind) {
		if f.ext == ext {
			return f.mime
		}
	}
	return ""
}

func supportedMediaExts(kind string) string {
	formats := mediaFormats(kind)
	exts := make([]string, len(formats))
	for i, f := range formats {
		exts[i] = f.ext
	}
	return strings.Join(exts, ", ")
}
