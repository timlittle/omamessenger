package sandbox

// classify.go turns a job's own captured stderr into one of a small,
// fixed set of safe words, for logging: never the raw text, which
// could echo a sender-controlled file name or path embedded in one of
// a command's own diagnostic lines (see privacy.md).

import "strings"

// The class words classifyStderr returns. All five are this package's
// own, fixed vocabulary - never a substring of the command's own
// text - so logging one is always safe.
const (
	classNone           = "none"            // nothing was captured
	classProtocolDenied = "protocol-denied" // -protocol_whitelist refused an unlisted protocol
	classInvalidData    = "invalid-data"    // the demuxer or decoder gave up on the bytes themselves
	classUnsupported    = "unsupported"     // the file was recognised, but not this stream's codec
	classOther          = "other"           // something was captured that matched none of the above
)

// classifyStderr maps raw, already-bounded stderr bytes to one of
// this package's own class words above, by matching a short list of
// ffmpeg's own English diagnostic phrases - this package's words,
// never ffmpeg's or a sender's.
func classifyStderr(raw []byte) string {
	if len(raw) == 0 {
		return classNone
	}

	text := string(raw)
	switch {
	case strings.Contains(text, "Protocol not on whitelist"):
		return classProtocolDenied
	case strings.Contains(text, "Invalid data found when processing input"):
		return classInvalidData
	case strings.Contains(text, "Unknown decoder"),
		strings.Contains(text, "Decoder") && strings.Contains(text, "not found"),
		strings.Contains(text, "Unsupported"):
		return classUnsupported
	default:
		return classOther
	}
}
