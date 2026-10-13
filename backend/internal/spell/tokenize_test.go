package spell

import "testing"

// TestCandidates checks which words candidates picks out of a message to
// spell-check, and which it skips. This is an internal test: the skip
// rules (URLs, mentions, emails, numbers, code spans) are easiest to
// prove against candidates' own output, in UTF-16 units, rather than
// indirectly through Checker.Check, which would also need a loaded
// dictionary just to prove a range was never produced.
func TestCandidates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want []wordCandidate
	}{
		{"single word", "hello", []wordCandidate{{text: "hello", start: 0, end: 5}}},
		{
			"two words",
			"hello wrold",
			[]wordCandidate{{text: "hello", start: 0, end: 5}, {text: "wrold", start: 6, end: 11}},
		},
		{"skips a url", "see https://example.com/path for it", []wordCandidate{
			{text: "see", start: 0, end: 3}, {text: "for", start: 29, end: 32}, {text: "it", start: 33, end: 35},
		}},
		{"skips a www url with no scheme", "visit www.example.com now", []wordCandidate{
			{text: "visit", start: 0, end: 5}, {text: "now", start: 22, end: 25},
		}},
		{"skips an email address", "mail me at tim@example.com please", []wordCandidate{
			{text: "mail", start: 0, end: 4},
			{text: "me", start: 5, end: 7},
			{text: "at", start: 8, end: 10},
			{text: "please", start: 27, end: 33},
		}},
		{"skips an at-mention", "hey @someone how are you", []wordCandidate{
			{text: "hey", start: 0, end: 3}, {text: "how", start: 13, end: 16}, {text: "are", start: 17, end: 20}, {text: "you", start: 21, end: 24},
		}},
		{"skips a number", "call me on 12345 today", []wordCandidate{
			{text: "call", start: 0, end: 4}, {text: "me", start: 5, end: 7}, {text: "on", start: 8, end: 10}, {text: "today", start: 17, end: 22},
		}},
		{"skips a word mixed with digits", "see you at COVID19 clinic", []wordCandidate{
			{text: "see", start: 0, end: 3}, {text: "you", start: 4, end: 7}, {text: "at", start: 8, end: 10}, {text: "clinic", start: 19, end: 25},
		}},
		{"skips a code span", "run `go test ./...` now", []wordCandidate{
			{text: "run", start: 0, end: 3}, {text: "now", start: 20, end: 23},
		}},
		{"keeps a contraction", "don't stop", []wordCandidate{
			{text: "don't", start: 0, end: 5}, {text: "stop", start: 6, end: 10},
		}},
		{"counts an emoji as one UTF-16 surrogate pair", "😀 wrold", []wordCandidate{
			{text: "wrold", start: 3, end: 8},
		}},
		{"empty text", "", nil},
		{"no words", "... --- 123", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := candidates(c.text)
			if !equalCandidates(got, c.want) {
				t.Errorf("candidates(%q) = %+v, want %+v", c.text, got, c.want)
			}
		})
	}
}

// equalCandidates compares two candidate slices for the test above; a
// hand-written helper, since wordCandidate is unexported and this is the
// only place that needs to compare slices of it.
func equalCandidates(a, b []wordCandidate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
