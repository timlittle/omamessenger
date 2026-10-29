package app

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

// sniffLimit bounds how many bytes this helper reads off a file to
// decide its real content type: http.DetectContentType itself only
// ever looks at the first 512 bytes of whatever it is given.
const sniffLimit = 512

// svgzContentType names a gzip-compressed SVG ("svgz"): not a
// registered MIME type of its own - a real server would serve one as
// plain "image/svg+xml" with a "Content-Encoding: gzip" header this
// helper has no equivalent of - but a name this package needs
// internally so a still gzip-compressed file is never named or risk-
// checked as if it had already been decompressed into a real,
// renderable "image/svg+xml" document (see looksLikeGzippedSVG).
const svgzContentType = "image/svg+xml+gzip"

// mhtmlContentType names a MIME web archive (".mht"/".mhtml"): a
// multipart MIME message whose root part is the active "text/html"
// page itself, saved as one file the way a browser's "Save page as"
// can. Not a type http.DetectContentType's own sniff table has any
// entry for at all.
const mhtmlContentType = "application/x-mimearchive"

// extensionByContentType is the fixed allow-list a downloaded file
// whose kind has no fixed extension of its own (see fixedMediaName) is
// named from: keyed by the base MIME type sniffContentType reports,
// never a sender's own claimed file name or extension, which a sender
// fully controls and this must never trust (see fetchIntoCache and
// docs/decisions.md). A type this list does not know gets ".bin".
var extensionByContentType = map[string]string{
	"application/pdf":              ".pdf",
	"application/zip":              ".zip",
	"application/x-gzip":           ".gz",
	"application/x-rar-compressed": ".rar",
	"application/ogg":              ".ogg",
	"application/wasm":             ".wasm",
	"image/jpeg":                   ".jpg",
	"image/png":                    ".png",
	"image/gif":                    ".gif",
	"image/webp":                   ".webp",
	"image/bmp":                    ".bmp",
	"image/svg+xml":                ".svg",
	svgzContentType:                ".svgz",
	"text/xml":                     ".xml",
	mhtmlContentType:               ".mhtml",
	"audio/wave":                   ".wav",
	"audio/webm":                   ".weba",
	"audio/basic":                  ".au",
	"audio/aiff":                   ".aiff",
	"video/webm":                   ".webm",
	"video/mp4":                    ".mp4",
	"video/avi":                    ".avi",
	"text/plain":                   ".txt",
	"text/html":                    ".html",
	"font/woff":                    ".woff",
	"font/woff2":                   ".woff2",
}

// extensionFor names the cache extension contentType's content earns,
// from extensionByContentType's own fixed allow-list, or ".bin" for
// anything this helper does not recognise: an unrecognised type is
// never assumed safe just because it was not matched here (see
// isRiskyContent in mediarisk.go, which treats "bin" as risky for
// exactly this reason).
func extensionFor(contentType string) string {
	if ext, ok := extensionByContentType[contentType]; ok {
		return ext
	}

	return ".bin"
}

// sniffContentType reads up to sniffLimit bytes of the file at path and
// reports its base MIME type (no "; charset=..." suffix), sniffed from
// those bytes alone - never path's own name or extension, which is
// exactly what a sender can freely choose and this must not trust. It
// also recognises three active-content formats http.DetectContentType
// itself has no entry for at all (see activeContentType): a bare SVG
// with no "<?xml" line ahead of its own root element, which is how
// almost every real SVG is actually written; a gzip-compressed SVG
// ("svgz"); and a MIME web archive ("mht"/"mhtml").
func sniffContentType(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() // reading only; nothing to lose on a close error

	buf := make([]byte, sniffLimit)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}

	data := buf[:n]
	if contentType, ok := activeContentType(data); ok {
		return contentType, nil
	}

	return baseContentType(http.DetectContentType(data)), nil
}

// baseContentType strips any "; charset=..." or other parameter
// http.DetectContentType appended, so callers can match it against a
// plain MIME type.
func baseContentType(contentType string) string {
	base, _, _ := strings.Cut(contentType, ";")

	return base
}

// activeContentType reports a more specific active-content MIME type
// for data than http.DetectContentType itself ever reports, for the
// formats its own sniff table has no entry for at all. Checked ahead
// of, and independently from, http.DetectContentType's own result -
// never gated on what that result already said the content was - so a
// bare SVG that never reaches its "<?xml" sniff rule at all is still
// caught, the gap a pre-release security review found: Go's sniffer
// has no rule for "<svg" itself, so a real SVG missing the "<?xml"
// line almost every one actually omits used to reach isRiskyContent
// as plain, harmless-looking "text/plain" (see docs/decisions.md).
func activeContentType(data []byte) (string, bool) {
	switch {
	case looksLikeSVG(data):
		return "image/svg+xml", true
	case looksLikeGzippedSVG(data):
		return svgzContentType, true
	case looksLikeMHTML(data):
		return mhtmlContentType, true
	default:
		return "", false
	}
}

// looksLikeSVG reports whether data's first bytes contain an "<svg"
// open tag, the one signal that tells a real SVG image apart from
// plain text or any other XML document, independent of whatever
// http.DetectContentType itself made of the same bytes.
func looksLikeSVG(data []byte) bool {
	return strings.Contains(strings.ToLower(string(data)), "<svg")
}

// looksLikeGzippedSVG reports whether data is gzip-compressed content
// whose decompressed start is itself a real SVG's own "<svg" open tag:
// exactly what a ".svgz" file is. Only sniffLimit's own bytes of
// compressed input are available here, which a large file can still
// truncate mid-stream; a short or failed decompression is treated as
// "not SVG" rather than an error, the same tolerance every other sniff
// check here has for data that merely does not match.
func looksLikeGzippedSVG(data []byte) bool {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return false
	}

	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return false
	}
	defer r.Close() // reading only, from an in-memory buffer; nothing to lose on a close error

	peek := make([]byte, sniffLimit)
	n, _ := io.ReadFull(r, peek) // a short read from truncated input is still checked below

	return looksLikeSVG(peek[:n])
}

// looksLikeMHTML reports whether data's own leading bytes are a MIME
// web archive: a multipart MIME message - "MIME-Version:" and a
// "multipart/related" part, the same way a real one always starts -
// whose root part is an active "text/html" page, something
// http.DetectContentType has no sniff rule for at all and would
// otherwise report as plain text.
func looksLikeMHTML(data []byte) bool {
	lower := strings.ToLower(string(data))

	return strings.Contains(lower, "mime-version:") && strings.Contains(lower, "multipart/related")
}
