.pragma library

// Pure logic for editing one of this account's own sent messages:
// which loaded message, if any, Up in an empty composer or the "Edit"
// action should load, and whether Up means that at all right now.

.import "Timeline.js" as Timeline

// isEditable reports whether m, a loaded message (or ListModel row), is
// something the UI can offer to edit: this account's own message, with
// no attachment, that has already reached the service. A still-pending
// or failed send has no remote id yet, matching the same rule the
// helper's own EditMessage command applies.
function isEditable(m) {
  return !!m && !!m.outgoing && !!m.remoteId && !Timeline.media(m);
}

// lastEditableId returns the id of the newest editable message in
// messages, loaded newest first the way the timeline keeps them, or ""
// when none qualifies.
function lastEditableId(messages) {
  const found = (messages || []).find(isEditable);
  return found ? found.id : '';
}

// upMeansEdit reports whether pressing Up in the composer should load
// the conversation's last editable message for editing, rather than
// moving the caret: only once the composer is empty and there is
// something to edit.
function upMeansEdit(composerText, messages) {
  return composerText.trim().length === 0 && lastEditableId(messages) !== '';
}
