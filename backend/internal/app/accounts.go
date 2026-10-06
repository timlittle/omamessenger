package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// authSteps are the answers a connector can ask for during sign-in.
var authSteps = []string{"phone", "code", "password"}

// AddAccount adds an account and starts signing it in; the connector then
// reports what it needs through auth.step events. Without an API id and
// hash, the account uses OmaMessenger's own Telegram app keys.
func (c *Commands) AddAccount(ctx context.Context, a NewAccount) (domain.Account, error) {
	if a.Service != domain.ServiceTelegram {
		return domain.Account{}, fmt.Errorf("%w: only Telegram accounts can be added so far", ErrInvalidInput)
	}

	ownKeys := a.APIID != 0 || a.APIHash != ""
	if ownKeys && (a.APIID <= 0 || strings.TrimSpace(a.APIHash) == "") {
		return domain.Account{}, fmt.Errorf("%w: enter both the API id and the API hash from my.telegram.org", ErrInvalidInput)
	}

	account, err := c.accounts.Add(ctx, a)
	if err != nil {
		return account, err
	}

	c.events.publish(ctx, EventAccountUpdated, account)

	return account, nil
}

// RemoveAccount signs an account out and deletes it with everything it
// holds.
func (c *Commands) RemoveAccount(ctx context.Context, accountID string) error {
	before := c.events.unreadTotal(ctx)
	if err := c.accounts.Remove(ctx, accountID); err != nil {
		return err
	}

	c.events.publish(ctx, EventAccountRemoved, AccountRemoved{AccountID: accountID})
	if after := c.events.unreadTotal(ctx); after != before {
		c.events.publish(ctx, EventUnreadChanged, UnreadChanged{Total: after})
	}

	return nil
}

// SubmitAuth answers the sign-in step an account's connector asked for.
func (c *Commands) SubmitAuth(ctx context.Context, accountID, step, value string) error {
	if !slices.Contains(authSteps, step) {
		return fmt.Errorf("%w: unknown sign-in step %q", ErrInvalidInput, step)
	}

	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: enter the %s first", ErrInvalidInput, step)
	}

	return c.signIn.SubmitAuth(ctx, accountID, step, strings.TrimSpace(value))
}
