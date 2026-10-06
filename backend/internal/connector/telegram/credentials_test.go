package telegram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentials_RoundTripPrivately(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "telegram")
	want := Credentials{APIID: 12345, APIHash: "abc"}
	if err := SaveCredentials(dir, "tg-1", want); err != nil {
		t.Fatal(err)
	}

	got, err := loadCredentials(dir, "tg-1")
	if err != nil || got != want {
		t.Fatalf("loadCredentials = %+v, %v", got, err)
	}

	for path, mode := range map[string]os.FileMode{dir: 0o700, credentialsPath(dir, "tg-1"): 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Errorf("%s has mode %v (%v), want %v", path, info.Mode().Perm(), err, mode)
		}
	}
}

func TestForget_RemovesCredentialsAndSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := SaveCredentials(dir, "tg-1", Credentials{APIID: 1, APIHash: "x"}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(sessionPath(dir, "tg-1"), []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Forget(dir, "tg-1"); err != nil {
		t.Fatal(err)
	}

	if _, err := loadCredentials(dir, "tg-1"); err == nil {
		t.Error("credentials survived Forget")
	}

	if _, err := os.Stat(sessionPath(dir, "tg-1")); !os.IsNotExist(err) {
		t.Error("session survived Forget")
	}

	if err := Forget(dir, "tg-1"); err != nil {
		t.Errorf("forgetting twice = %v, want nil", err)
	}
}
