package app

import (
	"path/filepath"
	"strings"
)

// riskyExtensions are the cache extensions this helper or the sniffed
// content type itself may ever produce that still mean active or
// executable content once opened: a genuine web page or vector image
// can carry a script, and unrecognised binary content ("bin", the
// fallback in mediasniff.go) is never confirmed safe. Most of the
// extensions a sender might have claimed - "exe", "sh", "desktop",
// "jar" and the rest - never survive extensionByContentType's own
// allow-list at all once a file is named from its real content
// instead; they stay listed here anyway as the published catalogue of
// active content this helper must never hand to xdg-open unconfirmed,
// so a future allow-list change can be checked against the same list
// rather than a new one.
var riskyExtensions = map[string]bool{
	"html": true, "htm": true, "xhtml": true, "svg": true,
	"desktop": true, "sh": true, "bash": true, "py": true, "pl": true,
	"rb": true, "jar": true, "exe": true, "msi": true, "appimage": true,
	"run": true, "bin": true, "js": true, "mjs": true, "hta": true,
	"lnk": true, "url": true, "webloc": true,
}

// riskyContentTypes are the sniffed MIME types riskyExtensions cannot
// catch by extension alone, because extensionByContentType maps more
// than one risky type onto the same safe-looking name ("text/plain"
// covers a shell or Python script's own content, say) or a type simply
// never reaches that map at all.
var riskyContentTypes = map[string]bool{
	"text/html": true, "application/xhtml+xml": true, "image/svg+xml": true,
	"application/x-sh": true, "application/x-shellscript": true, "text/x-shellscript": true,
	"application/x-python": true, "text/x-python": true,
	"application/x-executable": true, "application/x-sharedlib": true,
	"application/x-msdownload": true, "application/x-msi": true,
	"application/java-archive": true, "application/x-desktop": true,
	"application/javascript": true, "text/javascript": true,
	"application/x-mswinurl": true,
}

// MediaRisk reports whether the file at path is active or executable
// content - a web page, a vector image or a script - that must never
// reach xdg-open without the user's own confirmation first (see
// isRiskyContent and docs/decisions.md). The server calls this once
// media.fetch or media.save has already placed a file at path, rather
// than FetchMedia or SaveMedia deciding it themselves: a cache hit
// never re-runs whatever named the file the first time, so only
// sniffing the file that is actually about to be opened, every single
// time, is reliable. An unreadable file is reported risky too: this
// helper could not confirm it is safe, which is not the same as
// confirming that it is.
func MediaRisk(path string) bool {
	contentType, err := sniffContentType(path)
	if err != nil {
		return true
	}

	return isRiskyContent(filepath.Ext(path), contentType)
}

// isRiskyContent reports whether ext (without its leading dot, any
// case) or contentType names active or executable content: a web page,
// a vector image or a script can all run code or load remote content
// the moment xdg-open hands them to a browser, interpreter or shell,
// unlike a photo, video, PDF or plain document. Checked by both signals
// together, since either one alone can miss a case the other catches.
func isRiskyContent(ext, contentType string) bool {
	return riskyExtensions[strings.ToLower(strings.TrimPrefix(ext, "."))] || riskyContentTypes[contentType]
}
