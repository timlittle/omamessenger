package whatsapp

import (
	"reflect"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waVnameCert"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// testInfo is an incoming message in a direct chat from Nadia.
func testInfo() types.MessageInfo {
	return types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:   types.NewJID("15551234567", types.DefaultUserServer),
			Sender: types.NewJID("15551234567", types.DefaultUserServer),
		},
		ID:        "ABCD1234",
		PushName:  "Nadia Rahman",
		Timestamp: time.Unix(1_800_000_000, 0),
	}
}

func TestMessage_FromIncomingOutgoingAndMedia(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info types.MessageInfo
		msg  *waE2E.Message
		want domain.Message
	}{
		{
			"incoming text",
			testInfo(), &waE2E.Message{Conversation: strPtr("hi")},
			domain.Message{
				RemoteID: "ABCD1234", SenderID: "15551234567@s.whatsapp.net", SenderName: "Nadia Rahman",
				Text: "hi", Status: domain.StatusReceived, Created: 1_800_000_000_000,
			},
		},
		{
			"outgoing text",
			outgoingInfo(), &waE2E.Message{Conversation: strPtr("back")},
			domain.Message{
				RemoteID: "ABCD1234", SenderID: "self", SenderName: "You",
				Text: "back", Outgoing: true, Status: domain.StatusSent, Created: 1_800_000_000_000,
			},
		},
		{
			"photo with no caption",
			testInfo(), &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}},
			domain.Message{
				RemoteID: "ABCD1234", SenderID: "15551234567@s.whatsapp.net", SenderName: "Nadia Rahman",
				Text: "[Photo]", Status: domain.StatusReceived, Created: 1_800_000_000_000,
				Media: &domain.Media{Kind: domain.MediaPhoto},
			},
		},
		{
			"sender with no push name",
			types.MessageInfo{
				MessageSource: types.MessageSource{Chat: testInfo().Chat, Sender: testInfo().Sender},
				ID:            "X", Timestamp: time.Unix(1, 0),
			},
			&waE2E.Message{Conversation: strPtr("hi")},
			domain.Message{
				RemoteID: "X", SenderID: "15551234567@s.whatsapp.net", SenderName: "WhatsApp user",
				Text: "hi", Status: domain.StatusReceived, Created: 1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := message(tt.info, tt.msg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("message = %+v\nwant     %+v", got, tt.want)
			}
		})
	}
}

// outgoingInfo is a message this account sent itself.
func outgoingInfo() types.MessageInfo {
	info := testInfo()
	info.IsFromMe = true

	return info
}

func TestMessage_UnwrapsEphemeralAndViewOnce(t *testing.T) {
	t.Parallel()

	inner := &waE2E.Message{Conversation: strPtr("secret")}
	tests := []struct {
		name string
		msg  *waE2E.Message
	}{
		{"ephemeral", &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: inner}}},
		{"view once", &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: inner}}},
		{"view once v2", &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: inner}}},
		{"view once v2 extension", &waE2E.Message{ViewOnceMessageV2Extension: &waE2E.FutureProofMessage{Message: inner}}},
		{"device sent", &waE2E.Message{DeviceSentMessage: &waE2E.DeviceSentMessage{Message: inner}}},
		{
			"nested ephemeral view-once",
			&waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
				ViewOnceMessage: &waE2E.FutureProofMessage{Message: inner},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := message(testInfo(), tt.msg); got.Text != "secret" {
				t.Errorf("message(%s).Text = %q, want %q", tt.name, got.Text, "secret")
			}
		})
	}
}

func TestMessage_UnwrapGivesUpAfterTheDepthLimit(t *testing.T) {
	t.Parallel()

	// Wrap a message one layer deeper than unwrap will peel, so the
	// bound is actually exercised rather than just present in the code.
	msg := &waE2E.Message{Conversation: strPtr("secret")}
	for range unwrapDepth + 1 {
		msg = &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: msg}}
	}

	got := message(testInfo(), msg)
	if got.Text == "secret" {
		t.Error("message unwrapped past its depth limit")
	}
}

