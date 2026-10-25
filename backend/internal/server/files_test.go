package server_test

import "testing"

// TestFilesPick_ReturnsThePathOnASuccessfulPick confirms files.pick
// reaches app.Commands.PickFile and carries its own purpose and title
// through.
func TestFilesPick_ReturnsThePathOnASuccessfulPick(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.filePicker.path, s.filePicker.ok = "/home/user/vacation.jpg", true

	got, err := call[struct {
		Path string `json:"path"`
	}](t, s, "files.pick", map[string]string{"purpose": "attach", "title": "Attach a file"})
	if err != nil || got.Path != "/home/user/vacation.jpg" {
		t.Errorf("files.pick = %+v, %v", got, err)
	}
	if s.filePicker.title != "Attach a file" {
		t.Errorf("title given to the picker = %q, want %q", s.filePicker.title, "Attach a file")
	}
}

// TestFilesPick_OmitsPathWhenCancelled confirms a cancelled pick
// answers with no path and no error: cancelling is never a failure.
func TestFilesPick_OmitsPathWhenCancelled(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.filePicker.ok = false

	got, err := call[struct {
		Path string `json:"path"`
	}](t, s, "files.pick", map[string]string{"purpose": "attach"})
	if err != nil {
		t.Fatalf("files.pick over a cancelled pick: %v", err)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want empty once the user cancelled", got.Path)
	}
}

// TestFilesPick_CarriesAnUnavailableReasonWhenTheDialogCannotShow
// confirms a FilePicker failure reaches the UI as an internal error
// whose data names reason "unavailable": the UI's own cue to fall back
// to a different file picker, never the in-process GTK dialog that
// once crashed the whole shell (see docs/decisions.md).
func TestFilesPick_CarriesAnUnavailableReasonWhenTheDialogCannotShow(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.filePicker.err = errFilePickerUnreachable{}

	_, err := call[struct {
		Path string `json:"path"`
	}](t, s, "files.pick", map[string]string{"purpose": "sound"})
	if code(err) != -32603 {
		t.Fatalf("files.pick with no portal reachable code = %d, want internal error (-32603)", code(err))
	}

	data, ok := errorData[struct {
		Reason string `json:"reason"`
	}](err)
	if !ok || data.Reason != "unavailable" {
		t.Errorf("error data = %+v, ok=%v, want reason %q", data, ok, "unavailable")
	}
}

// errFilePickerUnreachable is a plain error standing in for the
// portal's own dial or OpenFile failure; its text never needs to cross
// the protocol, only app.FilePickError's fixed "unavailable" category
// does.
type errFilePickerUnreachable struct{}

func (errFilePickerUnreachable) Error() string { return "no portal running" }
