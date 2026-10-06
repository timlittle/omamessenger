// Checks MessageDelegate: sender names in groups, the read glyph, the
// retry line on a failed message, and that rich text escapes markup
// while linkifying URLs.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui/components"

ShellRoot {
  id: root

  property var retried: []
  property real now: Date.now()

  // fail stops the test with a reason on stderr and reports failure to
  // its caller, so each check can bail out with "return root.fail(...)".
  function fail(reason: string): bool {
    console.error("FAIL " + reason);
    Qt.exit(1);
    return false;
  }

  // collect walks the item tree gathering every node with a text property,
  // since both Text and TextEdit expose one.
  function collect(item, out) {
    if (typeof item.text === "string") out.push(item);
    for (const child of item.children) collect(child, out);
  }

  // findText returns the first collected node whose text matches exactly.
  function findText(out, text) {
    return out.find(node => node.text === text) ?? null;
  }

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 500
    visible: true

    MessageDelegate {
      id: delegate
      width: 360
      message: ({ id: "m1", senderId: "s1", senderName: "Alex", text: "hi", outgoing: false, status: "delivered", created: root.now })
      annotation: ({ showDay: false, dayLabel: "", showSender: true, groupedWithOlder: false })
      isGroup: true
      nowMs: root.now
      onRetry: id => root.retried.push(id)
    }
  }

  TestCase {
    id: t
    when: false
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives the delegate through each scenario and checks the outcome.
  // Each check returns false after a failure, so run() stops immediately
  // and the earlier Qt.exit(1) is the one that takes effect.
  function run(): void {
    if (!root.checkGroupSender()) return;
    if (!root.checkOutgoingReadGlyph()) return;
    if (!root.checkFailedRetry()) return;
    if (!root.checkRichText()) return;

    console.log("PASS MessageDelegate");
    Qt.exit(0);
  }

  // checkGroupSender verifies an incoming group message shows the sender.
  function checkGroupSender(): bool {
    const nodes = [];
    root.collect(delegate, nodes);
    const sender = root.findText(nodes, "Alex");
    if (!sender || !sender.visible) return root.fail("sender name \"Alex\" not shown for a grouped incoming message");
    return true;
  }

  // checkOutgoingReadGlyph verifies an outgoing read message hides the
  // sender name and shows the read glyph in the accent colour.
  function checkOutgoingReadGlyph(): bool {
    delegate.message = { id: "m2", senderId: "me", senderName: "Me", text: "ok", outgoing: true, status: "read", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };

    const nodes = [];
    root.collect(delegate, nodes);
    const sender = root.findText(nodes, "Me");
    if (sender && sender.visible) return root.fail("sender name shown for an outgoing message");

    const glyph = root.findText(nodes, "✓✓");
    if (!glyph || !glyph.visible) return root.fail("read glyph not shown for an outgoing read message");
    if (!Qt.colorEqual(glyph.color, Color.accent)) return root.fail("read glyph is not drawn in the accent colour");
    return true;
  }

  // checkFailedRetry verifies the retry line appears and clicking it emits
  // retry with the message id.
  function checkFailedRetry(): bool {
    delegate.message = { id: "m3", senderId: "me", senderName: "Me", text: "nope", outgoing: true, status: "failed", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };
    t.waitForRendering(delegate);

    const nodes = [];
    root.collect(delegate, nodes);
    const retryLine = root.findText(nodes, "Not sent · r to retry");
    if (!retryLine || !retryLine.visible) return root.fail("retry line not shown for a failed outgoing message");
    if (!Qt.colorEqual(retryLine.color, Color.urgent)) return root.fail("retry line is not drawn in the urgent colour");

    root.retried = [];
    t.mouseClick(retryLine);
    if (JSON.stringify(root.retried) !== '["m3"]') return root.fail("retry " + JSON.stringify(root.retried) + ", want [\"m3\"]");
    return true;
  }

  // checkRichText verifies markup is escaped and a URL becomes a link.
  function checkRichText(): bool {
    delegate.message = { id: "m4", senderId: "s1", senderName: "Alex", text: "<b>bold</b> see http://example.com/x", outgoing: false, status: "delivered", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };

    const nodes = [];
    root.collect(delegate, nodes);
    const body = nodes.find(node => node.text.indexOf("bold") >= 0);
    if (!body) return root.fail("message body not found");
    if (body.text.indexOf("&lt;b&gt;") < 0) return root.fail("markup was not escaped: " + body.text);
    if (body.text.indexOf("<a href=") < 0) return root.fail("URL was not linkified: " + body.text);
    return true;
  }
}
