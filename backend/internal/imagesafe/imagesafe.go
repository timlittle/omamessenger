// Package imagesafe strips the parts of a sender-supplied image that
// a connector has no use for before it is written to disk or embedded
// as a thumbnail: the compressed text chunks Qt's own image plugins
// still inflate (PNG zTXt/iTXt/iCCP, JPEG's APPn/COM segments) and a
// declared width or height Qt would otherwise allocate a bitmap for
// before ever checking it looks sane. Every byte this package returns
// still decodes to the same pixels; nothing here resizes or
// re-encodes an image, only removes what rendering never reads (see
// docs/decisions.md).
package imagesafe

import "bytes"

// Profile bounds how large a sanitized image may declare itself and
// how many undecoded bytes it may arrive as. The bound is chosen by
// how the image is shown, not by what the sender claims it is: a
// thumbnail or sticker is always drawn small, so a cap tuned to a
// photo would still let a bomb through where it is never needed.
type Profile struct {
	// MaxSide caps width and height, in pixels.
	MaxSide int
	// MaxPixels caps width multiplied by height, so a very wide, very
	// short image cannot slip under MaxSide on both sides and still
	// demand a huge bitmap.
	MaxPixels int64
	// MaxInputLen caps the undecoded input, in bytes, checked before
	// any parsing: a cheap rejection for anything absurdly large to be
	// a preview or sticker in the first place.
	MaxInputLen int64
}

// Thumbnail bounds a message's embedded preview and a downloaded
// sticker: both are always drawn small (see ui/components/PhotoView.qml
// and StickerView.qml), so a declared size above a phone screen's own
// pixel count already means the sender is lying, not sending a real
// preview.
var Thumbnail = Profile{MaxSide: 1024, MaxPixels: 1024 * 1024, MaxInputLen: 2 << 20}

// FullImage bounds a downloaded photo, and a video a service marked as
// a silent, looping "GIF" whose file is itself a real .gif (see
// InlineVideo.qml's realGif). Generous enough for any photo a phone
// camera takes - none shoot past a small fraction of 8192 pixels on a
// side - but far short of the gigabyte-scale bitmap a crafted header
// can otherwise make Qt allocate.
var FullImage = Profile{MaxSide: 8192, MaxPixels: 40_000_000, MaxInputLen: 32 << 20}

// format is one of the image formats this package knows how to
// sanitize; every other byte stream is rejected outright, never
// forwarded as if it were safe.
type format int

// The formats Sanitize recognizes, matched by magic bytes rather than
// a sender's claimed mimetype, which names nothing about what the
// bytes actually are.
const (
	formatUnknown format = iota
	formatPNG
	formatJPEG
	formatGIF
	formatWebP
)

// pngSignature is every PNG file's fixed first eight bytes.
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// detectFormat reads data's magic bytes to say which format it is, or
// formatUnknown for anything else, including a truncated header too
// short to hold one.
func detectFormat(data []byte) format {
	switch {
	case isPNG(data):
		return formatPNG
	case isJPEG(data):
		return formatJPEG
	case isGIF(data):
		return formatGIF
	case isWebP(data):
		return formatWebP
	default:
		return formatUnknown
	}
}

// isPNG reports whether data starts with PNG's own fixed signature.
func isPNG(data []byte) bool {
	return bytes.HasPrefix(data, pngSignature)
}

// isJPEG reports whether data starts with JPEG's SOI marker followed
// by the start of another marker, true of every JPEG regardless of
// what follows.
func isJPEG(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
}

// isGIF reports whether data starts with either of GIF's two fixed
// header signatures.
func isGIF(data []byte) bool {
	return len(data) >= 6 && (bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")))
}

// isWebP reports whether data is a RIFF container holding a WEBP
// payload.
func isWebP(data []byte) bool {
	return len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
}

// Sanitize returns data with every chunk or segment its format does
// not need for rendering removed, after checking its declared
// dimensions against profile. ok is false when data is not a format
// this package recognizes, is structurally broken (a bad length, a
// failed checksum, a truncated chunk), or declares a size profile
// rejects; out is always nil in that case, so a caller cannot
// accidentally use a partially-checked result. Nothing is logged here:
// callers log only the safe category they already have (see
// app.MediaFetchReason), never these bytes or this function's own
// reasoning.
func Sanitize(data []byte, profile Profile) (out []byte, ok bool) {
	if int64(len(data)) > profile.MaxInputLen || len(data) == 0 {
		return nil, false
	}

	switch detectFormat(data) {
	case formatPNG:
		return sanitizePNG(data, profile)
	case formatJPEG:
		return sanitizeJPEG(data, profile)
	case formatGIF:
		return sanitizeGIF(data, profile)
	case formatWebP:
		return sanitizeWebP(data, profile)
	default:
		return nil, false
	}
}

// withinCaps reports whether a width and height both fit profile: each
// side within MaxSide, and their product within MaxPixels. Width and
// height come in as int64 so a caller reading a 24- or 32-bit field
// straight off the wire never overflows int before this check runs.
func withinCaps(width, height int64, profile Profile) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	if width > int64(profile.MaxSide) || height > int64(profile.MaxSide) {
		return false
	}

	return width*height <= profile.MaxPixels
}
