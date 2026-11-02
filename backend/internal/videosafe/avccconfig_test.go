package videosafe_test

// avccconfig_test.go checks Validate against avcC structural edge
// cases - the box around the one SPS isobmff_test.go's own
// TestValidate_MP4SPSDimensions already covers - mirroring
// avccconfig.go the way isobmff_test.go mirrors isobmff.go.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_MP4AVCCStructuralChecks covers the exact gap a
// security review found: `readAVCDecoderConfig` read an avcC box's
// own SPS count and NAL length-prefix width, but never
// configurationVersion, never cross-checked avcC's own declared
// profile/compatibility/level bytes against the SPS's own copy of the
// same three fields, never refused the one reserved
// lengthSizeMinusOne value (2) ISO/IEC 14496-15 never defines, and
// never refused bytes left over once its own SPS was read - any of
// which, left unbound, hides a second, unbounded SPS in exactly the
// bytes FFmpeg's own avcC-mode decoder also reads from the same box
// (see docs/decisions.md).
func TestValidate_MP4AVCCStructuralChecks(t *testing.T) {
	t.Parallel()

	validConfig := buildAVCDecoderConfig(1280, 720)
	mutate := func(index int, value byte) []byte {
		cfg := append([]byte{}, validConfig...)
		cfg[index] = value
		return cfg
	}

	cases := map[string][]byte{
		"configurationVersion 0, otherwise a normally valid config":  mutate(0, 0),
		"avcC's own profile byte disagrees with the SPS":             mutate(1, 77),
		"avcC's own level byte disagrees with the SPS":               mutate(3, 51),
		"lengthSizeMinusOne 2, the one value the spec never defines": mutate(4, 0xFE),
		"trailing bytes past avcC's own picture-parameter-set list": append(
			append([]byte{}, validConfig...), 0x01, 0x02, 0x03, 0x04,
		),
	}

	for name, avccBody := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "avcc.mp4", buildCustomAVCFixture(1280, 720, avccBody))
			got, err := videosafe.Validate(path)
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if got {
				t.Errorf("Validate() = true for %q, want it refused", name)
			}
		})
	}
}
