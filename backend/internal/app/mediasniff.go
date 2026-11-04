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

// svgzContentType is this package's own internal name for a
// gzip-compressed SVG, so it is never named or risk-checked as if
// already decompressed.
const svgzContentType = "image/svg+xml+gzip"

// mhtmlContentType names a MIME web archive (".mht"/".mhtml"), a type
// http.DetectContentType has no sniff rule for.
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

// sniffContentType reads up to sniffLimit bytes of the file at path
// and reports its base MIME type, sniffed from those bytes alone,
// never path's own name or extension. It also recognises three
// active-content formats http.DetectContentType has no entry for:
// bare SVG, gzip-compressed SVG and MHTML (see activeContentType).
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
// for data than http.DetectContentType covers, checked independently
// of that result so a bare SVG with no "<?xml" line is still caught
// (see docs/decisions.md).
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
// open tag, the signal that tells a real SVG apart from plain text or
// other XML.
func looksLikeSVG(data []byte) bool {
	return strings.Contains(strings.ToLower(string(data)), "<svg")
}

// looksLikeGzippedSVG reports whether data is gzip-compressed content
// whose decompressed start is an SVG's "<svg" open tag. A short or
// failed decompression is treated as "not SVG" rather than an error.
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

// looksLikeMHTML reports whether data's leading bytes are a MIME web
// archive: a "MIME-Version:" header with a "multipart/related" part.
func looksLikeMHTML(data []byte) bool {
	lower := strings.ToLower(string(data))

	return strings.Contains(lower, "mime-version:") && strings.Contains(lower, "multipart/related")
}
