package fake

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"math"
)

// Fixed size and frame count for the procedural stand-in GIF every
// scripted message with a real .gif document downloads as: 320x200
// matches the message bubble's own maxWidth, and comfortably inside
// imagesafe.AnimatedGIF's own per-frame and total-pixel bounds
// (4,200,000 and 400,000,000 respectively).
const (
	gifWidth   = 320
	gifHeight  = 200
	gifFrames  = 20
	gifHorizon = gifHeight * 3 / 5

	gifSunCoreRadius = 13
	gifSunHaloRadius = 22
)

// gifSkyTop, gifSkyMid and so on index gifPalette's own entries by what
// they paint, so the drawing functions below read as names rather than
// raw numbers.
const (
	gifSkyTop = iota
	gifSkyMid
	gifSkyHorizon
	gifSunCore
	gifSunHalo
	gifWaterMid
	gifWaterDeep
	gifRipple
)

// placeholderGIF renders a tiny, genuine animated GIF: a sun low over
// calm water, its ripples sliding past, in the demo theme's own
// Catppuccin colours (see tests/demo/theme/colors.toml). It replaces
// what this fixture used to be, a dot bouncing on a near-black square,
// which read as broken rather than merely a placeholder once it showed
// up in a promo recording. It is a stand-in for a real GIF document a
// sender attached, not a video a service marked to play back like one,
// so this is the one scripted media FetchMedia ever writes whose bytes
// are an actual GIF header - the exact shape app.DecodeAsForMedia's
// SanitizeIfGIF check must confirm before the UI ever animates it (see
// docs/decisions.md). Generated fresh each run, the same reasoning
// placeholderPhoto gives for not shipping a committed image file.
func placeholderGIF() []byte {
	palette := gifPalette()

	g := &gif.GIF{}
	for i := range gifFrames {
		frame := image.NewPaletted(image.Rect(0, 0, gifWidth, gifHeight), palette)
		paintGifFrame(frame, i)
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 10)
	}

	var buf bytes.Buffer
	// gif.EncodeAll only fails for an empty GIF or a frame whose palette
	// exceeds 256 colours, neither of which this fixed, small,
	// fixed-length loop above can ever produce.
	if err := gif.EncodeAll(&buf, g); err != nil {
		panic("fake: placeholderGIF failed to encode: " + err.Error())
	}

	return buf.Bytes()
}

// gifThumb renders the same scene placeholderGIF animates, at its very
// first frame, as a small JPEG, and returns it already base64-encoded:
// the still preview an unplayed GIF bubble shows (see
// domain.Media.Thumb), the same shape a real Telegram or WhatsApp GIF
// document's own thumbnail arrives in. Unlike a sender-supplied
// thumbnail, these bytes are entirely this package's own, so there is
// nothing here for imagesafe to sanitize away.
func gifThumb() string {
	frame := image.NewPaletted(image.Rect(0, 0, gifWidth, gifHeight), gifPalette())
	paintGifFrame(frame, 0)

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, frame, &jpeg.Options{Quality: 85}) // encoding a fixed in-memory image never fails
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// gifPalette is the fixed set of colours placeholderGIF and gifThumb
// paint with, taken from the demo theme's own Catppuccin palette: a
// night sky fading toward the horizon, a warm sun, and the water's
// darker mirror plus a highlight for its ripples.
func gifPalette() color.Palette {
	return color.Palette{
		color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff}, // gifSkyTop
		color.RGBA{R: 0x31, G: 0x32, B: 0x44, A: 0xff}, // gifSkyMid
		color.RGBA{R: 0x45, G: 0x47, B: 0x5a, A: 0xff}, // gifSkyHorizon
		color.RGBA{R: 0xf9, G: 0xe2, B: 0xaf, A: 0xff}, // gifSunCore
		color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff}, // gifSunHalo
		color.RGBA{R: 0x24, G: 0x25, B: 0x34, A: 0xff}, // gifWaterMid
		color.RGBA{R: 0x10, G: 0x10, B: 0x19, A: 0xff}, // gifWaterDeep
		color.RGBA{R: 0xcd, G: 0xd6, B: 0xf4, A: 0xff}, // gifRipple
	}
}

// paintGifFrame draws frame step's own moment in the loop: a static sky
// and sun, with the water's ripple highlights sliding down to suggest a
// gentle, endless swell.
func paintGifFrame(img *image.Paletted, step int) {
	paintGifSky(img)
	paintGifSun(img)
	paintGifWater(img, step)
}

// paintGifSky fills the sky in three flat bands, darkest at the top and
// lightest at the horizon: a paletted image has no room for a smooth
// gradient's many shades, so a few flat steps stand in for one.
func paintGifSky(img *image.Paletted) {
	drawBand(img, 0, gifHorizon/3, gifSkyTop)
	drawBand(img, gifHorizon/3, gifHorizon*2/3, gifSkyMid)
	drawBand(img, gifHorizon*2/3, gifHorizon, gifSkyHorizon)
}

// paintGifSun draws a fixed, two-tone disc low in the sky: a bright
// core ringed by a softer halo, standing in for the many shades a full
// gradient would need.
func paintGifSun(img *image.Paletted) {
	cx, cy := gifWidth*7/10, gifHorizon*9/10

	for y := cy - gifSunHaloRadius; y <= cy+gifSunHaloRadius; y++ {
		for x := cx - gifSunHaloRadius; x <= cx+gifSunHaloRadius; x++ {
			paintGifSunPixel(img, x, y, cx, cy)
		}
	}
}

// paintGifSunPixel sets (x, y) to the sun's core or halo colour when it
// falls within the matching radius of (cx, cy), split out of
// paintGifSun to keep that function's own loop body within the house
// style's indentation limit.
func paintGifSunPixel(img *image.Paletted, x, y, cx, cy int) {
	if !inBounds(img, x, y) {
		return
	}

	d := dist(x, y, cx, cy)
	if d <= gifSunCoreRadius {
		img.SetColorIndex(x, y, gifSunCore)
		return
	}
	if d <= gifSunHaloRadius {
		img.SetColorIndex(x, y, gifSunHalo)
	}
}

// paintGifWater fills the bottom of the frame with the water's own
// darker mirror of the sky, then draws three ripple highlights that
// slide down the frame as step advances and wrap back to the top, the
// one thing that visibly differs between consecutive frames.
func paintGifWater(img *image.Paletted, step int) {
	mid := gifHorizon + (gifHeight-gifHorizon)/2
	drawBand(img, gifHorizon, mid, gifWaterMid)
	drawBand(img, mid, gifHeight, gifWaterDeep)

	span := gifHeight - gifHorizon
	for i := range 3 {
		y := gifHorizon + (step*3+i*span/3)%span
		drawBand(img, y, min(y+2, gifHeight), gifRipple)
	}
}

// drawBand fills img's full width, from y=from up to (not including)
// y=to, with gifPalette's colour at index: the one shape every band of
// sky, water or ripple in this scene is made of.
func drawBand(img *image.Paletted, from, to, index int) {
	draw.Draw(img, image.Rect(0, from, gifWidth, to), &image.Uniform{C: img.Palette[index]}, image.Point{}, draw.Src)
}

// dist returns the straight-line distance between (x, y) and (cx, cy).
func dist(x, y, cx, cy int) float64 {
	return math.Hypot(float64(x-cx), float64(y-cy))
}

// inBounds reports whether (x, y) lies within img's own bounds.
func inBounds(img *image.Paletted, x, y int) bool {
	return image.Pt(x, y).In(img.Bounds())
}
