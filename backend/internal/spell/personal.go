package spell

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/client9/gospell"
)

// loadPersonalWords reads path, a personal word list of one word per
// line, and adds each word to dict. A missing file means no personal
// words have been added yet for this locale, not an error.
func loadPersonalWords(path string, dict *gospell.GoSpell) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("spell: open personal word list: %w", err)
	}
	defer func() { _ = f.Close() }() // reading to EOF below already surfaces any error through scanner.Err

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if word := strings.TrimSpace(scanner.Text()); word != "" {
			dict.AddWordRaw(word)
		}
	}

	return scanner.Err()
}

// appendPersonalWord adds word to the personal word list at path,
// creating its directory and the file itself with owner-only
// permissions if needed. The append is one write call under O_APPEND,
// which the kernel keeps atomic against another process's own append
// to the same file.
func appendPersonalWord(path, word string) error {
	if err := securePersonalDir(filepath.Dir(path)); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("spell: open personal word list: %w", err)
	}
	defer func() { _ = f.Close() }() // the write below already reports any error; an append-only fd has nothing buffered left to flush

	if _, err := f.WriteString(word + "\n"); err != nil {
		return fmt.Errorf("spell: append personal word: %w", err)
	}

	return nil
}

// securePersonalDir creates dir at 0700 if it does not exist yet,
// mirroring store.securePrivateDir for this package's own private
// directory.
func securePersonalDir(dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}

	if info.Mode().Perm()&0o077 == 0 {
		return nil
	}

	return os.Chmod(dir, 0o700) //nolint:gosec // deliberate: 0700 is a directory mode, not a file mode; gosec's G302 does not distinguish them
}
