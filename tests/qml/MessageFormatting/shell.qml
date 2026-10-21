// Checks that a connector's "link" formatting span renders as a real,
// clickable anchor in the message body - not just escaped text that
// happens to contain "<a" - and that a span whose URL scheme is not
// allowed never does. TextEdit.linkAt(x, y) is Qt's own query for "is
// there a genuine anchor here": it only answers with a URL when the
// rich text engine actually parsed one at that position, so this proves
// the span reached real markup, not merely a string. It stops short of
// firing a real click (QtTest's mouseClick) to prove that, since
// linkAt already does; MessageBubbleContent's own onLinkActivated only
// reports linkOpenRequested rather than opening anything itself (see
// WindowController.openExternalLink, the one place that ever reaches
// the real Qt.openUrlExternally), so there is nothing further a real
// click here would prove.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  readonly property var noAnnotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })

  // linkAt finds the href TextEdit's rich text engine resolved for the
  // word starting at charIndex in delegate's own "body" TextEdit, or ""
  // when that position is not inside a real anchor.
  function linkAt(delegate: var, charIndex: int): string {
    const body = Check.find(delegate, "body");
    const rect = body.positionToRectangle(charIndex);
    return body.linkAt(rect.x + 1, rect.y + rect.height / 2);
  }

  function run(): void {
    const url = root.linkAt(linked, 4); // "link" starts at index 4 of "see link here"
    if (url !== "https://example.com")
      return Check.fail("linkAt over the formatted word found " + JSON.stringify(url) + ", want the span's url");

    const droppedUrl = root.linkAt(unsafe, 4);
    if (droppedUrl)
      return Check.fail("a disallowed scheme rendered as a clickable link: " + droppedUrl);

    const unsafeBody = Check.find(unsafe, "body");
    if (unsafeBody.text.indexOf("<img") !== -1)
      return Check.fail("rendered an <img>, which must never happen for message text");

    console.log("PASS MessageFormatting");
    Qt.exit(0);
  }

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 300
    visible: true

    Column {
      width: 400

      MessageDelegate {
        id: linked
        width: 400
        annotation: root.noAnnotation
        message: ({
          id: "a", outgoing: false, status: "received", created: 0, senderName: "",
          text: "see link here",
          formatting: [{ kind: "link", start: 4, end: 8, url: "https://example.com" }]
        })
      }

      MessageDelegate {
        id: unsafe
        width: 400
        annotation: root.noAnnotation
        message: ({
          id: "b", outgoing: false, status: "received", created: 0, senderName: "",
          text: "see link here",
          formatting: [{ kind: "link", start: 4, end: 8, url: "javascript:alert(1)" }]
        })
      }
    }
  }

  // Layout settles over a few frames, so measure after a short delay.
  Timer {
    running: true
    interval: 100
    onTriggered: root.run()
  }
}
