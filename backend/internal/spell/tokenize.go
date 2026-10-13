package spell

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// urlPattern, emailPattern and codeSpanPattern mark whole ranges of text
// that candidates never spell-checks.
var (
	urlPattern      = regexp.MustCompile(`(?:https?://|www\.)\S+`)
	emailPattern    = regexp.MustCompile(`[[:alnum:]._%+\-]+@[[:alnum:].\-]+\.[[:alpha:]]{2,}`)
	codeSpanPattern = regexp.MustCompile("`[^`]*`")
)

// wordCandidate is one word Check considers spell-checking: its text,
// and its UTF-16 start/end offsets in the original message, the unit
// the helper protocol already uses for a mention's offset (see
// domain.Mention).
type wordCandidate struct {
	text  string
	start int
	end   int
}

// candidates finds the words in text worth spell-checking: runs of
// letters, after skipping a code span between backticks, a URL, an
// email address, an @mention, and any run that contains a digit (a
// number, a time, or a word like "COVID19" that would otherwise be
// flagged a letter at a time).
func candidates(text string) []wordCandidate {
	runes, byteStart, utf16Start := textOffsets(text)
	skip := skipRuneRanges(text, byteStart)

	var out []wordCandidate
	for i := 0; i < len(runes); {
		if !isWordRune(runes[i]) {
			i++
			continue
		}

		j, hasDigit := wordRuneRange(runes, i)
		if word, ok := checkableWord(runes, i, j, hasDigit, skip); ok {
			out = append(out, wordCandidate{text: word, start: utf16Start[i], end: utf16Start[j]})
		}

		i = j
	}

	return out
}

// textOffsets decodes text into runes, alongside each rune's byte and
// UTF-16 offset, so later steps can convert between the three: regexp
// matches bytes, a word range is found over runes, and the result
// reports UTF-16 units.
func textOffsets(text string) (runes []rune, byteStart, utf16Start []int) {
	runes = []rune(text)
	byteStart = make([]int, len(runes)+1)
	utf16Start = make([]int, len(runes)+1)

	b, u := 0, 0
	for i, r := range runes {
		byteStart[i], utf16Start[i] = b, u
		b += utf8.RuneLen(r)
		u += utf16.RuneLen(r)
	}
	byteStart[len(runes)], utf16Start[len(runes)] = b, u

	return runes, byteStart, utf16Start
}

// isWordRune reports whether r is part of a word cluster: a letter, a
// digit (so a run mixing them is found and skipped as a whole, rather
// than split into separately checked letters), or an apostrophe inside
// a contraction such as "don't".
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '’'
}

// wordRuneRange returns the end of the word cluster starting at rune
// index i, and whether any rune in it is a digit.
func wordRuneRange(runes []rune, i int) (end int, hasDigit bool) {
	j := i
	for j < len(runes) && isWordRune(runes[j]) {
		hasDigit = hasDigit || unicode.IsDigit(runes[j])
		j++
	}

	return j, hasDigit
}

// checkableWord reports the cluster runes[i:j] as a word worth spell
// checking, trimmed of its own leading or trailing apostrophes, unless
// it contains a digit, falls inside a skipped range, or is an
// @mention.
func checkableWord(runes []rune, i, j int, hasDigit bool, skip [][2]int) (string, bool) {
	if hasDigit || overlapsSkip(i, j, skip) || precededByAt(runes, i) {
		return "", false
	}

	word := strings.Trim(string(runes[i:j]), "'’")

	return word, word != ""
}

// precededByAt reports whether the word starting at rune index i is an
// @mention, immediately preceded by "@".
func precededByAt(runes []rune, i int) bool {
	return i > 0 && runes[i-1] == '@'
}

// skipRuneRanges finds the rune-index ranges candidates never spell
// checks: a code span, a URL and an email address, found with regexes
// against the original text and converted from byte to rune offsets
// using byteStart.
func skipRuneRanges(text string, byteStart []int) [][2]int {
	var ranges [][2]int
	for _, pattern := range []*regexp.Regexp{codeSpanPattern, urlPattern, emailPattern} {
		for _, m := range pattern.FindAllStringIndex(text, -1) {
			ranges = append(ranges, byteRangeToRunes(m, byteStart))
		}
	}

	return ranges
}

// byteRangeToRunes converts a [start,end) byte range, as regexp
// returns, to the matching rune index range using byteStart, the byte
// offset of each rune index that textOffsets built.
func byteRangeToRunes(byteRange []int, byteStart []int) [2]int {
	start := sort.SearchInts(byteStart, byteRange[0])
	end := sort.SearchInts(byteStart, byteRange[1])

	return [2]int{start, end}
}

// overlapsSkip reports whether the rune range [i,j) intersects any of
// skip's ranges.
func overlapsSkip(i, j int, skip [][2]int) bool {
	for _, r := range skip {
		if i < r[1] && j > r[0] {
			return true
		}
	}

	return false
}
