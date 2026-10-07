// Checks MessageDelegate: sender names in groups, the read glyph, the
// retry line on a failed message, that rich text escapes markup while
// linkifying URLs, a pipe table rendered as real columns instead of
// smeared wrapped lines, a reply's quote, a photo whose full image fails
// to load, the hover toolbar's react and reply buttons, reaction chips,
// that hovering a message never moves or resizes any message, that the
// toolbar stays visible once the pointer reaches it, even after it has
// left the bubble underneath, that the toolbar sits beside the bubble
// without ever overlapping a neighbouring message for both an incoming
// and an outgoing bubble, that it falls back inside the bubble's own
// corner when there is no room beside it, and that a highlighted message
// shows its accent outline and accent bar confined to the bubble itself
// (nothing wider ever tints) and its key-hint text, never clipped.
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

  // testRoot is this test's own directory, resolved from the running
  // file rather than the process's working directory: a path outside it
  // resolves to a blackhole offscreen, so a screenshot saved for a human
  // to look at has to land inside it.
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")

  // tinyThumb is a valid 2x2 JPEG, base64 encoded, standing in for the
  // kind of preview a real photo carries.
  readonly property string tinyThumb: "/9j/2wCEAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDIBCQkJDAsMGA0NGDIhHCEyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMv/AABEIAAIAAgMBIgACEQEDEQH/xAGiAAABBQEBAQEBAQAAAAAAAAAAAQIDBAUGBwgJCgsQAAIBAwMCBAMFBQQEAAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk5ebn6Onq8fLz9PX29/j5+gEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoLEQACAQIEBAMEBwUEBAABAncAAQIDEQQFITEGEkFRB2FxEyIygQgUQpGhscEJIzNS8BVictEKFiQ04SXxFxgZGiYnKCkqNTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqCg4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2dri4+Tl5ufo6ery8/T19vf4+fr/2gAMAwEAAhEDEQA/APAGZnYszFmY5JJySaSiigG76s//2Q=="

  // findText returns the first collected node whose text matches exactly.
  function findText(out, text) {
    return out.find(node => node.text === text) ?? null;
  }

  // collectWideTints returns every Rectangle-like descendant of item
  // (one with its own "color" property) wider than maxWidth, into out:
  // used to check that nothing but the bubble itself ever tints while a
  // message is highlighted.
  function collectWideTints(item, maxWidth, out) {
    if (item.color !== undefined && item.width > maxWidth + 0.5) out.push(item);
    const kids = item.data || item.children;
    if (kids && typeof kids.length === 'number') {
      for (let i = 0; i < kids.length; i++) root.collectWideTints(kids[i], maxWidth, out);
    }
    return out;
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
    implicitHeight: 1100
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

      MessageDelegate {
        id: rowD
        width: 360
        message: ({ id: "l4", senderId: "me", senderName: "Me", text: "fourth message", outgoing: true, status: "delivered", created: root.now })
        annotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })
        nowMs: root.now
      }
    }

    // narrowDelegate is too narrow for the toolbar to fit beside its
    // bubble: the bubble's own maximum width (72% of this) leaves less
    // room than the toolbar needs, so it must fall back inside the
    // bubble's own corner instead.
    MessageDelegate {
      id: narrowDelegate
      y: 820
      width: 140
      message: ({ id: "n1", senderId: "s1", senderName: "Alex", text: "a message long enough to wrap onto several lines and reach the bubble's maximum width", outgoing: false, status: "received", created: root.now })
      annotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })
      nowMs: root.now
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
    if (!root.checkPipeTable()) return;
    if (!root.checkLinkPreview()) return;
    if (!root.checkPhoto()) return;
    if (!root.checkPhotoLoadFailure()) return;
    if (!root.checkVideoAndFile()) return;
    if (!root.checkReplyQuote()) return;
    if (!root.checkHoverToolbar()) return;
    if (!root.checkReactionChips()) return;
    if (!root.checkHoverNeverShiftsLayout()) return;
    if (!root.checkToolbarStaysVisibleOnItself()) return;
    if (!root.checkHoverToolbarBesideBubble()) return;
    if (!root.checkHoverToolbarFallbackInsideBubble()) return;
    if (!root.checkHighlightVisuals()) return;
    if (!root.checkHighlightNeverShiftsLayout()) return;

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
    const retryLine = root.findText(nodes, "Not sent · t to retry");
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

  // checkPipeTable verifies a bot's Markdown pipe table renders as a real
  // HTML table: a bold header row, a border on every cell, the data
  // preserved, the dash separator row gone from what is actually shown,
  // and the whole table fitting inside the bubble rather than overflowing
  // it. It also saves a screenshot for a human to look at.
  function checkPipeTable(): bool {
    const text = [
      "| Job                | When                     | What it does |",
      "| ------------------ | ------------------------ | ------------- |",
      "| 06:30 check        | Daily, 06:30             | It compares today's Strava run against yesterday's and writes a short note about the difference, then saves it for the weekly digest |",
      "| Missed-run reminder | Every 30 min, 12:00-20:30 | Looks for a run between those hours and pings if none has shown up yet |",
    ].join("\n");
    delegate.message = { id: "tbl1", senderId: "s1", senderName: "Alex", text: text, outgoing: false, status: "received", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };
    t.waitForRendering(delegate);

    const body = Check.find(delegate, "body");
    // TextEdit's own "text" getter reads the parsed document back as Qt's
    // normalized HTML, which folds "<th>" into a styled "<td>", so a bold
    // header shows as a bold span rather than the original tag name.
    if (body.text.indexOf("<table") < 0) return Check.fail("table markup not rendered for a pipe table message: " + body.text);
    if (!/font-weight:700;">Job<\/span>/.test(body.text)) return Check.fail("table header not rendered in bold: " + body.text);
    if (body.contentWidth > body.width + 1) return Check.fail(`table overflows its column width: contentWidth ${body.contentWidth} > width ${body.width}`);

    const rowCount = (body.text.match(/<tr>/g) || []).length;
    if (rowCount !== 3) return Check.fail(`table has ${rowCount} rows, want 3 (one header and two data rows): ${body.text}`);

    // The padded dashes that smear across many lines as plain text are
    // gone from what is actually shown: only its surrounding borders are
    // drawn with lines, never the separator row's own text.
    const shown = body.text.replace(/<[^>]*>/g, ' ');
    if (/-{3,}/.test(shown)) return Check.fail("the separator row's dashes still show as visible text: " + shown);
    if (shown.indexOf('06:30 check') < 0) return Check.fail("a data cell's text is missing from the rendered table: " + shown);

    const bubble = Check.find(delegate, "bubble");
    bubble.grabToImage((result) => result.saveToFile(root.testRoot + "/pipe-table.png"));
    t.wait(200); // give the async grab time to save before anything else runs
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
  // bubble is hovered, that its "+" and "↩" buttons ask to open the
  // emoji picker and to reply, and that each button's hit area is big
  // enough to find comfortably with a mouse, not just its small glyph.
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

    const minHit = Style.space(28);
    const reactButton = Check.find(toolbar, "reactButton");
    const replyButton = Check.find(toolbar, "replyButton");
    if (reactButton.width < minHit || reactButton.height < minHit) {
      return Check.fail(`react button hit area is ${reactButton.width}x${reactButton.height}, want at least ${minHit}x${minHit}`);
    }
    if (replyButton.width < minHit || replyButton.height < minHit) {
      return Check.fail(`reply button hit area is ${replyButton.width}x${replyButton.height}, want at least ${minHit}x${minHit}`);
    }

    root.pickerRequests = [];
    t.mouseClick(reactButton);
    if (JSON.stringify(root.pickerRequests) !== '["m13"]') return Check.fail("+ click reported " + JSON.stringify(root.pickerRequests) + ", want [\"m13\"]");

    root.replied = [];
    t.mouseClick(replyButton);
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
  // bubble has revealed the toolbar, walking the pointer across to one of
  // its own buttons keeps it visible the whole way, even though the
  // toolbar floats above the bubble's own bounds and the path therefore
  // crosses out of the area that first revealed it. It also checks the
  // toolbar hides again once the pointer leaves both.
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

    // Walk the pointer from the bubble to the react button in small
    // steps, the way a real mouse travels, instead of teleporting
    // straight onto it. active (not just visible) must hold at every
    // step: visible alone could stay true on the opacity fade's own
    // inertia even if the underlying hover was briefly lost, masking a
    // real gap between the bubble and the toolbar.
    const reactButton = Check.find(toolbar, "reactButton");
    const target = Check.rect(reactButton, bubble);
    const startX = bubble.width / 2;
    const startY = bubble.height / 2;
    const endX = target.x + target.width / 2;
    const endY = target.y + target.height / 2;
    const steps = 40;
    for (let i = 1; i <= steps; i++) {
      t.mouseMove(bubble, startX + (endX - startX) * i / steps, startY + (endY - startY) * i / steps);
      if (!toolbar.active) return Check.fail(`toolbar hover dropped out travelling from the bubble to the react button, at step ${i} of ${steps}`);
    }
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

  // checkHoverToolbarBesideBubble verifies the toolbar sits beside the
  // bubble, on the correct side for each direction, rather than above
  // it, for both an incoming and an outgoing message, and never
  // overlaps a neighbouring message in the stack.
  function checkHoverToolbarBesideBubble(): bool {
    if (!root.checkToolbarBeside(rowB, false, rowA, rowC)) return false;
    if (!root.checkToolbarBeside(rowD, true, rowC, null)) return false;
    return true;
  }

  // checkToolbarBeside hovers row's bubble and checks its toolbar: on
  // the side free space leaves it (left of an outgoing bubble, right of
  // an incoming one), vertically centred on the bubble, and overlapping
  // neither neighbourAbove's nor neighbourBelow's own rect.
  function checkToolbarBeside(row: var, outgoing: bool, neighborAbove: var, neighborBelow: var): bool {
    t.mouseMove(stack, 2, 2);
    t.wait(150);

    const bubble = Check.find(row, "bubble");
    t.mouseMove(bubble, bubble.width / 2, bubble.height / 2);
    t.wait(50);

    const toolbar = Check.find(row, "hoverToolbar");
    if (!toolbar.visible) return Check.fail("toolbar did not appear on hover");
    if (!toolbar.fitsBeside) return Check.fail("this row was expected to leave room beside its bubble");

    const bubbleRect = Check.rect(bubble, stack);
    const toolbarRect = Check.rect(toolbar, stack);

    if (outgoing && toolbarRect.x + toolbarRect.width > bubbleRect.x + 0.5) {
      return Check.fail(`outgoing toolbar at x=${toolbarRect.x} does not sit to the left of the bubble at x=${bubbleRect.x}`);
    }
    if (!outgoing && toolbarRect.x < bubbleRect.x + bubbleRect.width - 0.5) {
      return Check.fail(`incoming toolbar at x=${toolbarRect.x} does not sit to the right of the bubble ending at ${bubbleRect.x + bubbleRect.width}`);
    }

    const bubbleMid = bubbleRect.y + bubbleRect.height / 2;
    const toolbarMid = toolbarRect.y + toolbarRect.height / 2;
    if (Math.abs(bubbleMid - toolbarMid) > 1) {
      return Check.fail(`toolbar is not vertically centred on the bubble: bubble mid ${bubbleMid}, toolbar mid ${toolbarMid}`);
    }

    for (const neighbor of [neighborAbove, neighborBelow]) {
      if (!neighbor) continue;
      const neighborRect = Check.rect(neighbor, stack);
      const overlap = Check.overlapArea(toolbarRect, neighborRect, 0.5);
      if (overlap > 0) return Check.fail(`toolbar overlaps a neighbouring message by ${overlap} square pixels`);
    }

    t.mouseMove(stack, 2, 2);
    t.wait(150);
    return true;
  }

  // checkHoverToolbarFallbackInsideBubble verifies that a bubble too wide
  // to leave room beside it (because the row itself is narrow) falls back
  // to showing the toolbar inside its own top corner, never reaching
  // outside the bubble and so never a neighbour either.
  function checkHoverToolbarFallbackInsideBubble(): bool {
    t.waitForRendering(narrowDelegate);
    const bubble = Check.find(narrowDelegate, "bubble");
    t.mouseMove(bubble, bubble.width / 2, bubble.height / 2);
    t.wait(50);

    const toolbar = Check.find(narrowDelegate, "hoverToolbar");
    if (!toolbar.visible) return Check.fail("toolbar did not appear on hover for the narrow bubble");
    if (toolbar.fitsBeside) return Check.fail("the narrow bubble left room beside it; this check needs a true fallback case");

    const toolbarRect = Check.rect(toolbar, bubble);
    if (toolbarRect.x < -0.5 || toolbarRect.x + toolbarRect.width > bubble.width + 0.5) {
      return Check.fail(`fallback toolbar is not inside the bubble's own width: x=${toolbarRect.x} width=${toolbarRect.width} bubble width=${bubble.width}`);
    }
    if (toolbarRect.y < -0.5) return Check.fail(`fallback toolbar is above the bubble's own top: y=${toolbarRect.y}`);

    t.mouseMove(narrowDelegate, 2, 2);
    t.wait(150);
    return true;
  }

  // checkHighlightVisuals verifies the highlighted message shows its
  // accent outline and accent bar confined to the bubble itself, never
  // a tint across the whole row, that the bar sits on the correct side
  // for each direction, that its key-hint text is correct for a plain
  // message, a failed outgoing one and one carrying media, and that the
  // hint text is never clipped at the row's edge.
  function checkHighlightVisuals(): bool {
    delegate.message = { id: "h1", senderId: "s1", senderName: "Alex", text: "hi", outgoing: false, status: "received", created: root.now };
    delegate.annotation = { showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false };
    delegate.highlighted = false;
    t.waitForRendering(delegate);

    const bubble = Check.find(delegate, "bubble");
    const bar = Check.find(delegate, "highlightBar");
    const hints = Check.find(delegate, "highlightHints");
    if (bubble.border.width > 0) return Check.fail("bubble shows its accent outline without being highlighted");
    if (bar.visible) return Check.fail("highlight bar shown without being highlighted");
    if (hints.opacity > 0) return Check.fail("highlight hints shown without being highlighted");

    delegate.highlighted = true;
    t.waitForRendering(delegate);
    if (bubble.border.width <= 0 || !Qt.colorEqual(bubble.border.color, Color.accent)) {
      return Check.fail(`bubble does not show its accent outline while highlighted: width=${bubble.border.width} color=${bubble.border.color}`);
    }
    if (!bar.visible) return Check.fail("highlight bar not shown while highlighted");
    if (!Qt.colorEqual(bar.color, Color.accent)) return Check.fail("highlight bar is not drawn in the accent colour");

    // Saved for a human to look at: the quiet highlight on an incoming
    // bubble, with its hint row underneath.
    delegate.grabToImage((result) => result.saveToFile(root.testRoot + "/highlight.png"));
    t.wait(200); // give the async grab time to save before anything else runs

    const wide = root.collectWideTints(delegate, bubble.width, []);
    if (wide.length > 0) return Check.fail(`${wide.length} element(s) wider than the bubble are tinted while highlighted`);

    const barInBubble = Check.rect(bar, bubble);
    if (barInBubble.x < -0.5 || barInBubble.x + barInBubble.width > bubble.width + 0.5) {
      return Check.fail(`highlight bar reaches outside the bubble's own width: x=${barInBubble.x} width=${barInBubble.width} bubble width=${bubble.width}`);
    }
    if (Check.rect(bar, delegate).x > bubble.width) return Check.fail("an incoming message's highlight bar is not on the left");

    if (hints.opacity <= 0) return Check.fail("highlight hints not shown while highlighted");
    if (hints.text !== "r reply · e react") {
      return Check.fail(`hint text is "${hints.text}", want "r reply · e react"`);
    }
    const hintsRect = Check.rect(hints, delegate);
    if (hintsRect.x < 1) return Check.fail(`hint row sits flush against the row's own edge and would clip: x=${hintsRect.x}`);
    if (hintsRect.x + hintsRect.width > delegate.width + 0.5) return Check.fail("hint row's text reaches past the row's right edge");

    delegate.message = { id: "h2", senderId: "me", senderName: "Me", text: "oops", outgoing: true, status: "failed", created: root.now };
    t.waitForRendering(delegate);
    if (Check.rect(bar, delegate).x + bar.width < delegate.width - bubble.width) return Check.fail("an outgoing message's highlight bar is not on the right");
    if (hints.text !== "r reply · e react · t retry") return Check.fail(`hint text is "${hints.text}", want the failed-retry hint`);
    const outgoingHintsRect = Check.rect(hints, delegate);
    if (outgoingHintsRect.x + outgoingHintsRect.width > delegate.width - 1) return Check.fail(`an outgoing hint row sits flush against the row's own edge and would clip: right edge=${outgoingHintsRect.x + outgoingHintsRect.width}`);

    const photo = { kind: "photo", width: 10, height: 10, thumb: "" };
    delegate.message = { id: "h3", senderId: "s1", senderName: "Alex", text: "[Photo]", outgoing: false, status: "received", created: root.now, media: JSON.stringify(photo) };
    if (hints.text !== "r reply · e react · Enter open") return Check.fail(`hint text is "${hints.text}", want the media-open hint`);

    delegate.highlighted = false;
    return true;
  }

  // checkHighlightNeverShiftsLayout verifies that highlighting one
  // message in the stack, then moving the highlight to another, never
  // moves or resizes any row: the hint row always reserves its own
  // space in each delegate's own layout, so turning it on or off (or
  // moving which row shows it) never changes any row's height.
  function checkHighlightNeverShiftsLayout(): bool {
    rowA.highlighted = false;
    rowB.highlighted = false;
    rowC.highlighted = false;
    rowD.highlighted = false;
    t.waitForRendering(rowB);

    const rows = [rowA, rowB, rowC, rowD];
    const before = rows.map(r => ({ y: r.y, height: r.height }));

    rowB.highlighted = true;
    t.waitForRendering(rowB);
    for (let i = 0; i < rows.length; i++) {
      if (rows[i].y !== before[i].y || rows[i].height !== before[i].height) {
        return Check.fail(`row ${i} moved or resized when a different row was highlighted: was y=${before[i].y} h=${before[i].height}, now y=${rows[i].y} h=${rows[i].height}`);
      }
    }

    rowB.highlighted = false;
    rowD.highlighted = true;
    t.waitForRendering(rowD);
    for (let i = 0; i < rows.length; i++) {
      if (rows[i].y !== before[i].y || rows[i].height !== before[i].height) {
        return Check.fail(`row ${i} moved or resized when the highlight moved to another row: was y=${before[i].y} h=${before[i].height}, now y=${rows[i].y} h=${rows[i].height}`);
      }
    }

    rowD.highlighted = false;
    return true;
  }
}
