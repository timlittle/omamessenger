.pragma library

// Navigation context and escape-chain logic: determines which key bindings apply
// and what Escape does based on UI state.

// keyContext determines the highest-priority context for key bindings based on
// the current UI state. Precedence: the close question > account setup > the
// photo viewer > palette > dialog > search > compose > conversation > list.
function keyContext(state) {
  if (state.confirmOpen) return 'confirm';
  if (state.setupOpen) return 'setup';
  if (state.viewerOpen) return 'viewer';
  if (state.paletteOpen) return 'palette';
  if (state.dialogOpen) return 'dialog';
  if (state.searchFocused) return 'search';
  if (state.composeFocused) return 'compose';
  if (state.pane === 'conversation') return 'conversation';
  return 'list';
}

// escapeAction returns what Escape does: it undoes the innermost thing
// first (account setup, the photo viewer, palette, dialog, search, composer,
// open conversation, query) and hides the window only when there is nothing
// left to undo.
function escapeAction(state) {
  if (state.confirmOpen) return 'cancel-close';
  if (state.setupOpen) return 'close-setup';
  if (state.viewerOpen) return 'close-viewer';
  if (state.paletteOpen) return 'close-palette';
  if (state.dialogOpen) return 'close-dialog';

  if (state.searchFocused) {
    return state.query ? 'clear-search' : 'leave-search';
  }

  if (state.composeFocused) {
    return state.hasAttachment ? 'clear-attachment' : 'leave-compose';
  }

  if (state.activeId) {
    return 'close-conversation';
  }

  if (state.query) {
    return 'clear-search';
  }

  return 'hide-window';
}
