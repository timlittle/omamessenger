package app_test

import (
	"os"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/app/policy"
)

// withDetail returns s with its notification detail set to detail, for a
// test that checks what a notification shows rather than the default,
// which shows no name or text.
func withDetail(s app.Settings, detail policy.Detail) app.Settings {
	s.NotificationDetail = string(detail)

	return s
}

// skipAsRoot skips a test that relies on file permissions refusing
// access: root bypasses them, so as root (as CI's container runs) the
// failure the test expects can never happen.
func skipAsRoot(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("file permissions do not restrict root")
	}
}
