package signal

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestApplyBackupMedia_AttachmentOnly checks that a backup message
// with an attachment and no text body still carries its media,
// rather than being dropped for having an empty body.
func TestApplyBackupMedia_AttachmentOnly(t *testing.T) {
	t.Parallel()

	m := domain.Message{}
	std := &backuppb.StandardMessage{Attachments: []*backuppb.MessageAttachment{{
		Pointer: &backuppb.FilePointer{
			ContentType: proto.String("image/jpeg"), FileName: proto.String("photo.jpg"),
			Width: proto.Uint32(800), Height: proto.Uint32(600),
			LocatorInfo: &backuppb.FilePointer_LocatorInfo{Size: 4096},
		},
	}}}

	att := applyBackupMedia(&m, std)
	if att == nil {
		t.Fatal("applyBackupMedia() returned no attachment")
	}
	if want := domain.MediaPlaceholder(domain.MediaPhoto); m.Text != want {
		t.Errorf("Text = %q, want the placeholder %q since every stored message needs text", m.Text, want)
	}
	if m.Media == nil || m.Media.Kind != domain.MediaPhoto || m.Media.FileName != "photo.jpg" ||
		m.Media.Width != 800 || m.Media.Height != 600 || m.Media.Size != 4096 {
		t.Errorf("Media = %+v", m.Media)
	}
}

// TestApplyBackupMedia_CaptionOnPointerFillsEmptyText checks the first
// place a caption can live: the attachment's own FilePointer.Caption,
// used when the message's text body is empty. The result is one
// message carrying both the caption as text and the media.
func TestApplyBackupMedia_CaptionOnPointerFillsEmptyText(t *testing.T) {
	t.Parallel()

	m := domain.Message{}
	std := &backuppb.StandardMessage{Attachments: []*backuppb.MessageAttachment{{
		Pointer: &backuppb.FilePointer{ContentType: proto.String("image/jpeg"), Caption: proto.String("from the phone")},
	}}}

	applyBackupMedia(&m, std)

	if m.Text != "from the phone" {
		t.Errorf("Text = %q, want the pointer's own caption", m.Text)
	}
	if m.Media == nil {
		t.Error("Media was not set alongside the caption")
	}
}

// TestApplyBackupMedia_BodyTextTakesPriorityOverPointerCaption checks
// the second place a caption can live: the message's own text body.
// When both are set, the body wins and the pointer's caption is not
// appended or substituted, so there is still one message, not two.
func TestApplyBackupMedia_BodyTextTakesPriorityOverPointerCaption(t *testing.T) {
	t.Parallel()

	m := domain.Message{Text: "typed on the phone"}
	std := &backuppb.StandardMessage{
		Text: &backuppb.Text{Body: "typed on the phone"},
		Attachments: []*backuppb.MessageAttachment{{
			Pointer: &backuppb.FilePointer{ContentType: proto.String("image/jpeg"), Caption: proto.String("legacy caption")},
		}},
	}

	applyBackupMedia(&m, std)

	if m.Text != "typed on the phone" {
		t.Errorf("Text = %q, want the message body kept, not the pointer's caption", m.Text)
	}
	if m.Media == nil {
		t.Error("Media was not set alongside the body text")
	}
}

// TestApplyBackupMedia_Voice checks a voice-note attachment maps to
// the voice media kind with its duration, the way a live one does.
func TestApplyBackupMedia_Voice(t *testing.T) {
	t.Parallel()

	m := domain.Message{}
	std := &backuppb.StandardMessage{Attachments: []*backuppb.MessageAttachment{{
		Flag:    backuppb.MessageAttachment_VOICE_MESSAGE,
		Pointer: &backuppb.FilePointer{ContentType: proto.String("audio/aac"), AudioDurationSeconds: proto.Float32(12)},
	}}}

	applyBackupMedia(&m, std)

	if m.Media == nil || m.Media.Kind != domain.MediaVoice || m.Media.Duration != 12 {
		t.Errorf("Media = %+v", m.Media)
	}
}

