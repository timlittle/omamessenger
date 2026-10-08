.pragma library

// Navigation context and escape-chain logic: determines which key bindings apply
// and what Escape does based on UI state.

// keyContext determines the highest-priority context for key bindings based on
// the current UI state. Precedence: the close question > account setup > the
// photo viewer > the health check report > palette > reaction picker > the
// delete question > the archive-all question > dialog > search > compose >
// conversation > list. The viewer and the reaction picker are never open
// together, so their relative order only matters in theory.
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
