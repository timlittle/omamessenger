.pragma library

// Decides what "Send scheduled message now" / "Edit scheduled
// message" / "Cancel scheduled message" do, given the open chat's queued messages.

.import "Keymap.js" as Keymap

// resolve returns {kind: 'none'}, {kind: 'one', message} for a single
// queued message, or {kind: 'many', messages} to open a picker.
// messagesForChat is expected already filtered to the open conversation.
function resolve(messagesForChat) {
  const messages = messagesForChat || [];
  if (messages.length === 0) return { kind: 'none' };
  if (messages.length === 1) return { kind: 'one', message: messages[0] };
  return { kind: 'many', messages: messages };
}

// hints returns the key-hint text for a highlighted queued message,
// reading each key from message.open/react/delete (remapped by
// Panel.routeKey to send now/edit/cancel), so a keys.conf override shows up here too.
function hints(bindings) {
  const key = (action) => Keymap.keyFor(action, bindings);
  return `${key('message.open')} send now · ${key('message.react')} edit · ${key('message.delete')} cancel`;
}
