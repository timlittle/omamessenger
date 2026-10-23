// Checks that a connector's "link" formatting span renders as a real,
// clickable anchor in the message body - not just escaped text that
// happens to contain "<a" - and that a span whose URL scheme is not
// allowed never does. TextEdit.linkAt(x, y) is Qt's own query for "is
// there a genuine anchor here": it only answers with a URL when the
// rich text engine actually parsed one at that position, so this proves
// the span reached real markup, not merely a string. Its own href is
// now a private "oma-link:<index>" rather than the span's URL (see
// Format.linkHref), so linkAt can only prove an anchor exists there,
// not which one.
//
// What an anchor's own href actually resolves to is worth a real click
// (QtTest's mouseClick) for one specific reason: two spans that carry
// the very same URL but different visible text used to let a click on
// either report whichever span a lookup found first, never the one
// actually touched (see ui/lib/Links.js's own header comment). The
// "disguise" case below drives a real click on the second of two such
// spans and checks MessageBubbleContent's own linkOpenRequested names
// that span's own text, not the first one's.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  readonly property var noAnnotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })

  // disguiseURL is the one destination both spans in the disguise
  // message below carry: the sender's own trick is giving it to an
  // honest-looking span (its own text is the url itself) and a
  // disguise span (unrelated text) alike.
  readonly property string disguiseURL: "https://evil.example/login"
  readonly property string disguiseText: "see " + root.disguiseURL + " or go to Barclays here for your refund"
  readonly property var disguiseSpans: [
    { kind: "link", start: 4, end: 4 + root.disguiseURL.length, url: root.disguiseURL },
    { kind: "link", start: root.disguiseText.indexOf("Barclays here"), end: root.disguiseText.indexOf("Barclays here") + "Barclays here".length, url: root.disguiseURL }
  ]

  // capturedURL and capturedDisplayText record disguised's own
  // linkOpenRequested, the report WindowController.openLink would have
  // decided whether to question.
  property string capturedURL: ""
  property string capturedDisplayText: ""

  // linkAt finds the href TextEdit's rich text engine resolved for the
  // word starting at charIndex in delegate's own "body" TextEdit, or ""
  // when that position is not inside a real anchor.
  function linkAt(delegate: var, charIndex: int): string {
    const body = Check.find(delegate, "body");
    const rect = body.positionToRectangle(charIndex);
    return body.linkAt(rect.x + 1, rect.y + rect.height / 2);
  }

  // clickAt fires a real mouse click at charIndex's own position in
  // delegate's "body" TextEdit, the way a person actually triggers
  // onLinkActivated rather than calling it directly.
  function clickAt(delegate: var, charIndex: int): void {
    const body = Check.find(delegate, "body");
    const rect = body.positionToRectangle(charIndex);
    t.mouseClick(body, rect.x + 1, rect.y + rect.height / 2);
  }

  function run(): void {
    const href = root.linkAt(linked, 4); // "link" starts at index 4 of "see link here"
    if (!href || !href.startsWith("oma-link:"))
      return Check.fail("linkAt over the formatted word found " + JSON.stringify(href) + ", want one of this file's own private anchors");

    const droppedHref = root.linkAt(unsafe, 4);
    if (droppedHref)
      return Check.fail("a disallowed scheme rendered as a clickable link: " + droppedHref);

    const unsafeBody = Check.find(unsafe, "body");
    if (unsafeBody.text.indexOf("<img") !== -1)
      return Check.fail("rendered an <img>, which must never happen for message text");

    const disguiseStart = root.disguiseSpans[1].start;
    root.clickAt(disguised, disguiseStart + 2); // inside "Barclays here", the second span
    if (root.capturedDisplayText !== "Barclays here")
      return Check.fail("a click on the disguise span reported " + JSON.stringify(root.capturedDisplayText) + ", want its own text \"Barclays here\"");
    if (root.capturedURL !== root.disguiseURL)
      return Check.fail("a click on the disguise span reported url " + JSON.stringify(root.capturedURL) + ", want " + JSON.stringify(root.disguiseURL));

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

      MessageDelegate {
        id: disguised
        width: 400
        annotation: root.noAnnotation
        message: ({
          id: "c", outgoing: false, status: "received", created: 0, senderName: "",
          text: root.disguiseText,
          formatting: root.disguiseSpans
        })
        onLinkOpenRequested: (url, displayText) => {
          root.capturedURL = url;
          root.capturedDisplayText = displayText;
        }
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  // Layout settles over a few frames, so measure after a short delay.
  Timer {
    running: true
    interval: 100
    onTriggered: root.run()
  }
}
