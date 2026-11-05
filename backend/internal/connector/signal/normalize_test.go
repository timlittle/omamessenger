package signal

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// aci is a fixed account identifier every test in this file reuses,
// so expected remote ids are easy to read.
var aci = uuid.MustParse("22222222-2222-2222-2222-222222222222")

// TestNormalizeDataMessage_Text checks a plain text message's fields,
// built from a signalpb.DataMessage the same way signalmeow hands one
// to the event handler, never from a real account.
func TestNormalizeDataMessage_Text(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{
		Body:        proto.String("hello there"),
		Timestamp:   proto.Uint64(1700000000000),
		ExpireTimer: proto.Uint32(3600),
	}

	got, ptr := normalizeDataMessage(aci, "Alice", 1700000000000, msg, false)

	if got.RemoteID != "22222222-2222-2222-2222-222222222222:1700000000000" {
		t.Errorf("RemoteID = %q", got.RemoteID)
	}
	if got.Text != "hello there" {
		t.Errorf("Text = %q", got.Text)
	}
	if got.SenderName != "Alice" {
		t.Errorf("SenderName = %q", got.SenderName)
	}
	if got.Status != domain.StatusReceived {
		t.Errorf("Status = %q, want %q", got.Status, domain.StatusReceived)
	}
	if got.ExpireAfter != 3600 || !got.ArmOnRead {
		t.Errorf("ExpireAfter = %d, ArmOnRead = %t", got.ExpireAfter, got.ArmOnRead)
	}
	if ptr != nil {
		t.Errorf("ptr = %v, want nil for a text-only message", ptr)
	}
}

// TestNormalizeDataMessage_Outgoing checks that an outgoing (synced)
// message is marked accordingly and never arms the view-once /
// disappearing countdown on read, which only applies to what this
// account received.
func TestNormalizeDataMessage_Outgoing(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{Body: proto.String("sent"), ExpireTimer: proto.Uint32(60)}

	got, _ := normalizeDataMessage(aci, "Me", 1700000000001, msg, true)

	if !got.Outgoing || got.Status != domain.StatusSent {
		t.Errorf("Outgoing = %t, Status = %q", got.Outgoing, got.Status)
	}
	if got.ArmOnRead {
		t.Error("ArmOnRead = true for an outgoing message, want false")
	}
}

// TestNormalizeDataMessage_ViewOnce checks that view-once media is
// never downloaded: the message carries no Media at all.
func TestNormalizeDataMessage_ViewOnce(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{
		IsViewOnce:  proto.Bool(true),
		Attachments: []*signalpb.AttachmentPointer{{ContentType: proto.String("image/jpeg")}},
	}

	got, ptr := normalizeDataMessage(aci, "Alice", 1700000000002, msg, false)

	if !got.ViewOnce {
		t.Error("ViewOnce = false, want true")
	}
	if got.Media != nil {
		t.Errorf("Media = %+v, want nil for view-once", got.Media)
	}
	if ptr != nil {
		t.Error("ptr != nil for view-once, want nothing to fetch")
	}
}

// TestNormalizeDataMessage_Attachment checks a photo attachment maps
// to a fetchable Media and keeps its pointer for FetchMedia.
func TestNormalizeDataMessage_Attachment(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{
		Attachments: []*signalpb.AttachmentPointer{{
			ContentType: proto.String("image/jpeg"), Size: proto.Uint32(1024),
			Width: proto.Uint32(100), Height: proto.Uint32(50),
		}},
	}

	got, ptr := normalizeDataMessage(aci, "Alice", 1700000000003, msg, false)

	if got.Media == nil || got.Media.Kind != domain.MediaPhoto {
		t.Fatalf("Media = %+v, want a photo", got.Media)
	}
	if got.Media.Width != 100 || got.Media.Height != 50 || got.Media.Size != 1024 {
		t.Errorf("Media dimensions/size = %+v", got.Media)
	}
	if ptr == nil {
		t.Error("ptr = nil, want the attachment pointer kept for FetchMedia")
	}
}

