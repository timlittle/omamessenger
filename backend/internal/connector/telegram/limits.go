package telegram

import "github.com/timlittle/omamessenger/backend/internal/connector"

// limits.go sizes an outgoing attachment before it is ever uploaded:
// 2 GiB per file for every account, 4 GiB with Telegram Premium
// (https://telegram.org/faq, https://telegram.org/blog/premium).

// attachmentLimit is the largest file Telegram accepts from an account
// without Premium.
const attachmentLimit int64 = 2 << 30

// Connector satisfies connector.AttachmentLimiter: checked at compile
// time since nothing else here calls AttachmentLimit through that
// interface, so a signature drift would otherwise only show up as a
// missing composer hint, never a build failure.
var _ connector.AttachmentLimiter = (*Connector)(nil)

// premiumAttachmentLimit is the largest file Telegram accepts from an
// account with Premium.
const premiumAttachmentLimit int64 = 4 << 30

// AttachmentLimit reports the largest attachment this account can
// currently send. kind is accepted only to satisfy
// connector.AttachmentLimiter: Telegram's cap doesn't depend on it.
func (c *Connector) AttachmentLimit(kind string) int64 {
	if c.isPremium() {
		return premiumAttachmentLimit
	}

	return attachmentLimit
}
