.pragma library
.import "Markdown.js" as Markdown

// Text formatting for the conversation list and message view: HTML escaping
// and links, pipe tables (via Markdown.js), search highlights, initials,
// time labels, status glyphs and the automatic-retry countdown.

var DAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
var MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July',
  'August', 'September', 'October', 'November', 'December'];

// STATUS_GLYPHS are the delivery marks shown after an outgoing message.
var STATUS_GLYPHS = { pending: '○', sent: '✓', delivered: '✓✓', read: '✓✓', failed: '!' };

// MAX_RICH_TEXT_INPUT_LENGTH bounds how much of a message's own text
// messageHtml ever turns into rich text, in UTF-16 code units (the
// same unit a JavaScript string already indexes by). A generous match
// for the helper's own incoming cap (domain.MaxIncomingTextLength, 64
// KiB), kept here independently since this file has no way to import
// a Go constant - the same two-independently-maintained-but-matching
// pair Links.js's own allow-list and domain.AllowedLinkURL already
// are. Cutting the plain input first, before any escaping, linkifying
// or table rendering runs over it, means the HTML this file builds is
// always bounded by a predictable multiple of this cap: never a raw
// cut through the middle of a tag, which slicing the finished HTML
// itself could otherwise produce.
var MAX_RICH_TEXT_INPUT_LENGTH = 65536;

// MAX_FORMATTING_SPANS and MAX_MENTIONS bound how many formatting spans
// and mentions messageHtml ever turns into rich text, as defense in
// depth behind the helper's own domain.MaxFormattingSpans and
// domain.MaxMentions caps, applied once at ingest: a connector bug or
// an older helper could still hand this file a longer list than that,
// and styledHtml's own per-boundary work (see its own comment) must
// stay bounded either way. Kept here independently, the same two-
// independently-maintained-but-matching pair MAX_RICH_TEXT_INPUT_LENGTH's
// own comment describes, since this file has no way to import a Go
// constant.
var MAX_FORMATTING_SPANS = 256;
var MAX_MENTIONS = 64;

// URL_PATTERN matches http(s) links in escaped text. Entities such as &amp;
// may appear inside a link; any other '&' ends it.
var URL_PATTERN = /https?:\/\/[^\s<"&]*(?:&(?:amp|lt|quot|#\d+|#x[0-9a-fA-F]+);[^\s<"&]*)*/g;

// escapeHtml makes text safe to show as rich text.
function escapeHtml(text) {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// linkify turns http(s) URLs in escaped text into links, in color when
// given. Punctuation that ends a sentence stays outside the link.
function linkify(escaped, color) {
  const style = color ? ` style="color:${color}"` : '';
  return escaped.replace(URL_PATTERN, (match) => {
    const [, url, trailing] = match.match(/^(.*?)([.,;:!?)]*)$/);
    return `<a href="${url}"${style}>${url}</a>${trailing}`;
  });
}

// messageHtml shows a message's text as rich text, which would otherwise
// run every line together: it escapes it, links URLs, and keeps line
// breaks and runs of spaces, so lists and paragraphs keep their shape. A
// GitHub-style pipe table in the text renders as a real HTML table
// instead, with text before and after it handled as usual. Links take
// linkColor, and a table's cell borders take tableBorderColor, since a
// TextEdit has no colors of its own for either. mentions, when given,
// are the message's @-mention tokens ({offset, length} in the same
// UTF-16 units a JavaScript string already indexes by, so no
// conversion is needed here); each one renders in bold. spans, when
// given, are a connector's own formatting spans ({kind, start, end,
// url}, see styledRanges) - links, bold, italic, underline, strike,
// code, pre and spoiler - applied the same way on top of the escaped
// text. A message with mentions or spans skips table rendering, since
// neither is ordinary enough to coexist with a pipe table. text is cut
// to MAX_RICH_TEXT_INPUT_LENGTH, and mentions and spans to MAX_MENTIONS
// and MAX_FORMATTING_SPANS, all before any of this runs, so the rich
// text this builds is always bounded too, whatever the message's own
// length or entity count (see each cap's own comment).
function messageHtml(text, linkColor, tableBorderColor, mentions, spans) {
  const capped = text.length > MAX_RICH_TEXT_INPUT_LENGTH ? text.slice(0, MAX_RICH_TEXT_INPUT_LENGTH) : text;
  const cappedMentions = mentions && mentions.length > MAX_MENTIONS ? mentions.slice(0, MAX_MENTIONS) : mentions;
  const cappedSpans = spans && spans.length > MAX_FORMATTING_SPANS ? spans.slice(0, MAX_FORMATTING_SPANS) : spans;

  if (cappedSpans && cappedSpans.length > 0) {
    return styledHtml(capped, linkColor, cappedMentions, cappedSpans);
  }

  if (cappedMentions && cappedMentions.length > 0) {
    return mentionHtml(capped, linkColor, cappedMentions);
  }

  const borderColor = tableBorderColor ?? '#888888';
  return Markdown.splitTables(capped)
    .map((part) => (part.kind === 'table'
      ? Markdown.tableHtml(part.table, escapeHtml, (cell) => linkify(cell, linkColor), borderColor)
      : lineHtml(part.text, linkColor)))
    .join('');
}

