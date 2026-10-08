// Package doctor_test exercises Build with every combination of facts,
// confirming each check's outcome and that Problems counts only the
// checks that are not OK.
package doctor_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/doctor"
)

// TestBuild_VersionChecksTheExecutableNameAgainstTheCompiledVersion
// confirms the version check only has an opinion when the executable
// follows the installer's "oma-messenger-service-<version>" naming.
func TestBuild_VersionChecksTheExecutableNameAgainstTheCompiledVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		facts  doctor.Facts
		wantOK bool
	}{
		{"matches the pin", doctor.Facts{CompiledVersion: "0.3.0", ExecutableName: "oma-messenger-service-0.3.0"}, true},
		{"does not match the pin", doctor.Facts{CompiledVersion: "0.3.0", ExecutableName: "oma-messenger-service-0.2.0"}, false},
		{"an unpinned dev build", doctor.Facts{CompiledVersion: "0.3.0", ExecutableName: "oma-messenger-service"}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := checkNamed(t, doctor.Build(c.facts), "Helper version")
			if got.OK != c.wantOK {
				t.Errorf("OK = %t, want %t (detail %q)", got.OK, c.wantOK, got.Detail)
			}
		})
	}
}

// TestBuild_PermissionsFlagLooserThanExpected confirms a secure data
// directory and database file pass, an insecure one fails, and a path
// that has never been checked (or does not exist yet) is not a problem.
func TestBuild_PermissionsFlagLooserThanExpected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		facts  doctor.Facts
		wantOK bool
	}{
		{"both secure", doctor.Facts{DataDirSecure: doctor.StateGood, DBFileSecure: doctor.StateGood}, true},
		{"data directory too open", doctor.Facts{DataDirSecure: doctor.StateBad, DBFileSecure: doctor.StateGood}, false},
		{"database file too open", doctor.Facts{DataDirSecure: doctor.StateGood, DBFileSecure: doctor.StateBad}, false},
		{"neither checked yet", doctor.Facts{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := checkNamed(t, doctor.Build(c.facts), "Data permissions")
			if got.OK != c.wantOK {
				t.Errorf("OK = %t, want %t (detail %q)", got.OK, c.wantOK, got.Detail)
			}
		})
	}
}

// TestBuild_DatabaseReflectsWhetherItOpened confirms the database check
// follows Facts.DatabaseOK directly.
func TestBuild_DatabaseReflectsWhetherItOpened(t *testing.T) {
	t.Parallel()

	if got := checkNamed(t, doctor.Build(doctor.Facts{DatabaseOK: true}), "Database"); !got.OK {
		t.Errorf("an opened database reports a problem: %q", got.Detail)
	}
	if got := checkNamed(t, doctor.Build(doctor.Facts{DatabaseOK: false}), "Database"); got.OK {
		t.Errorf("a database that failed to open reports no problem: %q", got.Detail)
	}
}

// TestBuild_CacheFlagsOverLimit confirms the media cache check follows
// Facts.CacheSize, treating an unchecked cache as fine.
func TestBuild_CacheFlagsOverLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		state  doctor.State
		wantOK bool
	}{
		{"within its limit", doctor.StateGood, true},
		{"over its limit", doctor.StateBad, false},
		{"not checked", doctor.StateUnknown, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := checkNamed(t, doctor.Build(doctor.Facts{CacheSize: c.state}), "Media cache")
			if got.OK != c.wantOK {
				t.Errorf("OK = %t, want %t (detail %q)", got.OK, c.wantOK, got.Detail)
			}
		})
	}
}

// TestBuild_OutgoingSizeFlagsOverLimit confirms the outgoing attachments
// check follows Facts.OutgoingSize, treating an unchecked area as fine,
// the same as the downloaded media cache.
func TestBuild_OutgoingSizeFlagsOverLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		state  doctor.State
		wantOK bool
	}{
		{"within its limit", doctor.StateGood, true},
		{"over its limit", doctor.StateBad, false},
		{"not checked", doctor.StateUnknown, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := checkNamed(t, doctor.Build(doctor.Facts{OutgoingSize: c.state}), "Outgoing attachments")
			if got.OK != c.wantOK {
				t.Errorf("OK = %t, want %t (detail %q)", got.OK, c.wantOK, got.Detail)
			}
		})
	}
}

