// Checks the conversation list: a long title elides within the row, an
// unread row's title is bold, a pinned row shows a pin mark and an
// unpinned one does not, a dimmed row (shown only because show-all is on)
// is drawn with reduced opacity and a "Hidden" label, every empty state
// shows the right text, including the all-unreads view's own wording,
// clicking a row emits activated() with its id, and
// a row ListController keeps in place across a conversation.updated event
// (reordering it, rather than tearing the row down and redrawing it) still
// shows its pin and mute marks as soon as they change, not just on first
// draw. This drives a real ListController against a scripted service, the
// same path a pin from the command palette or from the phone takes.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui/components"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property var activated: []
  property real now: Date.now()

  property var conversations: [
    {
      id: "c1", title: "Short Chat", kind: "direct", service: "whatsapp", accountId: "a1",
      unread: 2, muted: false, pinned: false, archived: false, hidden: false, lastActivity: root.now, preview: "See you then",
      previewSender: "", previewOutgoing: false, match: "", dimmed: false, dimLabel: "",
      reminderAt: 0, reminderDue: false
    },
    {
      id: "c2", title: "Muted Group", kind: "group", service: "telegram", accountId: "a2",
      unread: 0, muted: true, pinned: false, archived: false, hidden: false, lastActivity: root.now, preview: "ok",
      previewSender: "Sam", previewOutgoing: false, match: "", dimmed: false, dimLabel: "",
      reminderAt: 0, reminderDue: false
    },
    {
      id: "c3",
      title: "A very long conversation title that will not fit in the narrow list column at all",
      kind: "direct", service: "whatsapp", accountId: "a1",
      unread: 0, muted: false, pinned: true, archived: false, hidden: false, lastActivity: root.now,
      preview: "A long preview that keeps going well past the edge of the narrow column",
      previewSender: "", previewOutgoing: false, match: "", dimmed: false, dimLabel: "",
      reminderAt: 0, reminderDue: false
    },
    {
      id: "c4", title: "Older Chat", kind: "direct", service: "whatsapp", accountId: "a1",
      unread: 0, muted: false, pinned: false, archived: false, hidden: true, lastActivity: root.now,
      preview: "see you around", previewSender: "", previewOutgoing: false, match: "", dimmed: true, dimLabel: "Hidden",
      reminderAt: 0, reminderDue: false
    }
  ]

  // liveService backs the live update scenario: two chats, the older one
  // already pinned, matching the order the real helper's conversations.list
  // already returns them in (pinned first).
  QtObject {
    id: liveService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", query: "", drafts: {} })

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") callback(null, [
        {
          id: "p1", title: "Already Pinned", kind: "direct", service: "whatsapp", accountId: "a1",
          unread: 0, muted: false, pinned: true, archived: false, hidden: false, lastActivity: root.now - 5000,
          preview: "", previewSender: "", previewOutgoing: false
        },
        {
          id: "p2", title: "Freshly Pinned", kind: "direct", service: "whatsapp", accountId: "a1",
          unread: 0, muted: false, pinned: false, archived: false, hidden: false, lastActivity: root.now,
          preview: "", previewSender: "", previewOutgoing: false
        }
      ]);
    }
  }

  ListController {
    id: liveController
    service: liveService
  }

  FloatingWindow {
    id: win
    implicitWidth: Style.space(300)
    implicitHeight: Style.space(720)
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

    ConversationList {
      id: unreadEmptyList
      x: 0
      y: Style.space(480)
      width: Style.space(300)
      height: Style.space(80)
      model: []
      query: ""
      unreadView: true
    }

    ConversationList {
      id: liveList
      x: 0
      y: Style.space(580)
      width: Style.space(300)
      height: Style.space(210)
      model: liveController.model
      selectedId: ""
      query: ""
    }
  }

  // settle lets ListView create and lay out delegates for a model change
  // made just now before the next check reads them, the way the real
  // window's own render loop would before the user sees the row.
  Timer {
    id: settle
    interval: 20
    property var callback: null
    onTriggered: root.settled(callback)
  }

  // wait schedules fn to run once ListView has caught up with the latest
  // model change.
  function wait(fn: var): void {
    settle.callback = fn;
    settle.restart();
  }

  // settled runs fn, the body of a wait() call.
  function settled(fn: var): void {
    fn();
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

    const plainRow = Check.find(list, "row-c1");
    if (!plainRow || plainRow.opacity !== 1)
      return Check.fail("a row shown in the standard list is dimmed: opacity " + (plainRow ? plainRow.opacity : "missing"));

    const dimmedRow = Check.find(list, "row-c4");
    if (!dimmedRow || dimmedRow.opacity >= 1)
      return Check.fail("a dimmed row is not drawn with reduced opacity: opacity " + (dimmedRow ? dimmedRow.opacity : "missing"));
    const dimLabel = dimmedRow ? Check.find(dimmedRow, "dimLabel") : null;
    if (!dimLabel || !dimLabel.visible || dimLabel.text !== "Hidden")
      return Check.fail("a hidden row shown in show-all does not carry a \"Hidden\" label");

    const noQueryEmpty = Check.find(emptyList, "emptyState");
    if (!noQueryEmpty || !noQueryEmpty.visible || noQueryEmpty.text.indexOf("Ctrl+N") < 0)
      return Check.fail("empty state for no conversations is wrong: "
        + (noQueryEmpty ? noQueryEmpty.text : "missing"));

    const queryEmpty = Check.find(searchEmptyList, "emptyState");
    if (!queryEmpty || !queryEmpty.visible || queryEmpty.text.indexOf("ticket") < 0)
      return Check.fail("empty state for a search query is wrong: "
        + (queryEmpty ? queryEmpty.text : "missing"));

    const unreadEmpty = Check.find(unreadEmptyList, "emptyState");
    if (!unreadEmpty || !unreadEmpty.visible || unreadEmpty.text.indexOf("No unread") < 0 || unreadEmpty.text.indexOf("Esc") < 0)
      return Check.fail("empty state for the all-unreads view is wrong: "
        + (unreadEmpty ? unreadEmpty.text : "missing"));

    const mutedRow = Check.find(list, "row-c2");
    if (!mutedRow)
      return Check.fail("row-c2 not found");
    t.mouseClick(mutedRow, mutedRow.width / 2, mutedRow.height / 2);

    if (JSON.stringify(root.activated) !== '["c2"]')
      return Check.fail("activated " + JSON.stringify(root.activated) + ", want [\"c2\"]");

    root.checkLiveUpdates();
  }

  // checkLiveUpdates exercises ListController's in-place update path
  // against a real ConversationList: a brand new chat arriving already
  // pinned (an insert), an existing chat being pinned (a move above the
  // one already pinned), the chat it displaced keeping its own mark, a
  // mute with no reorder at all, and a chat dropping out of the standard
  // list at the same moment a new one is inserted (so, with delegate reuse
  // on, a stale binding would show here). Each step waits a tick for
  // ListView to create or move the delegate before the next check reads
  // it.
  function checkLiveUpdates(): void {
    liveService.event("conversation.updated", {
      id: "p3", title: "Brand New And Pinned", kind: "direct", service: "whatsapp", accountId: "a1",
      unread: 0, muted: false, pinned: true, archived: false, hidden: false, lastActivity: root.now + 1000,
      preview: "", previewSender: "", previewOutgoing: false
    });

    root.wait(() => {
      const newPin = Check.find(liveList, "row-p3") ? Check.find(Check.find(liveList, "row-p3"), "pinIcon") : null;
      if (!newPin || !newPin.visible)
        return Check.fail("a brand new conversation inserted already pinned does not show the pin mark");

      const freshRow = Check.find(liveList, "row-p2");
      const freshPin = freshRow ? Check.find(freshRow, "pinIcon") : null;
      if (!freshPin || freshPin.visible)
        return Check.fail("a chat not yet pinned already shows a pin mark");

      // SetPinned on the helper both publishes a conversation.updated event
      // and returns the same conversation as the RPC result, so the UI
      // sees this twice back to back with no event-loop tick in between,
      // the way a pin from the command palette actually arrives.
      const p2Pinned = {
        id: "p2", title: "Freshly Pinned", kind: "direct", service: "whatsapp", accountId: "a1",
        unread: 0, muted: false, pinned: true, archived: false, hidden: false, lastActivity: root.now,
        preview: "", previewSender: "", previewOutgoing: false
      };
      liveService.event("conversation.updated", p2Pinned);
      liveService.event("conversation.updated", p2Pinned);

      root.wait(() => {
        const movedRow = Check.find(liveList, "row-p2");
        const movedPin = movedRow ? Check.find(movedRow, "pinIcon") : null;
        if (!movedPin || !movedPin.visible)
          return Check.fail("pinning a chat moved it up but its row does not show the pin mark");

        const displacedRow = Check.find(liveList, "row-p1");
        const displacedPin = displacedRow ? Check.find(displacedRow, "pinIcon") : null;
        if (!displacedPin || !displacedPin.visible)
          return Check.fail("the chat displaced by a new pin lost its own pin mark");

        liveService.event("conversation.updated", {
          id: "p1", title: "Already Pinned", kind: "direct", service: "whatsapp", accountId: "a1",
          unread: 0, muted: true, pinned: true, archived: false, hidden: false, lastActivity: root.now - 5000,
          preview: "", previewSender: "", previewOutgoing: false
        });

        root.wait(() => {
          const mutedRow = Check.find(liveList, "row-p1");
          const muteIcon = mutedRow ? Check.find(mutedRow, "muteIcon") : null;
          if (!muteIcon || !muteIcon.visible)
            return Check.fail("muting a chat in place did not show its mute mark");

          // Archiving p1 drops its row from the standard list (a remove)
          // in the same moment a brand new, already-pinned chat arrives
          // (an insert): if the list view reused p1's dropped row for the
          // new one, a stale binding would show here.
          liveService.event("conversation.updated", {
            id: "p1", title: "Already Pinned", kind: "direct", service: "whatsapp", accountId: "a1",
            unread: 0, muted: true, pinned: true, archived: true, hidden: false, lastActivity: root.now - 5000,
            preview: "", previewSender: "", previewOutgoing: false
          });
          liveService.event("conversation.updated", {
            id: "p4", title: "Also Brand New And Pinned", kind: "direct", service: "whatsapp", accountId: "a1",
            unread: 0, muted: false, pinned: true, archived: false, hidden: false, lastActivity: root.now + 2000,
            preview: "", previewSender: "", previewOutgoing: false
          });

          root.wait(() => {
            const reusedRow = Check.find(liveList, "row-p4");
            const reusedPin = reusedRow ? Check.find(reusedRow, "pinIcon") : null;
            if (!reusedPin || !reusedPin.visible)
              return Check.fail("a new pinned chat taking over a dropped row does not show the pin mark");

            console.log("PASS ConversationList");
            Qt.exit(0);
          });
        });
      });
    });
  }
}
