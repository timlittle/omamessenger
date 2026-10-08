package domain_test

import (
	"errors"
	"strings"
	"testing"

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

func TestNormalizeOutgoingText(t *testing.T) {
	t.Parallel()

	maxASCII := strings.Repeat("a", domain.MaxTextLength)
	maxWide := strings.Repeat("界", domain.MaxTextLength)
	tests := []struct {
		name    string
		text    string
		want    string
		wantErr error
	}{
		{name: "trims surrounding whitespace", text: " \t hello world \n", want: "hello world"},
		{name: "rejects empty", text: " \t\n", wantErr: domain.ErrEmptyText},
		{name: "accepts maximum length", text: maxASCII, want: maxASCII},
		{name: "rejects over maximum", text: maxASCII + "a", wantErr: domain.ErrTextTooLong},
		{name: "counts characters not bytes", text: maxWide, want: maxWide},
		{name: "rejects wide text over maximum", text: maxWide + "界", wantErr: domain.ErrTextTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NormalizeOutgoingText(tt.text)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NormalizeOutgoingText() error = %v, want %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("NormalizeOutgoingText() = %q, want %q", got, tt.want)
			}
		})
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
		{domain.MediaSticker, "[Sticker]"},
		{"unknown", "[File]"},
	}

	for _, tt := range tests {
		if got := domain.MediaPlaceholder(tt.kind); got != tt.want {
			t.Errorf("MediaPlaceholder(%q) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
