package pdf

import "testing"

// segment is a convenience constructor for a positioned text run.
func segment(x0, x1 float64) TextSegment {
	return TextSegment{X0: x0, X1: x1}
}

// wordSegments builds one line from word runs: left column words end before
// the gutter and right column words start after it.
func wordSegments(leftEnd, rightStart float64) []TextSegment {
	return []TextSegment{
		segment(20, 60), segment(64, 90), segment(94, leftEnd),
		segment(rightStart, rightStart+30), segment(rightStart+34, rightStart+70),
	}
}

func TestHasColumnLayoutMultiSpan(t *testing.T) {
	page := PageLines{Width: 400, Lines: []TextLine{
		{Segments: wordSegments(160, 210)},
		{Segments: wordSegments(158, 212)},
		{Segments: wordSegments(162, 210)},
		{Segments: wordSegments(159, 211)},
	}}
	if !hasColumnLayout(page) {
		t.Error("multi-span two-column page was not detected")
	}
}

func TestHasColumnLayoutStartClusters(t *testing.T) {
	page := PageLines{Width: 400, Lines: []TextLine{
		{X0: 20, Segments: []TextSegment{segment(20, 150)}},
		{X0: 20, Segments: []TextSegment{segment(20, 140)}},
		{X0: 20, Segments: []TextSegment{segment(20, 150)}},
		{X0: 20, Segments: []TextSegment{segment(20, 145)}},
		{X0: 210, Segments: []TextSegment{segment(210, 380)}},
		{X0: 212, Segments: []TextSegment{segment(212, 370)}},
		{X0: 210, Segments: []TextSegment{segment(210, 380)}},
		{X0: 211, Segments: []TextSegment{segment(211, 375)}},
	}}
	if !hasColumnLayout(page) {
		t.Error("independent-baseline columns were not detected")
	}
}

func TestHasColumnLayout(t *testing.T) {
	twoColumn := PageLines{Width: 400, Lines: []TextLine{
		{Segments: []TextSegment{segment(20, 150), segment(210, 380)}},
		{Segments: []TextSegment{segment(20, 140), segment(210, 370)}},
		{Segments: []TextSegment{segment(20, 160), segment(212, 380)}},
		{Segments: []TextSegment{segment(20, 130), segment(210, 360)}},
	}}
	if !hasColumnLayout(twoColumn) {
		t.Error("two-column page was not detected")
	}

	varyingTable := PageLines{Width: 400, Lines: []TextLine{
		{Segments: []TextSegment{segment(20, 60), segment(210, 260)}},
		{Segments: []TextSegment{segment(20, 60), segment(300, 350)}},
		{Segments: []TextSegment{segment(20, 60), segment(120, 170)}},
		{Segments: []TextSegment{segment(20, 60), segment(340, 390)}},
	}}
	if hasColumnLayout(varyingTable) {
		t.Error("varying table columns were mistaken for two-column text")
	}

	threeCellTable := PageLines{Width: 400, Lines: []TextLine{
		{Segments: []TextSegment{segment(20, 60), segment(150, 200), segment(300, 360)}},
		{Segments: []TextSegment{segment(20, 60), segment(150, 200), segment(300, 360)}},
		{Segments: []TextSegment{segment(20, 60), segment(150, 200), segment(300, 360)}},
		{Segments: []TextSegment{segment(20, 60), segment(150, 200), segment(300, 360)}},
	}}
	if hasColumnLayout(threeCellTable) {
		t.Error("three-column table was mistaken for two-column text")
	}

	singleColumn := PageLines{Width: 400, Lines: []TextLine{
		{Segments: []TextSegment{segment(20, 380)}},
		{Segments: []TextSegment{segment(20, 360)}},
		{Segments: []TextSegment{segment(20, 370)}},
		{Segments: []TextSegment{segment(20, 375)}},
	}}
	if hasColumnLayout(singleColumn) {
		t.Error("single-column page was detected as columns")
	}
}
