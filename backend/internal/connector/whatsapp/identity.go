package whatsapp

// identity.go names this helper on WhatsApp's own "Linked devices" list
// on the phone. whatsmeow keeps that identity in store.DeviceProps, a
// package-level global with no per-client equivalent, so every account
// this process connects shares the one value; identifyDevice sets it
// once, from New, rather than in an init func, so it runs as part of
// building a connector rather than as a load-time side effect.

import (
	"sync"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
)

// identifyOnce guards identifyDevice, so concurrent connectors never
// race on whatsmeow's shared globals and the identity is only ever set
// once per process.
var identifyOnce sync.Once

// identifyDevice sets whatsmeow's device identity to "OmaMessenger" on
// a desktop, which is what the phone's Linked Devices list shows once
// an account pairs.
func identifyDevice() {
	identifyOnce.Do(func() {
		store.SetOSInfo("OmaMessenger", [3]uint32{1, 0, 0}) // the version is not shown on the phone; only the name is
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()
	})
}
