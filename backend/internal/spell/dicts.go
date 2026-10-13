// Package spell checks English spelling against a bundled Hunspell
// dictionary (see dictdata/NOTICE.txt for where it comes from and its
// license), chosen by locale or an explicit setting, with a per-user
// personal word list layered on top. Checking stays off until a caller
// turns it on; the dictionary itself loads only on the first request
// after that, and is released again once checking is turned off.
package spell

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"

	"github.com/client9/gospell"
)

// maxDictBytes bounds how much a bundled dictionary may decompress to,
// so a corrupted embedded file cannot make gunzip read without limit.
// Both dictionaries decompress to well under 1 MiB.
const maxDictBytes = 8 << 20

//go:embed dictdata/en_GB.aff.gz
var enGBAffGz []byte

//go:embed dictdata/en_GB.dic.gz
var enGBDicGz []byte

//go:embed dictdata/en_US.aff.gz
var enUSAffGz []byte

//go:embed dictdata/en_US.dic.gz
var enUSDicGz []byte

// loadEmbeddedDict decompresses and parses the bundled Hunspell
// dictionary for locale, "en_GB" or "en_US".
func loadEmbeddedDict(locale string) (*gospell.GoSpell, error) {
	affGz, dicGz, ok := dictGzBytes(locale)
	if !ok {
		return nil, fmt.Errorf("spell: unknown locale %q", locale)
	}

	aff, err := gunzip(affGz)
	if err != nil {
		return nil, fmt.Errorf("spell: decompress %s affix file: %w", locale, err)
	}

	dic, err := gunzip(dicGz)
	if err != nil {
		return nil, fmt.Errorf("spell: decompress %s dictionary: %w", locale, err)
	}

	dict, err := gospell.NewGoSpellReader(bytes.NewReader(aff), bytes.NewReader(dic))
	if err != nil {
		return nil, fmt.Errorf("spell: parse %s dictionary: %w", locale, err)
	}

	return dict, nil
}

// dictGzBytes returns locale's gzip-compressed affix and dictionary
// files, embedded at build time.
func dictGzBytes(locale string) (affGz, dicGz []byte, ok bool) {
	switch locale {
	case "en_GB":
		return enGBAffGz, enGBDicGz, true
	case "en_US":
		return enUSAffGz, enUSDicGz, true
	default:
		return nil, nil, false
	}
}

// gunzip decompresses gzip-compressed data, up to maxDictBytes.
func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }() // reading to EOF below already surfaces a corrupt stream through its error

	return io.ReadAll(io.LimitReader(r, maxDictBytes))
}
