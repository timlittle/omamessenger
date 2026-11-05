package signal

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
)

// TestSessionStore_LinkedACI_RejectsGarbage checks that a sidecar
// file corrupted or written by something else is treated as nothing
// linked, rather than returning a garbage identifier.
func TestSessionStore_LinkedACI_RejectsGarbage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	session, err := openSession(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatalf("openSession() error = %v", err)
	}
	defer func() { _ = session.close() }()

	if err := os.WriteFile(aciSidecarPath(dir, "acc1"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write garbage sidecar: %v", err)
	}

	if _, ok := session.linkedACI(); ok {
		t.Error("linkedACI() succeeded reading a corrupted sidecar")
	}
}

// TestSessionStore_DeviceByPNI checks the PNI lookup delegates to the
// underlying container, reporting nil, nil for a device never linked
// under that identity.
func TestSessionStore_DeviceByPNI(t *testing.T) {
	t.Parallel()

	session, err := openSession(t.Context(), t.TempDir(), "acc1")
	if err != nil {
		t.Fatalf("openSession() error = %v", err)
	}
	defer func() { _ = session.close() }()

	dev, err := session.DeviceByPNI(t.Context(), uuid.New())
	if err != nil || dev != nil {
		t.Errorf("DeviceByPNI() = %v, %v, want nil, nil", dev, err)
	}
}

// TestOpenSession_CreatesPrivateFile checks that the session database
// and its directory are created with owner-only permissions, since
// SQLite would otherwise create the file world-readable.
func TestOpenSession_CreatesPrivateFile(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on windows")
	}

	dir := filepath.Join(t.TempDir(), "signal")
	session, err := openSession(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatalf("openSession() error = %v", err)
	}
	defer func() { _ = session.close() }()

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", dirInfo.Mode().Perm())
	}

	fileInfo, err := os.Stat(sessionPath(dir, "acc1"))
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fileInfo.Mode().Perm())
	}
}

// TestSessionStore_LinkedACIRoundTrips checks that an account's linked
// identifier survives being saved and read back, and that nothing has
// been linked yet reports found false.
func TestSessionStore_LinkedACIRoundTrips(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	session, err := openSession(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatalf("openSession() error = %v", err)
	}
	defer func() { _ = session.close() }()

	if _, ok := session.linkedACI(); ok {
		t.Error("linkedACI() found an account before any link")
	}

	want := uuid.New()
	if err := session.saveLinkedACI(want); err != nil {
		t.Fatalf("saveLinkedACI() error = %v", err)
	}

	got, ok := session.linkedACI()
	if !ok || got != want {
		t.Errorf("linkedACI() = %v, %t, want %v, true", got, ok, want)
	}
}

// TestAciSidecarPath_IsPrivate checks that the sidecar file saved by
// saveLinkedACI is owner-only, since it names which account this
// computer is linked as.
func TestAciSidecarPath_IsPrivate(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on windows")
	}

	dir := t.TempDir()
	session, err := openSession(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatalf("openSession() error = %v", err)
	}
	defer func() { _ = session.close() }()

	if err := session.saveLinkedACI(uuid.New()); err != nil {
		t.Fatalf("saveLinkedACI() error = %v", err)
	}

	info, err := os.Stat(aciSidecarPath(dir, "acc1"))
	if err != nil {
		t.Fatalf("stat sidecar: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("sidecar mode = %v, want 0600", info.Mode().Perm())
	}
}
