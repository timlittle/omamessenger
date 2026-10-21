package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

	if err := os.MkdirAll(c.downloadsDir, 0o750); err != nil {
		return "", fmt.Errorf("media: save: create downloads directory: %w", err)
	}

	dst, err := uniqueDownloadPath(c.downloadsDir, downloadName(m.Media, filepath.Base(src)))
	if err != nil {
		return "", fmt.Errorf("media: save: %w", err)
	}

	if err := copyFile(src, dst); err != nil {
		return "", fmt.Errorf("media: save: %w", err)
	}

	return dst, nil
}

// downloadName picks the name media.save gives the copy it leaves in
// the downloads directory: the sender's own file name when there is
// one - shown only, never used to pick how the file is cached or
// opened (see fetchIntoCache and docs/decisions.md) - or otherwise
// cacheName, the media cache's own name for it, already carrying a
// safe extension for whatever the file actually is.
func downloadName(media *domain.Media, cacheName string) string {
	if media != nil {
		if name := filepath.Base(media.FileName); name != "" && name != "." && name != "/" {
			return name
		}
	}

	return cacheName
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
