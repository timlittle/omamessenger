package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestStatusAdvances(t *testing.T) {
	statuses := []string{StatusPending, StatusSent, StatusDelivered, StatusRead, StatusFailed, StatusReceived}
	advances := map[[2]string]bool{
		{StatusPending, StatusSent}:      true,
		{StatusPending, StatusDelivered}: true,
		{StatusPending, StatusRead}:      true,
		{StatusPending, StatusFailed}:    true,
		{StatusSent, StatusDelivered}:    true,
		{StatusSent, StatusRead}:         true,
		{StatusSent, StatusFailed}:       true,
		{StatusDelivered, StatusRead}:    true,
		{StatusFailed, StatusPending}:    true,
		{StatusFailed, StatusSent}:       true,
	}

	for _, from := range statuses {
		for _, to := range statuses {
			t.Run(from+"_to_"+to, func(t *testing.T) {
				if got, want := StatusAdvances(from, to), advances[[2]string{from, to}]; got != want {
					t.Errorf("StatusAdvances(%q, %q) = %t, want %t", from, to, got, want)
				}
			})
		}
	}
	if StatusAdvances("unknown", StatusSent) || StatusAdvances(StatusSent, "unknown") {
		t.Fatal("unknown statuses must not advance")
	}
}

func TestNormalizeOutgoingText(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		want    string
		wantErr error
	}{
		{name: "trims surrounding whitespace", text: " \t hello world \n", want: "hello world"},
		{name: "rejects empty", text: " \t\n", wantErr: errEmptyText},
		{name: "accepts maximum length", text: strings.Repeat("a", maxMessageLength), want: strings.Repeat("a", maxMessageLength)},
		{name: "rejects over maximum", text: strings.Repeat("a", maxMessageLength+1), wantErr: errTextTooLong},
		{name: "counts unicode runes", text: strings.Repeat("界", maxMessageLength), want: strings.Repeat("界", maxMessageLength)},
		{name: "rejects multibyte over maximum", text: strings.Repeat("界", maxMessageLength+1), wantErr: errTextTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeOutgoingText(tt.text)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NormalizeOutgoingText() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("NormalizeOutgoingText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidService(t *testing.T) {
	tests := []struct {
		service string
		valid   bool
	}{
		{ServiceWhatsApp, true},
		{ServiceTelegram, true},
		{"", false},
		{"signal", false},
		{"WhatsApp", false},
	}
	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			if got := ValidService(tt.service); got != tt.valid {
				t.Errorf("ValidService(%q) = %t, want %t", tt.service, got, tt.valid)
			}
		})
	}
}
