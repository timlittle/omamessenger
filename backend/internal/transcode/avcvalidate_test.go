package transcode

// avcvalidate_test.go tests checkSPS, checkPPS and
// checkAudioSpecificConfig directly, as an internal test: each is
// tested against hand-built avc.SPS/avc.PPS/aac.AudioSpecificConfig
// values rather than a real H.264 or AAC bitstream, since building
// one by hand for every case under test (a crafted coded size, an
// excessive crop, too many reference frames, a declared slice group)
// would need bit-exact Exp-Golomb encoding this package has no other
// reason to ever do itself. remux_test.go's own real-ffmpeg
// integration tests, and its hand-built mainSPSHex/largeSPSHex
// fixtures, cover the same checks through an actual bitstream.

import (
	"errors"
	"testing"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/avc"
)

func TestCheckSPS(t *testing.T) {
	t.Parallel()

	valid := func() *avc.SPS {
		return &avc.SPS{
			Profile: spsProfileMain, Level: 31, ChromaFormatIDC: spsChroma420,
			FrameMbsOnlyFlag: true, Width: 320, Height: 240,
		}
	}

	tests := map[string]struct {
		sps     func() *avc.SPS
		width   uint16
		height  uint16
		wantErr bool
	}{
		"valid":                 {sps: valid, width: 320, height: 240},
		"wrong profile":         {sps: func() *avc.SPS { s := valid(); s.Profile = 100; return s }, width: 320, height: 240, wantErr: true},
		"level over max":        {sps: func() *avc.SPS { s := valid(); s.Level = 41; return s }, width: 320, height: 240, wantErr: true},
		"level at max is fine":  {sps: func() *avc.SPS { s := valid(); s.Level = 40; return s }, width: 320, height: 240},
		"wrong chroma":          {sps: func() *avc.SPS { s := valid(); s.ChromaFormatIDC = 2; return s }, width: 320, height: 240, wantErr: true},
		"not 8-bit luma":        {sps: func() *avc.SPS { s := valid(); s.BitDepthLumaMinus8 = 2; return s }, width: 320, height: 240, wantErr: true},
		"not 8-bit chroma":      {sps: func() *avc.SPS { s := valid(); s.BitDepthChromaMinus8 = 2; return s }, width: 320, height: 240, wantErr: true},
		"not frame-only":        {sps: func() *avc.SPS { s := valid(); s.FrameMbsOnlyFlag = false; return s }, width: 320, height: 240, wantErr: true},
		"odd width":             {sps: func() *avc.SPS { s := valid(); s.Width = 321; return s }, width: 321, height: 240, wantErr: true},
		"odd height":            {sps: func() *avc.SPS { s := valid(); s.Height = 241; return s }, width: 320, height: 241, wantErr: true},
		"width over max":        {sps: func() *avc.SPS { s := valid(); s.Width = 1282; return s }, width: 1282, height: 240, wantErr: true},
		"height over max":       {sps: func() *avc.SPS { s := valid(); s.Height = 1282; return s }, width: 320, height: 1282, wantErr: true},
		"width at max is fine":  {sps: func() *avc.SPS { s := valid(); s.Width = 1280; return s }, width: 1280, height: 240},
		"entry width mismatch":  {sps: valid, width: 640, height: 240, wantErr: true},
		"entry height mismatch": {sps: valid, width: 320, height: 480, wantErr: true},
		"huge coded size cropped down to a small display size": {
			// 1000x1000 macroblocks (16000x16000 coded) cropped all the
			// way down to 1280x720: exactly the shape checkSPSCoded
			// exists to catch, since mp4ff's own sps.Width/sps.Height
			// only ever exposes the cropped, already-small display size.
			sps: func() *avc.SPS {
				s := valid()
				s.Width, s.Height = 1280, 720
				s.FrameCroppingFlag = true
				s.FrameCropRightOffset = 7360  // (16000-1280)/cropUnitX(2)
				s.FrameCropBottomOffset = 7640 // (16000-720)/cropUnitY(2)
				return s
			},
			width: 1280, height: 720, wantErr: true,
		},
		"crop on the left edge": {
			sps:   func() *avc.SPS { s := valid(); s.FrameCroppingFlag, s.FrameCropLeftOffset = true, 1; return s },
			width: 320, height: 240, wantErr: true,
		},
		"crop on the top edge": {
			sps:   func() *avc.SPS { s := valid(); s.FrameCroppingFlag, s.FrameCropTopOffset = true, 1; return s },
			width: 320, height: 240, wantErr: true,
		},
		"right crop at 16 luma samples is refused": {
			// FrameCropRightOffset is in crop units (2 luma samples each
			// for 4:2:0 frame-only), so 8 units is exactly 16 samples -
			// a whole macroblock, which x264 never needs to cross.
			sps: func() *avc.SPS {
				s := valid()
				s.Width = 336 - 16
				s.FrameCroppingFlag, s.FrameCropRightOffset = true, 8
				return s
			},
			width: 336 - 16, height: 240, wantErr: true,
		},
		"right crop under 16 luma samples is fine": {
			// 336 coded (21 macroblocks), cropped by 14 samples (7
			// units) to a 322-wide display size: exactly the shape
			// x264's own rounding-up-to-a-macroblock cropping produces.
			sps: func() *avc.SPS {
				s := valid()
				s.Width = 322
				s.FrameCroppingFlag, s.FrameCropRightOffset = true, 7
				return s
			},
			width: 322, height: 240,
		},
		"too many ref frames": {
			sps:   func() *avc.SPS { s := valid(); s.NumRefFrames = spsMaxRefFrames + 1; return s },
			width: 320, height: 240, wantErr: true,
		},
		"ref frames at the cap is fine": {
			sps:   func() *avc.SPS { s := valid(); s.NumRefFrames = spsMaxRefFrames; return s },
			width: 320, height: 240,
		},
		"max_dec_frame_buffering over the cap": {
			sps: func() *avc.SPS {
				s := valid()
				s.VUI = &avc.VUIParameters{BitstreamRestrictionFlag: true, MaxDecFrameBuffering: spsMaxRefFrames + 1}
				return s
			},
			width: 320, height: 240, wantErr: true,
		},
		"max_dec_frame_buffering ignored without the bitstream restriction flag": {
			sps: func() *avc.SPS {
				s := valid()
				s.VUI = &avc.VUIParameters{BitstreamRestrictionFlag: false, MaxDecFrameBuffering: spsMaxRefFrames + 1}
				return s
			},
			width: 320, height: 240,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkSPS(tt.sps(), tt.width, tt.height)
			if tt.wantErr && !errors.Is(err, ErrFailed) {
				t.Fatalf("checkSPS() error = %v, want %v", err, ErrFailed)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkSPS() error = %v, want nil", err)
			}
		})
	}
}

