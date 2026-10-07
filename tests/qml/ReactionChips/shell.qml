// Checks ReactionChips: chips show their emoji and count, the user's own
// reaction is marked with more than colour alone, clicking a chip toggles
// it, and the row takes no space when a message has no reactions. Adding
// a reaction is started from the message's hover toolbar instead, so
// that is checked by the MessageDelegate test.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var toggled: []

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 100
    visible: true

    ReactionChips {
      id: chips
      width: 300
      reactions: [
        { emoji: "👍", count: 2, mine: false },
        { emoji: "❤️", count: 1, mine: true }
      ]
      onToggled: emoji => root.toggled.push(emoji)
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
    if (!root.checkChipsShowEmojiAndCount()) return;
    if (!root.checkMineIsMarkedBeyondColour()) return;
    if (!root.checkClickingAChipToggles()) return;
    if (!root.checkHiddenWithNoReactions()) return;

    console.log("PASS ReactionChips");
    Qt.exit(0);
  }

  function checkChipsShowEmojiAndCount(): bool {
    const shown = Check.texts(chips).map((item) => item.text);
    if (!shown.includes("👍 2")) return Check.fail("thumbs-up chip not shown with its count: " + shown);
    if (!shown.includes("❤️ 1")) return Check.fail("heart chip not shown with its count: " + shown);
    return true;
  }

  function checkMineIsMarkedBeyondColour(): bool {
    const mine = Check.find(chips, "chip-❤️");
    const other = Check.find(chips, "chip-👍");
    if (!mine || mine.border.width <= 0) return Check.fail("the user's own reaction has no border, so colour would be the only cue");
    if (!other || other.border.width > 0) return Check.fail("a reaction that is not the user's own was given a border too");
    return true;
  }

  function checkClickingAChipToggles(): bool {
    root.toggled = [];
    t.mouseClick(Check.find(chips, "chipArea-❤️"));
    if (JSON.stringify(root.toggled) !== '["❤️"]') return Check.fail("toggled " + JSON.stringify(root.toggled) + ", want [\"❤️\"]");

    root.toggled = [];
    t.mouseClick(Check.find(chips, "chipArea-👍"));
    if (JSON.stringify(root.toggled) !== '["👍"]') return Check.fail("toggled " + JSON.stringify(root.toggled) + ", want [\"👍\"]");
    return true;
  }

  function checkHiddenWithNoReactions(): bool {
    chips.reactions = [];
    if (chips.visible) return Check.fail("chips still visible with no reactions");
    return true;
  }
}
