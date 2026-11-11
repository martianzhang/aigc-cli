package docredact

import (
	"bytes"
	"fmt"
	"os"
)

type pdfImagePage struct {
	JPEG     []byte
	WidthPx  int
	HeightPx int
	WidthPt  float64
	HeightPt float64
}

// writeImagePDF builds a PDF whose pages are single JPEG XObjects. It is
// intentionally minimal: no text layer, fonts, annotations or metadata.
func writeImagePDF(path string, pages []pdfImagePage) error {
	if len(pages) == 0 {
		return fmt.Errorf("pdf: no pages")
	}
	var buf bytes.Buffer
	offsets := make([]int, 1, 3+len(pages)*3)
	writeObj := func(num int, body []byte, stream []byte) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", num)
		buf.Write(body)
		if stream != nil {
			fmt.Fprintf(&buf, "\nstream\n")
			buf.Write(stream)
			fmt.Fprintf(&buf, "\nendstream")
		}
		fmt.Fprintf(&buf, "\nendobj\n")
	}

	buf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	writeObj(1, []byte("<< /Type /Catalog /Pages 2 0 R >>"), nil)

	kids := make([]string, 0, len(pages))
	for i := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+i*3))
	}
	pagesDict := fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", joinStrings(kids, " "), len(pages))
	writeObj(2, []byte(pagesDict), nil)

	for i, page := range pages {
		pageNum, contentNum, imageNum := 3+i*3, 4+i*3, 5+i*3
		pageDict := fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.4f %.4f] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
			page.WidthPt, page.HeightPt, imageNum, contentNum)
		writeObj(pageNum, []byte(pageDict), nil)

		content := fmt.Sprintf("q\n%.4f 0 0 %.4f 0 0 cm\n/Im0 Do\nQ\n", page.WidthPt, page.HeightPt)
		writeObj(contentNum, []byte(fmt.Sprintf("<< /Length %d >>", len(content))), []byte(content))

		imgHeader := fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>",
			page.WidthPx, page.HeightPx, len(page.JPEG))
		writeObj(imageNum, []byte(imgHeader), page.JPEG)
	}

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(offsets))
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func joinStrings(parts []string, sep string) string {
	var b bytes.Buffer
	for i, p := range parts {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(p)
	}
	return b.String()
}
