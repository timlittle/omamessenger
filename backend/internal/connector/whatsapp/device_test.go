package whatsapp

// openDevice and sessionPath are unexported, with no public way to open
// a real session database, so this test reaches into the package rather
// than through Connector's exported API. It uses a real SQLite database
// in a temporary directory; nothing here reaches WhatsApp's servers.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionPath_NamesTheAccountsDatabase(t *testing.T) {
	t.Parallel()

	got := sessionPath("/data/whatsapp", "wa-1")
	if want := filepath.Join("/data/whatsapp", "wa-1.db"); got != want {
		t.Errorf("sessionPath = %q, want %q", got, want)
	}
}

// Neither of the two tests below run in parallel with each other: both
// create a brand new WhatsApp device, and go.mau.fi/libsignal's package
// logger lazily initializes itself with no synchronization the first
// time anything signs with a key, which the race detector (rightly)
// flags when two goroutines hit that first use at once.

func TestOpenDevice_CreatesAPrivateFreshSession(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "whatsapp")
	dev, err := openDevice(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dev.close() }()

	if dev.isPaired() {
		t.Error("a freshly created device reports isPaired, want false")
	}

	info, err := os.Stat(sessionPath(dir, "wa-1"))
	if err != nil {
		t.Fatalf("session file was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file mode = %o, want 0600", perm)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("session directory was not created: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("session directory mode = %o, want 0700", perm)
	}
}

func TestOpenDevice_ReopensAnExistingSession(t *testing.T) {
	dir := t.TempDir()
	first, err := openDevice(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.close(); err != nil {
		t.Fatal(err)
	}

	second, err := openDevice(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatalf("reopening an existing session failed: %v", err)
	}
	defer func() { _ = second.close() }()

	if second.isPaired() {
		t.Error("a session that was never paired reports isPaired, want false")
	}
}

func TestOpenDevice_RejectsAnUnwritableDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "whatsapp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }() // restore so t.TempDir() can clean up

	if _, err := openDevice(t.Context(), dir, "wa-1"); err == nil {
		t.Error("openDevice in an unwritable directory = nil error, want one")
	}
}
