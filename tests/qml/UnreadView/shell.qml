// Drives the all-unreads view end to end with real key events, the same
// dispatch Panel.routeKey does: Ctrl+Shift+A shows only the unread chat
// and the visible "Unread" header, Esc returns to the previous list, and
// once nothing is unread any more the view's own empty state names the key
// to leave it.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/controllers"
import "ui/lib/Keymap.js" as Keymap
import "ui/lib/Navigation.js" as Navigation
import "Check.js" as Check

ShellRoot {
  id: root

  // service answers conversations.list with one read and one unread chat
  // on different services, so the view's "across everything" behaviour
  // and the standard list's rail-filter behaviour are both exercised.
  QtObject {
    id: service

    property string status: "ready"
    property var accounts: []
    property var services: []
    property var uiState: ({ railKey: "all", selectedId: "", activeId: "", query: "", drafts: {} })

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, [
          {
            id: "c1", title: "Read Chat", kind: "direct", service: "whatsapp", accountId: "a1",
            unread: 0, muted: false, pinned: false, archived: false, hidden: false, lastActivity: Date.now(),
            preview: "", previewSender: "", previewOutgoing: false
          },
          {
            id: "c2", title: "Unread Chat", kind: "direct", service: "telegram", accountId: "a2",
            unread: 2, muted: false, pinned: false, archived: false, hidden: false, lastActivity: Date.now() - 1000,
            preview: "", previewSender: "", previewOutgoing: false
          }
        ]);
        return;
      }
      callback(null, {});
    }
  }

  ListController {
    id: listController
    service: service
  }

  WindowController {
    id: windowController
    service: service
    listController: listController
  }

  // routeKey mirrors Panel.routeKey: match the active context, then hand
  // the action to whichever of the two controllers here owns it.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const context = Navigation.keyContext({
      confirmOpen: false, setupOpen: false, viewerOpen: false, paletteOpen: windowController.paletteOpen,
      reactionPickerOpen: false, deleteConfirmOpen: false, dialogOpen: false,
      searchFocused: listController.searchFocused, composeFocused: false, pane: "list"
    });
    const action = Keymap.match(context, key, modifiers, text);
    if (!action) return false;

    const controllers = [listController, windowController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    return owner.run(action) !== false;
  }

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 400
    visible: true

    Item {
      id: keyArea
      anchors.fill: parent
      focus: true

      Keys.onPressed: event => {
        if (root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
      }

      ListColumn {
        id: listColumn
        anchors.fill: parent

        query: listController.query
        model: listController.model
        selectedId: listController.selectedId
        showAll: listController.showAll
        hiddenCount: listController.hiddenCount
        unreadView: listController.unreadView
        routeKey: root.routeKey

        onQueryEdited: text => listController.setQuery(text)
        onShowAllToggled: listController.setShowAll(!listController.showAll)
        onUnreadViewLeft: listController.setUnreadView(false)
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  // A deliberately unreachable deadline: it only fires if the checks
  // below never run.
  Timer {
    running: true
    interval: 10000
    onTriggered: Check.fail("timed out before the checks finished")
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.run()
  }

  // run presses the real shortcut and Esc, and checks the list and header
  // follow them.
  function run(): void {
    keyArea.forceActiveFocus();

    t.keyClick(Qt.Key_A, Qt.ControlModifier | Qt.ShiftModifier);
    if (!listController.unreadView) return Check.fail("Ctrl+Shift+A did not turn on the unread view");
    if (listController.model.count !== 1 || listController.model.get(0).id !== "c2")
      return Check.fail("the unread view did not show only the unread chat, overriding the rail filter");

    const header = Check.find(listColumn, "unreadHeaderRow");
    if (!header || !header.visible) return Check.fail("the \"Unread\" header is not visible while the view is on");

    t.keyClick(Qt.Key_Escape);
    if (listController.unreadView) return Check.fail("Esc did not leave the unread view");
    if (listController.model.count !== 2) return Check.fail("Esc did not return to the previous list");

    root.checkEmptyState();
  }

  // checkEmptyState marks the one unread chat read, re-enters the view and
  // checks it is now empty with the right wording, naming the key to leave.
  function checkEmptyState(): void {
    service.event("conversation.updated", {
      id: "c2", title: "Unread Chat", kind: "direct", service: "telegram", accountId: "a2",
      unread: 0, muted: false, pinned: false, archived: false, hidden: false, lastActivity: Date.now() - 1000,
      preview: "", previewSender: "", previewOutgoing: false
    });

    t.keyClick(Qt.Key_A, Qt.ControlModifier | Qt.ShiftModifier);
    if (listController.model.count !== 0) return Check.fail("the unread view still shows a chat once nothing is unread");

    const empty = Check.find(listColumn, "emptyState");
    if (!empty || !empty.visible || empty.text.indexOf("No unread") < 0 || empty.text.indexOf("Esc") < 0)
      return Check.fail("the unread view's empty state is wrong: " + (empty ? empty.text : "missing"));

    console.log("PASS UnreadView");
    Qt.exit(0);
  }
}
