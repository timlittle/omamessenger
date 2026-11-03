package domain_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestCapDisplayName_StripsControlCharacters confirms an ASCII control
// character, including a newline, never survives in a display name: a
// name could otherwise carry one to break a single-line layout or hide
// part of itself.
func TestCapDisplayName_StripsControlCharacters(t *testing.T) {
	t.Parallel()

	got := domain.CapDisplayName("Mum\n\t\x00\x7f Impersonator")
	if got != "Mum Impersonator" {
		t.Errorf("CapDisplayName(%q) = %q, want control characters stripped", "Mum\\n\\t\\x00\\x7f Impersonator", got)
	}
}

// rightToLeftOverride and zeroWidthSpace are built from their own rune
// values, rather than written as literal characters or \u escapes in
// this file's own source: the two bidi and invisible-format characters
// these tests exist to confirm get stripped, spelled out any other way
// in a Go string literal, so there is nothing here for a reader - or
// an editor - to mistake for ordinary text.
var (
	rightToLeftOverride = string(rune(0x202E))
	zeroWidthSpace      = string(rune(0x200B))
	zeroWidthJoiner     = string(rune(0x200D))
)

// TestCapDisplayName_StripsBidiAndFormatCharacters confirms the
// Unicode bidi override and invisible-format characters a self-chosen
// name could use to reorder or hide part of itself are stripped: the
// right-to-left override classically used to disguise a file
// extension, and the zero-width space.
func TestCapDisplayName_StripsBidiAndFormatCharacters(t *testing.T) {
	t.Parallel()

	got := domain.CapDisplayName("Mum" + rightToLeftOverride + zeroWidthSpace + "evil")
	if got != "Mumevil" {
		t.Errorf("CapDisplayName(%q) = %q, want bidi and format characters stripped", "Mum<RLO><ZWSP>evil", got)
	}
}

// TestCapDisplayName_KeepsZeroWidthJoiner confirms the zero-width
// joiner survives CapDisplayName, even though it sits inside the
// stripped range this cap otherwise removes: a real emoji sequence,
// such as a family or a profession, is several code points joined by
// it, and stripping it would break the glyph apart.
func TestCapDisplayName_KeepsZeroWidthJoiner(t *testing.T) {
	t.Parallel()

	family := "a" + zeroWidthJoiner + "b" + zeroWidthJoiner + "c"
	if got := domain.CapDisplayName(family); got != family {
		t.Errorf("CapDisplayName(%q) = %q, want the zero-width joiner kept", family, got)
	}
}

// TestCapFileName_StripsBidiAndFormatCharacters confirms a file name
// shown in the UI gets the same bidi and control stripping a display
// name does: a right-to-left override could otherwise make a
// dangerous extension read as a safe one.
func TestCapFileName_StripsBidiAndFormatCharacters(t *testing.T) {
	t.Parallel()

	got := domain.CapFileName("invoice" + rightToLeftOverride + "gnp.exe")
	if got != "invoicegnp.exe" {
		t.Errorf("CapFileName(%q) = %q, want the right-to-left override stripped", "invoice<RLO>gnp.exe", got)
	}
}
