package signal

// limits.go sizes an outgoing attachment before it is uploaded.
// Signal caps every attachment, whatever its kind, at 100 MiB.

import "github.com/timlittle/omamessenger/backend/internal/connector"

// attachmentLimit is the largest attachment Signal accepts.
const attachmentLimit int64 = 100 << 20

// Connector satisfies connector.AttachmentLimiter.
var _ connector.AttachmentLimiter = (*Connector)(nil)

// AttachmentLimit reports the largest attachment of kind this account
// can send: the same limit for every kind.
func (*Connector) AttachmentLimit(string) int64 {
	return attachmentLimit
}
