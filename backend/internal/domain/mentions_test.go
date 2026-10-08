package domain_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestUTF16Len(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		text string
		want int
	}{
		"empty":    {"", 0},
		"ascii":    {"hello", 5},
		"emoji":    {"👍", 2}, // outside the BMP: a surrogate pair
		"mixed":    {"hi 👍 there", 11},
		"accented": {"café", 4}, // within the BMP: one unit per rune
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := domain.UTF16Len(tc.text); got != tc.want {
				t.Errorf("UTF16Len(%q) = %d, want %d", tc.text, got, tc.want)
			}
		})
	}
}

func TestByteOffsetsForUTF16(t *testing.T) {
	t.Parallel()

	t.Run("plain ASCII range", func(t *testing.T) {
		t.Parallel()

		start, end, ok := domain.ByteOffsetsForUTF16("hi @Alice how are you", 3, 6)
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if got := "hi @Alice how are you"[start:end]; got != "@Alice" {
			t.Errorf("slice = %q, want %q", got, "@Alice")
		}
	})

	t.Run("range after a surrogate pair", func(t *testing.T) {
		t.Parallel()

		s := "👍 @Bob"
		start, end, ok := domain.ByteOffsetsForUTF16(s, 3, 4)
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if got := s[start:end]; got != "@Bob" {
			t.Errorf("slice = %q, want %q", got, "@Bob")
		}
	})

	t.Run("range at the very end", func(t *testing.T) {
		t.Parallel()

		s := "hello"
		start, end, ok := domain.ByteOffsetsForUTF16(s, 5, 0)
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if start != 5 || end != 5 {
			t.Errorf("start, end = %d, %d, want 5, 5", start, end)
		}
	})

	t.Run("range past the end is rejected", func(t *testing.T) {
		t.Parallel()

		if _, _, ok := domain.ByteOffsetsForUTF16("hi", 1, 5); ok {
			t.Error("ok = true, want false")
		}
	})

	t.Run("negative offset is rejected", func(t *testing.T) {
		t.Parallel()

		if _, _, ok := domain.ByteOffsetsForUTF16("hi", -1, 1); ok {
			t.Error("ok = true, want false")
		}
	})
}
