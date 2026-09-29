//go:build !darwin && !windows

// Fallback for Linux/BSD: instead of linking oto (whose unix driver needs CGO +
// ALSA and would make the whole binary depend on libasound.so.2 at runtime), we
// shell out to whichever command-line player is installed. That keeps the
// binary CGO-free and portable; when no player exists the callers fall back to
// the system handler (xdg-open).

package audio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// externalPlayer is a command-line player plus its quiet flags. formats limits
// the extensions it can play; an empty list means it handles any format the CLI
// decodes (WAV, MP3, FLAC, OGG).
type externalPlayer struct {
	name    string
	args    []string
	formats []string
}

var allFormats = []string{".wav", ".mp3", ".flac", ".ogg", ".oga"}

// externalPlayers is tried in order. Universal players first (ffmpeg/mpv/vlc/
// gstreamer), then common format-specific tools found on minimal systems.
var externalPlayers = []externalPlayer{
	{name: "ffplay", args: []string{"-nodisp", "-autoexit", "-loglevel", "quiet"}},
	{name: "mpv", args: []string{"--no-video", "--really-quiet"}},
	{name: "cvlc", args: []string{"--play-and-exit", "--intf", "dummy"}},
	{name: "gst-play-1.0", args: []string{"--quiet"}, formats: allFormats},
	{name: "paplay", formats: []string{".wav", ".flac", ".ogg", ".oga"}},
	{name: "aplay", args: []string{"-q"}, formats: []string{".wav"}},
	{name: "mpg123", args: []string{"-q"}, formats: []string{".mp3"}},
	{name: "ogg123", args: []string{"-q"}, formats: []string{".ogg", ".oga"}},
	{name: "play", args: []string{"-q"}, formats: allFormats},
}

// PlayAudioFile plays path with the first available external player.
func PlayAudioFile(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	var failures []string
	for _, p := range externalPlayers {
		if len(p.formats) > 0 && !slices.Contains(p.formats, ext) {
			continue
		}
		if _, err := exec.LookPath(p.name); err != nil {
			continue
		}
		args := append(append([]string{}, p.args...), path)
		if err := exec.Command(p.name, args...).Run(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p.name, err))
			continue
		}
		return nil
	}
	if len(failures) > 0 {
		return fmt.Errorf("audio playback failed: %s", strings.Join(failures, "; "))
	}
	return ErrPlaybackUnavailable
}

// PlayAudioData writes data to a temporary WAV and plays it.
func PlayAudioData(data *AudioData) error {
	f, err := os.CreateTemp("", "aigc-cli-*.wav")
	if err != nil {
		return fmt.Errorf("create temp wav: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(name) }()

	if err := WriteWAV(name, data); err != nil {
		return err
	}
	return PlayAudioFile(name)
}
