package webnovel

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"strings"
)

// BuildEPUB writes novel's chapters, in order, as an EPUB 3 book (with an NCX for older readers).
// Chapter files are numbered by position, so a reader's place in chapter N stays valid when
// later chapters are appended.
func BuildEPUB(novel Novel, source string, chapters []Chapter) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	// The mimetype entry must come first and be stored uncompressed.
	mimetype, err := archive.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	mimetype.Write([]byte("application/epub+zip"))
	files := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
	}
	esc := html.EscapeString
	var manifest, spine, nav, ncx strings.Builder
	for i, chapter := range chapters {
		name := fmt.Sprintf("c%05d.xhtml", i+1)
		fmt.Fprintf(&manifest, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`+"\n", i+1, name)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`+"\n", i+1)
		fmt.Fprintf(&nav, `<li><a href="%s">%s</a></li>`+"\n", name, esc(chapter.Name))
		fmt.Fprintf(&ncx, `<navPoint id="n%d" playOrder="%d"><navLabel><text>%s</text></navLabel><content src="%s"/></navPoint>`+"\n", i+1, i+1, esc(chapter.Name), name)
		var body strings.Builder
		for _, paragraph := range strings.Split(strings.ReplaceAll(chapter.Content, "\r\n", "\n"), "\n") {
			if paragraph = strings.TrimSpace(paragraph); paragraph != "" {
				body.WriteString("<p>" + esc(paragraph) + "</p>\n")
			}
		}
		files["OEBPS/"+name] = xhtml(chapter.Name, "<h2>"+esc(chapter.Name)+"</h2>\n"+body.String())
	}
	identifier := "urn:bookharbor:" + source + ":" + novel.ID
	var subjects strings.Builder
	for _, genre := range strings.Split(novel.Genres, ",") {
		if genre = strings.TrimSpace(genre); genre != "" {
			subjects.WriteString("<dc:subject>" + esc(genre) + "</dc:subject>\n")
		}
	}
	files["OEBPS/content.opf"] = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:identifier id="id">` + esc(identifier) + `</dc:identifier>
<dc:title>` + esc(novel.Title) + `</dc:title>
<dc:creator>` + esc(novel.Author) + `</dc:creator>
<dc:language>en</dc:language>
<dc:description>` + esc(novel.Description) + `</dc:description>
` + subjects.String() + `<meta property="dcterms:modified">2000-01-01T00:00:00Z</meta>
</metadata>
<manifest>
<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
` + manifest.String() + `</manifest>
<spine toc="ncx">
` + spine.String() + `</spine>
</package>`
	files["OEBPS/nav.xhtml"] = xhtml("Contents", `<nav epub:type="toc" id="toc"><h1>Contents</h1><ol>`+"\n"+nav.String()+`</ol></nav>`)
	files["OEBPS/toc.ncx"] = `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<head><meta name="dtb:uid" content="` + esc(identifier) + `"/></head>
<docTitle><text>` + esc(novel.Title) + `</text></docTitle>
<navMap>
` + ncx.String() + `</navMap>
</ncx>`
	// Fixed order, so the same chapters always build byte-identical files.
	names := []string{"META-INF/container.xml", "OEBPS/content.opf", "OEBPS/nav.xhtml", "OEBPS/toc.ncx"}
	for i := range chapters {
		names = append(names, fmt.Sprintf("OEBPS/c%05d.xhtml", i+1))
	}
	for _, name := range names {
		w, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func xhtml(title, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
<head><title>` + html.EscapeString(title) + `</title></head>
<body>
` + body + `
</body>
</html>`
}
