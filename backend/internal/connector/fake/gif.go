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
	"sort"
)

// Fixed size and frame count for the procedural stand-in GIF every
// scripted message with a real .gif document downloads as: 320x200
// matches the message bubble's own maxWidth.
const (
	gifWidth   = 320
	gifHeight  = 200
	gifFrames  = 24
	gifHorizon = gifHeight / 2

	// gifScale converts the photo's tuning, drafted 480 pixels wide (see
	// photo.go's scale), to this width, so the GIF draws the same scene.
	gifScale = float64(gifWidth) / 480.0
)

// placeholderGIF renders a genuine animated GIF: the same sunset over a
// mountain lake as placeholderPhoto, with the reflection rippling and
// sunlight glinting on the water in a seamless loop. A few flat bands
// read as a rendering glitch in a demo recording, so each frame is
// painted in full colour and dithered onto one shared 256-colour
// palette. It is a stand-in for a real GIF document a sender attached,
// not a video a service marked to play back like one, so this is the
// one scripted media FetchMedia ever writes whose bytes are an actual
// GIF header; it reaches the UI the same way any other "GIF video"
// does, played through the sandboxed transcoder's Animation profile
// (see docs/decisions.md). Generated fresh each run, the same
// reasoning placeholderPhoto gives for not shipping a committed image
// file.
func placeholderGIF() []byte {
	scenes := make([]*image.RGBA, gifFrames)
	for i := range gifFrames {
		scenes[i] = gifScene(2 * math.Pi * float64(i) / gifFrames)
	}
	palette := gifPalette(scenes[0])

	g := &gif.GIF{}
	for _, scene := range scenes {
		frame := image.NewPaletted(scene.Bounds(), palette)
		draw.FloydSteinberg.Draw(frame, frame.Bounds(), scene, image.Point{})
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 8)
	}

	var buf bytes.Buffer
	// gif.EncodeAll only fails for an empty GIF or a frame whose palette
	// exceeds 256 colours, neither of which the fixed loop above can
	// ever produce.
	if err := gif.EncodeAll(&buf, g); err != nil {
		panic("fake: placeholderGIF failed to encode: " + err.Error())
	}

	return buf.Bytes()
}

// gifThumb renders the scene placeholderGIF animates, at its first
// frame, as a JPEG, and returns it already base64-encoded: the still
// preview an unplayed GIF bubble shows (see domain.Media.Thumb), the
// same shape a real Telegram or WhatsApp GIF document's own thumbnail
// arrives in. Unlike a sender-supplied thumbnail, these bytes are
// entirely this package's own, so there is nothing here for imagesafe to
// sanitize away.
func gifThumb() string {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, gifScene(0), &jpeg.Options{Quality: 85}) // encoding a fixed in-memory image never fails
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// gifScene paints one full-colour frame at phase (radians): the photo's
// sky, sun and mountains, then water whose ripples and glints move with
// phase. Every moving term repeats over 2π, so the loop has no seam.
func gifScene(phase float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, gifWidth, gifHeight))
	paintGifSky(img)
	paintGifSun(img)
	paintGifMountains(img)
	paintGifWater(img, phase)
	return img
}

// paintGifSky fills the top half with placeholderPhoto's own sunset
// gradient, deep blue fading to warm orange at the horizon.
func paintGifSky(img *image.RGBA) {
	top := color.RGBA{R: 40, G: 60, B: 110, A: 255}
	horizon := color.RGBA{R: 250, G: 170, B: 110, A: 255}

	for y := range gifHorizon {
		c := lerpColor(top, horizon, float64(y)/gifHorizon)
		draw.Draw(img, image.Rect(0, y, gifWidth, y+1), &image.Uniform{C: c}, image.Point{}, draw.Src)
	}
}

// paintGifSun blends a soft-edged disc low on the right, where the
// photo's own setting sun sits.
func paintGifSun(img *image.RGBA) {
	cx, cy, radius := gifWidth*0.72, gifHorizon*0.92, 42.0*gifScale
	sun := color.RGBA{R: 255, G: 225, B: 150, A: 255}

	for y := range gifHorizon {
		for x := range gifWidth {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			t := math.Min(1, math.Max(0, 1-d/radius)*1.3)
			img.SetRGBA(x, y, lerpColor(img.RGBAAt(x, y), sun, t))
		}
	}
}

