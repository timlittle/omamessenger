//go:build !cgo

package main

import "github.com/timlittle/omamessenger/backend/internal/connector"

// signalProviders returns nothing: without cgo there is no libsignal link.
func signalProviders() []connector.Provider {
	return nil
}
