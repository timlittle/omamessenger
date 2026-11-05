//go:build cgo

package main

import (
	"log"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/signal"
)

// signalProviders returns the Signal provider: this build was linked
// against libsignal-ffi.
func signalProviders() []connector.Provider {
	return []connector.Provider{signal.Provider{}}
}

// signalSetUpLogging points signalmeow's own logger at out.
func signalSetUpLogging(out *log.Logger) {
	signal.SetUpLogging(out)
}
