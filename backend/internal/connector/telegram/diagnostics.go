package telegram

// diagnostics.go logs a privacy-safe line for the handful of updates
// this connector still cannot turn into a stored message or
// conversation, even after live.go's and sync.go's own fallbacks (a
// placeholder conversation, a cached peer) have had their chance. Every
// line here carries only a fixed reason, never a peer id, name or
// message's own text, matching WhatsApp's own logDropped (see
// whatsapp/diagnostics.go).

import "log"

// logDropped reports an update this connector could not store, and why.
func logDropped(reason string) {
	log.Printf("telegram: dropped message (reason=%s)", reason)
}
