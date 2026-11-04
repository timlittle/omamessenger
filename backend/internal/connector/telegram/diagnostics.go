package telegram

// diagnostics.go logs a privacy-safe line for an update this connector
// could not turn into a stored message or conversation.

import "log"

// logDropped reports an update this connector could not store, and why.
func logDropped(reason string) {
	log.Printf("telegram: dropped message (reason=%s)", reason)
}
