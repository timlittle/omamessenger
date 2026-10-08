.pragma library

// The plugin settings sent to the helper. Omarchy passes a bar widget only
// the settings the user has changed, so the manifest's defaults are applied
// here, and the manifest's own enum labels are translated to the values
// the helper's settings.apply expects.

// DEFAULTS mirror the defaultValue of each setting in manifest.json.
var DEFAULTS = { notifications: true, notificationDetail: 'nameAndMessage', readReceipts: true };

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
// currentReadReceipts, when given, replaces the manifest default as the
// fallback for readReceipts: Service.qml passes its own current value so
// that re-forwarding the bar widget's settings (which normally carries
// no readReceipts key at all, since Omarchy only sends what the user
// changed there) never undoes the palette's own "Toggle read receipts"
// command.
function withDefaults(settings, currentReadReceipts) {
  var given = settings ?? {};
  var readReceiptsFallback = typeof currentReadReceipts === 'boolean' ? currentReadReceipts : DEFAULTS.readReceipts;

  return {
    notifications: typeof given.notifications === 'boolean' ? given.notifications : DEFAULTS.notifications,
    notificationDetail: resolveDetail(given),
    readReceipts: typeof given.readReceipts === 'boolean' ? given.readReceipts : readReceiptsFallback,
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
