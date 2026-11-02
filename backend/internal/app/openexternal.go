package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// OpenError reports that a link or a local file could not be opened
// outside this process: the desktop portal could not be reached, or
// the request it answered failed. Reason is the only detail that
// crosses the protocol (see server/errors.go): a safe, fixed category,
// never the underlying D-Bus failure's own text.
type OpenError struct {
	Reason string
	err    error
}

// Error renders the failure for the helper's own logs; only Reason
// ever reaches the UI (see server/errors.go).
func (e *OpenError) Error() string {
	return fmt.Sprintf("open externally: %s: %v", e.Reason, e.err)
}

// Unwrap exposes the underlying failure for errors.Is/As, without it
// ever crossing the protocol itself.
func (e *OpenError) Unwrap() error {
	return e.err
}

// errOpenerNotWired reports that this helper build has no URIOpener at
// all; wrapped in an *OpenError, reason "unavailable", the same as any
// other failure to open the target.
var errOpenerNotWired = errors.New("no uri opener wired in")

// OpenLink asks the desktop portal to open url with the user's own
// default handler, instead of starting a process (xdg-open) whose own
// arguments - carrying the link itself, which can be as sensitive as a
// password-reset token or a meeting password - any other local user
// could read through /proc (see docs/decisions.md). url is re-checked
// against the same scheme allow-list the UI already applies to a
// message's own link before this is ever called, so neither a UI bug
// nor a change to the UI's own copy of that list can widen what this
// helper will open.
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
// the user's own default handler for its kind: a file this helper
// already downloaded (a message's photo, video or other attachment,
// already fetched into the media cache by the time this is called) or
// one the user owns, such as this plugin's own keys.conf, handed to
// the portal as an open file descriptor rather than as a path a
// spawned process's own arguments would otherwise carry (see
// docs/decisions.md). path must resolve, once its own symlinks are
// followed, inside the media cache, the outgoing attachment area or
// the downloads directory, or be exactly the key-bindings file;
// anything else is refused with invalid input, whatever string the
// caller sent, since a files.open call reaching this far already
// crossed the protocol and can no longer be trusted the way the UI's
// own, narrower call sites are.
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

// openPathAllowed reports whether resolved - already run through
// filepath.EvalSymlinks - names the key-bindings file or lies inside
// the media cache, the outgoing attachment area or the downloads
// directory: the only local files OpenFile should ever open. A root
// left empty (not wired in this build) or one that does not itself
// exist is skipped, never treated as matching everything.
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

// sameResolvedFile reports whether root, once its own symlinks (if
// any) are followed, names the same file as resolved. root not
// existing at all is never a match, not an error, since there is then
// nothing for resolved to legitimately be.
func sameResolvedFile(root, resolved string) bool {
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}

	return rootResolved == resolved
}

// resolvedWithinRoot reports whether resolved lies inside root (never
// root itself, since every root this is checked against is a
// directory this helper keeps files in, not a file to open on its
// own), once root's own symlinks, if any, are followed too.
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
