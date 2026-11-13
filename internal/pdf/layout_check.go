package pdf

import (
	"fmt"
	"math"
	"sort"
)

const (
	// columnWordGap merges text runs separated only by word spacing. Anything
	// wider is a layout gap (column gutter, indentation, table cell).
	columnWordGap = 8.0
	// columnMinGutter is the narrowest gap treated as a column gutter.
	columnMinGutter = 12.0
	// columnGutterBin clusters gutter midpoints (PDF points) across lines.
	columnGutterBin = 8.0
)

// LayoutIssues reports layout that Markdown cannot represent faithfully, so a
// text preview would mislead the reader:
//
//   - multi-column reading order: linearizing it interleaves the columns;
//   - rotated pages: text direction and reading order are lost;
//   - pages without a text layer in an otherwise textual PDF: their content
//     would silently disappear;
//   - glyphs that did not map to Unicode: the output is garbage.
//
// Headings, lists, bold/italic and simple tables are deliberately NOT reported:
// Markdown represents those, even when the extractor is imperfect.
func LayoutIssues(path string) []string {
	pages, err := ExtractPageLines(path)
	if err != nil || len(pages) == 0 {
		return nil
	}

	var issues []string
	rotated, noText, columnPages, textPages := 0, 0, 0, 0
	for _, page := range pages {
		if page.Rotation != 0 {
			rotated++
			continue
		}
		if len(page.Lines) == 0 {
			noText++
			continue
		}
		textPages++
		if hasColumnLayout(page) {
			columnPages++
		}
	}

	if rotated > 0 {
		issues = append(issues, fmt.Sprintf("%d rotated page(s)", rotated))
	}
	// A lone cover page is not worth leaving the terminal for; a scanned page in
	// a text document is.
	if noText >= 1 && noText*20 >= len(pages) {
		issues = append(issues, fmt.Sprintf("%d page(s) without a text layer", noText))
	}
	// One table-like page must not send a mostly single-column document to the
	// viewer; only report columns when they cover a substantial share.
	if columnPages > 0 && columnPages*3 >= textPages {
		issues = append(issues, fmt.Sprintf("multi-column text on %d page(s)", columnPages))
	}
	return issues
}

// hasColumnLayout reports whether the page has a reading order Markdown cannot
// express. It accepts either signal:
//   - a vertical gutter: most lines split into exactly two runs at the same x
//     near the middle of the page (columns that share baselines), or
//   - two start clusters: many lines begin at the left margin and many others
//     begin mid-page (columns with independent baselines).
//
// Tables split into three or more runs per row or move their cell edges from
// row to row, so neither signal matches them.
func hasColumnLayout(page PageLines) bool {
	if page.Width <= 0 || len(page.Lines) < 4 {
		return false
	}
	return hasGutter(page) || hasStartClusters(page)
}

// hasGutter looks for a persistent vertical gap between text runs.
func hasGutter(page PageLines) bool {
	buckets := make(map[int]int)
	for _, line := range page.Lines {
		ranges := mergedRunRanges(line)
		// Exactly two runs means two text bodies side by side. More runs is a
		// table row or a line with several positioned fragments.
		if len(ranges) != 2 {
			continue
		}
		gap := ranges[1].x0 - ranges[0].x1
		if gap < columnMinGutter {
			continue
		}
		mid := (ranges[1].x0 + ranges[0].x1) / 2
		if mid < page.Width*0.25 || mid > page.Width*0.75 {
			continue
		}
		buckets[int(mid/columnGutterBin)]++
	}

	best := 0
	for bin, count := range buckets {
		window := count + buckets[bin-1] + buckets[bin+1]
		best = max(best, window)
	}
	return best >= 3 && best*5 >= len(page.Lines)
}

// hasStartClusters looks for two stable line-start columns.
func hasStartClusters(page PageLines) bool {
	left, right := 0, 0
	for _, line := range page.Lines {
		start := line.X0
		switch {
		case start < page.Width*0.2:
			left++
		case start >= page.Width*0.25 && start <= page.Width*0.75:
			right++
		}
	}
	if left < 3 || right < 3 {
		return false
	}
	return right*4 >= len(page.Lines)
}

// xRange is one merged horizontal text run.
type xRange struct {
	x0, x1 float64
}

// mergedRunRanges merges segments separated by normal word spacing, so only
// layout-level gaps remain.
func mergedRunRanges(line TextLine) []xRange {
	ranges := make([]xRange, 0, len(line.Segments))
	for _, segment := range line.Segments {
		ranges = append(ranges, xRange{x0: segment.X0, x1: segment.X1})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].x0 < ranges[j].x0 })

	merged := ranges[:0]
	for _, r := range ranges {
		if len(merged) == 0 {
			merged = append(merged, r)
			continue
		}
		last := &merged[len(merged)-1]
		if r.x0-last.x1 <= columnWordGap {
			last.x1 = math.Max(last.x1, r.x1)
			continue
		}
		merged = append(merged, r)
	}
	return merged
}
