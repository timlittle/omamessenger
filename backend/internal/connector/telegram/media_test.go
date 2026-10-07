package telegram

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// stripped is a stripped thumbnail as Telegram sends it: a version byte,
// the height and width, then the JPEG body without its standard header.
var stripped = &tg.PhotoStrippedSize{Type: "i", Bytes: []byte{1, 40, 30, 0xAB, 0xCD}}

func TestMedia_LinkPreview(t *testing.T) {
	t.Parallel()

	page := &tg.WebPage{URL: "https://x.io/a", DisplayURL: "x.io/a"}
	page.SetSiteName("X")
	page.SetTitle("A page")
	page.SetDescription("About it")
	page.SetPhoto(&tg.Photo{Sizes: []tg.PhotoSizeClass{stripped}})

	got := media(&tg.MessageMediaWebPage{Webpage: page})
	if got == nil || got.Kind != domain.MediaLink || got.URL != "https://x.io/a" || got.SiteName != "X" ||
		got.Title != "A page" || got.Description != "About it" {
		t.Fatalf("media = %+v, want the link preview", got)
	}

	jpeg, err := base64.StdEncoding.DecodeString(got.Thumb)
	if err != nil || !bytes.HasPrefix(jpeg, []byte{0xFF, 0xD8}) {
		t.Errorf("thumb is not a JPEG: %v", err)
	}
}

func TestMedia_NothingToShow(t *testing.T) {
	t.Parallel()

	for name, m := range map[string]tg.MessageMediaClass{
		"no media":                               nil,
		"a preview not ready":                    &tg.MessageMediaWebPage{Webpage: &tg.WebPagePending{}},
		"a preview with no title or description": &tg.MessageMediaWebPage{Webpage: &tg.WebPage{URL: "https://x.io"}},
	} {
		if got := media(m); got != nil {
			t.Errorf("%s: media = %+v, want none", name, got)
		}
	}
}
