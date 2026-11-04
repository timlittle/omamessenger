// Package imagesafe makes a sender-supplied image safe to decode inside
// omarchy-shell by never letting the shell's own C decoders see the
// sender's compressed bytes: Reencode decodes in memory-safe Go, checked
// against a Profile's caps, then writes a fresh JPEG or PNG (see
// docs/decisions.md). GIF bytes are refused outright; animation goes
// through the sandboxed video path instead.
package imagesafe

import (
	"bytes"
	"image"
	"io"
)

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
	// MaxAspectRatio caps the longer side divided by the shorter, so a
	// thin, very long image cannot clear MaxSide and MaxPixels while
	// still forcing an oversized decode via sourceSize scaling (see
	// docs/decisions.md). Zero means no limit.
	MaxAspectRatio float64
	// MaxDecodeBytes caps a header-only estimate of peak decode memory,
	// checked before the real decode runs: MaxSide and MaxPixels alone
	// say nothing about how expensive a progressive JPEG or 16-bit PNG
	// can be per pixel (see docs/decisions.md). Zero means no limit.
	MaxDecodeBytes int64
	// DownscaleTo, when positive, is the longer side Reencode shrinks a
	// decoded image down to (never up) before encoding, once it has
	// already passed every cap above. Zero means refuse instead of resizing.
	DownscaleTo int
}

// maxRealisticAspectRatio bounds every profile's MaxAspectRatio to the most
// extreme shape a genuine photo has, while still catching a decode-bomb
// thumbnail's far more extreme shape (see docs/decisions.md).
const maxRealisticAspectRatio = 16

// Thumbnail bounds a message's embedded preview and a downloaded sticker:
// both are always drawn small, so a declared size above a phone screen's
// pixel count already means the sender is lying. DownscaleTo 512 matches
// the largest a real sticker legitimately is (see docs/decisions.md).
var Thumbnail = Profile{
	MaxSide: 1024, MaxPixels: 1024 * 1024, MaxInputLen: 2 << 20,
	MaxAspectRatio: maxRealisticAspectRatio,
	MaxDecodeBytes: 64 << 20, DownscaleTo: 512,
}

// FullImage bounds a downloaded photo, decoded and shown as-is, never
// shrunk. MaxSide and MaxPixels match what a real photo needs (WhatsApp's
// HD cap is 4096, Telegram's 2560), not what Qt could in principle decode
// (see docs/decisions.md for the measurements behind these caps). An
// outgoing attachment or paste uses Outgoing instead.
var FullImage = Profile{
	MaxSide: 4096, MaxPixels: 4096 * 4096, MaxInputLen: 32 << 20,
	MaxAspectRatio: maxRealisticAspectRatio,
	MaxDecodeBytes: 768 << 20,
}

// Outgoing bounds a photo the user is attaching or pasting to send: it
// accepts what a real camera or screenshot produces, then shrinks the
// result to DownscaleTo (WhatsApp's HD maximum) on its longer side rather
// than refusing it outright (see docs/decisions.md).
var Outgoing = Profile{
	MaxSide: 12000, MaxPixels: 50_000_000, MaxInputLen: 64 << 20,
	MaxAspectRatio: maxRealisticAspectRatio, MaxDecodeBytes: 768 << 20,
	DownscaleTo: 4096,
}

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

// OutputFormat is the image format Reencode actually wrote its result
// as, which a caller must use to name or label that result - a cache
// file's own extension, a data URI's mime type - since it need not
// match data's own input format at all (see Reencode).
type OutputFormat int

// The formats Reencode ever writes: always FormatJPEG or FormatPNG,
// whatever format data arrived as.
const (
	FormatUnknown OutputFormat = iota
	FormatJPEG
	FormatPNG
)

// Extension is format's own canonical file extension, for a caller
// naming a cache file from Reencode's result so the name on disk
// always matches the bytes actually written there.
func (f OutputFormat) Extension() string {
	switch f {
	case FormatJPEG:
		return ".jpg"
	case FormatPNG:
		return ".png"
	default:
		return ""
	}
}

// MIME is format's own canonical media type, for a caller building a
// data URI from Reencode's result.
func (f OutputFormat) MIME() string {
	switch f {
	case FormatJPEG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	default:
		return ""
	}
}

// Sanitize is Reencode without its own output format, for a caller
// that only ever treats an image as opaque bytes to write or embed
// and never needs to know which encoder produced them.
func Sanitize(data []byte, profile Profile) (out []byte, width, height int, ok bool) {
	out, _, width, height, ok = Reencode(data, profile)
	return out, width, height, ok
}

// Reencode makes data safe to decode inside omarchy-shell under profile's
// caps, reporting the format it wrote and the real width and height (see
// docs/decisions.md). Decoding recovers from any decoder panic as a plain
// refusal. A GIF or animated WebP is refused outright, never silently
// reduced to one frame.
//
// ok is false when data is unrecognized, a GIF, malformed, over profile's
// caps, or an animated WebP; the other results are zero in that case.
// Nothing is logged here.
func Reencode(data []byte, profile Profile) (out []byte, format OutputFormat, width, height int, ok bool) {
	if int64(len(data)) > profile.MaxInputLen || len(data) == 0 {
		return nil, FormatUnknown, 0, 0, false
	}

	switch detectFormat(data) {
	case formatUnknown, formatGIF:
		return nil, FormatUnknown, 0, 0, false
	}

	return reencodeStill(data, profile)
}

// Dimensions reads width and height from r's header alone, with no full
// decode. ok is false when r's content cannot be parsed.
func Dimensions(r io.Reader) (width, height int, ok bool) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, false
	}

	return cfg.Width, cfg.Height, true
}

// LooksLikeImage reports whether data's leading bytes are a recognized
// image format (PNG, JPEG, GIF or WebP), with none of Reencode's other
// checks run (see docs/decisions.md).
func LooksLikeImage(data []byte) bool {
	return detectFormat(data) != formatUnknown
}

// withinCaps reports whether a width and height both fit profile: each
// side within MaxSide, their product within MaxPixels, and their
// ratio within MaxAspectRatio. Width and height come in as int64 so a
// caller reading a 24- or 32-bit field straight off the wire never
// overflows int before this check runs.
func withinCaps(width, height int64, profile Profile) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	if width > int64(profile.MaxSide) || height > int64(profile.MaxSide) {
		return false
	}
	if width*height > profile.MaxPixels {
		return false
	}

	return withinAspectRatio(width, height, profile.MaxAspectRatio)
}

// withinAspectRatio reports whether width and height's ratio - the
// longer side divided by the shorter - stays within maxRatio. A
// maxRatio of zero means no limit, so a caller that never sets
// MaxAspectRatio keeps its old, unbounded behaviour.
func withinAspectRatio(width, height int64, maxRatio float64) bool {
	if maxRatio <= 0 {
		return true
	}

	longer, shorter := max(width, height), min(width, height)

	return float64(longer) <= float64(shorter)*maxRatio
}
