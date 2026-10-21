package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/spell"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// Commands are the actions the UI can take.
type Commands struct {
	store        *store.Store
	dispatcher   Dispatcher
	signIn       SignIn
	accounts     Accounts
	history      HistoryLoader
	media        MediaFetcher
	cache        MediaCache
	refresher    MessageRefresher
	organizer    Organizer
	reactor      Reactor
	voter        Voter
	deleter      Deleter
	editor       Editor
	members      MemberLister
	outgoing     OutgoingMedia
	clipboard    ClipboardRunner
	fake         Injector
	logger       Logger
	spell        *spell.Checker
	events       *events
	ui           *uiState
	ingest       *Ingest
	refreshed    *attemptedRefresh
	recentErrors *errorHistory
	reminders    *reminders
	retries      *retrier

	// dataDir, dbPath, execName and helperVersion are read only by
	// Doctor; see Deps.
	dataDir       string
	dbPath        string
	execName      string
	helperVersion string
}

// attemptedRefresh is the set of message ids this helper run has already
// asked the service to refresh, so scrolling back to the same page does
// not ask again.
type attemptedRefresh struct {
	mu   sync.Mutex
	done map[string]bool
}

// take marks each of ids as attempted and returns the ones that were not
// already, in the same order.
func (a *attemptedRefresh) take(ids []string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	fresh := make([]string, 0, len(ids))
	for _, id := range ids {
		if a.done[id] {
			continue
		}

		a.done[id] = true
		fresh = append(fresh, id)
	}

	return fresh
}

// Faked reports whether the helper runs the fake connectors of a test
// build.
func (c *Commands) Faked() bool {
	return c.fake != nil
}

// UnreadTotal counts unread messages outside muted conversations.
func (c *Commands) UnreadTotal(ctx context.Context) int {
	return c.events.unreadTotal(ctx)
}

// UnreadByService counts unread messages outside muted conversations,
// grouped by service, for the hello response's per-service breakdown.
func (c *Commands) UnreadByService(ctx context.Context) map[string]int {
	return c.events.unreadChanged(ctx, 0).ByService
}

// RunReminders fires every snoozed conversation's reminder at its due
// time, until ctx is cancelled. main.go starts it as its own goroutine
// once, alongside the connectors.
func (c *Commands) RunReminders(ctx context.Context) {
	c.reminders.run(ctx)
}

// RunRetries retries every failed outgoing message automatically, until
// ctx is cancelled. main.go starts it as its own goroutine once,
// alongside RunReminders.
func (c *Commands) RunRetries(ctx context.Context) {
	c.retries.run(ctx)
}

// Accounts lists the signed-in accounts.
func (c *Commands) Accounts(ctx context.Context) ([]domain.Account, error) {
	return c.store.Accounts(ctx)
}

// Contacts lists an account's contacts whose name contains query.
func (c *Commands) Contacts(ctx context.Context, accountID, query string) ([]domain.Contact, error) {
	if accountID == "" {
		return nil, fmt.Errorf("%w: accountId is required", ErrInvalidInput)
	}

	if _, err := c.store.Account(ctx, accountID); err != nil {
		return nil, err
	}

	return c.store.Contacts(ctx, accountID, query)
}

// SetFocus records which conversation the user is looking at, which
// decides whether arriving messages are read at once or notify. Landing
// on conv with the window active also clears whatever was already
// unread in it (see Ingest.Looked): Incoming only catches a message that
// arrives after this call, not a backlog left by switching chats, the
// window regaining focus, or the helper itself having just restarted
// while the UI still considered conv open.
func (c *Commands) SetFocus(ctx context.Context, conversationID string, windowActive bool) error {
	if conversationID == "" {
		c.ui.setFocus(conversationID, windowActive)
		return nil
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return err
	}

	c.ui.setFocus(conversationID, windowActive)
	if windowActive {
		c.ingest.Looked(ctx, conv)
	}

	return nil
}

// ApplySettings replaces the user's settings.
func (c *Commands) ApplySettings(s Settings) {
	c.ui.apply(s)

	if c.spell != nil {
		c.spell.SetEnabled(s.SpellCheck)
		c.spell.SetLanguage(s.SpellLanguage)
	}
}

// CheckSpelling finds misspelled words in text; see spell.Checker.Check.
// It returns nil without error when no spell checker was wired in, so a
// test build that does not need one is not forced to provide one.
func (c *Commands) CheckSpelling(ctx context.Context, text string) ([]spell.Misspelling, error) {
	if c.spell == nil {
		return nil, nil
	}

	return c.spell.Check(ctx, text)
}

// SuggestSpelling returns replacement words for a misspelled word; see
// spell.Checker.Suggest.
func (c *Commands) SuggestSpelling(ctx context.Context, word string) ([]string, error) {
	if c.spell == nil {
		return nil, nil
	}

	return c.spell.Suggest(ctx, word)
}

// AddSpellingWord adds word to the user's personal dictionary; see
// spell.Checker.AddWord.
func (c *Commands) AddSpellingWord(ctx context.Context, word string) error {
	if word == "" {
		return fmt.Errorf("%w: word is required", ErrInvalidInput)
	}

	if c.spell == nil {
		return nil
	}

	return c.spell.AddWord(ctx, word)
}

// Inject asks the fake connectors to deliver a message into a conversation
// now, and returns it as stored.
func (c *Commands) Inject(ctx context.Context, conversationID string) (domain.Message, error) {
	if c.fake == nil {
		return domain.Message{}, ErrNoFake
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Message{}, err
	}

	m, err := c.fake.Inject(ctx, conv.RemoteID)
	if err != nil {
		return m, err
	}

	return c.store.MessageByRemote(ctx, conv.ID, m.RemoteID)
}
