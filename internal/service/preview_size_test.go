package service

import (
	"image"
	"testing"
)

// --- fitWithin ---

func TestFitWithin(t *testing.T) {
	tests := []struct {
		name         string
		srcW, srcH   int
		maxW, maxH   int
		wantW, wantH int
	}{
		{"square into box is height-constrained", 1000, 1000, 632, 368, 368, 368},
		{"wide image is width-constrained", 4000, 1000, 632, 368, 632, 158},
		{"wide image is height-constrained", 1000, 100, 632, 368, 632, 63},
		{"image that fits is returned unchanged", 100, 50, 632, 368, 100, 50},
		{"small image is never enlarged", 10, 10, 632, 368, 10, 10},
		{"already matching box", 632, 368, 632, 368, 632, 368},
		{"non-positive source width", 0, 100, 632, 368, 0, 100},
		{"non-positive source height", 100, 0, 632, 368, 100, 0},
		{"non-positive max width", 100, 100, 0, 368, 100, 100},
		{"non-positive max height", 100, 100, 632, 0, 100, 100},
		{"negative inputs", 100, 100, -5, -5, 100, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := fitWithin(tt.srcW, tt.srcH, tt.maxW, tt.maxH)
			if gotW != tt.wantW || gotH != tt.wantH {
				t.Errorf("fitWithin(%d, %d, %d, %d) = %dx%d, want %dx%d",
					tt.srcW, tt.srcH, tt.maxW, tt.maxH, gotW, gotH, tt.wantW, tt.wantH)
			}
		})
	}
}

// --- terminalSizeFrom ---

func TestTerminalSizeFrom(t *testing.T) {
	probeOK := func(w, h int) func() (int, int, bool) {
		return func() (int, int, bool) { return w, h, true }
	}
	probeFail := func() (int, int, bool) { return 0, 0, false }

	tests := []struct {
		name     string
		probe    func() (int, int, bool)
		env      map[string]string
		wantCols int
		wantRows int
		wantOK   bool
	}{
		{"probe result wins over env", probeOK(100, 40), map[string]string{"COLUMNS": "120", "LINES": "30"}, 100, 40, true},
		{"env fallback when probe fails", probeFail, map[string]string{"COLUMNS": "120", "LINES": "30"}, 120, 30, true},
		{"env values are trimmed", probeFail, map[string]string{"COLUMNS": " 120 ", "LINES": "\t30\n"}, 120, 30, true},
		{"env unset", probeFail, map[string]string{}, 0, 0, false},
		{"env empty", probeFail, map[string]string{"COLUMNS": "", "LINES": ""}, 0, 0, false},
		{"env non-numeric", probeFail, map[string]string{"COLUMNS": "wide", "LINES": "tall"}, 0, 0, false},
		{"env zero", probeFail, map[string]string{"COLUMNS": "0", "LINES": "0"}, 0, 0, false},
		{"env negative", probeFail, map[string]string{"COLUMNS": "-10", "LINES": "-5"}, 0, 0, false},
		{"only columns set", probeFail, map[string]string{"COLUMNS": "120"}, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			gotCols, gotRows, gotOK := terminalSizeFrom(tt.probe, getenv)
			if gotCols != tt.wantCols || gotRows != tt.wantRows || gotOK != tt.wantOK {
				t.Errorf("terminalSizeFrom() = (%d, %d, %v), want (%d, %d, %v)",
					gotCols, gotRows, gotOK, tt.wantCols, tt.wantRows, tt.wantOK)
			}
		})
	}
}

// --- inlineSizeIn ---

func TestInlineSizeIn(t *testing.T) {
	const (
		maxCols = 48 // 48 * 8 = 384 px
		maxRows = 14 // 14 * 16 = 224 px
	)
	tests := []struct {
		name         string
		srcW, srcH   int
		wantW, wantH int
		wantOK       bool
	}{
		{"oversized is shrunk to fit", 2000, 1000, 384, 192, true},
		{"height-bound is shrunk", 400, 400, 224, 224, true},
		{"already fits stays native", 100, 100, 0, 0, false},
		{"exactly fits stays native", 384, 224, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH, gotOK := inlineSizeIn(tt.srcW, tt.srcH, maxCols, maxRows)
			if gotW != tt.wantW || gotH != tt.wantH || gotOK != tt.wantOK {
				t.Errorf("inlineSizeIn(%d, %d, %d, %d) = (%d, %d, %v), want (%d, %d, %v)",
					tt.srcW, tt.srcH, maxCols, maxRows, gotW, gotH, gotOK, tt.wantW, tt.wantH, tt.wantOK)
			}
		})
	}
}

// --- scaledCells ---

func TestScaledCells(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"80 cells", 80, 48},
		{"40 cells", 40, 24},
		{"24 cells floors", 24, 14},
		{"10 cells", 10, 6},
		{"5 cells floors", 5, 3},
		{"1 cell stays 1", 1, 1},
		{"0 cells never below 1", 0, 1},
		{"negative cells never below 1", -7, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scaledCells(tt.in); got != tt.want {
				t.Errorf("scaledCells(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// --- fitImageToTerminal ---

func TestFitImageToTerminal_fitsWithinWindow(t *testing.T) {
	// When stdout is a real TTY the probe result is host-dependent, so the
	// deterministic COLUMNS/LINES fallback cannot be exercised.
	if _, _, ok := queryTerminalSize(); ok {
		t.Skip("stdout is a TTY; terminal size is host-dependent")
	}
	t.Setenv("COLUMNS", "40")
	t.Setenv("LINES", "10")

	src := image.NewRGBA(image.Rect(0, 0, 4000, 1000))
	got := fitImageToTerminal(src)
	boxW := scaledCells(40) * previewCellWidthPx
	boxH := scaledCells(10) * previewCellHeightPx
	if got.Bounds().Dx() > boxW || got.Bounds().Dy() > boxH {
		t.Errorf("fitImageToTerminal() = %dx%d, exceeds terminal box %dx%d",
			got.Bounds().Dx(), got.Bounds().Dy(), boxW, boxH)
	}
	if got.Bounds().Dx() == 4000 && got.Bounds().Dy() == 1000 {
		t.Error("fitImageToTerminal() returned the oversized image unchanged")
	}
}

func TestFitImageToTerminal_unknownSizeKeepsImage(t *testing.T) {
	if _, _, ok := queryTerminalSize(); ok {
		t.Skip("stdout is a TTY; terminal size is host-dependent")
	}
	t.Setenv("COLUMNS", "")
	t.Setenv("LINES", "")

	src := image.NewRGBA(image.Rect(0, 0, 4000, 1000))
	got := fitImageToTerminal(src)
	if got.Bounds().Dx() != 4000 || got.Bounds().Dy() != 1000 {
		t.Errorf("fitImageToTerminal() without size = %dx%d, want original 4000x1000",
			got.Bounds().Dx(), got.Bounds().Dy())
	}
}