// TestApplyBackupMedia_GIFFlagged checks a GIF-flagged video keeps its
// animated marker, the way a live attachment's GIF flag does.
func TestApplyBackupMedia_GIFFlagged(t *testing.T) {
	t.Parallel()

	m := domain.Message{}
	std := &backuppb.StandardMessage{Attachments: []*backuppb.MessageAttachment{{
		Flag:    backuppb.MessageAttachment_GIF,
		Pointer: &backuppb.FilePointer{ContentType: proto.String("video/mp4")},
	}}}

	applyBackupMedia(&m, std)

	if m.Media == nil || !m.Media.Animated {
		t.Errorf("Media = %+v, want Animated", m.Media)
	}
}

// TestApplyBackupMedia_LinkPreviewWhenNoAttachment checks a link
// preview maps the same fields a live one does, and carries no
// attachment to remember since it is not downloaded media.
func TestApplyBackupMedia_LinkPreviewWhenNoAttachment(t *testing.T) {
	t.Parallel()

	m := domain.Message{}
	std := &backuppb.StandardMessage{LinkPreview: []*backuppb.LinkPreview{{
		Url: "https://example.com/a", Title: proto.String("A page"), Description: proto.String("about a thing"),
	}}}

	att := applyBackupMedia(&m, std)

	if att != nil {
		t.Error("applyBackupMedia() returned an attachment for a link preview")
	}
	if m.Media == nil || m.Media.URL != "https://example.com/a" || m.Media.Title != "A page" {
		t.Errorf("Media = %+v", m.Media)
	}
	if want := domain.MediaPlaceholder(domain.MediaFile); m.Text != want {
		t.Errorf("Text = %q, want the placeholder %q with no body text", m.Text, want)
	}
}

// TestApplyBackupMedia_None checks a plain text message, with neither
// an attachment nor a link preview, leaves Media unset.
func TestApplyBackupMedia_None(t *testing.T) {
	t.Parallel()

	m := domain.Message{Text: "hi"}
	att := applyBackupMedia(&m, &backuppb.StandardMessage{Text: &backuppb.Text{Body: "hi"}})

	if att != nil || m.Media != nil {
		t.Errorf("att = %+v, Media = %+v, want both nil", att, m.Media)
	}
}

// TestBackupTransitPointer_EncryptedDigest checks a locator verified
// by the sender's own encrypted digest converts to a signalpb
// pointer carrying that digest, with no plaintext hash.
func TestBackupTransitPointer_EncryptedDigest(t *testing.T) {
	t.Parallel()

	att := &backuppb.MessageAttachment{Pointer: &backuppb.FilePointer{
		ContentType: proto.String("image/png"), FileName: proto.String("x.png"),
		LocatorInfo: &backuppb.FilePointer_LocatorInfo{
			Key: []byte{1, 2}, Size: 10,
			TransitCdnKey: proto.String("k"), TransitCdnNumber: proto.Uint32(3),
			IntegrityCheck: &backuppb.FilePointer_LocatorInfo_EncryptedDigest{EncryptedDigest: []byte{9}},
		},
	}}

	ptr, hash, found := backupTransitPointer(att)
	if !found {
		t.Fatal("backupTransitPointer() found = false, want true")
	}
	if ptr.GetCdnKey() != "k" || ptr.GetCdnNumber() != 3 || string(ptr.GetDigest()) != "\x09" {
		t.Errorf("ptr = %+v", ptr)
	}
	if hash != nil {
		t.Errorf("plaintext hash = %v, want none when verified by digest", hash)
	}
}

// TestBackupTransitPointer_PlaintextHash checks a locator verified by
// a plaintext hash instead carries no digest on the pointer, with the
// hash returned separately for the download call to check.
func TestBackupTransitPointer_PlaintextHash(t *testing.T) {
	t.Parallel()

	att := &backuppb.MessageAttachment{Pointer: &backuppb.FilePointer{
		ContentType: proto.String("image/png"),
		LocatorInfo: &backuppb.FilePointer_LocatorInfo{
			TransitCdnKey:  proto.String("k"),
			IntegrityCheck: &backuppb.FilePointer_LocatorInfo_PlaintextHash{PlaintextHash: []byte{7, 7}},
		},
	}}

	ptr, hash, found := backupTransitPointer(att)
	if !found || ptr.GetDigest() != nil || string(hash) != "\x07\x07" {
		t.Errorf("ptr = %+v, hash = %v, found = %t", ptr, hash, found)
	}
}

