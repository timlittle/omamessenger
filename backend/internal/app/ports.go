package app

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Repository is the persistence the application needs. It is declared here,
// by its consumer; *store.Store satisfies it.
type Repository interface {
	Accounts() ([]domain.Account, error)
	Account(id string) (domain.Account, error)
	SetAccountStatus(id, status, detail string) (domain.Account, error)
	UpsertContact(domain.Contact) error
	Contact(accountID, remoteID string) (domain.Contact, error)
	Contacts(accountID, query string) ([]domain.Contact, error)
	EnsureConversation(domain.Conversation) (domain.Conversation, bool, error)
	Conversation(id string) (domain.Conversation, error)
	ConversationByRemote(accountID, remoteID string) (domain.Conversation, error)
	Conversations(query string) ([]domain.Conversation, error)
	MarkRead(id string) (bool, error)
	SetMuted(id string, muted bool) error
	UnreadTotal() (int, error)
	AddMessage(domain.Message) (domain.Message, bool, error)
	Message(id string) (domain.Message, error)
	MessageByRemote(conversationID, remoteID string) (domain.Message, error)
	SetMessageRemoteID(id, remoteID string) error
	UpdateMessageStatus(id, status string) (domain.Message, bool, error)
	Messages(conversationID, beforeID string, limit int) ([]domain.Message, bool, error)
}

// Dispatcher hands outgoing work to the connector that owns the account;
// *connector.Manager satisfies it.
type Dispatcher interface {
	Send(ctx context.Context, conv domain.Conversation, m domain.Message) error
	MarkRead(ctx context.Context, conv domain.Conversation) error
}

// Notifier raises a desktop notification.
type Notifier interface {
	Notify(title, body string)
}

// Emit publishes one protocol event to the UI.
type Emit func(name string, data any)

// DemoInjector is supplied when a demo connector is configured.
type DemoInjector interface {
	Inject(conversationRemoteID string) (domain.Message, error)
}
