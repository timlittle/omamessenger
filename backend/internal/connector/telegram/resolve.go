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
// background, tracked by c.resolving so Run can wait for it before
// returning, and, once found, reports the upgrade from its
// placeholder remote id to the fully resolved one through sink (see
// learnResolved and connector.RemoteIDUpgrader). ctx is the
// connector's own run context, not the single update that created
// the placeholder: finding the user's access hash afterwards is not
// itself part of handling that update, and must keep going after it
// returns.
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
// account's most recent dialogs (see docs/decisions.md for why this
// call), or reports false when it is not among them. It needs no
// connector state of its own, only the API handed to it.
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
// needs, resolving a user placeholder's access hash first when it is
// still unknown: remoteID writes it as 0, which placeholderConversation
// is the only path that ever does for a user peer (see its own
// comment). The resolved hash, once found, is also learned and its
// upgrade over convRemoteID reported unconditionally, by its known
// placeholder shape rather than whatever this run's own cache happens
// to hold (the same reasoning as sync.go's own unconditional check):
// Send can be the first thing to touch this peer after a helper
// restart, with nothing cached yet, and the store's own copy of the
// conversation still needs correcting at the moment a send finally
// succeeds under it. A hash still not found returns errUnresolvedPeer
// rather than sending with one guessed or left at 0.
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
