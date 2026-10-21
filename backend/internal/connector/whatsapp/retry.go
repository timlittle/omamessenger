package whatsapp

// retry.go asks WhatsApp's primary phone to re-upload a message's
// media once fetch.go finds its CDN link has expired (a 404 or 410),
// and waits for the phone's answer before fetch.go tries the download
// again. WhatsApp answers asynchronously, as an events.MediaRetry
// routed here by events.go, so this file also keeps the table that
// matches each answer back to the FetchMedia call waiting for it.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// mediaRetryTimeout bounds how long FetchMedia waits for the primary
// phone to answer a media retry request, so a phone that is offline or
// never answers cannot leave a download waiting forever.
const mediaRetryTimeout = 30 * time.Second

// retryDownload asks the primary phone to re-upload req.ref's media
// and, once it answers with a fresh path, downloads it once more. It
// reports domain.ErrMediaExpired when the phone says the media is gone
// or when it never answers in time. When this run never recorded the
// message's sender, and so cannot even ask (see keys.go), cause - the
// failure that sent downloadOrRetry here in the first place - is
// reported instead, classified the same way it would have been without
// any retry attempted: a stale link is still reported expired, but a
// decrypt failure stays a decrypt failure rather than being reported as
// if the primary phone had been asked and said the media was gone.
func (c *Connector) retryDownload(ctx context.Context, dev device, media *mediaStore, req fetchRequest, cause error) ([]byte, error) {
	ref, err := c.refreshRef(ctx, dev, media, req, cause)
	if err != nil {
		return nil, err
	}

	data, err := downloadOnce(ctx, dev, ref)
	if err == nil {
		return data, nil
	}

	if recovered, ok := recoverStaleDigest(data, err); ok {
		logStaleDigestAccepted()
		return recovered, nil
	}

	logDownloadFailed(err)
	return nil, classifyDownloadErr(err)
}

// retryStreamedDownload is retryDownload's own streamed-to-file
// counterpart (see fetch_stream.go's streamOrRetry), for a video, file
// or voice note too large to hold in memory: the same primary-phone
// retry, then one more bounded, deadline-aware streamed attempt at
// req.path with the fresh reference.
func (c *Connector) retryStreamedDownload(ctx context.Context, dev device, media *mediaStore, req fetchRequest, cause error) error {
	ref, err := c.refreshRef(ctx, dev, media, req, cause)
	if err != nil {
		return err
	}

	err = streamOnce(ctx, dev, ref, req.path, req.auto)
	if err == nil {
		return nil
	}

	if recoverableStaleDigest(err, fileHasBytes(req.path)) {
		logStaleDigestAccepted()
		return nil
	}

	removeIfPresent(req.path)
	logDownloadFailed(err)

	return classifyDownloadErr(err)
}

// refreshRef asks the primary phone to re-upload req.ref's media and
// returns the fresh reference it points the message's download at,
// saving it the same way the original reference was (see
// saveMediaRef). It reports domain.ErrMediaExpired when the phone says
// the media is gone or when it never answers in time, or, when this
// run never recorded the message's sender and so cannot even ask (see
// keys.go), reports cause instead, classified the same way it would
// have been without any retry attempted: see retryUnavailableErr.
func (c *Connector) refreshRef(ctx context.Context, dev device, media *mediaStore, req fetchRequest, cause error) (mediaRef, error) {
	chat, err := jidFromRemoteID(req.conv.RemoteID)
	if err != nil {
		return mediaRef{}, fmt.Errorf("whatsapp: media retry: %w", err)
	}

	key, found, err := media.messageKeyFor(ctx, req.conv.RemoteID, req.messageRemoteID)
	if err != nil {
		return mediaRef{}, fmt.Errorf("whatsapp: media retry: %w", err)
	}
	if !found {
		return mediaRef{}, retryUnavailableErr(cause)
	}

	info := retryMessageInfo(dev, chat, key, req.messageRemoteID)
	notif, err := c.requestMediaRetry(ctx, dev, info, req.ref.MediaKey)
	if err != nil {
		return mediaRef{}, err
	}

	ref := req.ref
	ref.DirectPath = notif.GetDirectPath()
	if err := media.put(ctx, req.conv.RemoteID, req.messageRemoteID, ref); err != nil {
		return mediaRef{}, fmt.Errorf("whatsapp: media retry: %w", err)
	}

	return ref, nil
}

