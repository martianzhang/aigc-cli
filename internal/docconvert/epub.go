package docconvert

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/webfetch"
)

// epubPackage is the subset of the OPF package document preview needs.
type epubPackage struct {
	Metadata struct {
		Title string `xml:"title"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID        string `xml:"id,attr"`
			Href      string `xml:"href,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Refs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// fromEPUB follows META-INF/container.xml → OPF spine → XHTML chapters and
// converts each chapter with the shared HTML-to-Markdown pipeline.
func fromEPUB(filePath string) (string, error) {
	archive, err := zip.OpenReader(filePath)
	if err != nil {
		return "", fmt.Errorf("open epub: %w", err)
	}
	defer func() { _ = archive.Close() }()

	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		files[path.Clean(file.Name)] = file
	}

	opfPath := epubOPFPath(files)
	var title string
	var chapters []string
	if opfPath != "" {
		if file := files[opfPath]; file != nil {
			data, err := readZipFile(file, maxEntryBytes)
			if err != nil {
				return "", fmt.Errorf("read epub package: %w", err)
			}
			var pkg epubPackage
			if err := xml.Unmarshal(data, &pkg); err != nil {
				return "", fmt.Errorf("parse epub package: %w", err)
			}
			title = strings.TrimSpace(pkg.Metadata.Title)
			chapters = epubSpinePaths(pkg, opfPath)
		}
	}
	if len(chapters) == 0 {
		chapters = epubFallbackChapters(files)
	}
	if len(chapters) == 0 {
		return "", fmt.Errorf("epub contains no readable chapters")
	}

	var out strings.Builder
	for _, name := range chapters {
		file := files[name]
		if file == nil {
			continue
		}
		data, err := readZipFile(file, maxEntryBytes)
		if err != nil {
			continue
		}
		_, markdown, err := webfetch.HTMLToMarkdown(string(data), nil)
		if err != nil {
			continue
		}
		markdown = strings.TrimSpace(markdown)
		if markdown == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(markdown)
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("epub contains no readable text")
	}

	body := out.String()
	if title != "" && !strings.HasPrefix(body, "#") {
		body = "# " + title + "\n\n" + body
	}
	return body + "\n", nil
}

// epubOPFPath reads META-INF/container.xml for the package document path.
func epubOPFPath(files map[string]*zip.File) string {
	file := files["META-INF/container.xml"]
	if file == nil {
		return ""
	}
	data, err := readZipFile(file, 1<<20)
	if err != nil {
		return ""
	}
	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(data, &container); err != nil || len(container.Rootfiles) == 0 {
		return ""
	}
	return path.Clean(container.Rootfiles[0].FullPath)
}

// epubSpinePaths resolves the spine itemrefs to archive entry names, in
// reading order.
func epubSpinePaths(pkg epubPackage, opfPath string) []string {
	items := make(map[string]string, len(pkg.Manifest.Items))
	for _, item := range pkg.Manifest.Items {
		if item.MediaType != "application/xhtml+xml" && item.MediaType != "text/html" {
			continue
		}
		items[item.ID] = item.Href
	}

	baseDir := path.Dir(opfPath)
	var chapters []string
	for _, ref := range pkg.Spine.Refs {
		href, ok := items[ref.IDRef]
		if !ok {
			continue
		}
		if name, ok := epubEntryPath(baseDir, href); ok {
			chapters = append(chapters, name)
		}
	}
	return chapters
}

// epubEntryPath turns a manifest href into a cleaned archive path.
func epubEntryPath(baseDir, href string) (string, bool) {
	href = strings.SplitN(href, "#", 2)[0]
	if href == "" {
		return "", false
	}
	if unescaped, err := url.PathUnescape(href); err == nil {
		href = unescaped
	}
	return path.Clean(path.Join(baseDir, href)), true
}

// epubFallbackChapters lists XHTML/HTML entries when the spine is unusable.
func epubFallbackChapters(files map[string]*zip.File) []string {
	var names []string
	for name := range files {
		switch strings.ToLower(path.Ext(name)) {
		case ".xhtml", ".html", ".htm":
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
