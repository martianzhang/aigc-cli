package audio

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/ebitengine/oto/v3"
)

// progressReader wraps bytes.Reader and tracks how many bytes oto has consumed.
type progressReader struct {
	*bytes.Reader
	total int64
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.total += int64(n)
	return n, err
}

// PlayAudioFile decodes an audio file and plays it through speakers.
// Blocks until playback finishes. Supported formats: WAV, MP3, FLAC, OGG.
func PlayAudioFile(path string) error {
	data, err := DecodeAudioFile(path)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return PlayAudioData(data)
}

// PlayAudioData plays raw PCM audio data through speakers.
// Blocks until playback finishes.
func PlayAudioData(data *AudioData) error {
	if data.Channels <= 0 {
		data.Channels = 1
	}

	opts := &oto.NewContextOptions{
		SampleRate:   data.SampleRate,
		ChannelCount: data.Channels,
		Format:       oto.FormatSignedInt16LE,
	}

	ctx, ready, err := oto.NewContext(opts)
	if err != nil {
		return fmt.Errorf("oto init: %w", err)
	}

	<-ready

	pcm := make([]byte, len(data.Samples)*2)
	for i, s := range data.Samples {
		pcm[i*2] = byte(s)
		pcm[i*2+1] = byte(s >> 8)
	}

	pr := &progressReader{Reader: bytes.NewReader(pcm)}
	player := ctx.NewPlayer(pr)
	player.Play()

	// Calculate the earliest safe return time: full audio duration + hardware margin.
	// oto's IsPlaying/BufferedSize only account for its ring buffer, not the
	// hardware's DMA buffer (CoreAudio on macOS, ALSA on Linux). Returning before
	// the hardware finishes causes the last ~50-200ms of audio to be cut off.
	audioDur := time.Duration(float64(len(data.Samples))/float64(data.SampleRate)*float64(time.Second)) + 500*time.Millisecond
	deadline := time.Now().Add(audioDur)

	// Phase 1: wait for oto to consume all PCM bytes
	for pr.total < int64(len(pcm)) {
		time.Sleep(5 * time.Millisecond)
	}

	// Phase 2: wait for oto's internal ring buffer to drain
	for player.IsPlaying() {
		time.Sleep(5 * time.Millisecond)
	}

	// Phase 3: wait for hardware DMA buffer
	for player.BufferedSize() > 0 {
		time.Sleep(5 * time.Millisecond)
	}

	// Ensure we don't return before the audio has fully played through hardware
	if remaining := time.Until(deadline); remaining > 0 {
		time.Sleep(remaining)
	}

	return nil
}

var _ io.Reader = (*progressReader)(nil)
