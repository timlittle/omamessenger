package imagesafe

import (
	"bytes"
	"image"
	_ "image/gif" // registers the GIF decoder Sanitize's own confirmation decode uses
)

// gifMaxFrames bounds how many image blocks sanitizeGIF will walk: far
// more than any sticker or message GIF has, so only a crafted file
// built purely to make the walk itself expensive ever hits it.
const gifMaxFrames = 1024

// gifMaxDecodeRate bounds how many pixels of a GIF's own canvas
// AnimatedImage may be made to decode each second: it decodes the
// whole canvas again for every frame (see AnimatedGIF's own comment),
// on the shell's main thread, so a sender's own frame delays are the
// only thing standing between a normal animation and one that pins
// that thread solid for as long as it plays - which a looping GIF can
// ask to be forever. 30 megapixels a second keeps even this package's
// largest per-frame cap (AnimatedGIF.MaxPixels, 4.2 million) to a
// handful of frames a second, well short of pinning a core, while
// never slowing a real GIF's own normal pace.
const gifMaxDecodeRate = 30_000_000

// gifMinFrameDelayCS is the floor a 0 or 1 centisecond delay is raised
// to before gifMinDelayCS's own rate bound is applied: the convention
// browsers already use, since both values really mean "no delay was
// set", not "render as fast as possible".
const gifMinFrameDelayCS = 10

// gifMinDelayCS returns the smallest delay, in centiseconds, that
// keeps decoding canvasPixels worth of canvas no faster than
// gifMaxDecodeRate pixels a second, raising raw first to
// gifMinFrameDelayCS when it is the 0 or 1 cs value that really means
// "unset". The loop count itself is left alone: once every frame's own
// delay is bounded this way, a GIF set to loop forever costs no more
// per second than any other video a person can already leave playing
// on loop, so it needs no cap of its own (see docs/decisions.md).
func gifMinDelayCS(raw uint16, canvasPixels int64) uint16 {
	delay := raw
	if delay <= 1 {
		delay = gifMinFrameDelayCS
	}

	if canvasPixels <= 0 {
		return delay
	}

	rate := (canvasPixels*100 + gifMaxDecodeRate - 1) / gifMaxDecodeRate
	if rate > int64(delay) {
		delay = uint16(min(rate, 65535))
	}

	return delay
}

// sanitizeGIF rebuilds data's block stream, dropping its Comment and
// Plain Text extensions outright and any Application extension except
// the Netscape looping one AnimatedImage needs, after checking the
// logical screen's and every frame's own dimensions against profile.
// ok is false for anything structurally wrong or over profile's caps.
// A file of more than one frame also gets every Graphics Control
// extension's own delay raised to gifMinDelayCS's bound (see
// writeGIFBlocks): countGIFFrames learns the frame count first,
// since a single still frame - true of every real, non-animated GIF,
// and never rewritten by this package before this bound existed -
// never repeats, so it carries none of the risk a delay bound exists
// to close and is left exactly as it arrived.
func sanitizeGIF(data []byte, profile Profile) ([]byte, bool) {
	if len(data) < 13 {
		return nil, false
	}

	width, height := int64(leUint16(data[6:8])), int64(leUint16(data[8:10]))
	if !withinCaps(width, height, profile) {
		return nil, false
	}

	screenEnd, ok := gifScreenEnd(data)
	if !ok {
		return nil, false
	}

	frames, ok := countGIFFrames(data, screenEnd, profile)
	if !ok || !withinTotal(width, height, frames, profile) {
		return nil, false
	}

	var out bytes.Buffer
	out.Write(data[0:6]) // "GIF87a" or "GIF89a"
	pos, ok := copyGIFScreen(data, &out)
	if !ok {
		return nil, false
	}

	w := &gifWriter{profile: profile, canvasPixels: width * height}
	kept, ok := writeGIFBlocks(data, pos, &out, w, frames > 1)
	if !ok {
		return nil, false
	}

	return kept, true
}

// gifScreenEnd returns the position right after the logical screen
// descriptor and, when the packed field's flag bit says there is
// one, the global color table right after it - every block walker's
// own starting point, computed without copying anything.
func gifScreenEnd(data []byte) (pos int, ok bool) {
	packed := data[10]
	pos = 13

	if packed&0x80 == 0 {
		return pos, true
	}

	size := 3 * (1 << ((packed & 0x07) + 1))
	if pos+size > len(data) {
		return 0, false
	}

	return pos + size, true
}

// copyGIFScreen copies the logical screen descriptor and, when
// gifScreenEnd finds one, the global color table right after it, and
// returns the position right after both.
func copyGIFScreen(data []byte, out *bytes.Buffer) (pos int, ok bool) {
	out.Write(data[6:13])

	pos, ok = gifScreenEnd(data)
	if !ok {
		return 0, false
	}
	if pos > 13 {
		out.Write(data[13:pos])
	}

	return pos, true
}

