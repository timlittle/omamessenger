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
// conversion is needed here); each one renders in bold. A message with
// mentions skips table rendering, since an @-mention is ordinary chat
// text, never a pipe table.
function messageHtml(text, linkColor, tableBorderColor, mentions) {
  if (mentions && mentions.length > 0) {
    return mentionHtml(text, linkColor, mentions);
  }

  const borderColor = tableBorderColor ?? '#888888';
  return Markdown.splitTables(text)
    .map((part) => (part.kind === 'table'
      ? Markdown.tableHtml(part.table, escapeHtml, (cell) => linkify(cell, linkColor), borderColor)
      : lineHtml(part.text, linkColor)))
    .join('');
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
    return conv.preview || '';
  }

  if (conv.previewOutgoing) {
    return `You: ${conv.preview}`;
  }

  return conv.previewSender ? `${conv.previewSender}: ${conv.preview}` : conv.preview;
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
  timeout: 'took too long'
};

// mediaFailureReason turns a media.fetch error's safe reason category
// into a short phrase for the "Unavailable" tooltip, or "" for an
// unrecognised or missing reason, so an older helper or a fetch that
// failed for some other tracked reason still shows plain "Unavailable".
function mediaFailureReason(reason) {
  return MEDIA_FAILURE_REASONS[reason] ?? '';
}
