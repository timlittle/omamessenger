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

func TestMedia_IsGIFFile(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		media *domain.Media
		want  bool
	}{
		"a genuine gif image, marked animated with a .gif name": {
			&domain.Media{Kind: domain.MediaVideo, Animated: true, FileName: "run.gif"}, true,
		},
		"a gif name is matched case-insensitively": {
			&domain.Media{Kind: domain.MediaVideo, Animated: true, FileName: "RUN.GIF"}, true,
		},
		"a silent, looping mp4 the service marked animated is not a gif file": {
			&domain.Media{Kind: domain.MediaVideo, Animated: true, FileName: "clip.gif.mp4"}, false,
		},
		"an ordinary video, even named .gif, is never animated": {
			&domain.Media{Kind: domain.MediaVideo, Animated: false, FileName: "run.gif"}, false,
		},
		"a photo is never a gif file, whatever its name": {
			&domain.Media{Kind: domain.MediaPhoto, Animated: true, FileName: "run.gif"}, false,
		},
		"nil media": {nil, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tt.media.IsGIFFile(); got != tt.want {
				t.Errorf("IsGIFFile() = %t, want %t", got, tt.want)
			}
		})
	}
}
