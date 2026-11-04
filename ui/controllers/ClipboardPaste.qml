import QtQuick
import "../lib/Paste.js" as Paste

// ClipboardPaste is the one paste round trip every editable field
// outside the composer shares for Ctrl+V and Shift+Insert: reads the
// clipboard's text through the helper's bounded clipboard.readText,
// never Qt's own unbounded in-process reader (see docs/decisions.md).
// The composer has its own larger version instead (see
// ComposerController.paste).
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance the owning controller was given.
  property var service: null

  // pasteInto reads the clipboard's text and inserts it at field's
  // cursor, bounded by field's own maximumLength. Refuses a read-only
  // or non-plain-text field up front: field.insert() does not check
  // readOnly itself, and a RichText field would insert raw HTML (see
  // docs/decisions.md).
  function pasteInto(field: var): void {
    if (!root.service || !field || typeof field.insert !== "function") return;
    if (field.readOnly === true) return;
    if (field.textFormat !== undefined && field.textFormat !== TextEdit.PlainText) return;

    root.service.request("clipboard.readText", {}, function(error, result) {
      if (error) return;

      try {
        field.insert(field.cursorPosition, Paste.forSingleLine(result.text));
      } catch (e) {
        // field was destroyed before the reply arrived; nothing left to paste into.
      }
    });
  }
}
