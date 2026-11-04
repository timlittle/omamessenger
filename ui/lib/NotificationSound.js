.pragma library

// NotificationSound.js works out which notification sound, if any,
// should play for an arriving message: a conversation's custom sound
// beats its service's manifest setting, which beats the global
// default. Whether to even ask is decided server-side (see
// backend/internal/app/ingest.go); this file never re-checks mute.

// settingState reduces a raw manifest setting value to one of three
// states: 'inherit' (unset), 'none' (explicit silence, stopping the
// cascade) or 'path' (the trimmed value to try).
function settingState(raw) {
  const value = String(raw ?? '').trim();
  if (value === '' || value.toLowerCase() === 'default') return { kind: 'inherit' };
  if (value.toLowerCase() === 'none') return { kind: 'none' };

  return { kind: 'path', path: value };
}

// resolve decides which of a conversation's custom sound, its
// service's manifest setting or the global default should play, in
// that precedence order. Returns { source: 'silent' } when nothing
// applies; otherwise { source, path }, with path omitted for
// 'conversation' since sounds.fetch looks that one up by id instead.
function resolve(hasCustomSound, serviceSound, defaultSound) {
  if (hasCustomSound) return { source: 'conversation' };

  const service = settingState(serviceSound);
  if (service.kind === 'path') return { source: 'service', path: service.path };
  if (service.kind === 'none') return { source: 'silent' };

  const fallback = settingState(defaultSound);
  if (fallback.kind === 'path') return { source: 'default', path: fallback.path };

  return { source: 'silent' };
}
