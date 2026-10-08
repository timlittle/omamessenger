package fake

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)

// Fixed size for the procedural stand-in photo every scripted message
// with a photo downloads as: close to the in-app photo viewer's own
// size, so a recording shows the real thing rather than a small image
// stretched blocky across the window.
const (
	photoWidth  = 1280
	photoHeight = 800
	horizonY    = photoHeight / 2

	// scale converts the artwork's tuning (amplitude, radius) from the
	// 480-wide draft it was designed at to photoWidth, so a higher
	// resolution still draws the same picture, just sharper rather than
	// busier. The handful of pixel offsets below that need a whole
	// number (a constant float-to-int conversion must be exact) are
	// scaled by hand instead: horizonY-10 becomes horizonY-27, +15
	// becomes +40, +5 becomes +13, and the ripple spacing of 7 becomes 19.
	scale = float64(photoWidth) / 480.0
)

// placeholderPhoto renders a small landscape JPEG: a sunset sky, a sun,
// three layered mountain silhouettes and a lake reflecting them. It is a
// stand-in for a real downloaded photo, not a copy of one, generated
// fresh each time rather than shipped as a committed image file; a flat
// colour would decode fine but still read as a broken placeholder in a
// screenshot or a demo recording.
func placeholderPhoto() []byte {
	img := image.NewRGBA(image.Rect(0, 0, photoWidth, photoHeight))
	paintSky(img)
	paintSun(img)
	paintMountains(img)
	paintLakeReflection(img)

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}) // encoding a fixed in-memory image never fails
	return buf.Bytes()
}

// paintSky fills the top half with a sunset gradient, deep blue at the
// top fading to warm orange toward the horizon.
func paintSky(img *image.RGBA) {
	top := color.RGBA{R: 40, G: 60, B: 110, A: 255}
	horizon := color.RGBA{R: 250, G: 170, B: 110, A: 255}

	for y := range horizonY {
		c := lerpColor(top, horizon, float64(y)/float64(horizonY))
		for x := range photoWidth {
			img.SetRGBA(x, y, c)
		}
	}
}

// paintSun blends a soft-edged bright disc into the sky, low and to the
// right, the way a setting sun sits just above a horizon.
func paintSun(img *image.RGBA) {
	cx, cy, radius := float64(photoWidth)*0.72, float64(horizonY)*0.92, 42.0*scale
	sun := color.RGBA{R: 255, G: 225, B: 150, A: 255}

	for y := range horizonY + 27 {
		for x := range photoWidth {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d > radius*1.6 {
				continue
			}
			t := math.Min(1, math.Max(0, 1-d/radius)*1.3)
			img.SetRGBA(x, y, lerpColor(img.RGBAAt(x, y), sun, t))
		}
	}
}

// paintMountains draws three jagged ridgelines, each closer and darker
// than the last, their peaks traced with a couple of combined sine waves
// rather than straight lines.
func paintMountains(img *image.RGBA) {
	layers := []struct {
		fill                   color.RGBA
		baseY                  int
		amplitude, freq, phase float64
	}{
		{color.RGBA{R: 80, G: 100, B: 130, A: 255}, horizonY - 27, 18 * scale, 0.018 / scale, 0.6},
		{color.RGBA{R: 55, G: 75, B: 100, A: 255}, horizonY, 28 * scale, 0.026 / scale, 2.1},
		{color.RGBA{R: 30, G: 45, B: 65, A: 255}, horizonY + 40, 36 * scale, 0.034 / scale, 4.0},
	}

	for _, l := range layers {
		for x := range photoWidth {
			wave := math.Sin(float64(x)*l.freq+l.phase) + math.Sin(float64(x)*l.freq*2.3+l.phase*1.7)
			peak := l.baseY - int(l.amplitude*wave/2)
			for y := peak; y < horizonY+13; y++ {
				img.SetRGBA(x, y, l.fill)
			}
		}
	}
}

// paintLakeReflection fills the bottom half with a dimmed, water-tinted
// mirror of the sky and mountains above it, plus a few lighter streaks
// for ripples.
func paintLakeReflection(img *image.RGBA) {
	water := color.RGBA{R: 50, G: 70, B: 90, A: 255}
	ripple := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	for y := horizonY; y < photoHeight; y++ {
		srcY := max(2*horizonY-y-1, 0)
		for x := range photoWidth {
			reflected := lerpColor(img.RGBAAt(x, srcY), water, 0.45)
			if y%19 == 0 {
				reflected = lerpColor(reflected, ripple, 0.12)
			}
			img.SetRGBA(x, y, reflected)
		}
	}
}

// lerpColor blends from a to b by t, where 0 is a and 1 is b.
func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: lerpByte(a.R, b.R, t),
		G: lerpByte(a.G, b.G, t),
		B: lerpByte(a.B, b.B, t),
		A: 255,
	}
}

// lerpByte blends a single colour channel from a to b by t.
func lerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}
