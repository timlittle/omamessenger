// resolve.go learns a placeholder peer's real access hash, in the
// background when a placeholder is first created and synchronously
// when a send needs one still unresolved, so a stranger's first
// message never ends up stuck as an unreplyable duplicate (see
// docs/decisions.md).

package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// placeholderResolveTimeout bounds resolvePlaceholderHash's background
// lookup, so a slow or unreachable Telegram can never leave its
// goroutine running past a few seconds (see Run's own wait on
// c.resolving).
const placeholderResolveTimeout = 10 * time.Second

// placeholderResolveDialogs is how many of the account's most recent
// dialogs findUserAccessHash fetches: see docs/decisions.md for why
// this is enough to find a peer that only just started a chat.
const placeholderResolveDialogs = 20

// resolvePlaceholderHash looks up userID's real access hash in the
// background, tracked by c.resolving so Run can wait for it, and once
// found reports the upgrade from the placeholder remote id to the
// resolved one through sink. ctx is the connector's run context, not
// the update that created the placeholder, since this must outlive it.
func (c *Connector) resolvePlaceholderHash(ctx context.Context, sink connector.Sink, userID int64) {
	c.resolving.Add(1)
	go func() {
		defer c.resolving.Done()

		lookupCtx, cancel := context.WithTimeout(ctx, placeholderResolveTimeout)
		defer cancel()

		api, _, err := c.session()
		if err != nil {
			return
		}

		hash, ok := findUserAccessHash(lookupCtx, api, userID)
		if !ok {
			return
		}

		c.learnResolved(ctx, sink, remoteID(&tg.InputPeerUser{UserID: userID, AccessHash: hash}))
	}()
}

// findUserAccessHash looks up userID's current access hash among the
// account's most recent dialogs, or reports false if it is not among
// them (see docs/decisions.md for why this call).
func findUserAccessHash(ctx context.Context, api *tg.Client, userID int64) (int64, bool) {
	result, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{}, Limit: placeholderResolveDialogs,
	})
	if err != nil {
		return 0, false
	}

	dialogs, ok := result.AsModified()
	if !ok {
		return 0, false
	}

	for _, u := range dialogs.GetUsers() {
		if user, ok := u.(*tg.User); ok && user.ID == userID {
			return user.AccessHash, true
		}
	}

	return 0, false
}

// errUnresolvedPeer reports a send refused because this conversation's
// user still has no known access hash: Telegram requires one for a
// private chat, and sending with a guessed or zero hash risks either
// an outright refusal or, if that number happens to resolve to
// something, delivering to the wrong account entirely.
var errUnresolvedPeer = errors.New("telegram: cannot send yet, still identifying this chat")

// resolveSendPeer reads convRemoteID into the peer and remote id Send
// needs, resolving a user placeholder's unknown access hash first.
// Once found, the hash is learned and the upgrade over convRemoteID
// reported unconditionally, since Send can be the first thing to
// touch this peer after a restart with nothing cached. Returns
// errUnresolvedPeer rather than sending with a guessed or zero hash.
func (c *Connector) resolveSendPeer(ctx context.Context, sink connector.Sink, api *tg.Client, convRemoteID string) (tg.InputPeerClass, string, error) {
	peer, err := inputPeer(convRemoteID)
	if err != nil {
		return nil, "", err
	}

	user, ok := peer.(*tg.InputPeerUser)
	if !ok || user.AccessHash != 0 {
		return peer, convRemoteID, nil
	}

	hash, ok := findUserAccessHash(ctx, api, user.UserID)
	if !ok {
		return nil, "", fmt.Errorf("telegram: send: %w", errUnresolvedPeer)
	}

	resolved := &tg.InputPeerUser{UserID: user.UserID, AccessHash: hash}
	resolvedRemote := remoteID(resolved)
	c.learn(resolvedRemote)
	c.reportUpgrade(ctx, sink, convRemoteID, resolvedRemote)

	return resolved, resolvedRemote, nil
}
