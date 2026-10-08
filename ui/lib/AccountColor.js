.pragma library
.import "SenderColor.js" as SenderColor

// AccountColor: a stable colour index for one account, so its chip in the
// conversation list and the rail keeps the same colour across restarts
// and re-renders. An account id is hashed the same way SenderColor hashes
// a sender id, since both need the same stability-and-spread guarantee;
// the two stay separate functions so the account and sender palettes can
// be sized and tested independently (see Theme.accountColor, which keeps
// the two palettes visually distinct even when both show at once).

// colorIndex returns a stable index in [0, paletteSize) for an account id.
// The same id always yields the same index; an empty or missing id (no
// account yet) safely returns 0 instead of throwing.
function colorIndex(accountId, paletteSize) {
  return SenderColor.colorIndex(accountId, paletteSize);
}
