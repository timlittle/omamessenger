package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errNoDownloadsDir reports a media.save asked for before the helper
// could resolve a downloads directory (no HOME set when it started).
var errNoDownloadsDir = errors.New("no downloads directory is configured")

// SaveMedia copies a message's photo, video, file or voice note into the
// user's downloads directory, fetching it first if it is not already on
// this machine, and returns where it ended up. The destination name
// never overwrites an existing file: see uniqueDownloadPath.
func (c *Commands) SaveMedia(ctx context.Context, messageID string) (string, error) {
	if c.downloadsDir == "" {
		return "", fmt.Errorf("%w: %w", ErrInvalidInput, errNoDownloadsDir)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return "", err
	}

	src, err := c.FetchMedia(ctx, messageID, false)
	if err != nil {
		return "", err
	}

	contentType, err := sniffContentType(src)
	if err != nil {
		return "", fmt.Errorf("media: save: %w", err)
	}

	if err := os.MkdirAll(c.downloadsDir, 0o750); err != nil {
		return "", fmt.Errorf("media: save: create downloads directory: %w", err)
	}

	dst, err := uniqueDownloadPath(c.downloadsDir, downloadName(m.Media, extensionFor(contentType)))
	if err != nil {
		return "", fmt.Errorf("media: save: %w", err)
	}

	if err := copyFile(src, dst); err != nil {
		return "", fmt.Errorf("media: save: %w", err)
	}

	return dst, nil
}

// fallbackStem names a save whose sender-claimed file name sanitizes
// to nothing usable at all: every control character, bidi or format
// character and surrounding whitespace stripped away, with nothing
// left (see sanitizedStem).
const fallbackStem = "file"

// maxStemLen caps a saved download's own name, before its extension,
// at a length no real file name needs and no filesystem need ever
// reject.
const maxStemLen = 100

// downloadName picks the name media.save gives the copy it leaves in
// the downloads directory: the sender's own file name, sanitized (see
// sanitizedStem), or fallbackStem when there is no usable sender name
// at all, always followed by extension - sniffed fresh from the
// downloaded bytes (see sniffContentType and extensionFor), never the
// sender's own claimed one, which this never trusted for how the file
// is cached or opened either (see fetchIntoCache and
// docs/decisions.md) and must not be trusted for what it is shown as
// now.
func downloadName(media *domain.Media, extension string) string {
	stem := sanitizedStem(media)
	if stem == "" {
		stem = fallbackStem
	}

	return stem + extension
}

// sanitizedStem returns media's own sender-claimed file name, safe to
// show as the stem of a saved download's name: its own claimed
// extension dropped (downloadName always appends a sniffed one
// instead), every control (Cc) or Unicode bidi/format (Cf) character
// stripped - which covers the right-to-left override and isolate
// characters (U+202A-U+202E, U+2066-U+2069) a sender could use to
// make a file's real extension read differently than it is - runs of
// whitespace collapsed to one space, and capped at maxStemLen runes.
// "" when media is nil, names nothing usable, or the result is empty
// once sanitized; downloadName falls back to fallbackStem for any of
// those.
func sanitizedStem(media *domain.Media) string {
	if media == nil {
		return ""
	}

	base := filepath.Base(media.FileName)
	if base == "" || base == "." || base == "/" {
		return ""
	}

	stem := strings.TrimSuffix(base, filepath.Ext(base))
	cleaned := collapseWhitespace(stripControlAndFormatChars(stem))

	return truncateRunes(cleaned, maxStemLen)
}

// stripControlAndFormatChars removes every Unicode control (Cc) or
// format (Cf) character from s.
func stripControlAndFormatChars(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			return -1
		}

		return r
	}, s)
}

// whitespaceRun matches one or more consecutive whitespace characters,
// for collapseWhitespace to fold into a single space.
var whitespaceRun = regexp.MustCompile(`\s+`)

// collapseWhitespace replaces every run of whitespace in s with a
// single space and trims the result.
func collapseWhitespace(s string) string {
	return strings.TrimSpace(whitespaceRun.ReplaceAllString(s, " "))
}

// truncateRunes cuts s to at most n runes, never splitting one in half.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n])
}

// uniqueDownloadPath joins dir and name, adding " (2)", " (3)" and so on
// before name's extension until it names a file that does not exist yet,
// so saving the same message's media twice never overwrites the first
// copy. It gives up after a generous number of attempts rather than
// looping forever against a directory it cannot read.
func uniqueDownloadPath(dir, name string) (string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	for n := 1; n <= 1000; n++ {
		candidate := name
		if n > 1 {
			candidate = base + " (" + strconv.Itoa(n) + ")" + ext
		}

		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not find a free name for %q", name)
}

// copyFile writes src's contents to dst, created fresh.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close() // reading only; nothing to lose on a close error

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	if _, copyErr := io.Copy(out, in); copyErr != nil {
		// The write already failed; a second error closing the same
		// broken file would add nothing a caller could act on.
		return fmt.Errorf("copy: %w", errors.Join(copyErr, out.Close()))
	}

	return out.Close()
}
