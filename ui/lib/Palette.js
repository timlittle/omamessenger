.pragma library
.import "Keymap.js" as Keymap
.import "KeyBindings.js" as KeyBindings
.import "Rail.js" as Rail
.import "Snooze.js" as Snooze

// Fuzzy matching for the command palette, in the style of Telescope or
// Alfred: the letters typed must appear in order, and matches at word
// starts or in unbroken runs rank first.
//
// modeFor's MODES table also says what each of the palette's modes
// ("commands", "conversations", "links", "snoozeCustom", "keyBindings")
// matches against and how it turns a match into a row to show, so
// WindowController.qml never branches on the mode string itself; adding
// a mode is one entry in that table.

// search returns the items whose text matches query, best first. textOf
// picks the text to match from an item. An empty query keeps every item
// in its original order.
function search(items, query, textOf) {
  const wanted = (query ?? '').trim().toLowerCase();
  if (!wanted) {
    return items.slice();
  }

  return items
    .map((item, index) => ({ item, index, score: score(textOf(item).toLowerCase(), wanted) }))
    .filter((entry) => entry.score > 0)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .map((entry) => entry.item);
}

// conversationOrder sorts conversations the way Ctrl+K lists them before a
// query narrows them: every unread one first, most recently active within
// that group, then the rest by recency. search()'s own tie-break keeps the
// original index order for equally-scored matches, so sorting into this
// order first keeps unread chats grouped first among the matches too, not
// just in the unfiltered list.
function conversationOrder(conversations) {
  return (conversations ?? []).slice().sort((a, b) => {
    const unreadA = (a.unread ?? 0) > 0;
    const unreadB = (b.unread ?? 0) > 0;
    if (unreadA !== unreadB) return unreadA ? -1 : 1;

    return (b.lastActivity ?? 0) - (a.lastActivity ?? 0);
  });
}

// messageRows turns conversations.list's search results into the unified
// command palette's "Messages" section: one row per conversation whose
// newest message matching the query is known (matchMessageId), naming
// who sent it, which conversation it is in and a snippet to show, kept
// in the order the search already ranked them. A row with no
// matchMessageId only matched by its title, so it belongs in the
// "Conversations" section instead, not here.
function messageRows(conversations) {
  return (conversations ?? [])
    .filter((c) => c.matchMessageId)
    .map((c) => ({
      conversationId: c.id,
      messageId: c.matchMessageId,
      sender: c.matchSender || '',
      conversationTitle: c.title,
      service: c.service,
      snippet: c.match || ''
    }));
}

// score rates how well text matches wanted, or returns 0 when the letters
// do not all appear in order. Each matched letter scores 1, plus 3 at the
// start of a word and 2 when it follows the previous match directly. Text
// containing the whole query scores extra, most of all at a word start.
function score(text, wanted) {
  const whole = text.indexOf(wanted);
  let total = whole < 0 ? 0 : 10 + (whole === 0 || text[whole - 1] === ' ' ? 10 : 0);
  let from = 0;
  let previous = -2;

  for (const ch of wanted) {
    const at = text.indexOf(ch, from);
    if (at < 0) {
      return 0;
    }

    total += 1 + (at === 0 || text[at - 1] === ' ' ? 3 : 0) + (at === previous + 1 ? 2 : 0);
    previous = at;
    from = at + 1;
  }

  return total;
}

// MODES says, for every palette mode but the default "commands" one,
// how to compute its matches (results) and how to turn those matches
// into rows ({label, detail, keys, unread, section}) for PaletteRow
// (items). ctx carries whatever a mode needs: query always, and
// otherwise whichever of listController, messageResults, linkChoices,
// bindings, keyBindingConflicts, keyBindingErrors, service and now
// WindowController.qml's _paletteContext fills in.
var MODES = {
  conversations: {
    results: (ctx) => search(ctx.listController ? conversationOrder(ctx.listController.all) : [], ctx.query, (c) => c.title),
    items: (results, ctx) => results.map((c) => ({
      label: c.title, detail: Rail.serviceLabel(c.service, ctx.service ? ctx.service.services : []),
      keys: '', unread: c.unread ?? 0, section: 'conversation'
    })).concat(ctx.messageResults.map((m) => ({
      label: (m.sender ? m.sender + ': ' : '') + m.snippet, detail: m.conversationTitle,
      keys: '', unread: 0, section: 'message'
    })))
  },
  links: {
    results: (ctx) => search(ctx.linkChoices.map((url) => ({ url })), ctx.query, (c) => c.url),
    items: (results) => results.map((c) => ({ label: 'Open link: ' + c.url, detail: '', keys: '' }))
  },
  snoozeCustom: {
    results: (ctx) => Snooze.preview(ctx.query, ctx.now),
    items: (results) => results.map((r) => ({ label: r.label, detail: '', keys: '' }))
  },
  keyBindings: {
    results: (ctx) => search(KeyBindings.rows(ctx.bindings, ctx.keyBindingConflicts, ctx.keyBindingErrors), ctx.query, (r) => r.label),
    items: (results) => results
  }
};

// DEFAULT_MODE is "commands": every command binding, best match first,
// shown as-is.
var DEFAULT_MODE = {
  results: (ctx) => search(Keymap.commands(ctx.bindings), ctx.query, (c) => c.label),
  items: (results) => results
};

// modeFor returns mode's MODES entry, or DEFAULT_MODE for "commands" or
// any mode without one, so results() and items() below never need to
// branch on the mode string themselves.
function modeFor(mode) {
  return MODES[mode] || DEFAULT_MODE;
}

// results returns paletteResults for mode: the matched commands,
// conversations, links, key-binding rows, or the one snoozeCustom
// preview row, best match first.
function results(mode, ctx) {
  return modeFor(mode).results(ctx);
}

// items turns results into the rows PaletteRow shows for mode, folding
// in anything mode-specific; "conversations" mode appends its separate
// "Messages" section from ctx.messageResults.
function items(mode, matches, ctx) {
  return modeFor(mode).items(matches, ctx);
}
