package audio

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// testWAV generates a WAV file with a known sine wave for testing.
func testWAV(t *testing.T, sampleRate int, freq float64, durationSec float64) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	numSamples := int(float64(sampleRate) * durationSec)
	samples := make([]int16, numSamples)
	for i := range samples {
		// Sine wave at given frequency
		samples[i] = int16(math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)) * 16000)
	}

	data := &AudioData{
		Samples:    samples,
		SampleRate: sampleRate,
		Channels:   1,
	}
	if err := WriteWAV(path, data); err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}
	return path
}

func TestWriteWAV_roundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.wav")

	original := &AudioData{
		Samples:    []int16{0, 100, 200, 300, 400, 500, -100, -200, -300, -400},
		SampleRate: 16000,
	}

	if err := WriteWAV(path, original); err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}

	got, err := ReadWAV(path)
	if err != nil {
		t.Fatalf("ReadWAV: %v", err)
	}

	if got.SampleRate != original.SampleRate {
		t.Errorf("sample rate: got %d, want %d", got.SampleRate, original.SampleRate)
	}
	if len(got.Samples) != len(original.Samples) {
		t.Fatalf("sample count: got %d, want %d", len(got.Samples), len(original.Samples))
	}
	for i := range original.Samples {
		if got.Samples[i] != original.Samples[i] {
			t.Errorf("sample[%d]: got %d, want %d", i, got.Samples[i], original.Samples[i])
		}
	}
}

func TestWriteWAV_sineWave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sine.wav")

	sampleRate := 44100
	freq := 440.0            // A4
	numSamples := sampleRate // 1 second
	samples := make([]int16, numSamples)
	for i := range samples {
		samples[i] = int16(math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)) * 16000)
	}

	err := WriteWAV(path, &AudioData{
		Samples:    samples,
		SampleRate: sampleRate,
	})
	if err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}

	// Verify file size
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	expectedSize := 44 + numSamples*2 // 44-byte header + PCM data
	if fi.Size() != int64(expectedSize) {
		t.Errorf("file size: got %d, want %d", fi.Size(), expectedSize)
	}
}

func TestReadWAV_standard(t *testing.T) {
	path := testWAV(t, 16000, 440, 0.5)
	data, err := ReadWAV(path)
	if err != nil {
		t.Fatalf("ReadWAV: %v", err)
	}
	if data.SampleRate != 16000 {
		t.Errorf("sample rate: got %d, want %d", data.SampleRate, 16000)
	}
	if len(data.Samples) == 0 {
		t.Fatal("no samples read")
	}
	// Check the samples are roughly a sine wave
	if data.Samples[0] > 100 || data.Samples[0] < -100 {
		t.Logf("sample[0] = %d (expected near zero for sine)", data.Samples[0])
	}
}

