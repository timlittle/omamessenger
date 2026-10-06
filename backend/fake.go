//go:build fake

package main

import (
	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
)

// fakeConnectors returns the scripted fake accounts that test builds run,
// and the injector tests use to deliver messages.
func fakeConnectors() ([]connector.Connector, app.Injector) {
	suite := fake.New()

	return suite.Connectors(), suite
}
