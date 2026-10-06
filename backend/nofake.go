//go:build !fake

package main

import (
	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// fakeConnectors returns nothing: release builds run only real accounts.
func fakeConnectors() ([]connector.Connector, app.Injector) {
	return nil, nil
}
