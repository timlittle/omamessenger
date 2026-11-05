package signal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestFetchMedia_Unknown checks that fetching media this connector
// never remembered an attachment for fails rather than writing an
// empty file.
func TestFetchMedia_Unknown(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, nil)

	err := c.FetchMedia(t.Context(), domain.Conversation{RemoteID: "conv1"}, "msg1", filepath.Join(t.TempDir(), "out"), false)
	if err == nil {
		t.Error("FetchMedia() succeeded for an unknown attachment, want an error")
	}
}

// TestFetchMedia_Downloads checks that a remembered attachment
// downloads to the given path.
func TestFetchMedia_Downloads(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.downloadData = []byte("plaintext bytes")
	media := newTestMediaStore(t)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, media)
	c.rememberAttachment(t.Context(), "conv1", "msg1", &signalpb.AttachmentPointer{ContentType: proto.String("image/jpeg")})

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.FetchMedia(t.Context(), domain.Conversation{RemoteID: "conv1"}, "msg1", path, false); err != nil {
		t.Fatalf("FetchMedia() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fetched file: %v", err)
	}
	if string(got) != "plaintext bytes" {
		t.Errorf("fetched file = %q", got)
	}
}

// TestFetchMedia_SurvivesARestart checks that an attachment remembered
// by one connector instance can still be fetched by a second one built
// around the same on-disk media store, standing in for a helper
// restart.
func TestFetchMedia_SurvivesARestart(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "signal")
	before, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.downloadData = []byte("plaintext bytes")
	first := newTestConnector(dev)
	first.connected(dev, &connectortest.Sink{}, before)
	first.rememberAttachment(t.Context(), "conv1", "msg1", &signalpb.AttachmentPointer{ContentType: proto.String("image/jpeg")})
	if err := before.close(); err != nil {
		t.Fatal(err)
	}

	after, err := openMediaStore(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = after.close() }()

	second := newTestConnector(dev)
	second.connected(dev, &connectortest.Sink{}, after)

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := second.FetchMedia(t.Context(), domain.Conversation{RemoteID: "conv1"}, "msg1", path, false); err != nil {
		t.Fatalf("FetchMedia() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fetched file: %v", err)
	}
	if string(got) != "plaintext bytes" {
		t.Errorf("fetched file = %q", got)
	}
}

// TestRememberAttachment_NoMediaStore checks that remembering an
// attachment with no media store wired in is a harmless no-op, rather
// than panicking.
func TestRememberAttachment_NoMediaStore(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	c.rememberAttachment(t.Context(), "conv1", "msg1", &signalpb.AttachmentPointer{})

	if got := c.attachmentFor(t.Context(), "conv1", "msg1"); got != nil {
		t.Errorf("attachmentFor() = %+v, want nil with no media store", got)
	}
}
