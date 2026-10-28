package cache_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
)

// TestCacheRevalidate_RemovesWhatCheckRejects covers Revalidate's own
// job on the downloaded media cache: every finished file in the
// directory is offered to check, and removed when it says keep=false,
// left alone otherwise.
func TestCacheRevalidate_RemovesWhatCheckRejects(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.jpg")
	drop := filepath.Join(dir, "drop.jpg")
	mustWriteFile(t, keep, "safe bytes")
	mustWriteFile(t, drop, "unsafe bytes")

	c := cache.New(dir, 1<<20)
	err := c.Revalidate(func(path string) (bool, error) {
		return filepath.Base(path) == "keep.jpg", nil
	})
	if err != nil {
		t.Fatalf("Revalidate() = %v, want nil", err)
	}

	if _, err := os.Stat(keep); err != nil {
		t.Errorf("keep.jpg was removed, want it left alone: %v", err)
	}
	if _, err := os.Stat(drop); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("drop.jpg stat = %v, want it removed", err)
	}
}

// TestCacheRevalidate_SkipsPartFilesAndAFreshDirectory covers the two
// cases Revalidate must never touch: a fill still in progress (a
// ".part" file), which check is never even asked about, and a cache
// directory not created yet at all, which is not an error.
func TestCacheRevalidate_SkipsPartFilesAndAFreshDirectory(t *testing.T) {
	t.Parallel()

	t.Run("part file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "m1.jpg.part"), "still filling")

		c := cache.New(dir, 1<<20)
		asked := false
		if err := c.Revalidate(func(string) (bool, error) { asked = true; return false, nil }); err != nil {
			t.Fatalf("Revalidate() = %v, want nil", err)
		}
		if asked {
			t.Error("check was asked about a .part file, want it skipped")
		}
		if _, err := os.Stat(filepath.Join(dir, "m1.jpg.part")); err != nil {
			t.Errorf(".part file was removed, want it left alone: %v", err)
		}
	})

	t.Run("directory never created", func(t *testing.T) {
		t.Parallel()

		c := cache.New(filepath.Join(t.TempDir(), "media"), 1<<20)
		if err := c.Revalidate(func(string) (bool, error) { return false, nil }); err != nil {
			t.Fatalf("Revalidate() = %v, want nil for a directory that does not exist yet", err)
		}
	})
}

// TestCacheRevalidate_ContinuesPastOneFileSFailure covers a check that
// fails to even read one file: the rest of the directory is still
// revalidated, and the failure is reported, not swallowed.
func TestCacheRevalidate_ContinuesPastOneFileSFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.jpg"), "a")
	mustWriteFile(t, filepath.Join(dir, "b.jpg"), "b")

	c := cache.New(dir, 1<<20)
	checked := map[string]bool{}
	err := c.Revalidate(func(path string) (bool, error) {
		checked[filepath.Base(path)] = true
		if filepath.Base(path) == "a.jpg" {
			return false, errors.New("boom")
		}
		return true, nil
	})
	if err == nil {
		t.Fatal("Revalidate() = nil, want the one file's failure reported")
	}
	if !checked["a.jpg"] || !checked["b.jpg"] {
		t.Errorf("checked = %v, want both files offered to check", checked)
	}
}

// TestCacheRevalidate_RemovesAFileItsCheckCouldNotEvenRead covers the
// bug this fixes: a check reporting an error - it could not even read
// the file - must still have Revalidate remove it, never leave it on
// disk just because it was never actually checked (see
// RevalidateCheck).
func TestCacheRevalidate_RemovesAFileItsCheckCouldNotEvenRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.jpg")
	mustWriteFile(t, path, "a")

	c := cache.New(dir, 1<<20)
	err := c.Revalidate(func(string) (bool, error) {
		return false, errors.New("could not read it")
	})
	if err == nil {
		t.Fatal("Revalidate() = nil, want the check's own failure reported")
	}

	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("unreadable file stat = %v, want it removed", statErr)
	}
}

// TestOutgoingRevalidate_RemovesWhatCheckRejects covers Revalidate's
// own job on the outgoing attachment area: the same contract Cache's
// own Revalidate has, over Outgoing's directory instead.
func TestOutgoingRevalidate_RemovesWhatCheckRejects(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)
	keep := mustStore(t, o, "m1", "keep.jpg", "safe bytes")
	drop := mustStore(t, o, "m2", "drop.jpg", "unsafe bytes")

	err := o.Revalidate(func(path string) (bool, error) {
		return filepath.Base(path) == "m1-keep.jpg", nil
	})
	if err != nil {
		t.Fatalf("Revalidate() = %v, want nil", err)
	}

	if _, err := os.Stat(keep); err != nil {
		t.Errorf("kept attachment was removed, want it left alone: %v", err)
	}
	if _, err := os.Stat(drop); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dropped attachment stat = %v, want it removed", err)
	}
}

// TestOutgoingFormatVersion_RoundTrips covers Outgoing's own
// FormatVersion and SetFormatVersion, the same contract Cache's own
// pair has.
func TestOutgoingFormatVersion_RoundTrips(t *testing.T) {
	t.Parallel()

	o := newOutgoing(t)

	if v, err := o.FormatVersion(); err != nil || v != 0 {
		t.Fatalf("FormatVersion() = %d, %v, want 0, nil before anything is recorded", v, err)
	}

	if err := o.SetFormatVersion(2); err != nil {
		t.Fatalf("SetFormatVersion() = %v, want nil", err)
	}
	if v, err := o.FormatVersion(); err != nil || v != 2 {
		t.Fatalf("FormatVersion() = %d, %v, want 2, nil", v, err)
	}
}

// TestCacheFormatVersion_RoundTrips covers SetFormatVersion followed
// by FormatVersion reading the same value back, and a directory with
// no marker yet reading back as version 0 - never itself a real
// version, so it always compares as older than any version this
// helper will ever record.
func TestCacheFormatVersion_RoundTrips(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := cache.New(dir, 1<<20)

	if v, err := c.FormatVersion(); err != nil || v != 0 {
		t.Fatalf("FormatVersion() = %d, %v, want 0, nil before anything is recorded", v, err)
	}

	if err := c.SetFormatVersion(3); err != nil {
		t.Fatalf("SetFormatVersion() = %v, want nil", err)
	}
	if v, err := c.FormatVersion(); err != nil || v != 3 {
		t.Fatalf("FormatVersion() = %d, %v, want 3, nil", v, err)
	}
}
