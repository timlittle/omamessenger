package whatsapp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector/whatsapp"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestProvider_ReportsItsServiceAndName(t *testing.T) {
	t.Parallel()

	p := whatsapp.Provider{}
	if p.Service() != domain.ServiceWhatsApp {
		t.Errorf("Service() = %q, want %q", p.Service(), domain.ServiceWhatsApp)
	}
	if p.Name() != "WhatsApp" {
		t.Errorf("Name() = %q, want WhatsApp", p.Name())
	}
}

func TestProvider_PrepareCreatesTheAccountDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "whatsapp")
	p := whatsapp.Provider{}
	if err := p.Prepare(dir, "wa-1", nil); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("Prepare did not create %s", dir)
	}
}

func TestProvider_ConnectReturnsAConnectorForTheAccount(t *testing.T) {
	t.Parallel()

	account := domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp}
	c := whatsapp.Provider{}.Connect(account, t.TempDir())

	if c.Account() != account {
		t.Errorf("Account() = %+v, want %+v", c.Account(), account)
	}
}

func TestProvider_ForgetDeletesTheSessionAndItsWALFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "wa-1.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(path+suffix, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	p := whatsapp.Provider{}
	if err := p.Forget(dir, "wa-1"); err != nil {
		t.Fatal(err)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Errorf("%s still exists after Forget", path+suffix)
		}
	}
}

func TestProvider_ForgetIsANoOpWithoutASession(t *testing.T) {
	t.Parallel()

	p := whatsapp.Provider{}
	if err := p.Forget(t.TempDir(), "wa-1"); err != nil {
		t.Errorf("Forget on a missing session = %v, want nil", err)
	}
}
