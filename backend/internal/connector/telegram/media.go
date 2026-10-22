package telegram

import (
	"encoding/base64"
	"strings"

	"github.com/gotd/td/telegram/thumbnail"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
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
	case *tg.MessageMediaPoll:
		return pollMedia(m)
	default:
		return nil
	}
}

// linkPreview is a web page preview, or nil until Telegram has one worth
// showing. Its URL is dropped, never reported, when domain.AllowedLinkURL
// rejects its scheme: Telegram resolves this itself, so it should never
// carry anything else, but the UI opens it with no filter of its own, so
// this connector never forwards a URL unchecked.
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

	url := page.URL
	if !domain.AllowedLinkURL(url) {
		url = ""
	}

	site, _ := page.GetSiteName()
	out := &domain.Media{Kind: domain.MediaLink, URL: url, SiteName: site, Title: title, Description: description}
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

	_, w, h, size := largest(photo.Sizes)

	return &domain.Media{Kind: domain.MediaPhoto, Width: w, Height: h, Size: size, Thumb: strippedThumb(photo.Sizes)}
}

// largest finds a photo's largest size: its type, which names it for
// download, its width and height, and its file size in bytes. A
// progressive JPEG reports its size as the last, full entry of its
// prefix sizes rather than a single Size field.
func largest(sizes []tg.PhotoSizeClass) (kind string, w, h int, size int64) {
	for _, s := range sizes {
		var sw, sh int
		var sz int64
		switch v := s.(type) {
		case *tg.PhotoSize:
			sw, sh, sz = v.W, v.H, int64(v.Size)
		case *tg.PhotoSizeProgressive:
			sw, sh = v.W, v.H
			if n := len(v.Sizes); n > 0 {
				sz = int64(v.Sizes[n-1])
			}
		default:
			continue
		}

		if sw*sh > w*h {
			kind, w, h, size = s.GetType(), sw, sh, sz
		}
	}

	return kind, w, h, size
}

// document is what a document's attributes say it is.
type document struct {
	name                       string
	video, voice, sticker      bool
	animated                   bool
	emoji                      string
	width, height, durationSec int
}

// describe reads a document's attributes. DocumentAttributeAnimated marks
// a video Telegram itself treats as a silent, looping "GIF" rather than
// an ordinary clip.
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
			d.sticker, d.emoji = true, a.Alt
		case *tg.DocumentAttributeAnimated:
			d.animated = true
		}
	}

	return d
}

// documentMedia is a video, a sticker, or any other document as a file
// to open, or nil for a document since deleted.
func documentMedia(dc tg.DocumentClass) *domain.Media {
	doc, ok := dc.(*tg.Document)
	if !ok {
		return nil
	}

	d := describe(doc)
	switch {
	case d.sticker:
		return stickerMedia(doc, d)
	case d.video:
		return &domain.Media{
			Kind: domain.MediaVideo, Width: d.width, Height: d.height, Duration: d.durationSec,
			FileName: d.name, Size: doc.Size, Thumb: strippedThumb(doc.Thumbs), Animated: d.animated,
		}
	case isGIFImage(doc.MimeType, d.name):
		// A real GIF image, sent as a plain document rather than
		// through Telegram's "send as animation" picker, carries none
		// of DocumentAttributeVideo or DocumentAttributeAnimated: only
		// its mime type or name says what it is. It still reaches the
		// UI as a video kind marked animated, exactly like a service's
		// own GIF playback, so it plays inline the same way; this is
		// only ever a classification hint, though, never what decides
		// the UI's decoder - see app.DecodeAsForMedia, which checks
		// the downloaded bytes themselves.
		return &domain.Media{Kind: domain.MediaVideo, FileName: gifFileName(d.name), Size: doc.Size, Animated: true}
	case d.voice:
		return &domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg", Size: doc.Size, Duration: d.durationSec}
	case d.name == "":
		return &domain.Media{Kind: domain.MediaFile, FileName: "file", Size: doc.Size}
	default:
		return &domain.Media{Kind: domain.MediaFile, FileName: d.name, Size: doc.Size}
	}
}

// isGIFImage reports whether a document is a real GIF image, by its
// mime type or, failing that, its name: Telegram's own GIF picker
// always sets "image/gif", but a GIF attached as a plain file carries
// whatever mime type the sender's system guessed, so the name is the
// fallback.
func isGIFImage(mimeType, name string) bool {
	return mimeType == "image/gif" || strings.HasSuffix(strings.ToLower(name), ".gif")
}

