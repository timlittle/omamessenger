// Checks the composer: trimmed submit, whitespace ignored, cleared input,
// a placeholder naming the conversation, the reply banner and sending the
// id it answers, cancelling a reply, the attachment chip, both the
// banner and the chip showing together, the @-mention picker (shown
// while typing "@" in a group, keyboard and mouse insertion, Escape
// closing it without inserting, and the resolved mention reaching
// submitted()), and that a key the picker does not own (such as the
// command palette's Ctrl+/) still reaches routeKey while the picker is
// open, rather than being silently swallowed.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var sent: []
  property var sentReplyIds: []
  property var sentMentions: []
  property var routeKeyCalls: []
  property int cancelCount: 0
  property var attached: []
  property int removeRequests: 0

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 300
    visible: true

    Composer {
      id: composer

      anchors.fill: parent
      title: "Mum"
      onSubmitted: (text, replyToId, mentions) => {
        root.sent.push(text); root.sentReplyIds.push(replyToId); root.sentMentions.push(mentions);
      }
      onReplyCanceled: root.cancelCount += 1
      onFileAttached: path => root.attached.push(path)
      onAttachmentRemoveRequested: root.removeRequests++
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

  // mentionFocusTimer gives forceActiveFocus() one turn of the event
  // loop to actually take hold before the first real key event is sent;
  // calling keyClick in the same synchronous turn as forceActiveFocus
  // fails, since the platform has not yet finished activating the
  // window.
  Timer {
    id: mentionFocusTimer
    interval: 20
    onTriggered: root.checkMentionPicker()
  }

  // run drives the composer and checks the outcome.
  function run(): void {
    if (!root.checkSubmit()) return;
    if (!root.checkReplyBanner()) return;
    if (!root.checkAttachment()) return;
    if (!root.checkBannerAndChipTogether()) return;

    composer.attachmentPath = "";
    composer.replyTo = null;
    composer.members = [{ id: "u1", name: "Nadia" }, { id: "u2", name: "Ben" }];
    composer.input.forceActiveFocus();
    mentionFocusTimer.start();
  }

  // checkSubmit verifies trimming, whitespace-only text being ignored,
  // and the placeholder naming the conversation.
  function checkSubmit(): bool {
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
    return true;
  }

  // checkReplyBanner verifies the banner names the reply's sender and
  // text, that submitting carries the reply's id, that cancelling it
  // reports it without sending, and that it is hidden with no reply.
  function checkReplyBanner(): bool {
    const banner = Check.find(composer, "replyBanner");
    if (banner.visible) return Check.fail("reply banner shown with no reply in progress");

    composer.replyTo = { id: "m1", senderName: "Alex", text: "original message" };
    if (!banner.visible) return Check.fail("reply banner not shown while replying");

    const bannerText = Check.find(composer, "replyBannerText");
    if (bannerText.text.indexOf("Alex") < 0 || bannerText.text.indexOf("original message") < 0)
      return Check.fail("banner text \"" + bannerText.text + "\" does not name the sender and text");

    root.sent = [];
    root.sentReplyIds = [];
    composer.text = "sure";
    composer.submit();
    if (JSON.stringify(root.sentReplyIds) !== '["m1"]')
      return Check.fail("submitted replyToId " + JSON.stringify(root.sentReplyIds) + ", want [\"m1\"]");

    composer.replyTo = { id: "m2", senderName: "Alex", text: "another one" };
    const cancelButton = Check.find(composer, "replyCancel");
    root.cancelCount = 0;
    cancelButton.clicked();
    if (root.cancelCount !== 1) return Check.fail("replyCanceled fired " + root.cancelCount + " times, want 1");

    composer.replyTo = null;
    return true;
  }

  // checkAttachment drives the attachment chip: it appears with a name,
  // lets a captionless message submit, and a click on its ✕ asks to
  // remove it.
  function checkAttachment(): bool {
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
    if (JSON.stringify(root.sent) !== '["sure",""]')
      return Check.fail("a captionless attachment did not submit: sent " + JSON.stringify(root.sent));

    const removeButton = Check.find(composer, "removeAttachmentButton");
    if (!removeButton) return Check.fail("no removeAttachmentButton found");
    removeButton.clicked();
    if (root.removeRequests !== 1)
      return Check.fail("attachmentRemoveRequested fired " + root.removeRequests + " times, want 1");

    composer.attachmentPath = "";
    return true;
  }

  // checkBannerAndChipTogether verifies replying to a message while an
  // attachment is already picked shows both the banner and the chip, and
  // that sending carries the reply id alongside the attachment.
  function checkBannerAndChipTogether(): bool {
    composer.attachmentPath = "/tmp/report.pdf";
    composer.replyTo = { id: "m3", senderName: "Alex", text: "see attached" };

    if (!Check.find(composer, "replyBanner").visible) return Check.fail("reply banner hidden while an attachment is pending");
    if (!Check.find(composer, "attachmentChip").visible) return Check.fail("attachment chip hidden while replying");

    root.sent = [];
    root.sentReplyIds = [];
    composer.text = "";
    composer.submit();
    if (JSON.stringify(root.sentReplyIds) !== '["m3"]')
      return Check.fail("submitted replyToId " + JSON.stringify(root.sentReplyIds) + " alongside an attachment, want [\"m3\"]");

    return true;
  }

  // checkMentionPicker drives the @-mention picker end to end: it opens
  // while typing "@query" in a group, filters to matching members,
  // Escape closes it without changing the text, Tab inserts the
  // highlighted member by keyboard, a click inserts one by mouse, and
  // the text submitted carries the resolved mention at its real
  // position in the final, trimmed text.
  function checkMentionPicker(): void {
    const input = composer.input;
    if (!input.activeFocus) { Check.fail("composer input did not get active focus"); return; }
    input.text = "hi @nad";
    input.cursorPosition = input.text.length;

    if (!composer.pickerOpen) { Check.fail("picker not open while typing an @query"); return; }

    const picker = Check.find(composer, "mentionPicker");
    if (!picker || !picker.visible) { Check.fail("mentionPicker not visible while the picker is open"); return; }

    const names = Check.texts(picker).map((item) => item.text);
    if (!names.includes("Nadia")) { Check.fail("picker did not show Nadia: " + names); return; }
    if (names.includes("Ben")) { Check.fail("picker showed Ben, which does not match \"nad\": " + names); return; }

    // A key the mention picker does not own, such as the command
    // palette's Ctrl+/, must still reach routeKey while the picker is
    // open: Keymap.match("mentionPicker", …) also matches every global
    // binding, so without checking the action is actually one of the
    // picker's own, it would be swallowed here and never reach the key
    // router at all.
    // keyClick synthesizes the Control press itself as its own key event
    // too, which also reaches routeKey regardless of this fix (nothing
    // in Keymap.BINDINGS matches a bare Control press), so only a call
    // for the Slash key itself counts as reaching routeKey.
    root.routeKeyCalls = [];
    composer.routeKey = (key, modifiers, text) => { root.routeKeyCalls.push(key); return true; };
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    if (!root.routeKeyCalls.includes(Qt.Key_Slash)) {
      Check.fail("Ctrl+/ while the mention picker was open did not reach routeKey: " + JSON.stringify(root.routeKeyCalls));
      return;
    }
    composer.routeKey = null;

    t.keyClick(Qt.Key_Escape);
    if (composer.pickerOpen) { Check.fail("Escape did not close the picker"); return; }
    if (input.text !== "hi @nad") { Check.fail("Escape changed the text: " + input.text); return; }

    input.text = "";
    input.text = "hi @nad";
    input.cursorPosition = input.text.length;
    if (!composer.pickerOpen) { Check.fail("picker did not reopen for a fresh query"); return; }

    t.keyClick(Qt.Key_Tab);
    if (input.text !== "hi @Nadia ") { Check.fail("Tab did not insert the mention: " + input.text); return; }
    if (composer.pickerOpen) { Check.fail("picker stayed open after accepting a mention"); return; }

    root.sent = [];
    root.sentReplyIds = [];
    root.sentMentions = [];
    composer.submit();
    if (JSON.stringify(root.sent) !== '["hi @Nadia"]') {
      Check.fail("sent " + JSON.stringify(root.sent) + ", want [\"hi @Nadia\"]");
      return;
    }

    const mentions = root.sentMentions[0];
    if (!mentions || mentions.length !== 1 || mentions[0].userId !== "u1" || mentions[0].offset !== 3 || mentions[0].length !== 6) {
      Check.fail("resolved mentions = " + JSON.stringify(mentions) + ", want one mention of u1 at offset 3, length 6");
      return;
    }

    input.text = "";
    input.text = "hi @b";
    input.cursorPosition = input.text.length;
    if (!composer.pickerOpen) { Check.fail("picker did not open for the second query"); return; }

    const picker2 = Check.find(composer, "mentionPicker");
    if (!picker2) { Check.fail("no mentionPicker found for the second query"); return; }
    // The real click goes through MentionPicker's own MouseArea straight
    // to this same accepted signal (see MentionPicker.qml's onClicked);
    // emitting it here is how the test harness reaches the mouse path
    // without depending on synthetic pointer coordinates landing inside
    // an offscreen, zero-geometry window.
    picker2.accepted(0);
    if (input.text !== "hi @Ben ") { Check.fail("clicking the row did not insert the mention: " + input.text); return; }

    console.log("PASS Composer");
    Qt.exit(0);
  }
}
