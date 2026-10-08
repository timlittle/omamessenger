package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// OmaMessenger's Telegram app, registered at my.telegram.org. Telegram
// identifies every client by these; they ship inside every helper, so
// they are not a secret, and every account uses them unless the user
// gives their own.
const (
	appID   = 38681077
	appHash = "0c880dcfe05b996d7594872255a1707d"
)

// AppCredentials returns OmaMessenger's own Telegram app keys.
func AppCredentials() Credentials {
	return Credentials{APIID: appID, APIHash: appHash}
}

// Credentials are the API id and hash an account signs in with, from
// my.telegram.org.
type Credentials struct {
	APIID   int    `json:"apiId"`
	APIHash string `json:"apiHash"`
}

// SaveCredentials writes an account's credentials into dir, readable only
// by the owner.
func SaveCredentials(dir, accountID string, c Credentials) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("telegram: save credentials: %w", err)
	}

	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("telegram: save credentials: %w", err)
	}

	if err := writeFileAtomically(credentialsPath(dir, accountID), data, 0o600); err != nil {
		return fmt.Errorf("telegram: save credentials: %w", err)
	}

	return nil
}

// loadCredentials reads an account's credentials from dir.
func loadCredentials(dir, accountID string) (Credentials, error) {
	var c Credentials
	data, err := os.ReadFile(credentialsPath(dir, accountID))
	if err != nil {
		return c, fmt.Errorf("telegram: load credentials: %w", err)
	}

	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("telegram: load credentials: %w", err)
	}

	return c, nil
}

// Forget deletes an account's credentials, session and saved update
// state from dir.
func Forget(dir, accountID string) error {
	var errs []error
	for _, path := range []string{credentialsPath(dir, accountID), sessionPath(dir, accountID), updateStatePath(dir, accountID)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// credentialsPath is where an account's credentials live.
func credentialsPath(dir, accountID string) string {
	return filepath.Join(dir, accountID+".json")
}

// sessionPath is where an account's Telegram session lives.
func sessionPath(dir, accountID string) string {
	return filepath.Join(dir, accountID+".session")
}
