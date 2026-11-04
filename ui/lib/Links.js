.pragma library

// Links.js: the checks every local or remote link this UI opens goes
// through before Qt.openUrlExternally (see WindowController.openExternalLink
// and tests/unit/openurl-coverage.test.cjs). isAllowed is the
// allow-list check for a remote URL; fileURL percent-encodes a local
// path into a "file://" URL. needsConfirmation and resolveClickedLink
// back the question asked before opening a link whose visible text
// does not match where it actually goes.

.import "Format.js" as Format

// isAllowed reports whether url is safe to open, reusing the same
// rule a message's own formatting span checks (see Format.spanURL).
function isAllowed(url) {
  return Format.spanURL(url) !== '';
}

// REFUSED_LINK_MESSAGE is the short, safe text shown instead of a
// link's real target once isAllowed has refused a URL outright.
var REFUSED_LINK_MESSAGE = 'This link has hidden or unusual characters and was not opened';

// fileURL returns a "file://" URL for the local absolute path, with
// each segment percent-encoded so characters like '#' or '?' can't
// break Qt's own URL parsing (see tests/unit/openurl-coverage.test.cjs).
function fileURL(path) {
  if (!path) {
    return '';
  }

  return 'file://' + path.split('/').map(encodeURIComponent).join('/');
}

// trimTrailingSlashes drops every "/" at text's end, scanning back in
// a loop rather than a regex like /\/+$/, which backtracks quadratically
// on a long run of slashes (a security review found this exploitable).
function trimTrailingSlashes(text) {
  let end = text.length;
  while (end > 0 && text[end - 1] === '/') {
    end--;
  }

  return text.slice(0, end);
}

// normalizeTarget lowers text, trims it, and drops a leading scheme,
// "www." and a trailing slash, so sameTarget ignores those differences.
function normalizeTarget(text) {
  const withoutSchemeAndWww = (text || '').trim().toLowerCase()
    .replace(/^[a-z][a-z0-9+.-]*:\/\//, '')
    .replace(/^www\./, '');

  return trimTrailingSlashes(withoutSchemeAndWww);
}

// sameTarget reports whether displayText and url name the same
// destination once normalizeTarget's own scheme, "www." and trailing
// slash slack is allowed for.
function sameTarget(displayText, url) {
  return normalizeTarget(displayText) === normalizeTarget(url);
}

// isQueryMailto reports whether url is a mailto: link carrying a query
// string, which can silently prefill a recipient, subject or body.
function isQueryMailto(url) {
  return /^mailto:/i.test(url || '') && url.includes('?');
}

// needsConfirmation reports whether a link's visible text could
// mislead about where it actually goes: a query-string mailto (see
// isQueryMailto), a suspicious anchor flagged by Format.js (a bidi
// character, or a span cut short of a full URL), or a textual
// mismatch (see sameTarget).
function needsConfirmation(displayText, url, suspicious = false) {
  return suspicious || isQueryMailto(url) || !sameTarget(displayText, url);
}

// resolveClickedLink turns href, from Qt's onLinkActivated, into
// {url, displayText} by resolving it to one of Format.messageHtml's
// own numbered anchors (see Format.linkHref), never by matching a URL
// against a span, since two spans can share one URL with different
// text. An href that can't be resolved falls back to isAllowed with
// an empty displayText, so the caller always confirms rather than
// opens it unquestioned; one isAllowed rejects resolves to null.
function resolveClickedLink(href, text, mentions, spans) {
  const anchors = Format.messageLinkAnchors(text, mentions, spans);
  const index = Format.linkIndexFromHref(href);
  if (index >= 0 && index < anchors.length) {
    return anchors[index];
  }

  return isAllowed(href) ? { url: href, displayText: '' } : null;
}

// urlHost returns url's authority host, exactly as spelled (never
// decoded or resolved, since decoding itself could be spoofed), or ""
// for a URL with no "//" authority. The real host is whatever comes
// after the LAST unescaped "@", matching how a browser resolves one,
// so userinfo padding with a fake domain can never hide it (see
// docs/decisions.md). Assumes isAllowed already rejected any
// backslash in the authority, which a browser's WHATWG parser treats
// differently from "/".
function urlHost(url) {
  const afterScheme = (url || '').replace(/^[a-zA-Z][a-zA-Z0-9+.-]*:/, '');
  if (!afterScheme.startsWith('//')) {
    return '';
  }

  const authority = afterScheme.slice(2).split(/[/?#]/, 1)[0];
  const host = authority.slice(authority.lastIndexOf('@') + 1);
  return host.replace(/:\d*$/, '');
}

// hostForDisplay returns host unchanged if pure ASCII, or with every
// non-ASCII character spelled out as "\uXXXX" instead of rendered as
// a glyph: a lookalike domain (Cyrillic "а" for Latin "a") would
// otherwise read identical to the real one in LinkOpenConfirm.
function hostForDisplay(host) {
  if (/^[\x00-\x7f]*$/.test(host)) {
    return host;
  }

  return Array.from(host)
    .map((ch) => (ch.codePointAt(0) <= 0x7f ? ch : '\\u{' + ch.codePointAt(0).toString(16) + '}'))
    .join('');
}

// previewDisplayText returns url itself only when it is the exact
// visible text of one of the message's own rendered link anchors (see
// Format.messageLinkAnchors) — never a mere href or substring match,
// both of which a sender could spoof (see docs/decisions.md).
// Otherwise returns the card's title, since an unshown URL must be
// confirmed before it opens.
function previewDisplayText(url, messageText, mentions, spans, title) {
  const anchors = Format.messageLinkAnchors(messageText || '', mentions, spans);
  if (url && anchors.some((a) => a.displayText === url)) return url;

  return title || 'Link preview';
}
