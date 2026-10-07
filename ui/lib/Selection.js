.pragma library

// Moving the selection through the conversation list. The selection is a
// conversation id, never an index, so a reordered list keeps it in place.

// move returns the id delta steps from the selected one, stopping at the
// ends. With no known selection it returns the first id; with no ids, "".
function move(ids, selectedId, delta) {
  if (!ids || ids.length === 0) {
    return '';
  }

  const current = Math.max(ids.indexOf(selectedId), 0);

  return ids[Math.max(0, Math.min(current + delta, ids.length - 1))];
}

// edge returns the first ("top") or last ("bottom") id, or "".
function edge(ids, which) {
  if (!ids || ids.length === 0) {
    return '';
  }

  return which === 'bottom' ? ids[ids.length - 1] : ids[0];
}

// afterRemoval returns the id that should take the highlight once id
// itself is gone from ids: the next one, or the previous when id was
// last. "" when id was the only one, or was not in ids at all.
function afterRemoval(ids, id) {
  const index = (ids ?? []).indexOf(id);
  if (index === -1) {
    return '';
  }

  return ids[index + 1] ?? ids[index - 1] ?? '';
}

// nextUnread returns the next conversation after the selected one that has
// unread messages and is not muted, wrapping round to the selected one
// itself. With no known selection the search starts at the top. "" when
// nothing is unread.
function nextUnread(conversations, selectedId) {
  const count = conversations ? conversations.length : 0;
  const start = count ? conversations.findIndex((c) => c.id === selectedId) : -1;

  for (let step = 1; step <= count; step++) {
    const c = conversations[(start + step + count) % count];
    if (c.unread > 0 && !c.muted) {
      return c.id;
    }
  }

  return '';
}
