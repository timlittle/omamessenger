package signal

// edit.go implements connector.MessageEditor: changing the text of a
// message this account already sent, by sending Signal's own edit
// protocol message (signalmeow.WrapEditMessage). Only a message this
// account sent, and only within Signal's own editing window, can be
// edited; both the sender and the original timestamp come straight out
// of the target message's own remote id (see parseMessageRemoteID), so
// no saved key is needed to check either.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.MessageEditor = (*Connector)(nil)

// editTimeout bounds how long EditMessage waits for Signal to accept
// one message's edit, matching this connector's other per-message
// calls.
const editTimeout = 30 * time.Second

// signalEditWindow is how long after sending a message Signal's own
// clients still let its sender edit it, and up to ten times each:
// https://support.signal.org/hc/en-us/articles/6255134251546-Edit-Message.
// This connector only checks the time window; the edit count is
// Signal's own server to enforce.
const signalEditWindow = 24 * time.Hour

// errCannotEdit reports "edit" asked of a message this account did
// not send: Signal only lets the sender edit their own message.
var errCannotEdit = fmt.Errorf("signal: edit: %w: only your own messages can be edited", connector.ErrEditUnsupported)

// errEditExpired reports a message past Signal's own time limit to
// edit it.
var errEditExpired = fmt.Errorf("signal: edit: %w: the time to edit this message has passed", connector.ErrEditUnsupported)

// EditMessage changes the text of the message remoteID names in conv
// to newText, reapplying mentions the same way a plain outgoing
// message does.
func (c *Connector) EditMessage(ctx context.Context, conv domain.Conversation, remoteID, newText string, mentions []domain.Mention) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	author, sentTimestamp, err := editableTarget(remoteID, c.selfID())
	if err != nil {
		return err
	}
	if time.Since(time.UnixMilli(int64(sentTimestamp))) > signalEditWindow {
		return errEditExpired
	}

	editCtx, cancel := context.WithTimeout(ctx, editTimeout)
	defer cancel()

	edit := &signalpb.EditMessage{
		TargetSentTimestamp: u64p(sentTimestamp),
		DataMessage: &signalpb.DataMessage{
			Body:       strp(newText),
			Timestamp:  u64p(uint64(time.Now().UnixMilli())),
			BodyRanges: outgoingBodyRanges(nil, mentions),
		},
	}
	_ = author // the sender check inside editableTarget already proved it is this account's own

	if err := sendToChat(editCtx, dev, conv.RemoteID, signalmeow.WrapEditMessage(edit)); err != nil {
		return fmt.Errorf("signal: edit: %w", err)
	}

	return nil
}

// editableTarget parses remoteID and checks it names a message self
// sent, returning errCannotEdit otherwise.
func editableTarget(remoteID string, self uuid.UUID) (author uuid.UUID, sentTimestamp uint64, err error) {
	author, sentTimestamp, parseErr := parseMessageRemoteID(remoteID)
	if parseErr != nil {
		return author, 0, fmt.Errorf("signal: edit: %w", parseErr)
	}
	if author != self {
		return author, 0, errCannotEdit
	}

	return author, sentTimestamp, nil
}
