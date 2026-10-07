// Checks the composer: trimmed submit, whitespace ignored, cleared input,
// a placeholder naming the conversation, and the attachment chip.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var sent: []
  property var attached: []
  property int removeRequests: 0

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 200
    visible: true

    Composer {
      id: composer

      anchors.fill: parent
      title: "Mum"
      onSubmitted: text => root.sent.push(text)
      onFileAttached: path => root.attached.push(path)
      onAttachmentRemoveRequested: root.removeRequests++
    }
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives the composer and checks the outcome.
  function run(): void {
    composer.text = "  hello  ";
    composer.submit();
    composer.text = "   ";
    composer.submit();

    if (JSON.stringify(root.sent) !== '["hello"]')
      return Check.fail("sent " + JSON.stringify(root.sent) + ", want [\"hello\"]");
    if (composer.text !== "   ")
      return Check.fail("whitespace-only text was cleared or sent");
    if (composer.input.placeholderText.indexOf("Mum") < 0)
      return Check.fail("placeholder \"" + composer.input.placeholderText + "\" does not name the conversation");

    return root._checkAttachment();
  }

  // _checkAttachment drives the attachment chip: it appears with a name,
  // lets a captionless message submit, and a click on its ✕ asks to
  // remove it.
  function _checkAttachment(): void {
    const chip = Check.find(composer, "attachmentChip");
    if (!chip) return Check.fail("no attachmentChip found");
    if (chip.visible) return Check.fail("chip visible before an attachment was set");

    composer.attachmentPath = "/tmp/photo.png";
    if (!chip.visible) return Check.fail("chip not visible with an attachment set");

    const name = Check.find(composer, "attachmentName");
    if (!name || name.text !== "photo.png")
      return Check.fail("attachment name = " + (name && name.text) + ", want photo.png");

    composer.text = "";
    composer.submit();
    if (JSON.stringify(root.sent) !== '["hello",""]')
      return Check.fail("a captionless attachment did not submit: sent " + JSON.stringify(root.sent));

    const removeButton = Check.find(composer, "removeAttachmentButton");
    if (!removeButton) return Check.fail("no removeAttachmentButton found");
    removeButton.clicked();
    if (root.removeRequests !== 1)
      return Check.fail("attachmentRemoveRequested fired " + root.removeRequests + " times, want 1");

    console.log("PASS Composer");
    Qt.exit(0);
  }
}
