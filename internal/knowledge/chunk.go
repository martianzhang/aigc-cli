package knowledge

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Chunker splits text into chunks.
type Chunker struct {
	opts ChunkOptions
}

// NewChunker creates a chunker with the given options.
func NewChunker(opts ChunkOptions) *Chunker {
	return &Chunker{opts: opts}
}

// Chunk splits markdown text into chunks, respecting headings.
func (c *Chunker) Chunk(text string) []Chunk {
	if len(text) == 0 {
		return nil
	}

	sections := splitByHeadings(text)
	var chunks []Chunk
	idx := 0

	for _, sec := range sections {
		if c.opts.MaxSize > 0 && len(sec.Content) > c.opts.MaxSize {
			subChunks := c.splitLargeSection(sec, &idx)
			chunks = append(chunks, subChunks...)
		} else {
			chunks = append(chunks, Chunk{
				Index:   idx,
				Content: sec.Content,
				Heading: sec.Heading,
			})
			idx++
		}
	}

	// Merge small chunks if overlap is configured
	if c.opts.Overlap > 0 {
		chunks = c.mergeSmall(chunks)
	}

	return chunks
}

type section struct {
	Heading string
	Content string
}

func splitByHeadings(text string) []section {
	lines := strings.Split(text, "\n")
	var sections []section
	var current section
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track code blocks to avoid splitting inside them
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
		}

		if !inCodeBlock && isHeading(trimmed) {
			if current.Content != "" || current.Heading != "" {
				sections = append(sections, current)
			}
			current = section{Heading: trimmed}
			current.Content = trimmed + "\n"
			continue
		}

		if current.Heading == "" && isHeading(trimmed) {
			current.Heading = trimmed
		}
		current.Content += line + "\n"
	}

	if current.Content != "" {
		sections = append(sections, current)
	}

	return sections
}

func isHeading(line string) bool {
	if len(line) == 0 {
		return false
	}
	// # ## ### #### ##### ######
	for i, r := range line {
		if r == '#' {
			if i >= 6 {
				return false
			}
			continue
		}
		if r == ' ' && i > 0 {
			return true
		}
		return false
	}
	return false
}

func (c *Chunker) splitLargeSection(s section, idx *int) []Chunk {
	text := s.Content
	var chunks []Chunk
	maxSize := c.opts.MaxSize
	if maxSize <= 0 {
		maxSize = 2048
	}

	for len(text) > 0 {
		if len(text) <= maxSize {
			chunks = append(chunks, Chunk{
				Index:   *idx,
				Content: s.Heading + "\n\n" + text,
				Heading: s.Heading,
			})
			*idx++
			break
		}

		// Try to break at a paragraph boundary
		splitAt := maxSize
		if c.opts.Overlap > 0 {
			splitAt = maxSize - c.opts.Overlap
			if splitAt < 1 {
				splitAt = 1
			}
		}

		// Search backward for paragraph break
		paraBreak := strings.LastIndex(text[:splitAt], "\n\n")
		if paraBreak > maxSize/2 {
			splitAt = paraBreak
		} else {
			// Search backward for sentence break
			sentenceBreak := strings.LastIndex(text[:splitAt], "。")
			if sentenceBreak > maxSize/2 {
				splitAt = sentenceBreak + 3 // include the period
			} else {
				// Search backward for newline
				nl := strings.LastIndex(text[:splitAt], "\n")
				if nl > maxSize/3 {
					splitAt = nl
				}
			}
		}

		splitAt = clamp(splitAt, 1, len(text))
		content := text[:splitAt]

		chunks = append(chunks, Chunk{
			Index:   *idx,
			Content: s.Heading + "\n\n" + content,
			Heading: s.Heading,
		})
		*idx++

		// Advance with overlap
		nextStart := splitAt - c.opts.Overlap
		if nextStart < 0 || nextStart >= splitAt {
			nextStart = splitAt
		}
		text = text[nextStart:]
	}

	return chunks
}

func (c *Chunker) mergeSmall(chunks []Chunk) []Chunk {
	if len(chunks) <= 1 {
		return chunks
	}
	var merged []Chunk
	buf := chunks[0]
	for i := 1; i < len(chunks); i++ {
		if len(buf.Content)+len(chunks[i].Content) < c.opts.MaxSize/2 {
			buf.Content += "\n\n" + chunks[i].Content
			continue
		}
		merged = append(merged, buf)
		buf = chunks[i]
	}
	merged = append(merged, buf)
	// Re-index
	for i := range merged {
		merged[i].Index = i
	}
	return merged
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

// WordCount returns the approximate word count in text.
func WordCount(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			inWord = false
		} else {
			if !inWord {
				count++
			}
			inWord = true
		}
	}
	return count
}

// CountTokens estimates the number of tokens in text (rough: 4 chars per token).
func CountTokens(text string) int {
	return int(math.Ceil(float64(utf8.RuneCountInString(text)) / 4.0))
}
