import QtQuick
import QtQuick.Layouts
import Quickshell
import Quickshell.Hyprland
import qs.Commons
import qs.Ui as Ui
import "theme"
import "components"
import "lib/Format.js" as Format

// The walking skeleton: a conversation list and the open conversation,
// enough to prove the helper, the protocol and the UI are wired end to
// end. The full three-column layout, keyboard routing and controllers
// replace this once they exist.
Item {
  id: root

  // service is the Service singleton the host injects; it owns the
  // helper connection and the state that survives this panel being
  // destroyed on hide.
  property var service: null

  // shell is the host facade used to tell it this panel closed.
  property var shell: null

  // closingFromHost is true only while close() is hiding the window, so
  // the visibility handler does not report back a close the host itself
  // asked for.
  property bool closingFromHost: false

  // _selectedId is the open conversation, or "" when none is open.
  property string _selectedId: ""

  // open shows the window and, if the payload names a conversation,
  // opens it. Omarchy calls this every time the panel is summoned, even
  // while it is already open, so it always reloads the conversation list
  // and only focuses an already-visible window rather than reshowing it.
  function open(payloadJson): void {
    const alreadyVisible = window.visible;
    root.closingFromHost = false;
    window.visible = true;
    root._loadConversations();

    const conversationId = root._conversationIdFrom(payloadJson);
    if (conversationId) root._selectConversation(conversationId);
    if (alreadyVisible) Hyprland.dispatch("focuswindow title:^OmaMessenger$");
  }

  // close hides the window on the host's request, without reporting back.
  function close(): void {
    root.closingFromHost = true;
    window.visible = false;
    root.closingFromHost = false;
  }

  // _conversationIdFrom reads conversationId out of open()'s JSON
  // payload, or "" if there is none.
  function _conversationIdFrom(payloadJson: string): string {
    if (!payloadJson) return "";

    try {
      const parsed = JSON.parse(String(payloadJson));
      return parsed && typeof parsed.conversationId === "string" ? parsed.conversationId : "";
    } catch (e) {
      return "";
    }
  }

  // _loadConversations replaces the conversation list from the helper.
  function _loadConversations(): void {
    if (!root.service) return;

    root.service.request("conversations.list", {}, function(error, result) {
      if (error || !result) return;

      conversationsModel.clear();
      for (const conv of result) conversationsModel.append(conv);
    });
  }

  // _upsertConversation updates one row in place, or adds it at the top
  // when it is new.
  function _upsertConversation(conv: var): void {
    for (let i = 0; i < conversationsModel.count; i++) {
      if (conversationsModel.get(i).id === conv.id) {
        conversationsModel.set(i, conv);
        return;
      }
    }

    conversationsModel.insert(0, conv);
  }

  // _selectConversation opens one conversation: loads its messages, marks
  // it read, tells the helper the user is looking at it, and remembers
  // the choice in service.uiState so it survives the panel being
  // recreated.
  function _selectConversation(id: string): void {
    if (!root.service || id === root._selectedId) return;

    root._selectedId = id;
    messagesModel.clear();
    root.service.request("messages.list", { conversationId: id, limit: 50 }, function(error, result) {
      if (error || !result) return;
      for (const m of result.messages) root._upsertMessage(m);
    });
    root.service.request("conversations.markRead", { conversationId: id }, function() {});
    root.service.request("ui.setFocus", { conversationId: id, windowActive: true }, function() {});

    const state = root.service.uiState;
    state.selectedId = id;
    root.service.uiState = state;
  }

  // _upsertMessage updates one message in place, or appends it when it is
  // new.
  function _upsertMessage(message: var): void {
    for (let i = 0; i < messagesModel.count; i++) {
      if (messagesModel.get(i).id === message.id) {
        messagesModel.set(i, message);
        return;
      }
    }

    messagesModel.append(message);
  }

  // _sendMessage sends the composer's text to the open conversation.
  function _sendMessage(text: string): void {
    if (!root.service || !root._selectedId) return;

    root.service.request("messages.send", { conversationId: root._selectedId, text: text }, function(error, result) {
      if (!error && result) root._upsertMessage(result);
    });
  }

  // _handleEvent refreshes the lists a helper notification affects.
  function _handleEvent(name: string, data: var): void {
    if (name === "conversation.updated") {
      root._upsertConversation(data);
    } else if ((name === "message.added" || name === "message.updated") && data.conversationId === root._selectedId) {
      root._upsertMessage(data);
    }
  }

  ListModel { id: conversationsModel }
  ListModel { id: messagesModel }

  Connections {
    target: root.service
    function onEvent(name, data) { root._handleEvent(name, data); }
  }

  FloatingWindow {
    id: window
    title: "OmaMessenger"
    color: Color.background
    implicitWidth: Style.space(1120)
    implicitHeight: Style.space(760)
    minimumSize: Qt.size(Style.space(760), Style.space(540))

    onVisibleChanged: {
      if (!window.visible && !root.closingFromHost && root.shell && typeof root.shell.hide === "function") {
        root.shell.hide("io.github.omamessenger");
      }
    }

    Shortcut {
      sequence: "Return"
      context: Qt.WindowShortcut
      enabled: composer.input.activeFocus
      onActivated: composer.submit()
    }

    ColumnLayout {
      anchors.fill: parent
      anchors.margins: Theme.spacing.md
      spacing: Theme.spacing.sm

      Row {
        Layout.fillWidth: true
        visible: !root.service || root.service.status !== "ready"
        spacing: Theme.spacing.sm

        Text {
          text: !root.service ? "Waiting for the helper service…"
            : root.service.status === "missing" ? "The helper is not installed."
            : root.service.status === "error" ? "The helper stopped: " + root.service.detail
            : "Starting the helper…"
          color: Color.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.bodySmall
        }

        Ui.Button {
          text: "Install helper"
          visible: root.service && root.service.status === "missing"
          onClicked: root.service.installHelper()
        }
      }

      RowLayout {
        Layout.fillWidth: true
        Layout.fillHeight: true
        spacing: Theme.spacing.sm

        ListView {
          id: conversationList
          Layout.preferredWidth: Math.max(Style.space(260), Math.min(Style.space(360), window.width * 0.32))
          Layout.fillHeight: true
          clip: true
          model: conversationsModel
          delegate: conversationDelegate
        }

        ColumnLayout {
          Layout.fillWidth: true
          Layout.fillHeight: true
          spacing: Theme.spacing.sm

          ListView {
            id: messageList
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            spacing: Theme.spacing.xs
            model: messagesModel
            delegate: messageDelegate
          }

          Composer {
            id: composer
            Layout.fillWidth: true
            enabled: root._selectedId !== ""
            onSubmitted: function(text) { root._sendMessage(text); }
          }
        }
      }
    }
  }

  Component {
    id: conversationDelegate

    Rectangle {
      id: convRow
      required property string title
      required property int unread
      required property bool muted

      width: ListView.view.width
      height: Theme.spacing.controlHeight + Theme.spacing.md
      color: model.id === root._selectedId
        ? Util.alpha(Color.accent, Style.selectedFillAlpha)
        : (hoverArea.containsMouse ? Style.hoverFill : "transparent")

      RowLayout {
        anchors.fill: parent
        anchors.margins: Theme.spacing.sm
        spacing: Theme.spacing.sm

        Text {
          Layout.fillWidth: true
          text: convRow.title
          color: Color.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.body
          font.bold: convRow.unread > 0
          elide: Text.ElideRight
        }

        UnreadBadge {
          count: convRow.unread
          muted: convRow.muted
        }
      }

      MouseArea {
        id: hoverArea
        anchors.fill: parent
        hoverEnabled: true
        onClicked: root._selectConversation(model.id)
      }
    }
  }

  Component {
    id: messageDelegate

    Item {
      id: msgRow
      required property string text
      required property bool outgoing
      required property string status

      width: ListView.view.width
      height: label.implicitHeight + Theme.spacing.md

      Text {
        id: label
        anchors.right: msgRow.outgoing ? parent.right : undefined
        anchors.left: msgRow.outgoing ? undefined : parent.left
        width: Math.min(implicitWidth, msgRow.width * 0.72)
        text: msgRow.outgoing ? (msgRow.text + "  " + Format.statusGlyph(msgRow.status)) : msgRow.text
        color: Color.foreground
        font.family: Theme.font.family
        font.pixelSize: Theme.font.body
        wrapMode: Text.Wrap
      }
    }
  }
}
