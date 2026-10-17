package fake

import "encoding/base64"

// placeholderVoiceNoteBase64 is three seconds of silence, encoded as
// Opus audio in an Ogg container (`ffmpeg -f lavfi -i
// anullsrc=r=16000:cl=mono -t 3 -c:a libopus -b:a 6k out.ogg`). It is
// committed as a fixed byte string, unlike placeholderPhoto's own
// procedurally painted image, because this module has no Opus encoder
// of its own to generate one with at runtime. This used to be a third
// of a second, which let tests/qml/VoicePlayback observe the in-window
// player genuinely reach EndOfMedia and flip itself back to paused
// between one poll and the next: a scheduling delay anywhere in the
// download, decode or poll loop could make the test see a finished clip
// instead of a playing one, intermittently failing in CI. Three seconds
// gives that test a wide margin to observe mid-playback regardless of
// how long the machine running it takes to get there.
const placeholderVoiceNoteBase64 = "T2dnUwACAAAAAAAAAAAGybcGAAAAAB1Q1cABE09wdXNIZWFkAQE4AYA+AAAAAABPZ2dTAAAAAAAAAAAAAAbJtwYBAAAAUMO0iwE8" +
	"T3B1c1RhZ3MMAAAATGF2ZjYzLjEuMTAxAQAAABwAAABlbmNvZGVyPUxhdmM2My4xLjEwMSBsaWJvcHVzT2dnUwAAgLsAAAAAAAAG" +
	"ybcGAgAAALTAx60yBwYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYIC+Y7I6tgCAissw7G" +
	"CAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAis" +
	"sw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7G" +
	"CAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAis" +
	"sw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GCAissw7GT2dnUwAAAHcBAAAA" +
	"AAAGybcGAwAAAJ1X86MyBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYICKyzDsYICKyz" +
	"DsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYI" +
	"CKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyz" +
	"DsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYI" +
	"CKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsYICKyzDsZPZ2dTAACAMgIA" +
	"AAAAAAbJtwYEAAAAtZfgFjIGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBggIrLMOxggI" +
	"rLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMO" +
	"xggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggI" +
	"rLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMO" +
	"xggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxggIrLMOxk9nZ1MABLgz" +
	"AgAAAAAABsm3BgUAAABFk3wTAQYICKyzDsY="

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