// TestNormalizeDataMessage_VoiceNote checks the voice flag maps to the
// voice media kind with its duration.
func TestNormalizeDataMessage_VoiceNote(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{
		Attachments: []*signalpb.AttachmentPointer{{
			ContentType:          proto.String("audio/aac"),
			Flags:                proto.Uint32(uint32(signalpb.AttachmentPointer_VOICE_MESSAGE)),
			AudioDurationSeconds: proto.Float32(12),
		}},
	}

	got, _ := normalizeDataMessage(aci, "Alice", 1700000000004, msg, false)

	if got.Media == nil || got.Media.Kind != domain.MediaVoice {
		t.Fatalf("Media = %+v, want voice", got.Media)
	}
	if got.Media.Duration != 12 {
		t.Errorf("Duration = %d, want 12", got.Media.Duration)
	}
}

// TestNormalizeDataMessage_Sticker checks a sticker maps to its own
// media kind with its emoji.
func TestNormalizeDataMessage_Sticker(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{
		Sticker: &signalpb.DataMessage_Sticker{
			Emoji: proto.String("🎉"),
			Data:  &signalpb.AttachmentPointer{ContentType: proto.String("image/webp")},
		},
	}

	got, ptr := normalizeDataMessage(aci, "Alice", 1700000000005, msg, false)

	if got.Media == nil || got.Media.Kind != domain.MediaSticker || got.Media.Emoji != "🎉" {
		t.Fatalf("Media = %+v", got.Media)
	}
	if ptr == nil {
		t.Error("ptr = nil, want the sticker's attachment pointer kept")
	}
}

// TestNormalizeDataMessage_PollCreate checks a new poll's options
// start at zero votes.
func TestNormalizeDataMessage_PollCreate(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{
		PollCreate: &signalpb.DataMessage_PollCreate{
			Question: proto.String("Pizza?"),
			Options:  []string{"Yes", "No"},
		},
	}

	got, _ := normalizeDataMessage(aci, "Alice", 1700000000006, msg, false)

	if got.Media == nil || got.Media.Kind != domain.MediaPoll {
		t.Fatalf("Media = %+v, want a poll", got.Media)
	}
	if got.Media.Poll.Question != "Pizza?" || len(got.Media.Poll.Options) != 2 {
		t.Errorf("Poll = %+v", got.Media.Poll)
	}
	for _, opt := range got.Media.Poll.Options {
		if opt.Votes != 0 {
			t.Errorf("option %q votes = %d, want 0 on creation", opt.Text, opt.Votes)
		}
	}
}

// TestNormalizeDataMessage_LinkPreview checks a link preview's url is
// dropped when it fails domain.AllowedLinkURL, so an unsafe scheme
// never reaches the UI as a clickable link.
func TestNormalizeDataMessage_LinkPreview(t *testing.T) {
	t.Parallel()

	msg := &signalpb.DataMessage{
		Preview: []*signalpb.Preview{{Url: proto.String("javascript:alert(1)"), Title: proto.String("evil")}},
	}

	got, _ := normalizeDataMessage(aci, "Alice", 1700000000007, msg, false)

	if got.Media == nil {
		t.Fatal("Media = nil, want a link preview")
	}
	if got.Media.URL != "" {
		t.Errorf("URL = %q, want dropped for an unsafe scheme", got.Media.URL)
	}
}

// TestBodyRanges_Formatting checks every style range maps to the
// matching span kind, and a mention range is dropped from formatting
// (see TestMentions).
func TestBodyRanges_Formatting(t *testing.T) {
	t.Parallel()

	ranges := []*signalpb.BodyRange{
		{Start: proto.Uint32(0), Length: proto.Uint32(4), AssociatedValue: &signalpb.BodyRange_Style_{Style: signalpb.BodyRange_BOLD}},
		{Start: proto.Uint32(5), Length: proto.Uint32(6), AssociatedValue: &signalpb.BodyRange_Style_{Style: signalpb.BodyRange_SPOILER}},
		{Start: proto.Uint32(12), Length: proto.Uint32(2), AssociatedValue: &signalpb.BodyRange_Style_{Style: signalpb.BodyRange_ITALIC}},
		{Start: proto.Uint32(15), Length: proto.Uint32(3), AssociatedValue: &signalpb.BodyRange_Style_{Style: signalpb.BodyRange_STRIKETHROUGH}},
		{Start: proto.Uint32(20), Length: proto.Uint32(4), AssociatedValue: &signalpb.BodyRange_Style_{Style: signalpb.BodyRange_MONOSPACE}},
		{Start: proto.Uint32(25), Length: proto.Uint32(1), AssociatedValue: &signalpb.BodyRange_Style_{Style: signalpb.BodyRange_NONE}},
	}

	spans := bodyRanges(ranges)
	if len(spans) != 5 {
		t.Fatalf("len(spans) = %d, want 5 (NONE dropped)", len(spans))
	}
	wantKinds := []string{domain.SpanBold, domain.SpanSpoiler, domain.SpanItalic, domain.SpanStrike, domain.SpanCode}
	for i, want := range wantKinds {
		if spans[i].Kind != want {
			t.Errorf("spans[%d].Kind = %q, want %q", i, spans[i].Kind, want)
		}
	}
}

