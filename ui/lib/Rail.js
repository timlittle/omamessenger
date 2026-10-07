.pragma library

// The service rail: an "All" entry, one entry per service with accounts,
// and one per account when a service has several. Each entry carries its
// unread total and the worst connection status it covers.

// SERVICES lists the services in rail order, with their labels.
var SERVICES = [
  { id: 'whatsapp', label: 'WhatsApp' },
  { id: 'telegram', label: 'Telegram' }
];

// RECENT_MS is how recent a chat's last message must be to show while older
// chats are hidden: a month.
var RECENT_MS = 30 * 24 * 60 * 60 * 1000;

// STATUS_RANK orders account statuses from worst to best.
// An account waiting for sign-in is worst: only the user can fix it.
var STATUS_RANK = { 'needs-auth': -1, error: 0, connecting: 1, offline: 2, connected: 3 };

// STATUS_LABELS names statuses whose id does not read as a word.
var STATUS_LABELS = { 'needs-auth': 'Sign-in needed' };

// items builds the rail entries for the accounts and conversations.
// knownServices, when given, is the helper's own list of services (from
// hello), whose names take over from the labels below.
function items(accounts, conversations, knownServices) {
  if (!accounts || !conversations) {
    return [];
  }

  const result = [entry('all', 'all', 'All', accounts, conversations)];
  for (const service of SERVICES) {
    const own = accounts.filter((a) => a.service === service.id);
    if (own.length === 0) {
      continue;
    }

    const serviceEntry = entry(`service:${service.id}`, 'service', serviceLabel(service.id, knownServices), own, conversations);
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

// serviceLabel returns a service's display name: from knownServices (the
// helper's own hello list) when it names the service, else the built-in
// label, else the service id itself.
function serviceLabel(service, knownServices) {
  const known = (knownServices ?? []).find((s) => s.id === service);
  if (known) {
    return known.name;
  }

  const fallback = SERVICES.find((s) => s.id === service);

  return fallback ? fallback.label : service;
}

// accountLabel names an account together with its service, because
// account names such as "Personal" repeat across services.
function accountLabel(account, knownServices) {
  return `${serviceLabel(account.service, knownServices)} · ${account.name}`;
}

// accountDescription names an account by who is signed in to it when the
// helper says so, since its name is often just the service's.
function accountDescription(account, knownServices) {
  return `${serviceLabel(account.service, knownServices)} · ${account.detail || account.name}`;
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

// accountNames maps each account id to its name, for rows that show it.
function accountNames(accounts) {
  const names = {};
  for (const a of accounts ?? []) {
    names[a.id] = a.name;
  }

  return names;
}

// multiAccountServices lists the services the rail split into per-account
// entries: exactly the services with more than one account.
function multiAccountServices(items) {
  return [...new Set(items.filter((i) => i.kind === 'account').map((i) => i.service))];
}

// showsStandard reports whether one conversation would appear in the
// standard list: not archived, not hidden, and active within the last
// month. Unread messages do not keep an old chat visible on their own;
// the rail's own unread total still counts them (see unreadTotal), and
// show-all still shows the chat, dimmed. The one open as keepId is exempt
// only from the recency rule, so replying in an old chat never makes it
// vanish from under the user; an explicit hide or archive still drops it
// at once, even while it is open, since that is the point of hiding or
// archiving it.
function showsStandard(c, nowMs, keepId) {
  if (c.archived || c.hidden) return false;
  if (c.id === keepId) return true;

  return nowMs - c.lastActivity <= RECENT_MS;
}

// standard keeps the conversations the standard list shows; see
// showsStandard.
function standard(conversations, nowMs, keepId) {
  return conversations.filter((c) => showsStandard(c, nowMs, keepId));
}

// isDimmed reports whether a conversation should be drawn dimmed in the
// show-all view: anything the standard list would not show on its own.
function isDimmed(c, nowMs, keepId) {
  return !showsStandard(c, nowMs, keepId);
}

// dimLabel names why a dimmed conversation would not appear in the
// standard list: "Hidden" or "Archived". A conversation dimmed only for
// being older carries no label, since its timestamp already explains it;
// one that is not dimmed at all carries none either.
function dimLabel(c, nowMs, keepId) {
  if (!isDimmed(c, nowMs, keepId)) return '';
  if (c.hidden) return 'Hidden';
  if (c.archived) return 'Archived';

  return '';
}

// compareConversations orders conversations the way the list shows them:
// pinned ones first, then newest activity first. Pass to Array.sort or
// ListSync.upsertById.
function compareConversations(a, b) {
  if (a.pinned !== b.pinned) {
    return a.pinned ? -1 : 1;
  }

  return b.lastActivity - a.lastActivity;
}
