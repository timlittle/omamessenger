// Package audiosafetest builds the minimal byte streams audiosafe.Validate
// accepts or rejects, for tests in other packages that need a real sound
// file without checking a binary audio sample into the repository.
package audiosafetest

import (
	"bytes"
	"encoding/binary"
)

// opusGranuleRate is Opus's fixed sample clock (RFC 7845), the same rate
// audiosafe's own ogg.go divides a stream's final granule position by to
// find its duration.
const opusGranuleRate = 48000

// ValidOpusFixture returns the bytes of the simplest file
// audiosafe.Validate reports playable: a one-second, single-bitstream
// Ogg stream carrying Opus.
func ValidOpusFixture() []byte {
	var out bytes.Buffer
	out.Write(oggPage(0x02, 0, 1, 0, []byte("OpusHead further opus header bytes")))
	out.Write(oggPage(0x04, opusGranuleRate, 1, 1, []byte("opus audio data frame")))

	return out.Bytes()
}

// InvalidFixture returns bytes audiosafe.Validate reports not playable:
// neither Ogg's "OggS" capture pattern, an ISO base media file box, nor
// an MPEG-1 Layer III frame sync.
func InvalidFixture() []byte {
	return []byte("not a sound file this helper recognizes, just plain padding")
}

// oggPage builds one minimal Ogg page carrying payload as its only
// segment. audiosafe never reads a page's CRC (see its own ogg.go), so
// this leaves that field zeroed rather than computing a real one.
func oggPage(headerType byte, granule uint64, serial, sequence uint32, payload []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("OggS")
	buf.WriteByte(0) // stream structure version
	buf.WriteByte(headerType)

	var granuleBytes [8]byte
	binary.LittleEndian.PutUint64(granuleBytes[:], granule)
	buf.Write(granuleBytes[:])

	var serialBytes [4]byte
	binary.LittleEndian.PutUint32(serialBytes[:], serial)
	buf.Write(serialBytes[:])

	var sequenceBytes [4]byte
	binary.LittleEndian.PutUint32(sequenceBytes[:], sequence)
	buf.Write(sequenceBytes[:])

	buf.Write([]byte{0, 0, 0, 0}) // CRC, unchecked
	buf.WriteByte(1)              // one lacing entry
	buf.WriteByte(byte(len(payload)))
	buf.Write(payload)

	return buf.Bytes()
}
