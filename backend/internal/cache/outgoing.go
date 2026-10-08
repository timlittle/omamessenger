package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Outgoing stores the files a user attaches to outgoing messages, named
// by the message's id, so a retry can resend one even after the user
// moves, renames or deletes the original. Unlike Cache, which only ever
// remembers the most recently used files, Outgoing keeps a copy until
// its message is confirmed delivered (Remove) or, for one still pending
// or failed, until Sweep's retention window or size limit says it has
// waited long enough.
type Outgoing struct {
	dir       string
	retention time.Duration
	sizeLimit int64

	mu       sync.Mutex
	inflight map[string]int
}

// NewOutgoing returns an outgoing media area rooted at dir. Sweep removes
// a copy older than retention, and, when the area still holds more than
// sizeLimit bytes, the least recently used copies next; neither applies
// until Sweep or RunSweeper is actually called.
func NewOutgoing(dir string, retention time.Duration, sizeLimit int64) *Outgoing {
	return &Outgoing{dir: dir, retention: retention, sizeLimit: sizeLimit, inflight: map[string]int{}}
}

// Store copies the bytes of r into the outgoing media area under id and
// fileName, readable only by the user, and returns the new path. A
// failed or interrupted copy leaves nothing behind.
func (o *Outgoing) Store(ctx context.Context, id, fileName string, r io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(o.dir, 0o700); err != nil {
		return "", fmt.Errorf("cache: outgoing: %w", err)
	}

	dst := o.Path(id, fileName)
	if err := copyToFile(dst, r); err != nil {
		return "", fmt.Errorf("cache: outgoing: %w", err)
	}

	return dst, nil
}

// Path is where an attachment named fileName for message id is stored,
// found again without re-copying it, such as for a retry.
func (o *Outgoing) Path(id, fileName string) string {
	return filepath.Join(o.dir, id+"-"+fileName)
}

// Remove deletes id's stored copy of fileName, once its message is
// confirmed sent, delivered or read and so will never be retried.
// Removing one already gone is not an error: a delivery receipt can
// arrive more than once, or the copy may already have been swept.
func (o *Outgoing) Remove(ctx context.Context, id, fileName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := removeIfPresent(o.Path(id, fileName)); err != nil {
		return fmt.Errorf("cache: outgoing: remove: %w", err)
	}

	return nil
}

// Reserve marks id as having a send in progress right now, so Sweep
// never removes its file out from under the connector that may still be
// reading it. Call the returned func exactly once, success or not, to
// release the mark once the send ends.
func (o *Outgoing) Reserve(id string) func() {
	o.mu.Lock()
	o.inflight[id]++
	o.mu.Unlock()

	return func() {
		o.mu.Lock()
		defer o.mu.Unlock()

		o.inflight[id]--
		if o.inflight[id] <= 0 {
			delete(o.inflight, id)
		}
	}
}

// reserved reports whether id currently has a send in progress.
func (o *Outgoing) reserved(id string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.inflight[id] > 0
}

// Stats returns how many bytes the outgoing area currently holds and its
// size limit, for a health check to compare. An area not created yet
// simply holds zero bytes, not an error.
func (o *Outgoing) Stats() (bytes, limit int64, err error) {
	entries, err := os.ReadDir(o.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, o.sizeLimit, nil
		}

		return 0, o.sizeLimit, fmt.Errorf("cache: outgoing: stats: %w", err)
	}

	_, total := cached(entries)

	return total, o.sizeLimit, nil
}

// Sweep drops outgoing copies that no longer need keeping: first any
// older than retention, then, if the area still holds more than
// sizeLimit bytes, the least recently used copies until it fits. A copy
// whose message Reserve currently marks as being sent is never touched,
// no matter its age or the area's size.
func (o *Outgoing) Sweep(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	entries, err := os.ReadDir(o.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("cache: outgoing: sweep: %w", err)
	}

	files, total := cached(entries)
	slices.SortFunc(files, func(a, b os.FileInfo) int { return a.ModTime().Compare(b.ModTime()) })

	kept, removed, total := o.sweepExpired(files, total)
	removed = append(removed, o.sweepOverLimit(kept, total)...)

	return errors.Join(removed...)
}

// sweepExpired removes every file in files older than retention, except
// one Reserve currently marks as being sent, and returns the files it
// kept, any removal errors, and the total bytes still held.
func (o *Outgoing) sweepExpired(files []os.FileInfo, total int64) (kept []os.FileInfo, errs []error, _ int64) {
	cutoff := time.Now().Add(-o.retention)

	for _, f := range files {
		if o.reserved(attachmentID(f.Name())) || !f.ModTime().Before(cutoff) {
			kept = append(kept, f)
			continue
		}

		errs = append(errs, removeIfPresent(filepath.Join(o.dir, f.Name())))
		total -= f.Size()
	}

	return kept, errs, total
}

// sweepOverLimit removes the least recently used of files, which must
// already be sorted oldest first, until total is within sizeLimit,
// skipping any file Reserve currently marks as being sent.
func (o *Outgoing) sweepOverLimit(files []os.FileInfo, total int64) []error {
	var errs []error
	for _, f := range files {
		if total <= o.sizeLimit {
			break
		}

		if o.reserved(attachmentID(f.Name())) {
			continue
		}

		errs = append(errs, removeIfPresent(filepath.Join(o.dir, f.Name())))
		total -= f.Size()
	}

	return errs
}

// RunSweeper calls Sweep once immediately, then again every interval,
// until ctx is cancelled. A failed sweep is logged, not fatal: it only
// leaves the area a little larger until the next tick.
func (o *Outgoing) RunSweeper(ctx context.Context, interval time.Duration, logger Logger) {
	if err := o.Sweep(ctx); err != nil && logger != nil {
		logger.Printf("cache: outgoing sweep failed")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := o.Sweep(ctx); err != nil && logger != nil {
				logger.Printf("cache: outgoing sweep failed")
			}
		}
	}
}

// Logger receives one diagnostic line when a background sweep fails.
// *log.Logger already satisfies it.
type Logger interface {
	Printf(format string, v ...any)
}

// attachmentID returns the message id a stored file's name starts with:
// the part before its first '-', which Path always puts there.
func attachmentID(name string) string {
	if i := strings.IndexByte(name, '-'); i >= 0 {
		return name[:i]
	}

	return name
}

// copyToFile writes r to dst through a temporary file, so a failed or
// interrupted copy never leaves a partial file at dst, readable only by
// the user.
func copyToFile(dst string, r io.Reader) error {
	part := dst + partSuffix
	out, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, r); err != nil {
		return errors.Join(err, out.Close(), removeIfPresent(part))
	}

	if err := out.Close(); err != nil {
		return errors.Join(err, removeIfPresent(part))
	}

	if err := os.Rename(part, dst); err != nil {
		return errors.Join(err, removeIfPresent(part))
	}

	return nil
}
