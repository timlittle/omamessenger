package telegram

import (
	"encoding/base64"

	"github.com/gotd/td/telegram/thumbnail"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// media describes what a message carries besides its text, or nil when
// there is nothing to show.
func media(m tg.MessageMediaClass) *domain.Media {
	switch m := m.(type) {
	case *tg.MessageMediaWebPage:
		return linkPreview(m.Webpage)
	case *tg.MessageMediaPhoto:
		return photoMedia(m.Photo)
	default:
		return nil
	}
}

// linkPreview is a web page preview, or nil until Telegram has one worth
// showing.
func linkPreview(w tg.WebPageClass) *domain.Media {
	page, ok := w.(*tg.WebPage)
	if !ok {
		return nil
	}

	title, _ := page.GetTitle()
	description, _ := page.GetDescription()
	if title == "" && description == "" {
		return nil
	}

	site, _ := page.GetSiteName()
	out := &domain.Media{Kind: domain.MediaLink, URL: page.URL, SiteName: site, Title: title, Description: description}
	if photo, ok := page.GetPhoto(); ok {
		out.Thumb = strippedThumb(photo)
	}

	return out
}

// photoMedia is a photo at its largest size, with its blurred preview to
// show until it is downloaded, or nil for a photo since deleted.
func photoMedia(p tg.PhotoClass) *domain.Media {
	photo, ok := p.(*tg.Photo)
	if !ok {
		return nil
	}

	_, w, h := largest(photo.Sizes)

	return &domain.Media{Kind: domain.MediaPhoto, Width: w, Height: h, Thumb: strippedThumb(photo)}
}

// largest finds a photo's largest size: its type, which names it for
// download, and its width and height.
func largest(sizes []tg.PhotoSizeClass) (kind string, w, h int) {
	for _, size := range sizes {
		var sw, sh int
		switch s := size.(type) {
		case *tg.PhotoSize:
			sw, sh = s.W, s.H
		case *tg.PhotoSizeProgressive:
			sw, sh = s.W, s.H
		default:
			continue
		}

		if sw*sh > w*h {
			kind, w, h = size.GetType(), sw, sh
		}
	}

	return kind, w, h
}

// strippedThumb is a photo's tiny blurred preview as a base64 JPEG, or ""
// when Telegram sent none.
func strippedThumb(p tg.PhotoClass) string {
	photo, ok := p.(*tg.Photo)
	if !ok {
		return ""
	}

	for _, size := range photo.Sizes {
		s, ok := size.(*tg.PhotoStrippedSize)
		if !ok {
			continue
		}

		if jpeg, err := thumbnail.Expand(s.Bytes); err == nil {
			return base64.StdEncoding.EncodeToString(jpeg)
		}
	}

	return ""
}
