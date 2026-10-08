// Package app holds what OmaMessenger does with messages, independent of the
// UI protocol and of any messaging service. Commands serves the UI; Ingest
// receives updates from connectors. Both publish events for the UI.
package app

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/app/policy"
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

// Reactor sets or clears the signed-in user's own reaction to a message,
// through its service.
type Reactor interface {
	React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error
}

// Deleter deletes messages from a conversation, through its service.
type Deleter interface {
	DeleteMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string, forEveryone bool) error
}

// MediaCache keeps downloaded media, filling a file the first time it is
// asked for.
type MediaCache interface {
	Fetch(ctx context.Context, name string, fill func(ctx context.Context, path string) error) (string, error)
}

// OutgoingMedia stores the bytes of a file the user is sending into the
// outgoing media area, named by the message's id, so a retry can resend
// it even after the user moves, renames or deletes the original. Path is
// pure and needs no context; keeping it alongside Store, rather than
// splitting it into its own one-method interface, keeps the one thing a
// caller needs for an attachment, store it and find it again, in one
// place.
type OutgoingMedia interface {
	Store(ctx context.Context, id, fileName string, r io.Reader) (string, error)
	Path(id, fileName string) string
}

// ClipboardRunner reads the Wayland clipboard. Quickshell's QML cannot
// read clipboard image data itself, only ask whether a paste happened, so
// the helper shells out to wl-paste; this is what PasteImage asks of it.
type ClipboardRunner interface {
	// Types lists the MIME types the clipboard currently offers.
	Types(ctx context.Context) ([]string, error)

	// Read writes the clipboard's data of mimeType to w.
	Read(ctx context.Context, mimeType string, w io.Writer) error
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

// Logger writes one diagnostic line to the helper's stderr. *log.Logger
// already satisfies it, so main.go wires the same logger the server
// uses, with nothing special to construct.
type Logger interface {
	Printf(format string, v ...any)
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
	Reactor    Reactor
	Deleter    Deleter
	Outgoing   OutgoingMedia
	Clipboard  ClipboardRunner
	Fake       Injector
	Logger     Logger
}

// New builds the two halves of the application, which share the event
// stream and the user's focus and settings.
func New(d Deps) (*Commands, *Ingest) {
	events := &events{store: d.Store, out: d.Publisher}
	state := &uiState{settings: DefaultSettings()}

	commands := &Commands{
		store: d.Store, dispatcher: d.Dispatcher, signIn: d.SignIn, accounts: d.Accounts, history: d.History, media: d.Media, cache: d.Cache, refresher: d.Refresher, organizer: d.Organizer, reactor: d.Reactor, deleter: d.Deleter, fake: d.Fake,
		outgoing: d.Outgoing, clipboard: d.Clipboard, logger: d.Logger,
		events: events, ui: state, refreshed: &attemptedRefresh{done: map[string]bool{}},
	}
	ingest := &Ingest{store: d.Store, notifier: d.Notifier, dispatcher: d.Dispatcher, events: events, ui: state}

	return commands, ingest
}

// Settings are the user's preferences from the Omarchy plugin settings.
// NotificationDetail holds one of policy.Detail's values; an older UI that
// still sends only NotificationPreview is handled by detail below, so the
// setting stays additive rather than breaking that UI.
type Settings struct {
	Notifications       bool
	NotificationPreview bool
	NotificationDetail  string
}

// DefaultSettings apply until the UI sends the user's settings.
func DefaultSettings() Settings {
	return Settings{Notifications: true, NotificationPreview: true, NotificationDetail: string(policy.DetailNameAndMessage)}
}

// detail resolves the notification detail level these settings ask for:
// NotificationDetail when it names one of the three levels, otherwise the
// older NotificationPreview boolean, so a UI built before the three-level
// setting existed still gets the detail it asked for.
func (s Settings) detail() policy.Detail {
	switch d := policy.Detail(s.NotificationDetail); d {
	case policy.DetailNameAndMessage, policy.DetailNameOnly, policy.DetailNone:
		return d
	}

	if s.NotificationPreview {
		return policy.DetailNameAndMessage
	}

	return policy.DetailNameOnly
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
