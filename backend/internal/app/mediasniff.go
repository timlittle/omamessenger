package app

import (
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

// extensionByContentType is the fixed allow-list a downloaded file
// whose kind has no fixed extension of its own (see fixedMediaName) is
// named from: keyed by the base MIME type sniffContentType reports,
// never a sender's own claimed file name or extension, which a sender
// fully controls and this must never trust (see fetchIntoCache and
// docs/decisions.md). A type this list does not know gets ".bin".
var extensionByContentType = map[string]string{
	"application/pdf":              ".pdf",
	"application/zip":               ".zip",
	"application/x-gzip":            ".gz",
	"application/x-rar-compressed":  ".rar",
	"application/ogg":               ".ogg",
	"application/wasm":              ".wasm",
	"image/jpeg":                    ".jpg",
	"image/png":                     ".png",
	"image/gif":                     ".gif",
	"image/webp":                    ".webp",
	"image/bmp":                     ".bmp",
	"image/svg+xml":                 ".svg",
	"audio/wave":                    ".wav",
	"audio/webm":                    ".weba",
	"audio/basic":                   ".au",
	"audio/aiff":                    ".aiff",
	"video/webm":                    ".webm",
	"video/mp4":                     ".mp4",
	"video/avi":                     ".avi",
	"text/plain":                    ".txt",
	"text/html":                     ".html",
	"font/woff":                     ".woff",
	"font/woff2":                    ".woff2",
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
// also recognises SVG, the one common active-content format
// http.DetectContentType itself only ever reports as generic XML.
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
	contentType := baseContentType(http.DetectContentType(data))
	if contentType == "text/xml" && looksLikeSVG(data) {
		return "image/svg+xml", nil
	}

	return contentType, nil
}

// baseContentType strips any "; charset=..." or other parameter
// http.DetectContentType appended, so callers can match it against a
// plain MIME type.
func baseContentType(contentType string) string {
	base, _, _ := strings.Cut(contentType, ";")

	return base
}

// looksLikeSVG reports whether data's first bytes contain an "<svg"
// open tag, the one signal that tells a real SVG image apart from any
// other XML document once http.DetectContentType has already told them
// both apart from everything else as generic XML.
func looksLikeSVG(data []byte) bool {
	return strings.Contains(strings.ToLower(string(data)), "<svg")
}
