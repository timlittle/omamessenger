package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// formatMarkerName is the file a media area keeps its own on-disk
// format version in, a plain integer: never a real file Fetch,
// FetchNamed or Store would ever name a message or attachment after,
// so Revalidate always skips it on its own.
const formatMarkerName = ".media-format"

// FormatVersion reads the integer the cache's own format marker
// currently holds, or 0 for a fresh area, or one written before the
// marker existed at all - never itself a real version, so it always
// compares as older than any version a caller records with
// SetFormatVersion.
func (c *Cache) FormatVersion() (int, error) {
	return readFormatVersion(c.dir)
}

// SetFormatVersion records version as the cache's own current format
// version, so a later run with the same or newer version has no
// reason to revalidate its files again.
func (c *Cache) SetFormatVersion(version int) error {
	return writeFormatVersion(c.dir, version)
}

// FormatVersion is Cache's own FormatVersion, over the outgoing
// attachment area instead.
func (o *Outgoing) FormatVersion() (int, error) {
	return readFormatVersion(o.dir)
}

// SetFormatVersion is Cache's own SetFormatVersion, over the outgoing
// attachment area instead.
func (o *Outgoing) SetFormatVersion(version int) error {
	return writeFormatVersion(o.dir, version)
}

// readFormatVersion reads dir's own format marker, shared by Cache's
// and Outgoing's own FormatVersion.
func readFormatVersion(dir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(dir, formatMarkerName))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("cache: format version: %w", err)
	}

	version, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		//nolint:nilerr // deliberate: a marker that cannot even be parsed is treated the same as a missing one (version 0), safer to revalidate once more than to trust a corrupt file as proof the area is already current
		return 0, nil
	}

	return version, nil
}

// writeFormatVersion writes version into dir's own format marker,
// shared by Cache's and Outgoing's own SetFormatVersion.
func writeFormatVersion(dir string, version int) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cache: set format version: %w", err)
	}

	path := filepath.Join(dir, formatMarkerName)
	if err := os.WriteFile(path, []byte(strconv.Itoa(version)), 0o600); err != nil {
		return fmt.Errorf("cache: set format version: %w", err)
	}

	return nil
}
