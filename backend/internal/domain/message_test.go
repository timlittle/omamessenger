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
