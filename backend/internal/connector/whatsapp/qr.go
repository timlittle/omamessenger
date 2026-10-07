package whatsapp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"rsc.io/qr"
)

// QR image layout: each square of the code is qrScale pixels, inside the
// white margin of qrMargin squares that scanners need.
const (
	qrScale  = 8
	qrMargin = 4
)

// qrPNG draws a WhatsApp pairing code as a base64 PNG. It draws the code
// itself because rsc.io/qr's own image ignores its scale and margin.
func qrPNG(data string) (string, error) {
	code, err := qr.Encode(data, qr.M)
	if err != nil {
		return "", fmt.Errorf("whatsapp: draw QR code: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, drawQR(code)); err != nil {
		return "", fmt.Errorf("whatsapp: draw QR code: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// drawQR draws a code black on white, scaled, with its margin.
func drawQR(code *qr.Code) *image.Gray {
	side := (code.Size + 2*qrMargin) * qrScale
	img := image.NewGray(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			if !code.Black(x/qrScale-qrMargin, y/qrScale-qrMargin) {
				img.SetGray(x, y, color.Gray{Y: 0xff})
			}
		}
	}

	return img
}
