package sfx

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeWAV converts interleaved float32 samples to a 16-bit PCM WAV file.
// Samples are clipped to [-1, 1] before conversion.
func EncodeWAV(samples []float32, channels, sampleRate int) ([]byte, error) {
	if channels <= 0 {
		return nil, fmt.Errorf("channels must be positive")
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sample rate must be positive")
	}
	if len(samples)%channels != 0 {
		return nil, fmt.Errorf("sample count %d is not a multiple of %d channels", len(samples), channels)
	}

	blockAlign := channels * 2
	dataLen := len(samples) * 2
	out := make([]byte, 44+dataLen)

	copy(out[0:], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+dataLen))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(out[20:], 1)  // PCM
	binary.LittleEndian.PutUint16(out[22:], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:], uint32(sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(out[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:], 16) // bits per sample
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(dataLen))

	for i, s := range samples {
		v := math.Round(float64(clamp1(s)) * 32767)
		binary.LittleEndian.PutUint16(out[44+i*2:], uint16(int16(v)))
	}
	return out, nil
}

func clamp1(v float32) float32 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}
