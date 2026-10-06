.pragma library

// Text formatting for the conversation list and message view: HTML escaping
// and links, search highlights, initials, time labels and status glyphs.

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

// linkify turns http(s) URLs in escaped text into links. Punctuation that
// ends a sentence stays outside the link.
function linkify(escaped) {
  return escaped.replace(URL_PATTERN, (match) => {
    const [, url, trailing] = match.match(/^(.*?)([.,;:!?)]*)$/);
    return `<a href="${url}">${url}</a>${trailing}`;
  });
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
// or "?" when there are none. Emoji and other wide characters count as one.
function initials(name) {
  const words = (name ?? '').trim().split(/\s+/).filter((word) => word.length > 0);
  const letters = words.slice(0, 2).map((word) => Array.from(word)[0]).join('');

  return letters ? letters.toUpperCase() : '?';
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
// Yesterday, a weekday within the week, or a short date.
function timeLabel(ms, nowMs) {
  const date = new Date(ms);
  if (daysAgo(ms, nowMs) === 0) {
    return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  }

  return relativeDay(ms, nowMs, MONTHS[date.getMonth()].slice(0, 3));
}

// dayLabel is the separator shown between days in a conversation: Today,
// Yesterday, a weekday within the week, or a full date.
function dayLabel(ms, nowMs) {
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

// pad writes n with at least two digits.
function pad(n) {
  return String(n).padStart(2, '0');
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
