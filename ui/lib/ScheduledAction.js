.pragma library

// Decides what "Send scheduled message now" / "Edit scheduled message"
// / "Cancel scheduled message" do, from the open chat's own queued
// messages: act on it directly with exactly one, let the command
// palette's own picker choose which with more than one, or say there
// is nothing to act on with none.

// resolve returns {kind: 'none'} with nothing queued for the open
// chat, {kind: 'one', message} to act on messagesForChat's only row
// directly, or {kind: 'many', messages: messagesForChat} to open a
// picker over them. messagesForChat is expected already filtered to
// the open conversation and sorted soonest due first, the same as
// ScheduledController.forConversation already returns.
function resolve(messagesForChat) {
  const messages = messagesForChat || [];
  if (messages.length === 0) return { kind: 'none' };
  if (messages.length === 1) return { kind: 'one', message: messages[0] };
  return { kind: 'many', messages: messages };
}
