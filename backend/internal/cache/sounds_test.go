package cache_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

func TestSoundsStore_CopiesAndFindsAgain(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "sounds")
	s := cache.NewSounds(dir)

	path, err := s.Store(t.Context(), "conversation-1", strings.NewReader("sound bytes"))
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if want := s.Path("conversation-1"); path != want {
		t.Errorf("Store() = %q, want %q", path, want)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "sound bytes" {
		t.Errorf("stored content = %q, %v, want %q", got, err, "sound bytes")
	}

	for p, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, %v; want %o", p, info.Mode().Perm(), err, want)
		}
	}
}

func TestSoundsStore_OverwritesTheSameKey(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(filepath.Join(t.TempDir(), "sounds"))

	if _, err := s.Store(t.Context(), "service-whatsapp", strings.NewReader("first")); err != nil {
		t.Fatalf("first Store() error = %v", err)
	}
	if _, err := s.Store(t.Context(), "service-whatsapp", strings.NewReader("second")); err != nil {
		t.Fatalf("second Store() error = %v", err)
	}

	got, err := os.ReadFile(s.Path("service-whatsapp"))
	if err != nil || string(got) != "second" {
		t.Errorf("stored content = %q, %v, want %q", got, err, "second")
	}
}

func TestSoundsRemove_IsNotAnErrorWhenNothingIsStored(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(filepath.Join(t.TempDir(), "sounds"))

	if err := s.Remove(t.Context(), "never-stored"); err != nil {
		t.Errorf("Remove() of a key never stored = %v, want nil", err)
	}
}

func TestSoundsRemove_DeletesTheStoredFile(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(filepath.Join(t.TempDir(), "sounds"))

	path, err := s.Store(t.Context(), "conversation-1", strings.NewReader("sound bytes"))
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if err := s.Remove(t.Context(), "conversation-1"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stat after Remove() = %v, want not exist", err)
	}
}

func TestSoundsStore_RefusesACancelledContext(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(filepath.Join(t.TempDir(), "sounds"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.Store(ctx, "conversation-1", strings.NewReader("bytes")); err == nil {
		t.Error("Store() with a cancelled context = nil error, want one")
	}
}
