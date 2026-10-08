import QtQuick
import "../lib/Keymap.js" as Keymap

// The windows that float over the three columns: the reaction picker, the
// command palette, the health check report, the in-app photo viewer, the
// close question, account setup and removal, and the new-chat dialog. Split out of
// MessengerLayout to keep that file within the size guideline: like it,
// this is allowed to call the controllers directly, since it is part of
// Panel.qml's own body rather than a view under ui/components proper.
Item {
  id: root

  // service is the Service instance, read for the lists of known and
  // signed-in services the account and new-chat overlays show.
  property var service: null
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), read for the photo viewer's own label.
  readonly property var bindings: (root.service && root.service.effectiveBindings) || Keymap.BINDINGS
  // windowController is bound into the command palette and the close
  // question.
  property var windowController: null
  // dialogController is bound into the new-chat dialog.
  property var dialogController: null
  // accountController is bound into account setup and its removal.
  property var accountController: null
  // photoViewerController is bound into the in-app photo viewer.
  property var photoViewerController: null
  // reactionsController is bound into the emoji picker.
  property var reactionsController: null
  // deleteController is bound into the delete question.
  property var deleteController: null
  // routeKey is forwarded to every overlay with its own key handling; see
  // Panel.qml for why it is a function property rather than a signal.
  property var routeKey: null
  // focusDefault returns keyboard focus to Panel's own key area once an
  // overlay that took it closes.
  property var focusDefault: null

  // _refocusIfSetupClosed returns keyboard focus to the key area once
  // account setup and its removal question are both closed: whichever
  // button or field had focus is gone by then, cancelling, finishing a
  // sign-in and the helper reporting an account removed all end here, so
  // one check after each covers every way out instead of a focusDefault
  // call scattered over every signal that can close them.
  function _refocusIfSetupClosed(): void {
    if (!root.accountController || root.accountController.open || root.accountController.removing) return;
    if (root.focusDefault) root.focusDefault();
  }

  // Opening the new-chat dialog focuses its search field.
  Connections {
    target: root.dialogController
    function onOpenChanged() {
      if (root.dialogController.open) dialog.focusSearch()
    }
  }

  ReactionPicker {
    open: root.reactionsController.pickerOpen
    emojis: root.reactionsController.pickerEmojis
    currentIndex: root.reactionsController.pickerIndex
    routeKey: root.routeKey
    onPicked: index => root.reactionsController.pickAt(index)
    onCancelled: root.reactionsController.closePicker()
    onOpenChanged: if (!open && root.focusDefault) root.focusDefault()
  }

  CommandPalette {
    open: root.windowController.paletteOpen
    placeholder: root.windowController.paletteMode === "conversations" ? "Jump to a conversation or search messages"
      : root.windowController.paletteMode === "links" ? "Open which link?"
      : "Type a command"
    items: root.windowController.paletteItems
    sectioned: root.windowController.paletteMode === "conversations"
    currentIndex: root.windowController.paletteIndex
    routeKey: root.routeKey
    onQueryEdited: text => root.windowController.setPaletteQuery(text)
    onAccepted: index => root.windowController.acceptPalette(index)
    onCancelled: root.windowController.closePalette()
    onOpenChanged: if (!open && root.focusDefault) root.focusDefault()
  }

  DoctorReport {
    open: root.windowController.doctorOpen
    checks: root.windowController.doctorChecks
    onClosed: root.windowController.closeDoctor()
    onOpenChanged: if (!open && root.focusDefault) root.focusDefault()
  }

  PhotoViewer {
    objectName: "photoViewer"
    anchors.fill: parent
    open: root.photoViewerController.viewerOpen
    photo: root.photoViewerController.viewerPhoto
    path: root.photoViewerController.viewerPath
    routeKey: root.routeKey
    bindings: root.bindings

    onClosed: root.photoViewerController.close()
    onOpenExternally: root.photoViewerController.openExternally()
  }

  CloseConfirm {
    open: root.windowController.confirmingClose
    highlightIndex: root.windowController.confirmIndex
    routeKey: root.routeKey
    onKeep: root.windowController.keepInBackground()
    onQuit: root.windowController.quit()
    onCancelled: root.windowController.cancelClose()
  }

  // Tests may build this layout without a delete controller.
  DeleteConfirm {
    open: root.deleteController?.open ?? false
    targetIsOwn: root.deleteController?.targetIsOwn ?? false
    highlightIndex: root.deleteController?.highlightIndex ?? 0
    routeKey: root.routeKey
    onEveryone: root.deleteController.run("delete.everyone")
    onForMe: root.deleteController.run("delete.forMe")
    onCancelled: root.deleteController.close()
    onOpenChanged: if (!open && root.focusDefault) root.focusDefault()
  }

  // Escape cancels the close question straight through WindowController,
  // never through CloseConfirm's own cancelled() signal, so this is the
  // one place that covers every way the question closes: the buttons, an
  // outside click and Escape alike. Without it, nothing has active
  // keyboard focus afterward and every shortcut goes quiet until the
  // window is closed and reopened.
  Connections {
    target: root.windowController
    function onConfirmingCloseChanged() {
      if (!root.windowController.confirmingClose && root.focusDefault) root.focusDefault();
    }
  }

  // Tests may build the layout without an account controller.
  AccountSetup {
    visible: root.accountController?.open ?? false

    stage: root.accountController?.stage ?? "credentials"
    services: root.service ? root.service.services : []
    serviceName: root.accountController?.serviceName ?? "Telegram"
    chooseIndex: root.accountController?.chooseIndex ?? -1
    qr: root.accountController?.qr ?? ""
    hint: root.accountController?.hint ?? ""
    error: root.accountController?.lastError ?? ""
    busy: root.accountController?.busy ?? false
    routeKey: root.routeKey

    onServiceChosen: serviceId => root.accountController.chooseService(serviceId)
    onCredentialsSubmitted: (apiId, apiHash) => root.accountController.submitCredentials(apiId, apiHash)
    onPhoneRequested: root.accountController.usePhone()
    onQrRequested: root.accountController.useQr()
    onAnswered: value => root.accountController.answer(value)
    onCancelled: root.accountController.cancel()
  }

  RemoveAccount {
    open: root.accountController?.removing ?? false
    accounts: root.service ? root.service.accounts : []
    removeIndex: root.accountController?.removeIndex ?? -1
    error: root.accountController?.lastError ?? ""
    knownServices: root.service ? root.service.services : []
    routeKey: root.routeKey

    onChosen: accountId => root.accountController.remove(accountId)
    onCancelled: root.accountController.cancel()
  }

  // Covers every way account setup or its removal question can close:
  // cancelling, a sign-in finishing, or the helper reporting an account
  // removed. Without this, the control that had focus is simply gone
  // once the overlay hides, and the key area never gets it back, so
  // shortcuts stop doing anything until the window is closed and reopened.
  Connections {
    target: root.accountController
    function onOpenChanged() { root._refocusIfSetupClosed(); }
    function onRemovingChanged() { root._refocusIfSetupClosed(); }
  }

  NewChatDialog {
    objectName: "newChatDialog"
    id: dialog
    anchors.fill: parent
    visible: root.dialogController.open

    accounts: root.service ? root.service.accounts : []
    accountId: root.dialogController.accountId
    knownServices: root.service ? root.service.services : []
    query: root.dialogController.query
    contacts: root.dialogController.contacts
    currentIndex: root.dialogController.currentIndex
    routeKey: root.routeKey

    onAccountChanged: id => root.dialogController.setAccount(id)
    onQueryEdited: text => root.dialogController.setQuery(text)
    onAccepted: (accountId, contactId) => root.dialogController.openConversation(accountId, contactId)
    onCancelled: root.dialogController.close()
  }
}
