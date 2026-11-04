.pragma library

// Undo.js describes the toast and the service call that undoes an
// action a key or a click just took on a conversation: archiving,
// muting, pinning or hiding it, or blocking its contact. ListController
// calls these to build what the notice says and what "u to undo" sends
// back, rather than each action working that out for itself.

// describeToggle describes undoing a boolean field just set to
// newValue, through method, on conversation id: the opposite call
// restores it.
function describeToggle(method, field, id, newValue, labelOn, labelOff, title) {
  const label = newValue ? labelOn : labelOff;
  const params = { conversationId: id };
  params[field] = !newValue;

  return { notice: noticeText(label, title), method: method, params: params };
}

// describeBlock describes undoing a block just made with
// contacts.block: contacts.unblock is the opposite call. There is no
// undo offered for unblocking itself, or for a spam report, which also
// hides the chat; see ListController._unblockCurrent and confirmBlock.
function describeBlock(id, title) {
  return { notice: noticeText('Blocked', title), method: 'contacts.unblock', params: { conversationId: id } };
}

// noticeText pairs label with title the toast shows, and the "u to
// undo" hint every description above ends with: "Archived Nadia · u to
// undo", or just "Archived · u to undo" for a title that is empty.
function noticeText(label, title) {
  const named = title ? `${label} ${title}` : label;
  return `${named} · u to undo`;
}
