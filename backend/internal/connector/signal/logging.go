package signal

import (
	"log"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
)

// SetUpLogging points signalmeow's own package-level logger (shared by
// libsignal-ffi, the Rust library behind it) at out, logging only
// warnings and above, since signalmeow logs some fields unsafe to keep
// (for example the whole media backup key, at info level) and this is
// process-wide and unsynchronized, so it must run once before any
// Signal account connects.
func SetUpLogging(out *log.Logger) {
	signalmeow.SetLogger(zerolog.New(logWriter{log: out}).Level(zerolog.WarnLevel))
}

// logWriter adapts a standard logger to an io.Writer, so zerolog's own
// warning-and-above output reaches it.
type logWriter struct {
	log *log.Logger
}

// Write logs p as one line, trimming the trailing newline zerolog adds.
func (w logWriter) Write(p []byte) (int, error) {
	n := len(p)
	for n > 0 && (p[n-1] == '\n' || p[n-1] == '\r') {
		n--
	}

	w.log.Print(string(p[:n]))

	return len(p), nil
}
