package telegram

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/color"
	"image/png"
	"testing"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestTranslate_MapsTelegramErrorsToSignInOutcomes(t *testing.T) {
	t.Parallel()

	other := errors.New("network down")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"success", nil, nil},
		{"password needed", auth.ErrPasswordAuthNeeded, errPasswordNeeded},
		{"password needed by code", tgerr.New(401, "SESSION_PASSWORD_NEEDED"), errPasswordNeeded},
		{"wrong code", tgerr.New(400, "PHONE_CODE_INVALID"), errWrongCode},
		{"empty code", tgerr.New(400, "PHONE_CODE_EMPTY"), errWrongCode},
		{"wrong password", auth.ErrPasswordInvalid, errWrongPassword},
		{"anything else", other, other},
	}

	for _, tt := range tests {
		if got := translate(tt.err); !errors.Is(got, tt.want) {
			t.Errorf("%s: translate = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCodeHint_SaysWhereTheCodeWent(t *testing.T) {
	t.Parallel()

	types := []tg.AuthSentCodeTypeClass{
		&tg.AuthSentCodeTypeApp{}, &tg.AuthSentCodeTypeSMS{}, &tg.AuthSentCodeTypeCall{},
		&tg.AuthSentCodeTypeFlashCall{}, &tg.AuthSentCodeTypeEmailCode{},
	}
	seen := map[string]bool{}
	for _, ty := range types {
		seen[codeHint(ty)] = true
	}

	if len(seen) != 4 {
		t.Errorf("got %d distinct hints, want 4 (calls share one)", len(seen))
	}
}

func TestQRPNG_DrawsAPNG(t *testing.T) {
	t.Parallel()

	encoded, err := qrPNG(qrlogin.NewToken([]byte("token"), 0))
	if err != nil {
		t.Fatal(err)
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}

	// A scanner needs a white margin round the code, and the code drawn
	// large: the top-left finder square starts dark just inside the margin.
	inside := qrMargin*qrScale + 1
	if dark(img.At(1, 1)) || !dark(img.At(inside, inside)) {
		t.Error("QR code is not drawn scaled inside a white margin")
	}

	if size := img.Bounds().Dx(); size < 200 {
		t.Errorf("QR image is %d pixels wide, too small to scan", size)
	}
}

// dark reports whether a pixel is closer to black than white.
func dark(c color.Color) bool {
	r, _, _, _ := c.RGBA()

	return r < 0x8000
}