// LINK_SCHEMES are the URL schemes a formatting span's link may open;
// anything else a service's own link-shaped entity could carry, such as
// a custom app scheme, is dropped rather than rendered as a clickable
// link.
var LINK_SCHEMES = ['http:', 'https:', 'mailto:', 'tg:'];

// spanURL returns url unchanged when its scheme is one LINK_SCHEMES
// allows, or '' to drop anything else, including text with no scheme at
// all.
function spanURL(url) {
  const match = /^[a-zA-Z][a-zA-Z0-9+.-]*:/.exec(url || '');
  return match && LINK_SCHEMES.includes(match[0].toLowerCase()) ? url : '';
}

// STYLE_TAGS pairs a span kind with the tag styledSegment wraps it in,
// applied in this order so two active styles always nest the same way
// regardless of the order spans were given in. "code" and "pre" are
// handled separately, sharing one monospace look; "spoiler" and any
// kind this list does not name add no tag at all, so an unrecognised
// span still shows its text rather than breaking the message.
var STYLE_TAGS = [
  ['bold', 'b'],
  ['italic', 'i'],
  ['underline', 'u'],
  ['strike', 's']
];

// plainHtml escapes text and keeps its line breaks and runs of spaces,
// the same as lineHtml but without linkifying bare URLs: used inside a
// span that is already its own link, where linkify would otherwise nest
// a second, redundant link inside the first.
function plainHtml(text) {
  return escapeHtml(text).replace(/ (?= )/g, '&nbsp;').replace(/\r?\n/g, '<br>');
}

// styledRanges turns mentions and formatting spans into one list of
// {start, end, kind, url} ranges messageHtml can render: a mention
// becomes a 'bold' range, and a link span whose URL's scheme spanURL
// does not allow is dropped rather than shown as a clickable link.
// Anything that no longer fits text, such as a mention or span left
// over after an edit shortened it, is dropped too.
function styledRanges(text, mentions, spans) {
  const ranges = [];

  for (const m of mentions || []) {
    if (m.length > 0 && m.offset >= 0 && m.offset + m.length <= text.length) {
      ranges.push({ start: m.offset, end: m.offset + m.length, kind: 'bold' });
    }
  }

  for (const s of spans || []) {
    if (s.start < 0 || s.end > text.length || s.start >= s.end) {
      continue;
    }

    if (s.kind === 'link') {
      const url = spanURL(s.url);
      if (url) {
        ranges.push({ start: s.start, end: s.end, kind: 'link', url });
      }
      continue;
    }

    ranges.push({ start: s.start, end: s.end, kind: s.kind });
  }

  return ranges;
}

