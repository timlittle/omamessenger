//go:build !cgo

package main

import "github.com/timlittle/omamessenger/backend/internal/connector"

// signalProviders returns nothing: a build without cgo cannot link
// libsignal, so it carries no Signal provider at all.
func signalProviders() []connector.Provider {
	return nil
}
