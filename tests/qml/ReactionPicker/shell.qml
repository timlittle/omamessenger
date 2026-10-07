// Checks ReactionPicker: it shows each emoji, highlights currentIndex,
// clicking one reports its index, and a click outside the card cancels.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var picked: []
  property int cancelled: 0

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 300
    visible: true

    ReactionPicker {
      id: picker
      anchors.fill: parent
      open: true
      emojis: ["👍", "❤️", "😂"]
      currentIndex: 1
      onPicked: index => root.picked.push(index)
      onCancelled: root.cancelled++
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  function run(): void {
    if (!root.checkShowsEveryEmoji()) return;
    if (!root.checkHighlightsCurrentIndex()) return;
    if (!root.checkClickingAnEmojiReportsItsIndex()) return;
    if (!root.checkOutsideClickCancels()) return;
    if (!root.checkClosedWhenNotOpen()) return;

    console.log("PASS ReactionPicker");
    Qt.exit(0);
  }

  function checkShowsEveryEmoji(): bool {
    const shown = Check.texts(picker).map((item) => item.text);
    for (const emoji of ["👍", "❤️", "😂"]) {
      if (!shown.includes(emoji)) return Check.fail("picker did not show " + emoji + ": " + shown);
    }
    return true;
  }

  function checkHighlightsCurrentIndex(): bool {
    const highlighted = Check.find(picker, "slot-1");
    const notHighlighted = Check.find(picker, "slot-0");
    if (!highlighted || highlighted.border.width <= 0) return Check.fail("the highlighted emoji has no border");
    if (!notHighlighted || notHighlighted.border.width > 0) return Check.fail("an emoji that is not highlighted was given a border too");
    return true;
  }

  function checkClickingAnEmojiReportsItsIndex(): bool {
    root.picked = [];
    t.mouseClick(Check.find(picker, "slotArea-2"));
    if (JSON.stringify(root.picked) !== "[2]") return Check.fail("picked " + JSON.stringify(root.picked) + ", want [2]");
    return true;
  }

  function checkOutsideClickCancels(): bool {
    root.cancelled = 0;
    Check.find(picker, "reactionPickerModal").outsideClicked();
    if (root.cancelled !== 1) return Check.fail("outside click cancelled " + root.cancelled + " times, want 1");
    return true;
  }

  function checkClosedWhenNotOpen(): bool {
    picker.open = false;
    if (picker.visible) return Check.fail("picker still visible when closed");
    return true;
  }
}
