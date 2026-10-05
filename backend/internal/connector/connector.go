// Package connector defines the provider-neutral boundary between messaging
// services and OmaMessenger's application layer.
package connector

import (
	"context"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Clock makes connector timing deterministic in tests.
type Clock interface {
	Now() time.Time
	AfterFunc(time.Duration, func()) (stop func() bool)
}

// Sink receives normalized account and messaging updates from a Connector.
type Sink interface {
	AccountStatus(accountID, status, detail string)
	Contact(c domain.Contact)
	Conversation(c domain.Conversation)
	Incoming(accountID, conversationRemoteID string, m domain.Message)
	OutgoingStatus(localMessageID, remoteID, status string)
	Typing(accountID, conversationRemoteID, name string, active bool)
}

// HistorySink is an optional extension used for initial/backfilled messages.
// Implementations persist messages without applying live-notification policy.
type HistorySink interface {
	History(accountID, conversationRemoteID string, m domain.Message)
}

// Connector adapts one authenticated messaging account to the normalized
// domain. Run blocks until its context is canceled or the connection fails.
type Connector interface {
	Account() domain.Account
	Run(ctx context.Context, sink Sink) error
	Send(ctx context.Context, conv domain.Conversation, m domain.Message) error
	MarkRead(ctx context.Context, conv domain.Conversation) error
}
