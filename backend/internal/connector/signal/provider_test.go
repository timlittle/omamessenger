package signal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector/signal"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestProvider_ServiceAndName checks the provider identifies itself
// as Signal.
func TestProvider_ServiceAndName(t *testing.T) {
	t.Parallel()

	p := signal.Provider{}
	if p.Service() != domain.ServiceSignal {
		t.Errorf("Service() = %q, want %q", p.Service(), domain.ServiceSignal)
	}
	if p.Name() != "Signal" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Signal")
	}
}

// TestProvider_Prepare checks that Prepare creates the account's
// service directory.
func TestProvider_Prepare(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "signal")
	if err := (signal.Provider{}).Prepare(dir, "acc1", nil); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("Prepare() did not create %q", dir)
	}
}

// TestProvider_Forget checks that Forget deletes the account's
// session files and sidecar, and tolerates them already being gone.
func TestProvider_Forget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, suffix := range []string{".db", ".db-wal", ".db-shm", ".aci.json"} {
		if err := os.WriteFile(filepath.Join(dir, "acc1"+suffix), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
	}

	if err := (signal.Provider{}).Forget(dir, "acc1"); err != nil {
		t.Fatalf("Forget() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Forget() left files behind: %v", entries)
	}

	if err := (signal.Provider{}).Forget(dir, "acc1"); err != nil {
		t.Errorf("Forget() on an already-forgotten account: %v", err)
	}
}

// TestProvider_Connect checks that Connect returns a usable connector
// for the account.
func TestProvider_Connect(t *testing.T) {
	t.Parallel()

	account := domain.Account{ID: "acc1", Service: domain.ServiceSignal}
	c := signal.Provider{}.Connect(account, t.TempDir())
	if c.Account() != account {
		t.Errorf("Account() = %+v, want %+v", c.Account(), account)
	}
}
