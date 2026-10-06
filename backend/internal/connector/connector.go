// Package connector is the boundary between messaging services and the
// application. A Connector speaks one service's protocol and reports
// normalized updates to a Sink; the Manager runs and supervises connectors.
package connector

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

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

// Authenticator is a Connector that signs in to its service. While it
// waits for the user it reports what it needs with Sink.AuthStep, and
// SubmitAuth delivers the answer.
type Authenticator interface {
	// SubmitAuth answers the step the connector asked for: "phone",
	// "code" or "password".
	SubmitAuth(ctx context.Context, step, value string) error
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

	// OutgoingStatus reports delivery progress of a message we sent, with
	// the service's id for it once known.
	OutgoingStatus(ctx context.Context, localMessageID, remoteID, status string)

	// Typing reports that someone started or stopped typing.
	Typing(ctx context.Context, accountID, conversationRemoteID, name string, active bool)

	// AuthStep reports what a signing-in connector needs from the user.
	AuthStep(ctx context.Context, accountID string, step AuthStep)
}
