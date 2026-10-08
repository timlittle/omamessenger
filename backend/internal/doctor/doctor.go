// Package doctor turns a snapshot of the helper's own health into a
// short, safe report: version consistency, data permissions, the
// database, the media cache, each account's last known connection state,
// whether notify-send is on the path, and any recent error categories
// recorded in process. Every check's detail text is safe to show as is:
// a state or a category word, never a path, a count, a name or a token.
// Gathering the facts is impure (file stats, a database query, a PATH
// lookup); Build itself does none of that, so every combination of
// outcomes is cheap to test.
package doctor

import "strings"

// State is a check's raw outcome before Build turns it into a Check.
// StateUnknown means the fact was never gathered, or the thing it
// describes does not exist yet (a fresh install with no media cached
// yet, say); neither counts as a problem.
type State int

// The three states a gathered fact can be in.
const (
	StateUnknown State = iota
	StateGood
	StateBad
)

// executablePrefix names the installer's own naming convention for the
// installed binary; see scripts/install-helper.sh.
const executablePrefix = "oma-messenger-service-"

// Facts are the raw observations Build turns into a Report.
type Facts struct {
	// CompiledVersion is this helper's own version, built in.
	CompiledVersion string
	// ExecutableName is the running binary's own file name, used to
	// recover the version it was installed as, when it follows the
	// installer's naming convention.
	ExecutableName string

	// DataDirSecure and DBFileSecure report the data directory's and the
	// database file's permissions: 0700 and 0600 respectively.
	DataDirSecure State
	DBFileSecure  State

	// DatabaseOK is true once the database has opened and answered a
	// query, which also confirms its migrations ran.
	DatabaseOK bool

	// CacheSize reports whether the media cache is within its limit.
	CacheSize State

	// Accounts are every configured account's service and last known
	// connection status.
	Accounts []AccountFact

	// NotifySendAvailable is true when notify-send is on the PATH.
	NotifySendAvailable bool

	// RecentErrorCategories are the last few safe error categories
	// recorded during this run, oldest first; empty when none have been
	// recorded, or when gathering facts outside a running helper, which
	// has no history to report.
	RecentErrorCategories []string
}

// AccountFact is one account's service and last known connection status,
// never its name: Account.Status is already one of a small set of safe
// words (domain.AccountConnected and its siblings).
type AccountFact struct {
	Service string
	Status  string
}

// Check is one line of the report: a name, whether it is fine, and a
// short, safe detail.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Report is every check Build ran, in a fixed order.
type Report struct {
	Checks []Check `json:"checks"`
}

// Problems counts the checks that are not OK.
func (r Report) Problems() int {
	n := 0
	for _, c := range r.Checks {
		if !c.OK {
			n++
		}
	}

	return n
}

// Build turns a snapshot of facts into a report, one check per concern
// plus one per account.
func Build(f Facts) Report {
	checks := []Check{
		versionCheck(f),
		permissionsCheck(f),
		databaseCheck(f),
		cacheCheck(f),
		notifyCheck(f),
		recentErrorsCheck(f),
	}
	for _, a := range f.Accounts {
		checks = append(checks, accountCheck(a))
	}

	return Report{Checks: checks}
}

// versionCheck compares the compiled version against the version the
// running executable's own file name claims, when it follows the
// installer's "oma-messenger-service-<version>" naming; a dev build run
// under another name has nothing to compare, which is not a problem.
func versionCheck(f Facts) Check {
	pinned, ok := strings.CutPrefix(f.ExecutableName, executablePrefix)
	if !ok {
		return Check{Name: "Helper version", OK: true, Detail: "running an unpinned build"}
	}

	if pinned != f.CompiledVersion {
		return Check{Name: "Helper version", OK: false, Detail: "does not match its installed pin"}
	}

	return Check{Name: "Helper version", OK: true, Detail: "matches its installed pin"}
}

// permissionsCheck flags a data directory or database file looser than
// the 0700/0600 privacy.md requires; a path never checked, or not
// created yet, is not a problem.
func permissionsCheck(f Facts) Check {
	if f.DataDirSecure == StateBad || f.DBFileSecure == StateBad {
		return Check{Name: "Data permissions", OK: false, Detail: "looser than expected"}
	}

	return Check{Name: "Data permissions", OK: true, Detail: "private to this account"}
}

// databaseCheck follows Facts.DatabaseOK, which already confirms the
// database opened, answered a query and so has its migrations current.
func databaseCheck(f Facts) Check {
	if !f.DatabaseOK {
		return Check{Name: "Database", OK: false, Detail: "could not open"}
	}

	return Check{Name: "Database", OK: true, Detail: "open and up to date"}
}

// cacheCheck flags a media cache that has grown past its limit; one
// never checked is not a problem.
func cacheCheck(f Facts) Check {
	if f.CacheSize == StateBad {
		return Check{Name: "Media cache", OK: false, Detail: "over its limit"}
	}

	return Check{Name: "Media cache", OK: true, Detail: "within its limit"}
}

// notifyCheck flags a missing notify-send, since without it no
// notification setting can do anything.
func notifyCheck(f Facts) Check {
	if !f.NotifySendAvailable {
		return Check{Name: "Desktop notifications", OK: false, Detail: "notify-send not found"}
	}

	return Check{Name: "Desktop notifications", OK: true, Detail: "notify-send available"}
}

// accountCheck flags an account waiting for sign-in or in error; any
// other status, including still connecting, is not a problem on its own.
func accountCheck(a AccountFact) Check {
	broken := a.Status == "error" || a.Status == "needs-auth"

	return Check{Name: "Account (" + a.Service + ")", OK: !broken, Detail: a.Status}
}

// recentErrorsCheck reports the last few error categories recorded, when
// any are available; this is history for context, never a live fault on
// its own, so it is always OK.
func recentErrorsCheck(f Facts) Check {
	if len(f.RecentErrorCategories) == 0 {
		return Check{Name: "Recent errors", OK: true, Detail: "none recorded"}
	}

	return Check{Name: "Recent errors", OK: true, Detail: strings.Join(f.RecentErrorCategories, ", ")}
}
