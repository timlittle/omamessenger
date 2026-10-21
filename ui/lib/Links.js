.pragma library

// Links.js: the one allow-list check for a URL that came from a
// message, a link preview or a service's own text, before this UI ever
// hands it to Qt.openUrlExternally (see WindowController.openExternalLink
// and tests/unit/openurl-coverage.test.cjs, which fails the build if
// anything else calls Qt.openUrlExternally directly). A file:// path
// this UI built itself from a downloaded photo or video never goes
// through this check: it was never sender-controlled text in the first
// place.

.import "Format.js" as Format

// isAllowed reports whether url's scheme is one a link may safely open,
// reusing the exact allow-list a message's own formatting span already
// checks (see Format.spanURL), so there is only one list to keep in
// step with the helper's own domain.AllowedLinkURL.
function isAllowed(url) {
  return Format.spanURL(url) !== '';
}
