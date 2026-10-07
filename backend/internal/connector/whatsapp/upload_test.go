package whatsapp

// Send's attachment path is driven through a fake device and the
// in-memory media store connectedToWithMedia wires in, as send_test.go
// drives plain text, because the fake is the only way to see what
// Connector asked whatsmeow to upload and send without reaching
// WhatsApp's servers.

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// writeAttachment writes data to a new file under the test's temp
// directory and returns its path, as an outgoing message's Media.Path
// already points to a file the app layer copied into place.
func writeAttachment(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// testPNG returns a small, valid PNG's bytes, for tests that need
// image data the standard library can actually decode.
func testPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 32))); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func TestSend_UploadsAPhotoWithItsCaptionAndThumbnail(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.nextMessageID = "wire-1"
	dev.uploadResp = whatsmeow.UploadResponse{
		DirectPath: "/v/new", MediaKey: []byte("key"), FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), FileLength: 20,
	}
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	path := writeAttachment(t, "photo.png", testPNG(t))
	m := domain.Message{ID: "local-1", Text: "look", Media: &domain.Media{Kind: domain.MediaPhoto, Path: path, FileName: "photo.png"}}

	if err := c.Send(t.Context(), directChat, m); err != nil {
		t.Fatal(err)
	}

	if len(dev.uploadCalls) != 1 || dev.uploadCalls[0].kind != mediaKindImage {
		t.Fatalf("uploadCalls = %+v, want one image upload", dev.uploadCalls)
	}

	img := dev.sent[0].msg.GetImageMessage()
	if img == nil || img.GetCaption() != "look" || img.GetDirectPath() != "/v/new" {
		t.Fatalf("image message = %+v, want the uploaded reference and caption", img)
	}
	if img.GetWidth() != 64 || img.GetHeight() != 32 {
		t.Errorf("dimensions = %dx%d, want 64x32", img.GetWidth(), img.GetHeight())
	}
	if len(img.GetJPEGThumbnail()) == 0 {
		t.Error("image message carries no thumbnail")
	}

	if !sink.Has("outgoing local-1 wire-1 " + domain.StatusSent) {
		t.Errorf("events = %q, want local-1 reported sent", sink.Lines())
	}

	ref, ok, err := c.mediaFor().get(t.Context(), directChat.RemoteID, "wire-1")
	if err != nil || !ok || ref.DirectPath != "/v/new" || ref.Kind != mediaKindImage {
		t.Errorf("saved reference = %+v, %v, %v, want the uploaded reference saved under the wire id", ref, ok, err)
	}
}

func TestSend_UploadsAVideoWithItsCaption(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.nextMessageID = "wire-2"
	dev.uploadResp = whatsmeow.UploadResponse{DirectPath: "/v/video", FileLength: 99}
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	path := writeAttachment(t, "clip.mp4", []byte("fake mp4 bytes"))
	m := domain.Message{
		ID: "local-2", Text: domain.MediaPlaceholder(domain.MediaVideo),
		Media: &domain.Media{Kind: domain.MediaVideo, Path: path, FileName: "clip.mp4"},
	}

	if err := c.Send(t.Context(), directChat, m); err != nil {
		t.Fatal(err)
	}

	if len(dev.uploadCalls) != 1 || dev.uploadCalls[0].kind != mediaKindVideo {
		t.Fatalf("uploadCalls = %+v, want one video upload", dev.uploadCalls)
	}

	video := dev.sent[0].msg.GetVideoMessage()
	if video == nil || video.GetDirectPath() != "/v/video" {
		t.Fatalf("video message = %+v, want the uploaded reference", video)
	}
	if video.GetCaption() != "" {
		t.Errorf("caption = %q, want the placeholder stripped to no caption", video.GetCaption())
	}
}

func TestSend_UploadsADocumentWithItsFileName(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.nextMessageID = "wire-3"
	dev.uploadResp = whatsmeow.UploadResponse{DirectPath: "/v/doc"}
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	path := writeAttachment(t, "report.pdf", []byte("%PDF-1.4 fake"))
	m := domain.Message{ID: "local-3", Text: "see attached", Media: &domain.Media{Kind: domain.MediaFile, Path: path, FileName: "report.pdf"}}

	if err := c.Send(t.Context(), directChat, m); err != nil {
		t.Fatal(err)
	}

	doc := dev.sent[0].msg.GetDocumentMessage()
	if doc == nil || doc.GetFileName() != "report.pdf" || doc.GetCaption() != "see attached" {
		t.Fatalf("document message = %+v, want the file name and caption kept", doc)
	}
	if len(dev.uploadCalls) != 1 || dev.uploadCalls[0].kind != mediaKindDocument {
		t.Errorf("uploadCalls = %+v, want one document upload", dev.uploadCalls)
	}
}

