package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
)

// AudioData holds decoded PCM audio.
type AudioData struct {
	Samples    []int16 // PCM 16-bit signed samples
	SampleRate int     // sample rate in Hz
	Channels   int     // number of channels
}

// ReadWAV decodes a WAV file and returns PCM 16-bit mono samples.
// Multi-channel audio is averaged to mono.
func ReadWAV(path string) (*AudioData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	return DecodeWAV(f)
}

// DecodeWAV decodes WAV audio from a reader and returns PCM 16-bit mono samples.
func DecodeWAV(r io.Reader) (*AudioData, error) {
	var header [44]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Validate RIFF header
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a valid WAV file")
	}

	// Read fmt chunk
	if string(header[12:16]) != "fmt " {
		return nil, fmt.Errorf("missing fmt chunk")
	}

	// Parse format fields (PCM = 1)
	audioFormat := binary.LittleEndian.Uint16(header[20:22])
	numChannels := int(binary.LittleEndian.Uint16(header[22:24]))
	sampleRate := int(binary.LittleEndian.Uint32(header[24:28]))
	bitsPerSample := int(binary.LittleEndian.Uint16(header[34:36]))

	if audioFormat != 1 {
		return nil, fmt.Errorf("unsupported audio format %d (only PCM supported)", audioFormat)
	}

	// Find data chunk — handle extra chunks between fmt and data
	dataSize := findDataChunk(header[:], r)
	if dataSize <= 0 {
		return nil, fmt.Errorf("data chunk not found")
	}

	// Read raw sample data
	raw := make([]byte, dataSize)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, fmt.Errorf("read samples: %w", err)
	}

	// Decode to mono int16
	samples := decodePCM(raw, numChannels, bitsPerSample)

	return &AudioData{
		Samples:    samples,
		SampleRate: sampleRate,
		Channels:   1, // always mono
	}, nil
}

// findDataChunk locates the "data" chunk in a WAV stream, reading past any
// extra chunks between fmt and data. Returns the data chunk size.
// header is the first 44 bytes; extra reads go through r.
func findDataChunk(header []byte, r io.Reader) int {
	// First check if data chunk is at the expected offset (byte 40)
	if len(header) >= 44 && string(header[36:40]) == "data" {
		return int(binary.LittleEndian.Uint32(header[40:44]))
	}

	// Data chunk is elsewhere — scan forward past extra chunks
	// Start right after the fmt chunk (which ends at offset 20 + subchunk1Size)
	fmtSize := int(binary.LittleEndian.Uint32(header[16:20]))
	offset := 20 + fmtSize

	// Read remaining bytes if we have header data past the standard 44 bytes
	remaining := header[offset:]
	if len(remaining) >= 8 {
		if string(remaining[0:4]) == "data" {
			return int(binary.LittleEndian.Uint32(remaining[4:8]))
		}
		skipSize := int(binary.LittleEndian.Uint32(remaining[4:8]))
		offset += 8 + skipSize
	}

	// Scan through the stream
	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(r, chunkHeader[:]); err != nil {
			return -1
		}
		chunkSize := int(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		if string(chunkHeader[0:4]) == "data" {
			return chunkSize
		}
		// Skip this chunk (round up to even boundary per RIFF spec)
		skip := chunkSize
		if skip%2 != 0 {
			skip++
		}
		if _, err := io.CopyN(io.Discard, r, int64(skip)); err != nil {
			return -1
		}
	}
}

// decodePCM converts raw PCM bytes to mono int16 samples.
func decodePCM(raw []byte, numChannels, bitsPerSample int) []int16 {
	switch bitsPerSample {
	case 8:
		return decodePCM8(raw, numChannels)
	case 16:
		return decodePCM16(raw, numChannels)
	case 24:
		return decodePCM24(raw, numChannels)
	case 32:
		return decodePCM32(raw, numChannels)
	default:
		// Fallback: treat as 16-bit
		return decodePCM16(raw, numChannels)
	}
}

