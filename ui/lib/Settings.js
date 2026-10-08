.pragma library

// The plugin settings sent to the helper. Omarchy passes a bar widget only
// the settings the user has changed, so the manifest's defaults are applied
// here, and the manifest's own enum labels are translated to the values
// the helper's settings.apply expects.

// DEFAULTS mirror the defaultValue of each setting in manifest.json.
var DEFAULTS = { notifications: true, notificationDetail: 'nameAndMessage' };

// DETAIL_LABELS maps manifest.json's notificationDetail option labels,
// which Omarchy shows the user as is, to the helper's own values.
var DETAIL_LABELS = {
  'Name and message': 'nameAndMessage',
  'Name only': 'nameOnly',
  'Nothing': 'none',
};

// DETAIL_VALUES are the helper's own notificationDetail values, accepted
// as well as the labels above so a value already in that form passes
// through unchanged.
var DETAIL_VALUES = ['nameAndMessage', 'nameOnly', 'none'];

// withDefaults returns every known setting, taking the user's value where
// there is one and the default otherwise. Unknown keys are dropped.
function withDefaults(settings) {
  var given = settings ?? {};

  return {
    notifications: typeof given.notifications === 'boolean' ? given.notifications : DEFAULTS.notifications,
    notificationDetail: resolveDetail(given),
  };
}

// resolveDetail picks the notificationDetail value to send: the manifest's
// label or the helper's own value when given names one, otherwise the
// older notificationPreview boolean translated to the matching level
// (true meaning name and message), otherwise the manifest default.
function resolveDetail(given) {
  var detail = given.notificationDetail;
  if (DETAIL_LABELS[detail]) return DETAIL_LABELS[detail];
  if (DETAIL_VALUES.includes(detail)) return detail;
  if (typeof given.notificationPreview === 'boolean') {
    return given.notificationPreview ? 'nameAndMessage' : 'nameOnly';
  }

  return DEFAULTS.notificationDetail;
}
