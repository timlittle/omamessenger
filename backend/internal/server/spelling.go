package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// spellCheckParams names the text to spell-check.
type spellCheckParams struct {
	Text string `json:"text"`
}

// spellMisspelling is one flagged word in a spell.check result, in the
// same UTF-16 offsets convention a mention's offset uses.
type spellMisspelling struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Word  string `json:"word"`
}

// spellCheckResult is spell.check's result.
type spellCheckResult struct {
	Misspellings []spellMisspelling `json:"misspellings"`
}

// spellCheck finds misspelled words in text.
func spellCheck(c *app.Commands) func(context.Context, spellCheckParams) (any, error) {
	return func(ctx context.Context, p spellCheckParams) (any, error) {
		found, err := c.CheckSpelling(ctx, p.Text)
		if err != nil {
			return nil, err
		}

		out := make([]spellMisspelling, len(found))
		for i, m := range found {
			out[i] = spellMisspelling{Start: m.Start, End: m.End, Word: m.Word}
		}

		return spellCheckResult{Misspellings: out}, nil
	}
}

// spellWordParams names a word, for spell.suggest and spell.addWord.
type spellWordParams struct {
	Word string `json:"word"`
}

// spellSuggestResult is spell.suggest's result.
type spellSuggestResult struct {
	Suggestions []string `json:"suggestions"`
}

// spellSuggest returns up to five replacement words for a misspelled
// word.
func spellSuggest(c *app.Commands) func(context.Context, spellWordParams) (any, error) {
	return func(ctx context.Context, p spellWordParams) (any, error) {
		suggestions, err := c.SuggestSpelling(ctx, p.Word)
		if err != nil {
			return nil, err
		}

		return spellSuggestResult{Suggestions: suggestions}, nil
	}
}

// spellAddWord adds a word to the user's personal dictionary, so it is
// never flagged again.
func spellAddWord(c *app.Commands) func(context.Context, spellWordParams) (any, error) {
	return func(ctx context.Context, p spellWordParams) (any, error) {
		return none{}, c.AddSpellingWord(ctx, p.Word)
	}
}
