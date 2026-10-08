.pragma library

// @-mention support for the composer: recognising an "@query" being
// typed, filtering a group's members by it, inserting the chosen
// member's name as a mention token, and resolving the final set of
// mentions to send once the message is composed. A JavaScript string's
// own indices are already UTF-16 code units, the same unit the helper's
// protocol uses for a mention's offset and length (see domain.Mention
// on the Go side), so none of this needs any unit conversion.

// activeQuery finds the "@query" the caret sits inside of, if any:
// an "@" with no whitespace between it and cursor, and either at the
// very start of text or preceded by whitespace, so "user@host" never
// triggers the picker. Returns {start, query} (query excludes the "@"),
// or null when the caret is not inside one.
function activeQuery(text, cursor) {
  const before = text.slice(0, cursor);
  const at = before.lastIndexOf('@');
  if (at < 0) {
    return null;
  }

  const between = before.slice(at + 1);
  if (/\s/.test(between)) {
    return null;
  }

  if (at > 0 && !/\s/.test(text[at - 1])) {
    return null;
  }

  return { start: at, query: between };
}

// filterMembers lists members whose name contains query, case
// insensitively, with a name that starts with query sorted first, so
// typing the start of someone's name finds them fastest.
function filterMembers(members, query) {
  const needle = query.toLowerCase();
  return (members || [])
    .filter((m) => m.name.toLowerCase().includes(needle))
    .sort((a, b) => rank(a.name, needle) - rank(b.name, needle));
}

// rank orders a startsWith match before any other kind of match.
function rank(name, needle) {
  return name.toLowerCase().startsWith(needle) ? 0 : 1;
}

// insertMention replaces the "@query" span from start to cursor with
// "@<member.name> ", so typing can continue straight after it, and
// returns the new text and caret position.
function insertMention(text, start, cursor, member) {
  const token = `@${member.name} `;
  return { text: text.slice(0, start) + token + text.slice(cursor), cursor: start + token.length };
}

// resolveMentions finds each inserted member's "@name" token in the
// final composed text, in the order the composer inserted them,
// searching only after the previous one's own match so the same name
// mentioned twice resolves to two different positions. A member whose
// token the user has since deleted or edited away is left out, rather
// than reported at the wrong position.
function resolveMentions(text, inserted) {
  const out = [];
  let searchFrom = 0;

  for (const member of inserted || []) {
    const token = `@${member.name}`;
    const at = text.indexOf(token, searchFrom);
    if (at < 0) {
      continue;
    }

    out.push({ userId: member.id, name: member.name, offset: at, length: token.length });
    searchFrom = at + token.length;
  }

  return out;
}
