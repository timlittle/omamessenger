package spell

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/client9/gospell"
)

// suggestLimit is how many replacement words Suggest returns.
const suggestLimit = 5

// Misspelling is one flagged word in a Check result. Start and End are
// UTF-16 code unit offsets into the checked text, the unit JavaScript
// strings already index by and the convention domain.Mention uses.
type Misspelling struct {
	Start int
	End   int
	Word  string
}

// Checker checks English spelling with a dictionary loaded lazily on
// first use, and a personal word list saved under its data directory.
// Only one dictionary (English, GB or US) is ever loaded at a time; see
// SetLanguage.
type Checker struct {
	mu sync.Mutex

	dataDir   string
	lookupEnv func(string) string
	loadDict  func(locale string) (*gospell.GoSpell, error)

	enabled  bool
	language string

	loadedLocale string
	dict         *gospell.GoSpell
}

// New creates a Checker that saves its personal word list under
// dataDir and reads the locale environment through lookupEnv (os.Getenv
// in production). Checking starts disabled and the language setting
// starts at "auto"; call SetEnabled and SetLanguage to change them.
func New(dataDir string, lookupEnv func(string) string) *Checker {
	return &Checker{
		dataDir:   dataDir,
		lookupEnv: lookupEnv,
		loadDict:  loadEmbeddedDict,
		language:  "auto",
	}
}

// SetEnabled turns spell checking on or off. Turning it off releases
// the loaded dictionary, the bulk of the checker's memory, until the
// next request turns checking on again.
func (c *Checker) SetEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enabled = enabled
	if !enabled {
		c.unloadLocked()
	}
}

// SetLanguage sets which dictionary to use: "auto" picks one from the
// locale environment, "en_GB" or "en_US" names one explicitly. Changing
// it away from whichever locale is currently loaded releases that
// dictionary; the next request loads the right one.
func (c *Checker) SetLanguage(language string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.language = language
	if chooseLocale(language, c.lookupEnv) != c.loadedLocale {
		c.unloadLocked()
	}
}

// unloadLocked drops the loaded dictionary. c.mu must already be held.
func (c *Checker) unloadLocked() {
	c.dict, c.loadedLocale = nil, ""
}

// Check finds misspelled words in text, applying the skip rules in
// tokenize.go (URLs, @mentions, emails, numbers, code spans). It
// returns nil without loading the dictionary while checking is
// disabled.
func (c *Checker) Check(ctx context.Context, text string) ([]Misspelling, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.enabled {
		return nil, nil
	}

	dict, err := c.loadLocked(ctx)
	if err != nil {
		return nil, err
	}

	var found []Misspelling
	for _, word := range candidates(text) {
		if !dict.Spell(word.text) {
			found = append(found, Misspelling{Start: word.start, End: word.end, Word: word.text})
		}
	}

	return found, nil
}

// Suggest returns up to five replacement words for word, best match
// first. It loads the dictionary if needed regardless of SetEnabled,
// since a caller only asks for suggestions after Check already found a
// misspelling.
func (c *Checker) Suggest(ctx context.Context, word string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	dict, err := c.loadLocked(ctx)
	if err != nil {
		return nil, err
	}

	matches, err := dict.Suggest(word, suggestLimit)
	if err != nil {
		return nil, fmt.Errorf("spell: suggest: %w", err)
	}

	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.Word
	}

	return out, nil
}

// AddWord adds word to the loaded dictionary and appends it to the
// personal word list on disk, so it is recognised again the next time
// the dictionary loads.
func (c *Checker) AddWord(ctx context.Context, word string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if word == "" {
		return fmt.Errorf("spell: word is required")
	}

	dict, err := c.loadLocked(ctx)
	if err != nil {
		return err
	}

	dict.AddWordRaw(word)

	return appendPersonalWord(c.personalPath(c.loadedLocale), word)
}

// loadLocked loads the dictionary for the current language setting if
// it is not already loaded, merging in the personal word list. c.mu
// must already be held.
func (c *Checker) loadLocked(ctx context.Context) (*gospell.GoSpell, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	locale := chooseLocale(c.language, c.lookupEnv)
	if c.dict != nil && c.loadedLocale == locale {
		return c.dict, nil
	}

	dict, err := c.loadDict(locale)
	if err != nil {
		return nil, err
	}

	if err := loadPersonalWords(c.personalPath(locale), dict); err != nil {
		return nil, err
	}

	c.dict, c.loadedLocale = dict, locale

	return c.dict, nil
}

// personalPath returns where locale's personal word list lives on
// disk.
func (c *Checker) personalPath(locale string) string {
	return filepath.Join(c.dataDir, "spelling", locale+".txt")
}
