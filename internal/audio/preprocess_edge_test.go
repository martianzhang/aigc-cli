package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// --- in-memory WAV/PCM builders ---

// buildWAVHeader returns a canonical 44-byte WAV header (RIFF/WAVE + fmt + data).
// The caller chooses the format fields; fmt is always declared as 16 bytes.
func buildWAVHeader(audioFormat, channels, bitsPerSample uint16, sampleRate, dataSize uint32) []byte {
	hdr := make([]byte, 44)
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], 36+dataSize)
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], audioFormat)
	binary.LittleEndian.PutUint16(hdr[22:24], channels)
	binary.LittleEndian.PutUint32(hdr[24:28], sampleRate)
	binary.LittleEndian.PutUint32(hdr[28:32], sampleRate*uint32(channels)*uint32(bitsPerSample)/8)
	binary.LittleEndian.PutUint16(hdr[32:34], channels*bitsPerSample/8)
	binary.LittleEndian.PutUint16(hdr[34:36], bitsPerSample)
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], dataSize)
	return hdr
}

// wrapWAV concatenates a header and its PCM payload into one in-memory WAV stream.
func wrapWAV(header, payload []byte) []byte {
	var buf bytes.Buffer
	buf.Write(header)
	buf.Write(payload)
	return buf.Bytes()
}

// setChunkAt36 rewrites the chunk tag/size stored at bytes 36:44 of a canonical
// header — the slot normally occupied by the "data" chunk header.
func setChunkAt36(hdr []byte, tag string, size uint32) {
	copy(hdr[36:40], tag)
	binary.LittleEndian.PutUint32(hdr[40:44], size)
}

// riffChunkHeader builds an 8-byte RIFF chunk header (id + little-endian size).
func riffChunkHeader(tag string, size uint32) []byte {
	hdr := make([]byte, 8)
	copy(hdr[0:4], tag)
	binary.LittleEndian.PutUint32(hdr[4:8], size)
	return hdr
}

// riffChunk builds a complete RIFF chunk, padded to an even byte boundary
// as required by the RIFF specification.
func riffChunk(tag string, payload []byte) []byte {
	out := riffChunkHeader(tag, uint32(len(payload)))
	out = append(out, payload...)
	if len(payload)%2 != 0 {
		out = append(out, 0)
	}
	return out
}

// samplesLE16 encodes int16 samples as little-endian 16-bit PCM.
func samplesLE16(values ...int16) []byte {
	out := make([]byte, 0, 2*len(values))
	for _, v := range values {
		out = binary.LittleEndian.AppendUint16(out, uint16(v))
	}
	return out
}

// samplesLE24 encodes signed values as little-endian 24-bit PCM.
func samplesLE24(values ...int32) []byte {
	out := make([]byte, 0, 3*len(values))
	for _, v := range values {
		u := uint32(v)
		out = append(out, byte(u), byte(u>>8), byte(u>>16))
	}
	return out
}