// gifFileName is name, or a sensible default ending ".gif" when
// Telegram sent no filename attribute at all: a classification hint
// for the UI's file-type labelling only, never what picks its decoder
// (see app.DecodeAsForMedia).
func gifFileName(name string) string {
	if name == "" {
		return "image.gif"
	}

	return name
}

// stickerMedia normalizes a sticker document: a static WebP is
// fetchable the same way a photo is, with FileName set for
// Connector.FetchMedia to download; a Lottie animation
// ("application/x-tgsticker") or a WebM video sticker gets no FileName,
// since this UI has nothing that can show either as a static image, so
// the UI falls back to the thumbnail or the sticker's own emoji. The mime
// type is the sender's claim, so FetchMedia still runs any sticker it is
// asked for through imagesafe (see documentImageProfile).
func stickerMedia(doc *tg.Document, d document) *domain.Media {
	m := &domain.Media{
		Kind: domain.MediaSticker, Width: d.width, Height: d.height,
		Size: doc.Size, Thumb: strippedThumb(doc.Thumbs), Emoji: d.emoji,
	}
	if doc.MimeType == "image/webp" {
		m.FileName = "sticker.webp"
	}

	return m
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

// strippedThumb is the tiny blurred preview among a photo's or
// document's sizes as a base64 JPEG, once imagesafe has checked it,
// or "" when Telegram sent none or it failed that check. Expand turns
// Telegram's own compact encoding back into a real, if tiny, JPEG
// using fixed quantization tables, so this is already this
// connector's own bytes rather than a sender's; running it through
// imagesafe anyway costs little and means every thumbnail this UI
// ever shows took the same path (see docs/decisions.md).
func strippedThumb(sizes []tg.PhotoSizeClass) string {
	for _, size := range sizes {
		s, ok := size.(*tg.PhotoStrippedSize)
		if !ok {
			continue
		}

		jpeg, err := thumbnail.Expand(s.Bytes)
		if err != nil {
			continue
		}

		safe, ok := imagesafe.Sanitize(jpeg, imagesafe.Thumbnail)
		if !ok {
			return ""
		}

		return base64.StdEncoding.EncodeToString(safe)
	}

	return ""
}

// imageProfile is the imagesafe.Profile a downloaded message's media
// must pass before FetchMedia writes it to disk, and whether it is
// one at all: a photo, a static WebP sticker, or a video document
// Telegram itself marks as an actual GIF file rather than an encoded
// clip, the only media kinds this UI ever loads into a QML Image or
// AnimatedImage. Everything else is written unchanged.
func imageProfile(m tg.MessageMediaClass) (imagesafe.Profile, bool) {
	switch mm := m.(type) {
	case *tg.MessageMediaPhoto:
		return imagesafe.FullImage, true
	case *tg.MessageMediaDocument:
		return documentImageProfile(mm.Document)
	default:
		return imagesafe.Profile{}, false
	}
}

// documentImageProfile is imageProfile's own case for a document,
// mirroring documentMedia's own dispatch order exactly so the two
// never disagree about which document is which: any sticker, whatever
// mime type the sender claims for it, since the UI shows a fetched
// sticker in a QML Image and imagesafe refuses bytes it cannot
// validate, or - once d.sticker and d.video are both ruled out, the
// same order documentMedia checks them in - a document isGIFImage
// says is a genuine GIF file. A document carrying DocumentAttributeVideo
// is written unchanged here regardless of its Animated attribute or
// name: a sender fully controls both and could set them on bytes that
// are not GIF-format at all, which is exactly what this skips running
// through imagesafe's image-only checks for. That sender-controlled
// claim is never the last word, though - app.DecodeAsForMedia checks
// the actual downloaded bytes afterwards and only then sanitizes and
// reports a genuine GIF, never trusting this attribute alone.
func documentImageProfile(dc tg.DocumentClass) (imagesafe.Profile, bool) {
	doc, ok := dc.(*tg.Document)
	if !ok {
		return imagesafe.Profile{}, false
	}

	d := describe(doc)
	switch {
	case d.sticker:
		return imagesafe.Thumbnail, true
	case d.video:
		return imagesafe.Profile{}, false
	case isGIFImage(doc.MimeType, d.name):
		return imagesafe.FullImage, true
	default:
		return imagesafe.Profile{}, false
	}
}
