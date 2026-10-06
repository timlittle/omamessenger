.pragma library

// Navigation context and escape-chain logic: determines which key bindings apply
// and what Escape does based on UI state.

// keyContext determines the highest-priority context for key bindings based on
// the current UI state. Precedence: help > dialog > search > compose > conversation > list.
function keyContext(state) {
  if (state.helpOpen) return 'help';
  if (state.dialogOpen) return 'dialog';
  if (state.searchFocused) return 'search';
  if (state.composeFocused) return 'compose';
  if (state.pane === 'conversation') return 'conversation';
  return 'list';
}

// escapeAction returns what Escape does: it undoes the innermost thing
// first (help, dialog, search, composer, open conversation, query) and
// hides the window only when there is nothing left to undo.
function escapeAction(state) {
  if (state.helpOpen) return 'close-help';
  if (state.dialogOpen) return 'close-dialog';

  if (state.searchFocused) {
    return state.query ? 'clear-search' : 'leave-search';
  }

  if (state.composeFocused) return 'leave-compose';

  if (state.activeId) {
    return 'close-conversation';
  }

  if (state.query) {
    return 'clear-search';
  }

  return 'hide-window';
}
