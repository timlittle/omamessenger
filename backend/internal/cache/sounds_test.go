package cache_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

// TestSoundsPath_UsesTheWavExtension pins the one extension Store
// itself ever writes: a name no development build before this one,
// released or not, ever used for a sound store file (see
// docs/decisions.md).
func TestSoundsPath_UsesTheWavExtension(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(t.TempDir())

	if got, want := filepath.Ext(s.Path("conversation-1")), ".wav"; got != want {
		t.Errorf("Path() extension = %q, want %q", got, want)
	}
}

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

// TestSoundsSweep_RemovesAnOldStyleRawFile covers the actual security
// finding: an old development build once copied a user's raw chosen
// file straight into the sound store's own directory, under the same
// kind of fixed name Store itself used at the time (see
// docs/decisions.md). Sweep must remove anything in the directory that
// Store itself could not have written - here, simulated with the one
// name every build before this fix ever wrote.
func TestSoundsSweep_RemovesAnOldStyleRawFile(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "sounds")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(dir, "conversation-1.sound")
	if err := os.WriteFile(stray, []byte("the user's own unvalidated file, never transcoded"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := cache.NewSounds(dir)
	if err := s.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Errorf("an old-style raw file was kept: %v", err)
	}
}

// TestSoundsSweep_KeepsAStoredSound covers the other side: Sweep must
// never remove a sound actually stored through Store.
func TestSoundsSweep_KeepsAStoredSound(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(filepath.Join(t.TempDir(), "sounds"))
	path, err := s.Store(t.Context(), "conversation-1", strings.NewReader("sound bytes"))
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if err := s.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("a freshly stored sound was removed by Sweep(): %v", err)
	}
}

// TestSoundsSweep_ToleratesAnAreaNeverCreated confirms sweeping a
// sound store with nothing ever stored in it is not an error.
func TestSoundsSweep_ToleratesAnAreaNeverCreated(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(filepath.Join(t.TempDir(), "sounds"))
	if err := s.Sweep(t.Context()); err != nil {
		t.Errorf("Sweep() on a fresh area = %v, want nil", err)
	}
}

// TestSoundsSweep_RefusesACancelledContext confirms Sweep checks ctx
// before touching the filesystem.
func TestSoundsSweep_RefusesACancelledContext(t *testing.T) {
	t.Parallel()

	s := cache.NewSounds(filepath.Join(t.TempDir(), "sounds"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := s.Sweep(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Sweep(cancelled ctx) error = %v, want context.Canceled", err)
	}
}
