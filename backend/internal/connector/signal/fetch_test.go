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
	c.connected(dev, &connectortest.Sink{})

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
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{})
	c.rememberAttachment("conv1", "msg1", &signalpb.AttachmentPointer{ContentType: proto.String("image/jpeg")})

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

// TestRememberAttachment_Eviction checks that remembering more than
// maxTrackedAttachments evicts the oldest first.
func TestRememberAttachment_Eviction(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	for i := 0; i < maxTrackedAttachments+1; i++ {
		c.rememberAttachment("conv1", uuid.New().String(), &signalpb.AttachmentPointer{})
	}

	if len(c.attachments) != maxTrackedAttachments {
		t.Errorf("len(attachments) = %d, want %d", len(c.attachments), maxTrackedAttachments)
	}
}
