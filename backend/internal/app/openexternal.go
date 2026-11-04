package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// OpenError reports that a link or local file could not be opened
// outside this process. Reason is the only detail that crosses the
// protocol.
type OpenError struct {
	Reason string
	err    error
}

// Error renders the failure for the helper's own logs.
func (e *OpenError) Error() string {
	return fmt.Sprintf("open externally: %s: %v", e.Reason, e.err)
}

// Unwrap exposes the underlying failure for errors.Is/As.
func (e *OpenError) Unwrap() error {
	return e.err
}

// errOpenerNotWired reports that this helper build has no URIOpener.
var errOpenerNotWired = errors.New("no uri opener wired in")

// OpenLink asks the desktop portal to open url with the user's default
// handler, rather than spawning a process whose argv would carry the
// link itself (see docs/decisions.md). url is re-checked against the
// same scheme allow-list the UI applies.
func (c *Commands) OpenLink(ctx context.Context, url string) error {
	if !domain.AllowedLinkURL(url) {
		return fmt.Errorf("%w: link scheme not allowed", ErrInvalidInput)
	}

	if c.opener == nil {
		return &OpenError{Reason: "unavailable", err: errOpenerNotWired}
	}

	if err := c.opener.Open(ctx, url); err != nil {
		return &OpenError{Reason: "unavailable", err: err}
	}

	return nil
}

// OpenFile asks the desktop portal to open the local file at path with
// its default handler. path must resolve, after symlinks, inside the
// media cache, the outgoing attachment area or the downloads
// directory, or be exactly the key-bindings file; anything else is
// refused as invalid input.
func (c *Commands) OpenFile(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("%w: path is required", ErrInvalidInput)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("%w: path does not exist", ErrInvalidInput)
	}

	if !c.openPathAllowed(resolved) {
		return fmt.Errorf("%w: path is not allowed", ErrInvalidInput)
	}

	if c.opener == nil {
		return &OpenError{Reason: "unavailable", err: errOpenerNotWired}
	}

	if err := c.opener.OpenFile(ctx, resolved); err != nil {
		return &OpenError{Reason: "unavailable", err: err}
	}

	return nil
}

// openPathAllowed reports whether resolved names the key-bindings file
// or lies inside the media cache, the outgoing attachment area or the
// downloads directory. An empty or unset root is skipped, never
// treated as matching everything.
func (c *Commands) openPathAllowed(resolved string) bool {
	if c.keysConfigPath != "" && sameResolvedFile(c.keysConfigPath, resolved) {
		return true
	}

	for _, root := range []string{c.mediaCacheDir, c.outgoingDir, c.downloadsDir} {
		if root != "" && resolvedWithinRoot(root, resolved) {
			return true
		}
	}

	return false
}

// sameResolvedFile reports whether root, once symlinks are followed,
// names the same file as resolved.
func sameResolvedFile(root, resolved string) bool {
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}

	return rootResolved == resolved
}

// resolvedWithinRoot reports whether resolved lies strictly inside
// root, once root's own symlinks are followed too.
func resolvedWithinRoot(root, resolved string) bool {
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(rootResolved, resolved)
	if err != nil {
		return false
	}

	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
