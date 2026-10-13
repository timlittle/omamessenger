package spell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/client9/gospell"
)

// This file is an internal test (package spell, not spell_test): Checker
// swaps in a tiny hand-written dictionary through its unexported loadDict
// field instead of the real ~27 MB embedded ones, which would make these
// tests slow and couple them to SCOWL's exact word list. The dictionary's
// own loading (dicts.go) is covered separately in dicts_test.go.

// testChecker returns a Checker over dataDir whose dictionary is a tiny
// three-word list ("hello", "world", "stop"), loaded once per locale no
// matter how many times it is asked for, so a test can tell a fresh load
// from a cached one.
func testChecker(t *testing.T, dataDir string) (*Checker, *int) {
	t.Helper()

	loads := 0
	c := New(dataDir, func(string) string { return "" })
	c.loadDict = func(_ string) (*gospell.GoSpell, error) {
		loads++
		return gospell.NewGoSpellReader(strings.NewReader(""), strings.NewReader("3\nhello\nworld\nstop\n"))
	}

	return c, &loads
}

// TestChecker_CheckWhenDisabled reports nothing and never loads the
// dictionary while checking is off.
func TestChecker_CheckWhenDisabled(t *testing.T) {
	t.Parallel()

	c, loads := testChecker(t, t.TempDir())

	got, err := c.Check(t.Context(), "wrold")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got != nil {
		t.Errorf("Check while disabled = %+v, want nil", got)
	}
	if *loads != 0 {
		t.Errorf("loads = %d, want 0 (dictionary must not load while disabled)", *loads)
	}
}

