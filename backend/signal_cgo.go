//go:build cgo

package main

import (
	// Blank import: forces the cgo link now, before the connector exists.
	_ "go.mau.fi/mautrix-signal/pkg/signalmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// signalProviders returns no provider yet; the connector lands later.
func signalProviders() []connector.Provider {
	return nil
}
