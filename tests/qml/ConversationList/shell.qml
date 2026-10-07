// Checks the conversation list: a long title elides within the row, an
// unread row's title is bold, a pinned row shows a pin mark and an
// unpinned one does not, both empty states show the right text, and
// clicking a row emits activated() with its id.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var activated: []
  property real now: Date.now()

  property var conversations: [
    {
      id: "c1", title: "Short Chat", kind: "direct", service: "whatsapp", accountId: "a1",
      unread: 2, muted: false, pinned: false, archived: false, lastActivity: root.now, preview: "See you then",
      previewSender: "", previewOutgoing: false, match: ""
    },
    {
      id: "c2", title: "Muted Group", kind: "group", service: "telegram", accountId: "a2",
      unread: 0, muted: true, pinned: false, archived: false, lastActivity: root.now, preview: "ok",
      previewSender: "Sam", previewOutgoing: false, match: ""
    },
    {
      id: "c3",
      title: "A very long conversation title that will not fit in the narrow list column at all",
      kind: "direct", service: "whatsapp", accountId: "a1",
      unread: 0, muted: false, pinned: true, archived: false, lastActivity: root.now,
      preview: "A long preview that keeps going well past the edge of the narrow column",
      previewSender: "", previewOutgoing: false, match: ""
    }
  ]

  FloatingWindow {
    id: win
    implicitWidth: Style.space(300)
    implicitHeight: Style.space(500)
    visible: true

    ConversationList {
      id: list
      x: 0
      y: 0
      width: Style.space(300)
      height: Style.space(300)
      model: root.conversations
      selectedId: ""
      query: ""
      nowMs: root.now
      accountNames: ({ a1: "Personal", a2: "Team" })
      multiAccountServices: []
      onActivated: id => root.activated.push(id)
    }

    ConversationList {
      id: emptyList
      x: 0
      y: Style.space(310)
      width: Style.space(300)
      height: Style.space(80)
      model: []
      query: ""
    }

    ConversationList {
      id: searchEmptyList
      x: 0
      y: Style.space(400)
      width: Style.space(300)
      height: Style.space(80)
      model: []
      query: "ticket"
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

  // run drives the lists and checks the outcome.
  function run(): void {
    const longRow = Check.find(list, "row-c3");
    if (!longRow)
      return Check.fail("row-c3 not found");
    const longTitle = Check.find(longRow, "titleText");
    if (!longTitle)
      return Check.fail("title text for the long row not found");
    if (longTitle.width > longRow.width)
      return Check.fail("long title width " + longTitle.width + " exceeds row width " + longRow.width);
    if (!longTitle.truncated)
      return Check.fail("long title was not elided");
    const longPreview = Check.find(longRow, "previewText");
    if (!longPreview || !longPreview.truncated)
      return Check.fail("long preview was not elided");

    const unreadRow = Check.find(list, "row-c1");
    const unreadTitle = unreadRow ? Check.find(unreadRow, "titleText") : null;
    if (!unreadTitle || !unreadTitle.font.bold)
      return Check.fail("unread row title is not bold");

    const unpinnedPin = Check.find(unreadRow, "pinIcon");
    if (!unpinnedPin || unpinnedPin.visible)
      return Check.fail("an unpinned row shows a pin mark");

    const pinnedRow = Check.find(list, "row-c3");
    const pinnedMark = pinnedRow ? Check.find(pinnedRow, "pinIcon") : null;
    if (!pinnedMark || !pinnedMark.visible)
      return Check.fail("a pinned row does not show its pin mark");

    const noQueryEmpty = Check.find(emptyList, "emptyState");
    if (!noQueryEmpty || !noQueryEmpty.visible || noQueryEmpty.text.indexOf("Ctrl+N") < 0)
      return Check.fail("empty state for no conversations is wrong: "
        + (noQueryEmpty ? noQueryEmpty.text : "missing"));

    const queryEmpty = Check.find(searchEmptyList, "emptyState");
    if (!queryEmpty || !queryEmpty.visible || queryEmpty.text.indexOf("ticket") < 0)
      return Check.fail("empty state for a search query is wrong: "
        + (queryEmpty ? queryEmpty.text : "missing"));

    const mutedRow = Check.find(list, "row-c2");
    if (!mutedRow)
      return Check.fail("row-c2 not found");
    t.mouseClick(mutedRow, mutedRow.width / 2, mutedRow.height / 2);

    if (JSON.stringify(root.activated) !== '["c2"]')
      return Check.fail("activated " + JSON.stringify(root.activated) + ", want [\"c2\"]");

    console.log("PASS ConversationList");
    Qt.exit(0);
  }
}
