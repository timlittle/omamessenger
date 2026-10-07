// Package connector is the boundary between messaging services and the
// application. A Connector speaks one service's protocol and reports
// normalized updates to a Sink; the Manager runs and supervises connectors.
package connector

import (
	"context"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// ErrInvalidSetup reports options Provider.Prepare could not use, such as
// an API id given without its hash.
var ErrInvalidSetup = errors.New("connector: invalid setup")

// ErrUnknownProvider reports a service with no registered Provider.
var ErrUnknownProvider = errors.New("connector: unknown provider")

// Connector adapts one signed-in messaging account to the normalized domain.
// Send and MarkRead return quickly; progress is reported through the Sink.
type Connector interface {
	// Account describes the account this connector serves.
	Account() domain.Account

	// Run connects and reports updates to sink until ctx is cancelled or
	// the connection fails.
	Run(ctx context.Context, sink Sink) error

	// Send delivers an outgoing message.
	Send(ctx context.Context, conv domain.Conversation, m domain.Message) error

	// MarkRead tells the service the user has read a conversation.
	MarkRead(ctx context.Context, conv domain.Conversation) error
}

// Provider is the published way to add a messaging service to
// OmaMessenger: it names the service, prepares a new account's
// credentials, connects it, and forgets it when removed. The four methods
// beyond Service are justified by what a new service must supply end to
// end, not just a running connection: a Connector alone cannot be set up
// or torn down without service-specific code living outside this package.
type Provider interface {
	// Service is the service id this provider handles, such as "telegram".
	Service() string

	// Name is the human-readable name the UI shows for this service.
	Name() string

	// Prepare saves whatever a new account needs before it first
	// connects, reading any user-supplied values from options. Invalid
	// options return an error wrapping ErrInvalidSetup.
	Prepare(dir, accountID string, options map[string]string) error

	// Connect returns the account's connector, reading what Prepare saved
	// from dir.
	Connect(account domain.Account, dir string) Connector

	// Forget deletes everything Prepare saved for the account.
	Forget(dir, accountID string) error
}

// Authenticator is a Connector that signs in to its service. While it
// waits for the user it reports what it needs with Sink.AuthStep, and
// SubmitAuth delivers the answer.
type Authenticator interface {
	// SubmitAuth answers the step the connector asked for: "phone",
	// "code" or "password".
	SubmitAuth(ctx context.Context, step, value string) error
}

// HistoryLoader is a Connector that can fetch history older than what has
// been reported, for when the user scrolls back past it.
type HistoryLoader interface {
	// LoadOlder reports, through the Sink's History, up to limit messages
	// older than the one the service knows as beforeRemoteID, or the newest
	// when it is "", and says how many it found.
	LoadOlder(ctx context.Context, conv domain.Conversation, beforeRemoteID string, limit int) (int, error)
}

// MediaFetcher is a Connector that can download a message's photo, video
// or file.
type MediaFetcher interface {
	// FetchMedia writes the media of the message the service knows as
	// messageRemoteID to path.
	FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string) error
}

// MessageRefresher is a Connector that can re-report messages already
// stored, for when they were saved before the connector could report
// their media or a link preview.
type MessageRefresher interface {
	// RefreshMessages re-reports the messages the service knows by
	// remoteIDs, through the Sink's History, so the store can fill in
	// what they were missing.
	RefreshMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string) error
}

// Reactor is a Connector that can add or remove the signed-in user's own
// reaction to a message.
type Reactor interface {
	// React sets the user's reaction to the message the service knows as
	// messageRemoteID to emoji, or clears it when emoji is "".
	React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error
}

// ErrPinLimit reports a pin an Organizer's service refuses because the
// account already has as many conversations pinned as that service
// allows.
var ErrPinLimit = errors.New("connector: pin limit reached")