// TestBackupTransitPointer_MediaTierOnly checks a locator with no
// transit-tier key, as a media-tier-only or expired-transit backup
// attachment carries, reports found false rather than a pointer that
// cannot actually be downloaded.
func TestBackupTransitPointer_MediaTierOnly(t *testing.T) {
	t.Parallel()

	att := &backuppb.MessageAttachment{Pointer: &backuppb.FilePointer{
		LocatorInfo: &backuppb.FilePointer_LocatorInfo{MediaTierCdnNumber: proto.Uint32(1)},
	}}

	if _, _, found := backupTransitPointer(att); found {
		t.Error("backupTransitPointer() found a transit copy with no transit key")
	}
}

// TestRememberBackupAttachment_NilAttachmentIsNoop checks that a
// message with no attachment at all is a harmless no-op.
func TestRememberBackupAttachment_NilAttachmentIsNoop(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	c.rememberBackupAttachment(t.Context(), "conv1", "msg1", nil)

	if got, _ := c.attachmentFor(t.Context(), "conv1", "msg1"); got != nil {
		t.Errorf("attachmentFor() = %+v, want nil", got)
	}
}

// TestImportHistory_AttachmentOnly_StoresMedia drives a transfer
// import through a real app.Ingest over a real store, for a message
// that carries an attachment and no text body: the bug this brief
// fixes, where such a message was dropped entirely.
func TestImportHistory_AttachmentOnly_StoresMedia(t *testing.T) {
	t.Parallel()

	friend := uuid.New()
	friendBytes, err := friend.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.transferMeta = &signalmeow.TransferArchiveMetadata{CDN: 3, Key: "k"}
	dev.recipients = map[uint64]*backuppb.Recipient{
		1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: friendBytes}}},
	}
	dev.chats = []*sstore.BackupChat{{Chat: &backuppb.Chat{Id: 10, RecipientId: 1}}}
	dev.itemsByChat = map[uint64][]*backuppb.ChatItem{
		10: {{AuthorId: 1, DateSent: 1, Item: &backuppb.ChatItem_StandardMessage{StandardMessage: &backuppb.StandardMessage{
			Attachments: []*backuppb.MessageAttachment{{Pointer: &backuppb.FilePointer{
				ContentType: proto.String("image/jpeg"), FileName: proto.String("photo.jpg"),
				LocatorInfo: &backuppb.FilePointer_LocatorInfo{
					Size: 2048, Key: []byte{1, 2, 3},
					TransitCdnKey: proto.String("transit-key"), TransitCdnNumber: proto.Uint32(1),
					IntegrityCheck: &backuppb.FilePointer_LocatorInfo_EncryptedDigest{EncryptedDigest: []byte{9}},
				},
			}}},
		}}}},
	}

	c := newTestConnector(dev)
	ingest := newRealIngest(t)

	c.importHistory(t.Context(), dev, ingest)

	conv, err := ingest.store.ConversationByRemote(t.Context(), "acc1", friend.String())
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}

	messages, _, err := ingest.store.Messages(t.Context(), conv.ID, "", 10)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v, %v; want the attachment-only message stored", messages, err)
	}
	if want := domain.MediaPlaceholder(domain.MediaPhoto); messages[0].Text != want {
		t.Errorf("Text = %q, want the placeholder %q since every stored message needs text", messages[0].Text, want)
	}
	if messages[0].Media == nil || messages[0].Media.Kind != domain.MediaPhoto {
		t.Errorf("Media = %+v, want the attachment's media kept", messages[0].Media)
	}
}

