package whatsapp

import (
	"errors"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Provider implements connector.Provider for WhatsApp accounts.
type Provider struct{}

var _ connector.Provider = Provider{}

// Service reports the service id Provider handles.
func (Provider) Service() string { return domain.ServiceWhatsApp }

// Name is the human-readable name the UI shows for WhatsApp.
func (Provider) Name() string { return "WhatsApp" }

// Prepare makes sure dir exists for a new account. WhatsApp needs no
// setup values from the user: pairing happens entirely over the
// connection, unlike Telegram's optional API id and hash.
func (Provider) Prepare(dir, _ string, _ map[string]string) error {
	return os.MkdirAll(dir, 0o700)
}

// Connect returns the WhatsApp connector for the account, reading what
// Prepare saved from dir.
func (Provider) Connect(account domain.Account, dir string) connector.Connector {
	return New(account, dir)
}

// Forget deletes the account's session database, including its
// write-ahead log and shared-memory files.
func (Provider) Forget(dir, accountID string) error {
	return removeSessionFiles(sessionPath(dir, accountID))
}

// removeSessionFiles deletes a session database and the extra files
// WAL mode leaves beside it, ignoring whichever of them do not exist.
func removeSessionFiles(path string) error {
	var errs []error
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
