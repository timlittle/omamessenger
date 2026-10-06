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
  // nowMs: the current time, for each row's relative time label.
  property real nowMs: Date.now()
  // accountNames: accountId -> account name, for rows that show it.
  property var accountNames: ({})
  // multiAccountServices: services with more than one account.
  property var multiAccountServices: []

  // activated fires when a row is clicked.
  signal activated(string id)

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

  readonly property bool _empty: root._count() === 0

  implicitWidth: Style.space(260)
  implicitHeight: Style.space(400)

  ListView {
    id: list
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
    text: root.query.length > 0
      ? "No chats match “" + root.query + "”"
      : "No conversations yet · Ctrl+N to start one"
    color: Util.alpha(Color.foreground, 0.6)
    font.family: Theme.font.family
    font.pixelSize: Theme.font.body
  }

  Component {
    id: rowDelegate

    Item {
      id: wrapper
      required property string id
      required property string title
      required property string kind
      required property string service
      required property string accountId
      required property int unread
      required property bool muted
      required property real lastActivity
      required property string preview
      required property string previewSender
      required property bool previewOutgoing
      required property string match

      width: list.width
      height: row.implicitHeight

      ConversationRow {
        id: row
        width: wrapper.width
        selected: wrapper.id === root.selectedId
        query: root.query
        showAccount: root.multiAccountServices.includes(wrapper.service)
        accountName: root.accountNames[wrapper.accountId] ?? ""
        nowMs: root.nowMs
        conversation: {
          "id": wrapper.id,
          "title": wrapper.title,
          "kind": wrapper.kind,
          "service": wrapper.service,
          "accountId": wrapper.accountId,
          "unread": wrapper.unread,
          "muted": wrapper.muted,
          "lastActivity": wrapper.lastActivity,
          "preview": wrapper.preview,
          "previewSender": wrapper.previewSender,
          "previewOutgoing": wrapper.previewOutgoing,
          "match": wrapper.match
        }

        onClicked: root.activated(wrapper.id)
      }
    }
  }
}
