// Package imagesafe makes a sender-supplied image safe to decode
// inside omarchy-shell by never letting the shell's own C decoders
// (libjpeg-turbo, libpng, libwebp) see the sender's compressed bytes
// at all: Reencode decodes a PNG, JPEG or still WebP fully in
// memory-safe Go, checked against a Profile's caps before and after
// that decode, then writes a fresh JPEG or PNG of its own - the one
// class of bug this closes is CVE-2023-4863 and its relatives, where
// a crafted libwebp input corrupts memory inside the process that
// decodes it (see docs/decisions.md). A real GIF animation is no
// longer a case this package handles at all: every connector routes
// one through the same sandboxed video path a "GIF video" already
// used (see docs/decisions.md), so Reencode refuses GIF bytes
// outright, by their own magic header, never attempting a decode -
// deliberately, rather than relying on this package not happening to
// link a GIF decoder, which a test binary or a future change
// elsewhere can never guarantee on its own.
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
	// MaxTotalPixels caps the canvas's pixel count multiplied by its
	// frame count: Qt decodes the whole canvas for every frame of an
	// animation, so this is the decoding work one loop costs, which a
	// small, highly compressed file could otherwise make enormous.
	MaxTotalPixels int64
	// MaxAspectRatio caps how much longer one side may be than the
	// other - the longer side divided by the shorter - so a very thin,
	// very long image cannot pass MaxSide and MaxPixels on its own
	// declared shape and still force Qt's own sourceSize scaling to
	// decode far more than either cap alone suggests: a box asks Qt to
	// cover it by scaling to the image's longer side, so a 1x1024
	// thumbnail asked to fill a 320x400 box decodes around 320x327680,
	// about 105 megapixels, even though 1024 pixels is well under every
	// other cap here (see docs/decisions.md). Zero means no limit.
	MaxAspectRatio float64
	// MaxDecodeBytes caps a cheap, header-only estimate of how much
	// memory the real decode below would need at peak - width times
	// height times bytes per pixel for the format and bit depth
	// involved, plus a progressive JPEG's own coefficient blocks (see
	// estimatedDecodeBytes) - checked before that decode ever runs.
	// MaxSide and MaxPixels alone say nothing about how expensive a
	// shape within them can still be: a progressive JPEG or a
	// 16-bit-per-channel PNG costs many times more per pixel than an
	// ordinary photo, which is exactly what this catches instead (see
	// docs/decisions.md for the measured peaks this is calibrated
	// against). Zero means no limit, the same convention
	// MaxAspectRatio already uses.
	MaxDecodeBytes int64
	// DownscaleTo, when positive, is the longer side Reencode shrinks a
	// decoded image down to - never up - before encoding it, once it
	// has already passed every cap above: Outgoing's own way of
	// meeting a real camera photo's actual size without having to
	// accept everything that size could cost to decode as this
	// profile's own accept caps. Zero means never resize: refuse
	// instead, Thumbnail and FullImage's own behaviour, unchanged.
	DownscaleTo int
}

// maxRealisticAspectRatio bounds every profile's own MaxAspectRatio:
// the most extreme shape a genuine photo actually has. A panorama
// stitched from several frames can reach about 10:1 and a tall phone
// screenshot is only about 1:2.2, so 16:1 clears every real shape with
// headroom to spare, while still catching the far more extreme shape
// (1024:1 and beyond) a decode-bomb thumbnail needs to turn a bounded
// sourceSize box into a huge decode (see Profile.MaxAspectRatio).
const maxRealisticAspectRatio = 16

// Thumbnail bounds a message's embedded preview and a downloaded
// sticker: both are always drawn small (see ui/components/PhotoView.qml
// and StickerView.qml), so a declared size above a phone screen's own
// pixel count already means the sender is lying, not sending a real
// preview.
var Thumbnail = Profile{
	MaxSide: 1024, MaxPixels: 1024 * 1024, MaxInputLen: 2 << 20,
	MaxTotalPixels: 64_000_000, MaxAspectRatio: maxRealisticAspectRatio,
	MaxDecodeBytes: 64 << 20,
}

