package service

import (
	"image"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Approximate pixel size of one terminal cell, used to translate the terminal's
// character grid into a pixel budget for the sixel path. Sixel draws pixels
// directly, so it has no cell-based sizing option.
const (
	previewCellWidthPx  = 8
	previewCellHeightPx = 16
)

// previewFillRatio is the fraction of the terminal grid an inline image may
// occupy. Below 1.0 it keeps the preview comfortable instead of filling the
// whole window; 0.6 was chosen by eye. Tune this single value to adjust size.
const previewFillRatio = 0.6

// Maximum pixel dimensions for the preview image, regardless of terminal size.
// Prevents overflow when the terminal size detection returns an unexpectedly
// large value. Derived from a typical minimum terminal canvas of ~1024×768
// scaled by previewFillRatio (0.6) so the preview never exceeds ~60% of a
// modest terminal window.
const (
	maxPreviewWidthPx  = 614
	maxPreviewHeightPx = 460
)

// Default terminal grid assumed when the real size cannot be detected. Bounding
// to a conservative box beats native size, which can scroll the output away.
const (
	defaultTerminalCols = 80
	defaultTerminalRows = 24
)

// previewTerminalSize returns the terminal grid size in character cells,
// preferring a real TTY query and falling back to the COLUMNS/LINES env vars.
// ok is false when neither yields a usable size.
func previewTerminalSize() (cols, rows int, ok bool) {
	return terminalSizeFrom(queryTerminalSize, os.Getenv)
}

// queryTerminalSize reports the terminal window size, trying stdout, then
// stderr and stdin so a redirected stdout (e.g. `| tee`) still yields the real
// size without relying on a Unix-only /dev/tty.
func queryTerminalSize() (cols, rows int, ok bool) {
	for _, f := range []*os.File{os.Stdout, os.Stderr, os.Stdin} {
		if w, h, err := term.GetSize(int(f.Fd())); err == nil && w > 0 && h > 0 {
			return w, h, true
		}
	}
	return 0, 0, false
}

// terminalSizeFrom resolves the terminal grid size from an injected size probe
// and environment lookup, so it is fully unit-testable.
func terminalSizeFrom(getSize func() (int, int, bool), getenv func(string) string) (cols, rows int, ok bool) {
	if w, h, ok := getSize(); ok {
		return w, h, true
	}
	w, errW := strconv.Atoi(strings.TrimSpace(getenv("COLUMNS")))
	h, errH := strconv.Atoi(strings.TrimSpace(getenv("LINES")))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// fitWithin returns the largest width x height that fits srcW x srcH inside
// maxW x maxH while preserving aspect ratio. The caller-provided max budget
// is itself capped to maxPreviewWidthPx x maxPreviewHeightPx so that a rogue
// terminal size never produces an unmanageably large preview. It only ever
// shrinks: when the source already fits the budget it is returned unchanged
// and never enlarged. The result never drops below 1 in either dimension, and
// the source dimensions are returned unchanged when any input is non-positive.
func fitWithin(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= 0 || srcH <= 0 || maxW <= 0 || maxH <= 0 {
		return srcW, srcH
	}
	if maxW > maxPreviewWidthPx {
		maxW = maxPreviewWidthPx
	}
	if maxH > maxPreviewHeightPx {
		maxH = maxPreviewHeightPx
	}
	scale := float64(maxW) / float64(srcW)
	if s := float64(maxH) / float64(srcH); s < scale {
		scale = s
	}
	if scale > 1 {
		scale = 1
	}
	w := int(float64(srcW)*scale + 0.5)
	h := int(float64(srcH)*scale + 0.5)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// inlineSizeIn returns the pixel size an inline image should be rendered at
// inside a maxCols x maxRows cell budget: the source shrunk to fit, or
// ok=false when the source already fits the budget and must be left at its
// native size (never enlarged).
func inlineSizeIn(srcW, srcH, maxCols, maxRows int) (w, h int, ok bool) {
	w, h = fitWithin(srcW, srcH, maxCols*previewCellWidthPx, maxRows*previewCellHeightPx)
	if w >= srcW || h >= srcH {
		return 0, 0, false
	}
	return w, h, true
}

// previewInlineSize resolves inlineSizeIn against the current terminal.
func previewInlineSize(srcW, srcH int) (w, h int, ok bool) {
	maxCols, maxRows := previewCellBox()
	return inlineSizeIn(srcW, srcH, maxCols, maxRows)
}

// scaledCells converts a terminal dimension in cells to the preview budget,
// rounding down and never returning less than 1.
func scaledCells(cells int) int {
	n := int(float64(cells) * previewFillRatio)
	if n < 1 {
		n = 1
	}
	return n
}

// previewCellBox returns the maximum size in character cells an inline image
// may occupy: previewFillRatio of the terminal grid. When the real terminal
// size cannot be detected it falls back to a conservative default grid rather
// than letting the image through at native size.
func previewCellBox() (maxCols, maxRows int) {
	cols, rows, ok := previewTerminalSize()
	if !ok {
		cols, rows = defaultTerminalCols, defaultTerminalRows
	}
	return scaledCells(cols), scaledCells(rows)
}

// fitImageToTerminal scales img to fit the preview budget derived from the
// current terminal window (or a conservative default when the size cannot be
// detected), preserving aspect ratio. It returns img unchanged when it already
// matches the target dimensions.
func fitImageToTerminal(img image.Image) image.Image {
	maxCols, maxRows := previewCellBox()
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	targetW, targetH := fitWithin(srcW, srcH, maxCols*previewCellWidthPx, maxRows*previewCellHeightPx)
	if targetW == srcW && targetH == srcH {
		return img
	}
	return resizeImage(img, targetW, targetH)
}
