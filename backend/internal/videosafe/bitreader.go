package videosafe

// bitreader.go is the generic, bit-level reader H.264's own Exp-Golomb
// bitstream syntax needs: h264sps.go's own SPS field parsing and
// avcsamples.go's own in-band PPS check (ppsReferencesSPS) both read
// their NAL unit's bits through it, after removeEmulationPrevention
// has stripped the one byte-level encoding H.264 itself inserts.
// Neither function here has any idea what field it is being used to
// read; that is entirely up to the caller.

// removeEmulationPrevention strips every emulation-prevention byte
// (a 0x03 immediately after two 0x00 bytes) H.264 inserts into a NAL
// unit's own encoded bytes, returning the raw RBSP bits parseSPSDims
// reads: without this, any legitimate SPS containing two zero bytes
// in a row would misalign every bit field after them.
func removeEmulationPrevention(b []byte) []byte {
	out := make([]byte, 0, len(b))
	zeroRun := 0
	for _, v := range b {
		if zeroRun >= 2 && v == 0x03 {
			zeroRun = 0
			continue
		}
		if v == 0 {
			zeroRun++
		} else {
			zeroRun = 0
		}
		out = append(out, v)
	}
	return out
}

// bitReader reads an H.264 RBSP one bit at a time, already stripped
// of emulation prevention: a plain []byte and bit offset, never an
// io.ReaderAt, since an SPS is always small enough (maxSPSLength) to
// hold in memory as a single slice.
type bitReader struct {
	data []byte
	pos  int
}

// u reads the next n bits as an unsigned integer, most significant
// bit first, reporting false once fewer than n bits remain.
func (r *bitReader) u(n int) (uint32, bool) {
	if n > 32 || r.pos+n > len(r.data)*8 {
		return 0, false
	}

	var v uint32
	for i := 0; i < n; i++ {
		byteIdx := r.pos / 8
		bitIdx := 7 - r.pos%8
		v = v<<1 | uint32((r.data[byteIdx]>>bitIdx)&1)
		r.pos++
	}
	return v, true
}

// ue reads an Exp-Golomb-coded unsigned integer (H.264's own ue(v)):
// a run of leading zero bits, a terminating one bit, then that many
// more bits as the value's own low-order bits. A prefix longer than
// maxExpGolombPrefix is refused rather than read, the one way a
// crafted SPS could otherwise make this decode an implausibly large
// value.
func (r *bitReader) ue() (uint32, bool) {
	leadingZeros := 0
	for {
		b, ok := r.u(1)
		if !ok {
			return 0, false
		}
		if b == 1 {
			break
		}
		leadingZeros++
		if leadingZeros > maxExpGolombPrefix {
			return 0, false
		}
	}
	if leadingZeros == 0 {
		return 0, true
	}

	rest, ok := r.u(leadingZeros)
	if !ok {
		return 0, false
	}
	return (1<<leadingZeros - 1) + rest, true
}

// se reads an Exp-Golomb-coded signed integer (H.264's own se(v)),
// derived from ue the way the specification defines it: alternating
// signed values in increasing magnitude as the underlying unsigned
// code increases.
func (r *bitReader) se() (int32, bool) {
	k, ok := r.ue()
	if !ok {
		return 0, false
	}

	v := int32((k + 1) / 2)
	if k%2 == 0 {
		v = -v
	}
	return v, true
}
