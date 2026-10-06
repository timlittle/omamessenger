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

// ErrNotDemo reports a demo-only request made outside demo mode.
var ErrNotDemo = errors.New("only available in demo mode")

// Dispatcher hands outgoing work to the connector that owns the account.
type Dispatcher interface {
	Send(ctx context.Context, conv domain.Conversation, m domain.Message) error
	MarkRead(ctx context.Context, conv domain.Conversation) error
}

// Notifier shows a desktop notification.
type Notifier interface {
	Notify(title, body string)
}

// Publisher sends an event to the UI.
type Publisher interface {
	Publish(ctx context.Context, event string, data any)
}

// Demo controls the scripted demo connectors.
type Demo interface {
	Inject(ctx context.Context, conversationRemoteID string) (domain.Message, error)
	SetChatter(enabled bool)
}

// Deps are the application's dependencies. Demo is nil outside demo mode.
type Deps struct {
	Store      *store.Store
	Dispatcher Dispatcher
	Notifier   Notifier
	Publisher  Publisher
	Demo       Demo
}

// New builds the two halves of the application, which share the event
// stream and the user's focus and settings.
func New(d Deps) (*Commands, *Ingest) {
	events := &events{store: d.Store, out: d.Publisher}
	state := &uiState{settings: DefaultSettings()}

	commands := &Commands{
		store: d.Store, dispatcher: d.Dispatcher, demo: d.Demo,
		events: events, ui: state,
	}
	ingest := &Ingest{store: d.Store, notifier: d.Notifier, events: events, ui: state}

	return commands, ingest
}

// Settings are the user's preferences from the Omarchy plugin settings.
type Settings struct {
	Notifications       bool
	NotificationPreview bool
	DemoChatter         bool
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
