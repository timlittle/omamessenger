package telegram

import (
	"bytes"
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestVote_SendsTheOptionsAndReportsTheEchoedPoll(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	results := tg.PollResults{}
	results.SetTotalVoters(4)
	results.SetResults([]tg.PollAnswerVoters{{Option: []byte{0}, Voters: 3, Chosen: true}, {Option: []byte{1}, Voters: 1}})
	pollUpdate := &tg.UpdateMessagePoll{MsgID: 7, Results: results}
	pollUpdate.SetPeer(&tg.PeerUser{UserID: 42})
	f.reply(&tg.MessagesSendVoteRequest{}, &tg.Updates{Updates: []tg.UpdateClass{pollUpdate}})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	c.learn(chatWithNadia.RemoteID)

	id := pollOptionID([]byte{0})
	if err := c.Vote(t.Context(), chatWithNadia, "7", []string{id}); err != nil {
		t.Fatal(err)
	}

	req, ok := f.sent()[0].(*tg.MessagesSendVoteRequest)
	if !ok || req.MsgID != 7 || !slices.EqualFunc(req.Options, [][]byte{{0}}, bytes.Equal) {
		t.Errorf("request = %+v, want message 7 voting for option 0", f.sent()[0])
	}

	poll, ok := sink.PollFor(chatWithNadia.RemoteID, "7")
	if !ok || poll.TotalVoters != 4 || len(poll.Options) != 2 || !poll.Options[0].Chosen {
		t.Errorf("poll = %+v, %t; want the echoed tally reported", poll, ok)
	}
}

func TestVote_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in":      New(domain.Account{ID: "tg"}, "").Vote(t.Context(), chatWithNadia, "7", []string{"a"}),
		"a malformed peer":       connected.Vote(t.Context(), domain.Conversation{RemoteID: "bad"}, "7", []string{"a"}),
		"a malformed message id": connected.Vote(t.Context(), chatWithNadia, "not-a-number", []string{"a"}),
		"a malformed option id":  connected.Vote(t.Context(), chatWithNadia, "7", []string{"not valid base64!!"}),
	} {
		if err == nil {
			t.Errorf("Vote(%s) = nil, want an error", name)
		}
	}
}

func TestPollChanged_ReportsForAKnownChatOnly(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := New(domain.Account{ID: "tg"}, "")
	results := tg.PollResults{}
	results.SetTotalVoters(2)
	results.SetResults([]tg.PollAnswerVoters{{Option: []byte{0}, Voters: 2}})
	update := &tg.UpdateMessagePoll{MsgID: 7, Results: results}
	update.SetPeer(&tg.PeerUser{UserID: 42})

	c.pollChanged(t.Context(), &sink, update)
	if got := sink.Lines(); len(got) != 0 {
		t.Errorf("events = %q, want none for an unknown chat", got)
	}

	c.learn("user:42:99")
	c.pollChanged(t.Context(), &sink, update)

	poll, ok := sink.PollFor("user:42:99", "7")
	if !ok || poll.TotalVoters != 2 {
		t.Errorf("poll = %+v, %t; want the tally reported", poll, ok)
	}
}
