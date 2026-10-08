package connector

// retry.go lets a connector's Send error tell the app layer's retry
// scheduler whether an automatic retry is worth trying at all, and, when
// the service itself says how long to wait, how long that is.

import (
	"errors"
	"time"
)

// ErrSendPermanent reports an outgoing message a service refused for a
// reason that retrying will never fix: the recipient is gone, blocked
// this account, or similar. A connector's Send wraps its own error with
// this so the retry scheduler stops trying rather than retry forever.
var ErrSendPermanent = errors.New("connector: send permanently refused")

// SendRetryAfter is a Send error that also carries how long the service
// itself asked to wait before trying again, such as Telegram's flood
// wait. The retry scheduler honours it instead of its own backoff
// whenever it asks for longer.
type SendRetryAfter struct {
	Err   error
	After time.Duration
}

// Error returns the wrapped error's own message.
func (e *SendRetryAfter) Error() string { return e.Err.Error() }

// Unwrap exposes the wrapped error for errors.Is and errors.As.
func (e *SendRetryAfter) Unwrap() error { return e.Err }

// RetryAfter reports the wait a Send error's service explicitly asked
// for, if any.
func RetryAfter(err error) (time.Duration, bool) {
	var ra *SendRetryAfter
	if errors.As(err, &ra) {
		return ra.After, true
	}

	return 0, false
}
