package telegram_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/telegram"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestProvider_ServiceAndName(t *testing.T) {
	t.Parallel()

	p := telegram.Provider{}
	if p.Service() != domain.ServiceTelegram || p.Name() != "Telegram" {
		t.Errorf("Service() = %q, Name() = %q", p.Service(), p.Name())
	}
}

func TestProvider_PrepareUsesOurAppKeysUnlessGivenOthers(t *testing.T) {
	t.Parallel()

	p := telegram.Provider{}
	dir := t.TempDir()

	if err := p.Prepare(dir, "tg-1", nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "tg-1.json"))
	if err != nil {
		t.Fatal(err)
	}

	var saved telegram.Credentials
	if err := json.Unmarshal(data, &saved); err != nil || saved != telegram.AppCredentials() {
		t.Errorf("saved %+v, %v; want OmaMessenger's own keys", saved, err)
	}
}

func TestProvider_PrepareSavesTheGivenKeys(t *testing.T) {
	t.Parallel()

	p := telegram.Provider{}
	dir := t.TempDir()

	if err := p.Prepare(dir, "tg-1", map[string]string{"apiId": "777", "apiHash": "own"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "tg-1.json"))
	if err != nil {
		t.Fatal(err)
	}

	var saved telegram.Credentials
	if err := json.Unmarshal(data, &saved); err != nil || saved != (telegram.Credentials{APIID: 777, APIHash: "own"}) {
		t.Errorf("saved %+v, %v; want the given keys", saved, err)
	}
}

func TestProvider_PrepareRejectsAHalfGivenKeyPair(t *testing.T) {
	t.Parallel()

	p := telegram.Provider{}
	dir := t.TempDir()

	for name, options := range map[string]map[string]string{
		"no hash": {"apiId": "777"},
		"no id":   {"apiHash": "own"},
		"bad id":  {"apiId": "x", "apiHash": "own"},
	} {
		if err := p.Prepare(dir, "tg-1", options); !errors.Is(err, connector.ErrInvalidSetup) {
			t.Errorf("%s: Prepare = %v, want connector.ErrInvalidSetup", name, err)
		}
	}
}

func TestProvider_ConnectReturnsATelegramConnector(t *testing.T) {
	t.Parallel()

	p := telegram.Provider{}
	account := domain.Account{ID: "tg-1", Service: domain.ServiceTelegram}

	c := p.Connect(account, t.TempDir())
	if c.Account() != account {
		t.Errorf("Connect().Account() = %+v, want %+v", c.Account(), account)
	}
}

func TestProvider_ForgetDeletesWhatPrepareSaved(t *testing.T) {
	t.Parallel()

	p := telegram.Provider{}
	dir := t.TempDir()
	if err := p.Prepare(dir, "tg-1", nil); err != nil {
		t.Fatal(err)
	}

	if err := p.Forget(dir, "tg-1"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "tg-1.json")); !os.IsNotExist(err) {
		t.Errorf("credentials survived Forget: %v", err)
	}
}
