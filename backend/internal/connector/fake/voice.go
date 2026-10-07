package fake

import "encoding/base64"

// placeholderVoiceNoteBase64 is a third of a second of silence, encoded
// as Opus audio in an Ogg container (`ffmpeg -f lavfi -i
// anullsrc=r=16000:cl=mono -t 0.3 -c:a libopus -b:a 6k out.ogg`). It is
// committed as a fixed byte string, unlike placeholderPhoto's own
// procedurally painted image, because this module has no Opus encoder
// of its own to generate one with at runtime; a real voice note is
// never this short, but it is enough for the in-window player to load,
// decode and play, which a borrowed image file renamed to ".ogg" is not.
const placeholderVoiceNoteBase64 = "T2dnUwACAAAAAAAAAACXFUDCAAAAAN6ixxUBE09wdXNIZWFkAQE4AYA+AAAAAABPZ2dTAAAAAAAAAAAAAJcVQMIBAAAAVh113QE8" +
	"T3B1c1RhZ3MMAAAATGF2ZjYzLjEuMTAxAQAAABwAAABlbmNvZGVyPUxhdmM2My4xLjEwMSBsaWJvcHVzT2dnUwAEeDkAAAAAAACX" +
	"FUDCAgAAAG8/ZtYQBwYGBgYGBgYGBgYGBgYGBggL5jsjq2AICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyz" +
	"DsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsY="

// placeholderVoiceNote decodes placeholderVoiceNoteBase64, for
// FetchMedia to write as the stand-in for a scripted voice note.
// Decoding a fixed, tested constant cannot fail outside a bug in this
// file, which base64 syntax alone would already have caught; a panic
// here would mean this constant was edited and broken, not a condition
// a caller could recover from.
func placeholderVoiceNote() []byte {
	data, err := base64.StdEncoding.DecodeString(placeholderVoiceNoteBase64)
	if err != nil {
		panic("fake: placeholderVoiceNoteBase64 does not decode: " + err.Error())
	}

	return data
}
