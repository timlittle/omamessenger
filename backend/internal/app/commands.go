package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// Commands are the actions the UI can take.
type Commands struct {
	store      *store.Store
	dispatcher Dispatcher
	signIn     SignIn
	accounts   Accounts
	history    HistoryLoader
	media      MediaFetcher
	cache      MediaCache
	refresher  MessageRefresher
	organizer  Organizer
	reactor    Reactor
	deleter    Deleter
	members    MemberLister
	outgoing   OutgoingMedia
	clipboard  ClipboardRunner
	fake       Injector
	logger     Logger
	events     *events
	ui         *uiState
	refreshed  *attemptedRefresh
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

// SetFocus records which conversation the user is looking at, which decides
// whether arriving messages are read at once or notify.
func (c *Commands) SetFocus(ctx context.Context, conversationID string, windowActive bool) error {
	if conversationID != "" {
		if _, err := c.store.Conversation(ctx, conversationID); err != nil {
			return err
		}
	}

	c.ui.setFocus(conversationID, windowActive)

	return nil
}

// ApplySettings replaces the user's settings.
func (c *Commands) ApplySettings(s Settings) {
	c.ui.apply(s)
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
