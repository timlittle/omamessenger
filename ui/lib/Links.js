.pragma library

// Links.js: the checks every local or remote link this UI opens goes
// through. isAllowed is the one allow-list check for a URL that came
// from a message, a link preview or a service's own text, before this
// UI ever hands it to Qt.openUrlExternally (see
// WindowController.openExternalLink and tests/unit/openurl-coverage.test.cjs,
// which fails the build if anything else calls Qt.openUrlExternally
// directly). fileURL is the one place a local path - this helper's own
// downloaded media, the user's own outgoing attachment, this plugin's
// keys.conf - becomes a "file://" URL, percent-encoded so a character
// the path legitimately holds can never break it. A local path is not
// the same kind of untrusted text a message's link is, so isAllowed's
// scheme check does not apply to it, but it is not risk-free either:
// this helper names a downloaded file from its own sniffed content,
// never a sender's claimed name (see docs/decisions.md), yet the path
// can still hold any character a file name allows, so it is never
// handed to a URL as a raw string either. needsConfirmation and
// resolveClickedLink back WindowController.openLink's own question
// before opening a link whose own visible text does not read as the
// same destination as its href - a Telegram MessageEntityTextURL lets
// a sender show one piece of text while actually linking somewhere
// else entirely, which isAllowed's scheme check alone would never
// catch. resolveClickedLink exists because that is not even safe to
// decide by matching a click's href against a span's own URL: two
// spans can carry the very same URL with different visible text, so
// every anchor messageHtml writes gets its own private, numbered href
// instead (see Format.linkHref), naming exactly the anchor clicked
// rather than one that merely shares its destination.

.import "Format.js" as Format

// isAllowed reports whether url is safe to open - its scheme, and now
// also its characters and, for http(s), its own "scheme://authority"
// shape - reusing the exact rule a message's own formatting span
// already checks (see Format.spanURL), so there is only one rule to
// keep in step with the helper's own domain.AllowedLinkURL.
function isAllowed(url) {
  return Format.spanURL(url) !== '';
}

// REFUSED_LINK_MESSAGE is the short, safe text shown instead of a
// link's real target, or any question about it, once isAllowed has
// refused a URL outright: one of Format.HIDDEN_OR_UNUSUAL_CHAR's own
// characters, or an http(s) URL with no real authority of its own
// (see Format.spanURL). Never a confirmation naming a host computed
// from characters that could make it wrong (see docs/decisions.md).
var REFUSED_LINK_MESSAGE = 'This link has hidden or unusual characters and was not opened';

// fileURL returns a "file://" URL for the local absolute path, with
// each path segment percent-encoded: Qt parses whatever is handed to a
// URL property or Qt.openUrlExternally as a URL, not a plain string,
// so a path holding '#', '?', '%', a space or non-ASCII text would
// otherwise be cut short or misread. Every local file this UI opens or
// shows goes through this rather than a raw "file://" + path
// concatenation (see tests/unit/openurl-coverage.test.cjs).
function fileURL(path) {
  if (!path) {
    return '';
  }

  return 'file://' + path.split('/').map(encodeURIComponent).join('/');
}

// trimTrailingSlashes drops every "/" at text's own end, scanning back
// from the end in a loop rather than the regex this replaced,
// /\/+$/: with no start anchor, a regex engine retries that pattern
// from every position in a long run of slashes, backtracking the "+"
// at each one, which costs the square of the run's length. A security
// review found exactly that shape here - the same reasoning that
// already turned Format.splitTrailingPunctuation into a loop (which
// Highlight.links now reuses, through Format.plainLinks, rather than
// keeping a second copy of its own) - just missed on this one. A
// sender's own link, and the text naming it, both reach this with no
// length limit of their own at this layer, so the loop is what keeps
// a single click or the "o" key linear rather than letting one
// adversarial link stall the whole shell.
function trimTrailingSlashes(text) {
  let end = text.length;
  while (end > 0 && text[end - 1] === '/') {
    end--;
  }

  return text.slice(0, end);
}

// normalizeTarget lowers text, trims it, and drops a leading scheme,
// a leading "www." and a trailing slash, the one comparison
// sameTarget applies to both a link's visible text and its href, so
// neither difference alone counts as a mismatch.
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
// string ("?subject=", "?body=", "?attach=", ...): one of those can
// silently prefill a recipient, subject, body or attachment a person
// never typed, so it always needs confirming, whatever its own
// visible text says.
function isQueryMailto(url) {
  return /^mailto:/i.test(url || '') && url.includes('?');
}

// needsConfirmation reports whether a link's own visible text could
// mislead about where it actually goes: true for a mailto: link
// carrying a query string (see isQueryMailto), when suspicious says
// so, or when displayText does not name the same destination as url
// (see sameTarget). suspicious is the anchor's own flag Format.js
// computed while rendering (see Format.messageLinkAnchors): true when
// the message carries a bidi or other invisible format character
// anywhere at all - a right-to-left override placed just before a
// plain URL reverses how the whole run reads on screen while leaving
// the matched text and its own href identical, which sameTarget's
// plain textual comparison can never catch - or when the anchor's own
// span stopped short of what reads as one continuous URL, cut by a
// connector's own link entity or a formatting span that covers only
// part of it (see Format.continuesURLToken and docs/decisions.md). A
// plain autolinked URL always has text identical to its own href, so
// sameTarget alone would otherwise never catch either case.
function needsConfirmation(displayText, url, suspicious = false) {
  return suspicious || isQueryMailto(url) || !sameTarget(displayText, url);
}

