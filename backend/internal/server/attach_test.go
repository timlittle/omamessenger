package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// attachResult mirrors server.attachResult for decoding media.attach's
// reply in a test.
type attachResult struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
	Limit  int64  `json:"limit"`
}

// TestMediaAttach_CopiesAndSanitizesAPickedPhoto confirms a photo
// picked from disk comes back as a copy inside the outgoing media
// area, sanitized with imagesafe, with its own name and dimensions
// kept, not guessed from the file's extension.
func TestMediaAttach_CopiesAndSanitizesAPickedPhoto(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := filepath.Join(t.TempDir(), "vacation.png")
	if err := os.WriteFile(path, pngBytes(t, 3, 2), 0o600); err != nil {
		t.Fatal(err)
	}

	attached, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path})
	if err != nil || attached.Kind != domain.MediaPhoto || attached.Name != "vacation.png" ||
		attached.Width != 3 || attached.Height != 2 || attached.Path == path {
		t.Errorf("media.attach = %+v, %v", attached, err)
	}
}

// TestMediaAttach_RejectsAnOversizedImage confirms a photo declaring
// more bytes than imagesafe's own FullImage profile accepts is refused
// as soon as the file is picked, before the composer shows anything.
func TestMediaAttach_RejectsAnOversizedImage(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := filepath.Join(t.TempDir(), "huge.png")
	oversized := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 33*1024*1024)...)
	if err := os.WriteFile(path, oversized, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path}); code(err) != -32602 {
		t.Errorf("media.attach(oversized image) code = %d, want invalid params", code(err))
	}
}

// TestMediaAttach_AcceptsANonImageFile confirms a plain file is
// attached as-is, with no dimensions, never refused just for not being
// a photo.
func TestMediaAttach_AcceptsANonImageFile(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	attached, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path})
	if err != nil || attached.Kind != domain.MediaFile || attached.Name != "notes.txt" || attached.Width != 0 {
		t.Errorf("media.attach(file) = %+v, %v", attached, err)
	}
}

// TestMediaAttach_RejectsAMissingFile confirms a path that no longer
// exists fails with a safe, invalid-input message rather than an
// internal error.
func TestMediaAttach_RejectsAMissingFile(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := filepath.Join(t.TempDir(), "missing")

	if _, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path}); code(err) != -32602 {
		t.Errorf("media.attach(missing file) code = %d, want invalid params", code(err))
	}
}

// TestMediaAttach_ReportsSizeAndTheDefaultLimit confirms a plain file
// comes back with its own byte count and the 2 GB default limit that
// applies when no connector reports one of its own, for the composer's
// chip to compare against.
func TestMediaAttach_ReportsSizeAndTheDefaultLimit(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	attached, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path})
	if err != nil || attached.Size != 5 || attached.Limit != app.MaxAttachmentSize {
		t.Errorf("media.attach(file) size/limit = %d/%d, %v, want 5/%d", attached.Size, attached.Limit, err, app.MaxAttachmentSize)
	}
}

// TestMediaAttach_RejectsOverTheAccountsLimit confirms a file larger
// than the open conversation's own account reports through its
// connector is refused at media.attach time, naming the limit, rather
// than only being caught later at messages.send.
func TestMediaAttach_RejectsOverTheAccountsLimit(t *testing.T) {
	t.Parallel()

	s := connectWithLimiter(t, fakeLimiter{limits: map[string]int64{"wa file": 10}})
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("more than ten bytes of text"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path, "accountId": "wa"})
	if code(err) != -32602 {
		t.Errorf("media.attach(over account limit) code = %d, want invalid params", code(err))
	}
	if err == nil || !strings.Contains(err.Error(), "10 B") {
		t.Errorf("media.attach(over account limit) err = %v, want it to mention the 10 B limit", err)
	}
}

// TestMediaAttach_AllowsUnderTheAccountsLimit confirms a file within
// the open conversation's own account limit is still accepted, so the
// per-account check only ever narrows what the default allows, never
// widens it.
func TestMediaAttach_AllowsUnderTheAccountsLimit(t *testing.T) {
	t.Parallel()

	s := connectWithLimiter(t, fakeLimiter{limits: map[string]int64{"wa file": 1024}})
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	attached, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path, "accountId": "wa"})
	if err != nil || attached.Limit != 1024 {
		t.Errorf("media.attach(under account limit) = %+v, %v, want limit 1024", attached, err)
	}
}

// TestAccountsList_ReportsAttachmentLimit confirms accounts.list
// carries each account's own attachment limit, for the composer's hint
// before the user has picked a file at all.
func TestAccountsList_ReportsAttachmentLimit(t *testing.T) {
	t.Parallel()

	s := connectWithLimiter(t, fakeLimiter{limits: map[string]int64{"wa file": 1024}})

	type accountResult struct {
		ID              string `json:"id"`
		AttachmentLimit int64  `json:"attachmentLimit"`
	}

	accounts, err := call[[]accountResult](t, s, "accounts.list", nil)
	if err != nil || len(accounts) != 1 || accounts[0].AttachmentLimit != 1024 {
		t.Errorf("accounts.list = %+v, %v, want one account with attachmentLimit 1024", accounts, err)
	}
}

// TestMessagesSend_ReusesAnAttachedFileAndKeepsItsName confirms
// messages.send, given the path and name media.attach already
// returned, sends the message with the attachment's original file
// name kept, not the internal id its outgoing copy is stored under.
func TestMessagesSend_ReusesAnAttachedFileAndKeepsItsName(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	path := filepath.Join(t.TempDir(), "vacation.png")
	if err := os.WriteFile(path, pngBytes(t, 3, 2), 0o600); err != nil {
		t.Fatal(err)
	}

	attached, err := call[attachResult](t, s, "media.attach", map[string]string{"path": path})
	if err != nil {
		t.Fatal(err)
	}

	sent, err := call[domain.Message](t, s, "messages.send", map[string]any{
		"conversationId": "chat", "text": "",
		"attachment": map[string]string{"path": attached.Path, "name": attached.Name},
	})
	if err != nil || sent.Media == nil || sent.Media.FileName != "vacation.png" || sent.Media.Kind != domain.MediaPhoto {
		t.Errorf("messages.send reusing an attached file = %+v, %v", sent, err)
	}
}
