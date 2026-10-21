package connector_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestMaxBytes_PicksTheLimitForAutoOrUserRequested confirms the two
// hard caps every connector's FetchMedia is meant to pick between.
func TestMaxBytes_PicksTheLimitForAutoOrUserRequested(t *testing.T) {
	t.Parallel()

	if got := connector.MaxBytes(true); got != connector.AutoFetchMaxBytes {
		t.Errorf("MaxBytes(true) = %d, want AutoFetchMaxBytes", got)
	}
	if got := connector.MaxBytes(false); got != connector.MaxUserFetchBytes {
		t.Errorf("MaxBytes(false) = %d, want MaxUserFetchBytes", got)
	}
}

// TestByteCap_NarrowsToTheDeclaredSizePlusTolerance confirms a known
// declared size caps a download well below max, while an unknown or
// already-large declared size simply falls back to max itself.
func TestByteCap_NarrowsToTheDeclaredSizePlusTolerance(t *testing.T) {
	t.Parallel()

	const max = 100_000_000

	tests := map[string]struct {
		declared, max, want int64
	}{
		"known declared size adds the tolerance":   {declared: 100, max: max, want: 100 + connector.SizeTolerance},
		"zero declared size falls back to max":     {declared: 0, max: max, want: max},
		"negative declared size falls back to max": {declared: -1, max: max, want: max},
		"declared size near max falls back to max": {declared: max - 100, max: max, want: max},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := connector.ByteCap(tt.declared, tt.max); got != tt.want {
				t.Errorf("ByteCap(%d, %d) = %d, want %d", tt.declared, tt.max, got, tt.want)
			}
		})
	}
}

// TestBoundedWriter_PassesThroughUnderTheCap confirms writes within Max
// reach the underlying writer unchanged.
func TestBoundedWriter_PassesThroughUnderTheCap(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := &connector.BoundedWriter{W: &buf, Max: 10}

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() = %v, want no error under the cap", err)
	}
	if buf.String() != "hello" {
		t.Errorf("buf = %q, want %q", buf.String(), "hello")
	}
}

// TestImageFetchCap_NarrowsToTheAutoCapWhenSmaller confirms an
// automatic fetch of a photo, sticker or GIF image is never trusted
// with more than AutoFetchMaxBytes, even though imagesafe.FullImage's
// own MaxInputLen alone would allow more; a deliberate fetch keeps the
// profile's own, larger limit.
func TestImageFetchCap_NarrowsToTheAutoCapWhenSmaller(t *testing.T) {
	t.Parallel()

	if got := connector.ImageFetchCap(imagesafe.FullImage, true); got != connector.AutoFetchMaxBytes {
		t.Errorf("ImageFetchCap(FullImage, true) = %d, want AutoFetchMaxBytes", got)
	}
	if got := connector.ImageFetchCap(imagesafe.FullImage, false); got != imagesafe.FullImage.MaxInputLen {
		t.Errorf("ImageFetchCap(FullImage, false) = %d, want the profile's own MaxInputLen", got)
	}
}

// TestImageFetchCap_KeepsTheProfileCapWhenItIsAlreadySmaller confirms
// a profile whose own MaxInputLen is already under AutoFetchMaxBytes
// (imagesafe.Thumbnail) is never widened by an automatic fetch.
func TestImageFetchCap_KeepsTheProfileCapWhenItIsAlreadySmaller(t *testing.T) {
	t.Parallel()

	if got := connector.ImageFetchCap(imagesafe.Thumbnail, true); got != imagesafe.Thumbnail.MaxInputLen {
		t.Errorf("ImageFetchCap(Thumbnail, true) = %d, want the profile's own MaxInputLen", got)
	}
}

// TestBoundedWriter_RefusesOnceTheTotalExceedsMax confirms a single
// write that alone exceeds Max, and the combination of two writes that
// only exceeds it together, are both refused with ErrMediaTooLarge and
// never reach the underlying writer's bytes past the cap.
func TestBoundedWriter_RefusesOnceTheTotalExceedsMax(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := &connector.BoundedWriter{W: &buf, Max: 5}

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() = %v, want the first 5 bytes accepted", err)
	}
	if _, err := w.Write([]byte("!")); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Errorf("Write() = %v, want ErrMediaTooLarge once the total exceeds Max", err)
	}
	if buf.String() != "hello" {
		t.Errorf("buf = %q, want only the accepted bytes written", buf.String())
	}
}

// TestBoundedWriter_CallsOnWriteOnlyWhenBytesLand confirms OnWrite runs
// after every write that actually sent bytes through, but never after
// one refused for exceeding the cap.
func TestBoundedWriter_CallsOnWriteOnlyWhenBytesLand(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	calls := 0
	w := &connector.BoundedWriter{W: &buf, Max: 5, OnWrite: func() { calls++ }}

	if _, err := w.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("nope!")); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("Write() = %v, want ErrMediaTooLarge", err)
	}

	if calls != 1 {
		t.Errorf("OnWrite called %d times, want exactly 1 (only the write that landed)", calls)
	}
}

// TestIdleContext_CancelsOnceIdleTimeElapsesWithNoReset confirms a
// stalled transfer - one that never calls reset again - is cancelled
// once idle elapses, well before the absolute ceiling.
func TestIdleContext_CancelsOnceIdleTimeElapsesWithNoReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, _, stop := connector.IdleContext(t.Context(), time.Second, time.Hour)
		defer stop()

		time.Sleep(time.Second)
		synctest.Wait()

		select {
		case <-ctx.Done():
		default:
			t.Fatal("ctx was not cancelled once idle elapsed with no reset")
		}
	})
}

// TestIdleContext_ResetKeepsItAliveUntilTheCeiling confirms a transfer
// that keeps calling reset survives past the idle duration, but is
// still cut off once the absolute ceiling elapses regardless.
func TestIdleContext_ResetKeepsItAliveUntilTheCeiling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, reset, stop := connector.IdleContext(t.Context(), 2*time.Second, 5*time.Second)
		defer stop()

		for i := 0; i < 3; i++ {
			time.Sleep(time.Second)
			reset()
		}
		synctest.Wait()

		select {
		case <-ctx.Done():
			t.Fatal("ctx was cancelled despite being reset before idle elapsed")
		default:
		}

		time.Sleep(5 * time.Second)
		synctest.Wait()

		select {
		case <-ctx.Done():
		default:
			t.Fatal("ctx was not cancelled once the absolute ceiling elapsed")
		}
	})
}

// TestIdleContext_StopReleasesTheTimerWithoutCancellingEarly confirms
// stop alone, called right after a successful download, does not leave
// the timer firing later into a reused or already-cancelled context,
// and that ctx is left in a cancelled state so nothing reads from it
// again.
func TestIdleContext_StopReleasesTheTimerWithoutCancellingEarly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, _, stop := connector.IdleContext(t.Context(), time.Minute, time.Hour)
		stop()

		select {
		case <-ctx.Done():
		default:
			t.Fatal("ctx was not cancelled by stop")
		}

		if !errors.Is(context.Cause(ctx), context.Canceled) {
			t.Errorf("context.Cause(ctx) = %v, want context.Canceled", context.Cause(ctx))
		}
	})
}
