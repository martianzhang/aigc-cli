package pdf

import (
	"math"
	"testing"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

func TestSpanFontSizeUsesAdvanceWhenTextMatrixScales(t *testing.T) {
	// Real-world PDFs often declare a large Tf size and shrink it with the
	// text matrix: "/FT8 209 Tf" plus "0.05 ... Tm".
	span := gopdf.TextSpan{X: 90, EndX: 100.45, Y: 758.69, FontSize: 209, Text: "民"}
	if got := spanFontSize(span); math.Abs(got-10.45) > 0.01 {
		t.Fatalf("spanFontSize = %.4f, want 10.45", got)
	}
	bottom, top := textSpanBounds(span)
	if math.Abs(bottom-756.0775) > 0.01 || math.Abs(top-769.14) > 0.01 {
		t.Fatalf("bounds = (%.4f,%.4f), want (756.08,769.14)", bottom, top)
	}
}

func TestSpanFontSizeKeepsDeclaredSizeWhenUnscaled(t *testing.T) {
	span := gopdf.TextSpan{X: 0, EndX: 31, Y: 100, FontSize: 12, Text: "Email:"}
	if got := spanFontSize(span); got != 12 {
		t.Fatalf("spanFontSize = %.4f, want 12", got)
	}
}

func TestMedianSpanSizePrefersCJK(t *testing.T) {
	spans := []gopdf.TextSpan{
		{X: 0, EndX: 10.45, FontSize: 10.45, Text: "统"},
		{X: 10, EndX: 20.45, FontSize: 10.45, Text: "一"},
		// Wide Latin letters over-estimate their size from the advance.
		{X: 20, EndX: 29.1, FontSize: 18.2, Text: "M"},
	}
	if got := medianSpanSize(spans); math.Abs(got-10.45) > 0.01 {
		t.Fatalf("medianSpanSize = %.4f, want 10.45", got)
	}
}
