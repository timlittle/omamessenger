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
	"github.com/timlittle/omamessenger/backend/internal/portal"
	"github.com/timlittle/omamessenger/backend/internal/spell"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// ErrInvalidInput reports a request the user can fix, such as empty text.
// Its message is safe to show.
var ErrInvalidInput = errors.New("invalid input")

// ErrNoFake reports an injection asked of a helper built without the fake
// connectors.
var ErrNoFake = errors.New("no fake connectors in this build")

// Dispatcher hands outgoing work to the connector that owns the account.
// Send and MarkRead are deliberately the only two ways anything here can
// tell a service about this account's own activity: there is no method
// to report typing, presence or a voice note played, because neither
// connector ever sends any of those in the first place (see
// docs/privacy.md). MarkRead is the only one Settings.ReadReceipts
// gates, and it is the only one there is to gate.
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
// service. auto is true for a fetch the UI made on its own, with no
// click from the user - see Commands.FetchMedia.
type MediaFetcher interface {
	FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string, auto bool) error
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

// ArchiveUnpinner reports whether archiving a conversation also unpins
// it with the account's own service, so SetArchived knows to unpin it
// locally in the same call too, rather than only once the service's own
// round trip reports it back - a correction that never arrives at all
// if that call fails (see docs/decisions.md).
type ArchiveUnpinner interface {
	ArchiveUnpins(accountID string) bool
}

// Reactor sets or clears the signed-in user's own reaction to a message,
// through its service.
type Reactor interface {
	React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error
}

// Voter casts the signed-in user's vote in a poll, through its service.
type Voter interface {
	Vote(ctx context.Context, conv domain.Conversation, messageRemoteID string, optionIDs []string) error
}

// Deleter deletes messages from a conversation, through its service.
type Deleter interface {
	DeleteMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string, forEveryone bool) error
}

// Editor changes the text of a message this account already sent,
// through its service.
type Editor interface {
	EditMessage(ctx context.Context, conv domain.Conversation, messageRemoteID, newText string, mentions []domain.Mention) error
}

// Blocker blocks or unblocks a direct conversation's contact, through
// its service.
type Blocker interface {
	SetBlocked(ctx context.Context, conv domain.Conversation, blocked bool) error
}

// SpamReporter reports a conversation as spam or abuse, through its
// service, when the service offers one; a connector without one is
// asked nothing, since Commands.ReportSpam blocks the contact either
// way.
type SpamReporter interface {
	ReportSpam(ctx context.Context, conv domain.Conversation) error
}

// MemberLister lists a group conversation's current members, through
// its service, for the @-mention picker.
type MemberLister interface {
	Members(ctx context.Context, conv domain.Conversation) ([]domain.Member, error)
}

// AttachmentLimiter reports the largest attachment an account's
// connector currently accepts, through its service, when it knows one
// narrower than MaxAttachmentSize, the backstop that applies
// regardless.
type AttachmentLimiter interface {
	// AttachmentLimit is the limit accountID's connector reports for
	// kind ("photo", "video" or "file"), and whether it reported one
	// at all; false means there is no connector running for the
	// account right now, or its connector has none narrower than
	// MaxAttachmentSize.
	AttachmentLimit(accountID, kind string) (int64, bool)
}

// MediaCache keeps downloaded media, filling a file the first time it is
// asked for, or taking in one already on disk - a sent attachment, say -
// so it needs no download at all. Three methods, not the usual one or
// two (see interfaces.md): FetchNamed is Fetch's own counterpart for a
// kind whose correct cache name can only be known once its bytes
// exist - every kind but a photo or video, named from a verified MIME
// type sniffed after downloading rather than a sender's own claim (see
// fetchIntoCache and docs/decisions.md) - and genuinely differs in when
// the final name is decided, not merely in its parameters.
type MediaCache interface {
	Fetch(ctx context.Context, name string, fill func(ctx context.Context, path string) error) (string, error)
	FetchNamed(ctx context.Context, id string, fill func(ctx context.Context, path string) error, nameFor func(path string) (string, error)) (string, error)
	Adopt(ctx context.Context, name, path string) error
}

// OutgoingMedia stores the bytes of a file the user is sending into the
// outgoing media area, named by the message's id, so a retry can resend
// it even after the user moves, renames or deletes the original. Every
// consumer of that area needs to find, guard or drop one of its files by
// that same id and file name - Path for a retry, Reserve so a background
// sweep never removes a file mid-upload, Remove once delivery is
// confirmed and the copy is no longer needed - so they are kept in this
// one interface rather than split into several that would each mirror
// the same two arguments.
type OutgoingMedia interface {
	Store(ctx context.Context, id, fileName string, r io.Reader) (string, error)
	Path(id, fileName string) string
	Reserve(id string) func()
	Remove(ctx context.Context, id, fileName string) error
}

// SoundStore stores a validated copy of every custom notification sound
// this helper plays, one file per key: a conversation's own id, or a
// fixed name for a service's or the global default sound (see
// notificationsound.go). Every key always overwrites its own file in
// place, so a caller replacing one never needs to remove the old copy
// first.
type SoundStore interface {
	Store(ctx context.Context, key string, r io.Reader) (string, error)
	Path(key string) string
	Remove(ctx context.Context, key string) error
}

