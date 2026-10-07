package telegram

// identity_test.go checks the device identity wiring directly: version
// is unexported and deviceConfig's result never crosses a public
// method, so there is no way to reach either through Connector's
// exported API.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestDeviceConfig_NamesThisHelperAndCarriesItsVersion(t *testing.T) {
	t.Parallel()

	d := deviceConfig("1.2.3")
	if d.DeviceModel != "OmaMessenger" || d.SystemVersion != "Omarchy" || d.AppVersion != "1.2.3" {
		t.Errorf("deviceConfig = %+v, want OmaMessenger on Omarchy at version 1.2.3", d)
	}
}

func TestProvider_ConnectCarriesItsVersionToTheConnector(t *testing.T) {
	t.Parallel()

	p := Provider{Version: "9.9.9"}

	c, ok := p.Connect(domain.Account{ID: "tg-1"}, t.TempDir()).(*Connector)
	if !ok {
		t.Fatal("Connect did not return a *Connector")
	}
	if c.version != "9.9.9" {
		t.Errorf("connector version = %q, want 9.9.9", c.version)
	}
}
