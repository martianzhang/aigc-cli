package sfx

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeWAVHeader(t *testing.T) {
	samples := []float32{0.5, -0.5, 0.25, -0.25}
	data, err := EncodeWAV(samples, 2, 44100)
	if err != nil {
		t.Fatalf("EncodeWAV() error = %v", err)
	}
	if len(data) != 44+len(samples)*2 {
		t.Fatalf("len = %d, want %d", len(data), 44+len(samples)*2)
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" || string(data[36:40]) != "data" {
		t.Fatalf("missing RIFF/WAVE/data markers")
	}
	if got := binary.LittleEndian.Uint16(data[22:]); got != 2 {
		t.Errorf("channels = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(data[24:]); got != 44100 {
		t.Errorf("sample rate = %d", got)
	}
	if got := binary.LittleEndian.Uint16(data[34:]); got != 16 {
		t.Errorf("bits per sample = %d", got)
	}
	if got := binary.LittleEndian.Uint32(data[40:]); got != uint32(len(samples)*2) {
		t.Errorf("data size = %d", got)
	}

	want := []int16{16384, -16384, 8192, -8192}
	for i, w := range want {
		got := int16(binary.LittleEndian.Uint16(data[44+i*2:]))
		if got != w {
			t.Errorf("sample %d = %d, want %d", i, got, w)
		}
	}
}

func TestEncodeWAVClips(t *testing.T) {
	data, err := EncodeWAV([]float32{2, -2}, 2, 44100)
	if err != nil {
		t.Fatalf("EncodeWAV() error = %v", err)
	}
	left := int16(binary.LittleEndian.Uint16(data[44:]))
	right := int16(binary.LittleEndian.Uint16(data[46:]))
	if left != 32767 || right != -32767 {
		t.Errorf("clipped samples = %d, %d", left, right)
	}
}

func TestEncodeWAVValidation(t *testing.T) {
	if _, err := EncodeWAV([]float32{0}, 0, 44100); err == nil {
		t.Error("zero channels must error")
	}
	if _, err := EncodeWAV([]float32{0}, 1, 0); err == nil {
		t.Error("zero sample rate must error")
	}
	if _, err := EncodeWAV([]float32{0, 0, 0}, 2, 44100); err == nil {
		t.Error("odd sample count with 2 channels must error")
	}
}

func TestEncodeWAVIsDeterministic(t *testing.T) {
	samples := []float32{0.1, 0.2, -0.3, 0.4}
	a, _ := EncodeWAV(samples, 2, 44100)
	b, _ := EncodeWAV(samples, 2, 44100)
	if !bytes.Equal(a, b) {
		t.Error("EncodeWAV output is not deterministic")
	}
}