func decodePCM16(raw []byte, numChannels int) []int16 {
	frameSize := 2 * numChannels
	frameCount := len(raw) / frameSize
	out := make([]int16, frameCount)
	for i := 0; i < frameCount; i++ {
		frame := raw[i*frameSize : i*frameSize+frameSize]
		var sum int32
		for ch := 0; ch < numChannels; ch++ {
			sum += int32(int16(binary.LittleEndian.Uint16(frame[2*ch : 2*ch+2])))
		}
		out[i] = int16(sum / int32(numChannels))
	}
	return out
}

func decodePCM8(raw []byte, numChannels int) []int16 {
	frameSize := 1 * numChannels
	frameCount := len(raw) / frameSize
	out := make([]int16, frameCount)
	for i := 0; i < frameCount; i++ {
		frame := raw[i*frameSize : i*frameSize+frameSize]
		var sum int32
		for ch := 0; ch < numChannels; ch++ {
			sum += int32(frame[ch]) - 128 // unsigned 8-bit to signed
		}
		out[i] = int16(sum / int32(numChannels))
	}
	return out
}

func decodePCM24(raw []byte, numChannels int) []int16 {
	frameSize := 3 * numChannels
	frameCount := len(raw) / frameSize
	out := make([]int16, frameCount)
	for i := 0; i < frameCount; i++ {
		frame := raw[i*frameSize : i*frameSize+frameSize]
		var sum int32
		for ch := 0; ch < numChannels; ch++ {
			// 24-bit signed, little-endian
			s := int32(int8(frame[3*ch+2]))<<16 | int32(frame[3*ch+1])<<8 | int32(frame[3*ch])
			sum += s
		}
		out[i] = int16(sum / int32(numChannels) >> 8) // scale down to 16-bit
	}
	return out
}

func decodePCM32(raw []byte, numChannels int) []int16 {
	frameSize := 4 * numChannels
	frameCount := len(raw) / frameSize
	out := make([]int16, frameCount)
	for i := 0; i < frameCount; i++ {
		frame := raw[i*frameSize : i*frameSize+frameSize]
		var sum int32
		for ch := 0; ch < numChannels; ch++ {
			sum += int32(int32(binary.LittleEndian.Uint32(frame[4*ch:4*ch+4])) >> 16)
		}
		out[i] = int16(sum / int32(numChannels))
	}
	return out
}

// WriteWAV encodes PCM 16-bit mono samples as a WAV file.
func WriteWAV(path string, data *AudioData) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	return EncodeWAV(f, data)
}

// EncodeWAV encodes PCM audio as WAV and writes it to w.
func EncodeWAV(w io.Writer, data *AudioData) error {
	if data == nil {
		return fmt.Errorf("audio data is nil")
	}
	samples := data.Samples
	sampleRate := data.SampleRate
	if sampleRate <= 0 {
		sampleRate = 24000
	}

	numChannels := 1
	bitsPerSample := 16
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := len(samples) * blockAlign
	fileSize := 36 + dataSize

	// Write RIFF header
	writeString(w, "RIFF")
	writeLE32(w, uint32(fileSize))
	writeString(w, "WAVE")

	// fmt chunk
	writeString(w, "fmt ")
	writeLE32(w, 16) // subchunk1 size (PCM = 16)
	writeLE16(w, 1)  // audio format (PCM)
	writeLE16(w, uint16(numChannels))
	writeLE32(w, uint32(sampleRate))
	writeLE32(w, uint32(byteRate))
	writeLE16(w, uint16(blockAlign))
	writeLE16(w, uint16(bitsPerSample))

	// data chunk
	writeString(w, "data")
	writeLE32(w, uint32(dataSize))

	// Write samples
	for _, s := range samples {
		var buf [2]byte
		binary.LittleEndian.PutUint16(buf[:], uint16(s))
		if _, err := w.Write(buf[:]); err != nil {
			return fmt.Errorf("write sample: %w", err)
		}
	}
	return nil
}