// countGIFFrames walks data from pos exactly as writeGIFBlocks does,
// without writing anything, only to learn how many image blocks the
// file has: sanitizeGIF needs that count before it can decide
// whether a file actually animates, since an image block's own
// Graphics Control extension is only ever rewritten for one that
// does (see writeGIFBlocks).
func countGIFFrames(data []byte, pos int, profile Profile) (int, bool) {
	frames := 0

	for pos < len(data) {
		switch data[pos] {
		case 0x3B:
			return frames, true
		case 0x21:
			end, _, ok := gifExtension(data, pos)
			if !ok {
				return 0, false
			}
			pos = end
		case 0x2C:
			end, ok := gifImage(data, pos, profile)
			if !ok {
				return 0, false
			}

			frames++
			if frames > gifMaxFrames {
				return 0, false
			}
			pos = end
		default:
			return 0, false
		}
	}

	return 0, false
}

// gifWriter carries writeGIFBlocks' own context across the whole
// walk - the profile a frame's dimensions are checked against, the
// canvas pixel count gifMinDelayCS rates a frame's delay against,
// the frame count so far, and whether a Graphics Control extension
// has already been seen for the frame about to be written - so that
// walk's own functions, and the one it calls per frame, each take
// only the handful of arguments that call actually needs.
type gifWriter struct {
	profile      Profile
	canvasPixels int64
	frames       int
	sawGCE       bool
}

// writeGIFBlocks copies every extension and image block from pos to
// the trailer, dropping the ones gifExtension says to, and returns
// the position right after it is done. When animated is true - more
// than one frame, so the file actually plays as an animation rather
// than a single still image - it also raises every Graphics Control
// extension's own delay time to gifMinDelayCS's bound for w's own
// canvasPixels (see noteExtension and writeFrame below). A single
// still frame is never touched this way: it does not repeat, so it
// carries none of the risk the bound exists to close.
func writeGIFBlocks(data []byte, pos int, out *bytes.Buffer, w *gifWriter, animated bool) ([]byte, bool) {
	for pos < len(data) {
		switch data[pos] {
		case 0x3B:
			out.WriteByte(0x3B)
			return finishGIF(out.Bytes())
		case 0x21:
			w.noteExtension(data, pos)
			next, ok := copyGIFExtensionBlock(data, pos, out, w.canvasPixels, animated)
			if !ok {
				return nil, false
			}
			pos = next
		case 0x2C:
			next, ok := w.writeFrame(data, pos, out, animated)
			if !ok {
				return nil, false
			}
			pos = next
		default:
			return nil, false
		}
	}

	return nil, false
}

// noteExtension records in w whether the extension block at pos is a
// Graphics Control extension, for writeFrame to check once it reaches
// the image block this extension, if any, belongs to.
func (w *gifWriter) noteExtension(data []byte, pos int) {
	if pos+1 < len(data) && data[pos+1] == 0xF9 {
		w.sawGCE = true
	}
}

// writeFrame writes the image block at pos to out, first
// synthesizing a minimal Graphics Control extension ahead of it, at
// gifMinDelayCS's own floor for w's own canvasPixels, when animated
// is true and none has been seen for this frame yet (see
// noteExtension): every reader's default delay for a frame missing
// one is zero, the same unbounded case a sender's own explicit zero
// already is. It returns the position right after the image block.
func (w *gifWriter) writeFrame(data []byte, pos int, out *bytes.Buffer, animated bool) (int, bool) {
	if animated && !w.sawGCE {
		writeMinimalGraphicsControl(w.canvasPixels, out)
	}
	w.sawGCE = false

	return copyGIFImageBlock(data, pos, out, w.profile, &w.frames)
}

// writeMinimalGraphicsControl writes a Graphics Control extension
// carrying no transparency or disposal of its own, only
// gifMinDelayCS's own minimum delay for canvasPixels, for
// writeGIFBlocks to insert ahead of an animated file's image block
// that arrived with no Graphics Control extension of its own.
func writeMinimalGraphicsControl(canvasPixels int64, out *bytes.Buffer) {
	delay := gifMinDelayCS(0, canvasPixels)
	out.Write([]byte{0x21, 0xF9, 4, 0, byte(delay), byte(delay >> 8), 0, 0})
}

// copyGIFExtensionBlock copies the extension block at pos to out when
// gifExtension says to keep it, dropping it otherwise, and returns
// the position right after it. In an animated file, a Graphics
// Control extension is never copied verbatim: writeGraphicsControl
// rebuilds it with its own delay time raised to gifMinDelayCS's
// bound, so a sender's own value, including zero, can never reach
// AnimatedImage unchanged.
func copyGIFExtensionBlock(data []byte, pos int, out *bytes.Buffer, canvasPixels int64, animated bool) (int, bool) {
	end, keep, ok := gifExtension(data, pos)
	if !ok {
		return 0, false
	}
	if !keep {
		return end, true
	}

	if animated && data[pos+1] == 0xF9 {
		writeGraphicsControl(data, pos, canvasPixels, out)
		return end, true
	}

	out.Write(data[pos:end])

	return end, true
}

