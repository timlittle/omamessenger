package whatsapp

// mentions.go turns WhatsApp's mention metadata into ours, and back.
// WhatsApp carries no position information of its own: a mention is
// just a literal "@<the mentioned JID's user part>" substring in the
// message's text, alongside ContextInfo.MentionedJID naming who each
// one is; a receiving client is expected to replace that substring with
// the person's name, and a sending client to write it back. Both
// directions go through domain.ByteOffsetsForUTF16 and domain.UTF16Len,
// since the UI's own text and Mention offsets are in UTF-16 code units,
// while this file edits the UTF-8 Go string WhatsApp's wire format uses.

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// mentionToken matches a mention's literal text: "@" followed by the
// digits of a phone number or a LID, long enough that it is never
// mistaken for an ordinary "@3" in running text.
var mentionToken = regexp.MustCompile(`@([0-9]{5,})`)

// rewriteMentions replaces each "@<digits>" mention token named by
// mentionedJIDs with the mentioned person's resolved name, reporting
// the rewritten text, a Mention for each one at its new position, and
// whether any of them is the signed-in account itself. Text with no
// mentioned JIDs, or none of its tokens matching one, is returned
// unchanged.
func rewriteMentions(ctx context.Context, dev device, text string, mentionedJIDs []string) (string, []domain.Mention, bool) {
	if len(mentionedJIDs) == 0 {
		return text, nil, false
	}

	byDigits := map[string]types.JID{}
	for _, raw := range mentionedJIDs {
		if jid, err := types.ParseJID(raw); err == nil {
			byDigits[jid.User] = jid
		}
	}

	matches := mentionToken.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil, false
	}

	var b strings.Builder
	var out []domain.Mention
	mentionsMe, last, utf16Pos := false, 0, 0

	for _, m := range matches {
		start, end, digitsStart, digitsEnd := m[0], m[1], m[2], m[3]
		jid, ok := byDigits[text[digitsStart:digitsEnd]]
		if !ok {
			continue
		}

		before := text[last:start]
		b.WriteString(before)
		utf16Pos += domain.UTF16Len(before)

		name := mentionDisplayName(ctx, dev, jid)
		token := "@" + name
		b.WriteString(token)
		out = append(out, domain.Mention{UserID: remoteID(jid), Name: name, Offset: utf16Pos, Length: domain.UTF16Len(token)})
		utf16Pos += domain.UTF16Len(token)

		if dev.isSelfChat(ctx, jid) {
			mentionsMe = true
		}
		last = end
	}

	if out == nil {
		return text, nil, false
	}

	b.WriteString(text[last:])

	return b.String(), out, mentionsMe
}

// mentionDisplayName is the name a mention of jid renders with: their
// resolved contact or push name, or the generic label when nothing is
// known locally.
func mentionDisplayName(ctx context.Context, dev device, jid types.JID) string {
	if name := dev.contactName(ctx, jid); name != "" {
		return name
	}

	return genericSenderName
}

// outgoingMentions rewrites text's mention tokens, each addressed by a
// Mention's UTF-16 Offset and Length, back into WhatsApp's own
// "@<digits>" form, and lists the JIDs to put in ContextInfo's
// MentionedJID so WhatsApp's clients render them. Every byte offset is
// found in the original text, never in a copy already rewritten by an
// earlier mention, since offsets are only valid against the text the
// composer actually measured them in. A mention whose UserID is not a
// WhatsApp remote id this connector made, or whose range no longer fits
// text or overlaps an earlier one, is left as plain text rather than
// failing the whole send.
func outgoingMentions(text string, mentions []domain.Mention) (string, []string) {
	if len(mentions) == 0 {
		return text, nil
	}

	ordered := slices.Clone(mentions)
	slices.SortFunc(ordered, func(a, b domain.Mention) int { return a.Offset - b.Offset })

	var b strings.Builder
	jids := make([]string, 0, len(ordered))
	copied := 0

	for _, mn := range ordered {
		jid, err := jidFromRemoteID(mn.UserID)
		if err != nil {
			continue
		}

		start, end, ok := domain.ByteOffsetsForUTF16(text, mn.Offset, mn.Length)
		if !ok || start < copied {
			continue
		}

		b.WriteString(text[copied:start])
		b.WriteString("@" + jid.User)
		jids = append(jids, remoteID(jid))
		copied = end
	}

	if len(jids) == 0 {
		return text, nil
	}

	b.WriteString(text[copied:])

	return b.String(), jids
}