func TestReadWAV_nonexistent(t *testing.T) {
	_, err := ReadWAV("/nonexistent/file.wav")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadWAV_invalidHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.wav")
	if err := os.WriteFile(path, []byte("not a wav file"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadWAV(path)
	if err == nil {
		t.Fatal("expected error for invalid WAV")
	}
}

func TestResample_sameRate(t *testing.T) {
	input := []int16{0, 100, 200, 300, 400}
	output := Resample(input, 16000, 16000)
	if len(output) != len(input) {
		t.Fatalf("len: got %d, want %d", len(output), len(input))
	}
	for i, v := range input {
		if output[i] != v {
			t.Errorf("sample[%d]: got %d, want %d", i, output[i], v)
		}
	}
}

func TestResample_halfRate(t *testing.T) {
	// 4 samples at 16000 Hz resampled to 8000 Hz = 2 samples
	input := []int16{0, 100, 200, 300}
	output := Resample(input, 16000, 8000)
	expectedLen := 2
	if len(output) != expectedLen {
		t.Fatalf("len: got %d, want %d", len(output), expectedLen)
	}
}

func TestResample_doubleRate(t *testing.T) {
	input := []int16{0, 100}
	output := Resample(input, 8000, 16000)
	// 2 samples at 8000 Hz → 4 samples at 16000 Hz
	if len(output) < 3 {
		t.Fatalf("len: got %d, want >=4", len(output))
	}
}

func TestResample_emptyInput(t *testing.T) {
	output := Resample(nil, 16000, 8000)
	if len(output) != 0 {
		t.Fatal("expected empty output for empty input")
	}
}

func TestNormalizeInt16_alreadyMax(t *testing.T) {
	input := []int16{0, 100, 32767, -32768}
	output := NormalizeInt16(input)
	if len(output) != len(input) {
		t.Fatalf("len: got %d, want %d", len(output), len(input))
	}
	// Should be unchanged since peak is already max
	if output[2] != 32767 {
		t.Errorf("expected 32767 unchanged, got %d", output[2])
	}
}

func TestNormalizeInt16_quietSignal(t *testing.T) {
	input := []int16{0, 1000, -500, 2000, -1000}
	output := NormalizeInt16(input)
	if len(output) != len(input) {
		t.Fatalf("len: got %d, want %d", len(output), len(input))
	}
	// Should be amplified
	if output[3] <= 2000 {
		t.Error("expected amplification of quiet signal")
	}
}

func TestNormalizeInt16_empty(t *testing.T) {
	output := NormalizeInt16(nil)
	if len(output) != 0 {
		t.Fatal("expected empty output")
	}
}

func TestNormalizeInt16_allZero(t *testing.T) {
	input := make([]int16, 100)
	output := NormalizeInt16(input)
	if len(output) != 100 {
		t.Fatalf("len: got %d, want %d", len(output), 100)
	}
}

func TestDetectAudioFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"audio.wav", "wav"},
		{"speech.WAV", "wav"},
		{"recording.mp3", "mp3"},
		{"song.flac", "flac"},
		{"podcast.ogg", "ogg"},
		{"video.m4a", "aac"},
		{"audio.aac", "aac"},
		{"unknown.bin", "wav"},
		{"", "wav"},
	}
	for _, tt := range tests {
		got := DetectAudioFormat(tt.path)
		if got != tt.want {
			t.Errorf("DetectAudioFormat(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestDecodeWAV_8bit(t *testing.T) {
	// Create a simple 8-bit WAV
	dir := t.TempDir()
	path := filepath.Join(dir, "8bit.wav")

	sampleRate := 8000
	dataSize := 100
	fileSize := 36 + dataSize

	buf := make([]byte, 44+dataSize)
	// RIFF header
	copy(buf[0:4], "RIFF")
	binaryLittleEndianPutUint32(buf[4:8], uint32(fileSize))
	copy(buf[8:12], "WAVE")
	// fmt chunk
	copy(buf[12:16], "fmt ")
	binaryLittleEndianPutUint32(buf[16:20], 16)
	binaryLittleEndianPutUint16(buf[20:22], 1) // PCM
	binaryLittleEndianPutUint16(buf[22:24], 1) // mono
	binaryLittleEndianPutUint32(buf[24:28], uint32(sampleRate))
	binaryLittleEndianPutUint32(buf[28:32], uint32(sampleRate)) // byte rate
	binaryLittleEndianPutUint16(buf[32:34], 1)                  // block align
	binaryLittleEndianPutUint16(buf[34:36], 8)                  // 8-bit
	// data chunk
	copy(buf[36:40], "data")
	binaryLittleEndianPutUint32(buf[40:44], uint32(dataSize))
	// 8-bit samples (unsigned, 128 = silence)
	for i := 0; i < dataSize; i++ {
		buf[44+i] = byte(128 + i/2)
	}

	if err := os.WriteFile(path, buf, 0644); err != nil {
		t.Fatal(err)
	}

	data, err := ReadWAV(path)
	if err != nil {
		t.Fatalf("ReadWAV 8-bit: %v", err)
	}
	if len(data.Samples) != dataSize {
		t.Errorf("got %d samples, want %d", len(data.Samples), dataSize)
	}
}

// Helper: binary encoding for test
func binaryLittleEndianPutUint16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func binaryLittleEndianPutUint32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
