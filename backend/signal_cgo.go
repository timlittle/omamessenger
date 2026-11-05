//go:build cgo

package main

import (
	// Importing signalmeow here, even with no Signal connector yet, makes
	// a cgo build actually link libsignal's native library: a plain
	// "go build" with nothing calling into it would never catch a
	// missing or mismatched archive until the connector itself landed.
	_ "go.mau.fi/mautrix-signal/pkg/signalmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// signalProviders returns the Signal provider, once the connector
// exists; for now a cgo build carries the native link with nothing to
// offer yet.
func signalProviders() []connector.Provider {
	return nil
}