func TestCheckPPS(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pps     *avc.PPS
		wantErr bool
	}{
		"no slice groups": {pps: &avc.PPS{NumSliceGroupsMinus1: 0}},
		"slice groups":    {pps: &avc.PPS{NumSliceGroupsMinus1: 1}, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkPPS(tt.pps)
			if tt.wantErr && !errors.Is(err, ErrFailed) {
				t.Fatalf("checkPPS() error = %v, want %v", err, ErrFailed)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkPPS() error = %v, want nil", err)
			}
		})
	}
}

func TestCheckAudioSpecificConfig(t *testing.T) {
	t.Parallel()

	valid := func() *aac.AudioSpecificConfig {
		return &aac.AudioSpecificConfig{ObjectType: aac.AAClc, SamplingFrequency: 48000, ChannelConfiguration: 2}
	}

	tests := map[string]struct {
		asc     func() *aac.AudioSpecificConfig
		rate    uint16
		wantErr bool
	}{
		"valid":               {asc: valid, rate: 48000},
		"44100 is fine":       {asc: func() *aac.AudioSpecificConfig { a := valid(); a.SamplingFrequency = 44100; return a }, rate: 44100},
		"mono is fine":        {asc: func() *aac.AudioSpecificConfig { a := valid(); a.ChannelConfiguration = 1; return a }, rate: 48000},
		"wrong object type":   {asc: func() *aac.AudioSpecificConfig { a := valid(); a.ObjectType = aac.HEAACv1; return a }, rate: 48000, wantErr: true},
		"wrong sample rate":   {asc: func() *aac.AudioSpecificConfig { a := valid(); a.SamplingFrequency = 32000; return a }, rate: 48000, wantErr: true},
		"too many channels":   {asc: func() *aac.AudioSpecificConfig { a := valid(); a.ChannelConfiguration = 6; return a }, rate: 48000, wantErr: true},
		"entry rate mismatch": {asc: valid, rate: 44100, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := checkAudioSpecificConfig(tt.asc(), tt.rate)
			if tt.wantErr && !errors.Is(err, ErrFailed) {
				t.Fatalf("checkAudioSpecificConfig() error = %v, want %v", err, ErrFailed)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkAudioSpecificConfig() error = %v, want nil", err)
			}
		})
	}
}
