package pdf

// BlockType represents the type of a document block.
type BlockType int

const (
	// BlockParagraph is a regular text paragraph.
	BlockParagraph BlockType = iota
	// BlockHeading is a section heading (H1-H4).
	BlockHeading
	// BlockTable is a table grid.
	BlockTable
	// BlockList is a bulleted or numbered list item.
	BlockList
)

// InlineStyle tracks inline formatting for a block's content.
type InlineStyle struct {
	Bold   bool
	Italic bool
}

// DocumentBlock represents a structured block of content extracted from a PDF.
type DocumentBlock struct {
	Type    BlockType
	Level   int         // heading level 1-4 (only for BlockHeading)
	Content string      // text content
	Style   InlineStyle // inline formatting
	BBox    [4]float64  // bounding box: [x0, y0, x1, y1]
}

// headingLevel calculates heading level from font size ratio.
// Returns 1-4, where 1 is the largest.
func headingLevel(fontSize, bodyFontSize float64) int {
	ratio := fontSize / bodyFontSize
	switch {
	case ratio >= 1.8:
		return 1
	case ratio >= 1.5:
		return 2
	case ratio >= 1.3:
		return 3
	default:
		return 4
	}
}