// paintGifMountains draws the photo's three ridgelines, each closer and
// darker than the last.
func paintGifMountains(img *image.RGBA) {
	layers := []struct {
		fill                   color.RGBA
		baseY                  float64
		amplitude, freq, phase float64
	}{
		{color.RGBA{R: 80, G: 100, B: 130, A: 255}, gifHorizon - 10*gifScale, 18 * gifScale, 0.018 / gifScale, 0.6},
		{color.RGBA{R: 55, G: 75, B: 100, A: 255}, gifHorizon, 28 * gifScale, 0.026 / gifScale, 2.1},
		{color.RGBA{R: 30, G: 45, B: 65, A: 255}, gifHorizon + 15*gifScale, 36 * gifScale, 0.034 / gifScale, 4.0},
	}

	for _, l := range layers {
		for x := range gifWidth {
			wave := math.Sin(float64(x)*l.freq+l.phase) + math.Sin(float64(x)*l.freq*2.3+l.phase*1.7)
			peak := int(l.baseY - l.amplitude*wave/2)
			draw.Draw(img, image.Rect(x, peak, x+1, gifHorizon+3), &image.Uniform{C: l.fill}, image.Point{}, draw.Src)
		}
	}
}

// paintGifWater fills the bottom half with a tinted mirror of the scene
// above, its rows swaying sideways a little more the nearer they are,
// so the reflection ripples, plus a band of glints under the sun.
func paintGifWater(img *image.RGBA, phase float64) {
	water := color.RGBA{R: 50, G: 70, B: 90, A: 255}

	for y := gifHorizon; y < gifHeight; y++ {
		depth := float64(y-gifHorizon) / float64(gifHeight-gifHorizon)
		srcY := max(2*gifHorizon-y-1, 0)
		sway := int(math.Round((1 + 3*depth) * math.Sin(float64(y)*0.45-2*phase)))
		for x := range gifWidth {
			srcX := min(max(x+sway, 0), gifWidth-1)
			c := lerpColor(img.RGBAAt(srcX, srcY), water, 0.35+0.2*depth)
			img.SetRGBA(x, y, gifGlint(c, x, y, phase))
		}
	}
}

// gifGlint brightens (x, y) where a moving sparkle pattern crosses the
// column of water under the sun, the light catching on the ripples.
func gifGlint(c color.RGBA, x, y int, phase float64) color.RGBA {
	spread := math.Abs(float64(x)-gifWidth*0.72) / (30 * gifScale)
	if spread > 1 {
		return c
	}

	sparkle := math.Sin(float64(x)*0.9+float64(y)*1.7+3*phase) * math.Sin(float64(y)*0.6-phase)
	if sparkle < 0.55 {
		return c
	}
	glint := color.RGBA{R: 255, G: 235, B: 190, A: 255}
	return lerpColor(c, glint, (sparkle-0.55)/0.45*(1-spread))
}

// gifPalette picks the 256 colours the GIF is dithered onto: the most
// common colours in scene, each rounded to 5 bits a channel so a smooth
// gradient's near-identical neighbours count as one.
func gifPalette(scene *image.RGBA) color.Palette {
	counts := map[color.RGBA]int{}
	for i := 0; i < len(scene.Pix); i += 4 {
		c := color.RGBA{R: scene.Pix[i] &^ 7, G: scene.Pix[i+1] &^ 7, B: scene.Pix[i+2] &^ 7, A: 255}
		counts[c]++
	}

	colours := make([]color.RGBA, 0, len(counts))
	for c := range counts {
		colours = append(colours, c)
	}
	sort.Slice(colours, func(i, j int) bool {
		if counts[colours[i]] != counts[colours[j]] {
			return counts[colours[i]] > counts[colours[j]]
		}
		return gifColourKey(colours[i]) < gifColourKey(colours[j])
	})

	palette := make(color.Palette, 0, 256)
	for _, c := range colours[:min(len(colours), 255)] {
		palette = append(palette, c)
	}
	return append(palette, color.RGBA{R: 255, G: 235, B: 190, A: 255})
}

// gifColourKey orders colours with equal counts the same way every run,
// so the palette, and the GIF's bytes, never depend on map order.
func gifColourKey(c color.RGBA) uint32 {
	return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}
