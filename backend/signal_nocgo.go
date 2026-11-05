//go:build !cgo

package main

import (
	"log"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// signalProviders returns nothing: without cgo there is no libsignal link.
func signalProviders() []connector.Provider {
	return nil
}

// signalSetUpLogging does nothing: without cgo, signalmeow is never
// linked in, so there is no logger of its own to redirect.
func signalSetUpLogging(*log.Logger) {}
