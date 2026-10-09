//go:build fake

package main

import (
	"os"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
)

// fakeConnectors returns the scripted fake accounts that test builds run,
// and the injector tests use to deliver messages. OMA_FAKE_DEMO switches
// to the smaller, curated set the README demo recording uses instead.
func fakeConnectors() ([]connector.Connector, app.Injector) {
	suite := fake.New()
	if os.Getenv("OMA_FAKE_DEMO") != "" {
		suite = fake.NewDemo()
	}

	connectors := suite.Connectors()

	return connectors, suite
}
