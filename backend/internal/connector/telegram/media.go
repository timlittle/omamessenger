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
	case *tg.MessageMediaDocument:
		return documentMedia(m.Document)
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
		if p, ok := photo.(*tg.Photo); ok {
			out.Thumb = strippedThumb(p.Sizes)
		}
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

	return &domain.Media{Kind: domain.MediaPhoto, Width: w, Height: h, Thumb: strippedThumb(photo.Sizes)}
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

// document is what a document's attributes say it is.
type document struct {
	name                       string
	video, voice, sticker      bool
	width, height, durationSec int
}

// describe reads a document's attributes.
func describe(doc *tg.Document) document {
	var d document
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeVideo:
			d.video, d.width, d.height, d.durationSec = true, a.W, a.H, int(a.Duration)
		case *tg.DocumentAttributeAudio:
			d.voice, d.durationSec = a.Voice, a.Duration
		case *tg.DocumentAttributeFilename:
			d.name = a.FileName
		case *tg.DocumentAttributeSticker:
			d.sticker = true
		}
	}

	return d
}

// documentMedia is a video, or any other document as a file to open. A
// sticker is left as its text label, and a document since deleted as
// nothing.
func documentMedia(dc tg.DocumentClass) *domain.Media {
	doc, ok := dc.(*tg.Document)
	if !ok {
		return nil
	}

	d := describe(doc)
	switch {
	case d.sticker:
		return nil
	case d.video:
		return &domain.Media{
			Kind: domain.MediaVideo, Width: d.width, Height: d.height, Duration: d.durationSec,
			FileName: d.name, Size: doc.Size, Thumb: strippedThumb(doc.Thumbs),
		}
	case d.voice:
		return &domain.Media{Kind: domain.MediaFile, FileName: "voice-message.ogg", Size: doc.Size, Duration: d.durationSec}
	case d.name == "":
		return &domain.Media{Kind: domain.MediaFile, FileName: "file", Size: doc.Size}
	default:
		return &domain.Media{Kind: domain.MediaFile, FileName: d.name, Size: doc.Size}
	}
}

// documentLabel names a document sent without a caption.
func documentLabel(dc tg.DocumentClass) string {
	doc, ok := dc.(*tg.Document)
	if !ok {
		return "[File]"
	}

	switch d := describe(doc); {
	case d.sticker:
		return "[Sticker]"
	case d.video:
		return "[Video]"
	case d.voice:
		return "[Voice message]"
	default:
		return "[File]"
	}
}

// strippedThumb is the tiny blurred preview among a photo's or document's
// sizes as a base64 JPEG, or "" when Telegram sent none.
func strippedThumb(sizes []tg.PhotoSizeClass) string {
	for _, size := range sizes {
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
