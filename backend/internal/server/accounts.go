package server

// accounts.go handles accounts.list, accounts.add, accounts.remove and accounts.setAlias.

import (
	"context"
	"maps"
	"strconv"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// accountsList lists the signed-in accounts with their connector's current attachment limit.
func accountsList(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		accounts, err := c.Accounts(ctx)
		if err != nil {
			return nil, err
		}

		results := make([]app.AccountView, len(accounts))
		for i, a := range accounts {
			results[i] = c.ViewAccount(a)
		}

		return results, nil
	}
}

// addAccountParams names the service to add an account for. apiId and apiHash
// hold Telegram's own API keys; options carries other providers' setup fields.
type addAccountParams struct {
	Service string            `json:"service"`
	APIID   int               `json:"apiId"`
	APIHash string            `json:"apiHash"`
	Options map[string]string `json:"options"`
}

// accountsAdd adds an account and starts signing it in.
func accountsAdd(c *app.Commands) func(context.Context, addAccountParams) (any, error) {
	return func(ctx context.Context, p addAccountParams) (any, error) {
		return c.AddAccount(ctx, app.NewAccount{Service: p.Service, Options: setupOptions(p)})
	}
}

// setupOptions merges a request's options object with its apiId and apiHash fields.
func setupOptions(p addAccountParams) map[string]string {
	options := maps.Clone(p.Options)
	if p.APIID == 0 && p.APIHash == "" {
		return options
	}

	if options == nil {
		options = map[string]string{}
	}
	if p.APIID != 0 {
		options["apiId"] = strconv.Itoa(p.APIID)
	}
	if p.APIHash != "" {
		options["apiHash"] = p.APIHash
	}

	return options
}

// accountParams names one account.
type accountParams struct {
	AccountID string `json:"accountId"`
}

// accountsRemove signs an account out and deletes it.
func accountsRemove(c *app.Commands) func(context.Context, accountParams) (any, error) {
	return func(ctx context.Context, p accountParams) (any, error) {
		return none{}, c.RemoveAccount(ctx, p.AccountID)
	}
}

// setAliasParams names an account and the alias to give it; an empty
// alias clears it.
type setAliasParams struct {
	AccountID string `json:"accountId"`
	Alias     string `json:"alias"`
}

// accountsSetAlias saves the display name the user chose for an account.
func accountsSetAlias(c *app.Commands) func(context.Context, setAliasParams) (any, error) {
	return func(ctx context.Context, p setAliasParams) (any, error) {
		return c.SetAccountAlias(ctx, p.AccountID, p.Alias)
	}
}
