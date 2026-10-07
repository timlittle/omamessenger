// Checks MessageDelegate: sender names in groups, the read glyph, the
// retry line on a failed message, and that rich text escapes markup
// while linkifying URLs.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var retried: []
  // wanted records the ids of messages whose photo was asked for.
  // opened records the ids of messages whose media was asked to open.
  property var opened: []
  property var wanted: []
  property var reacted: []
  property var pickerRequests: []
  property real now: Date.now()

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
      onMediaWanted: id => root.wanted.push(id)
      onMediaOpen: id => root.opened.push(id)
      onReact: (id, emoji) => root.reacted.push(id + " " + emoji)
      onReactPickerRequested: id => root.pickerRequests.push(id)
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
    if (!root.checkEditedLabel()) return;
    if (!root.checkFailedRetry()) return;
    if (!root.checkRichText()) return;
    if (!root.checkLineBreaks()) return;
    if (!root.checkLinkPreview()) return;
    if (!root.checkPhoto()) return;
    if (!root.checkVideoAndFile()) return;
    if (!root.checkReactionChips()) return;

    console.log("PASS MessageDelegate");
    Qt.exit(0);
  }

  // checkGroupSender verifies an incoming group message shows the sender.
  function checkGroupSender(): bool {
    const nodes = Check.texts(delegate);
    const sender = root.findText(nodes, "Alex");
    if (!sender || !sender.visible) return Check.fail("sender name \"Alex\" not shown for a grouped incoming message");
    return true;
  }

  // checkOutgoingReadGlyph verifies an outgoing read message hides the
  // sender name and shows the read glyph in the accent colour.
  function checkOutgoingReadGlyph(): bool {
    delegate.message = { id: "m2", senderId: "me", senderName: "Me", text: "ok", outgoing: true, status: "read", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };

    const nodes = Check.texts(delegate);
    const sender = root.findText(nodes, "Me");
    if (sender && sender.visible) return Check.fail("sender name shown for an outgoing message");

    const glyph = root.findText(nodes, "✓✓");
    if (!glyph || !glyph.visible) return Check.fail("read glyph not shown for an outgoing read message");
    if (!Qt.colorEqual(glyph.color, Color.accent)) return Check.fail("read glyph is not drawn in the accent colour");
    return true;
  }

  // checkEditedLabel verifies the "edited" label shows only for a message
  // the service reported changed.
  function checkEditedLabel(): bool {
    delegate.message = { id: "me1", senderId: "s1", senderName: "Alex", text: "fixed", outgoing: false, status: "delivered", created: root.now, edited: true };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };

    let label = root.findText(Check.texts(delegate), "edited");
    if (!label || !label.visible) return Check.fail("edited label not shown for an edited message");

    delegate.message = { id: "me2", senderId: "s1", senderName: "Alex", text: "plain", outgoing: false, status: "delivered", created: root.now };
    label = root.findText(Check.texts(delegate), "edited");
    if (label && label.visible) return Check.fail("edited label shown for a message that was not edited");
    return true;
  }

  // checkFailedRetry verifies the retry line appears and clicking it emits
  // retry with the message id.
  function checkFailedRetry(): bool {
    delegate.message = { id: "m3", senderId: "me", senderName: "Me", text: "nope", outgoing: true, status: "failed", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };
    t.waitForRendering(delegate);

    const nodes = Check.texts(delegate);
    const retryLine = root.findText(nodes, "Not sent · r to retry");
    if (!retryLine || !retryLine.visible) return Check.fail("retry line not shown for a failed outgoing message");
    if (!Qt.colorEqual(retryLine.color, Color.urgent)) return Check.fail("retry line is not drawn in the urgent colour");

    root.retried = [];
    t.mouseClick(retryLine);
    if (JSON.stringify(root.retried) !== '["m3"]') return Check.fail("retry " + JSON.stringify(root.retried) + ", want [\"m3\"]");
    return true;
  }

  // checkRichText verifies markup is escaped and a URL becomes a link.
  function checkRichText(): bool {
    delegate.message = { id: "m4", senderId: "s1", senderName: "Alex", text: "<b>bold</b> see http://example.com/x", outgoing: false, status: "delivered", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };

    const nodes = Check.texts(delegate);
    const body = nodes.find(node => node.text.indexOf("bold") >= 0);
    if (!body) return Check.fail("message body not found");
    if (body.text.indexOf("&lt;b&gt;") < 0) return Check.fail("markup was not escaped: " + body.text);
    if (body.text.indexOf("<a href=") < 0) return Check.fail("URL was not linkified: " + body.text);
    if (body.text.indexOf("color:" + String(Color.accent)) < 0) return Check.fail("link is not in the accent color: " + body.text);
    return true;
  }

  // checkLineBreaks verifies a bulleted message keeps one line per bullet.
  function checkLineBreaks(): bool {
    delegate.message = { id: "m5", senderId: "s1", senderName: "Alex", text: "Plan:\n• one\n• two", outgoing: false, status: "delivered", created: root.now };

    const nodes = Check.texts(delegate);
    const body = nodes.find(node => node.objectName === "body");
    if (!body) return Check.fail("message body not found");
    const breaks = (body.text.match(/<br/g) || []).length;
    if (breaks !== 2) return Check.fail("bulleted message has " + breaks + " line breaks, want 2: " + body.text);
    return true;
  }

  // checkLinkPreview verifies a message with a link preview shows its card
  // inside the bubble, and one without shows none.
  function checkLinkPreview(): bool {
    const link = { kind: "link", url: "https://x.io", siteName: "X", title: "A page", description: "About it" };
    delegate.message = { id: "m6", senderId: "s1", senderName: "Alex", text: "see https://x.io", outgoing: false, status: "received", created: root.now, media: JSON.stringify(link) };

    const card = Check.find(delegate, "linkPreview");
    const bubble = Check.find(delegate, "bubble");
    if (!card || !card.visible) return Check.fail("link preview not shown");
    if (!Check.texts(card).some((item) => item.text === "A page")) return Check.fail("link preview does not show the page title");
    if (bubble.width < card.width) return Check.fail("bubble is narrower than its link preview");

    delegate.message = { id: "m7", senderId: "s1", senderName: "Alex", text: "plain", outgoing: false, status: "received", created: root.now, media: "" };
    if (card.visible) return Check.fail("link preview shown for a message without one");
    return true;
  }

  // checkPhoto verifies a photo without a caption shows only the photo, at
  // its shape, asks to be downloaded, and shows the download once it is in.
  function checkPhoto(): bool {
    const photo = { kind: "photo", width: 400, height: 200, thumb: "" };
    delegate.message = { id: "m8", senderId: "s1", senderName: "Alex", text: "[Photo]", outgoing: false, status: "received", created: root.now, media: JSON.stringify(photo), mediaPath: "" };

    const view = Check.find(delegate, "photoView");
    if (!view || !view.visible) return Check.fail("photo not shown");
    if (Check.find(delegate, "body").visible) return Check.fail("the [Photo] label is shown beside the photo");
    if (Math.abs(view.height - view.width / 2) > 1) return Check.fail(`photo is ${view.width}x${view.height}, not at its 2:1 shape`);
    if (root.wanted.indexOf("m8") < 0) return Check.fail("the photo did not ask to be downloaded");

    delegate.message = Object.assign({}, delegate.message, { mediaPath: "/tmp/m8.jpg" });
    if (String(Check.find(delegate, "photoImage").source) !== "file:///tmp/m8.jpg") return Check.fail("the downloaded photo is not shown");
    return true;
  }

  // checkVideoAndFile verifies a video shows a play mark and its length
  // without downloading itself, a file shows its name and size, and a
  // click on either asks to open it.
  function checkVideoAndFile(): bool {
    root.wanted = [];
    const video = { kind: "video", width: 1280, height: 720, duration: 65 };
    delegate.message = { id: "m9", senderId: "s1", senderName: "Alex", text: "[Video]", outgoing: false, status: "received", created: root.now, media: JSON.stringify(video), mediaPath: "" };

    const view = Check.find(delegate, "photoView");
    if (!view.visible || !Check.find(delegate, "playMark").visible) return Check.fail("video shown without its play mark");
    if (!Check.texts(view).some((item) => item.text === "1:05" && item.visible)) return Check.fail("video length not shown");
    if (root.wanted.length !== 0) return Check.fail("a video started downloading by itself");
    t.wait(50); // let the layout catch up with the new message before clicking
    t.mouseClick(view);
    if (root.opened.indexOf("m9") < 0) return Check.fail("clicking a video did not ask to open it");

    const file = { kind: "file", fileName: "report.pdf", size: 2048 };
    delegate.message = { id: "m10", senderId: "s1", senderName: "Alex", text: "[File]", outgoing: false, status: "received", created: root.now, media: JSON.stringify(file), mediaPath: "" };

    const fileView = Check.find(delegate, "fileView");
    const shown = Check.texts(fileView).map((item) => item.text);
    if (!fileView.visible || !shown.includes("report.pdf") || !shown.includes("2.0 KB") || !shown.includes("PDF")) {
      return Check.fail("file not shown with its name, size and type: " + shown);
    }
    t.wait(50);
    t.mouseClick(fileView);
    if (root.opened.indexOf("m10") < 0) return Check.fail("clicking a file did not ask to open it");
    return true;
  }

  // checkReactionChips verifies a message's reactions show as chips, a
  // message without any shows none while the bubble is not hovered, and
  // clicking a chip reports this message's id and emoji.
  function checkReactionChips(): bool {
    delegate.message = {
      id: "m11", senderId: "s1", senderName: "Alex", text: "hi", outgoing: false, status: "delivered", created: root.now,
      reactions: [{ emoji: "👍", count: 1, mine: true }]
    };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };

    const chips = Check.find(delegate, "reactionChips");
    if (!chips || !chips.visible) return Check.fail("reaction chips not shown for a message with reactions");

    root.reacted = [];
    t.mouseClick(Check.find(chips, "chipArea-👍"));
    if (JSON.stringify(root.reacted) !== '["m11 👍"]') return Check.fail("reacted " + JSON.stringify(root.reacted) + ", want [\"m11 \\ud83d\\udc4d\"]");

    // Move the pointer away from the bubble, so an earlier click near it
    // does not leave it hovered once the layout settles around a shorter
    // message.
    t.mouseMove(delegate, delegate.width - 5, 2);
    delegate.message = { id: "m12", senderId: "s1", senderName: "Alex", text: "plain", outgoing: false, status: "delivered", created: root.now };
    t.waitForRendering(delegate);
    if (Check.find(delegate, "reactionChips").visible) return Check.fail("reaction chips shown for a message with none, unhovered");
    return true;
  }
}
