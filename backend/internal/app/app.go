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

// HistoryLoader fetches a conversation's older history from its service,
// storing it as it arrives, and says how many messages it found.
type HistoryLoader interface {
	LoadOlder(ctx context.Context, conv domain.Conversation, beforeRemoteID string, limit int) (int, error)
}

// MediaFetcher downloads a message's photo, video or file from its
// service.
type MediaFetcher interface {
	FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string) error
}

// MessageRefresher re-reports messages already stored, so the store can
// fill in media or a link preview a message had none of when it was
// first synced.
type MessageRefresher interface {
	RefreshMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string) error
}

// Organizer keeps a conversation's pinned and archived state in step with
// its service.
type Organizer interface {
	SetPinned(ctx context.Context, conv domain.Conversation, pinned bool) error
	SetArchived(ctx context.Context, conv domain.Conversation, archived bool) error
}

// MediaCache keeps downloaded media, filling a file the first time it is
// asked for.
type MediaCache interface {
	Fetch(ctx context.Context, name string, fill func(ctx context.Context, path string) error) (string, error)
}

// SignIn hands sign-in input, such as a code, to an account's connector.
type SignIn interface {
	SubmitAuth(ctx context.Context, accountID, step, value string) error
}

// NewAccount is what adding an account needs: the service, and whatever
// options its provider's Prepare wants, such as Telegram's "apiId" and
// "apiHash" from my.telegram.org.
type NewAccount struct {
	Service string
	Options map[string]string
}

// Accounts adds and removes accounts, starting or stopping their
// connectors and keeping or deleting their sessions.
type Accounts interface {
	Add(ctx context.Context, a NewAccount) (domain.Account, error)
	Remove(ctx context.Context, accountID string) error
}

// ServiceLister lists the messaging services a provider is registered
// for, so the UI can offer them when adding an account. The accounts
// registry implements it; Commands.Services returns none when the given
// Accounts does not.
type ServiceLister interface {
	Services() []domain.Service
}

// Notifier shows a desktop notification for a conversation.
type Notifier interface {
	Notify(title, body, conversationID string)
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
	History    HistoryLoader
	Media      MediaFetcher
	Cache      MediaCache
	Refresher  MessageRefresher
	Organizer  Organizer
	Fake       Injector
}

// New builds the two halves of the application, which share the event
// stream and the user's focus and settings.
func New(d Deps) (*Commands, *Ingest) {
	events := &events{store: d.Store, out: d.Publisher}
	state := &uiState{settings: DefaultSettings()}

	commands := &Commands{
		store: d.Store, dispatcher: d.Dispatcher, signIn: d.SignIn, accounts: d.Accounts, history: d.History, media: d.Media, cache: d.Cache, refresher: d.Refresher, organizer: d.Organizer, fake: d.Fake,
		events: events, ui: state, refreshed: &attemptedRefresh{done: map[string]bool{}},
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
