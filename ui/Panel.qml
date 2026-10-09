import QtQuick
import QtQuick.Layouts
import Quickshell
import Quickshell.Wayland
import qs.Commons
import qs.Ui as Ui
import "theme"
import "components"
import "controllers"
import "lib/Keymap.js" as Keymap
import "lib/Navigation.js" as Navigation

// The OmaMessenger window: the five controllers from ui/controllers, the
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

  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides, computed in Service.qml), used for key
  // routing and read by every view that shows a shortcut.
  readonly property var bindings: Keymap.effectiveBindings(root.service)

  // windowFocused is true while the window is shown and has keyboard
  // focus: the only time the user is looking at the open conversation.
  readonly property bool windowFocused: window.visible && keyArea.Window.active
  onWindowFocusedChanged: conversationController.setWindowActive(root.windowFocused)

  // helperInstallFailed tracks the Retry button's own visible condition,
  // so focus follows it there the moment it shows: a user who just saw
  // the helper fail to install should not have to tab to find out how to
  // try again. Qt.callLater waits a turn of the event loop, the same as
  // the photo viewer's own focus does, since an item still hidden when
  // this changes cannot take focus.
  readonly property bool helperInstallFailed: !!(root.service && root.service.status === "installFailed")
  onHelperInstallFailedChanged: if (root.helperInstallFailed) Qt.callLater(() => retryButton.forceActiveFocus())
  Component.onCompleted: {
    if (!root.service) console.warn("OmaMessenger: the window opened without its service");
    conversationController.setWindowActive(root.windowFocused);
  }

  // hidingByChoice is true only while _hide() lowers the window after the
  // user answered the close question.
  property bool hidingByChoice: false

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
    // "missing" means no attempt has been made yet (install() leaves a
    // failed attempt at "installFailed" instead), so opening the window
    // is the one moment this starts without the user clicking anything.
    if (root.service && root.service.status === "missing") root.service.installHelper();
    window.visible = true;
    keyArea.forceActiveFocus();

    const conversationId = root._conversationIdFrom(payloadJson);
    if (conversationId) root._openConversationId(conversationId);
    if (alreadyVisible) root._focusWindow();
  }

  // _focusWindow asks the compositor to raise and focus this panel's own
  // window, the way Omarchy's own bar widget refocuses windows: through
  // the Wayland foreign-toplevel protocol's activate request. A Hyprland
  // dispatch string ("hl.dsp.focus({ window = "title:…" })") looked right
  // but Hyprland 0.56 reported "window not found" for it, and dispatch
  // syntax is tied to the Hyprland version; activate() is not.
  function _focusWindow(): void {
    const toplevels = ToplevelManager.toplevels.values;
    for (let i = 0; i < toplevels.length; i++) {
      if (toplevels[i].title === window.title) {
        toplevels[i].activate();
        return;
      }
    }
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
    const action = Keymap.match(context, key, modifiers, text, root.bindings);
    if (!action) return false;

    const controllers = [listController, conversationController, composerController, photoViewerController,
      reactionsController, pollsController, deleteController, dialogController, accountController, windowController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    return owner.run(action) !== false;
  }

  // _navState assembles what Navigation.keyContext needs, through the
  // same Navigation.buildState WindowController's own Escape chain
  // uses, so the two cannot drift the way they once did: this one left
  // out the health check report, and a key pressed while it covered the
  // window fell through to the conversation underneath it.
  function _navState(): var {
    return Navigation.buildState({
      service: root.service,
      windowController: windowController,
      accountController: accountController,
      photoViewerController: photoViewerController,
      reactionsController: reactionsController,
      pollsController: pollsController,
      deleteController: deleteController,
      listController: listController,
      dialogController: dialogController,
      composerController: composerController,
      conversationController: conversationController
    });
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
    if (root.service.status === "installing") return "Installing the helper…";
    if (root.service.status === "installFailed") return "Could not install the helper: " + root.service.detail;
    if (root.service.status === "missing") return "Installing the helper…";
    if (root.service.status === "error") return "The helper stopped: " + root.service.detail;
    return "Starting the helper…";
  }

  ListController {
    id: listController
    service: root.service

    onConversationFolded: (id) => conversationController.closeIfOpen(id)
  }

  ConversationController {
    id: conversationController
    service: root.service
    listController: listController
    composer: composerController
    photoViewer: photoViewerController
    voiceController: voiceNoteController
  }

  VoiceNoteController {
    id: voiceNoteController
  }

  ComposerController {
    id: composerController
    service: root.service
    conversation: conversationController
  }

  PhotoViewerController {
    id: photoViewerController
    conversation: conversationController
  }

  ReactionsController {
    id: reactionsController
    service: root.service
    conversation: conversationController
  }

  PollsController {
    id: pollsController
    service: root.service
    conversation: conversationController
  }

  DeleteController {
    id: deleteController
    service: root.service
    conversation: conversationController
  }

  DialogController {
    id: dialogController
    service: root.service

    onOpened: (conversation) => conversationController.open(conversation)
  }

  AccountController {
    id: accountController
    service: root.service
  }

  WindowController {
    id: windowController
    service: root.service
    listController: listController
    conversationController: conversationController
    composerController: composerController
    photoViewerController: photoViewerController
    reactionsController: reactionsController
    pollsController: pollsController
    deleteController: deleteController
    dialogController: dialogController
    accountController: accountController

    onHideRequested: root._hide()
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
      objectName: "keyArea"
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
            textFormat: Text.PlainText
            elide: Text.ElideRight
            color: Color.foreground
            font.family: Theme.font.family
            font.pixelSize: Theme.font.bodySmall
          }

          Ui.Button {
            id: retryButton
            objectName: "retryButton"
            text: "Retry (" + Keymap.keyFor("helper.retryInstall", root.bindings) + ")"
            visible: root.service && root.service.status === "installFailed"
            focusable: true
            onClicked: windowController.retryInstall()
          }
        }

        Text {
          id: errorLine
          Layout.fillWidth: true
          readonly property string message: conversationController.lastError || reactionsController.lastError
            || deleteController.lastError || listController.lastError || dialogController.lastError || windowController.lastError
          visible: errorLine.message.length > 0
          text: errorLine.message
          textFormat: Text.PlainText
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
          composerController: composerController
          photoViewerController: photoViewerController
          reactionsController: reactionsController
          pollsController: pollsController
          deleteController: deleteController
          dialogController: dialogController
          accountController: accountController
          windowController: windowController
          bindings: root.bindings
          routeKey: root.routeKey
          focusDefault: () => keyArea.forceActiveFocus()
        }

        RowLayout {
          Layout.fillWidth: true
          Layout.preferredHeight: Style.space(24)
          spacing: Theme.spacing.md

          KeyHints {
            objectName: "keyHints"
            Layout.fillWidth: true
            context: Navigation.keyContext(root._navState())
            bindings: root.bindings
          }

          ReadReceiptsIndicator {
            objectName: "readReceiptsIndicator"
            active: root.service !== null && root.service.readReceipts === false
          }
        }
      }
    }
  }
}
