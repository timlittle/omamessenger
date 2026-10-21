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
// handed to a URL as a raw string either.

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
