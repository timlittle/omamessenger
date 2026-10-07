// Checks ListColumn's show-all fold: with chats hidden it reads "N chats
// hidden" beside a "Show all" button, showing all flips it to "Showing all
// chats" beside "Show fewer", and the button reports showAllToggled().
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property int toggled: 0

  FloatingWindow {
    id: win
    implicitWidth: Style.space(300)
    implicitHeight: Style.space(400)
    visible: true

    ListColumn {
      id: column
      x: 0
      y: 0
      width: Style.space(300)
      height: Style.space(400)
      model: []
      hiddenCount: 3
      showAll: false
      onShowAllToggled: root.toggled++
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.run()
  }

  // run checks the fold's text and button in both states, and the click.
  function run(): void {
    const row = Check.find(column, "showAllRow");
    if (!row || !row.visible)
      return Check.fail("the show-all row is not visible while chats are hidden");

    const texts = Check.texts(row).map((item) => item.text);
    if (!texts.includes("3 chats hidden"))
      return Check.fail("hidden count text is wrong: " + JSON.stringify(texts));

    const button = Check.find(row, "showAllButton");
    if (!button || button.text !== "Show all")
      return Check.fail("the button does not read \"Show all\": " + (button ? button.text : "missing"));

    t.mouseClick(button);
    if (root.toggled !== 1)
      return Check.fail("clicking the button did not report showAllToggled");

    column.showAll = true;
    column.hiddenCount = 0;

    const afterTexts = Check.texts(row).map((item) => item.text);
    if (!afterTexts.includes("Showing all chats"))
      return Check.fail("showing-all text is wrong: " + JSON.stringify(afterTexts));

    if (button.text !== "Show fewer")
      return Check.fail("the button does not read \"Show fewer\" once showing all: " + button.text);

    if (!row.visible)
      return Check.fail("the show-all row hides itself while showing all, with nothing left hidden");

    console.log("PASS ListColumn");
    Qt.exit(0);
  }
}