// TestMentions checks a mention range maps to a domain mention naming
// the mentioned account.
func TestMentions(t *testing.T) {
	t.Parallel()

	target := uuid.New().String()
	ranges := []*signalpb.BodyRange{{
		Start: proto.Uint32(0), Length: proto.Uint32(5),
		AssociatedValue: &signalpb.BodyRange_MentionAci{MentionAci: target},
	}}

	got := mentions(ranges)
	if len(got) != 1 || got[0].UserID != target {
		t.Errorf("mentions = %+v, want one naming %q", got, target)
	}
}

// TestMentions_SkipsRangesWithNoMention checks that a plain style
// range (no mention) is dropped rather than reported as an empty
// mention.
func TestMentions_SkipsRangesWithNoMention(t *testing.T) {
	t.Parallel()

	ranges := []*signalpb.BodyRange{{Start: proto.Uint32(0), Length: proto.Uint32(4), AssociatedValue: &signalpb.BodyRange_Style_{Style: signalpb.BodyRange_BOLD}}}

	if got := mentions(ranges); len(got) != 0 {
		t.Errorf("mentions = %+v, want none", got)
	}
}

// TestAttachmentKind checks every content type and flag combination
// this connector maps.
func TestAttachmentKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		voice       bool
		want        string
	}{
		{"image", "image/jpeg", false, domain.MediaPhoto},
		{"video", "video/mp4", false, domain.MediaVideo},
		{"voice", "audio/aac", true, domain.MediaVoice},
		{"other", "application/pdf", false, domain.MediaFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ptr := &signalpb.AttachmentPointer{ContentType: proto.String(tt.contentType)}
			if tt.voice {
				ptr.Flags = proto.Uint32(uint32(signalpb.AttachmentPointer_VOICE_MESSAGE))
			}
			if got := attachmentKind(ptr); got != tt.want {
				t.Errorf("attachmentKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSanitizedThumb_SanitizesEmbeddedImage checks that a valid
// embedded thumbnail is re-encoded and base64-returned, and that
// garbage bytes are dropped rather than passed through.
func TestSanitizedThumb_SanitizesEmbeddedImage(t *testing.T) {
	t.Parallel()

	if got := sanitizedThumb([]byte("not an image")); got != "" {
		t.Errorf("sanitizedThumb(garbage) = %q, want empty", got)
	}
	if got := sanitizedThumb(tinyPNG(t)); got == "" {
		t.Error("sanitizedThumb(valid PNG) = empty, want sanitized bytes")
	}
}

// tinyPNG returns a minimal, valid 1x1 PNG, for tests exercising the
// success path of image sanitization.
func tinyPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode tiny PNG: %v", err)
	}

	return buf.Bytes()
}

// FuzzNormalizeDataMessage checks that no wire bytes, however
// malformed, make normalizeDataMessage panic: signalmeow decoded them
// off the wire from another Signal client, which this connector does
// not otherwise trust.
func FuzzNormalizeDataMessage(f *testing.F) {
	f.Add([]byte{})
	seed := &signalpb.DataMessage{Body: proto.String("seed"), Timestamp: proto.Uint64(1)}
	raw, err := proto.Marshal(seed)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(raw)

	f.Fuzz(func(t *testing.T, data []byte) {
		var msg signalpb.DataMessage
		if proto.Unmarshal(data, &msg) != nil {
			return
		}

		normalizeDataMessage(aci, "Fuzzer", msg.GetTimestamp(), &msg, false)
	})
}
