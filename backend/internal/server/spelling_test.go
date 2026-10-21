package server_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/server"
)

// TestSpellMethods_RoundTrip exercises spell.check, spell.suggest and
// spell.addWord together: a misspelling is found, a suggestion for it
// is offered, and adding it to the personal dictionary clears the mark.
func TestSpellMethods_RoundTrip(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	// Spell checking is on by default (see app.DefaultSettings), so no
	// settings.apply call is needed before checking.
	checked, err := call[struct {
		Misspellings []struct {
			Start int    `json:"start"`
			End   int    `json:"end"`
			Word  string `json:"word"`
		} `json:"misspellings"`
	}](t, s, "spell.check", map[string]string{"text": "this is wrongg"})
	if err != nil || len(checked.Misspellings) != 1 || checked.Misspellings[0].Word != "wrongg" {
		t.Fatalf("spell.check = %+v, %v", checked, err)
	}
	if checked.Misspellings[0].Start != 8 || checked.Misspellings[0].End != 14 {
		t.Errorf("spell.check offsets = %+v, want start 8 end 14", checked.Misspellings[0])
	}

	suggested, err := call[struct {
		Suggestions []string `json:"suggestions"`
	}](t, s, "spell.suggest", map[string]string{"word": "wrongg"})
	if err != nil {
		t.Fatalf("spell.suggest: %v", err)
	}
	if !slices.Contains(suggested.Suggestions, "wrong") {
		t.Errorf("spell.suggest(wrongg) = %v, want it to contain %q", suggested.Suggestions, "wrong")
	}

	if _, err := call[struct{}](t, s, "spell.addWord", map[string]string{"word": "wrongg"}); err != nil {
		t.Fatalf("spell.addWord: %v", err)
	}

	recheck, err := call[struct {
		Misspellings []struct {
			Start int    `json:"start"`
			End   int    `json:"end"`
			Word  string `json:"word"`
		} `json:"misspellings"`
	}](t, s, "spell.check", map[string]string{"text": "wrongg"})
	if err != nil || len(recheck.Misspellings) != 0 {
		t.Errorf("spell.check(wrongg) after spell.addWord = %+v, %v, want no misspellings", recheck, err)
	}
}

// TestSpellAddWord_RejectsEmptyWord reports invalid params rather than
// adding nothing.
func TestSpellAddWord_RejectsEmptyWord(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	_, err := call[struct{}](t, s, "spell.addWord", map[string]string{"word": ""})
	if code(err) != server.CodeInvalidParams {
		t.Errorf("spell.addWord(\"\") code = %v, want CodeInvalidParams", err)
	}
}
