.pragma library

// NotificationSound.js works out which notification sound, if any,
// should play for an arriving message: a conversation's own custom
// sound (set from the command palette) beats its service's own
// manifest setting, which beats the global default setting, which
// plays nothing at all unless the user gave it a sound of its own.
// Whether to even ask at all is decided server-side, before the
// notification.sound event this feeds ever reaches the UI: the helper
// never sends it for a muted, missed or already-read arrival, the same
// gate the desktop notification itself uses (see
// backend/internal/app/ingest.go), so this file never re-checks mute.
// Everything here is a pure decision over plain strings; turning the
// winning choice into an actual playable file happens elsewhere, over
// sounds.fetch, once this has already decided whether it is worth
// asking for one at all.

// settingState reduces a raw manifest setting value - what the user
// typed or picked into "Default notification sound", "WhatsApp
// notification sound" or "Telegram notification sound" - to one of
// three states: 'inherit' (a blank field, or the literal "default":
// Omarchy's own plugin settings panel calls the unset state that),
// 'none' (the user explicitly asked for silence at this level, which
// must stop the cascade rather than falling through to a lower one),
// or 'path' (still worth trying, with the trimmed value to try).
function settingState(raw) {
  const value = String(raw ?? '').trim();
  if (value === '' || value.toLowerCase() === 'default') return { kind: 'inherit' };
  if (value.toLowerCase() === 'none') return { kind: 'none' };

  return { kind: 'path', path: value };
}

// resolve decides which of a conversation's own custom sound, its
// service's manifest setting (serviceSound) or the global default
// setting (defaultSound) should play, in that order of precedence.
// hasCustomSound is the conversation's own notificationSound field
// already reduced to a boolean ("custom" vs anything else). Returns
// { source: 'silent' } when nothing applies, whether because every
// level inherited all the way down with nothing left to inherit, or
// because a level explicitly asked for silence; otherwise
// { source, path } naming which one won and, for 'service' or
// 'default', the raw manifest path sounds.fetch still has to validate.
// A 'conversation' result carries no path at all, since sounds.fetch
// looks that one up by conversation id instead (see
// ui/controllers/ListController.qml's _setNotificationSound).
function resolve(hasCustomSound, serviceSound, defaultSound) {
  if (hasCustomSound) return { source: 'conversation' };

  const service = settingState(serviceSound);
  if (service.kind === 'path') return { source: 'service', path: service.path };
  if (service.kind === 'none') return { source: 'silent' };

  const fallback = settingState(defaultSound);
  if (fallback.kind === 'path') return { source: 'default', path: fallback.path };

  return { source: 'silent' };
}
