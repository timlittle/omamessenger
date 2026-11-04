.pragma library
.import "Markdown.js" as Markdown
.import "Verify.js" as Verify

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

// URL_PATTERN matches http(s) links in escaped text. Entities such as
// &amp; may appear inside a link; any other '&' ends it, as do '<', a
// double quote, and every character this spells out in place of JS's
// own \s - real, visible or line-breaking whitespace, listed out
// rather than written as \s because \s also matches the byte-order
// mark (U+FEFF), which, unlike the rest, renders as nothing at all. A
// security review found that let a URL carrying one autolink as only
// the short prefix before it: "https://paypal-login.ru", the mark,
// then ".paypal.com/login" read, on screen, as one unbroken
// paypal.com-looking run, with only the short, honest-looking prefix
// actually linked - and, since a plain autolink's own displayText is
// always its own url, opening with no confirmation at all. Every
// other invisible Unicode "format" character HIDDEN_OR_UNUSUAL_CHAR
// lists (zero-width spaces and marks, bidi overrides and isolates,
// word joiner) was never matched by \s to begin with, so leaving only
// the byte-order mark out of this pattern's own terminator class is
// enough to sweep all of them into the match instead of ending it
// there: the combined url then fails spanURL's own hidden-character
// rule and is refused outright, rather than silently linking the
// shorter, innocent-looking run before it (see docs/decisions.md).
var URL_PATTERN =/https?:\/\/[^\u0009\u000a\u000b\u000c\u000d\u0020\u00a0\u1680\u2028\u2029\u202f\u205f\u3000\u2000-\u200a<\x22&]*(?:&(?:amp|lt|quot|#\d+|#x[0-9a-fA-F]+);[^\u0009\u000a\u000b\u000c\u000d\u0020\u00a0\u1680\u2028\u2029\u202f\u205f\u3000\u2000-\u200a<\x22&]*)*/g;

// LINK_HREF_SCHEME marks an anchor href this file wrote itself, naming
// that anchor's own position in the ordered list messageLinkAnchors
// returns, rather than the URL it goes to. Qt's onLinkActivated hands a
// click back only as the clicked anchor's own href: two anchors that
// carry the same real URL would otherwise write that identical href
// twice, and the one thing Qt reports could never tell them apart - the
// gap a sender can abuse by giving two spans the same URL and different
// visible text (see Links.js's own header comment). A private, numbered
// href per anchor means a click always names its own exact anchor,
// never one that merely happens to share its destination.
var LINK_HREF_SCHEME = 'oma-link:';

// linkHref returns the href this file writes for the index-th anchor in
// an anchors list (see the anchors parameter throughout this file): a
// value no sender-controlled text or URL can ever produce, since every
// scheme this file allows a real link to use (see LINK_SCHEMES) is
// checked before this runs and none of them is this one.
function linkHref(index) {
  return LINK_HREF_SCHEME + index;
}

// linkIndexFromHref returns the anchor index href names, or -1 when
// href was never written by linkHref: a caller must never resolve an
// unrecognised href to some anchor by accident, so this only ever
// answers "exactly this one" or "none at all".
function linkIndexFromHref(href) {
  if (typeof href !== 'string' || !href.startsWith(LINK_HREF_SCHEME)) {
    return -1;
  }

  const suffix = href.slice(LINK_HREF_SCHEME.length);
  return /^\d+$/.test(suffix) ? Number(suffix) : -1;
}

// escapeHtml makes text safe to show as rich text.
function escapeHtml(text) {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// ENTITY_PATTERN matches exactly the entities escapeHtml can produce
// (&amp; &lt; &gt; &quot;) plus a decimal or hexadecimal numeric
// character reference, the same set URL_PATTERN's own trailing
// alternation tolerates inside an already-matched URL.
var ENTITY_PATTERN = /&(amp|lt|gt|quot|#\d+|#x[0-9a-fA-F]+);/g;

// unescapeHtml reverses escapeHtml, decoding each entity back to the
// raw character or code point it came from: linkify's own way to
// recover a matched URL's real characters from the escaped text
// URL_PATTERN actually matched (see linkify and docs/decisions.md),
// so the url this file records and later opens is never the HTML-
// escaped form a sender's own '&' would otherwise turn into '&amp;'.
function unescapeHtml(text) {
  return text.replace(ENTITY_PATTERN, (whole, body) => {
    if (body === 'amp') return '&';
    if (body === 'lt') return '<';
    if (body === 'gt') return '>';
    if (body === 'quot') return '"';

    const isHex = body[1] === 'x' || body[1] === 'X';
    const codePoint = parseInt(isHex ? body.slice(2) : body.slice(1), isHex ? 16 : 10);
    return String.fromCodePoint(codePoint);
  });
}

// TRAILING_LINK_PUNCTUATION is the set of characters linkify drops
// off a matched URL's own end.
var TRAILING_LINK_PUNCTUATION = '.,;:!?)';

// splitTrailingPunctuation returns [url, trailing]: match with any run
// of TRAILING_LINK_PUNCTUATION at its own end moved into trailing,
// scanning back from the end in a loop rather than a regex of a lazy
// group followed by a greedy class anchored at $. That shape backtracks
// the lazy group one character at a time for every split point the
// class fails at, which costs the square of match's length on a URL a
// sender pads with tens of thousands of trailing punctuation
// characters - match's length has no limit of its own at this layer.
function splitTrailingPunctuation(match) {
  let end = match.length;
  while (end > 0 && TRAILING_LINK_PUNCTUATION.includes(match[end - 1])) {
    end--;
  }

  return [match.slice(0, end), match.slice(end)];
}

// linkify turns http(s) URLs in escaped text into anchors, in color
// when given, appending one {url, displayText} entry to anchors for
// each (see messageLinkAnchors): a plain autolinked URL's own visible
// text is always the URL itself. Each anchor's href names its own
// position in anchors (see linkHref) rather than carrying the URL
// directly, the same private scheme every anchor this file writes
// uses. Punctuation that ends a sentence stays outside the link. The
// visible text between the anchor's own tags stays in escaped's own
// escaped form, so it renders correctly on screen, but anchors itself
// gets the raw, unescaped url (see unescapeHtml): a security review
// found this recording the escaped form instead - a sender's own '&'
// came back as literally '&amp;' - made the url this file's own
// callers later open functionally different from the one the sender
// actually sent, and made it never match a link preview's own raw URL
// either (see previewDisplayText and docs/decisions.md).
function linkify(escaped, color, anchors = []) {
  const style = color ? ` style="color:${color}"` : '';
  return escaped.replace(URL_PATTERN, (match) => {
    const [displayURL, trailing] = splitTrailingPunctuation(match);
    const url = unescapeHtml(displayURL);
    const href = linkHref(anchors.length);
    anchors.push({ url, displayText: url });
    return `<a href="${href}"${style}>${displayURL}</a>${trailing}`;
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
// length or entity count (see each cap's own comment). Every anchor
// this writes gets a private, numbered href rather than its own URL
// (see linkHref); messageLinkAnchors, called with the same text,
// mentions and spans, is the one place that URL - and the anchor's own
// visible text - can be read back for one, by that href's index, once
// Qt reports a click (see Links.js's resolveClickedLink).
function messageHtml(text, linkColor, tableBorderColor, mentions, spans) {
  return renderMessage(text, linkColor, tableBorderColor, mentions, spans).html;
}

// messageLinkAnchors returns, in the same left-to-right order their
// hrefs appear in messageHtml's own output for the same text, mentions
// and spans, every anchor messageHtml wrote: {url, displayText} pairs,
// where displayText is the whole formatting span's own visible text
// for a span-backed link, even when styledHtml had to cut that span
// into more than one adjoining anchor to nest another style inside it
// (see styledSegment) - never just the clicked fragment. linkColor and
// tableBorderColor never change which anchors exist or their order,
// only their on-screen color, so this runs text through the same
// render as messageHtml without them.
function messageLinkAnchors(text, mentions, spans) {
  return renderMessage(text, '', '', mentions, spans).anchors;
}

// renderMessage is messageHtml's and messageLinkAnchors' shared work:
// the exact same href-writing, anchor-collecting sweep runs for both,
// so an anchor's index can never fall out of step between the href
// messageHtml wrote for it and the entry messageLinkAnchors names for
// that same position.
function renderMessage(text, linkColor, tableBorderColor, mentions, spans) {
  const capped = text.length > MAX_RICH_TEXT_INPUT_LENGTH ? text.slice(0, MAX_RICH_TEXT_INPUT_LENGTH) : text;
  const cappedMentions = mentions && mentions.length > MAX_MENTIONS ? mentions.slice(0, MAX_MENTIONS) : mentions;
  const cappedSpans = spans && spans.length > MAX_FORMATTING_SPANS ? spans.slice(0, MAX_FORMATTING_SPANS) : spans;
  const anchors = [];

  if (cappedSpans && cappedSpans.length > 0) {
    return { html: styledHtml(capped, linkColor, cappedMentions, cappedSpans, anchors), anchors };
  }

  if (cappedMentions && cappedMentions.length > 0) {
    return { html: mentionHtml(capped, linkColor, cappedMentions, anchors), anchors };
  }

  const borderColor = tableBorderColor ?? '#888888';
  const html = Markdown.splitTables(capped)
    .map((part) => (part.kind === 'table'
      ? Markdown.tableHtml(part.table, escapeHtml, (cell) => linkify(cell, linkColor, anchors), borderColor)
      : lineHtml(part.text, linkColor, anchors)))
    .join('');
  return { html, anchors };
}

// LINK_SCHEMES are the URL schemes a formatting span's link may open;
// anything else a service's own link-shaped entity could carry, such as
// a custom app scheme, is dropped rather than rendered as a clickable
// link.
var LINK_SCHEMES = ['http:', 'https:', 'mailto:', 'tg:'];

// HIDDEN_OR_UNUSUAL_CHAR matches a backslash, real whitespace, an
// ASCII control character, or one of the Unicode "format" characters
// (category Cf) a spoofed link has used to hide or split its own
// destination: zero-width spaces and marks (U+200B-U+200F), bidi
// embeddings and overrides (U+202A-U+202E), word joiner and invisible
// operators (U+2060-U+2064), bidi isolates (U+2066-U+2069), and the
// byte-order mark (U+FEFF). None of these ever belongs in a real link;
// spanURL refuses a URL carrying any of them outright, the same rule
// the helper's own domain.AllowedLinkURL enforces (see
// docs/decisions.md). A backslash matters on its own because a
// browser's own WHATWG URL parser ends an http(s) authority at one
// the same as it does a forward slash, while urlHost's own authority
// split (Links.js) does not.
var HIDDEN_OR_UNUSUAL_CHAR = /[\\\s\x00-\x1f\x7f\u200B-\u200F\u202A-\u202E\u2060-\u2064\u2066-\u2069\uFEFF]/;

// hasHTTPAuthority reports whether value - already known to start with
// "http:" or "https:" - spells out "://" followed by a real, non-empty
// authority: the one shape an allowed http(s) URL must take. A
// browser's own URL parser still resolves a host for
// "https:evil.example", "https:\\evil.example" or
// "https:/\evil.example" - each missing a real "//" authority of its
// own, or carrying a backslash standing in for part of it - so this is
// refused here rather than relying only on HIDDEN_OR_UNUSUAL_CHAR's
// backslash check.
function hasHTTPAuthority(value) {
  const rest = value.replace(/^https?:/i, '');
  if (!rest.startsWith('//')) return false;

  const first = rest.charAt(2);
  return first !== '' && first !== '/' && first !== '?' && first !== '#';
}

// spanURL returns url unchanged when its scheme is one LINK_SCHEMES
// allows, it carries none of HIDDEN_OR_UNUSUAL_CHAR's own characters,
// and, for http(s), has a real "scheme://authority" shape (see
// hasHTTPAuthority) - or '' to drop anything else, including text
// with no scheme at all. mailto: and tg: carry no authority of their
// own, so neither is held to that last rule.
function spanURL(url) {
  const value = url || '';
  const match = /^[a-zA-Z][a-zA-Z0-9+.-]*:/.exec(value);
  if (!match || !LINK_SCHEMES.includes(match[0].toLowerCase())) return '';
  if (HIDDEN_OR_UNUSUAL_CHAR.test(value)) return '';

  const scheme = match[0].toLowerCase();
  const isHTTP = scheme === 'http:' || scheme === 'https:';
  return isHTTP && !hasHTTPAuthority(value) ? '' : value;
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
// needs, not the square of its span count. anchors collects one
// {url, displayText} entry per link anchor written, in the order
// their hrefs appear in the returned html (see linkHref and
// messageLinkAnchors).
function styledHtml(text, linkColor, mentions, spans, anchors = []) {
  const ranges = styledRanges(text, mentions, spans);
  if (ranges.length === 0) {
    return lineHtml(text, linkColor, anchors);
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
    html += styledSegment(text, start, end, linkColor, active, anchors);

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

// styledSegment renders text's [start, end) cut with every range active
// over it, nesting bold, italic, underline and strike in STYLE_TAGS's
// fixed order inside a link, which always wraps outermost, so two
// overlapping styles always produce well-formed, matching tags
// regardless of the order the caller listed them in. Code and pre
// share one monospace look; spoiler and any other kind add no tag,
// showing as plain text. A link's anchor gets a private, numbered href
// (see linkHref) rather than its own URL, and anchors gets one more
// {url, displayText} entry for it - displayText is the link range's
// whole own text (text.slice(link.start, link.end)), not just this one
// cut of it, so a link split into several adjoining anchors by a
// nested style still reports its full visible text for any one of
// them (see messageLinkAnchors).
function styledSegment(text, start, end, linkColor, active, anchors = []) {
  const segment = text.slice(start, end);
  const link = active.find((r) => r.kind === 'link');
  let html = link ? plainHtml(segment) : lineHtml(segment, linkColor, anchors);

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
    const href = linkHref(anchors.length);
    anchors.push({ url: link.url, displayText: text.slice(link.start, link.end) });
    html = `<a href="${href}"${style}>${html}</a>`;
  }

  return html;
}

// mentionHtml renders text with each mention's span wrapped in <b>,
// skipping ahead past a mention that no longer fits text (the message
// was edited to something shorter after it was sent, for instance)
// rather than mis-rendering around it. anchors collects a {url,
// displayText} entry for every autolinked bare URL found along the way
// (see lineHtml and messageLinkAnchors); a mention is never a link
// itself, so it adds none of its own.
function mentionHtml(text, linkColor, mentions, anchors = []) {
  const ordered = [...mentions].sort((a, b) => a.offset - b.offset);
  let html = '';
  let pos = 0;

  for (const m of ordered) {
    if (m.offset < pos || m.offset + m.length > text.length) {
      continue;
    }

    html += lineHtml(text.slice(pos, m.offset), linkColor, anchors);
    html += `<b>${lineHtml(text.slice(m.offset, m.offset + m.length), linkColor, anchors)}</b>`;
    pos = m.offset + m.length;
  }

  return html + lineHtml(text.slice(pos), linkColor, anchors);
}

// lineHtml is messageHtml's handling for a stretch of text that is not a
// table: escape it, link URLs, and keep its line breaks and runs of
// spaces. anchors collects one {url, displayText} entry per autolinked
// URL (see linkify and messageLinkAnchors).
function lineHtml(text, linkColor, anchors = []) {
  return linkify(escapeHtml(text), linkColor, anchors).replace(/ (?= )/g, '&nbsp;').replace(/\r?\n/g, '<br>');
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

// splitEntities splits text into alternating plain and entity chunks,
// an entity being "&" through its own next ";" kept together as one
// piece - the same split /(&[^;]*;)/ produced, as a forward scan
// rather than that regex: a security review found the regex shape
// unanchored quantifier, excluded-terminator character class,
// applied globally - backtracks that class to zero at every "&" in a
// row with no ";" anywhere after it, costing the square of the run's
// length, the same root cause already fixed for a different regex in
// Links.trimTrailingSlashes. highlight's own escaped text is meant to
// always carry a ";" within a few characters of every "&" (escapeHtml
// writes only "&amp;", "&lt;", "&gt;" or "&quot;"), but this keeps the
// function itself safe regardless of what a future caller hands it.
function splitEntities(text) {
  const parts = [];
  let start = 0;
  let i = 0;
  while (i < text.length) {
    if (text[i] !== '&') {
      i++;
      continue;
    }

    const semicolon = text.indexOf(';', i + 1);
    if (semicolon === -1) {
      // No ";" anywhere after this "&", so none of the "&"s still
      // ahead can find one either: stop scanning and keep the rest
      // as one plain chunk.
      break;
    }

    parts.push(text.slice(start, i));
    parts.push(text.slice(i, semicolon + 1));
    i = semicolon + 1;
    start = i;
  }

  parts.push(text.slice(start));
  return parts;
}

// highlight wraps case-insensitive matches of query in <b>, never inside an
// HTML entity.
function highlight(escaped, query) {
  if (!query) {
    return escaped;
  }

  const pattern = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');

  return splitEntities(escaped)
    .map((part) => (part.startsWith('&') && part.endsWith(';') ? part : part.replace(pattern, '<b>$&</b>')))
    .join('');
}

// stripAsides removes a bracketed aside such as "(Manager)" from
// full, replacing each with a single space - the same substitution
// /\([^)]*\)/g made, as a forward scan rather than that regex, for
// the same reason splitEntities replaced highlight's own one: a run
// of "(" with no ")" anywhere after it backtracks that regex's
// excluded-terminator class to zero at every one of them. A display
// name reaching initials is capped server-side
// (domain.MaxDisplayNameLength) today, but this keeps the function
// itself safe regardless of any caller's own limit.
function stripAsides(full) {
  let out = '';
  let i = 0;
  while (i < full.length) {
    if (full[i] !== '(') {
      out += full[i];
      i++;
      continue;
    }

    const close = full.indexOf(')', i + 1);
    if (close === -1) {
      // No ")" anywhere after this "(": keep the rest exactly as it
      // is, the same as the regex's own failed match would have.
      out += full.slice(i);
      break;
    }

    out += ' ';
    i = close + 1;
  }

  return out;
}

// initials returns the first letter of the first two words, upper-cased,
// or "?" when there are none. Bracketed asides such as "(Manager)" are
// skipped, and so is punctuation, so "Sam (spotty signal)" gives "S".
// Emoji and other wide characters count as one.
function initials(name) {
  const full = name ?? '';
  const withoutAsides = stripAsides(full);
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

// sendAtLabel describes when a scheduled message will send: "Sends at
// 18:00" for later today, otherwise "Sends at Tomorrow 09:00" or "Sends
// at Mon 09:00", the same clock-time wording snoozeUntilLabel already
// reads a future time with, just under this command's own prefix.
function sendAtLabel(ms, nowMs) {
  return `Sends at ${snoozeUntilLabel(ms, nowMs)}`;
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
// names who wrote the last message, marked with a leading "~" when
// that sender's own name is unverified - a bare, self-chosen name a
// group member fully controls - the same way an unverified senderName
// is marked inside the chat itself (see Verify.markedName and
// docs/decisions.md): a security review found this line left that
// name unmarked even when the bubble it comes from shows it marked.
function previewLine(conv) {
  if (!conv.preview || conv.kind !== 'group') {
    return previewText(conv.preview);
  }

  if (conv.previewOutgoing) {
    return previewText(`You: ${conv.preview}`);
  }

  if (!conv.previewSender) {
    return previewText(conv.preview);
  }

  const sender = Verify.markedName(conv.previewSender, conv.previewSenderVerified);
  return previewText(`${sender}: ${conv.preview}`);
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
// voice note, audio message or sticker sent without a caption, so lists
// and notifications have something to show.
var MEDIA_LABELS = { photo: '[Photo]', video: '[Video]', file: '[File]', voice: '[Voice message]', audio: '[Audio]', sticker: '[Sticker]' };

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
var MEDIA_KIND_NOUNS = { photo: 'photo', video: 'video', voice: 'voice message', audio: 'audio message', sticker: 'sticker', file: 'file' };

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