// TestChecker_CheckFindsMisspellings flags a word the dictionary does not
// have and leaves a correctly spelled one alone.
func TestChecker_CheckFindsMisspellings(t *testing.T) {
	t.Parallel()

	c, _ := testChecker(t, t.TempDir())
	c.SetEnabled(true)

	got, err := c.Check(t.Context(), "hello wrold")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	want := []Misspelling{{Start: 6, End: 11, Word: "wrold"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("Check(%q) = %+v, want %+v", "hello wrold", got, want)
	}
}

// TestChecker_LazyLoadAndUnload loads the dictionary only once enabled and
// asked for, keeps reusing it across calls, and drops it again once
// disabled.
func TestChecker_LazyLoadAndUnload(t *testing.T) {
	t.Parallel()

	c, loads := testChecker(t, t.TempDir())
	if *loads != 0 {
		t.Fatalf("loads = %d before enabling, want 0", *loads)
	}

	c.SetEnabled(true)
	if _, err := c.Check(t.Context(), "hello"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := c.Check(t.Context(), "world"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if *loads != 1 {
		t.Errorf("loads = %d after two checks, want 1 (cached)", *loads)
	}

	c.SetEnabled(false)
	c.SetEnabled(true)
	if _, err := c.Check(t.Context(), "hello"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if *loads != 2 {
		t.Errorf("loads = %d after re-enabling, want 2 (disabling released it)", *loads)
	}
}

// TestChecker_SetLanguageReloads keeps the loaded dictionary when the
// language setting does not actually change the chosen locale, and
// reloads it when it does.
func TestChecker_SetLanguageReloads(t *testing.T) {
	t.Parallel()

	c, loads := testChecker(t, t.TempDir())
	c.SetEnabled(true)

	if _, err := c.Check(t.Context(), "hello"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	c.SetLanguage("en_US")
	if _, err := c.Check(t.Context(), "hello"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if *loads != 1 {
		t.Errorf("loads = %d, want 1 (en_US was already the chosen locale)", *loads)
	}

	c.SetLanguage("en_GB")
	if _, err := c.Check(t.Context(), "hello"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if *loads != 2 {
		t.Errorf("loads = %d, want 2 (switching locale must reload)", *loads)
	}
}

// TestChecker_Suggest returns the dictionary's own suggestions for a
// misspelled word.
func TestChecker_Suggest(t *testing.T) {
	t.Parallel()

	c, _ := testChecker(t, t.TempDir())

	got, err := c.Suggest(t.Context(), "helo")
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if !slicesContain(got, "hello") {
		t.Errorf("Suggest(helo) = %v, want it to contain %q", got, "hello")
	}
}

// TestChecker_AddWordPersists adds a word to the dictionary, saves it to
// the personal word list with private permissions, and finds it again
// after the dictionary reloads.
func TestChecker_AddWordPersists(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	c, loads := testChecker(t, dataDir)
	c.SetEnabled(true)

	if err := c.AddWord(t.Context(), "quux"); err != nil {
		t.Fatalf("AddWord: %v", err)
	}

	got, err := c.Check(t.Context(), "quux")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got != nil {
		t.Errorf("Check(quux) after AddWord = %+v, want nil (no longer misspelled)", got)
	}

	path := filepath.Join(dataDir, "spelling", "en_US.txt")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat personal word list: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("personal word list mode = %o, want 0600", perm)
	}
	if dirInfo, err := os.Stat(filepath.Dir(path)); err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("personal word list directory mode = %v, want 0700 (err %v)", dirInfo, err)
	}

	if *loads != 1 {
		t.Errorf("loads = %d, want 1", *loads)
	}

	// A fresh Checker over the same data directory picks the word back up
	// from disk, without AddWord being called again.
	c2, loads2 := testChecker(t, dataDir)
	c2.SetEnabled(true)
	got2, err := c2.Check(t.Context(), "quux")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got2 != nil {
		t.Errorf("Check(quux) on a fresh Checker = %+v, want nil (personal word list must persist)", got2)
	}
	if *loads2 != 1 {
		t.Errorf("loads2 = %d, want 1", *loads2)
	}
}

// TestChecker_AddWordRejectsEmpty reports an error for an empty word
// rather than silently adding it.
func TestChecker_AddWordRejectsEmpty(t *testing.T) {
	t.Parallel()

	c, _ := testChecker(t, t.TempDir())

	if err := c.AddWord(t.Context(), ""); err == nil {
		t.Error("AddWord(\"\") = nil error, want one")
	}
}

// TestChecker_CheckRespectsCancelledContext returns the context's error
// instead of loading the dictionary.
func TestChecker_CheckRespectsCancelledContext(t *testing.T) {
	t.Parallel()

	c, loads := testChecker(t, t.TempDir())
	c.SetEnabled(true)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := c.Check(ctx, "hello"); err == nil {
		t.Error("Check with a cancelled context = nil error, want one")
	}
	if *loads != 0 {
		t.Errorf("loads = %d, want 0 (must not load after a cancelled context)", *loads)
	}
}

// slicesContain reports whether s contains want.
func slicesContain(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

// TestChecker_SuggestAndAddWord_LoadDictError report the dictionary
// loader's own error rather than panicking or swallowing it.
func TestChecker_SuggestAndAddWord_LoadDictError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	c := New(t.TempDir(), func(string) string { return "" })
	c.loadDict = func(string) (*gospell.GoSpell, error) { return nil, wantErr }

	if _, err := c.Suggest(t.Context(), "hello"); !errors.Is(err, wantErr) {
		t.Errorf("Suggest load error = %v, want %v", err, wantErr)
	}
	if err := c.AddWord(t.Context(), "hello"); !errors.Is(err, wantErr) {
		t.Errorf("AddWord load error = %v, want %v", err, wantErr)
	}
}

// TestChecker_Suggest_NoSuggesterConfigured reports gospell's own error
// rather than panicking when the loaded dictionary has no suggester.
func TestChecker_Suggest_NoSuggesterConfigured(t *testing.T) {
	t.Parallel()

	c := New(t.TempDir(), func(string) string { return "" })
	c.loadDict = func(string) (*gospell.GoSpell, error) {
		dict, err := gospell.NewGoSpellReader(strings.NewReader(""), strings.NewReader("1\nhello\n"))
		if err != nil {
			return nil, err
		}
		if err := dict.SetSuggester(nil); err != nil {
			return nil, err
		}
		return dict, nil
	}

	if _, err := c.Suggest(t.Context(), "helo"); err == nil {
		t.Error("Suggest with no suggester configured = nil error, want one")
	}
}

// TestChecker_Check_PersonalWordListLoadError reports a personal word
// list that cannot be read as an error, rather than silently checking
// against the dictionary alone.
func TestChecker_Check_PersonalWordListLoadError(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	c, _ := testChecker(t, dataDir)
	c.SetEnabled(true)

	// A directory where the personal word list file should be: os.Open
	// succeeds (it is a valid path), but reading it as a word list
	// fails, the way a permission problem or disk error would.
	personalPath := filepath.Join(dataDir, "spelling", "en_US.txt")
	if err := os.MkdirAll(personalPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Check(t.Context(), "hello"); err == nil {
		t.Error("Check with an unreadable personal word list = nil error, want one")
	}
}