func TestSenderName_FallsBackSensibly(t *testing.T) {
	t.Parallel()

	verified := &types.VerifiedName{Details: &waVnameCert.VerifiedNameCertificate_Details{VerifiedName: strPtr("Acme Support")}}
	tests := []struct {
		name string
		info types.MessageInfo
		want string
	}{
		{"push name", types.MessageInfo{PushName: "Nadia"}, "Nadia"},
		{"verified business name", types.MessageInfo{VerifiedName: verified}, "Acme Support"},
		{"verified name with no details", types.MessageInfo{VerifiedName: &types.VerifiedName{}}, "WhatsApp user"},
		{"no name at all", types.MessageInfo{}, "WhatsApp user"},
	}

	for _, tt := range tests {
		if got := senderName(tt.info); got != tt.want {
			t.Errorf("senderName(%s) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestReplyTo_ReportsTheQuotedStanzaOrNil(t *testing.T) {
	t.Parallel()

	withReply := message(testInfo(), &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: strPtr("sure"), ContextInfo: &waE2E.ContextInfo{StanzaID: strPtr("Q1")}},
	})
	if withReply.ReplyTo == nil || withReply.ReplyTo.RemoteID != "Q1" {
		t.Fatalf("message.ReplyTo = %+v, want it to quote remote id Q1", withReply.ReplyTo)
	}

	plain := message(testInfo(), &waE2E.Message{Conversation: strPtr("hi")})
	if plain.ReplyTo != nil {
		t.Errorf("message.ReplyTo = %+v, want nil for a message that answers nothing", plain.ReplyTo)
	}
}

func TestMessageText_LabelsMediaWithoutACaption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		msg  *waE2E.Message
		want string
	}{
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, "[Photo]"},
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: strPtr("sunset")}}, "sunset"},
		{&waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, "[Video]"},
		{&waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}}, "[File]"},
		{&waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}, "[Sticker]"},
		{&waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: boolPtr(true)}}, "[Voice message]"},
		{&waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: boolPtr(false)}}, "[Audio]"},
		{&waE2E.Message{ContactMessage: &waE2E.ContactMessage{}}, "[Contact]"},
		{&waE2E.Message{LocationMessage: &waE2E.LocationMessage{}}, "[Location]"},
		{&waE2E.Message{}, "[Message]"},
	}

	for _, tt := range tests {
		if got := messageText(tt.msg); got != tt.want {
			t.Errorf("messageText(%+v) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestContextInfo_ReadsEveryMediaKindThatCarriesOne(t *testing.T) {
	t.Parallel()

	ctx := &waE2E.ContextInfo{StanzaID: strPtr("Q")}
	tests := []*waE2E.Message{
		{ExtendedTextMessage: &waE2E.ExtendedTextMessage{ContextInfo: ctx}},
		{ImageMessage: &waE2E.ImageMessage{ContextInfo: ctx}},
		{VideoMessage: &waE2E.VideoMessage{ContextInfo: ctx}},
		{AudioMessage: &waE2E.AudioMessage{ContextInfo: ctx}},
		{DocumentMessage: &waE2E.DocumentMessage{ContextInfo: ctx}},
		{StickerMessage: &waE2E.StickerMessage{ContextInfo: ctx}},
	}

	for _, msg := range tests {
		if got := contextInfo(msg); got.GetStanzaID() != "Q" {
			t.Errorf("contextInfo(%+v).GetStanzaID() = %q, want %q", msg, got.GetStanzaID(), "Q")
		}
	}

	if got := contextInfo(&waE2E.Message{}); got != nil {
		t.Errorf("contextInfo(plain) = %+v, want nil", got)
	}
}

func TestHistoryMessage_DirectAndGroup(t *testing.T) {
	t.Parallel()

	direct := types.NewJID("15551234567", types.DefaultUserServer)
	group := types.NewJID("12345-1600000000", types.GroupServer)

	tests := []struct {
		name string
		chat types.JID
		hm   *waHistorySync.HistorySyncMsg
		want domain.Message
	}{
		{
			"incoming direct message",
			direct,
			&waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
				Key:              &waCommon.MessageKey{ID: strPtr("H1")},
				Message:          &waE2E.Message{Conversation: strPtr("hi")},
				PushName:         strPtr("Nadia"),
				MessageTimestamp: u64(1_800_000_000),
			}},
			domain.Message{
				RemoteID: "H1", SenderID: "15551234567@s.whatsapp.net", SenderName: "Nadia",
				Text: "hi", Status: domain.StatusReceived, Created: 1_800_000_000_000,
			},
		},
		{
			"outgoing message",
			direct,
			&waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
				Key:              &waCommon.MessageKey{ID: strPtr("H2"), FromMe: boolPtr(true)},
				Message:          &waE2E.Message{Conversation: strPtr("back")},
				MessageTimestamp: u64(1_800_000_000),
			}},
			domain.Message{
				RemoteID: "H2", SenderID: "self", SenderName: "You",
				Text: "back", Outgoing: true, Status: domain.StatusSent, Created: 1_800_000_000_000,
			},
		},
		{
			"group message names its participant",
			group,
			&waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
				Key:              &waCommon.MessageKey{ID: strPtr("H3")},
				Message:          &waE2E.Message{Conversation: strPtr("hi all")},
				Participant:      strPtr("15551234567@s.whatsapp.net"),
				PushName:         strPtr("Nadia"),
				MessageTimestamp: u64(1_800_000_000),
			}},
			domain.Message{
				RemoteID: "H3", SenderID: "15551234567@s.whatsapp.net", SenderName: "Nadia",
				Text: "hi all", Status: domain.StatusReceived, Created: 1_800_000_000_000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := historyMessage(tt.chat, tt.hm)
			if !ok || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("historyMessage = %+v, %t; want %+v, true", got, ok, tt.want)
			}
		})
	}
}

