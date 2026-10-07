// Checks ListColumn's show-all fold: with chats hidden it reads "N chats
// hidden" beside a "Show all" button, showing all flips it to "Showing all
// chats" beside "Show fewer", and the button reports showAllToggled(). Also
// checks the "Unread" header shown while the all-unreads view is on: it
// hides the show-all row (irrelevant while that view overrides the rail
// filter) and its own "Leave" button reports unreadViewLeft().
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property int toggled: 0
  property int unreadLeft: 0

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
      onUnreadViewLeft: root.unreadLeft++
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

    root.checkUnreadHeader();
  }

  // checkUnreadHeader turns on the all-unreads view and checks its own
  // header appears, the show-all row (still with hidden chats to show)
  // hides since it is irrelevant while that view overrides the rail
  // filter, and the header's own button reports leaving the view.
  function checkUnreadHeader(): void {
    column.showAll = false;
    column.hiddenCount = 3;
    column.unreadView = true;

    const header = Check.find(column, "unreadHeaderRow");
    if (!header || !header.visible)
      return Check.fail("the \"Unread\" header is not visible while the all-unreads view is on");

    const headerTexts = Check.texts(header).map((item) => item.text);
    if (!headerTexts.includes("Unread"))
      return Check.fail("the header does not read \"Unread\": " + JSON.stringify(headerTexts));

    const showAllRow = Check.find(column, "showAllRow");
    if (showAllRow && showAllRow.visible)
      return Check.fail("the show-all row still shows while the all-unreads view overrides the rail filter");

    const leaveButton = Check.find(header, "leaveUnreadViewButton");
    if (!leaveButton || leaveButton.text.indexOf("Leave") !== 0)
      return Check.fail("the header's own button does not read \"Leave\": " + (leaveButton ? leaveButton.text : "missing"));

    t.mouseClick(leaveButton);
    if (root.unreadLeft !== 1)
      return Check.fail("clicking \"Leave\" did not report unreadViewLeft");

    console.log("PASS ListColumn");
    Qt.exit(0);
  }
}
