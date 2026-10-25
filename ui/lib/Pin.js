.pragma library

// Pin.js works out how to reorder a pinned conversation, in the terms
// the helper's own conversations.movePin understands: a direction, "up"
// or "down", and how many single adjacent swaps to ask for. Moving a
// row with the keyboard always asks for exactly one swap; dragging a
// row to a new position (see ListController.movePinTo) asks for
// whatever a drop at a different index needs, which is exactly the
// number of single-step swaps that would land it there, since moving an
// item through a sequence of adjacent swaps produces the same result as
// splicing it straight to that index.

// planMove works out the conversations.movePin calls needed to move
// fromId, found in pinnedIds (the pinned conversations in their current
// order), to toIndex. count is 0 when fromId is already at toIndex, or
// either index does not name a real position in pinnedIds: there is
// nothing to ask the helper for.
function planMove(pinnedIds, fromId, toIndex) {
  const fromIndex = pinnedIds.indexOf(fromId);
  if (fromIndex < 0 || toIndex < 0 || toIndex >= pinnedIds.length) {
    return { direction: 'down', count: 0 };
  }

  return {
    direction: toIndex > fromIndex ? 'down' : 'up',
    count: Math.abs(toIndex - fromIndex)
  };
}

// reordered returns pinnedIds with fromId moved to toIndex, the order a
// drag's own live preview should show while the pointer is still
// moving, before any conversations.movePin request has even been sent.
// Returns pinnedIds unchanged when fromId or toIndex is not a real
// position in it, the same cases planMove treats as nothing to do.
function reordered(pinnedIds, fromId, toIndex) {
  const fromIndex = pinnedIds.indexOf(fromId);
  if (fromIndex < 0 || toIndex < 0 || toIndex >= pinnedIds.length) {
    return pinnedIds;
  }

  const copy = pinnedIds.slice();
  copy.splice(fromIndex, 1);
  copy.splice(toIndex, 0, fromId);

  return copy;
}
