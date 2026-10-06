package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

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

	return os.WriteFile(credentialsPath(dir, accountID), data, 0o600)
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

// Forget deletes an account's credentials and session from dir.
func Forget(dir, accountID string) error {
	var errs []error
	for _, path := range []string{credentialsPath(dir, accountID), sessionPath(dir, accountID)} {
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
