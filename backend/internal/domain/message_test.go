package domain_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestStatusAdvances(t *testing.T) {
	t.Parallel()

	statuses := []string{
		domain.StatusPending, domain.StatusSent, domain.StatusDelivered,
		domain.StatusRead, domain.StatusFailed, domain.StatusReceived, "unknown",
	}
	advances := map[[2]string]bool{
		{domain.StatusPending, domain.StatusSent}:      true,
		{domain.StatusPending, domain.StatusDelivered}: true,
		{domain.StatusPending, domain.StatusRead}:      true,
		{domain.StatusPending, domain.StatusFailed}:    true,
		{domain.StatusSent, domain.StatusDelivered}:    true,
		{domain.StatusSent, domain.StatusRead}:         true,
		{domain.StatusSent, domain.StatusFailed}:       true,
		{domain.StatusDelivered, domain.StatusRead}:    true,
		{domain.StatusFailed, domain.StatusPending}:    true,
		{domain.StatusFailed, domain.StatusSent}:       true,
	}

	for _, from := range statuses {
		for _, to := range statuses {
			t.Run(from+"_to_"+to, func(t *testing.T) {
				t.Parallel()

				want := advances[[2]string{from, to}]
				if got := domain.StatusAdvances(from, to); got != want {
					t.Errorf("StatusAdvances(%q, %q) = %t, want %t", from, to, got, want)
				}
			})
		}
	}
}

func TestExcerpt(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", domain.ExcerptLength+10)
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "short text is unchanged", text: "hello", want: "hello"},
		{name: "keeps only the first line", text: "line one\nline two", want: "line one"},
		{name: "trims surrounding whitespace", text: "  hi there  \n", want: "hi there"},
		{name: "cuts long text and marks it", text: long, want: strings.Repeat("a", domain.ExcerptLength) + "…"},
		{name: "empty text stays empty", text: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := domain.Excerpt(tt.text); got != tt.want {
				t.Errorf("Excerpt(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// TestTruncateIncomingText_LeavesShortTextAlone covers text already
// within MaxIncomingTextLength: returned unchanged, with no suffix
// appended.
func TestTruncateIncomingText_LeavesShortTextAlone(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "hello", strings.Repeat("a", domain.MaxIncomingTextLength)} {
		if got := domain.TruncateIncomingText(text); got != text {
			t.Errorf("TruncateIncomingText(%d bytes) changed unchanged text", len(text))
		}
	}
}

// TestTruncateIncomingText_CutsLongTextOnARuneBoundary covers text over
// the cap: the result is at most MaxIncomingTextLength bytes, still
// valid UTF-8, ends with the truncation notice, and keeps as a prefix
// of the original text everything before that notice.
func TestTruncateIncomingText_CutsLongTextOnARuneBoundary(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", domain.MaxIncomingTextLength+1000)
	got := domain.TruncateIncomingText(long)

	const suffix = "\n\n[message truncated]"

	if len(got) > domain.MaxIncomingTextLength {
		t.Fatalf("len(got) = %d, want at most %d", len(got), domain.MaxIncomingTextLength)
	}
	if !utf8.ValidString(got) {
		t.Fatal("TruncateIncomingText produced invalid UTF-8")
	}
	if !strings.HasSuffix(got, suffix) {
		t.Fatalf("TruncateIncomingText(long) does not end with the truncation notice: %q", got[len(got)-40:])
	}

	prefix := strings.TrimSuffix(got, suffix)
	if !strings.HasPrefix(long, prefix) {
		t.Error("TruncateIncomingText's own prefix is not a prefix of the original text")
	}
}

// TestTruncateIncomingText_NeverSplitsAMultiByteRune covers text whose
// cut point would otherwise land inside a multi-byte rune: the cut
// backs up to a rune boundary rather than producing invalid UTF-8.
func TestTruncateIncomingText_NeverSplitsAMultiByteRune(t *testing.T) {
	t.Parallel()

	// A multi-byte rune repeated enough times to cross the cap, chosen
	// so the naive byte cap would land mid-rune almost regardless of
	// the exact cap and suffix length.
	long := strings.Repeat("€", domain.MaxIncomingTextLength)
	got := domain.TruncateIncomingText(long)

	if !utf8.ValidString(got) {
		t.Fatal("TruncateIncomingText split a multi-byte rune")
	}
}

func TestMediaPlaceholder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind string
		want string
	}{
		{domain.MediaPhoto, "[Photo]"},
		{domain.MediaVideo, "[Video]"},
		{domain.MediaFile, "[File]"},
		{domain.MediaVoice, "[Voice message]"},
		{domain.MediaAudio, "[Audio]"},
		{domain.MediaSticker, "[Sticker]"},
		{"unknown", "[File]"},
	}

	for _, tt := range tests {
		if got := domain.MediaPlaceholder(tt.kind); got != tt.want {
			t.Errorf("MediaPlaceholder(%q) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
