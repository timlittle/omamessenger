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

// handleContactUpdate re-titles e's conversation when WhatsApp's own
// contact list now names them, so a chat first shown with a phone
// number or hidden id fixes itself once its contact syncs, with no need
// to re-pair.
func (c *Connector) handleContactUpdate(ctx context.Context, sink connector.Sink, e *events.Contact) {
	name := contactActionName(e.Action)
	if name == "" {
		return
	}

	c.retitleDirectChat(ctx, sink, e.JID, name, nameRankContact)
}

// handlePushNameUpdate re-titles e's conversation when a message carries
// a push name this connector had not seen for them before, the same way
// a contact update does, just with a lower-trust name that a later
// contact update can still improve on.
func (c *Connector) handlePushNameUpdate(ctx context.Context, sink connector.Sink, e *events.PushName) {
	if e.NewPushName == "" {
		return
	}

	c.retitleDirectChat(ctx, sink, e.JID, e.NewPushName, nameRankPushName)
}

// handleAppStateSyncComplete rechecks every known direct chat's title
// once a category of app-state has finished syncing: contacts and push
// names often finish moments after history sync already titled those
// chats with a fallback, and this event is the only signal that more
// names may now be known, with no particular JID of its own to check.
// Each lookup is a local read already synced to this device, never a
// network call, so rechecking every known chat stays cheap.
func (c *Connector) handleAppStateSyncComplete(ctx context.Context, sink connector.Sink, dev device, _ *events.AppStateSyncComplete) {
	for _, remote := range c.knownDirectChats() {
		jid, err := jidFromRemoteID(remote)
		if err != nil {
			continue
		}

		if name := dev.contactName(ctx, jid); name != "" {
			c.retitleDirectChat(ctx, sink, jid, name, nameRankContact)
		}
	}
}

// retitleDirectChat updates this connector's cached name for jid and, if
// that improves its conversation's title, reports the conversation
// again so the UI picks up the better name without a restart.
// rememberName never lets this replace a good title with a worse one.
func (c *Connector) retitleDirectChat(ctx context.Context, sink connector.Sink, jid types.JID, name string, rank nameRank) {
	remote := remoteID(jid)
	before := c.nameFor(remote)
	after := c.rememberName(remote, name, rank)
	if after == before {
		return
	}

	c.reportConversation(ctx, sink, domain.Conversation{
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
