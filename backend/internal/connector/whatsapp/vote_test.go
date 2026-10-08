package whatsapp

// Vote is driven through a fake device, the way react_test.go drives
// React, since the fake is the only way to see the vote message
// Connector asked whatsmeow to build and send without reaching
// WhatsApp's servers; handlePollVote is driven directly over the fake,
// the way live_test.go drives handleMessage.

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// pollCreationKey is the key a PollUpdateMessage carries to name which
// poll it votes in, by the poll creation message's own id.
func pollCreationKey(id string) *waCommon.MessageKey {
	return &waCommon.MessageKey{ID: strPtr(id)}
}

// mustHash is the SHA-256 hash WhatsApp's vote protocol identifies
// option name by, matching pollOptionID's own hex of the same hash.
func mustHash(t *testing.T, name string) []byte {
	t.Helper()

	return whatsmeow.HashPollOptions([]string{name})[0]
}

// lunchPoll is a two-option poll, saved under a direct chat's media
// store the way savePoll would when its creation message first
// arrives.
func lunchPoll() domain.Poll {
	return domain.Poll{
		Question: "Lunch?",
		Options:  []domain.PollOption{{ID: pollOptionID("Pizza"), Text: "Pizza"}, {ID: pollOptionID("Salad"), Text: "Salad"}},
	}
}

func TestVote_SendsTheEncryptedMessageAndReportsTheTally(t *testing.T) {
	t.Parallel()

	dev, sink, c, media := connectedMediaFixture(t)
	dev.buildVoteResp = &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}}

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putPoll(t.Context(), directChat.RemoteID, "M1", lunchPoll()); err != nil {
		t.Fatal(err)
	}

	if err := c.Vote(t.Context(), directChat, "M1", []string{pollOptionID("Pizza")}); err != nil {
		t.Fatal(err)
	}

	if len(dev.buildVoteCalls) != 1 || len(dev.buildVoteCalls[0].optionNames) != 1 || dev.buildVoteCalls[0].optionNames[0] != "Pizza" {
		t.Fatalf("buildVoteCalls = %+v, want one call voting for Pizza", dev.buildVoteCalls)
	}
	call := dev.buildVoteCalls[0]
	if call.pollInfo.Chat != directPeer || call.pollInfo.Sender != directPeer || call.pollInfo.ID != "M1" {
		t.Errorf("pollInfo = %+v, want the poll's own chat, sender and id", call.pollInfo)
	}

	if len(dev.sent) != 1 || dev.sent[0].msg != dev.buildVoteResp {
		t.Fatalf("sent = %+v, want the built vote message sent", dev.sent)
	}

	poll, ok := sink.PollFor(directChat.RemoteID, "M1")
	if !ok || poll.TotalVoters != 1 || !poll.Options[0].Chosen || poll.Options[1].Chosen {
		t.Errorf("poll = %+v, %t; want Pizza chosen for self", poll, ok)
	}
}

func TestVote_UnknownTargetFailsSafely(t *testing.T) {
	t.Parallel()

	dev, _, c, _ := connectedMediaFixture(t)

	err := c.Vote(t.Context(), directChat, "never-seen", []string{"a"})
	if !errors.Is(err, errUnknownVoteTarget) {
		t.Errorf("Vote = %v, want errUnknownVoteTarget", err)
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent for an unknown target", dev.sent)
	}
}

func TestVote_UnknownPollFailsSafely(t *testing.T) {
	t.Parallel()

	_, _, c, media := connectedMediaFixture(t)

	// The message key is known, as it would be for any ordinary
	// message, but no poll was ever saved for it, as a plain text
	// message's would not be.
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.Vote(t.Context(), directChat, "M1", []string{"a"}); !errors.Is(err, errUnknownVoteTarget) {
		t.Errorf("Vote = %v, want errUnknownVoteTarget", err)
	}
}

func TestVote_UnknownOptionFailsSafely(t *testing.T) {
	t.Parallel()

	_, _, c, media := connectedMediaFixture(t)

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putPoll(t.Context(), directChat.RemoteID, "M1", lunchPoll()); err != nil {
		t.Fatal(err)
	}

	if err := c.Vote(t.Context(), directChat, "M1", []string{"not-a-real-option"}); !errors.Is(err, errUnknownPollOption) {
		t.Errorf("Vote = %v, want errUnknownPollOption", err)
	}
}

func TestVote_FailsWithoutAMediaStore(t *testing.T) {
	t.Parallel()

	_, _, c := connectedFixture(t)

	if err := c.Vote(t.Context(), directChat, "M1", []string{"a"}); !errors.Is(err, errNotConnected) {
		t.Errorf("Vote = %v, want errNotConnected without a media store", err)
	}
}

func TestVote_WrapsADeviceError(t *testing.T) {
	t.Parallel()

	dev, sink, c, media := connectedMediaFixture(t)
	dev.sendErr = errors.New("server unavailable")
	dev.buildVoteResp = &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}}

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := media.putPoll(t.Context(), directChat.RemoteID, "M1", lunchPoll()); err != nil {
		t.Fatal(err)
	}

	if err := c.Vote(t.Context(), directChat, "M1", []string{pollOptionID("Pizza")}); !errors.Is(err, dev.sendErr) {
		t.Errorf("Vote = %v, want it to wrap the device's error", err)
	}
	if _, ok := sink.PollFor(directChat.RemoteID, "M1"); ok {
		t.Error("a vote was tallied despite the send failing")
	}
}

