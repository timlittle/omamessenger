import QtQuick
import "../lib/Paste.js" as Paste

// ClipboardPaste is the one paste round trip every editable field
// outside the composer shares for Ctrl+V and Shift+Insert, reached
// through Panel.qml's own routeKey (the "field.paste" action in
// Keymap.js): read the clipboard's text through the helper's bounded
// clipboard.readText, never through Qt's own unbounded in-process
// reader, which has no size or time limit of its own (see
// docs/decisions.md). The composer has its own larger version instead
// (see ComposerController.paste), since it also tries a pasted image
// first and must keep its own line breaks; every field reached this
// way is single-line, so this always collapses them.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance the owning controller was given.
  property var service: null

  // pasteInto reads the clipboard's text and inserts it at field's own
  // cursor position, bounded by field's own maximumLength the same way
  // typing already is. field is whatever currently holds keyboard
  // focus (see Panel.qml's own routeKey): most of the time that is a
  // TextField or TextArea, but it can be a plain Item with no insert()
  // of its own (nothing editable focused) or null (nothing focused at
  // all), either of which does nothing here, the same as a field
  // destroyed - its Loader closing as its overlay closes, say - before
  // the reply arrives: there is nothing left to paste into either way.
  // A security review found field.insert() does not check readOnly at
  // all, and, given a RichText field, inserts HTML rather than plain
  // text - MessageBubbleContent's own read-only message body is
  // exactly that shape, reachable because it sets selectByMouse so a
  // click can give it real focus - so a clipboard offer carrying
  // "<img src=...>" would make this fetch and decode whatever it
  // points at. Both are refused outright, before clipboard.readText
  // is even asked for, never merely left unapplied afterward.
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
