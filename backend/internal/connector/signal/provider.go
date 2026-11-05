package signal

import (
	"errors"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Provider implements connector.Provider for Signal accounts.
type Provider struct{}

var _ connector.Provider = Provider{}

// Service reports the service id Provider handles.
func (Provider) Service() string { return domain.ServiceSignal }

// Name is the human-readable name the UI shows for Signal.
func (Provider) Name() string { return "Signal" }

// Prepare makes sure dir exists for a new account. Signal needs no
// setup values from the user: linking happens entirely by scanning a
// QR code.
func (Provider) Prepare(dir, _ string, _ map[string]string) error {
	return os.MkdirAll(dir, 0o700)
}

// Connect returns the Signal connector for the account, reading what
// Prepare saved from dir.
func (Provider) Connect(account domain.Account, dir string) connector.Connector {
	return New(account, dir)
}

// Forget deletes the account's session database, including its
// write-ahead log and shared-memory files, and the sidecar file
// remembering which account it linked as.
func (Provider) Forget(dir, accountID string) error {
	return errors.Join(
		removeIfExists(sessionPath(dir, accountID)),
		removeIfExists(sessionPath(dir, accountID)+"-wal"),
		removeIfExists(sessionPath(dir, accountID)+"-shm"),
		removeIfExists(aciSidecarPath(dir, accountID)),
	)
}

// removeIfExists deletes path, treating it already being gone as
// success.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
