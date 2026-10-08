package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MessageExists reports whether a message with id is still stored, so
// Sweep can tell an outgoing copy that belongs to no message at all -
// an image pasted from the clipboard but never sent, say, or one whose
// message the user deleted - from a pending or failed message's copy,
// which Sweep must never remove.
type MessageExists func(ctx context.Context, id string) (bool, error)

// Outgoing stores the files a user attaches to outgoing messages, named
// by the message's id, so a retry can resend one even after the user
// moves, renames or deletes the original. A copy is kept until its
// message is confirmed delivered (Remove), deleted (also Remove) or,
// for one that never became a message at all, until Sweep's grace
// period has passed; a pending or failed message's copy is never swept.
type Outgoing struct {
	dir       string
	grace     time.Duration
	sizeLimit int64
	exists    MessageExists

	mu       sync.Mutex
	inflight map[string]int
}

// NewOutgoing returns an outgoing media area rooted at dir. Sweep
// removes a file older than grace whose id matches no message exists
// reports as stored at all; sizeLimit is reported through Stats for a
// health check to warn about, never evicted against. Neither applies
// until Sweep or RunSweeper is actually called.
func NewOutgoing(dir string, grace time.Duration, sizeLimit int64, exists MessageExists) *Outgoing {
	return &Outgoing{dir: dir, grace: grace, sizeLimit: sizeLimit, exists: exists, inflight: map[string]int{}}
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
// confirmed sent, delivered or read, or once the user deletes it.
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
// reading it - or a delete racing the same send, since both go through
// Remove without checking this themselves. Call the returned func
// exactly once, success or not, to release the mark once the send ends.
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

// Sweep drops outgoing copies abandoned by anything recognisable as a
// message: a file older than grace whose id matches no message at all,
// such as an image pasted from the clipboard but never sent, or one
// whose message the user deleted. It never removes a pending or failed
// message's copy - those wait for Remove, once sent or deleted - and
// never a file Reserve currently marks as being sent.
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

	cutoff := time.Now().Add(-o.grace)

	var errs []error
	for _, e := range entries {
		if err := o.sweepOne(ctx, e, cutoff); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// sweepOne removes e's file when it is a finished, unreserved file older
// than cutoff whose id matches no message at all; anything else -
// still being written, reserved, too young to judge yet, or belonging
// to a real message - is left alone.
func (o *Outgoing) sweepOne(ctx context.Context, e os.DirEntry, cutoff time.Time) error {
	info, err := e.Info()
	if err != nil || !info.Mode().IsRegular() || strings.HasSuffix(e.Name(), partSuffix) {
		return nil
	}

	if info.ModTime().After(cutoff) || o.reserved(attachmentID(e.Name())) {
		return nil
	}

	exists, err := o.exists(ctx, attachmentID(e.Name()))
	if err != nil || exists {
		return err
	}

	return removeIfPresent(filepath.Join(o.dir, e.Name()))
}

// RunSweeper calls Sweep once immediately, then again every interval,
// until ctx is cancelled. A failed sweep is logged, not fatal: it only
// leaves an orphaned file around until the next tick.
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
