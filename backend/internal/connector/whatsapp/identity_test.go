package whatsapp

// identifyDevice touches whatsmeow's own package-level globals, so this
// test reads them back directly rather than through any seam of ours;
// nothing here reaches WhatsApp. Because that identity is shared by
// every account the whole process connects, this only checks that it
// ends up correct once New has run, not that New is the only thing that
// could have set it.

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestNew_IdentifiesTheDeviceOnWhatsAppsOwnDeviceList(t *testing.T) {
	t.Parallel()

	New(domain.Account{ID: "wa-1"}, t.TempDir())

	if got := store.DeviceProps.GetOs(); got != "OmaMessenger" {
		t.Errorf("DeviceProps.Os = %q, want OmaMessenger set before Run can pair", got)
	}
	if got := store.DeviceProps.GetPlatformType(); got != waCompanionReg.DeviceProps_DESKTOP {
		t.Errorf("DeviceProps.PlatformType = %v, want DESKTOP", got)
	}
}