// retryUnavailableErr reports what retryDownload could not even attempt
// to recover: cause classified as a decrypt failure when that is why
// downloadOrRetry sent it here, or domain.ErrMediaExpired otherwise, the
// category a stale CDN link with nothing to retry it with has always
// been reported as.
func retryUnavailableErr(cause error) error {
	if isDecryptFailure(cause) {
		return classifyDownloadErr(cause)
	}

	return domain.ErrMediaExpired
}

// requestMediaRetry sends the retry receipt for info and waits for the
// phone's decrypted answer, translating a "not available" or
// unrecognised result, or a timeout, into domain.ErrMediaExpired.
func (c *Connector) requestMediaRetry(ctx context.Context, dev device, info *types.MessageInfo, mediaKey []byte) (*waMmsRetry.MediaRetryNotification, error) {
	ch, cleanup := c.registerRetryWaiter(string(info.ID))
	defer cleanup()

	if err := dev.sendMediaRetryReceipt(ctx, info, mediaKey); err != nil {
		return nil, fmt.Errorf("whatsapp: media retry: %w", err)
	}

	retryCtx, cancel := context.WithTimeout(ctx, mediaRetryTimeout)
	defer cancel()

	select {
	case evt := <-ch:
		return decryptRetryNotification(evt, mediaKey)
	case <-retryCtx.Done():
		return nil, domain.ErrMediaExpired
	}
}

// decryptRetryNotification decrypts the phone's answer to a media
// retry request. Both a decrypt failure (an answer for a key that no
// longer matches, or that this connector cannot make sense of at all)
// and the phone reporting anything other than success are reported as
// domain.ErrMediaExpired: either way, there is nothing further to
// download.
func decryptRetryNotification(evt *events.MediaRetry, mediaKey []byte) (*waMmsRetry.MediaRetryNotification, error) {
	notif, err := whatsmeow.DecryptMediaRetryNotification(evt, mediaKey)
	if err != nil {
		return nil, domain.ErrMediaExpired
	}
	if notif.GetResult() != waMmsRetry.MediaRetryNotification_SUCCESS {
		return nil, domain.ErrMediaExpired
	}

	return notif, nil
}

// retryMessageInfo builds the message identity SendMediaRetryReceipt
// needs: the chat, whether this account sent the message, and, for a
// group, who did, from whichever of those message_keys last recorded
// for it (see keys.go).
func retryMessageInfo(dev device, chat types.JID, key messageKey, messageRemoteID string) *types.MessageInfo {
	return &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat: chat, Sender: retrySender(dev, key), IsFromMe: key.fromMe,
			IsGroup: chat.Server == types.GroupServer,
		},
		ID: types.MessageID(messageRemoteID),
	}
}

// retrySender is the sender retryMessageInfo reports: the message's own
// recorded sender for one this account did not send, or this account's
// own JID for one it did, matching how targetKey (keys.go) decides
// whose participant a group action names.
func retrySender(dev device, key messageKey) types.JID {
	if !key.fromMe && key.senderID != "" {
		if jid, err := types.ParseJID(key.senderID); err == nil {
			return jid
		}
	}

	jid, err := jidFromRemoteID(dev.selfChatID())
	if err != nil {
		return types.JID{}
	}

	return jid
}

// registerRetryWaiter reserves messageRemoteID's slot in this
// connector's table of pending media retries, so deliverRetry has
// somewhere to hand the phone's answer once it arrives. A second
// register for the same id overwrites the first's channel rather than
// refusing, unlike registerOnDemandWaiter (history_ondemand.go); the
// first caller's own cleanup then finds its channel no longer in the
// slot and leaves the second caller's entry alone (see
// waiterTable.cleanup). The returned cleanup must run once the caller
// stops waiting, successfully or not, so a request nobody is listening
// for any more cannot accumulate in the table forever.
func (c *Connector) registerRetryWaiter(messageRemoteID string) (<-chan *events.MediaRetry, func()) {
	ch, _ := c.retryWaiters.register(messageRemoteID, false)
	return ch, func() { c.retryWaiters.cleanup(messageRemoteID, ch) }
}

// deliverRetry hands a media retry notification to whichever
// requestMediaRetry call is waiting for it, matched by message id, or
// drops it when nothing is waiting: a notification for a message this
// run never asked a retry for, or one that already timed out and
// stopped listening.
func (c *Connector) deliverRetry(e *events.MediaRetry) {
	c.retryWaiters.deliver(string(e.MessageID), e)
}
