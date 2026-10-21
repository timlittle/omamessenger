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

// sanitizeGIF rebuilds data's block stream, dropping its Comment and
// Plain Text extensions outright and any Application extension except
// the Netscape looping one AnimatedImage needs, after checking the
// logical screen's and every frame's own dimensions against profile.
// ok is false for anything structurally wrong or over profile's caps.
func sanitizeGIF(data []byte, profile Profile) ([]byte, bool) {
	if len(data) < 13 {
		return nil, false
	}

	width, height := int64(leUint16(data[6:8])), int64(leUint16(data[8:10]))
	if !withinCaps(width, height, profile) {
		return nil, false
	}

	var out bytes.Buffer
	out.Write(data[0:6]) // "GIF87a" or "GIF89a"
	pos, ok := copyGIFScreen(data, &out)
	if !ok {
		return nil, false
	}

	return walkGIFBlocks(data, pos, &out, profile)
}

// copyGIFScreen copies the logical screen descriptor and, when the
// packed field's flag bit says there is one, the global color table
// right after it, and returns the position right after both.
func copyGIFScreen(data []byte, out *bytes.Buffer) (pos int, ok bool) {
	out.Write(data[6:13])
	packed := data[10]
	pos = 13

	if packed&0x80 == 0 {
		return pos, true
	}

	size := 3 * (1 << ((packed & 0x07) + 1))
	if pos+size > len(data) {
		return 0, false
	}
	out.Write(data[pos : pos+size])

	return pos + size, true
}

// walkGIFBlocks copies every extension and image block from pos to
// the trailer, dropping the ones gifExtension says to, until
// gifMaxFrames image blocks or the trailer, whichever comes first.
func walkGIFBlocks(data []byte, pos int, out *bytes.Buffer, profile Profile) ([]byte, bool) {
	frames := 0

	for pos < len(data) {
		switch data[pos] {
		case 0x3B:
			out.WriteByte(0x3B)
			return finishGIF(out.Bytes())
		case 0x21:
			next, ok := copyGIFExtensionBlock(data, pos, out)
			if !ok {
				return nil, false
			}
			pos = next
		case 0x2C:
			next, ok := copyGIFImageBlock(data, pos, out, profile, &frames)
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

// copyGIFExtensionBlock copies the extension block at pos to out when
// gifExtension says to keep it, dropping it otherwise, and returns
// the position right after it.
func copyGIFExtensionBlock(data []byte, pos int, out *bytes.Buffer) (int, bool) {
	end, keep, ok := gifExtension(data, pos)
	if !ok {
		return 0, false
	}
	if keep {
		out.Write(data[pos:end])
	}

	return end, true
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
// walkGIFBlocks should keep: every Graphics Control extension, since
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
