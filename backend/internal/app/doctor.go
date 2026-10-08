package app

import (
	"context"
	"os"
	"os/exec"

	"github.com/timlittle/omamessenger/backend/internal/doctor"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// CacheStats reports how many bytes a media cache holds against its
// limit, a capability MediaCache itself does not need: Doctor still
// works, just without an opinion on the cache, when a test double does
// not implement it.
type CacheStats interface {
	Stats() (bytes, limit int64, err error)
}

// Doctor gathers the helper's own health report, from the data directory
// and database path, executable name and helper version wired in at
// startup (see Deps). Every account's last recorded status is used
// rather than a live probe, so Doctor works whether or not any connector
// has finished starting.
func (c *Commands) Doctor(ctx context.Context) (doctor.Report, error) {
	accounts, err := c.store.Accounts(ctx)

	facts := doctor.Facts{
		CompiledVersion:       c.helperVersion,
		ExecutableName:        c.execName,
		DataDirSecure:         permissionState(c.dataDir, 0o700),
		DBFileSecure:          permissionState(c.dbPath, 0o600),
		DatabaseOK:            err == nil,
		CacheSize:             cacheSizeState(c.cache),
		NotifySendAvailable:   notifySendOnPath(),
		RecentErrorCategories: c.recentErrors.snapshot(),
		Accounts:              accountFacts(accounts),
	}

	return doctor.Build(facts), nil
}

// accountFacts narrows accounts to what Doctor may show: a service and
// a connection status, never a name.
func accountFacts(accounts []domain.Account) []doctor.AccountFact {
	facts := make([]doctor.AccountFact, len(accounts))
	for i, a := range accounts {
		facts[i] = doctor.AccountFact{Service: a.Service, Status: a.Status}
	}

	return facts
}

// permissionState reports whether path's permissions match want; a path
// that cannot be stat'd (not created yet, or never checked) is unknown
// rather than a problem.
func permissionState(path string, want os.FileMode) doctor.State {
	info, err := os.Stat(path)
	if err != nil {
		return doctor.StateUnknown
	}

	if info.Mode().Perm() != want {
		return doctor.StateBad
	}

	return doctor.StateGood
}

// cacheSizeState reports whether cache holds no more than its limit, or
// unknown when cache does not implement CacheStats.
func cacheSizeState(c MediaCache) doctor.State {
	stats, ok := c.(CacheStats)
	if !ok {
		return doctor.StateUnknown
	}

	bytes, limit, err := stats.Stats()
	if err != nil {
		return doctor.StateUnknown
	}

	if bytes > limit {
		return doctor.StateBad
	}

	return doctor.StateGood
}

// notifySendOnPath reports whether notify-send is available to run.
func notifySendOnPath() bool {
	_, err := exec.LookPath("notify-send")

	return err == nil
}
