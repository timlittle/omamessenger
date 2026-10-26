.pragma library

// Decides what "Send scheduled message now" / "Edit scheduled message"
// / "Cancel scheduled message" do, from the open chat's own queued
// messages: act on it directly with exactly one, let the command
// palette's own picker choose which with more than one, or say there
// is nothing to act on with none.

.import "Keymap.js" as Keymap

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

// hints returns the key-hint text for the open chat's own highlighted
// queued message (see ConversationController.highlightedScheduledId
// and Panel.routeKey's own remap), reading each key's effective
// binding from message.open/message.react/message.delete - the same
// three Panel.routeKey remaps to send now, edit and cancel once a
// queued row, rather than a message, is highlighted - so a keys.conf
// override shows up here too, the same way Highlight.hints already
// does for a highlighted message's own row.
function hints(bindings) {
  const key = (action) => Keymap.keyFor(action, bindings);
  return `${key('message.open')} send now · ${key('message.react')} edit · ${key('message.delete')} cancel`;
}
