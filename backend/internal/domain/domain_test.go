package domain_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestValidService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		service string
		valid   bool
	}{
		{domain.ServiceWhatsApp, true},
		{domain.ServiceTelegram, true},
		{"", false},
		{"signal", false},
		{"WhatsApp", false},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			t.Parallel()

			if got := domain.ValidService(tt.service); got != tt.valid {
				t.Errorf("ValidService(%q) = %t, want %t", tt.service, got, tt.valid)
			}
		})
	}
}
