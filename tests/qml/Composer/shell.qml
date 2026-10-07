// Checks the composer: trimmed submit, whitespace ignored, cleared input,
// a placeholder naming the conversation, the reply banner and sending
// the id it answers, and cancelling a reply.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var sent: []
  property var sentReplyIds: []
  property int cancelCount: 0

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 200
    visible: true

    Composer {
      id: composer

      anchors.fill: parent
      title: "Mum"
      onSubmitted: (text, replyToId) => { root.sent.push(text); root.sentReplyIds.push(replyToId); }
      onReplyCanceled: root.cancelCount += 1
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

  // run drives the composer and checks the outcome.
  function run(): void {
    if (!root.checkSubmit()) return;
    if (!root.checkReplyBanner()) return;

    console.log("PASS Composer");
    Qt.exit(0);
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
    t.mouseClick(cancelButton);
    if (root.cancelCount !== 1) return Check.fail("replyCanceled fired " + root.cancelCount + " times, want 1");

    return true;
  }
}
