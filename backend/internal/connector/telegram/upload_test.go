package telegram

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// writeUploadFile writes data to a new file under the test's temp
// directory and returns its path.
func writeUploadFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestSend_UploadsAPhotoWithItsCaption(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.UploadSaveFilePartRequest{}, &tg.BoolTrue{})
	f.reply(&tg.MessagesSendMediaRequest{}, &tg.UpdateShortSentMessage{ID: 77})

	path := writeUploadFile(t, "photo.jpg", []byte("fake jpeg bytes"))
	m := domain.Message{ID: "m1", Text: "look at this", Media: &domain.Media{Kind: domain.MediaPhoto, Path: path, FileName: "photo.jpg"}}

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	if err := c.Send(t.Context(), chatWithNadia, m); err != nil {
		t.Fatal(err)
	}

	part, ok := f.sent()[0].(*tg.UploadSaveFilePartRequest)
	if !ok || string(part.Bytes) != "fake jpeg bytes" {
		t.Errorf("part = %+v, want the file's bytes uploaded", f.sent()[0])
	}

	req, ok := f.sent()[1].(*tg.MessagesSendMediaRequest)
	if !ok || req.Message != "look at this" {
		t.Fatalf("request = %+v, want the caption kept", f.sent()[1])
	}

	if _, ok := req.Media.(*tg.InputMediaUploadedPhoto); !ok {
		t.Errorf("media = %T, want an uploaded photo", req.Media)
	}

	want := []string{"outgoing m1 77 " + domain.StatusSent}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestSend_UploadsADocumentWithItsNameAndType(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.UploadSaveFilePartRequest{}, &tg.BoolTrue{})
	f.reply(&tg.MessagesSendMediaRequest{}, &tg.UpdateShortSentMessage{ID: 78})

	path := writeUploadFile(t, "report.pdf", []byte("%PDF-1.4 fake"))
	m := domain.Message{
		ID: "m2", Text: domain.MediaPlaceholder(domain.MediaFile),
		Media: &domain.Media{Kind: domain.MediaFile, Path: path, FileName: "report.pdf"},
	}

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	if err := c.Send(t.Context(), chatWithNadia, m); err != nil {
		t.Fatal(err)
	}

	req, ok := f.sent()[1].(*tg.MessagesSendMediaRequest)
	if !ok {
		t.Fatalf("request = %+v, want messages.sendMedia", f.sent()[1])
	}

	if req.Message != "" {
		t.Errorf("caption = %q, want the placeholder stripped to no caption", req.Message)
	}

	doc, ok := req.Media.(*tg.InputMediaUploadedDocument)
	if !ok {
		t.Fatalf("media = %T, want an uploaded document", req.Media)
	}

	name, ok := documentFileName(doc.Attributes)
	if !ok || name != "report.pdf" {
		t.Errorf("file name attribute = %q, %t, want %q", name, ok, "report.pdf")
	}
}

// documentFileName finds a document attribute list's file name, if any.
func documentFileName(attrs []tg.DocumentAttributeClass) (string, bool) {
	for _, attr := range attrs {
		if a, ok := attr.(*tg.DocumentAttributeFilename); ok {
			return a.FileName, true
		}
	}

	return "", false
}