func TestHandlePollVote_TalliesAVoteFromSomeoneElse(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	if err := media.putPoll(t.Context(), directChat.RemoteID, "poll1", lunchPoll()); err != nil {
		t.Fatal(err)
	}
	dev.decryptVoteResp = &waE2E.PollVoteMessage{SelectedOptions: [][]byte{mustHash(t, "Pizza")}}

	// liveInfo is not from me, so this is Nadia's own vote arriving
	// live: it counts towards the tally, but is never "chosen", which
	// means chosen by the signed-in account.
	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{
		PollCreationMessageKey: pollCreationKey("poll1"),
	}}}

	c.handleMessage(t.Context(), sink, dev, media, e)

	poll, ok := sink.PollFor(directChat.RemoteID, "poll1")
	if !ok || poll.TotalVoters != 1 || poll.Options[0].Votes != 1 || poll.Options[0].Chosen {
		t.Errorf("poll = %+v, %t; want Pizza with one vote, not chosen by the signed-in account", poll, ok)
	}
}

func TestHandlePollVote_TalliesOurOwnVoteEchoedBack(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	if err := media.putPoll(t.Context(), directChat.RemoteID, "poll1", lunchPoll()); err != nil {
		t.Fatal(err)
	}
	dev.decryptVoteResp = &waE2E.PollVoteMessage{SelectedOptions: [][]byte{mustHash(t, "Pizza")}}

	info := liveInfo()
	info.IsFromMe = true
	e := &events.Message{Info: info, Message: &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{
		PollCreationMessageKey: pollCreationKey("poll1"),
	}}}

	c.handleMessage(t.Context(), sink, dev, media, e)

	poll, ok := sink.PollFor(directChat.RemoteID, "poll1")
	if !ok || !poll.Options[0].Chosen {
		t.Errorf("poll = %+v, %t; want Pizza chosen after our own vote echoes back", poll, ok)
	}
}

// TestHandlePollVote_RevoteUnderTheOtherAddressFormReplacesNotAdds
// reproduces the bug fixed by canonicalizing the poll voter id (see
// reactorKey's use of personID): the same real voter revoting after
// being addressed by their LID the first time and their mapped phone
// JID the second must still count as one voter, with the old
// selection cleared, not a second voter inflating TotalVoters while
// the first, stale vote lingers on the original option.
func TestHandlePollVote_RevoteUnderTheOtherAddressFormReplacesNotAdds(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)

	c, dev, sink, media := handlerMediaFixture(t)
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	if err := media.putPoll(t.Context(), directChat.RemoteID, "poll1", lunchPoll()); err != nil {
		t.Fatal(err)
	}

	info := liveInfo()
	info.Sender = lid
	dev.decryptVoteResp = &waE2E.PollVoteMessage{SelectedOptions: [][]byte{mustHash(t, "Pizza")}}
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info: info, Message: &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{PollCreationMessageKey: pollCreationKey("poll1")}},
	})

	info.Sender = phone
	dev.decryptVoteResp = &waE2E.PollVoteMessage{SelectedOptions: [][]byte{mustHash(t, "Salad")}}
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info: info, Message: &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{PollCreationMessageKey: pollCreationKey("poll1")}},
	})

	poll, ok := sink.PollFor(directChat.RemoteID, "poll1")
	if !ok || poll.TotalVoters != 1 {
		t.Fatalf("poll = %+v, %t; want exactly one distinct voter", poll, ok)
	}
	if poll.Options[0].Votes != 0 {
		t.Errorf("Pizza votes = %d, want the stale vote cleared once the same voter switched to Salad", poll.Options[0].Votes)
	}
	if poll.Options[1].Votes != 1 {
		t.Errorf("Salad votes = %d, want the revote counted", poll.Options[1].Votes)
	}
}

func TestHandlePollVote_DropsOnDecryptFailure(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	if err := media.putPoll(t.Context(), directChat.RemoteID, "poll1", lunchPoll()); err != nil {
		t.Fatal(err)
	}
	dev.decryptVoteErr = errors.New("decrypt failed")

	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{
		PollCreationMessageKey: pollCreationKey("poll1"),
	}}}

	c.handleMessage(t.Context(), sink, dev, media, e)

	if _, ok := sink.PollFor(directChat.RemoteID, "poll1"); ok {
		t.Error("a poll was tallied despite the vote failing to decrypt")
	}
}

func TestHandlePollVote_DropsWithoutAMediaStore(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)

	e := &events.Message{Info: liveInfo(), Message: &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{
		PollCreationMessageKey: pollCreationKey("poll1"),
	}}}

	// Must not panic with a nil media store, the same as every other
	// handler that only saves a message key or reference best effort.
	c.handleMessage(t.Context(), sink, dev, nil, e)
}
