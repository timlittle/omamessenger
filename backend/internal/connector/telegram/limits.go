package telegram

// limits.go sizes an outgoing attachment before it is ever uploaded.
// Telegram's own FAQ ("How large a file can I send over Telegram?",
// https://telegram.org/faq#q-how-do-i-send-files) and its Premium
// announcement (https://telegram.org/blog/premium) both give the same
// two figures: 2 GiB per file for every account, doubled to 4 GiB once
// an account has Telegram Premium. The cap does not depend on whether
// the attachment is a photo, a video or a plain file - gotd uploads
// every kind through the same chunked file API - so AttachmentLimit's
// kind argument only exists to satisfy connector.AttachmentLimiter.

// attachmentLimit is the largest file Telegram accepts from an account
// without Premium.
const attachmentLimit int64 = 2 << 30

// premiumAttachmentLimit is the largest file Telegram accepts from an
// account with Premium.
const premiumAttachmentLimit int64 = 4 << 30

// AttachmentLimit reports the largest attachment this account can
// currently send: premiumAttachmentLimit once Run has signed in as a
// Premium account, attachmentLimit otherwise. kind is accepted only to
// satisfy connector.AttachmentLimiter: Telegram's own cap is the same
// for a photo, a video or a file.
func (c *Connector) AttachmentLimit(kind string) int64 {
	if c.isPremium() {
		return premiumAttachmentLimit
	}

	return attachmentLimit
}
