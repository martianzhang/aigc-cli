package docconvert

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// fromDOCX extracts the body of word/document.xml and converts paragraphs,
// headings, list items and simple tables to Markdown. Images, footnotes and
// fields are not rendered.
func fromDOCX(path string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open docx: %w", err)
	}
	defer func() { _ = archive.Close() }()

	var document []byte
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		document, err = readZipFile(file, maxEntryBytes)
		if err != nil {
			return "", fmt.Errorf("read docx body: %w", err)
		}
		break
	}
	if document == nil {
		return "", fmt.Errorf("docx has no word/document.xml")
	}

	markdown, err := docxMarkdown(document)
	if err != nil {
		return "", err
	}
	if markdown == "" {
		return "", fmt.Errorf("docx contains no extractable text")
	}
	return markdown + "\n", nil
}

// docxMarkdown walks the OOXML token stream. Paragraphs flush at </w:p>,
// cells at </w:tc> and rows at </w:tr>; the first row of a table becomes the
// header row.
func docxMarkdown(document []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(document))

	var (
		out            strings.Builder
		paragraph      strings.Builder
		cell           strings.Builder
		cells          []string
		paragraphStyle string
		listItem       bool
		inText         bool
		inTable        int
		headerPending  bool
	)

	flushParagraph := func() {
		text := strings.TrimSpace(paragraph.String())
		paragraph.Reset()
		isList := listItem
		style := paragraphStyle
		listItem, paragraphStyle = false, ""
		if text == "" {
			return
		}
		if inTable > 0 {
			if cell.Len() > 0 {
				cell.WriteByte(' ')
			}
			cell.WriteString(text)
			return
		}
		if level := docxHeadingLevel(style); level > 0 {
			out.WriteString(strings.Repeat("#", level))
			out.WriteByte(' ')
			out.WriteString(text)
			out.WriteString("\n\n")
			return
		}
		if isList {
			out.WriteString("- ")
		}
		out.WriteString(text)
		out.WriteString("\n\n")
	}

	flushRow := func() {
		if len(cells) == 0 {
			return
		}
		out.WriteString("| ")
		out.WriteString(strings.Join(cells, " | "))
		out.WriteString(" |\n")
		if headerPending {
			out.WriteString("|")
			out.WriteString(strings.Repeat(" --- |", len(cells)))
			out.WriteByte('\n')
			headerPending = false
		}
		cells = nil
	}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse docx body: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				paragraph.Reset()
				paragraphStyle, listItem = "", false
			case "pStyle":
				paragraphStyle = attrValue(t, "val")
			case "numPr":
				listItem = true
			case "t":
				inText = true
			case "tab":
				paragraph.WriteByte('\t')
			case "br", "cr":
				paragraph.WriteByte('\n')
			case "tbl":
				inTable++
				if inTable == 1 {
					headerPending = true
				}
			case "tc":
				if inTable > 0 {
					cell.Reset()
				}
			}
		case xml.CharData:
			if inText {
				paragraph.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				flushParagraph()
			case "tc":
				if inTable > 0 {
					cells = append(cells, strings.TrimSpace(cell.String()))
					cell.Reset()
				}
			case "tr":
				flushRow()
			case "tbl":
				if inTable > 0 {
					inTable--
					if inTable == 0 {
						out.WriteByte('\n')
					}
				}
			}
		}
	}
	return strings.TrimSpace(out.String()), nil
}

var docxHeadingRe = regexp.MustCompile(`(?i)^\s*(?:heading|标题)\s*([1-6])\s*$`)

// docxHeadingLevel maps a Word paragraph style to a Markdown heading level.
func docxHeadingLevel(style string) int {
	style = strings.TrimSpace(style)
	if strings.EqualFold(style, "Title") {
		return 1
	}
	match := docxHeadingRe.FindStringSubmatch(style)
	if match == nil {
		return 0
	}
	return int(match[1][0] - '0')
}

// attrValue returns the first attribute with the given local name.
func attrValue(element xml.StartElement, local string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}
