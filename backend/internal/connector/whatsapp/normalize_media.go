package whatsapp

import (
	"encoding/base64"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
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
	// FileName is only ever set for a mediaKindDocument reference,
	// where it is what lets FetchMedia tell a genuine GIF image sent
	// as a plain file (see isGIFImage) apart from every other kind of
	// document, which Kind alone cannot: unlike image and sticker,
	// Kind never gets its own value for this case (see imageProfile).
	FileName string
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
	case msg.GetPtvMessage() != nil:
		// A video note ("round" video message) is WhatsApp's own
		// VideoMessage with a different field name for its wrapper;
		// nothing about how it is downloaded or shown differs from an
		// ordinary video.
		return videoMedia(msg.GetPtvMessage())
	case msg.GetAudioMessage() != nil:
		return audioMedia(msg.GetAudioMessage())
	case msg.GetDocumentMessage() != nil:
		return documentMedia(msg.GetDocumentMessage())
	case msg.GetStickerMessage() != nil:
		return stickerMedia(msg.GetStickerMessage())
	case pollCreation(msg) != nil:
		// A poll has no file to fetch: its question, options and
		// tallies are structured data, not media, so there is no
		// reference for FetchMedia to keep.
		return pollCreationMedia(pollCreation(msg)), mediaRef{}, false
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
		// GifPlayback is WhatsApp's own flag for a video sent as a
		// silent, looping "GIF" rather than an ordinary clip.
		Animated: m.GetGifPlayback(),
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
// FetchMedia to download it later. A real GIF image sent this way
// (rather than a video WhatsApp itself converted to a silent mp4 with
// GifPlayback set) carries nothing to tell it apart from any other
// document except its mime type or name, so it is marked the same way
// that mp4 is - MediaVideo with Animated true - to reach the same
// inline view. That mime type and name are only ever classification
// hints, though, both fully sender-controlled: the UI's actual
// decoder choice never comes from here, only from app.DecodeAsForMedia
// checking the downloaded file's own bytes once it is on disk.
func documentMedia(m *waE2E.DocumentMessage) (*domain.Media, mediaRef, bool) {
	ref := mediaRef{
		Kind: mediaKindDocument, DirectPath: m.GetDirectPath(), MediaKey: m.GetMediaKey(), FileSHA256: m.GetFileSHA256(),
		FileEncSHA256: m.GetFileEncSHA256(), FileLength: m.GetFileLength(), Mimetype: m.GetMimetype(),
		FileName: m.GetFileName(),
	}

	if isGIFImage(m.GetMimetype(), m.GetFileName()) {
		doc := &domain.Media{Kind: domain.MediaVideo, FileName: gifFileName(m.GetFileName()), Size: int64(m.GetFileLength()), Animated: true}
		return doc, ref, true
	}

	name := m.GetFileName()
	if name == "" {
		name = "file"
	}

	doc := &domain.Media{Kind: domain.MediaFile, FileName: name, Size: int64(m.GetFileLength())}

	return doc, ref, true
}

// isGIFImage reports whether a document is a real GIF image, by its
// mime type or, failing that, its name: WhatsApp's own client always
// sets "image/gif" for one, but a GIF forwarded from elsewhere may only
// have its name to go on.
func isGIFImage(mimeType, name string) bool {
	return mimeType == "image/gif" || strings.HasSuffix(strings.ToLower(name), ".gif")
}

// gifFileName is name, or a sensible default ending ".gif" when the
// message carries no file name at all: a classification hint for the
// UI's file-type labelling only, never what picks its decoder (see
// app.DecodeAsForMedia).
func gifFileName(name string) string {
	if name == "" {
		return "image.gif"
	}

	return name
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

// thumb base64-encodes a message's embedded preview - a JPEG for
// every kind but a sticker, which carries a PNG - once imagesafe has
// stripped it of anything compressed or declaring a size this UI
// never needs for a thumbnail. Returns "" when the message sent no
// preview at all, or the preview failed that check: a sender's
// thumbnail is exactly the untrusted image bytes the review that
// found this package's own reason for existing was about (see
// docs/decisions.md), so nothing unsanitized ever reaches the UI as a
// thumbnail either.
func thumb(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	safe, ok := imagesafe.Sanitize(data, imagesafe.Thumbnail)
	if !ok {
		return ""
	}

	return base64.StdEncoding.EncodeToString(safe)
}

// imageProfile is the imagesafe.Profile ref's downloaded bytes must
// pass before FetchMedia writes them to disk, and whether ref is one
// at all: an image or a sticker, both of which whatsmeow authenticates
// the same way (see appInfo), always reach a QML Image component
// directly, and so does a document that turned out to be a genuine
// GIF image (see isGIFImage) - the one case Kind itself cannot name,
// since documentMedia has no kind of its own for it and FileName is
// what this falls back to. Every other kind is written unchanged.
func imageProfile(ref mediaRef) (imagesafe.Profile, bool) {
	switch {
	case ref.Kind == mediaKindImage:
		return imagesafe.FullImage, true
	case ref.Kind == mediaKindSticker:
		return imagesafe.Thumbnail, true
	case ref.Kind == mediaKindDocument && isGIFImage(ref.Mimetype, ref.FileName):
		return imagesafe.FullImage, true
	default:
		return imagesafe.Profile{}, false
	}
}
