package transcode

// remux.go rebuilds an MP4 the sandboxed ffmpeg wrote into a brand
// new file this package writes itself, with mp4ff: every box in the
// sandboxed file is read only to extract the handful of values a
// canonical, minimal MP4 needs, each validated before use, and
// nothing it contains - not a byte of its own moov, not a NAL unit
// this package does not itself recognise as video slice data - ever
// crosses into the rebuilt file uninspected. A sandboxed ffmpeg that
// is fully compromised can then still only ever reach Qt's own
// H.264 and AAC decoders, never anything that parses a container,
// because the container itself always one this package wrote. See
// docs/decisions.md for the exact shape this writes and why.

import (
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

// remuxSampleCountSlack gives a profile's own duration-cap-derived
// sample count a little room: fpsmax and a fixed AAC frame rate cap
// the encoder, not the input's own frame timing exactly, so a source
// a hair over the cap can still land a sample or two past the
// nominal count.
const remuxSampleCountSlack = 60

// maxAACFramesPerSecond bounds how many AAC-LC frames per second of
// audio this package expects: generous for both sample rates it
// accepts (1024-sample frames give about 43/sec at 44.1 kHz, 47/sec
// at 48 kHz).
const maxAACFramesPerSecond = 50

// maxRemuxVideoSampleBytes bounds any one video sample this package
// will rebuild: videoRateControlArgs' own VBV bufsize (5000 kbit,
// see args.go) already bounds how much any single frame can burst to
// under the encoder's own rate control, with headroom to spare.
const maxRemuxVideoSampleBytes = 4 << 20

// maxRemuxAudioSampleBytes bounds any one AAC frame a remuxed
// video's own audio track may carry: generous for even the highest
// bitrate this package would ever ask ffmpeg to produce.
const maxRemuxAudioSampleBytes = 32 << 10

// maxMoovBytes bounds a moov box's own declared size, checked
// before mp4.DecodeFile ever reads it: decoding a moov box reads its
// entire payload into memory to decode its own tables (stts, stsz,
// stco and the rest), and mp4ff's own per-table checks bound each
// table to its own box's declared size, never the box itself. A
// crafted intermediate could otherwise make that one read cost
// nearly as much as checkOutput's own whole-file cap allows, before
// any of this package's own structural checks run at all. Measured
// directly: Video's own worst case - ten minutes at 1280x720/30fps
// with 48 kHz AAC, the longest and most densely sampled file this
// package's own ffmpeg arguments ever legitimately produce - has a
// 519,147-byte moov; 2 MiB leaves roughly 4x that as headroom.
const maxMoovBytes = 2 << 20

// maxTopLevelBoxes bounds how many top-level boxes
// checkTopLevelBoxes will walk before refusing: an ordinary
// faststart MP4 this package's own ffmpeg arguments produce has four
// (ftyp, moov, free, mdat). This leaves headroom for minor variation
// without letting a crafted file force an unbounded walk.
const maxTopLevelBoxes = 8

// checkTopLevelBoxes walks f's own top-level boxes from its current
// position to the file's end, reading only each box's own header
// (mp4.DecodeHeader) and seeking past its body without ever reading
// it, so this stays cheap regardless of what a hostile file claims.
// It refuses more than one moov box, a moov box over maxMoovBytes,
// more than maxTopLevelBoxes boxes in total, or any box whose own
// declared size reaches past the file's actual end. f is left
// positioned wherever the walk stopped; openIntermediate rewinds it
// before using it for anything else.
func checkTopLevelBoxes(f io.ReadSeeker) error {
	fileSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("%w: finding file size: %w", ErrFailed, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("%w: rewinding to walk top-level boxes: %w", ErrFailed, err)
	}

	moovCount := 0
	boxes := 0
	for pos := int64(0); pos < fileSize; boxes++ {
		if boxes >= maxTopLevelBoxes {
			return fmt.Errorf("%w: more than %d top-level boxes", ErrFailed, maxTopLevelBoxes)
		}

		header, err := mp4.DecodeHeader(f)
		if err != nil {
			return fmt.Errorf("%w: reading top-level box header: %w", ErrFailed, err)
		}
		if err := checkBoxFitsFile(header, pos, fileSize); err != nil {
			return err
		}
		if err := checkMoovSize(header, &moovCount); err != nil {
			return err
		}

		if _, err := f.Seek(int64(header.Size)-int64(header.Hdrlen), io.SeekCurrent); err != nil {
			return fmt.Errorf("%w: skipping box %q: %w", ErrFailed, header.Name, err)
		}
		pos += int64(header.Size)
	}
	return nil
}

// checkBoxFitsFile refuses header when it claims to reach past
// fileSize. The comparison stays entirely in uint64: header.Size can
// claim any 64-bit value (mdat's own large-size form allows one),
// and converting that down to int64 first could wrap negative and
// let a too-large claim slip past a signed comparison instead of
// being caught by it.
func checkBoxFitsFile(header mp4.BoxHeader, pos, fileSize int64) error {
	remaining := uint64(fileSize - pos)
	if header.Size > remaining {
		return fmt.Errorf("%w: box %q size %d runs past the file's own end", ErrFailed, header.Name, header.Size)
	}
	return nil
}

// checkMoovSize updates moovCount for header and refuses a second
// moov box, or one over maxMoovBytes.
func checkMoovSize(header mp4.BoxHeader, moovCount *int) error {
	if header.Name != "moov" {
		return nil
	}
	*moovCount++
	if *moovCount > 1 {
		return fmt.Errorf("%w: more than one moov box", ErrFailed)
	}
	if header.Size > maxMoovBytes {
		return fmt.Errorf("%w: moov box is %d bytes, more than %d allowed", ErrFailed, header.Size, maxMoovBytes)
	}
	return nil
}

// remuxCaps bounds what remuxMP4 is willing to accept from the
// sandboxed file it rebuilds, sized per profile (see video.go and
// animation.go). Neither bound is this package's real defence
// against an oversized output - checkOutput's own cap on the
// sandboxed file as a whole, enforced before remuxMP4 ever runs,
// already provides that - they exist only so a single absurd value
// (a billion declared samples, one claiming to be a gigabyte) cannot
// cost this package more time or memory than parsing a file already
// known to be small should.
type remuxCaps struct {
	allowAudio        bool
	maxVideoSamples   int
	maxAudioSamples   int
	maxVideoSampleLen int64
	maxAudioSampleLen int64
}

// remuxMP4 validates path - an MP4 the sandboxed ffmpeg wrote -
// against caps, then writes a new file at outPath holding only what
// it validated: one avc1 video track, AVC Main profile, with at most
// one mp4a AAC-LC audio track when caps.allowAudio allows it. Any
// failure, including a panic while parsing path (mp4ff decodes
// untrusted input; it is not expected to panic, but nothing here
// trusts that it cannot), is reported as ErrFailed.
func remuxMP4(path, outPath string, caps remuxCaps) (err error) {
	defer func() {
		// The recovered value itself is never logged or wrapped: it
		// can carry whatever string a panicking line happened to
		// build, which may itself echo bytes from the untrusted file
		// being parsed. %T names only the panic's own type, never its
		// value (see tools/nologcontent and docs/decisions.md).
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: panic rebuilding container (%T)", ErrFailed, r)
		}
	}()
	return doRemux(path, outPath, caps)
}