// TestImportHistory_CaptionedAttachment_KeepsBothTextAndMedia checks
// the two places a caption can live in a backup: the message's own
// text body, and the attachment pointer's own caption field. Either
// way, the stored message keeps both its text and its media, never
// text alone and never two messages.
func TestImportHistory_CaptionedAttachment_KeepsBothTextAndMedia(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		bodyText       string
		pointerCaption string
		wantText       string
	}{
		{name: "caption in the message body", bodyText: "look at this", wantText: "look at this"},
		{name: "caption on the attachment pointer only", pointerCaption: "look at this", wantText: "look at this"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			friend := uuid.New()
			friendBytes, err := friend.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}

			dev := newFakeDevice(uuid.New())
			dev.transferMeta = &signalmeow.TransferArchiveMetadata{CDN: 3, Key: "k"}
			dev.recipients = map[uint64]*backuppb.Recipient{
				1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: friendBytes}}},
			}
			dev.chats = []*sstore.BackupChat{{Chat: &backuppb.Chat{Id: 10, RecipientId: 1}}}

			standard := &backuppb.StandardMessage{Attachments: []*backuppb.MessageAttachment{{Pointer: &backuppb.FilePointer{
				ContentType: proto.String("image/jpeg"), Caption: proto.String(tc.pointerCaption),
				LocatorInfo: &backuppb.FilePointer_LocatorInfo{TransitCdnKey: proto.String("k")},
			}}}}
			if tc.bodyText != "" {
				standard.Text = &backuppb.Text{Body: tc.bodyText}
			}
			dev.itemsByChat = map[uint64][]*backuppb.ChatItem{
				10: {{AuthorId: 1, DateSent: 1, Item: &backuppb.ChatItem_StandardMessage{StandardMessage: standard}}},
			}

			c := newTestConnector(dev)
			ingest := newRealIngest(t)

			c.importHistory(t.Context(), dev, ingest)

			conv, err := ingest.store.ConversationByRemote(t.Context(), "acc1", friend.String())
			if err != nil {
				t.Fatalf("ConversationByRemote() = %v", err)
			}

			messages, _, err := ingest.store.Messages(t.Context(), conv.ID, "", 10)
			if err != nil || len(messages) != 1 {
				t.Fatalf("messages = %+v, %v; want exactly one message", messages, err)
			}
			if messages[0].Text != tc.wantText {
				t.Errorf("Text = %q, want %q", messages[0].Text, tc.wantText)
			}
			if messages[0].Media == nil {
				t.Error("Media was not kept alongside the caption")
			}
		})
	}
}

// TestImportHistory_MediaTierOnly_StoresMessageFetchUnavailable
// checks that an attachment with no transit-tier copy (older than
// Signal's transit window, or uploaded only to the media tier) still
// imports its message with the media's own metadata, and that
// fetching it reports unavailable rather than panicking or silently
// losing the message.
func TestImportHistory_MediaTierOnly_StoresMessageFetchUnavailable(t *testing.T) {
	t.Parallel()

	friend := uuid.New()
	friendBytes, err := friend.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.transferMeta = &signalmeow.TransferArchiveMetadata{CDN: 3, Key: "k"}
	dev.recipients = map[uint64]*backuppb.Recipient{
		1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: friendBytes}}},
	}
	dev.chats = []*sstore.BackupChat{{Chat: &backuppb.Chat{Id: 10, RecipientId: 1}}}
	dev.itemsByChat = map[uint64][]*backuppb.ChatItem{
		10: {{AuthorId: 1, DateSent: 7, Item: &backuppb.ChatItem_StandardMessage{StandardMessage: &backuppb.StandardMessage{
			Attachments: []*backuppb.MessageAttachment{{Pointer: &backuppb.FilePointer{
				ContentType: proto.String("image/jpeg"),
				LocatorInfo: &backuppb.FilePointer_LocatorInfo{MediaTierCdnNumber: proto.Uint32(2)},
			}}},
		}}}},
	}

	c := newTestConnector(dev)
	media := newTestMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)
	ingest := newRealIngest(t)

	c.importHistory(t.Context(), dev, ingest)

	conv, err := ingest.store.ConversationByRemote(t.Context(), "acc1", friend.String())
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}

	messages, _, err := ingest.store.Messages(t.Context(), conv.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Media == nil {
		t.Fatalf("messages = %+v, %v; want the media-tier-only message stored with its media metadata", messages, err)
	}

	err = c.FetchMedia(t.Context(), domain.Conversation{RemoteID: friend.String()}, messages[0].RemoteID, "/dev/null", false)
	if !errors.Is(err, errNotImplemented) {
		t.Errorf("FetchMedia() error = %v, want errNotImplemented", err)
	}
}
