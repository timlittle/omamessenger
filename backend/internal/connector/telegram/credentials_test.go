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

// TestSaveCredentials_WriteIsAtomic confirms SaveCredentials never
// leaves its temporary sibling behind, and overwriting existing
// credentials still leaves a fully valid file, never a half-written one.
func TestSaveCredentials_WriteIsAtomic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := SaveCredentials(dir, "tg-1", Credentials{APIID: 1, APIHash: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials(dir, "tg-1", Credentials{APIID: 2, APIHash: "second"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(credentialsPath(dir, "tg-1") + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("a temporary file was left behind: %v", err)
	}

	got, err := loadCredentials(dir, "tg-1")
	want := Credentials{APIID: 2, APIHash: "second"}
	if err != nil || got != want {
		t.Fatalf("loadCredentials after overwriting = %+v, %v, want %+v", got, err, want)
	}
}

func TestForget_RemovesCredentialsSessionAndUpdateState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := SaveCredentials(dir, "tg-1", Credentials{APIID: 1, APIHash: "x"}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(sessionPath(dir, "tg-1"), []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(updateStatePath(dir, "tg-1"), []byte("{}"), 0o600); err != nil {
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

	if _, err := os.Stat(updateStatePath(dir, "tg-1")); !os.IsNotExist(err) {
		t.Error("update state survived Forget")
	}

	if err := Forget(dir, "tg-1"); err != nil {
		t.Errorf("forgetting twice = %v, want nil", err)
	}
}

func TestAppCredentials_AreComplete(t *testing.T) {
	t.Parallel()

	if c := AppCredentials(); c.APIID <= 0 || len(c.APIHash) != 32 {
		t.Errorf("AppCredentials = %+v, want an id and a 32-character hash", c)
	}
}