// styledHtml renders text with mentions bolded and formatting spans
// applied on top of it. It cuts text at every range's start and end, so
// a stretch covered by more than one range - a bold word inside a link,
// say - renders with every style active over it, rather than one tag
// closing early and leaving the markup broken. Ranges are capped by
// messageHtml's own caller before this ever runs, but it still sweeps
// the sorted boundaries once, adding and dropping ranges from a running
// active list as it goes, rather than re-filtering every range for
// every one of the boundaries in between: a message built from many
// overlapping spans does only as much work as the overlap actually
// needs, not the square of its span count.
function styledHtml(text, linkColor, mentions, spans) {
  const ranges = styledRanges(text, mentions, spans);
  if (ranges.length === 0) {
    return lineHtml(text, linkColor);
  }

  const bounds = boundaryPoints(text, ranges);
  const startingAt = rangesByBound(ranges, (r) => r.start);
  const endingAt = rangesByBound(ranges, (r) => r.end);

  let html = '';
  let active = [];
  for (let i = 0; i < bounds.length - 1; i++) {
    const start = bounds[i];
    const end = bounds[i + 1];

    active = active.concat(startingAt.get(start) || []);
    html += styledSegment(text.slice(start, end), linkColor, active);

    const leaving = endingAt.get(end);
    if (leaving) {
      active = active.filter((r) => !leaving.includes(r));
    }
  }

  return html;
}

// boundaryPoints returns every distinct position ranges cuts text at -
// every range's own start and end, plus text's own start and end -
// sorted ascending once, so styledHtml's sweep never has to re-sort or
// re-scan every range per segment.
function boundaryPoints(text, ranges) {
  const points = new Set([0, text.length]);
  for (const r of ranges) {
    points.add(r.start);
    points.add(r.end);
  }

  return [...points].sort((a, b) => a - b);
}

// rangesByBound buckets ranges by boundAt(range), preserving each
// bucket's own relative order, as a Map from that position to the
// ranges starting or ending there: styledHtml's sweep looks up what to
// add to or drop from its active list at each boundary in one map read
// rather than a scan over every range.
function rangesByBound(ranges, boundAt) {
  const byBound = new Map();
  for (const r of ranges) {
    const at = boundAt(r);
    if (!byBound.has(at)) {
      byBound.set(at, []);
    }
    byBound.get(at).push(r);
  }

  return byBound;
}

// styledSegment renders one cut of text with every range active over
// it, nesting bold, italic, underline and strike in STYLE_TAGS's fixed
// order inside a link, which always wraps outermost, so two overlapping
// styles always produce well-formed, matching tags regardless of the
// order the caller listed them in. Code and pre share one monospace
// look; spoiler and any other kind add no tag, showing as plain text.
function styledSegment(text, linkColor, active) {
  const link = active.find((r) => r.kind === 'link');
  let html = link ? plainHtml(text) : lineHtml(text, linkColor);

  if (active.some((r) => r.kind === 'code' || r.kind === 'pre')) {
    html = `<span style="font-family:monospace">${html}</span>`;
  }

  for (const [kind, tag] of STYLE_TAGS) {
    if (active.some((r) => r.kind === kind)) {
      html = `<${tag}>${html}</${tag}>`;
    }
  }

  if (link) {
    const style = linkColor ? ` style="color:${linkColor}"` : '';
    html = `<a href="${escapeHtml(link.url)}"${style}>${html}</a>`;
  }

  return html;
}

// mentionHtml renders text with each mention's span wrapped in <b>,
// skipping ahead past a mention that no longer fits text (the message
// was edited to something shorter after it was sent, for instance)
// rather than mis-rendering around it.
function mentionHtml(text, linkColor, mentions) {
  const ordered = [...mentions].sort((a, b) => a.offset - b.offset);
  let html = '';
  let pos = 0;

  for (const m of ordered) {
    if (m.offset < pos || m.offset + m.length > text.length) {
      continue;
    }

    html += lineHtml(text.slice(pos, m.offset), linkColor);
    html += `<b>${lineHtml(text.slice(m.offset, m.offset + m.length), linkColor)}</b>`;
    pos = m.offset + m.length;
  }

  return html + lineHtml(text.slice(pos), linkColor);
}

// lineHtml is messageHtml's handling for a stretch of text that is not a
// table: escape it, link URLs, and keep its line breaks and runs of
// spaces.
function lineHtml(text, linkColor) {
  return linkify(escapeHtml(text), linkColor).replace(/ (?= )/g, '&nbsp;').replace(/\r?\n/g, '<br>');
}