func TestHistoryMessage_DropsReactionEditAndRevoke(t *testing.T) {
	t.Parallel()

	chat := types.NewJID("15551234567", types.DefaultUserServer)
	tests := []struct {
		name string
		msg  *waE2E.Message
	}{
		{"reaction", &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: strPtr("👍")}}},
		{"revoke", &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum()}}},
		{
			"edit",
			&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
				Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), EditedMessage: &waE2E.Message{Conversation: strPtr("new text")},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hm := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
				Key: &waCommon.MessageKey{ID: strPtr("H1")}, Message: tt.msg,
			}}
			if _, ok := historyMessage(chat, hm); ok {
				t.Error("historyMessage reported a protocol message as content")
			}
		})
	}
}

// FuzzHistoryMessage checks that historyMessage never panics on a history
// sync message decoded from arbitrary bytes.
func FuzzHistoryMessage(f *testing.F) {
	seed, _ := proto.Marshal(&waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{ID: strPtr("H1")}, Message: &waE2E.Message{Conversation: strPtr("hi")},
	}})
	f.Add(seed)
	f.Add([]byte{})

	chat := types.NewJID("15551234567", types.DefaultUserServer)

	f.Fuzz(func(t *testing.T, data []byte) {
		var hm waHistorySync.HistorySyncMsg
		if err := proto.Unmarshal(data, &hm); err != nil {
			return
		}

		historyMessage(chat, &hm)
	})
}

// FuzzMessage checks that message never panics on a message decoded
// from arbitrary bytes, and always returns some text, since every
// stored message needs one.
func FuzzMessage(f *testing.F) {
	seed, _ := proto.Marshal(&waE2E.Message{Conversation: strPtr("hi")})
	f.Add(seed)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		var msg waE2E.Message
		if err := proto.Unmarshal(data, &msg); err != nil {
			return
		}

		got := message(testInfo(), &msg)
		if got.Text == "" {
			t.Errorf("message produced no text for %+v", &msg)
		}
	})
}
