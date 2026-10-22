package fake

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
)

// Fixed size for the procedural stand-in GIF every scripted message with
// a real .gif document downloads as: small enough to animate cheaply, in
// line with imagesafe.Thumbnail's own bound, well inside FullImage's.
const (
	gifSide   = 64
	gifFrames = 6
)

// placeholderGIF renders a tiny, genuine animated GIF: a dot bouncing
// across a plain background. It is a stand-in for a real GIF document a
// sender attached, not a video a service marked to play back like one,
// so this is the one scripted media FetchMedia ever writes whose bytes
// are an actual GIF header - the exact shape app.DecodeAsForMedia's
// SanitizeIfGIF check must confirm before the UI ever animates it (see
// docs/decisions.md). Generated fresh each run, the same reasoning
// placeholderPhoto gives for not shipping a committed image file.
func placeholderGIF() []byte {
	palette := color.Palette{
		color.RGBA{R: 0x1b, G: 0x1f, B: 0x29, A: 0xff}, // background
		color.RGBA{R: 0xf2, G: 0xa5, B: 0x3f, A: 0xff}, // dot
	}

	g := &gif.GIF{}
	for i := range gifFrames {
		frame := image.NewPaletted(image.Rect(0, 0, gifSide, gifSide), palette)
		draw.Draw(frame, frame.Bounds(), &image.Uniform{C: palette[0]}, image.Point{}, draw.Src)
		paintBouncingDot(frame, i)

		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 15)
	}

	var buf bytes.Buffer
	// gif.EncodeAll only fails for an empty GIF or a frame whose palette
	// exceeds 256 colours, neither of which this fixed, two-colour,
	// fixed-length loop above can ever produce.
	if err := gif.EncodeAll(&buf, g); err != nil {
		panic("fake: placeholderGIF failed to encode: " + err.Error())
	}

	return buf.Bytes()
}

// paintBouncingDot draws frame step's own position of a small square
// sweeping left to right and back across the image, so consecutive
// frames visibly differ for a recording or a test to tell apart.
func paintBouncingDot(img *image.Paletted, step int) {
	const dotSize = 10

	span := gifSide - dotSize
	phase := step % (2 * gifFrames)
	x := phase * span / gifFrames
	if phase >= gifFrames {
		x = span - (phase-gifFrames)*span/gifFrames
	}

	dot := image.Rect(x, (gifSide-dotSize)/2, x+dotSize, (gifSide+dotSize)/2)
	draw.Draw(img, dot, &image.Uniform{C: img.Palette[1]}, image.Point{}, draw.Src)
}
