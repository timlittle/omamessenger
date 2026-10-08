package whatsapp

// contacts.go re-titles a direct chat when WhatsApp tells this connector
// who someone actually is after that chat was already reported. A
// contact's saved name or a sender's push name often arrive from
// app-state sync moments after history sync already titled a direct
// chat with its phone number or, for a hidden id, a neutral
// placeholder (see titleFallback); each of these events names at most
// one person, except AppStateSyncComplete, which only signals that a
// whole category of app-state has finished syncing, so this connector
// rechecks every direct chat it has ever reported.

import (
	"context"

	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// nameUpdate bundles the sink, device and media store retitleDirectChat
// and its callers need to resolve and report a better name, so adding
// one more of either never pushes a caller's own argument count over
// this codebase's limit.
type nameUpdate struct {
	sink  connector.Sink
	dev   device
	media *mediaStore
}

// handleContactUpdate re-titles e's conversation when WhatsApp's own
// contact list now names them, so a chat first shown with a phone
// number or hidden id fixes itself once its contact syncs, with no need
// to re-pair.
func (c *Connector) handleContactUpdate(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Contact) {
	name := contactActionName(e.Action)
	if name == "" {
		return
	}

	c.retitleDirectChat(ctx, nameUpdate{sink, dev, media}, e.JID, name, nameRankContact)
}

// handlePushNameUpdate re-titles e's conversation when a message carries
// a push name this connector had not seen for them before, the same way
// a contact update does, just with a lower-trust name that a later
// contact update can still improve on.
func (c *Connector) handlePushNameUpdate(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.PushName) {
	if e.NewPushName == "" {
		return
	}

	c.retitleDirectChat(ctx, nameUpdate{sink, dev, media}, e.JID, e.NewPushName, nameRankPushName)
}

// handleBusinessNameUpdate re-titles e's conversation when WhatsApp
// reports a business's verified name this connector had not seen for
// them before: a business account often sends its first messages
// before whatsmeow has saved its verified name locally (see
// device.contactName), so the chat may have started out titled by its
// phone number or "Unknown contact" until this arrives.
func (c *Connector) handleBusinessNameUpdate(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.BusinessName) {
	if e.NewBusinessName == "" {
		return
	}

	c.retitleDirectChat(ctx, nameUpdate{sink, dev, media}, e.JID, e.NewBusinessName, nameRankBusiness)
}

// handleAppStateSyncComplete rechecks every known direct chat's title
// once a category of app-state has finished syncing: contacts and push
// names often finish moments after history sync already titled those
// chats with a fallback, and this event is the only signal that more
// names may now be known, with no particular JID of its own to check.
// Each lookup is a local read already synced to this device, never a
// network call, so rechecking every known chat stays cheap.
func (c *Connector) handleAppStateSyncComplete(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, _ *events.AppStateSyncComplete) {
	upd := nameUpdate{sink, dev, media}
	for _, remote := range c.knownDirectChats() {
		jid, err := jidFromRemoteID(remote)
		if err != nil {
			continue
		}

		if name := dev.contactName(ctx, jid); name != "" {
			c.retitleDirectChat(ctx, upd, jid, name, nameRankContact)
		}
	}
}

// retitleDirectChat updates this connector's cached name for jid and,
// if that improves it, reports jid's own direct chat again, so the UI
// picks up the better name without a restart, and corrects jid's name
// on every message already stored under a weaker one, anywhere they
// sent one, so a group's preview catches up too when one of its
// members is who resolved (see Sink.SenderName). rememberName never
// lets either of these replace a good name with a worse one. The
// self-chat is never retitled this way, however a contact or push
// name might resolve for the account's own identity: ensureChat is the
// only place that titles it, always with the fixed "Message yourself"
// label (see normalize.go's selfChatTitle). jid is resolved through
// chatID, not the bare remoteID: a contact, push name or business name
// update can name someone by a LID even when their chat is already
// known by its phone JID (or the reverse), and retitling the wrong,
// unresolved id would create a ghost conversation instead of fixing
// the real one's name.
func (c *Connector) retitleDirectChat(ctx context.Context, upd nameUpdate, jid types.JID, name string, rank nameRank) {
	if upd.dev.isSelfChat(ctx, jid) {
		return
	}

	remote := chatID(ctx, upd.dev, upd.media, jid)
	before := c.nameFor(remote)
	after := c.rememberName(remote, name, rank)
	if after == before {
		return
	}

	if namer, ok := upd.sink.(connector.SenderNamer); ok {
		namer.SenderName(ctx, c.account.ID, remote, after)
	}

	c.reportConversation(ctx, upd.sink, domain.Conversation{
		AccountID: c.account.ID, RemoteID: remote, Kind: domain.KindDirect, Title: after,
	})
}

// contactActionName is a contact app-state change's resolved name, in
// WhatsApp's own priority order, or "" when it names no one new.
func contactActionName(act *waSyncAction.ContactAction) string {
	switch {
	case act.GetFullName() != "":
		return act.GetFullName()
	case act.GetFirstName() != "":
		return act.GetFirstName()
	default:
		return ""
	}
}
