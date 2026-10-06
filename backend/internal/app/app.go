// Package app holds what OmaMessenger does with messages, independent of the
// UI protocol and of any messaging service. Commands serves the UI; Ingest
// receives updates from connectors. Both publish events for the UI.
package app

import (
	"context"
	"errors"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// ErrInvalidInput reports a request the user can fix, such as empty text.
// Its message is safe to show.
var ErrInvalidInput = errors.New("invalid input")

// ErrNoFake reports an injection asked of a helper built without the fake
// connectors.
var ErrNoFake = errors.New("no fake connectors in this build")

// Dispatcher hands outgoing work to the connector that owns the account.
type Dispatcher interface {
	Send(ctx context.Context, conv domain.Conversation, m domain.Message) error
	MarkRead(ctx context.Context, conv domain.Conversation) error
}

// SignIn hands sign-in input, such as a code, to an account's connector.
type SignIn interface {
	SubmitAuth(ctx context.Context, accountID, step, value string) error
}

// NewAccount is what adding an account needs: the service, and for
// Telegram the API id and hash from my.telegram.org.
type NewAccount struct {
	Service string
	APIID   int
	APIHash string
}

// Accounts adds and removes accounts, starting or stopping their
// connectors and keeping or deleting their sessions.
type Accounts interface {
	Add(ctx context.Context, a NewAccount) (domain.Account, error)
	Remove(ctx context.Context, accountID string) error
}

// Notifier shows a desktop notification.
type Notifier interface {
	Notify(title, body string)
}

// Publisher sends an event to the UI.
type Publisher interface {
	Publish(ctx context.Context, event string, data any)
}

// Injector delivers a scripted message, as if someone had sent it. Only
// the fake connectors in test builds provide one.
type Injector interface {
	Inject(ctx context.Context, conversationRemoteID string) (domain.Message, error)
}

// Deps are the application's dependencies. Fake is nil outside test builds.
type Deps struct {
	Store      *store.Store
	Dispatcher Dispatcher
	Notifier   Notifier
	Publisher  Publisher
	SignIn     SignIn
	Accounts   Accounts
	Fake       Injector
}

// New builds the two halves of the application, which share the event
// stream and the user's focus and settings.
func New(d Deps) (*Commands, *Ingest) {
	events := &events{store: d.Store, out: d.Publisher}
	state := &uiState{settings: DefaultSettings()}

	commands := &Commands{
		store: d.Store, dispatcher: d.Dispatcher, signIn: d.SignIn, accounts: d.Accounts, fake: d.Fake,
		events: events, ui: state,
	}
	ingest := &Ingest{store: d.Store, notifier: d.Notifier, events: events, ui: state}

	return commands, ingest
}

// Settings are the user's preferences from the Omarchy plugin settings.
type Settings struct {
	Notifications       bool
	NotificationPreview bool
}

// DefaultSettings apply until the UI sends the user's settings.
func DefaultSettings() Settings {
	return Settings{Notifications: true, NotificationPreview: true}
}

// uiState is what the user is doing in the UI: their settings and which
// conversation, if any, they are looking at.
type uiState struct {
	mu           sync.RWMutex
	settings     Settings
	focused      string
	windowActive bool
}

// setFocus records the open conversation and whether the window is active.
func (s *uiState) setFocus(conversationID string, windowActive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.focused, s.windowActive = conversationID, windowActive
}

// apply replaces the settings.
func (s *uiState) apply(settings Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = settings
}

// snapshot returns the settings, the focused conversation and whether the
// window is active.
func (s *uiState) snapshot() (Settings, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.settings, s.focused, s.windowActive
}
