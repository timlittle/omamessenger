package transcode

// remux.go rebuilds an MP4 the sandboxed ffmpeg wrote into a brand
// new file this package writes itself with mp4ff: every value is
// validated before use, and nothing in the source crosses into the
// rebuilt file uninspected, so a compromised sandboxed ffmpeg can
// still only ever reach Qt's own H.264 and AAC decoders, never a
// container parser (see docs/decisions.md).

import (
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

// remuxSampleCountSlack gives a profile's duration-cap-derived sample
// count a little room, since fpsmax and the AAC frame rate only
// approximate the encoder's real output.
const remuxSampleCountSlack = 60

// maxAACFramesPerSecond bounds how many AAC-LC frames per second of
// audio this package expects, generous for both sample rates it
// accepts.
const maxAACFramesPerSecond = 50

// maxRemuxVideoSampleBytes bounds any one video sample this package
// will rebuild, with headroom over the encoder's own VBV bufsize cap.
const maxRemuxVideoSampleBytes = 4 << 20

// maxRemuxAudioSampleBytes bounds any one AAC frame a remuxed video's
// audio track may carry, generous for the highest bitrate ffmpeg is
// ever asked to produce.
const maxRemuxAudioSampleBytes = 32 << 10

// maxMoovBytes bounds a moov box's declared size, checked before
// mp4.DecodeFile reads its whole payload into memory. The worst case
// this package legitimately produces is a 519,147-byte moov; 2 MiB
// leaves roughly 4x that as headroom.
const maxMoovBytes = 2 << 20

// maxTopLevelBoxes bounds how many top-level boxes
// checkTopLevelBoxes walks before refusing: an ordinary faststart MP4
// this package produces has four (ftyp, moov, free, mdat).
const maxTopLevelBoxes = 8

// checkTopLevelBoxes walks f's top-level boxes, reading only each
// header and seeking past the body, refusing more than one moov box,
// a moov over maxMoovBytes, more than maxTopLevelBoxes boxes, or any
// box reaching past the file's end. openIntermediate rewinds f
// afterwards.
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
// fileSize. The comparison stays in uint64, since converting
// header.Size to int64 first could wrap negative and let an
// oversized claim slip past a signed comparison.
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

// remuxCaps bounds what remuxMP4 accepts from the sandboxed file it
// rebuilds, sized per profile (see video.go and animation.go).
// maxVideoSamples, maxAudioSamples, maxVideoSampleLen and
// maxAudioSampleLen stop a single absurd declared value from costing
// more time or memory than a small file should.
//
// maxOutputBytes guards against a different attack: a crafted sample
// table can declare many samples that all alias the same small mdat
// payload, which collectSamples would otherwise write out again on
// every declared sample, growing this package's own disk rather than
// reflecting the untrusted file's real content (see scratchWriter in
// remuxsamples.go).
type remuxCaps struct {
	allowAudio        bool
	maxVideoSamples   int
	maxAudioSamples   int
	maxVideoSampleLen int64
	maxAudioSampleLen int64
	maxOutputBytes    int64
}

// remuxMP4 validates path, an MP4 the sandboxed ffmpeg wrote, against
// caps, then writes outPath holding only one avc1 video track plus an
// optional mp4a AAC-LC audio track. Any failure, including a panic
// while parsing untrusted input, is reported as ErrFailed.
func remuxMP4(path, outPath string, caps remuxCaps) (err error) {
	defer func() {
		// %T logs only the panic's type, never its value, which could echo untrusted file bytes.
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: panic rebuilding container (%T)", ErrFailed, r)
		}
	}()
	return doRemux(path, outPath, caps)
}

// doRemux is remuxMP4's body, split out so remuxMP4's recover covers
// every return path through one deferred call.
func doRemux(path, outPath string, caps remuxCaps) error {
	in, parsed, err := openIntermediate(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close() // read-only handle; nothing left to report
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
		_ = os.Remove(scratchPath) // best-effort cleanup; the real error is already returned
		return err
	}
	defer func() {
		_ = os.Remove(scratchPath) // no longer needed once writeRemuxed has read it back
	}()

	return writeRemuxed(outPath, scratchPath, remuxDescriptors{avcC: avcC, asc: asc}, video, audio)
}

// openIntermediate opens path, checks its top-level box structure,
// and only then parses it with mp4ff's lazy mdat decoding, so a large
// sample payload is never read into memory just to find out how many
// tracks the file has.
func openIntermediate(path string) (*os.File, *mp4.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: opening intermediate: %w", ErrFailed, err)
	}

	if err := checkTopLevelBoxes(f); err != nil {
		_ = f.Close() // already failing; nothing left to report
		return nil, nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close() // same as above
		return nil, nil, fmt.Errorf("%w: rewinding intermediate: %w", ErrFailed, err)
	}

	parsed, err := mp4.DecodeFile(f, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		_ = f.Close() // same as above
		return nil, nil, fmt.Errorf("%w: parsing intermediate: %w", ErrFailed, err)
	}
	if parsed.Moov == nil {
		_ = f.Close() // same as above
		return nil, nil, fmt.Errorf("%w: no moov box", ErrFailed)
	}
	return f, parsed, nil
}
