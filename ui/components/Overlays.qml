pragma ComponentBehavior: Bound
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
  // windowActive is whether the user can see the window at all: shown
  // and focused. Forwarded to the photo viewer's own video player, so
  // leaving the window unloads it the same way an inline video bubble
  // already does.
  property bool windowActive: true
  // listController is bound into the archive-all question.
  property var listController: null
  // conversationController is bound into the risky-open question.
  property var conversationController: null
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), read for the photo viewer's own label.
  readonly property var bindings: Keymap.effectiveBindings(root.service)
  // windowController is bound into the command palette and the close
  // question.
  property var windowController: null
  // dialogController is bound into the new-chat dialog.
  property var dialogController: null
  // accountController is bound into account setup and its removal.
  property var accountController: null
  // photoViewerController is bound into the in-app photo viewer, which
  // also shows a video or GIF (see PhotoViewer.qml).
  property var photoViewerController: null
  // videoController supplies whether in-window video playback is
  // available and the shared mute preference, for the viewer's own
  // video player.
  property var videoController: null
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

  // _refocusIfClosed returns keyboard focus to Panel's own key area once
  // an overlay that took it has closed: every overlay below whose open
  // state is a single property shares this one check on its
  // onOpenChanged, instead of repeating the same two-part condition at
  // each one.
  function _refocusIfClosed(open: bool): void {
    if (!open && root.focusDefault) root.focusDefault();
  }

  // _refocusIfSetupClosed returns keyboard focus to the key area once
  // account setup and its removal question are both closed: whichever
  // button or field had focus is gone by then, cancelling, finishing a
  // sign-in and the helper reporting an account removed all end here, so
  // one check after each covers every way out instead of a focusDefault
  // call scattered over every signal that can close them.
  function _refocusIfSetupClosed(): void {
    if (!root.accountController || root.accountController.open || root.accountController.removing || root.accountController.renaming) return;
    root._refocusIfClosed(false);
  }

  // Closing the new-chat dialog returns focus to the key area, the same
  // as every other overlay below: without this, nothing else ever
  // reclaims it. Opening it focuses its own search field, in
  // NewChatDialog's own Component.onCompleted, since the dialog (and
  // that field) exists only while open; see the Loader below.
  Connections {
    target: root.dialogController
    function onOpenChanged() { root._refocusIfClosed(root.dialogController.open); }
  }

  // Returns focus to the key area once the command palette closes, for
  // the same reason as the new-chat dialog above: the palette's own
  // onOpenChanged cannot report that transition any more once it exists
  // only while open (see the Loader below).
  Connections {
    target: root.windowController
    function onPaletteOpenChanged() { root._refocusIfClosed(root.windowController.paletteOpen); }
  }

  // Same reason again for the "What's new" overlay, also Loader-wrapped
  // below.
  Connections {
    target: root.windowController
    function onWhatsNewOpenChanged() { root._refocusIfClosed(root.windowController.whatsNewOpen); }
  }

  ReactionPicker {
    open: root.reactionsController.pickerOpen
    emojis: root.reactionsController.pickerEmojis
    currentIndex: root.reactionsController.pickerIndex
    routeKey: root.routeKey
    onPicked: index => root.reactionsController.pickAt(index)
    onCancelled: root.reactionsController.closePicker()
    onOpenChanged: root._refocusIfClosed(open)
  }

  // Active only while the palette is open, so its search TextField is
  // destroyed on close rather than merely hidden: a hidden-but-alive
  // TextField still reads clipboard changes on supported Qt versions,
  // with no size limit or deadline, which a maintainer flagged as a way
  // to stall or exhaust the shared shell with a streaming clipboard
  // offer before any paste ever happens.
  Loader {
    id: paletteLoader
    anchors.fill: parent
    active: root.windowController.paletteOpen

    sourceComponent: CommandPalette {
      open: true
      placeholder: root.windowController.paletteMode === "conversations" ? "Jump to a conversation or search messages"
        : root.windowController.paletteMode === "links" ? "Open which link?"
        : root.windowController.paletteMode === "snoozeCustom" ? "e.g. 2h, 18:00, mon 9:00"
        : "Type a command"
      items: root.windowController.paletteItems
      sectioned: root.windowController.paletteMode === "conversations"
      currentIndex: root.windowController.paletteIndex
      routeKey: root.routeKey
      onQueryEdited: text => root.windowController.setPaletteQuery(text)
      onAccepted: index => root.windowController.acceptPalette(index)
      onCancelled: root.windowController.closePalette()
    }
  }

  DoctorReport {
    open: root.windowController.doctorOpen
    checks: root.windowController.doctorChecks
    onClosed: root.windowController.closeDoctor()
    onOpenChanged: root._refocusIfClosed(open)
  }

  PhotoViewer {
    objectName: "photoViewer"
    anchors.fill: parent
    open: root.photoViewerController.viewerOpen
    photo: root.photoViewerController.viewerPhoto
    path: root.photoViewerController.viewerPath
    decodeAs: root.photoViewerController.viewerDecodeAs
    videoAvailable: root.videoController ? root.videoController.available : true
    videoPlayable: root.photoViewerController.viewerPlayable
    windowActive: root.windowActive
    videoMuted: root.videoController ? root.videoController.muted : true
    routeKey: root.routeKey
    bindings: root.bindings

    onClosed: root.photoViewerController.close()
    onOpenExternally: root.photoViewerController.openExternally()
    onVideoMuteToggled: { if (root.videoController) root.videoController.setMuted(!root.videoController.muted); }
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
    onOpenChanged: root._refocusIfClosed(open)
  }

  // Tests may build this layout without a list controller.
  ArchiveAllConfirm {
    open: root.listController?.archiveAllOpen ?? false
    count: root.listController?.archiveAllCount ?? 0
    routeKey: root.routeKey
    onConfirmed: root.listController.confirmArchiveAllRead()
    onCancelled: root.listController.cancelArchiveAllRead()
    onOpenChanged: root._refocusIfClosed(open)
  }

  // Tests may build this layout without a list controller.
  BlockConfirm {
    open: root.listController?.blockConfirmOpen ?? false
    kind: root.listController?.blockConfirmKind ?? "block"
    routeKey: root.routeKey
    onConfirmed: root.listController.confirmBlock()
    onCancelled: root.listController.cancelBlock()
    onOpenChanged: root._refocusIfClosed(open)
  }

  // Tests may build this layout without a conversation controller.
  OpenRiskyConfirm {
    open: root.conversationController?.riskyOpenConfirmOpen ?? false
    routeKey: root.routeKey
    onConfirmedOpen: root.conversationController.confirmRiskyOpen()
    onSavedToDownloads: root.conversationController.saveRiskyToDownloads()
    onCancelled: root.conversationController.cancelRiskyOpen()
    onOpenChanged: root._refocusIfClosed(open)
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
      root._refocusIfClosed(root.windowController.confirmingClose);
    }
  }

  // Active only while setup is open, so its credential, phone, code and
  // password fields are destroyed on close rather than merely hidden;
  // see the palette's own Loader above for why. AccountController keeps
  // the step itself (stage, serviceName, …), so closing and reopening
  // still resumes wherever a sign-in in progress left off. Tests may
  // build the layout without an account controller.
  Loader {
    id: accountSetupLoader
    anchors.fill: parent
    active: root.accountController?.open ?? false

    sourceComponent: AccountSetup {
      stage: root.accountController?.stage ?? "credentials"
      services: root.service ? root.service.services : []
      serviceName: root.accountController?.serviceName ?? "Telegram"
      chooseIndex: root.accountController?.chooseIndex ?? -1
      qr: root.accountController?.qr ?? ""
      hint: root.accountController?.hint ?? ""
      passwordHint: root.accountController?.passwordHint ?? ""
      error: root.accountController?.lastError ?? ""
      busy: root.accountController?.busy ?? false
      routeKey: root.routeKey

      onServiceChosen: serviceId => root.accountController.chooseService(serviceId)
      onCredentialsSubmitted: (apiId, apiHash) => root.accountController.submitCredentials(apiId, apiHash)
      onPhoneRequested: root.accountController.usePhone()
      onQrRequested: root.accountController.useQr()
      onAnswered: value => root.accountController.answer(value)
      onCancelled: root.accountController.cancel()
      onLinkOpenRequested: url => root.windowController.openLink(url)
    }
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

  // Active only while renaming, so its alias TextField is destroyed on
  // close rather than merely hidden; see the palette's own Loader above
  // for why. AccountController keeps the chosen account and its
  // prefilled alias, so reopening mid-flow (there is none here, but the
  // pattern matches account setup) would resume cleanly too.
  Loader {
    id: renameAccountLoader
    anchors.fill: parent
    active: root.accountController?.renaming ?? false

    sourceComponent: RenameAccount {
      open: true
      stage: (root.accountController?.renameAccountId ?? "") === "" ? "choose" : "alias"
      accounts: root.service ? root.service.accounts : []
      chooseIndex: root.accountController?.renameIndex ?? -1
      currentAlias: root.accountController?.renameAlias ?? ""
      error: root.accountController?.lastError ?? ""
      busy: root.accountController?.busy ?? false
      knownServices: root.service ? root.service.services : []
      routeKey: root.routeKey

      onChosen: accountId => root.accountController.chooseRename(accountId)
      onSubmitted: alias => root.accountController.submitAlias(alias)
      onCancelled: root.accountController.cancel()
    }
  }

  // Covers every way account setup, its removal question or its rename
  // dialog can close: cancelling, a sign-in finishing, saving an alias,
  // or the helper reporting an account removed. Without this, the
  // control that had focus is simply gone once the overlay hides, and
  // the key area never gets it back, so shortcuts stop doing anything
  // until the window is closed and reopened.
  Connections {
    target: root.accountController
    function onOpenChanged() { root._refocusIfSetupClosed(); }
    function onRemovingChanged() { root._refocusIfSetupClosed(); }
    function onRenamingChanged() { root._refocusIfSetupClosed(); }
  }

  // Active only while the dialog is open, so its contact search
  // TextField is destroyed on close rather than merely hidden; see the
  // palette's own Loader above for why.
  Loader {
    id: newChatDialogLoader
    anchors.fill: parent
    active: root.dialogController.open

    sourceComponent: NewChatDialog {
      objectName: "newChatDialog"

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

  // Active only while open, matching every other Loader-wrapped overlay
  // above.
  Loader {
    id: whatsNewLoader
    anchors.fill: parent
    active: root.windowController.whatsNewOpen

    sourceComponent: WhatsNewOverlay {
      open: true
      releases: root.service ? root.service.whatsNewReleases : []
      highlightedIndex: root.windowController.whatsNewIndex
      onClosed: root.windowController.closeWhatsNew()
    }
  }
}