// doRemux is remuxMP4's own body, split out so remuxMP4's recover
// covers every return path through one deferred call.
func doRemux(path, outPath string, caps remuxCaps) error {
	in, parsed, err := openIntermediate(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close() // read-only handle: nothing left to report that matters once the checks below have already returned their own error
	}()

	videoTrak, audioTrak, err := classifyTracks(parsed.Moov, caps.allowAudio)
	if err != nil {
		return err
	}
	avcC, err := validateVideoTrack(videoTrak)
	if err != nil {
		return err
	}
	var asc *aac.AudioSpecificConfig
	if audioTrak != nil {
		if asc, err = validateAudioTrack(audioTrak); err != nil {
			return err
		}
	}

	scratchPath := outPath + ".scratch"
	src := mp4Source{file: parsed, rs: in}
	video, audio, err := collectSamples(src, videoTrak, audioTrak, scratchPath, caps)
	if err != nil {
		_ = os.Remove(scratchPath) // best-effort: doRemux is already returning the real error
		return err
	}
	defer func() {
		_ = os.Remove(scratchPath) // the scratch file's job ends the moment writeRemuxed has read it back
	}()

	return writeRemuxed(outPath, scratchPath, remuxDescriptors{avcC: avcC, asc: asc}, video, audio)
}

// openIntermediate opens path, checks its top-level box structure
// (checkTopLevelBoxes) and only then parses it as an MP4 with
// mp4ff's own lazy mdat decoding, so a large sample payload - or an
// oversized moov, which mp4ff reads whole regardless of decode mode -
// is never read into memory just to find out how many tracks the
// file has.
func openIntermediate(path string) (*os.File, *mp4.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: opening intermediate: %w", ErrFailed, err)
	}

	if err := checkTopLevelBoxes(f); err != nil {
		_ = f.Close() // read-only handle already failing; nothing left to report that matters
		return nil, nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close() // same as above
		return nil, nil, fmt.Errorf("%w: rewinding intermediate: %w", ErrFailed, err)
	}

	parsed, err := mp4.DecodeFile(f, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		_ = f.Close() // read-only handle already failing to parse; nothing left to report that matters
		return nil, nil, fmt.Errorf("%w: parsing intermediate: %w", ErrFailed, err)
	}
	if parsed.Moov == nil {
		_ = f.Close() // same as above
		return nil, nil, fmt.Errorf("%w: no moov box", ErrFailed)
	}
	return f, parsed, nil
}
