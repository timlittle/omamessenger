// Package audiosafe decides whether a downloaded audio file's own
// bytes describe something the shell's QtMultimedia-backed player may
// be trusted to decode, before any of its bytes ever reach that
// player - the same discipline videosafe already applies to video,
// extended to audio because a voice note or a shared song used to
// reach MediaPlayer with nothing checking its codec, stream count or
// duration first (see docs/decisions.md).
//
// Validate reads only the handful of container or frame headers each
// supported format needs, through bounded, absolute-offset reads
// (see cursor), and never shells out or decodes a single audio
// sample. Three formats are accepted: Ogg carrying a single Opus
// stream (identified by its own "OpusHead" packet, never a sender's
// file name or mime type), MP4/M4A carrying a single AAC or Opus
// audio track and no video track, and MPEG-1 Layer III (MP3) in the
// simple, constant-bitrate case a frame header's own fields can
// confirm without walking variable-length frames. Anything else -
// including a format this package recognizes but cannot validate
// simply, such as a free-bitrate or multiplexed-stream file - is
// reported not playable, the same way videosafe and imagesafe report
// bad input: this is attacker-controlled data, not a program fault,
// so Validate returns an error only when path itself cannot be read.
package audiosafe

import (
	"bytes"
	"fmt"
	"os"
)

// Validate reports whether path is an audio file this shell may
// safely hand to QtMultimedia: Ogg/Opus, MP4/M4A with AAC or Opus and
// no video track, or a simple constant-bitrate MPEG-1 Layer III
// (MP3) stream - in every case, a single audio stream, no video
// stream, and a duration of at most 30 minutes.
func Validate(path string) (playable bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("audiosafe: validate: %w", err)
	}
	defer f.Close() // reading only; nothing to flush

	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("audiosafe: validate: %w", err)
	}
	if info.Size() < 4 {
		return false, nil
	}

	c := &cursor{f: f, size: info.Size(), boxes: maxBoxBudget}

	var head [12]byte
	if !c.readAt(head[:4], 0) {
		return false, nil
	}

	switch {
	case bytes.Equal(head[:4], []byte("OggS")):
		return validateOgg(c), nil
	case c.readAt(head[:], 0) && isISOBMFF(head[:]):
		return validateISOBMFFAudio(c), nil
	default:
		return validateMP3(c), nil
	}
}

// isISOBMFF reports whether head, the first 12 bytes of a file,
// opens with an MP4/MOV box this package recognizes as the start of
// an ISO base media file - the same four-character types videosafe's
// own detectContainer accepts, read from the file's own bytes, never
// a sender's claimed name or mime type.
func isISOBMFF(head []byte) bool {
	switch string(head[4:8]) {
	case "ftyp", "moov", "free", "skip", "wide", "mdat":
		return true
	default:
		return false
	}
}
