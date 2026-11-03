pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import qs.Commons
import "../theme"

// The unified conversation list: one row per conversation, with empty
// states for no conversations and no search matches. Rows are identified
// by id, so an in-place model update never moves the selection. This is a
// view only: it reports intent through a signal and never calls the
// helper.
Item {
  id: root

  // ------------------------------------------------------------- API
  // model: conversations, as a ListModel or a plain array.
  property var model: null
  // selectedId: the open conversation's id.
  property string selectedId: ""
  // query: the active search text.
  property string query: ""
  // accountNames: accountId -> account name, for rows that show it.
  property var accountNames: ({})
  // multiAccountServices: services with more than one account.
  property var multiAccountServices: []
  // accountColors: accountId -> colour tag, for the account colour stripe.
  property var accountColors: ({})
  // showAccountColors: true once more than one account exists, the only
  // time a row's account colour stripe is worth showing.
  property bool showAccountColors: false
  // unreadView: true while the all-unreads view is showing, for the empty
  // state's own wording.
  property bool unreadView: false

  // activated fires when a row is clicked.
  signal activated(string id)

  // pinDropped fires once a drag on a pinned row ends, naming where it
  // landed: toIndex is its final position among every row (which, since
  // pinned rows always sit first, is also its rank among just the
  // pinned ones - see ListController.movePinTo, the one thing this ever
  // drives). Fires even when the row never actually moved, so the
  // caller's own request can decide that is a no-op rather than this
  // view trying to.
  signal pinDropped(string id, int toIndex)

  // positionOn scrolls the row with this id into view.
  function positionOn(id: string): void {
    const index = root._indexOf(id);
    if (index >= 0) list.positionViewAtIndex(index, ListView.Contain);
  }

  // _indexOf finds a row's position by id, across either model kind.
  function _indexOf(id) {
    const count = root._count();
    for (let i = 0; i < count; i++) {
      const row = root.model.get ? root.model.get(i) : root.model[i];
      if (row.id === id) return i;
    }
    return -1;
  }

  // _count returns the row count, across either model kind.
  function _count() {
    if (!root.model) return 0;
    return root.model.count !== undefined ? root.model.count : root.model.length;
  }

  // _pinnedCount returns how many rows at the top of the list are
  // pinned: pinned rows are always sorted as one contiguous group ahead
  // of everything else (see Rail.compareConversations), so this is
  // exactly how far a dragged pinned row may ever move.
  function _pinnedCount() {
    const count = root._count();
    let pinned = 0;
    while (pinned < count && (root.model.get ? root.model.get(pinned) : root.model[pinned]).pinned) pinned++;
    return pinned;
  }

  readonly property bool _empty: root._count() === 0

  implicitWidth: Style.space(260)
  implicitHeight: Style.space(400)

  ListView {
    id: list
    objectName: "conversationListView"
    anchors.fill: parent
    clip: true
    visible: !root._empty
    model: root.model
    delegate: rowDelegate
    ScrollBar.vertical: ScrollBar {}
  }

  // Empty states: no conversations at all, or none matching a search.
  Text {
    objectName: "emptyState"
    visible: root._empty
    anchors.centerIn: parent
    width: parent.width - Theme.spacing.xl * 2
    horizontalAlignment: Text.AlignHCenter
    wrapMode: Text.WordWrap
    textFormat: Text.PlainText
    text: root.query.length > 0
      ? "No chats match “" + root.query + "”"
      : root.unreadView
      ? "No unread conversations · Esc to leave"
      : "No conversations yet · Ctrl+N to start one"
    color: Util.alpha(Theme.foreground, 0.6)
    font.family: Theme.font.family
    font.pixelSize: Theme.font.body
  }

  Component {
    id: rowDelegate

    Item {
      id: wrapper
      required property int index
      required property string id
      required property string title
      // titleVerified is whether title is a saved contact's name, a
      // verified business name or a group's own name, rather than a
      // bare, self-chosen one; absent (so false) for a row from an
      // older helper, which ConversationRow then marks with "~" (see
      // Verify.markedName and docs/decisions.md).
      required property bool titleVerified
      required property string kind
      required property string service
      required property string accountId
      required property int unread
      required property bool muted
      required property bool pinned
      required property bool archived
      required property bool hidden
      required property real lastActivity
      required property string preview
      required property string previewSender
      required property bool previewOutgoing
      required property string match
      required property bool dimmed
      required property string dimLabel
      required property real reminderAt
      required property bool reminderDue
      required property bool blocked

      // _dragStartIndex is where this row sat when a drag on it began,
      // -1 while none is in progress: the drop target is always worked
      // out from this fixed start plus the pointer's own total travel,
      // never from this row's own index, which keeps changing as the
      // live reorder below moves it.
      property int _dragStartIndex: -1

      width: list.width
      height: row.implicitHeight

      DragHandler {
        id: pinDrag
        target: null
        xAxis.enabled: false
        enabled: wrapper.pinned && typeof root.model.move === "function"
        // ConversationRow's own MouseArea (for the plain click that
        // opens a chat) grabs on press by default; this lets a real
        // drag - movement past Qt's own drag threshold - take that
        // grab over instead of being shut out of it, the same way any
        // PointerHandler must when it shares an item with a MouseArea.
        grabPermissions: PointerHandler.CanTakeOverFromItems

        onActiveChanged: {
          if (active) {
            wrapper._dragStartIndex = wrapper.index;
            return;
          }

          const droppedAt = wrapper.index;
          wrapper._dragStartIndex = -1;
          root.pinDropped(wrapper.id, droppedAt);
        }

        onTranslationChanged: {
          if (!active || wrapper._dragStartIndex < 0) return;

          const steps = Math.round(translation.y / wrapper.height);
          const pinnedCount = root._pinnedCount();
          const target = Math.max(0, Math.min(pinnedCount - 1, wrapper._dragStartIndex + steps));
          if (target !== wrapper.index) root.model.move(wrapper.index, target, 1);
        }
      }

      ConversationRow {
        id: row
        width: wrapper.width
        selected: wrapper.id === root.selectedId
        query: root.query
        showAccount: root.multiAccountServices.includes(wrapper.service)
        accountName: root.accountNames[wrapper.accountId] ?? ""
        showAccountColor: root.showAccountColors
        accountColor: root.accountColors[wrapper.accountId] ?? "transparent"
        dimmed: wrapper.dimmed
        dimLabel: wrapper.dimLabel
        reminderDue: wrapper.reminderDue
        conversation: {
          "id": wrapper.id,
          "title": wrapper.title,
          "titleVerified": wrapper.titleVerified,
          "kind": wrapper.kind,
          "service": wrapper.service,
          "accountId": wrapper.accountId,
          "unread": wrapper.unread,
          "muted": wrapper.muted,
          "pinned": wrapper.pinned,
          "archived": wrapper.archived,
          "hidden": wrapper.hidden,
          "lastActivity": wrapper.lastActivity,
          "preview": wrapper.preview,
          "previewSender": wrapper.previewSender,
          "previewOutgoing": wrapper.previewOutgoing,
          "match": wrapper.match,
          "reminderAt": wrapper.reminderAt,
          "blocked": wrapper.blocked
        }

        onClicked: root.activated(wrapper.id)
      }
    }
  }
}