// samplesLE32 encodes signed values as little-endian 32-bit PCM.
func samplesLE32(values ...int32) []byte {
	out := make([]byte, 0, 4*len(values))
	for _, v := range values {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// assertSamplesEqual compares decoded PCM against the expected values.
func assertSamplesEqual(t *testing.T, got, want []int16) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sample count: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

// --- DecodeWAV edge cases ---

func TestDecodeWAV_emptyReader(t *testing.T) {
	_, err := DecodeWAV(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty reader")
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("error should wrap io.EOF, got: %v", err)
	}
	if !strings.Contains(err.Error(), "read header") {
		t.Errorf("error should mention the header read, got: %v", err)
	}
}

func TestDecodeWAV_shortHeader(t *testing.T) {
	_, err := DecodeWAV(bytes.NewReader([]byte("RIFF")))
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("error should wrap io.ErrUnexpectedEOF, got: %v", err)
	}
}

func TestDecodeWAV_invalidRIFFTag(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 4)
	copy(hdr[0:4], "RIFX")

	_, err := DecodeWAV(bytes.NewReader(wrapWAV(hdr, samplesLE16(1, 2))))
	if err == nil {
		t.Fatal("expected error for invalid RIFF tag")
	}
	if !strings.Contains(err.Error(), "not a valid WAV file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeWAV_invalidWAVETag(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 4)
	copy(hdr[8:12], "AVI ")

	_, err := DecodeWAV(bytes.NewReader(wrapWAV(hdr, samplesLE16(1, 2))))
	if err == nil {
		t.Fatal("expected error for invalid WAVE tag")
	}
	if !strings.Contains(err.Error(), "not a valid WAV file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeWAV_missingFmtChunk(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 4)
	copy(hdr[12:16], "junk")

	_, err := DecodeWAV(bytes.NewReader(wrapWAV(hdr, samplesLE16(1, 2))))
	if err == nil {
		t.Fatal("expected error for missing fmt chunk")
	}
	if !strings.Contains(err.Error(), "missing fmt chunk") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeWAV_unsupportedAudioFormat(t *testing.T) {
	// 0 = unknown, 2 = ADPCM, 3 = IEEE float, 6 = A-law, 7 = mu-law,
	// 0xFFFE = WAVE_FORMAT_EXTENSIBLE. Only PCM (1) is supported.
	for _, format := range []uint16{0, 2, 3, 6, 7, 0xFFFE} {
		t.Run(fmt.Sprintf("format=%d", format), func(t *testing.T) {
			hdr := buildWAVHeader(format, 1, 16, 8000, 4)

			_, err := DecodeWAV(bytes.NewReader(wrapWAV(hdr, samplesLE16(1, 2))))
			if err == nil {
				t.Fatalf("expected error for audio format %d", format)
			}
			if !strings.Contains(err.Error(), "unsupported audio format") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDecodeWAV_missingDataChunk(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 0)
	setChunkAt36(hdr, "JUNK", 0) // no data chunk anywhere in the stream

	_, err := DecodeWAV(bytes.NewReader(hdr))
	if err == nil {
		t.Fatal("expected error for missing data chunk")
	}
	if !strings.Contains(err.Error(), "data chunk not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeWAV_dataChunkSizeZero(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 0) // data chunk present but declared empty

	// A zero declared size is treated as "not found" by DecodeWAV.
	_, err := DecodeWAV(bytes.NewReader(hdr))
	if err == nil {
		t.Fatal("expected error for zero-length data chunk")
	}
	if !strings.Contains(err.Error(), "data chunk not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeWAV_truncatedSampleData(t *testing.T) {
	payload := samplesLE16(1, 2, 3, 4)
	hdr := buildWAVHeader(1, 1, 16, 8000, uint32(len(payload)+10)) // lies about the size

	_, err := DecodeWAV(bytes.NewReader(wrapWAV(hdr, payload)))
	if err == nil {
		t.Fatal("expected error for truncated sample data")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("error should wrap io.ErrUnexpectedEOF, got: %v", err)
	}
	if !strings.Contains(err.Error(), "read samples") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeWAV_extraChunkBeforeData(t *testing.T) {
	payload := samplesLE16(100, -100, 32767, -32768)
	hdr := buildWAVHeader(1, 1, 16, 8000, uint32(len(payload)))
	setChunkAt36(hdr, "JUNK", 0) // zero-length extra chunk pushes data to byte 52

	stream := wrapWAV(hdr, riffChunkHeader("data", uint32(len(payload))))
	stream = append(stream, payload...)

	got, err := DecodeWAV(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if got.SampleRate != 8000 {
		t.Errorf("sample rate: got %d, want 8000", got.SampleRate)
	}
	if got.Channels != 1 {
		t.Errorf("channels: got %d, want 1", got.Channels)
	}
	assertSamplesEqual(t, got.Samples, []int16{100, -100, 32767, -32768})
}

func TestDecodeWAV_bitsPerSample(t *testing.T) {
	tests := []struct {
		name    string
		bits    uint16
		payload []byte
		want    []int16
	}{
		{"8-bit unsigned", 8, []byte{0x80, 0xFF, 0x00}, []int16{0, 127, -128}},
		{"16-bit signed", 16, samplesLE16(1234, -1234, 32767, -32768), []int16{1234, -1234, 32767, -32768}},
		{"24-bit signed", 24, samplesLE24(1<<8, -(1 << 8), 0x7FFF00, -0x800000), []int16{1, -1, 32767, -32768}},
		{"32-bit signed", 32, samplesLE32(1<<16, -(1 << 16), 0x7FFF0000, -2147483648), []int16{1, -1, 32767, -32768}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr := buildWAVHeader(1, 1, tt.bits, 22050, uint32(len(tt.payload)))

			got, err := DecodeWAV(bytes.NewReader(wrapWAV(hdr, tt.payload)))
			if err != nil {
				t.Fatalf("DecodeWAV: %v", err)
			}
			if got.Channels != 1 {
				t.Errorf("channels: got %d, want 1", got.Channels)
			}
			assertSamplesEqual(t, got.Samples, tt.want)
		})
	}
}

func TestDecodeWAV_stereoMixesToMono(t *testing.T) {
	payload := samplesLE16(100, 200, -101, -200)
	hdr := buildWAVHeader(1, 2, 16, 44100, uint32(len(payload)))

	got, err := DecodeWAV(bytes.NewReader(wrapWAV(hdr, payload)))
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if got.SampleRate != 44100 {
		t.Errorf("sample rate: got %d, want 44100", got.SampleRate)
	}
	if got.Channels != 1 {
		t.Errorf("channels: got %d, want 1", got.Channels)
	}
	assertSamplesEqual(t, got.Samples, []int16{150, -150})
}

// --- EncodeWAV edge cases ---

func TestEncodeWAV_nilData(t *testing.T) {
	var buf bytes.Buffer

	err := EncodeWAV(&buf, nil)
	if err == nil {
		t.Fatal("expected error for nil AudioData")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for nil data, wrote %d bytes", buf.Len())
	}
}

func TestEncodeWAV_headerAndDataLayout(t *testing.T) {
	samples := []int16{1, -1, 32767, -32768}
	// Channels is ignored by EncodeWAV, which always writes mono 16-bit PCM.
	data := &AudioData{Samples: samples, SampleRate: 8000, Channels: 99}

	var buf bytes.Buffer
	if err := EncodeWAV(&buf, data); err != nil {
		t.Fatalf("EncodeWAV: %v", err)
	}
	b := buf.Bytes()
	dataSize := 2 * len(samples)
	if len(b) != 44+dataSize {
		t.Fatalf("encoded length: got %d, want %d", len(b), 44+dataSize)
	}

	if got := string(b[0:4]); got != "RIFF" {
		t.Errorf("RIFF tag: got %q, want %q", got, "RIFF")
	}
	if got := binary.LittleEndian.Uint32(b[4:8]); got != uint32(36+dataSize) {
		t.Errorf("RIFF size: got %d, want %d", got, 36+dataSize)
	}
	if got := string(b[8:12]); got != "WAVE" {
		t.Errorf("WAVE tag: got %q, want %q", got, "WAVE")
	}
	if got := string(b[12:16]); got != "fmt " {
		t.Errorf("fmt tag: got %q, want %q", got, "fmt ")
	}
	if got := binary.LittleEndian.Uint32(b[16:20]); got != 16 {
		t.Errorf("fmt chunk size: got %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint16(b[20:22]); got != 1 {
		t.Errorf("audio format: got %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(b[22:24]); got != 1 {
		t.Errorf("channels: got %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint32(b[24:28]); got != 8000 {
		t.Errorf("sample rate: got %d, want 8000", got)
	}
	if got := binary.LittleEndian.Uint32(b[28:32]); got != 16000 {
		t.Errorf("byte rate: got %d, want 16000", got)
	}
	if got := binary.LittleEndian.Uint16(b[32:34]); got != 2 {
		t.Errorf("block align: got %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint16(b[34:36]); got != 16 {
		t.Errorf("bits per sample: got %d, want 16", got)
	}
	if got := string(b[36:40]); got != "data" {
		t.Errorf("data tag: got %q, want %q", got, "data")
	}
	if got := binary.LittleEndian.Uint32(b[40:44]); got != uint32(dataSize) {
		t.Errorf("data chunk size: got %d, want %d", got, dataSize)
	}
	for i, want := range samples {
		got := int16(binary.LittleEndian.Uint16(b[44+2*i : 46+2*i]))
		if got != want {
			t.Errorf("payload sample[%d]: got %d, want %d", i, got, want)
		}
	}
}

func TestEncodeWAV_defaultSampleRate(t *testing.T) {
	tests := []struct {
		name string
		rate int
	}{
		{"zero rate", 0},
		{"negative rate", -16000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			data := &AudioData{Samples: []int16{1}, SampleRate: tt.rate}

			if err := EncodeWAV(&buf, data); err != nil {
				t.Fatalf("EncodeWAV: %v", err)
			}
			b := buf.Bytes()
			if got := binary.LittleEndian.Uint32(b[24:28]); got != 24000 {
				t.Errorf("sample rate: got %d, want default 24000", got)
			}
			if got := binary.LittleEndian.Uint32(b[28:32]); got != 48000 {
				t.Errorf("byte rate: got %d, want 48000", got)
			}
		})
	}
}

func TestEncodeWAV_roundTripInMemory(t *testing.T) {
	original := &AudioData{
		Samples:    []int16{0, 1, -1, 12345, -12345, 32767, -32768},
		SampleRate: 16000,
	}

	var buf bytes.Buffer
	if err := EncodeWAV(&buf, original); err != nil {
		t.Fatalf("EncodeWAV: %v", err)
	}

	got, err := DecodeWAV(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if got.SampleRate != original.SampleRate {
		t.Errorf("sample rate: got %d, want %d", got.SampleRate, original.SampleRate)
	}
	if got.Channels != 1 {
		t.Errorf("channels: got %d, want 1", got.Channels)
	}
	assertSamplesEqual(t, got.Samples, original.Samples)
}

// errWriter fails every write with the wrapped sentinel error.
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

func TestEncodeWAV_writerError(t *testing.T) {
	boom := errors.New("boom")
	data := &AudioData{Samples: []int16{1, 2, 3}, SampleRate: 8000}

	err := EncodeWAV(errWriter{err: boom}, data)
	if err == nil {
		t.Fatal("expected error from failing writer")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error should wrap the writer error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "write sample") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- decodePCM* edge cases ---

func TestDecodePCM16_mono(t *testing.T) {
	raw := samplesLE16(0, 513, -1, 32767, -32768)
	assertSamplesEqual(t, decodePCM16(raw, 1), []int16{0, 513, -1, 32767, -32768})
}

func TestDecodePCM16_stereo(t *testing.T) {
	raw := samplesLE16(
		100, 200, // 150
		101, 200, // 150 — .5 truncates toward zero
		-101, -200, // -150 — -.5 truncates toward zero
		32767, 32767, // 32767 — the sum must not overflow int32
		-32768, -32768, // -32768
	)
	assertSamplesEqual(t, decodePCM16(raw, 2), []int16{150, 150, -150, 32767, -32768})
}

func TestDecodePCM16_ignoresTrailingPartialFrame(t *testing.T) {
	raw := []byte{0x01, 0x00, 0xFF} // one mono frame plus a stray byte
	assertSamplesEqual(t, decodePCM16(raw, 1), []int16{1})
}

func TestDecodePCM8_unsignedToSigned(t *testing.T) {
	raw := []byte{0x00, 0x7F, 0x80, 0x81, 0xFF}
	assertSamplesEqual(t, decodePCM8(raw, 1), []int16{-128, -1, 0, 1, 127})
}

func TestDecodePCM8_stereo(t *testing.T) {
	raw := []byte{
		0x80, 0x80, // 0
		0xFF, 0x81, // 64
		0x00, 0x80, // -64
		0xFF, 0x00, // 0 — (-1)/2 truncates toward zero
	}
	assertSamplesEqual(t, decodePCM8(raw, 2), []int16{0, 64, -64, 0})
}

func TestDecodePCM24_signExtension(t *testing.T) {
	raw := samplesLE24(0, 1<<8, -(1 << 8), 0x7FFFFF, -0x800000, 512)
	assertSamplesEqual(t, decodePCM24(raw, 1), []int16{0, 1, -1, 32767, -32768, 2})
}

func TestDecodePCM24_stereo(t *testing.T) {
	raw := samplesLE24(256, 768, -512, -256)
	assertSamplesEqual(t, decodePCM24(raw, 2), []int16{2, -2})
}

func TestDecodePCM32_signExtension(t *testing.T) {
	raw := samplesLE32(0, 1<<16, -(1 << 16), 0x7FFF0000, -2147483648, -16777216)
	assertSamplesEqual(t, decodePCM32(raw, 1), []int16{0, 1, -1, 32767, -32768, -256})
}

func TestDecodePCM32_stereo(t *testing.T) {
	raw := samplesLE32(1<<16, 3<<16, -(1 << 16), -(3 << 16))
	assertSamplesEqual(t, decodePCM32(raw, 2), []int16{2, -2})
}

func TestDecodePCM_bitsPerSampleDispatch(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		bits int
		want []int16
	}{
		{"8-bit", []byte{0x81}, 8, []int16{1}},
		{"16-bit", samplesLE16(1000), 16, []int16{1000}},
		{"24-bit", samplesLE24(1 << 8), 24, []int16{1}},
		{"32-bit", samplesLE32(1 << 16), 32, []int16{1}},
		{"unsupported 12-bit falls back to 16-bit", samplesLE16(1000), 12, []int16{1000}},
		{"zero bits falls back to 16-bit", samplesLE16(-1000), 0, []int16{-1000}},
		{"unsupported 64-bit falls back to 16-bit", samplesLE16(7), 64, []int16{7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSamplesEqual(t, decodePCM(tt.raw, 1, tt.bits), tt.want)
		})
	}
}

// --- findDataChunk edge cases ---

func TestFindDataChunk_standardOffset(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 1234)

	// The reader is untouched: a data chunk at the canonical offset
	// (tag at byte 36, size at byte 40) is found in the header alone.
	if got := findDataChunk(hdr, bytes.NewReader(nil)); got != 1234 {
		t.Errorf("findDataChunk: got %d, want 1234", got)
	}
}

func TestFindDataChunk_nonStandardOffset(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 0)
	binary.LittleEndian.PutUint32(hdr[16:20], 18) // fmt chunk declares 2 extra bytes
	clear(hdr[36:44])                             // wipe the canonical data chunk slot
	hdr = append(hdr, 0, 0)                       // room for a data header at 38..46
	copy(hdr[38:42], "data")                      // data now starts at byte 38
	binary.LittleEndian.PutUint32(hdr[42:46], 1000)

	if got := findDataChunk(hdr, bytes.NewReader(nil)); got != 1000 {
		t.Errorf("findDataChunk: got %d, want 1000", got)
	}
}

func TestFindDataChunk_skipsExtraChunksInStream(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 0)
	setChunkAt36(hdr, "JUNK", 0) // zero-length chunk; the scan resumes at byte 44

	var stream bytes.Buffer
	stream.Write(riffChunk("LIST", []byte{1, 2, 3})) // odd size 3 → padded to 4 bytes
	stream.Write(riffChunkHeader("data", 500))

	if got := findDataChunk(hdr, &stream); got != 500 {
		t.Errorf("findDataChunk: got %d, want 500", got)
	}
}

func TestFindDataChunk_notFound(t *testing.T) {
	hdr := buildWAVHeader(1, 1, 16, 8000, 0)
	setChunkAt36(hdr, "JUNK", 0)

	tests := []struct {
		name   string
		stream []byte
	}{
		{"empty stream", nil},
		{"truncated chunk header", []byte("LIST")},
		{"payload overruns stream", append(riffChunkHeader("LIST", 100), 1, 2, 3, 4)},
		{"chunk without a following data chunk", riffChunk("LIST", []byte{1, 2})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findDataChunk(hdr, bytes.NewReader(tt.stream)); got != -1 {
				t.Errorf("findDataChunk: got %d, want -1", got)
			}
		})
	}
}