// singleLine collapses a message's line breaks and runs of whitespace
// into single spaces, for showing it on one line: the reply banner above
// the composer, or a quote's preview text. Width-fitting is left to the
// caller's own eliding.
function singleLine(text) {
  return text.replace(/\s+/g, ' ').trim();
}

// longestLine returns a text's widest line, which sets a bubble's width.
function longestLine(text) {
  return text.split('\n').reduce((longest, line) => (line.length > longest.length ? line : longest), '');
}

// highlight wraps case-insensitive matches of query in <b>, never inside an
// HTML entity.
function highlight(escaped, query) {
  if (!query) {
    return escaped;
  }

  const pattern = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');

  return escaped
    .split(/(&[^;]*;)/)
    .map((part) => (part.startsWith('&') && part.endsWith(';') ? part : part.replace(pattern, '<b>$&</b>')))
    .join('');
}

// initials returns the first letter of the first two words, upper-cased,
// or "?" when there are none. Bracketed asides such as "(Manager)" are
// skipped, and so is punctuation, so "Sam (spotty signal)" gives "S".
// Emoji and other wide characters count as one.
function initials(name) {
  const full = name ?? '';
  const withoutAsides = full.replace(/\([^)]*\)/g, ' ');
  const source = /\S/.test(withoutAsides) ? withoutAsides : full;
  const letters = source.split(/\s+/).map(firstLetter).filter((c) => c !== '').slice(0, 2).join('');

  return letters ? letters.toUpperCase() : '?';
}

