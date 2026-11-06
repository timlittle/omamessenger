package signal

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSend_DirectReportsSent checks that sending a plain text message
// to a direct chat sends it to that account and reports it sent with
// a remote id this connector can act on later.
func TestSend_DirectReportsSent(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	other := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}
	c.connected(dev, sink, mustOpenMediaStore(t))

	conv := domain.Conversation{RemoteID: other.String()}
	if err := c.Send(t.Context(), conv, domain.Message{ID: "m1", Text: "hello"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if len(dev.sentDirect) != 1 || dev.sentDirect[0].recipient != other {
		t.Fatalf("sentDirect = %+v, want one message to %v", dev.sentDirect, other)
	}
	if dev.sentDirect[0].content.GetDataMessage().GetBody() != "hello" {
		t.Errorf("sent body = %q, want %q", dev.sentDirect[0].content.GetDataMessage().GetBody(), "hello")
	}
	updates := sink.Outgoing("m1")
	if len(updates) != 1 || updates[0].Status != domain.StatusSent {
		t.Errorf("sink.Outgoing(\"m1\") = %+v, want one sent status", updates)
	}
}

// TestSend_GroupUsesSendGroup checks that sending to a group chat id
// goes through sendGroup rather than sendDirect.
func TestSend_GroupUsesSendGroup(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	groupID := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if err := c.Send(t.Context(), domain.Conversation{RemoteID: groupID}, domain.Message{ID: "m1", Text: "hi"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if len(dev.sentGroup) != 1 || len(dev.sentDirect) != 0 {
		t.Errorf("sentGroup = %d, sentDirect = %d, want 1, 0", len(dev.sentGroup), len(dev.sentDirect))
	}
}

// TestSend_UploadsAttachment checks that a message carrying media
// uploads it first and attaches the resulting pointer.
func TestSend_UploadsAttachment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := dir + "/photo.jpg"
	writeTestFile(t, path, []byte("\xff\xd8\xff\xe0fake jpeg"))

	dev := newFakeDevice(uuid.New())
	dev.uploadPtr = &signalpb.AttachmentPointer{}
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	m := domain.Message{ID: "m1", Media: &domain.Media{Kind: domain.MediaPhoto, Path: path}}
	if err := c.Send(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, m); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if len(dev.uploaded) != 1 {
		t.Fatalf("uploaded = %d files, want 1", len(dev.uploaded))
	}
	if len(dev.sentDirect) != 1 || len(dev.sentDirect[0].content.GetDataMessage().GetAttachments()) != 1 {
		t.Errorf("sent attachments = %+v, want one", dev.sentDirect)
	}
}

// TestSend_UnregisteredRecipientIsPermanent checks that an
// unregistered-user refusal from signalmeow is classified as
// permanent, so the retry scheduler stops trying.
func TestSend_UnregisteredRecipientIsPermanent(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.sendDirectErr = signalmeow.ErrUnregisteredUser
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	err := c.Send(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, domain.Message{ID: "m1", Text: "hi"})
	if !errors.Is(err, connector.ErrSendPermanent) {
		t.Errorf("Send() error = %v, want connector.ErrSendPermanent", err)
	}
}

// TestSend_OtherFailureIsRetryable checks that an ordinary send
// failure is not classified as permanent, so the retry scheduler
// tries again.
func TestSend_OtherFailureIsRetryable(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.sendDirectErr = errors.New("network trouble")
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	err := c.Send(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, domain.Message{ID: "m1", Text: "hi"})
	if err == nil || errors.Is(err, connector.ErrSendPermanent) {
		t.Errorf("Send() error = %v, want a retryable error", err)
	}
}

// TestSend_UploadFailurePropagates checks that an attachment that
// fails to upload fails the whole send, with no message sent.
func TestSend_UploadFailurePropagates(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.uploadErr = errors.New("upload failed")
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	path := t.TempDir() + "/photo.jpg"
	writeTestFile(t, path, []byte("fake jpeg"))
	m := domain.Message{ID: "m1", Media: &domain.Media{Kind: domain.MediaPhoto, Path: path}}
	if err := c.Send(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, m); err == nil {
		t.Error("Send() with a missing attachment file succeeded, want an error")
	}
	if len(dev.sentDirect) != 0 {
		t.Errorf("sentDirect = %d, want 0", len(dev.sentDirect))
	}
}

// TestSend_InvalidConversationID checks that sending to a
// conversation remote id that is neither a group nor a plain account
// id is refused with an error.
func TestSend_InvalidConversationID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	err := c.Send(t.Context(), domain.Conversation{RemoteID: "not-an-account-id"}, domain.Message{ID: "m1", Text: "hi"})
	if err == nil {
		t.Error("Send() with an invalid conversation id succeeded, want an error")
	}
}

// TestWireCaption_MediaPlaceholderBecomesEmpty checks that the
// placeholder caption media with no caption of its own gets is never
// sent as real text.
func TestWireCaption_MediaPlaceholderBecomesEmpty(t *testing.T) {
	t.Parallel()

	m := domain.Message{Text: domain.MediaPlaceholder(domain.MediaPhoto), Media: &domain.Media{Kind: domain.MediaPhoto}}
	if got := wireCaption(m); got != "" {
		t.Errorf("wireCaption() = %q, want empty", got)
	}
}
