package service

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// streamingWAV builds a 44-byte WAV header with the all-ones sentinel sizes
// that streaming encoders (e.g. Ollama's /v1/audio/speech) emit, followed by
// the given PCM payload.
func streamingWAV(payload []byte) []byte {
	hdr := make([]byte, 44)
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], 0xFFFFFFFF)
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:24], 1) // mono
	binary.LittleEndian.PutUint32(hdr[24:28], 24000)
	binary.LittleEndian.PutUint32(hdr[28:32], 24000*2)
	binary.LittleEndian.PutUint16(hdr[32:34], 2)
	binary.LittleEndian.PutUint16(hdr[34:36], 16)
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], 0xFFFFFFFF)
	return append(hdr, payload...)
}

func TestSaveAudioFile_repairsStreamingWAVSizes(t *testing.T) {
	dir := t.TempDir()
	payload := []byte{1, 0, 2, 0, 3, 0}

	path, err := SaveAudioFile(streamingWAV(payload), "wav", dir)
	if err != nil {
		t.Fatalf("SaveAudioFile: %v", err)
	}
	if got := filepath.Ext(path); got != ".wav" {
		t.Errorf("extension: got %q, want .wav", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if got, want := binary.LittleEndian.Uint32(data[4:8]), uint32(len(data)-8); got != want {
		t.Errorf("RIFF size: got %d, want %d", got, want)
	}
	if got, want := binary.LittleEndian.Uint32(data[40:44]), uint32(len(payload)); got != want {
		t.Errorf("data size: got %d, want %d", got, want)
	}
	if !bytes.Equal(data[44:], payload) {
		t.Error("payload changed while repairing the header")
	}
}

func TestSaveAudioFile_nonWAVUnchanged(t *testing.T) {
	dir := t.TempDir()
	data := []byte("ID3\x04\x00\x00\x00\x00\x00\x00")

	path, err := SaveAudioFile(data, "mp3", dir)
	if err != nil {
		t.Fatalf("SaveAudioFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("non-WAV payload was modified")
	}
}

func TestSaveAudioFile_unknownFormatUsesBinExtension(t *testing.T) {
	path, err := SaveAudioFile([]byte("raw"), "weird", t.TempDir())
	if err != nil {
		t.Fatalf("SaveAudioFile: %v", err)
	}
	if ext := filepath.Ext(path); ext != ".bin" {
		t.Errorf("extension: got %q, want .bin", ext)
	}
}

func TestSaveAudioFile_createsOutputDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "out")

	path, err := SaveAudioFile([]byte("x"), "wav", dir)
	if err != nil {
		t.Fatalf("SaveAudioFile: %v", err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("saved path %q is outside %q", path, dir)
	}
}
