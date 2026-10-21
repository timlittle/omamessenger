// Package videosafe decides whether a downloaded video file's own
// container and stream headers describe something the shell's
// QtMultimedia-backed player may be trusted to decode, before any of
// its bytes ever reach that player. It never shells out to ffprobe
// and never reads a file's actual sample data: only the handful of
// MP4/MOV (ISO base media) boxes or WebM/Matroska (EBML) elements
// that name a video's codec, dimensions, duration, track count and
// audio codec, each read through a single bounded os.File.ReadAt, so
// a crafted header can never make this package read or allocate more
// than a few hundred bytes regardless of how large the file on disk
// actually is.
//
// Validate reports an error only for an actual failure to read the
// file at all (missing, or permission denied); a file whose
// container or headers fail any check - wrong format, an unsupported
// codec, oversized dimensions, too long a duration, too many video
// tracks, an absurd declared sample count, or audio in a codec
// outside the allowed list - is reported simply as not playable, the
// same way imagesafe reports a bad image: this is attacker-controlled
// data, not a program fault. A file with an audio track in an
// unsupported codec is refused outright rather than muted: this
// package only answers "is this safe to decode", never rewrites a
// file's bytes the way imagesafe does, so there is no way to strip or
// mute a track without re-muxing the file, which is out of scope here
// (see docs/decisions.md).
package videosafe

import (
	"fmt"
	"os"
)

// container names which format Validate found at the start of a file,
// or that it recognized neither.
type container int

// The containers Validate understands. Anything else is containerUnknown.
const (
	containerUnknown container = iota
	containerISOBMFF
	containerMatroska
)

// Validate reports whether path is a video this shell may safely
// hand to QtMultimedia: an MP4/MOV container with H.264, VP9 or AV1
// video and, if present, AAC or Opus audio; or a WebM/Matroska
// container with VP8, VP9 or AV1 video and, if present, AAC, Opus or
// Vorbis audio - in either case within the dimension, duration,
// track-count and sample-count limits in limits.go.
func Validate(path string) (playable bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("videosafe: validate: %w", err)
	}
	defer f.Close() // reading only; nothing to flush

	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("videosafe: validate: %w", err)
	}
	if info.Size() < 12 {
		return false, nil
	}

	c := &cursor{f: f, size: info.Size(), boxes: maxBoxBudget}

	switch detectContainer(c) {
	case containerISOBMFF:
		return isoBMFFPlayable(c), nil
	case containerMatroska:
		return matroskaPlayable(c), nil
	default:
		return false, nil
	}
}

// detectContainer recognizes an MP4/MOV file by its first box's own
// four-byte type, and a WebM/Matroska file by its fixed EBML header
// magic, never a sender's file name or mime type.
func detectContainer(c *cursor) container {
	var head [8]byte
	if !c.readAt(head[:], 0) {
		return containerUnknown
	}

	if head[0] == 0x1A && head[1] == 0x45 && head[2] == 0xDF && head[3] == 0xA3 {
		return containerMatroska
	}

	switch string(head[4:8]) {
	case "ftyp", "moov", "free", "skip", "wide", "mdat":
		return containerISOBMFF
	default:
		return containerUnknown
	}
}
