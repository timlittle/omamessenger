.pragma library

// The plugin settings sent to the helper. Omarchy passes a bar widget only
// the settings the user has changed, so the manifest's defaults are applied
// here.

// DEFAULTS mirror the defaultValue of each setting in manifest.json.
var DEFAULTS = { notifications: true, notificationPreview: true, demoChatter: true };

// withDefaults returns every known setting, taking the user's value where
// there is one and the default otherwise. Unknown keys are dropped.
function withDefaults(settings) {
  const given = settings ?? {};
  const result = {};
  for (const key of Object.keys(DEFAULTS)) {
    result[key] = typeof given[key] === 'boolean' ? given[key] : DEFAULTS[key];
  }

  return result;
}
