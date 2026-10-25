.pragma library

// The plugin settings sent to the helper. Omarchy passes a bar widget only
// the settings the user has changed, so the manifest's defaults are applied
// here, and the manifest's own enum labels are translated to the values
// the helper's settings.apply expects.

// DEFAULTS mirror the defaultValue of each setting in manifest.json.
var DEFAULTS = {
  notifications: true, notificationDetail: 'none', readReceipts: true,
  spellCheck: true, spellLanguage: 'auto', reopenAfterRestart: true,
  defaultNotificationSound: '', whatsappNotificationSound: '', telegramNotificationSound: '',
};

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

// LANGUAGE_LABELS maps manifest.json's spellLanguage option labels to
// the helper's own values.
var LANGUAGE_LABELS = {
  'Automatic': 'auto',
  'British English': 'en_GB',
  'American English': 'en_US',
};

// LANGUAGE_VALUES are the helper's own spellLanguage values, accepted as
// well as the labels above so a value already in that form passes
// through unchanged.
var LANGUAGE_VALUES = ['auto', 'en_GB', 'en_US'];

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
    spellCheck: typeof given.spellCheck === 'boolean' ? given.spellCheck : DEFAULTS.spellCheck,
    spellLanguage: resolveLanguage(given),
    defaultNotificationSound: resolveSound(given.defaultNotificationSound),
    whatsappNotificationSound: resolveSound(given.whatsappNotificationSound),
    telegramNotificationSound: resolveSound(given.telegramNotificationSound),
  };
}

// resolveSound passes a manifest sound setting straight through: a
// path, or the sentinels "default" and "none" (see
// ui/lib/NotificationSound.js, which gives those two their meaning).
// Anything not a string at all falls back to "", the same blank
// default manifest.json itself gives every one of these three settings.
function resolveSound(value) {
  return typeof value === 'string' ? value : '';
}

// reopenAfterRestart resolves the "Reopen after restart" setting: true
// unless the user turned it off in Omarchy's bar editor. It is read
// straight off the raw settings object rather than through withDefaults,
// since it is this UI's own choice and is never one of the fields
// withDefaults prepares for settings.apply to send the helper.
function reopenAfterRestart(settings) {
  var given = settings ?? {};
  return typeof given.reopenAfterRestart === 'boolean' ? given.reopenAfterRestart : DEFAULTS.reopenAfterRestart;
}

// resolveLanguage picks the spellLanguage value to send: the manifest's
// label or the helper's own value when given names one, otherwise the
// manifest default.
function resolveLanguage(given) {
  var language = given.spellLanguage;
  if (LANGUAGE_LABELS[language]) return LANGUAGE_LABELS[language];
  if (LANGUAGE_VALUES.includes(language)) return language;

  return DEFAULTS.spellLanguage;
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
