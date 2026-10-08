package whatsapp

import (
	"encoding/base64"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// mediaRef is what a connector needs to download a message's media
// later. Telegram re-fetches a message for a fresh file reference at
// download time (see the Telegram connector's FetchMedia), because its
// server keeps messages around to ask for again; WhatsApp's end-to-end
// messages have no such server copy, so whatever reference exists must
// be captured now, at receive time, for the connector to keep. It never
// becomes part of domain.Media: that type is also what the store would
// otherwise persist straight back out over the UI's protocol, and this
// data means nothing there.
type mediaRef struct {
	Kind          mediaKind
	DirectPath    string
	MediaKey      []byte
	FileSHA256    []byte
	FileEncSHA256 []byte
	FileLength    uint64
	Mimetype      string
}

// mediaKind says which of WhatsApp's media types a reference is for, so
// fetch.go and upload.go know the app-info key (see appInfo) to
// download or upload it with; nothing in the reference's own bytes says
// which key that is.
type mediaKind string

// The media kinds this connector normalizes a message's attachment
// into. mediaKindAudio covers both voice notes and ordinary audio
// files, which only send is unable to produce (see upload.go).
// mediaKindSticker shares WhatsApp's image app-info key with
// mediaKindImage: whatsmeow authenticates a sticker's download the same
// way it does an ordinary photo's.
const (
	mediaKindImage    mediaKind = "image"
	mediaKindVideo    mediaKind = "video"
	mediaKindAudio    mediaKind = "audio"
	mediaKindDocument mediaKind = "document"
	mediaKindSticker  mediaKind = "sticker"
)

// appInfo is the whatsmeow media type a reference's download or upload
// must authenticate with.
func appInfo(kind mediaKind) whatsmeow.MediaType {
	switch kind {
	case mediaKindImage, mediaKindSticker:
		return whatsmeow.MediaImage
	case mediaKindVideo:
		return whatsmeow.MediaVideo
	case mediaKindAudio:
		return whatsmeow.MediaAudio
	default:
		return whatsmeow.MediaDocument
	}
}

// media describes what a message carries besides its text, or nil when
// it carries nothing this UI can show as media.
func media(msg *waE2E.Message) *domain.Media {
	m, _, _ := mediaAndRef(msg)
	return m
}

// downloadRef is the reference needed to fetch a message's media later,
// and whether it carries one at all.
func downloadRef(msg *waE2E.Message) (mediaRef, bool) {
	_, ref, ok := mediaAndRef(msg)
	return ref, ok
}

// mediaAndRef is the one dispatch every media kind goes through, so
// media and downloadRef never disagree about which submessage a
// message's media came from.
func mediaAndRef(msg *waE2E.Message) (*domain.Media, mediaRef, bool) {
	switch {
	case msg.GetExtendedTextMessage() != nil:
		return linkPreview(msg.GetExtendedTextMessage()), mediaRef{}, false
	case msg.GetImageMessage() != nil:
		return imageMedia(msg.GetImageMessage())
	case msg.GetVideoMessage() != nil:
		return videoMedia(msg.GetVideoMessage())
	case msg.GetAudioMessage() != nil:
		return audioMedia(msg.GetAudioMessage())
	case msg.GetDocumentMessage() != nil:
		return documentMedia(msg.GetDocumentMessage())
	case msg.GetStickerMessage() != nil:
		return stickerMedia(msg.GetStickerMessage())
	default:
		return nil, mediaRef{}, false
	}
}

// linkPreview is a web page preview attached to a text message, or nil
// until WhatsApp found one worth showing.
func linkPreview(m *waE2E.ExtendedTextMessage) *domain.Media {
	title, description := m.GetTitle(), m.GetDescription()
	if title == "" && description == "" {
		return nil
	}

	return &domain.Media{
		Kind: domain.MediaLink, URL: m.GetMatchedText(),
		Title: title, Description: description, Thumb: thumb(m.GetJPEGThumbnail()),
	}
}

// imageMedia is a photo, with its reference for FetchMedia to download
// the full image later.
func imageMedia(m *waE2E.ImageMessage) (*domain.Media, mediaRef, bool) {
	image := &domain.Media{
		Kind: domain.MediaPhoto, Width: int(m.GetWidth()), Height: int(m.GetHeight()),
		Size: int64(m.GetFileLength()), Thumb: thumb(m.GetJPEGThumbnail()),
	}
	ref := mediaRef{
		Kind: mediaKindImage, DirectPath: m.GetDirectPath(), MediaKey: m.GetMediaKey(), FileSHA256: m.GetFileSHA256(),
		FileEncSHA256: m.GetFileEncSHA256(), FileLength: m.GetFileLength(), Mimetype: m.GetMimetype(),
	}

	return image, ref, true
}

// videoMedia is a video, with its reference for FetchMedia to download
// the full file later.
func videoMedia(m *waE2E.VideoMessage) (*domain.Media, mediaRef, bool) {
	video := &domain.Media{
		Kind: domain.MediaVideo, Width: int(m.GetWidth()), Height: int(m.GetHeight()),
		Duration: int(m.GetSeconds()), Size: int64(m.GetFileLength()), Thumb: thumb(m.GetJPEGThumbnail()),
	}
	ref := mediaRef{
		Kind: mediaKindVideo, DirectPath: m.GetDirectPath(), MediaKey: m.GetMediaKey(), FileSHA256: m.GetFileSHA256(),
		FileEncSHA256: m.GetFileEncSHA256(), FileLength: m.GetFileLength(), Mimetype: m.GetMimetype(),
	}

	return video, ref, true
}

// audioMedia is a voice note or other audio file, with its duration and
// its reference for FetchMedia to download it later. A voice note (PTT,
// "push to talk") gets its own kind so the UI can show an inline player
// instead of the plain file row an ordinary audio message gets.
func audioMedia(m *waE2E.AudioMessage) (*domain.Media, mediaRef, bool) {
	kind, name := domain.MediaFile, "audio-message.ogg"
	if m.GetPTT() {
		kind, name = domain.MediaVoice, "voice-message.ogg"
	}

	audio := &domain.Media{Kind: kind, FileName: name, Size: int64(m.GetFileLength()), Duration: int(m.GetSeconds())}
	ref := mediaRef{
		Kind: mediaKindAudio, DirectPath: m.GetDirectPath(), MediaKey: m.GetMediaKey(), FileSHA256: m.GetFileSHA256(),
		FileEncSHA256: m.GetFileEncSHA256(), FileLength: m.GetFileLength(), Mimetype: m.GetMimetype(),
	}

	return audio, ref, true
}

// documentMedia is a file sent as a document, with its reference for
// FetchMedia to download it later.
func documentMedia(m *waE2E.DocumentMessage) (*domain.Media, mediaRef, bool) {
	name := m.GetFileName()
	if name == "" {
		name = "file"
	}

	doc := &domain.Media{Kind: domain.MediaFile, FileName: name, Size: int64(m.GetFileLength())}
	ref := mediaRef{
		Kind: mediaKindDocument, DirectPath: m.GetDirectPath(), MediaKey: m.GetMediaKey(), FileSHA256: m.GetFileSHA256(),
		FileEncSHA256: m.GetFileEncSHA256(), FileLength: m.GetFileLength(), Mimetype: m.GetMimetype(),
	}

	return doc, ref, true
}

// stickerMedia normalizes a sticker: always a WebP image, animated or
// not, which FetchMedia downloads the same way it does a photo. A
// plain, non-animated Image component renders only a WebP's first
// frame, which is exactly the static display this UI wants, so an
// animated sticker needs no special case: Emoji still carries its
// associated emoji, shown instead of the image until it is fetched.
func stickerMedia(m *waE2E.StickerMessage) (*domain.Media, mediaRef, bool) {
	sticker := &domain.Media{
		Kind: domain.MediaSticker, Width: int(m.GetWidth()), Height: int(m.GetHeight()),
		Size: int64(m.GetFileLength()), FileName: "sticker.webp", Emoji: m.GetEmojis(),
		Thumb: thumb(m.GetPngThumbnail()),
	}
	ref := mediaRef{
		Kind: mediaKindSticker, DirectPath: m.GetDirectPath(), MediaKey: m.GetMediaKey(), FileSHA256: m.GetFileSHA256(),
		FileEncSHA256: m.GetFileEncSHA256(), FileLength: m.GetFileLength(), Mimetype: m.GetMimetype(),
	}

	return sticker, ref, true
}

// thumb base64-encodes a message's embedded JPEG preview, or "" when it
// sent none.
func thumb(jpeg []byte) string {
	if len(jpeg) == 0 {
		return ""
	}

	return base64.StdEncoding.EncodeToString(jpeg)
}
