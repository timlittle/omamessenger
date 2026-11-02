package sandbox

// classify_test.go tests classifyStderr directly, as an internal
// test: it is pure and unexported, and every case below is already
// covered end to end through Run in process_internal_test.go and
// sandbox_test.go, so this file only needs to isolate classifyStderr's
// own mapping.

import "testing"

func TestClassifyStderr(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw  string
		want string
	}{
		"empty":             {raw: "", want: classNone},
		"protocol denied":   {raw: "in.mp4: Protocol not on whitelist 'file,crypto', you may need to enable crypto\n", want: classProtocolDenied},
		"invalid data":      {raw: "[mov,mp4,m4a,3gp,3g2,mj2 @ 0x0] Invalid data found when processing input\n", want: classInvalidData},
		"unknown decoder":   {raw: "Unknown decoder 'foo'\n", want: classUnsupported},
		"decoder not found": {raw: "Decoder (codec av1) not found for input stream #0:0\n", want: classUnsupported},
		"unsupported codec": {raw: "Unsupported codec for output stream\n", want: classUnsupported},
		"something else":    {raw: "a line this package has never seen before\n", want: classOther},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := classifyStderr([]byte(tt.raw)); got != tt.want {
				t.Errorf("classifyStderr(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