// FullImage bounds a downloaded photo: one this account received,
// which this helper never trusts further than confirming it decodes
// safely and showing it, never shrinking it - there is no "too big to
// receive" the way there is a limit to what makes sense to send. Its
// MaxSide and MaxPixels match what a real photo actually needs, not
// what Qt could in principle be asked to decode: WhatsApp's own HD
// photos cap out at 4096 pixels on the long side, and Telegram's at
// 2560 (see connector/whatsapp/limits.go and
// connector/telegram/limits.go), so 4096x4096 clears every real photo
// with room to spare. It used to be 8192x8192/40 megapixels;
// measuring runtime.MemStats peaks at that old cap found a single
// decode could reach close to a gigabyte on its own (a 4-component
// progressive JPEG, or a 16-bit PNG) - reasonable for one photo, but
// the UI auto-fetches every visible one, so several at once could
// reach many gigabytes with no cap on how many decode in parallel at
// all (see app.decodeSlots, which now serialises every one of them,
// and docs/decisions.md for the measurements this cap and
// MaxDecodeBytes are both calibrated against). Its total allows a
// single frame of that size and no more. An outgoing attachment or
// clipboard paste - the user's own deliberate choice, not an
// attacker's, and shrunk to fit a real service's own limit rather
// than refused outright - uses Outgoing instead.
var FullImage = Profile{
	MaxSide: 4096, MaxPixels: 4096 * 4096, MaxInputLen: 32 << 20,
	MaxTotalPixels: 40_000_000, MaxAspectRatio: maxRealisticAspectRatio,
	MaxDecodeBytes: 768 << 20,
}

// Outgoing bounds a photo the user is attaching or pasting to send:
// their own deliberate choice, one at a time, behind the same
// app.decodeSlots serialisation FullImage's own decodes share - never
// an attacker's, and never auto-fetched - so it accepts what a real
// camera or screenshot actually produces (a 24, 48 or 50 megapixel
// phone photo; a 5K or dual-4K desktop screenshot) rather than
// FullImage's own, far smaller limit, then shrinks the result to
// DownscaleTo (4096, WhatsApp's own HD maximum; Telegram serves 2560)
// on its longer side - never upscaled - before encoding, rather than
// refusing a real photo outright just because it needs resizing
// before it can be sent. MaxDecodeBytes is deliberately the same
// budget FullImage uses, not a larger one scaled up for the larger
// pixel caps: the measurements it was calibrated against (see
// docs/decisions.md) already found that budget sane for one decode at
// a time, and a real photo - almost always a baseline, 8-bit JPEG -
// fits it with room to spare even at 50 megapixels; only a
// progressive or 16-bit-per-channel shape anywhere near that size,
// never produced by an ordinary camera or screenshot, hits it.
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

// Reencode makes data safe to decode inside omarchy-shell under
// profile's own caps, together with the format it wrote (see
// OutputFormat) and the real width and height of what it wrote -
// never whatever size an unrelated caller, such as a message's own
// claimed photo size, might otherwise assume matches it (see
// docs/decisions.md).
//
// data is decoded fully, in memory-safe Go (recovering from any panic
// a decoder raises on a crafted input as a plain refusal, never a
// crash - see decodeImage), after its declared dimensions already
// passed profile's caps from a cheap header read alone (see
// reencodeStill), so a declared-huge image is refused before
// anything tries to allocate a bitmap for it. The decoded pixels are
// then written fresh as JPEG, quality 90, or PNG when they carry any
// real transparency (see hasAlpha) - a sticker's own cut-out shape,
// say. A real GIF animation is refused outright, by its own magic
// header, before any decode is attempted: every connector routes one
// through the sandboxed video path instead (see docs/decisions.md),
// and this package must refuse it deliberately rather than rely on
// no GIF decoder happening to be linked into the binary, which
// nothing here can actually guarantee. An animated WebP has no Go
// decoder either (golang.org/x/image/webp decodes a still WebP
// only), so it is refused outright too, never silently reduced to
// one frame.
//
// ok is false when data is not a format this package recognizes, is
// a GIF, is too large or malformed to even parse, decodes to a size
// over profile's own caps, or is an animated WebP; out, format,
// width and height are all zero or FormatUnknown in that case, so a
// caller cannot accidentally use a partially-checked result. Nothing
// is logged here: callers log only the safe category they already
// have (see app.MediaFetchReason), never these bytes or this
// function's own reasoning.
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

// Dimensions reads width and height from r's own header alone, with
// no full decode: cheap enough to call on every cache hit a caller
// serves again for a file Reencode already wrote, unlike Reencode's
// own full decode. ok is false when r's content cannot even be
// parsed as one of the formats this package registers a decoder for.
func Dimensions(r io.Reader) (width, height int, ok bool) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, false
	}

	return cfg.Width, cfg.Height, true
}

// LooksLikeImage reports whether data's own leading bytes are one of
// the formats Sanitize recognizes (PNG, JPEG, GIF or WebP), with none
// of Sanitize's other checks run at all: a cheap pre-filter for a
// caller deciding whether a file already on disk is an image in the
// first place - the one kind of downloaded media Qt's own decoders
// read directly - before treating anything that fails that question
// as some other, unrelated kind of file it must leave alone, rather
// than an image that merely failed Sanitize outright (see
// docs/decisions.md).
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
