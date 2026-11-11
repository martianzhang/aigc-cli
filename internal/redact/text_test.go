package redact

import "testing"

func TestFoldWidthPreservesRuneOffsets(t *testing.T) {
	original := "１２３ABC"
	f := FoldWidth(original)
	if f.Text != "123ABC" {
		t.Fatalf("folded = %q, want %q", f.Text, "123ABC")
	}
	if len(f.ToOrig) != len(f.Text)+1 {
		t.Fatalf("ToOrig length = %d, want %d", len(f.ToOrig), len(f.Text)+1)
	}
	// Each full-width digit is 3 bytes; following ASCII starts at byte 9.
	if got := f.ToOrig[3]; got != 9 {
		t.Errorf("ToOrig[3] = %d, want 9", got)
	}
	if got := f.ToOrig[6]; got != len(original) {
		t.Errorf("ToOrig[6] = %d, want %d", got, len(original))
	}
}

func TestBoundaryOKChineseNeighbours(t *testing.T) {
	folded := FoldWidth("电话13812345678。").Text
	start := len("电话")
	end := start + len("13812345678")
	if !BoundaryOK(folded, start, end, "digit") {
		t.Fatal("Chinese neighbours must not block a phone match")
	}
	if BoundaryOK("a13812345678b", 1, 12, "word") {
		t.Fatal("ASCII letters must block a word boundary")
	}
	if BoundaryOK("9913812345678", 2, 13, "digit") {
		t.Fatal("an adjacent digit must block a digit boundary")
	}
}

func TestDocumentOffsets(t *testing.T) {
	doc := NewDocument("t", "张😀三")
	if got := doc.RuneOffset(3); got != 1 {
		t.Errorf("RuneOffset(3) = %d, want 1", got)
	}
	if got := doc.ByteOffset(2); got != 7 {
		t.Errorf("ByteOffset(2) = %d, want 7", got)
	}
	if got := doc.ByteOffset(99); got != len(doc.Text) {
		t.Errorf("ByteOffset(99) = %d, want %d", got, len(doc.Text))
	}
}

func TestStripSeparators(t *testing.T) {
	if got := StripSeparators("+86 139-1234-5678"); got != "+8613912345678" {
		t.Fatalf("StripSeparators = %q", got)
	}
	if got := StripSeparators("６２２２ ０２０２"); got != "62220202" {
		t.Fatalf("full-width separators: got %q", got)
	}
}