// Organizer is a Connector that keeps a conversation pinned or archived in
// step with the service, so a change made here, or made on another
// device and synced back, agrees everywhere. The two methods are kept
// together because every service that offers either offers both through
// the same mechanism (Telegram: toggling a dialog flag), so a caller that
// type-asserts for one almost always wants the other.
type Organizer interface {
	// SetPinned pins or unpins a conversation at the top of the list.
	SetPinned(ctx context.Context, conv domain.Conversation, pinned bool) error

	// SetArchived files a conversation away, or brings it back.
	SetArchived(ctx context.Context, conv domain.Conversation, archived bool) error
}

// LogoutOnRemove is a Connector that can tell its service to unlink this
// device before Remove deletes its local session, so removing the
// account here also removes it from the list of devices linked on the
// user's phone.
type LogoutOnRemove interface {
	// Logout unlinks this device from its service. It is best effort:
	// Remove deletes the local session whether or not it succeeds.
	Logout(ctx context.Context) error
}

// AuthStep is what a signing-in connector needs from the user next.
type AuthStep struct {
	// Kind is "qr", "phone", "code" or "password".
	Kind string `json:"kind"`

	// QR is a PNG image, base64-encoded, to scan with the service's phone
	// app. Set only for the "qr" kind.
	QR string `json:"qr,omitempty"`

	// Hint explains the step, for example where the code was sent.
	Hint string `json:"hint,omitempty"`
}

// Sink receives normalized updates from a running Connector. It has one
// method per kind of update so that connectors stay free of any protocol
// or storage detail.
type Sink interface {
	// AccountStatus reports a change in the account's connection state.
	AccountStatus(ctx context.Context, accountID, status, detail string)

	// Contact reports a contact, new or renamed.
	Contact(ctx context.Context, c domain.Contact)

	// Conversation reports a conversation, new or changed.
	Conversation(ctx context.Context, c domain.Conversation)

	// Incoming reports a live message, which may notify the user.
	Incoming(ctx context.Context, accountID, conversationRemoteID string, m domain.Message)

	// History reports an earlier message, which never notifies.
	History(ctx context.Context, accountID, conversationRemoteID string, m domain.Message)

	// Edited reports a message changed after it was sent: its new text,
	// media and reactions. It never notifies.
	Edited(ctx context.Context, accountID, conversationRemoteID string, m domain.Message)

	// Reacted reports a message's reaction chips changing on their own,
	// without a full edit, such as someone reacting live. It never
	// notifies.
	Reacted(ctx context.Context, accountID, conversationRemoteID, messageRemoteID string, reactions []domain.Reaction)

	// Deleted reports messages removed from the service, named by the
	// ids it gave them. conversationRemoteIDs lists every conversation
	// the ids might belong to: a connector whose service gives message
	// ids that are unique per account rather than per conversation lists
	// every such conversation it knows, since only it knows which of its
	// conversations share that numbering. It never notifies.
	Deleted(ctx context.Context, accountID string, conversationRemoteIDs, remoteIDs []string)

	// OutgoingStatus reports delivery progress of a message we sent, with
	// the service's id for it once known.
	OutgoingStatus(ctx context.Context, localMessageID, remoteID, status string)

	// Typing reports that someone started or stopped typing.
	Typing(ctx context.Context, accountID, conversationRemoteID, name string, active bool)

	// Unread reports the service's own count of unread messages in a
	// conversation, which replaces ours: after a sync, or when the user
	// read it on another device.
	Unread(ctx context.Context, accountID, conversationRemoteID string, count int)

	// Organized reports a conversation's pinned and archived state, as a
	// dialog sync knows it. Only a full sync of the service's chat list
	// carries this; a conversation reported some other way, such as one
	// a live message just started, is left as it was.
	Organized(ctx context.Context, accountID, conversationRemoteID string, pinned, archived bool)

	// AuthStep reports what a signing-in connector needs from the user.
	AuthStep(ctx context.Context, accountID string, step AuthStep)
}
