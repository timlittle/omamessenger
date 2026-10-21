package connector

// limits.go gives every connector's FetchMedia the same discipline for
// a video, file or voice note (a photo, sticker or real GIF file
// instead goes through imagesafe's own, stricter profile-based caps):
// check the sender's own declared size before asking the service for
// anything, then cap the bytes actually received, bounded by an idle
// timeout so a stalled transfer is caught quickly without cutting off
// a slow but steadily arriving one.

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// AutoFetchMaxBytes bounds a fetch the UI made on its own, with no
// click from the user asking for this particular file: today, only an
// animated "GIF" video autoplaying as soon as its message bubble is
// shown (WhatsApp's gifPlayback flag, Telegram's animated document
// attribute - see media.fetch's auto param in docs/api.md). A sender
// fully controls that flag, so the UI must never let it alone justify
// downloading something large before the user has asked for anything;
// 16 MiB is generous for the short, silent clips services actually
// mark this way (most are well under a MiB) while bounding what a
// hostile sender can make every reader of a conversation download
// without a click.
const AutoFetchMaxBytes int64 = 16 << 20

// MaxUserFetchBytes bounds a video, file or voice note the user
// explicitly clicked to open, play or save. Generous enough for any
// ordinary attachment a messaging service carries - WhatsApp and
// Telegram both cap a single upload well under this themselves - while
// still bounding the memory and disk a single download can claim
// against a hostile or corrupted reference.
const MaxUserFetchBytes int64 = 2 << 30

// SizeTolerance is added to a sender's own declared size before
// capping the bytes a download may actually receive, so a declared
// count that is accurate but rounded, or a little short of the real
// framed size, is never refused for undershooting its own number by a
// few bytes. It is never added on top of AutoFetchMaxBytes or
// MaxUserFetchBytes themselves: see ByteCap.
const SizeTolerance int64 = 1 << 20

// IdleTimeout bounds how long a streamed download may go without any
// bytes arriving at all, for a user-requested video or file large
// enough to stream to disk rather than buffer in memory (see
// AbsoluteCeiling). Preferred over a single flat deadline for these:
// a slow but steadily arriving transfer of a large file should not be
// cut off just because it takes longer than a small one would, while a
// connection that has stalled outright is still caught quickly.
const IdleTimeout = 30 * time.Second

// AbsoluteCeiling bounds a streamed, user-requested download overall,
// however steadily its bytes keep arriving, so a connection that never
// actually stalls long enough to trip IdleTimeout still cannot hold a
// fetch open forever.
const AbsoluteCeiling = 10 * time.Minute

// ErrMediaTooLarge reports a download refused because its declared or
// actual size exceeds the limit that applies to it: AutoFetchMaxBytes
// for an automatic fetch, MaxUserFetchBytes otherwise. Refused outright
// before any bytes are requested, or as soon as more than the capped
// amount (see ByteCap) has actually arrived, this is always a
// permanent failure and is never retried.
var ErrMediaTooLarge = errors.New("connector: media exceeds the size limit")

// MaxBytes is the hard cap that applies to a download: AutoFetchMaxBytes
// for an automatic fetch (auto true), MaxUserFetchBytes otherwise.
func MaxBytes(auto bool) int64 {
	if auto {
		return AutoFetchMaxBytes
	}

	return MaxUserFetchBytes
}

// ImageFetchCap is the hard byte limit any connector's FetchMedia ever
// streams a photo, sticker or GIF image's own bytes to: profile's own
// MaxInputLen, narrowed to AutoFetchMaxBytes when auto is set and that
// cap is smaller. A profile's own cap alone (imagesafe.FullImage's 32
// MiB) can allow more than every other automatic fetch is trusted
// with, so an automatic fetch of an image is never the exception: a
// sender fully controls whether their photo autoplays or merely shows
// as a thumbnail until shown, the same as any other media kind (see
// AutoFetchMaxBytes).
func ImageFetchCap(profile imagesafe.Profile, auto bool) int64 {
	if auto && AutoFetchMaxBytes < profile.MaxInputLen {
		return AutoFetchMaxBytes
	}

	return profile.MaxInputLen
}

// ByteCap narrows max to a sender's own declared size plus
// SizeTolerance, so a stream that keeps growing past what it claimed
// is refused well before max is ever reached. A declared size that is
// zero or negative (unknown) or already leaves no room for the
// tolerance below max is simply capped at max itself.
func ByteCap(declared, max int64) int64 {
	if declared <= 0 || declared > max-SizeTolerance {
		return max
	}

	return declared + SizeTolerance
}

// BoundedWriter wraps W, failing with ErrMediaTooLarge once more than
// Max bytes have been written to it in total: the actual-bytes
// backstop behind a declared size that undersold what a stream really
// sends. OnWrite, when set, runs after every write that actually sent
// at least one byte through, so a caller can reset an idle timeout
// (see IdleContext) exactly when the transfer is still making
// progress.
type BoundedWriter struct {
	W       io.Writer
	Max     int64
	OnWrite func()

	written int64
}

// Write appends p to W, refusing once the total written would exceed Max.
func (b *BoundedWriter) Write(p []byte) (int, error) {
	if b.written+int64(len(p)) > b.Max {
		return 0, ErrMediaTooLarge
	}

	n, err := b.W.Write(p)
	b.written += int64(n)
	if n > 0 && b.OnWrite != nil {
		b.OnWrite()
	}

	return n, err
}

// IdleContext returns a context derived from parent that is canceled
// once ceiling elapses regardless, or sooner if reset is not called
// again within idle of the last call (or of IdleContext itself, before
// the first). stop releases the timer and must always be called by the
// caller, typically deferred right after this returns.
//
// An idle-triggered cancellation reports context.Canceled on the
// returned context, not context.DeadlineExceeded: Go's own context
// package reserves DeadlineExceeded for a deadline actually elapsing on
// its own timer, which only the absolute ceiling, never idle's early
// manual cancel, triggers here. Callers that classify a download
// failure (see app.fetchFailureReason) already treat both the same way,
// as a timeout.
func IdleContext(parent context.Context, idle, ceiling time.Duration) (ctx context.Context, reset func(), stop func()) {
	ctx, cancel := context.WithTimeout(parent, ceiling)
	timer := time.AfterFunc(idle, cancel)

	return ctx, func() { timer.Reset(idle) }, func() { timer.Stop(); cancel() }
}