// firstLetter returns a word's first character that is not punctuation,
// or "".
function firstLetter(word) {
  return Array.from(word).find((ch) => !/[!-\/:-@\[-`{-~\s]/.test(ch)) ?? '';
}

// daysAgo counts calendar days from ms to nowMs in local time. It rounds
// because a day across a daylight-saving change is 23 or 25 hours long.
function daysAgo(ms, nowMs) {
  const startOfDay = (t) => {
    const d = new Date(t);
    return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  };

  return Math.round((startOfDay(nowMs) - startOfDay(ms)) / 86400000);
}

// timeLabel is the time shown in the conversation list: HH:mm today, then
// Yesterday, a weekday within the week, or a short date. A conversation
// with no activity yet (time 0) has no label.
function timeLabel(ms, nowMs) {
  if (!ms) {
    return '';
  }

  const date = new Date(ms);
  if (daysAgo(ms, nowMs) === 0) {
    return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  }

  return relativeDay(ms, nowMs, MONTHS[date.getMonth()].slice(0, 3));
}

// dayLabel is the separator shown between days in a conversation: Today,
// Yesterday, a weekday within the week, or a full date.
function dayLabel(ms, nowMs) {
  if (!ms) {
    return '';
  }

  if (daysAgo(ms, nowMs) === 0) {
    return 'Today';
  }

  return relativeDay(ms, nowMs, MONTHS[new Date(ms).getMonth()]);
}

// relativeDay names a day before today: Yesterday, a weekday within the
// week, or "day month", with the year when it is not this year.
function relativeDay(ms, nowMs, month) {
  const date = new Date(ms);
  const days = daysAgo(ms, nowMs);

  if (days === 1) {
    return 'Yesterday';
  }

  if (days >= 2 && days <= 6) {
    return DAY_NAMES[date.getDay()];
  }

  const sameYear = date.getFullYear() === new Date(nowMs).getFullYear();

  return sameYear ? `${date.getDate()} ${month}` : `${date.getDate()} ${month} ${date.getFullYear()}`;
}

// snoozeUntilLabel describes when a snoozed conversation comes back:
// just the clock time for later today, otherwise the day name or date
// plus the clock time, so a dimmed row reads "Snoozed until 18:00" or
// "Snoozed until Tomorrow 09:00" at a glance. Unlike timeLabel and
// dayLabel, ms is a future time, so days ahead rather than days ago
// names the day.
function snoozeUntilLabel(ms, nowMs) {
  const date = new Date(ms);
  const clock = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  const days = -daysAgo(ms, nowMs);

  if (days <= 0) {
    return clock;
  }

  if (days === 1) {
    return `Tomorrow ${clock}`;
  }

  if (days <= 6) {
    return `${DAY_NAMES[date.getDay()]} ${clock}`;
  }

  const sameYear = date.getFullYear() === new Date(nowMs).getFullYear();
  const datePart = sameYear
    ? `${date.getDate()} ${MONTHS[date.getMonth()].slice(0, 3)}`
    : `${date.getDate()} ${MONTHS[date.getMonth()].slice(0, 3)} ${date.getFullYear()}`;

  return `${datePart} ${clock}`;
}

// pad writes n with at least two digits.
function pad(n) {
  return String(n).padStart(2, '0');
}

// RETRY_SOON_MS is how far ahead a scheduled retry still counts as "soon
// enough to name in minutes" rather than by the clock.
var RETRY_SOON_MS = 60 * 60 * 1000;

// retryingLabel is the quiet line shown next to "Not sent" while a
// failed outgoing message has an automatic retry scheduled: minutes
// away for one due within the hour, the clock time otherwise, or an
// ellipsis once it is due now or overdue, so a tick that lands between
// "scheduled" and "fired" never reads as a wait with no time left.
function retryingLabel(retryAtMs, nowMs) {
  const remaining = retryAtMs - nowMs;
  if (remaining <= 0) {
    return 'Retrying…';
  }

  if (remaining < RETRY_SOON_MS) {
    return `Retrying in ${Math.max(1, Math.round(remaining / 60000))} min`;
  }

  const at = new Date(retryAtMs);
  return `Retrying at ${pad(at.getHours())}:${pad(at.getMinutes())}`;
}

// statusGlyph returns the delivery mark for a status, or "" for none.
function statusGlyph(status) {
  return STATUS_GLYPHS[status] ?? '';
}

// previewLine is the preview under a conversation's title. In groups it
// names who wrote the last message.
function previewLine(conv) {
  if (!conv.preview || conv.kind !== 'group') {
    return previewText(conv.preview);
  }

  if (conv.previewOutgoing) {
    return previewText(`You: ${conv.preview}`);
  }

  return previewText(conv.previewSender ? `${conv.previewSender}: ${conv.preview}` : conv.preview);
}

// MAX_PREVIEW_LENGTH bounds a conversation row's one-line preview: a row
// shows a single elided line, so anything longer is laid out as rich
// text on the shell's UI thread for nothing.
var MAX_PREVIEW_LENGTH = 200;

// previewText cuts text to MAX_PREVIEW_LENGTH characters, for a
// conversation row's preview or search match, before it is escaped.
function previewText(text) {
  return String(text || '').slice(0, MAX_PREVIEW_LENGTH);
}

// MEDIA_LABELS are the texts the helper gives a photo, video, file,
// voice note or sticker sent without a caption, so lists and
// notifications have something to show.
var MEDIA_LABELS = { photo: '[Photo]', video: '[Video]', file: '[File]', voice: '[Voice message]', sticker: '[Sticker]' };

// caption is the text to show beside media: none when the text is only
// the label standing in for the media the bubble already shows.
function caption(text, media) {
  return media && MEDIA_LABELS[media.kind] === text ? '' : text;
}

// fileSize shows a size in bytes as a file manager would, or "" for none.
function fileSize(bytes) {
  if (!bytes) {
    return '';
  }

  const units = ['B', 'KB', 'MB', 'GB'];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }

  return unit === 0 ? `${size} B` : `${size.toFixed(1)} ${units[unit]}`;
}

// IMAGE_EXTENSIONS and VIDEO_EXTENSIONS back guessMediaKind's preview-only
// guess, before the helper has sniffed a picked file's real content.
var IMAGE_EXTENSIONS = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp'];
var VIDEO_EXTENSIONS = ['mp4', 'mov', 'webm', 'mkv', 'avi'];

// baseName returns a file path's last segment, so a local path never
// shows in the composer's attachment chip.
function baseName(path) {
  return path.split('/').pop();
}

// savedLocationLabel names where media.save put a file, for the brief
// confirmation shown after Save: the full path with the user's home
// directory folded to "~", so it reads "Saved to ~/Downloads/clip.gif"
// rather than spelling out the whole path. This file is pure and never
// reads the environment itself, so home is whatever the caller's own
// $HOME resolved to; path is shown in full when it is not under home,
// or home is empty.
function savedLocationLabel(path, home) {
  const shown = home && path.startsWith(home + '/') ? '~' + path.slice(home.length) : path;
  return `Saved to ${shown}`;
}

// guessMediaKind guesses whether a locally picked file is a photo, a
// video or, for anything else, a file, from its name alone, so the
// composer's attachment chip knows whether to show a thumbnail before
// the message is ever sent. The helper sniffs the real content once it
// is sent; this is only ever used for that one preview.
function guessMediaKind(path) {
  const ext = path.split('.').pop().toLowerCase();
  if (IMAGE_EXTENSIONS.includes(ext)) {
    return 'photo';
  }
  if (VIDEO_EXTENSIONS.includes(ext)) {
    return 'video';
  }

  return 'file';
}

// unreadLabel is the text an unread badge shows: the count itself, or
// "99+" once a wider number would start crowding whatever sits beside it.
function unreadLabel(count) {
  return count > 99 ? '99+' : String(count);
}

// duration shows a length in seconds as m:ss, or h:mm:ss, or "" for none.
function duration(seconds) {
  if (!seconds) {
    return '';
  }

  return formatClock(seconds);
}

// elapsed shows the same m:ss / h:mm:ss clock as duration, but always,
// even at zero: the voice note player's own position counts up from
// "0:00" rather than vanishing before playback starts, which duration's
// "unknown length" blank would do for an actual position of zero.
function elapsed(seconds) {
  return formatClock(Math.max(0, seconds || 0));
}

// formatClock is the m:ss / h:mm:ss text duration and elapsed share.
function formatClock(seconds) {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);

  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

// photoSize shows a photo's pixel dimensions as "1920 × 1080", or "" when
// either dimension is not known yet.
function photoSize(width, height) {
  if (!width || !height) {
    return '';
  }

  return `${width} × ${height}`;
}

// MEDIA_FAILURE_REASONS are the safe category words the helper's
// media.fetch error can carry (see server/errors.go), each turned into a
// short phrase a person can actually read in a tooltip.
var MEDIA_FAILURE_REASONS = {
  'not-found': 'no longer available',
  expired: 'no longer on the phone',
  download: 'connection problem',
  decrypt: 'could not be verified',
  cache: 'storage problem',
  timeout: 'took too long',
  'too-large': 'too large to load automatically — tap to load'
};

// mediaFailureReason turns a media.fetch error's safe reason category
// into a short phrase for the "Unavailable" tooltip, or "" for an
// unrecognised or missing reason, so an older helper or a fetch that
// failed for some other tracked reason still shows plain "Unavailable".
function mediaFailureReason(reason) {
  return MEDIA_FAILURE_REASONS[reason] ?? '';
}

// MEDIA_KIND_NOUNS names the media a fetch failure happened for, in the
// phrasing "This <noun> ...", with "file" as the safe catch-all for a
// kind this map does not list.
var MEDIA_KIND_NOUNS = { photo: 'photo', video: 'video', voice: 'voice message', sticker: 'sticker', file: 'file' };

// MEDIA_UNAVAILABLE_ENDINGS are the media.fetch reason categories that
// mean the file is truly gone, each as the sentence ending for
// mediaUnavailableMessage. Any other reason (a connection, cache or
// timeout problem) is worth retrying, so it keeps the helper's own
// generic error text instead of claiming the media is gone for good.
var MEDIA_UNAVAILABLE_ENDINGS = {
  'not-found': 'is no longer available.',
  expired: 'is no longer on the phone.',
  decrypt: 'could not be verified.'
};

// mediaUnavailableMessage names what could not be fetched, such as
// "This video is no longer available.", for a media.fetch reason that
// means the file is truly gone, or "" for any other reason, so the
// caller falls back to the helper's own generic error text instead.
function mediaUnavailableMessage(kind, reason) {
  const ending = MEDIA_UNAVAILABLE_ENDINGS[reason];
  if (!ending) {
    return '';
  }

  const noun = MEDIA_KIND_NOUNS[kind] ?? 'file';
  return `This ${noun} ${ending}`;
}
