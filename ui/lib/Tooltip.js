.pragma library

// Formats the bar icon's hover tooltip: the plugin name and total unread
// on the first line, one line per service with unread messages, and one
// line per account that is not connected, so a glance at the bar says
// whether a service needs attention.

// SERVICE_NAMES is this UI's fallback label for a service id, used only
// when the helper's own services list does not already name it.
var SERVICE_NAMES = { whatsapp: 'WhatsApp', telegram: 'Telegram' };

// STATUS_LABELS describes a disconnected account's status in a few
// words, for the tooltip's own status lines.
var STATUS_LABELS = {
  connecting: 'connecting',
  'needs-auth': 'needs sign-in',
  error: 'error',
  offline: 'offline'
};

// serviceName returns a service id's display name: the helper's own
// services list when it names one, else this file's fallback, else the
// id itself.
function serviceName(id, services) {
  const known = (services || []).find((s) => s.id === id);
  if (known && known.name) return known.name;
  return SERVICE_NAMES[id] || id;
}

// format builds the bar icon's tooltip text from the service's current
// state: `total` the combined unread count, `byService` unread per
// service id, `services` the helper's known services (for display
// names) and `accounts` the signed-in accounts (for connection state).
function format(state) {
  const total = state.total || 0;
  const byService = state.byService || {};
  const services = state.services || [];
  const accounts = state.accounts || [];

  const lines = ['OmaMessenger · ' + total + ' unread'];

  for (const id of Object.keys(byService).sort()) {
    lines.push(serviceName(id, services) + ': ' + byService[id] + ' unread');
  }

  for (const account of accounts) {
    if (account.status === 'connected') continue;
    const label = STATUS_LABELS[account.status] || account.status;
    lines.push(serviceName(account.service, services) + ' (' + account.name + '): ' + label);
  }

  return lines.join('\n');
}
