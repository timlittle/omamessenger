package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// authSteps are the answers a connector can ask for during sign-in.
var authSteps = []string{"phone", "code", "password"}

// AddAccount adds an account and starts signing it in; the connector then
// reports what it needs through auth.step events. What counts as valid
// setup, such as Telegram's optional API id and hash, is up to the
// service's own provider.
func (c *Commands) AddAccount(ctx context.Context, a NewAccount) (domain.Account, error) {
	account, err := c.accounts.Add(ctx, a)
	if err != nil {
		return domain.Account{}, addAccountError(err)
	}

	c.events.publish(ctx, EventAccountUpdated, account)

	return account, nil
}

// addAccountError turns a registry failure the user can fix into
// ErrInvalidInput with a safe message; anything else, such as a database
// failure, is returned unchanged so it becomes a fixed internal error.
func addAccountError(err error) error {
	switch {
	case errors.Is(err, connector.ErrUnknownProvider):
		return fmt.Errorf("%w: this messaging service is not available", ErrInvalidInput)
	case errors.Is(err, connector.ErrInvalidSetup):
		return fmt.Errorf("%w: enter both the API id and the API hash from my.telegram.org", ErrInvalidInput)
	default:
		return err
	}
}

// Services lists the messaging services available to add, in the order
// their providers were registered, or none when the given Accounts
// cannot list them.
func (c *Commands) Services() []domain.Service {
	lister, ok := c.accounts.(ServiceLister)
	if !ok {
		return nil
	}

	return lister.Services()
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
