package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Provider implements connector.Provider for Telegram accounts.
type Provider struct {
	// Version is this helper's own release, carried to every connector
	// Connect returns so it can tell Telegram which build is running
	// (see identity.go's deviceConfig).
	Version string
}

var _ connector.Provider = Provider{}

// Service reports the service id Provider handles.
func (Provider) Service() string { return domain.ServiceTelegram }

// Name is the human-readable name the UI shows for Telegram.
func (Provider) Name() string { return "Telegram" }

// Prepare saves an account's API credentials: the ones given in options,
// under "apiId" and "apiHash", or OmaMessenger's own when neither is
// given. Giving only one of the two is rejected.
func (Provider) Prepare(dir, accountID string, options map[string]string) error {
	creds, err := credentialsFrom(options)
	if err != nil {
		return err
	}

	return SaveCredentials(dir, accountID, creds)
}

// Connect returns the Telegram connector for the account, reading what
// Prepare saved from dir.
func (p Provider) Connect(account domain.Account, dir string) connector.Connector {
	c := New(account, dir)
	c.version = p.Version

	return c
}

// Forget deletes the account's credentials and session from dir.
func (Provider) Forget(dir, accountID string) error {
	return Forget(dir, accountID)
}

// credentialsFrom reads an API id and hash out of options, falling back
// to OmaMessenger's own keys when neither is given.
func credentialsFrom(options map[string]string) (Credentials, error) {
	rawID, rawHash := options["apiId"], options["apiHash"]
	if rawID == "" && strings.TrimSpace(rawHash) == "" {
		return AppCredentials(), nil
	}

	id, err := strconv.Atoi(rawID)
	if err != nil || id <= 0 || strings.TrimSpace(rawHash) == "" {
		return Credentials{}, fmt.Errorf("telegram: %w: enter both the API id and the API hash from my.telegram.org", connector.ErrInvalidSetup)
	}

	return Credentials{APIID: id, APIHash: rawHash}, nil
}
