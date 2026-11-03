// Checks ListColumn's show-all fold: with chats hidden it reads "N chats
// hidden" beside a "Show all" button, showing all flips it to "Showing all
// chats" beside "Show fewer", and the button reports showAllToggled(). Also
// checks the "Unread" header shown while the all-unreads view is on: it
// hides the show-all row (irrelevant while that view overrides the rail
// filter) and its own "Leave" button reports unreadViewLeft(). Finally,
// that the search field - the one editable field in this UI a security
// review found not gated on windowActive at all, unlike every other
// field - is destroyed while the window is inactive and comes back
// seeded from query with real keyboard focus once it is active again.
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

    root.beginSearchWindowInactiveCheck();
  }

  // beginSearchWindowInactiveCheck proves the fix for a follow-up
  // security review: the search field was the one editable field not
  // gated on windowActive, unlike every other field in this UI - with
  // focus not following a click, a click on the inactive window's own
  // placeholder created a live field that never had focus and so never
  // lost it, reading every clipboard offer for as long as the window
  // sat open and unfocused (see docs/decisions.md). query is set
  // directly here, the fixed input this component takes from its
  // caller (ListController, in the real app) - that round trip is the
  // controller's own job, not this component's - then turning the
  // window inactive must destroy the field, and turning it active
  // again must bring it back still seeded from query, with real
  // keyboard focus restored. Deferred with Qt.callLater after
  // focusSearch(), so this never assumes the Loader it just activated
  // has already created the field within the same call.
  function beginSearchWindowInactiveCheck(): void {
    column.query = "looking for someone";
    column.focusSearch();
    Qt.callLater(root.checkSearchFieldSeededThenGoInactive);
  }

  // checkSearchFieldSeededThenGoInactive confirms the field exists,
  // seeded from query, then that making the window inactive destroys
  // it outright.
  function checkSearchFieldSeededThenGoInactive(): void {
    const field = Check.find(column, "searchField");
    if (!field) return Check.fail("the search field does not exist to begin the window-inactive check");
    if (field.text !== "looking for someone")
      return Check.fail("the search field did not seed from query: " + JSON.stringify(field.text));
    field.forceActiveFocus();

    column.windowActive = false;
    if (Check.find(column, "searchField"))
      return Check.fail("the search field still exists with the window inactive");

    column.windowActive = true;
    Qt.callLater(root.checkSearchFieldRestoredWithTextAndFocus);
  }

  // checkSearchFieldRestoredWithTextAndFocus confirms the field came
  // back once the window was active again with the same text and real
  // keyboard focus, the same as every other field this project already
  // checks this way.
  function checkSearchFieldRestoredWithTextAndFocus(): void {
    const field = Check.find(column, "searchField");
    if (!field) return Check.fail("the search field did not come back once the window was active again");
    if (field.text !== "looking for someone")
      return Check.fail("the search field's text did not survive: " + JSON.stringify(field.text));
    if (!field.activeFocus)
      return Check.fail("the search field did not get focus back once the window was active again");

    console.log("PASS ListColumn");
    Qt.exit(0);
  }
}