// writeGraphicsControl copies the Graphics Control extension at pos
// to out, with its own delay time raised to gifMinDelayCS's bound for
// canvasPixels. gifExtension has already confirmed this block parses,
// so gifSubBlocks cannot fail here. A block whose first sub-block is
// shorter than the 4 bytes a well-formed Graphics Control extension
// always has (packed fields, a 2-byte delay, a transparent colour
// index) is copied unchanged instead: it carries no delay to raise,
// and GIF's own trailing fields may still be extended by a future
// version.
func writeGraphicsControl(data []byte, pos int, canvasPixels int64, out *bytes.Buffer) {
	_, blocks, _ := gifSubBlocks(data, pos+2)
	out.Write(data[pos : pos+2]) // introducer and the 0xF9 label

	if len(blocks) == 0 || len(blocks[0]) < 3 {
		for _, b := range blocks {
			out.WriteByte(byte(len(b)))
			out.Write(b)
		}
		out.WriteByte(0)

		return
	}

	block := bytes.Clone(blocks[0])
	delay := gifMinDelayCS(leUint16(block[1:3]), canvasPixels)
	block[1], block[2] = byte(delay), byte(delay>>8)

	out.WriteByte(byte(len(block)))
	out.Write(block)
	for _, b := range blocks[1:] {
		out.WriteByte(byte(len(b)))
		out.Write(b)
	}
	out.WriteByte(0)
}

// copyGIFImageBlock copies the image block at pos to out, counts it
// against frames, and returns the position right after it. ok is
// false once frames passes gifMaxFrames, the same as any other
// structural failure.
func copyGIFImageBlock(data []byte, pos int, out *bytes.Buffer, profile Profile, frames *int) (int, bool) {
	end, ok := gifImage(data, pos, profile)
	if !ok {
		return 0, false
	}

	out.Write(data[pos:end])
	*frames++

	return end, *frames <= gifMaxFrames
}

// gifExtension reads the extension block at pos (introducer, label
// and its size-prefixed sub-blocks) and says whether it is one
// writeGIFBlocks should keep: every Graphics Control extension, since
// a frame's delay and transparency need it, and only the Netscape
// looping Application extension, never a Comment, a Plain Text block
// or any other Application extension, which carry nothing rendering
// needs and are exactly where a GIF can smuggle arbitrary text (an
// embedded XMP packet, for one, travels as an Application extension).
func gifExtension(data []byte, pos int) (end int, keep, ok bool) {
	if pos+2 > len(data) {
		return 0, false, false
	}

	label := data[pos+1]
	end, blocks, ok := gifSubBlocks(data, pos+2)
	if !ok {
		return 0, false, false
	}

	switch label {
	case 0xF9:
		return end, true, true
	case 0xFF:
		return end, isNetscapeLoop(blocks), true
	default:
		return end, false, true
	}
}

// isNetscapeLoop reports whether an Application extension's first
// sub-block is the Netscape "NETSCAPE2.0" identifier and
// authentication code that carries a GIF's loop count.
func isNetscapeLoop(blocks [][]byte) bool {
	return len(blocks) > 0 && string(blocks[0]) == "NETSCAPE2.0"
}

// gifImage reads the image block at pos - its descriptor, local
// color table if its packed field has one, and its LZW-compressed,
// sub-block-framed data - checks its own width and height against
// profile, and returns the position right after it.
func gifImage(data []byte, pos int, profile Profile) (end int, ok bool) {
	if pos+10 > len(data) {
		return 0, false
	}

	width := int64(leUint16(data[pos+5 : pos+7]))
	height := int64(leUint16(data[pos+7 : pos+9]))
	if !withinCaps(width, height, profile) {
		return 0, false
	}

	next := pos + 10
	if packed := data[pos+9]; packed&0x80 != 0 {
		size := 3 * (1 << ((packed & 0x07) + 1))
		if next+size > len(data) {
			return 0, false
		}
		next += size
	}

	if next >= len(data) {
		return 0, false
	}
	end, _, ok = gifSubBlocks(data, next+1) // skip the LZW minimum code size byte
	return end, ok
}

// gifSubBlocks reads the size-prefixed data blocks starting at pos,
// every GIF extension's and image's own data framing, until the
// zero-length block that ends them, and returns the position right
// after that terminator.
func gifSubBlocks(data []byte, pos int) (end int, blocks [][]byte, ok bool) {
	for {
		if pos >= len(data) {
			return 0, nil, false
		}

		size := int(data[pos])
		pos++
		if size == 0 {
			return pos, blocks, true
		}
		if pos+size > len(data) {
			return 0, nil, false
		}

		blocks = append(blocks, data[pos:pos+size])
		pos += size
	}
}

// leUint16 reads a little-endian 16-bit field, the byte order every
// numeric field in a GIF uses.
func leUint16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

// finishGIF confirms out still decodes as a GIF before trusting it as
// sanitizeGIF's result.
func finishGIF(out []byte) ([]byte, bool) {
	if _, _, err := image.DecodeConfig(bytes.NewReader(out)); err != nil {
		return nil, false
	}

	return out, true
}
