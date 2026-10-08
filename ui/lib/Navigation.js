.pragma library

// Navigation context and escape-chain logic: determines which key bindings apply
// and what Escape does based on UI state.

// keyContext determines the highest-priority context for key bindings based on
// the current UI state. Precedence: the close question > account setup > the
// photo viewer > the health check report > palette > reaction picker > poll
// vote mode > the delete question > the archive-all question > dialog >
// search > compose > conversation > list. The viewer, the reaction picker
// and poll vote mode are never open together, so their relative order
// only matters in theory.
function keyContext(state) {
  if (state.confirmOpen) return 'confirm';
  // setupContext names the exact step showing (chooseService, qr, phone,
  // removeAccount, …) so each one can answer to its own keys; "setup"
  // is the fallback for a caller that only knows setup is open.
  if (state.setupOpen) return state.setupContext || 'setup';
  if (state.viewerOpen) return 'viewer';
  if (state.doctorOpen) return 'doctor';
  if (state.paletteOpen) return 'palette';
  if (state.reactionPickerOpen) return 'reactionPicker';
  if (state.pollVoteOpen) return 'pollVote';
  if (state.deleteConfirmOpen) return 'deleteConfirm';
  if (state.archiveConfirmOpen) return 'archiveConfirm';
  if (state.dialogOpen) return 'dialog';
  if (state.searchFocused) return 'search';
  if (state.composeFocused) return 'compose';
  if (state.pane === 'conversation') return 'conversation';
  return 'list';
}

// escapeAction returns what Escape does: it undoes the innermost thing
// first (account setup, the photo viewer, the health check report, palette,
// the reaction picker, the delete question, dialog, search, composer, open
// conversation, query, the all-unreads view) and hides the window only when
// there is nothing left to undo. Inside the composer itself the order is:
// remove a pending attachment, then cancel a reply in progress, then leave
// the composer.
function escapeAction(state) {
  if (state.confirmOpen) return 'cancel-close';
  if (state.setupOpen) return 'close-setup';
  if (state.viewerOpen) return 'close-viewer';
  if (state.doctorOpen) return 'close-doctor';
  if (state.paletteOpen) return 'close-palette';
  if (state.reactionPickerOpen) return 'close-reaction-picker';
  if (state.deleteConfirmOpen) return 'close-delete-confirm';
  if (state.archiveConfirmOpen) return 'cancel-archive-all';
  if (state.dialogOpen) return 'close-dialog';

  if (state.searchFocused) {
    return state.query ? 'clear-search' : 'leave-search';
  }

  if (state.composeFocused) {
    if (state.hasAttachment) return 'clear-attachment';
    if (state.replying) return 'cancel-reply';
    return 'leave-compose';
  }

  if (state.activeId) {
    return 'close-conversation';
  }

  if (state.query) {
    return 'clear-search';
  }

  if (state.unreadView) {
    return 'leave-unread-view';
  }

  return 'hide-window';
}

// buildState assembles the state keyContext and escapeAction read from,
// out of whichever controllers ctx hands it. Panel's key router and
// WindowController's Escape chain each used to list every controller by
// hand in their own copy of this object, which is how Panel's once
// missed the health check report: a key pressed while it covered the
// window fell through to whatever context the conversation underneath
// was in. Both now call this one function instead, so a new piece of
// state only has to be wired in here.
function buildState(ctx) {
  const ui = ctx.service ? ctx.service.uiState : { pane: 'list', activeId: '' };

  return {
    confirmOpen: ctx.windowController ? ctx.windowController.confirmingClose : false,
    setupOpen: ctx.accountController ? (ctx.accountController.open || ctx.accountController.removing) : false,
    setupContext: ctx.accountController ? ctx.accountController.navContext : '',
    viewerOpen: ctx.photoViewerController ? ctx.photoViewerController.viewerOpen : false,
    doctorOpen: ctx.windowController ? ctx.windowController.doctorOpen : false,
    paletteOpen: ctx.windowController ? ctx.windowController.paletteOpen : false,
    reactionPickerOpen: ctx.reactionsController ? ctx.reactionsController.pickerOpen : false,
    pollVoteOpen: ctx.pollsController ? ctx.pollsController.voteTarget !== '' : false,
    deleteConfirmOpen: ctx.deleteController ? ctx.deleteController.open : false,
    archiveConfirmOpen: ctx.listController ? ctx.listController.archiveAllOpen : false,
    dialogOpen: ctx.dialogController ? ctx.dialogController.open : false,
    searchFocused: ctx.listController ? ctx.listController.searchFocused : false,
    composeFocused: ctx.composerController ? ctx.composerController.composeFocused : false,
    hasAttachment: ctx.composerController ? ctx.composerController.attachmentPath !== '' : false,
    replying: ctx.composerController ? ctx.composerController.replying : false,
    pane: ctx.conversationController ? ctx.conversationController.pane : ui.pane,
    activeId: ui.activeId,
    query: ctx.listController ? ctx.listController.query : '',
    unreadView: ctx.listController ? ctx.listController.unreadView : false
  };
}
