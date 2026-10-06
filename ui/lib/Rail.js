.pragma library

// The service rail: an "All" entry, one entry per service with accounts,
// and one per account when a service has several. Each entry carries its
// unread total and the worst connection status it covers.

// SERVICES lists the services in rail order, with their labels.
var SERVICES = [
  { id: 'whatsapp', label: 'WhatsApp' },
  { id: 'telegram', label: 'Telegram' }
];

// STATUS_RANK orders account statuses from worst to best.
// An account waiting for sign-in is worst: only the user can fix it.
var STATUS_RANK = { 'needs-auth': -1, error: 0, connecting: 1, offline: 2, connected: 3 };

// STATUS_LABELS names statuses whose id does not read as a word.
var STATUS_LABELS = { 'needs-auth': 'Sign-in needed' };

// items builds the rail entries for the accounts and conversations.
function items(accounts, conversations) {
  if (!accounts || !conversations) {
    return [];
  }

  const result = [entry('all', 'all', 'All', accounts, conversations)];
  for (const service of SERVICES) {
    const own = accounts.filter((a) => a.service === service.id);
    if (own.length === 0) {
      continue;
    }

    const serviceEntry = entry(`service:${service.id}`, 'service', serviceLabel(service.id), own, conversations);
    result.push(Object.assign(serviceEntry, { service: service.id }));

    if (own.length > 1) {
      result.push(...accountEntries(own, conversations));
    }
  }

  return result;
}

// accountEntries returns one entry per account, sorted by name.
function accountEntries(accounts, conversations) {
  return accounts
    .slice()
    .sort((a, b) => a.name.localeCompare(b.name))
    .map((a) => Object.assign(entry(`account:${a.id}`, 'account', a.name, [a], conversations),
      { service: a.service, accountId: a.id }));
}

// entry builds one rail entry covering the given accounts.
function entry(key, kind, label, accounts, conversations) {
  const covered = filter(conversations, key);

  return { key, kind, label, unread: unreadTotal(covered), status: worstStatus(accounts) };
}

// filter returns the conversations a rail key covers, in order. An unknown
// key covers everything.
function filter(conversations, railKey) {
  const list = conversations ?? [];
  const [kind, id] = (railKey ?? '').split(':');

  if (kind === 'service') {
    return list.filter((c) => c.service === id);
  }

  if (kind === 'account') {
    return list.filter((c) => c.accountId === id);
  }

  return list;
}

// next returns the key delta entries away from key, wrapping. An unknown
// key counts as the first entry; no entries gives "all".
function next(items, key, delta) {
  if (!items || items.length === 0) {
    return 'all';
  }

  const current = Math.max(items.findIndex((i) => i.key === key), 0);
  const count = items.length;

  return items[(((current + delta) % count) + count) % count].key;
}

// serviceLabel returns a service's display name, or its id when unknown.
function serviceLabel(service) {
  const known = SERVICES.find((s) => s.id === service);

  return known ? known.label : service;
}

// accountLabel names an account together with its service, because
// account names such as "Personal" repeat across services.
function accountLabel(account) {
  return `${serviceLabel(account.service)} · ${account.name}`;
}

// unreadTotal sums the unread counts of conversations that are not muted.
function unreadTotal(conversations) {
  return conversations.reduce((sum, c) => sum + (c.muted ? 0 : c.unread ?? 0), 0);
}

// worstStatus returns the worst account status, or "connected" for none.
function worstStatus(accounts) {
  return accounts
    .map((a) => a.status)
    .reduce((worst, s) => ((STATUS_RANK[s] ?? 3) < STATUS_RANK[worst] ? s : worst), 'connected');
}

// statusLabel names an account status in words: "needs-auth" as
// "Sign-in needed", "connecting" as "Connecting".
function statusLabel(status) {
  if (!status) {
    return '';
  }

  return STATUS_LABELS[status] ?? status.charAt(0).toUpperCase() + status.slice(1);
}