// ClipboardRunner reads and writes the Wayland clipboard. Quickshell's
// QML cannot read clipboard image data, or put an image or video on the
// clipboard at all, itself, so the helper shells out to wl-paste and
// wl-copy; PasteImage asks for the first, CopyMedia for the second.
// Three methods, not the usual one or two, because all three are the
// one real resource - the system clipboard - a single command-line
// pair already owns: splitting Read and Write into their own
// interfaces would only mean two fields wired to the same
// implementation for no caller's benefit.
type ClipboardRunner interface {
	// Types lists the MIME types the clipboard currently offers.
	Types(ctx context.Context) ([]string, error)

	// Read writes the clipboard's data of mimeType to w.
	Read(ctx context.Context, mimeType string, w io.Writer) error

	// Write puts data on the clipboard as mimeType.
	Write(ctx context.Context, mimeType string, data io.Reader) error
}

// SignIn hands sign-in input, such as a code, to an account's connector.
type SignIn interface {
	SubmitAuth(ctx context.Context, accountID, step, value string) error
}

// FilePicker opens a file-choosing dialog outside this process, so
// picking a file never runs GTK inside the shared Omarchy shell (see
// docs/decisions.md); *portal.Picker is the production implementation.
// Pick returns ok false with no error when the user cancelled the
// dialog, never confused with path "" being a real answer; err is
// returned only when the dialog itself could not be shown at all,
// such as no portal being reachable on the session bus.
type FilePicker interface {
	Pick(ctx context.Context, title string, filters []portal.Filter) (path string, ok bool, err error)
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
	Store           *store.Store
	Dispatcher      Dispatcher
	Notifier        Notifier
	Publisher       Publisher
	SignIn          SignIn
	Accounts        Accounts
	History         HistoryLoader
	Media           MediaFetcher
	Cache           MediaCache
	Refresher       MessageRefresher
	Organizer       Organizer
	ArchiveUnpinner ArchiveUnpinner
	Reactor         Reactor
	Voter           Voter
	Deleter         Deleter
	Editor          Editor
	Blocker         Blocker
	SpamReporter    SpamReporter
	Members         MemberLister
	Limiter         AttachmentLimiter
	Outgoing        OutgoingMedia
	Sounds          SoundStore
	Clipboard       ClipboardRunner
	FilePicker      FilePicker
	Fake            Injector
	Logger          Logger
	Spell           *spell.Checker

	// DataDir, DBPath, ExecutableName and HelperVersion are read only by
	// Doctor, to stat the data directory's and database's permissions
	// and recover which version the running binary was installed as.
	DataDir        string
	DBPath         string
	ExecutableName string
	HelperVersion  string

	// DownloadsDir is where SaveMedia copies a message's media to, at
	// the user's own request; empty when the helper could not resolve
	// one (no HOME set), in which case SaveMedia fails rather than
	// guessing a location.
	DownloadsDir string
}

// New builds the two halves of the application, which share the event
// stream and the user's focus and settings.
func New(d Deps) (*Commands, *Ingest) {
	events := &events{store: d.Store, out: d.Publisher}
	state := &uiState{settings: DefaultSettings()}
	retries := newRetrier(d.Store)
	scheduled := newScheduledSender(d.Store)

	commands := &Commands{
		store: d.Store, dispatcher: d.Dispatcher, signIn: d.SignIn, accounts: d.Accounts, history: d.History, media: d.Media, cache: d.Cache, refresher: d.Refresher, organizer: d.Organizer, archiveUnpinner: d.ArchiveUnpinner, reactor: d.Reactor, voter: d.Voter, deleter: d.Deleter, editor: d.Editor, blocker: d.Blocker, spamReporter: d.SpamReporter, members: d.Members, limiter: d.Limiter, fake: d.Fake,
		outgoing: d.Outgoing, sounds: d.Sounds, clipboard: d.Clipboard, filePicker: d.FilePicker, logger: d.Logger, spell: d.Spell,
		events: events, ui: state, refreshed: &attemptedRefresh{done: map[string]bool{}},
		recentErrors: &errorHistory{},
		reminders:    newReminders(d.Store, d.Notifier, events, state),
		retries:      retries,
		scheduled:    scheduled,
		dataDir:      d.DataDir, dbPath: d.DBPath, execName: d.ExecutableName, helperVersion: d.HelperVersion,
		downloadsDir: d.DownloadsDir,
	}
	retries.commands = commands
	scheduled.commands = commands

	// Spell checking starts at the same defaults as every other
	// setting, so it behaves sensibly even if the UI is slow to call
	// settings.apply after connecting; ApplySettings updates it again
	// once the UI's own settings arrive.
	if commands.spell != nil {
		commands.spell.SetEnabled(state.settings.SpellCheck)
		commands.spell.SetLanguage(state.settings.SpellLanguage)
	}

	ingest := &Ingest{
		store: d.Store, notifier: d.Notifier, dispatcher: d.Dispatcher, events: events, ui: state,
		outgoing: d.Outgoing, cache: d.Cache, logger: d.Logger, retries: retries, limiter: d.Limiter,
	}
	commands.ingest = ingest

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

	// ReadReceipts is false for incognito mode: MarkRead still clears the
	// local unread count, but neither Commands.MarkRead nor Ingest's
	// debounced read receipt ever reaches the dispatcher, so the
	// service, and any other device signed into the same account, keep
	// showing the chat unread.
	ReadReceipts bool

	// SpellCheck turns the composer's English spell check on or off.
	// SpellLanguage picks the dictionary: "auto" (from the locale
	// environment), "en_GB" or "en_US". See spell.Checker.
	SpellCheck    bool
	SpellLanguage string
}

// DefaultSettings apply until the UI sends the user's settings. A
// notification names no sender and shows no text by default: Omarchy's
// own notification service keeps a copy of each one where other local
// programs can read it (see docs/decisions.md).
func DefaultSettings() Settings {
	return Settings{
		Notifications: true, NotificationPreview: false, NotificationDetail: string(policy.DetailNone),
		ReadReceipts: true,
		SpellCheck:   true, SpellLanguage: "auto",
	}
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