// TestBuild_NotifySendAvailability confirms the desktop notification
// check follows Facts.NotifySendAvailable.
func TestBuild_NotifySendAvailability(t *testing.T) {
	t.Parallel()

	if got := checkNamed(t, doctor.Build(doctor.Facts{NotifySendAvailable: true}), "Desktop notifications"); !got.OK {
		t.Errorf("notify-send available reports a problem: %q", got.Detail)
	}
	if got := checkNamed(t, doctor.Build(doctor.Facts{NotifySendAvailable: false}), "Desktop notifications"); got.OK {
		t.Errorf("notify-send missing reports no problem: %q", got.Detail)
	}
}

// TestBuild_OneCheckPerAccountFlagsErrorAndNeedsAuth confirms each
// account gets its own check, failing only when it needs sign-in or has
// errored.
func TestBuild_OneCheckPerAccountFlagsErrorAndNeedsAuth(t *testing.T) {
	t.Parallel()

	facts := doctor.Facts{Accounts: []doctor.AccountFact{
		{Service: "telegram", Status: "connected"},
		{Service: "whatsapp", Status: "error"},
		{Service: "telegram", Status: "needs-auth"},
	}}

	report := doctor.Build(facts)
	accountChecks := checksFor(report, "Account")
	if len(accountChecks) != 3 {
		t.Fatalf("got %d account checks, want 3 (one per account): %+v", len(accountChecks), accountChecks)
	}

	wantOK := []bool{true, false, false}
	for i, c := range accountChecks {
		if c.OK != wantOK[i] {
			t.Errorf("account check %d: OK = %t, want %t (detail %q)", i, c.OK, wantOK[i], c.Detail)
		}
	}
}

// TestBuild_ProblemsCountsOnlyFailingChecks confirms Problems tallies
// exactly the checks that are not OK.
func TestBuild_ProblemsCountsOnlyFailingChecks(t *testing.T) {
	t.Parallel()

	report := doctor.Build(doctor.Facts{
		DatabaseOK:          false,
		NotifySendAvailable: false,
		Accounts:            []doctor.AccountFact{{Service: "telegram", Status: "connected"}},
	})

	if got := report.Problems(); got != 2 {
		t.Errorf("Problems = %d, want 2 (database and notify-send)", got)
	}
}

// TestBuild_RecentErrorsAreInformationalOnly confirms a recorded error
// category never counts as a problem on its own; it is a history, not a
// live fault.
func TestBuild_RecentErrorsAreInformationalOnly(t *testing.T) {
	t.Parallel()

	report := doctor.Build(doctor.Facts{RecentErrorCategories: []string{"timeout", "expired"}})
	if got := checkNamed(t, report, "Recent errors"); !got.OK {
		t.Errorf("a recorded error category counts as a problem: %q", got.Detail)
	}
}

// checkNamed finds the one check named name, failing the test if there is
// not exactly one.
func checkNamed(t *testing.T, r doctor.Report, name string) doctor.Check {
	t.Helper()

	matches := checksFor(r, name)
	if len(matches) != 1 {
		t.Fatalf("checkNamed(%q): want exactly one match, got %d", name, len(matches))
	}

	return matches[0]
}

// checksFor returns every check whose name is exactly prefix or starts
// with "prefix (", such as the per-account checks starting "Account (".
func checksFor(r doctor.Report, prefix string) []doctor.Check {
	var out []doctor.Check
	for _, c := range r.Checks {
		if c.Name == prefix || len(c.Name) > len(prefix) && c.Name[:len(prefix)] == prefix {
			out = append(out, c)
		}
	}

	return out
}
