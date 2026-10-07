package telegram

import (
	gotd "github.com/gotd/td/telegram"
)

// deviceConfig names this helper on Telegram's own device list. Its
// DeviceModel and SystemVersion show up wherever Telegram lists a
// signed-in session's device; SystemVersion names the distro this
// plugin targets rather than this machine's actual kernel or hostname,
// which Telegram has no reason to know. version is this helper's own
// release (see main.go's helperVersion). Every other DeviceConfig field
// is left at its zero value, which gotd.Options.setDefaults fills in
// (language codes and so on) exactly as it does for a caller that sets
// no Device at all.
func deviceConfig(version string) gotd.DeviceConfig {
	return gotd.DeviceConfig{
		DeviceModel:   "OmaMessenger",
		SystemVersion: "Omarchy",
		AppVersion:    version,
	}
}