func TestSend_QuotesAReplyWhenSendingAnAttachment(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	path := writeAttachment(t, "report.pdf", []byte("fake"))
	m := domain.Message{
		ID: "local-4", Text: "see attached", ReplyTo: &domain.Reply{RemoteID: "quoted-1", SenderName: "Nadia"},
		Media: &domain.Media{Kind: domain.MediaFile, Path: path, FileName: "report.pdf"},
	}

	if err := c.Send(t.Context(), directChat, m); err != nil {
		t.Fatal(err)
	}

	ctxInfo := dev.sent[0].msg.GetDocumentMessage().GetContextInfo()
	if ctxInfo.GetStanzaID() != "quoted-1" {
		t.Errorf("context = %+v, want it to quote quoted-1", ctxInfo)
	}
}

func TestSend_FailsForAnUnreadableAttachment(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	m := domain.Message{ID: "local-5", Media: &domain.Media{Kind: domain.MediaPhoto, Path: filepath.Join(t.TempDir(), "missing.jpg")}}
	if err := c.Send(t.Context(), directChat, m); err == nil {
		t.Error("Send with an unreadable attachment = nil error, want one")
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent when the file cannot be read", dev.sent)
	}
}

func TestAttachmentKind_MapsDomainKindsOrReportsUnsupported(t *testing.T) {
	t.Parallel()

	tests := map[string]mediaKind{
		domain.MediaPhoto: mediaKindImage,
		domain.MediaVideo: mediaKindVideo,
		domain.MediaFile:  mediaKindDocument,
	}
	for domainKind, want := range tests {
		got, err := attachmentKind(domainKind)
		if err != nil || got != want {
			t.Errorf("attachmentKind(%q) = %q, %v, want %q, nil", domainKind, got, err, want)
		}
	}

	if _, err := attachmentKind(domain.MediaLink); !errors.Is(err, errUnsupportedAttachment) {
		t.Errorf("attachmentKind(link) = %v, want errUnsupportedAttachment", err)
	}
}

func TestImageDimensions_ReportsZeroForUndecodableData(t *testing.T) {
	t.Parallel()

	if w, h := imageDimensions([]byte("not an image")); w != 0 || h != 0 {
		t.Errorf("imageDimensions = %d, %d, want 0, 0", w, h)
	}
}

func TestJPEGThumbnail_ReturnsNilForUndecodableData(t *testing.T) {
	t.Parallel()

	if got := jpegThumbnail([]byte("not an image")); got != nil {
		t.Errorf("jpegThumbnail = %v, want nil", got)
	}
}

func TestShrinkThumbnail_ScalesDownOnlyWhenLargerThanTheLimit(t *testing.T) {
	t.Parallel()

	small := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if got := shrinkThumbnail(small, 48); got != small {
		t.Error("shrinkThumbnail resized an image already within the limit")
	}

	large := image.NewRGBA(image.Rect(0, 0, 200, 100))
	got := shrinkThumbnail(large, 48)
	b := got.Bounds()
	if b.Dx() != 48 || b.Dy() != 24 {
		t.Errorf("shrinkThumbnail size = %dx%d, want 48x24", b.Dx(), b.Dy())
	}
}

func TestScaledThumbSize_NeverGoesBelowOnePixel(t *testing.T) {
	t.Parallel()

	if got := scaledThumbSize(1, 0.001); got != 1 {
		t.Errorf("scaledThumbSize = %d, want 1", got)
	}
}

func TestSend_ReportsFailureWhenUploadFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.uploadErr = errors.New("server rejected upload")
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	path := writeAttachment(t, "photo.png", testPNG(t))
	m := domain.Message{ID: "local-6", Media: &domain.Media{Kind: domain.MediaPhoto, Path: path, FileName: "photo.png"}}

	if err := c.Send(t.Context(), directChat, m); !errors.Is(err, dev.uploadErr) {
		t.Errorf("Send = %v, want it to wrap the device's upload error", err)
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent when the upload fails", dev.sent)
	}
	if len(sink.Outgoing("local-6")) != 0 {
		t.Errorf("outgoing updates = %v, want none for a failed send", sink.Outgoing("local-6"))
	}
}
