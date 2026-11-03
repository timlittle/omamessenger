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

// isAllowed reports whether url's scheme is one a link may safely open,
// reusing the exact allow-list a message's own formatting span already
// checks (see Format.spanURL), so there is only one list to keep in
// step with the helper's own domain.AllowedLinkURL.
function isAllowed(url) {
  return Format.spanURL(url) !== '';
}

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
// carrying a query string (see isQueryMailto), or when displayText
// does not name the same destination as url (see sameTarget). A plain
// autolinked URL always has text identical to its own href, so it
// already matches by the time this runs.
function needsConfirmation(displayText, url) {
  return isQueryMailto(url) || !sameTarget(displayText, url);
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
// which has no limit of its own at this layer.
function urlHost(url) {
  const afterScheme = (url || '').replace(/^[a-zA-Z][a-zA-Z0-9+.-]*:/, '');
  if (!afterScheme.startsWith('//')) {
    return '';
  }

  const authority = afterScheme.slice(2).split(/[/?#]/, 1)[0];
  const host = authority.slice(authority.lastIndexOf('@') + 1);
  return host.replace(/:\d*$/, '');
}

// previewDisplayText returns what a link preview card showed for url,
// for WindowController.openLink to compare against: url itself only
// when it exactly matches one of the anchors the message's own bubble
// actually renders for its text, mentions and formatting spans (see
// Format.messageLinkAnchors) - never a plain substring match of url
// against the raw text, which let a different, shorter domain that
// merely reads as a textual prefix of the text's own unrelated link
// (paypal.co inside paypal.com/signin, say) count as "shown" even
// though the sender controls the preview's own URL entirely apart from
// what the text displays (see docs/decisions.md). Otherwise this
// returns the card's title (or a generic label), since a card whose
// URL appears in none of the bubble's own rendered anchors must be
// confirmed before it opens.
function previewDisplayText(url, messageText, mentions, spans, title) {
  const anchors = Format.messageLinkAnchors(messageText || '', mentions, spans);
  if (url && anchors.some((a) => a.url === url)) return url;

  return title || 'Link preview';
}