// Resample changes the sample rate of PCM audio using linear interpolation.
func Resample(input []int16, inputRate, outputRate int) []int16 {
	if inputRate == outputRate {
		out := make([]int16, len(input))
		copy(out, input)
		return out
	}

	outputLen := int(math.Ceil(float64(len(input)) * float64(outputRate) / float64(inputRate)))
	output := make([]int16, outputLen)
	ratio := float64(inputRate) / float64(outputRate)

	for i := range output {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= len(input)-1 {
			output[i] = input[len(input)-1]
		} else {
			// Linear interpolation
			output[i] = int16(float64(input[srcIdx])*(1-frac) + float64(input[srcIdx+1])*frac)
		}
	}
	return output
}

// NormalizeInt16 scales PCM samples to use the full int16 range.
func NormalizeInt16(samples []int16) []int16 {
	if len(samples) == 0 {
		return samples
	}

	// Find peak
	maxAbs := 0
	for _, s := range samples {
		abs := int(s)
		if abs < 0 {
			abs = -abs
		}
		if abs > maxAbs {
			maxAbs = abs
		}
	}

	if maxAbs == 0 || maxAbs >= 32767 {
		return samples
	}

	// Scale to 90% of max to avoid clipping
	target := 32767 * 0.9
	scale := target / float64(maxAbs)

	out := make([]int16, len(samples))
	for i, s := range samples {
		out[i] = int16(float64(s) * scale)
	}
	return out
}

// --- helper pool ---

var pool16 = sync.Pool{New: func() any { return make([]int16, 0, 4096) }}

// --- small helpers ---

func writeString(w io.Writer, s string) {
	io.WriteString(w, s) //nolint:errcheck
}

func writeLE16(w io.Writer, v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	w.Write(buf[:]) //nolint:errcheck
}

func writeLE32(w io.Writer, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	w.Write(buf[:]) //nolint:errcheck
}

// --- stream-based Fbank (for ASR, placeholder for now) ---

// MelFilterBank computes log-mel spectrogram features.
// This is a placeholder — full implementation is model-specific.
type MelFilterBank struct {
	numFilters int
	sampleRate int
}

// NewMelFilterBank creates a mel filter bank for feature extraction.
func NewMelFilterBank(numFilters, sampleRate int) *MelFilterBank {
	return &MelFilterBank{numFilters: numFilters, sampleRate: sampleRate}
}

// Compute extracts log-mel spectrogram from PCM audio.
// Returns a 2D slice [numFrames][numFilters]float32.
func (m *MelFilterBank) Compute(samples []int16) [][]float32 {
	// Placeholder: returns zeros with the right shape
	// Full implementation requires FFT + mel scaling + log
	frameShift := int(0.010 * float64(m.sampleRate)) // 10ms
	numFrames := len(samples)/frameShift + 1

	frames := make([][]float32, numFrames)
	for i := range frames {
		frames[i] = make([]float32, m.numFilters)
	}
	return frames
}

// DetectAudioFormat guesses the audio format from a file extension.
func DetectAudioFormat(path string) string {
	ext := strings.ToLower(path)
	switch {
	case strings.HasSuffix(ext, ".wav"):
		return "wav"
	case strings.HasSuffix(ext, ".mp3"):
		return "mp3"
	case strings.HasSuffix(ext, ".flac"):
		return "flac"
	case strings.HasSuffix(ext, ".ogg"):
		return "ogg"
	case strings.HasSuffix(ext, ".m4a"), strings.HasSuffix(ext, ".aac"):
		return "aac"
	default:
		return "wav"
	}
}

// --- sort helpers for model registry ---

// SortModelsByID sorts a slice of ModelInfo by ID in place.
func SortModelsByID(models []ModelInfo) {
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
}
