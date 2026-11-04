.pragma library

// Pin.js works out pin-reorder moves in the terms conversations.movePin
// understands: a direction and a number of adjacent swaps.

// planMove returns the direction and swap count to move fromId to
// toIndex within pinnedIds. count is 0 when there is nothing to do.
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

// reordered returns pinnedIds with fromId moved to toIndex, for a
// drag's live preview before the move is sent to the helper.
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