// resolveClickedLink turns href - exactly what Qt's onLinkActivated
// hands back for a click inside a message's own rich text - into
// {url, displayText}: href names one of Format.messageHtml's own
// anchors by position (see Format.linkHref), computed here from the
// same text, mentions and spans the bubble rendered, never a URL to
// match a span against, which is what let two spans sharing one URL
// make a click on either report whichever span's text a lookup found
// first (see this file's own header comment). An href this function
// cannot resolve to one of those anchors - message fell out of step
// with it, Qt's own report mangled - is never assumed safe: its own
// scheme must still pass isAllowed, and even then it comes back with
// an empty displayText, which sameTarget can never call a match for
// any non-empty url, so the caller always asks rather than opening it
// unquestioned. An href whose scheme isAllowed rejects resolves to
// null instead, for the caller to refuse outright.
function resolveClickedLink(href, text, mentions, spans) {
  const anchors = Format.messageLinkAnchors(text, mentions, spans);
  const index = Format.linkIndexFromHref(href);
  if (index >= 0 && index < anchors.length) {
    return anchors[index];
  }

  return isAllowed(href) ? { url: href, displayText: '' } : null;
}

// urlHost returns url's own authority host, exactly as the URL spells
// it - scheme and userinfo stripped, a punycode (xn--...) or literal
// IPv4/IPv6 host shown as is, never decoded or resolved, since
// decoding is itself a step a spoof could abuse - or "" when url
// carries no "//" authority at all (a mailto: link, say, which has no
// separate host to show). The real host is whatever comes after the
// LAST unescaped "@" in the authority, matching how a browser
// resolves one: a sender can pad the userinfo before it with any text
// at all, including a convincing fake domain name, and LinkOpenConfirm
// shows exactly this - never elided - so that padding can never push
// the real host out of view (see docs/decisions.md). lastIndexOf and a
// single bounded regex replace keep this linear in url's own length,
// which has no limit of its own at this layer. This only agrees with
// the browser that will actually open url once isAllowed has already
// accepted it: a backslash inside the authority ends it one way here
// (splitting only on "/", "?" or "#") and another way in a browser's
// own WHATWG parser (which treats "\" the same as "/"). isAllowed now
// refuses any URL carrying one at all (see Format.spanURL and
// docs/decisions.md), and LinkOpenConfirm checks that itself before
// this function ever runs on the link it shows, rather than leaving
// this function to resolve a disallowed URL's host differently from
// the browser that would have opened it.
function urlHost(url) {
  const afterScheme = (url || '').replace(/^[a-zA-Z][a-zA-Z0-9+.-]*:/, '');
  if (!afterScheme.startsWith('//')) {
    return '';
  }

  const authority = afterScheme.slice(2).split(/[/?#]/, 1)[0];
  const host = authority.slice(authority.lastIndexOf('@') + 1);
  return host.replace(/:\d*$/, '');
}

// hostForDisplay returns host unchanged when it is pure ASCII, or, when
// it carries any character outside that range, a form that spells each
// one out as its own "\uXXXX" code point rather than rendering it as a
// glyph at all: a security review found LinkOpenConfirm showing a
// lookalike host exactly as spelled - "pаypal.com" with a Cyrillic "а"
// standing in for the Latin letter - looks identical to the real
// domain the user actually meant to open, which defeats the one job
// this confirmation has. No decoding, resolving or script-name lookup
// of any kind runs here, since any of those is itself a step a spoof
// could abuse (see urlHost's own doc comment); this only ever changes
// how a character already in host is shown, never which host string
// this helper or Qt.openUrlExternally actually treats as real.
function hostForDisplay(host) {
  if (/^[\x00-\x7f]*$/.test(host)) {
    return host;
  }

  return Array.from(host)
    .map((ch) => (ch.codePointAt(0) <= 0x7f ? ch : '\\u{' + ch.codePointAt(0).toString(16) + '}'))
    .join('');
}

// previewDisplayText returns what a link preview card showed for url,
// for WindowController.openLink to compare against: url itself only
// when it is itself the exact visible text of one of the anchors the
// message's own bubble actually renders for its text, mentions and
// formatting spans (see Format.messageLinkAnchors) - never merely one
// of those anchors' own href, which let a span whose visible text
// reads as something else entirely ("Click here", say) count a
// preview sharing that same href as "shown", even though the user
// never saw that url as text anywhere in the message; nor a plain
// substring match of url against the raw text, which let a different,
// shorter domain that merely reads as a textual prefix of the text's
// own unrelated link (paypal.co inside paypal.com/signin, say) count
// as "shown" the same way (see docs/decisions.md). Otherwise this
// returns the card's title (or a generic label), since a card whose
// URL was never itself shown as text anywhere in the bubble must be
// confirmed before it opens - the sender picks both the preview's own
// URL and its title entirely apart from what the message text shows.
function previewDisplayText(url, messageText, mentions, spans, title) {
  const anchors = Format.messageLinkAnchors(messageText || '', mentions, spans);
  if (url && anchors.some((a) => a.displayText === url)) return url;

  return title || 'Link preview';
}
