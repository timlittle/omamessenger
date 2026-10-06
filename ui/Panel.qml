import QtQuick
import QtQuick.Layouts
import Quickshell
import Quickshell.Hyprland
import qs.Commons
import qs.Ui as Ui
import "theme"
import "components"
import "controllers"
import "lib/Keymap.js" as Keymap
import "lib/Navigation.js" as Navigation

// The OmaMessenger window: the four controllers from ui/controllers, the
// views they drive, and the one key router that decides which controller
// a key press belongs to. Omarchy destroys this whole tree when the
// window is hidden, so nothing here is state that must survive that; see
// ui/Service.qml for what does.
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

  // windowFocused is true while the window is shown and has keyboard
  // focus: the only time the user is looking at the open conversation.
  readonly property bool windowFocused: window.visible && keyArea.Window.active
  onWindowFocusedChanged: conversationController.setWindowActive(root.windowFocused)
  Component.onCompleted: conversationController.setWindowActive(root.windowFocused)

  // hidingByChoice is true only while _hide() lowers the window after the
  // user answered the close question.
  property bool hidingByChoice: false

  // nowMs refreshes every 30 seconds so the list and conversation can
  // recompute their relative time labels.
  property real nowMs: Date.now()

  // _pendingConversationId is a conversation open() was asked to open
  // before the list had loaded it, retried until it appears or this
  // gives up.
  property string _pendingConversationId: ""
  // _pendingAttempts counts the retries _tryOpenPending has made.
  property int _pendingAttempts: 0

  // open shows the window and, if the payload names a conversation,
  // opens it. Omarchy calls this every time the panel is summoned, even
  // while it is already open, so it always resets keyboard focus and
  // only focuses an already-visible window rather than reshowing it.
  function open(payloadJson: string): void {
    const alreadyVisible = window.visible;
    root.closingFromHost = false;
    if (root.service && root.service.status === "stopped") root.service.start();
    window.visible = true;
    keyArea.forceActiveFocus();

    const conversationId = root._conversationIdFrom(payloadJson);
    if (conversationId) root._openConversationId(conversationId);
    // Hyprland 0.56 takes Lua dispatchers; the old "focuswindow title:…"
    // string no longer parses.
    if (alreadyVisible) Hyprland.dispatch('hl.dsp.focus({ window = "title:^OmaMessenger$" })');
  }

  // close hides the window on the host's request, without reporting back.
  function close(): void {
    root.closingFromHost = true;
    window.visible = false;
    root.closingFromHost = false;
  }

  // routeKey matches a key press to an action for the current context and
  // runs whichever controller owns it, returning whether one did. It
  // takes the key's raw fields rather than the KeyEvent itself: a
  // KeyEvent copies when it crosses a signal from a field several
  // components away (the search field, the composer, the dialog), and
  // setting `accepted` on a copy would not stop the real one. Every
  // caller — the window's own key handler and those fields — gets a
  // plain boolean back and sets `accepted` on its own, real event.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const context = Navigation.keyContext(root._navState());
    const demo = root.service ? root.service.demo : false;
    const action = Keymap.match(context, key, modifiers, text, demo);
    if (!action) return false;

    const controllers = [listController, conversationController, dialogController, windowController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    owner.run(action);
    return true;
  }

  // _navState assembles what Navigation.keyContext needs from whichever
  // controller owns each piece of it.
  function _navState(): var {
    return {
      confirmOpen: windowController.confirmingClose,
      paletteOpen: windowController.paletteOpen,
      dialogOpen: dialogController.open,
      searchFocused: listController.searchFocused,
      composeFocused: conversationController.composeFocused,
      pane: conversationController.pane
    };
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

  // _openConversationId opens id once listController has loaded it,
  // retrying briefly since the panel was just (re)created and its list
  // may still be loading.
  function _openConversationId(id: string): void {
    root._pendingConversationId = id;
    root._pendingAttempts = 0;
    root._tryOpenPending();
  }

  // _tryOpenPending opens _pendingConversationId once it is found, or
  // gives up quietly after twenty tries (three seconds).
  function _tryOpenPending(): void {
    if (!root._pendingConversationId) return;

    const conversation = listController.findConversation(root._pendingConversationId);
    if (conversation) {
      conversationController.open(conversation);
      root._pendingConversationId = "";
      return;
    }

    root._pendingAttempts++;
    if (root._pendingAttempts < 20) pendingOpenTimer.restart();
  }

  // _hide lowers the window once the user has chosen to; onVisibleChanged
  // below reports it to the host.
  function _hide(): void {
    root.hidingByChoice = true;
    window.visible = false;
    root.hidingByChoice = false;
  }

  // _onWindowHidden handles the window going away. The host asking, or the
  // user choosing in the close question, is final. Anything else is the
  // compositor closing it, which cannot be refused, so the window comes
  // straight back with the question.
  function _onWindowHidden(): void {
    if (root.closingFromHost) return;

    if (root.hidingByChoice) {
      if (root.shell && typeof root.shell.hide === "function") root.shell.hide("io.github.omamessenger");
      return;
    }

    // Showing it again from inside its own hide notification is ignored,
    // so it happens on the next turn of the event loop.
    Qt.callLater(() => {
      window.visible = true;
      windowController.askToClose();
    });
  }

  // _helperStatusText names the current helper status for the row shown
  // while it is not ready.
  function _helperStatusText(): string {
    if (!root.service) return "Waiting for the helper service…";
    if (root.service.status === "missing") return "The helper is not installed.";
    if (root.service.status === "error") return "The helper stopped: " + root.service.detail;
    return "Starting the helper…";
  }

  ListController {
    id: listController
    service: root.service
  }

  ConversationController {
    id: conversationController
    service: root.service
    listController: listController
  }

  DialogController {
    id: dialogController
    service: root.service

    onOpened: (conversation) => conversationController.open(conversation)
  }

  WindowController {
    id: windowController
    service: root.service
    listController: listController
    conversationController: conversationController
    dialogController: dialogController

    onHideRequested: root._hide()
  }

  Timer {
    interval: 30000
    running: true
    repeat: true
    onTriggered: root.nowMs = Date.now()
  }

  Timer {
    id: pendingOpenTimer
    interval: 150
    onTriggered: root._tryOpenPending()
  }

  FloatingWindow {
    id: window
    objectName: "panelWindow"
    title: "OmaMessenger"
    color: Color.background
    implicitWidth: Style.space(1120)
    implicitHeight: Style.space(760)
    minimumSize: Qt.size(Style.space(420), Style.space(420))

    onVisibleChanged: {
      if (!window.visible) root._onWindowHidden();
    }

    Item {
      id: keyArea
      anchors.fill: parent
      focus: true

      Keys.onPressed: event => {
        if (root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
      }

      ColumnLayout {
        anchors.fill: parent
        anchors.margins: Theme.spacing.md
        spacing: Theme.spacing.sm

        RowLayout {
          Layout.fillWidth: true
          visible: !root.service || root.service.status !== "ready"
          spacing: Theme.spacing.sm

          Text {
            Layout.fillWidth: true
            text: root._helperStatusText()
            elide: Text.ElideRight
            color: Color.foreground
            font.family: Theme.font.family
            font.pixelSize: Theme.font.bodySmall
          }

          Ui.Button {
            objectName: "installButton"
            text: "Install helper"
            visible: root.service && root.service.status === "missing"
            onClicked: root.service.installHelper()
          }
        }

        Text {
          id: errorLine
          Layout.fillWidth: true
          readonly property string message: conversationController.lastError || listController.lastError
            || dialogController.lastError || windowController.lastError
          visible: errorLine.message.length > 0
          text: errorLine.message
          elide: Text.ElideRight
          color: Color.urgent
          font.family: Theme.font.family
          font.pixelSize: Theme.font.bodySmall
        }

        MessengerLayout {
          id: layout
          Layout.fillWidth: true
          Layout.fillHeight: true

          service: root.service
          listController: listController
          conversationController: conversationController
          dialogController: dialogController
          windowController: windowController
          nowMs: root.nowMs
          routeKey: root.routeKey
          focusDefault: () => keyArea.forceActiveFocus()
        }

        KeyHints {
          Layout.fillWidth: true
          Layout.preferredHeight: Style.space(24)
          context: Navigation.keyContext(root._navState())
        }
      }
    }
  }
}
