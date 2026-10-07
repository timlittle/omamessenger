// Checks MessageDelegate: sender names in groups, the read glyph, the
// retry line on a failed message, that rich text escapes markup while
// linkifying URLs, a reply's quote, a photo whose full image fails to load,
// the hover toolbar's react and reply buttons, reaction chips, that
// hovering a message never moves or resizes any message, and that the
// toolbar stays visible once the pointer reaches it, even after it has
// left the bubble underneath.
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
  // replied records the ids a reply was requested for.
  // quoted records the remote ids a quote was clicked to open.
  property var replied: []
  property var quoted: []
  property var reacted: []
  property var pickerRequests: []
  property real now: Date.now()

  // tinyThumb is a valid 2x2 JPEG, base64 encoded, standing in for the
  // kind of preview a real photo carries.
  readonly property string tinyThumb: "/9j/2wCEAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDIBCQkJDAsMGA0NGDIhHCEyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMv/AABEIAAIAAgMBIgACEQEDEQH/xAGiAAABBQEBAQEBAQAAAAAAAAAAAQIDBAUGBwgJCgsQAAIBAwMCBAMFBQQEAAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk5ebn6Onq8fLz9PX29/j5+gEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoLEQACAQIEBAMEBwUEBAABAncAAQIDEQQFITEGEkFRB2FxEyIygQgUQpGhscEJIzNS8BVictEKFiQ04SXxFxgZGiYnKCkqNTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqCg4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2dri4+Tl5ufo6ery8/T19vf4+fr/2gAMAwEAAhEDEQA/APAGZnYszFmY5JJySaSiigG76s//2Q=="

  // findText returns the first collected node whose text matches exactly.
  function findText(out, text) {
    return out.find(node => node.text === text) ?? null;
  }

  // waitUntil polls predicate, letting the event loop run between tries,
  // until it is true or timeoutMs has passed; its last result is the
  // return value either way. An async image load needs this, since it
  // finishes on a later turn of the event loop, not within this function.
  function waitUntil(predicate, timeoutMs) {
    const start = Date.now();
    let ok = predicate();
    while (!ok && Date.now() - start < timeoutMs) {
      t.wait(20);
      ok = predicate();
    }
    return ok;
  }

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 900
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
      onReplyRequested: id => root.replied.push(id)
      onQuoteOpened: remoteId => root.quoted.push(remoteId)
      onReact: (id, emoji) => root.reacted.push(id + " " + emoji)
      onReactPickerRequested: id => root.pickerRequests.push(id)
    }

    // A stack of three delegates, stood up only to measure layout: it
    // checks that hovering one message never moves or resizes another,
    // or itself, the way a single delegate in isolation cannot.
    Column {
      id: stack
      y: 520
      width: 360

      MessageDelegate {
        id: rowA
        width: 360
        message: ({ id: "l1", senderId: "s1", senderName: "Alex", text: "first message", outgoing: false, status: "received", created: root.now })
        annotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })
        nowMs: root.now
      }

      MessageDelegate {
        id: rowB
        width: 360
        message: ({ id: "l2", senderId: "s1", senderName: "Alex", text: "second message", outgoing: false, status: "received", created: root.now })
        annotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })
        nowMs: root.now
        onReplyRequested: id => root.replied.push(id)
        onReactPickerRequested: id => root.pickerRequests.push(id)
      }

      MessageDelegate {
        id: rowC
        width: 360
        message: ({ id: "l3", senderId: "s1", senderName: "Alex", text: "third message", outgoing: false, status: "received", created: root.now })
        annotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })
        nowMs: root.now
      }
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
    if (!root.checkPhotoLoadFailure()) return;
    if (!root.checkVideoAndFile()) return;
    if (!root.checkReplyQuote()) return;
    if (!root.checkHoverToolbar()) return;
    if (!root.checkReactionChips()) return;
    if (!root.checkHoverNeverShiftsLayout()) return;
    if (!root.checkToolbarStaysVisibleOnItself()) return;

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

  // checkPhotoLoadFailure verifies a photo whose full image fails to load
  // (a mediaPath left over from a file since moved or deleted) falls back
  // to its thumb when it has one, or otherwise shows a quiet message
  // naming the file rather than staying an empty box.
  function checkPhotoLoadFailure(): bool {
    const noThumb = { kind: "photo", width: 400, height: 200, thumb: "", fileName: "trip.jpg" };
    delegate.message = { id: "m16", senderId: "s1", senderName: "Alex", text: "[Photo]", outgoing: false, status: "received", created: root.now, media: JSON.stringify(noThumb), mediaPath: "/does/not/exist.jpg" };

    // The PhotoView swaps the Image away from the failed file once it
    // reports the error, so the visible outcome, not the Image's own
    // status, is what settles.
    const label = Check.find(delegate, "unavailableLabel");
    if (!root.waitUntil(() => label.visible, 3000)) return Check.fail("unavailable message not shown once the photo failed to load");
    if (label.text.indexOf("trip.jpg") < 0) return Check.fail("unavailable message does not name the file: " + label.text);

    const withThumb = { kind: "photo", width: 400, height: 200, thumb: root.tinyThumb, fileName: "trip.jpg" };
    delegate.message = { id: "m16b", senderId: "s1", senderName: "Alex", text: "[Photo]", outgoing: false, status: "received", created: root.now, media: JSON.stringify(withThumb), mediaPath: "/does/not/exist-either.jpg" };

    const image2 = Check.find(delegate, "photoImage");
    if (!root.waitUntil(() => String(image2.source).indexOf("data:image/jpeg") === 0, 3000)) return Check.fail("thumb fallback not shown once the full image failed");
    if (Check.find(delegate, "unavailableLabel").visible) return Check.fail("unavailable message shown although a thumb is available");
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

  // checkReplyQuote verifies a reply shows a quote of the sender and text
  // it answers, that a message without one shows none, and that clicking
  // the quote asks to scroll to the quoted message.
  function checkReplyQuote(): bool {
    const reply = { remoteId: "7", senderName: "Alex", text: "original message" };
    delegate.message = { id: "m11", senderId: "s1", senderName: "Alex", text: "sure", outgoing: false, status: "received", created: root.now, replyTo: JSON.stringify(reply) };

    const quote = Check.find(delegate, "replyQuote");
    if (!quote || !quote.visible) return Check.fail("reply quote not shown");
    if (!Check.texts(quote).some((item) => item.text === "Alex")) return Check.fail("quote does not show the quoted sender");
    if (!Check.texts(quote).some((item) => item.text === "original message")) return Check.fail("quote does not show the quoted text");

    root.quoted = [];
    t.wait(50); // let the layout catch up with the new message before clicking
    t.mouseClick(quote);
    if (JSON.stringify(root.quoted) !== '["7"]') return Check.fail("quote click reported " + JSON.stringify(root.quoted) + ", want [\"7\"]");

    delegate.message = { id: "m12", senderId: "s1", senderName: "Alex", text: "plain", outgoing: false, status: "received", created: root.now };
    if (Check.find(delegate, "replyQuote").visible) return Check.fail("reply quote shown for a message that answers nothing");
    return true;
  }

  // checkHoverToolbar verifies the hover toolbar is hidden until the
  // bubble is hovered, and that its "+" and "↩" buttons ask to open the
  // emoji picker and to reply.
  function checkHoverToolbar(): bool {
    delegate.message = { id: "m13", senderId: "s1", senderName: "Alex", text: "hi", outgoing: false, status: "received", created: root.now };

    // Move the pointer away first: an earlier click left it resting on
    // the bubble, which would otherwise count as an existing hover. The
    // wait lets the fade-out finish, since the toolbar stays visible for
    // the length of its opacity Behavior after a hover ends.
    t.mouseMove(delegate, 2, 2);
    t.wait(150);

    const toolbar = Check.find(delegate, "hoverToolbar");
    if (toolbar.visible) return Check.fail("hover toolbar shown without a hover");

    const bubble = Check.find(delegate, "bubble");
    t.mouseMove(bubble, bubble.width / 2, bubble.height / 2);
    t.wait(50); // let the hover-driven opacity settle
    if (!toolbar.visible) return Check.fail("hover toolbar not shown on hover");

    root.pickerRequests = [];
    t.mouseClick(Check.find(toolbar, "reactButton"));
    if (JSON.stringify(root.pickerRequests) !== '["m13"]') return Check.fail("+ click reported " + JSON.stringify(root.pickerRequests) + ", want [\"m13\"]");

    root.replied = [];
    t.mouseClick(Check.find(toolbar, "replyButton"));
    if (JSON.stringify(root.replied) !== '["m13"]') return Check.fail("reply click reported " + JSON.stringify(root.replied) + ", want [\"m13\"]");
    return true;
  }

  // checkReactionChips verifies a message's reactions show as chips, a
  // message without any shows none, and clicking a chip reports this
  // message's id and emoji.
  function checkReactionChips(): bool {
    delegate.message = {
      id: "m14", senderId: "s1", senderName: "Alex", text: "hi", outgoing: false, status: "delivered", created: root.now,
      reactions: [{ emoji: "👍", count: 1, mine: true }]
    };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };
    t.waitForRendering(delegate); // let the chip row realize before it is clicked

    const chips = Check.find(delegate, "reactionChips");
    if (!chips || !chips.visible) return Check.fail("reaction chips not shown for a message with reactions");

    root.reacted = [];
    t.mouseClick(Check.find(chips, "chipArea-👍"));
    if (JSON.stringify(root.reacted) !== '["m14 👍"]') return Check.fail("reacted " + JSON.stringify(root.reacted) + ", want [\"m14 \\ud83d\\udc4d\"]");

    delegate.message = { id: "m15", senderId: "s1", senderName: "Alex", text: "plain", outgoing: false, status: "delivered", created: root.now };
    t.waitForRendering(delegate);
    if (Check.find(delegate, "reactionChips").visible) return Check.fail("reaction chips shown for a message with none");
    return true;
  }

  // checkHoverNeverShiftsLayout verifies that hovering a message to show
  // its toolbar never moves or resizes any message in the stack: the
  // toolbar floats outside each delegate's own Column, so appearing must
  // not change a single y or height.
  function checkHoverNeverShiftsLayout(): bool {
    t.mouseMove(stack, 2, 2); // start with the pointer away from every row
    t.waitForRendering(rowB);

    const rows = [rowA, rowB, rowC];
    const before = rows.map(r => ({ y: r.y, height: r.height }));

    const bubble = Check.find(rowB, "bubble");
    t.mouseMove(bubble, bubble.width / 2, bubble.height / 2);
    t.wait(50); // let the hover-driven opacity settle
    if (!Check.find(rowB, "hoverToolbar").visible) return Check.fail("hover toolbar did not appear over the hovered row");

    for (let i = 0; i < rows.length; i++) {
      if (rows[i].y !== before[i].y || rows[i].height !== before[i].height) {
        return Check.fail(`row ${i} moved or resized on hover: was y=${before[i].y} h=${before[i].height}, now y=${rows[i].y} h=${rows[i].height}`);
      }
    }
    return true;
  }

  // checkToolbarStaysVisibleOnItself verifies that once hovering the
  // bubble has revealed the toolbar, moving the pointer on to one of its
  // own buttons keeps it visible, even though the toolbar floats above
  // the bubble's own bounds and the pointer has therefore left the area
  // that first revealed it. It also checks the toolbar hides again once
  // the pointer leaves both.
  function checkToolbarStaysVisibleOnItself(): bool {
    // Start clean: an earlier check left the pointer resting on rowB's
    // bubble, which would otherwise make the next assertion meaningless.
    t.mouseMove(stack, 2, 2);
    t.wait(150);

    const toolbar = Check.find(rowB, "hoverToolbar");
    if (toolbar.visible) return Check.fail("toolbar visible before any hover");

    // Hover the bubble first, exactly as a user would before reaching
    // for one of the toolbar's buttons: the toolbar cannot be hovered
    // directly while it is still hidden.
    const bubble = Check.find(rowB, "bubble");
    t.mouseMove(bubble, bubble.width / 2, bubble.height / 2);
    t.wait(50);
    if (!toolbar.visible) return Check.fail("toolbar did not appear on hovering the bubble");

    // Move on to the toolbar's own button next; only its own hover area
    // can be keeping it shown once the pointer has left the bubble.
    const reactButton = Check.find(toolbar, "reactButton");
    t.mouseMove(reactButton, reactButton.width / 2, reactButton.height / 2);
    t.wait(150); // long enough that a real fade-out would have finished
    if (!toolbar.visible) return Check.fail("toolbar hid when the pointer moved from the bubble onto its own button");

    root.pickerRequests = [];
    t.mouseClick(reactButton);
    if (JSON.stringify(root.pickerRequests) !== '["l2"]') return Check.fail("+ click reported " + JSON.stringify(root.pickerRequests) + ", want [\"l2\"]");

    const replyButton = Check.find(toolbar, "replyButton");
    root.replied = [];
    t.mouseClick(replyButton);
    if (JSON.stringify(root.replied) !== '["l2"]') return Check.fail("reply click reported " + JSON.stringify(root.replied) + ", want [\"l2\"]");

    t.mouseMove(stack, 2, 2);
    t.wait(150); // let the fade-out finish
    if (toolbar.visible) return Check.fail("toolbar stayed visible once the pointer left both the bubble and the toolbar");
    return true;
  }
}
