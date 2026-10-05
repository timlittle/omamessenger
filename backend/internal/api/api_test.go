package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/rpc"
)

// fakeCommands records the method called and its decoded params, and returns
// err from every method when set.
type fakeCommands struct {
	demo   bool
	err    error
	method string
	params any
}

func record[P any, R any](f *fakeCommands, name string, params P, result R) (R, error) {
	f.method, f.params = name, params
	if f.err != nil {
		var zero R
		return zero, f.err
	}
	return result, nil
}

func (f *fakeCommands) DemoMode() bool { return f.demo }
func (f *fakeCommands) Hello(_ context.Context, p struct{}) (app.HelloResult, error) {
	return record(f, "Hello", p, app.HelloResult{Protocol: 1})
}
func (f *fakeCommands) AccountsList(_ context.Context, p struct{}) ([]domain.Account, error) {
	return record(f, "AccountsList", p, []domain.Account{{ID: "wa"}})
}
func (f *fakeCommands) ConversationsList(_ context.Context, p app.ConversationsListParams) ([]domain.Conversation, error) {
	return record(f, "ConversationsList", p, []domain.Conversation{{ID: "c"}})
}
func (f *fakeCommands) MessagesList(_ context.Context, p app.MessagesListParams) (app.MessagesListResult, error) {
	return record(f, "MessagesList", p, app.MessagesListResult{HasMore: true})
}
func (f *fakeCommands) SendMessage(_ context.Context, p app.SendMessageParams) (domain.Message, error) {
	return record(f, "SendMessage", p, domain.Message{ID: "m"})
}
func (f *fakeCommands) Retry(_ context.Context, p app.RetryParams) (domain.Message, error) {
	return record(f, "Retry", p, domain.Message{ID: "m"})
}
func (f *fakeCommands) MarkRead(_ context.Context, p app.ConversationParams) (struct{}, error) {
	return record(f, "MarkRead", p, struct{}{})
}
func (f *fakeCommands) SetMuted(_ context.Context, p app.SetMutedParams) (domain.Conversation, error) {
	return record(f, "SetMuted", p, domain.Conversation{ID: "c"})
}
func (f *fakeCommands) OpenConversation(_ context.Context, p app.OpenConversationParams) (domain.Conversation, error) {
	return record(f, "OpenConversation", p, domain.Conversation{ID: "c"})
}
func (f *fakeCommands) ContactsList(_ context.Context, p app.ContactsListParams) ([]domain.Contact, error) {
	return record(f, "ContactsList", p, []domain.Contact{{RemoteID: "r"}})
}
func (f *fakeCommands) SetFocus(_ context.Context, p app.FocusParams) (struct{}, error) {
	return record(f, "SetFocus", p, struct{}{})
}
func (f *fakeCommands) ApplySettings(_ context.Context, p app.SettingsParams) (struct{}, error) {
	return record(f, "ApplySettings", p, struct{}{})
}
func (f *fakeCommands) Inject(_ context.Context, p app.InjectParams) (domain.Message, error) {
	return record(f, "Inject", p, domain.Message{ID: "m"})
}

var _ Commands = (*app.App)(nil)

// c3Calls is every C3 method with sample params and the typed call it must
// produce. The table is the executable form of the C3 method list.
var c3Calls = []struct {
	method string
	params string
	call   string
	want   any
}{
	{"hello", `{}`, "Hello", struct{}{}},
	{"accounts.list", `{}`, "AccountsList", struct{}{}},
	{"conversations.list", `{"query":"q"}`, "ConversationsList", app.ConversationsListParams{Query: "q"}},
	{"messages.list", `{"conversationId":"c","before":"m","limit":5}`, "MessagesList", app.MessagesListParams{ConversationID: "c", Before: "m", Limit: 5}},
	{"messages.send", `{"conversationId":"c","text":"hi"}`, "SendMessage", app.SendMessageParams{ConversationID: "c", Text: "hi"}},
	{"messages.retry", `{"messageId":"m"}`, "Retry", app.RetryParams{MessageID: "m"}},
	{"conversations.markRead", `{"conversationId":"c"}`, "MarkRead", app.ConversationParams{ConversationID: "c"}},
	{"conversations.setMuted", `{"conversationId":"c","muted":true}`, "SetMuted", app.SetMutedParams{ConversationID: "c", Muted: true}},
	{"conversations.open", `{"accountId":"wa","contactId":"r"}`, "OpenConversation", app.OpenConversationParams{AccountID: "wa", ContactID: "r"}},
	{"contacts.list", `{"accountId":"wa","query":"b"}`, "ContactsList", app.ContactsListParams{AccountID: "wa", Query: "b"}},
	{"ui.setFocus", `{"conversationId":"c","windowActive":true}`, "SetFocus", app.FocusParams{ConversationID: "c", WindowActive: true}},
	{"settings.apply", `{"notifications":true,"notificationPreview":false,"demoChatter":true}`, "ApplySettings", app.SettingsParams{Notifications: true, NotificationPreview: false, DemoChatter: true}},
	{"demo.inject", `{"conversationId":"c"}`, "Inject", app.InjectParams{ConversationID: "c"}},
}

func TestRegisterRoutesEveryC3Method(t *testing.T) {
	fake := &fakeCommands{demo: true}
	handler := Register(fake)
	if len(handler) != len(c3Calls) {
		t.Fatalf("Register has %d methods, the C3 table has %d", len(handler), len(c3Calls))
	}
	for _, c := range c3Calls {
		t.Run(c.method, func(t *testing.T) {
			method, ok := handler[c.method]
			if !ok {
				t.Fatalf("%s is not registered", c.method)
			}
			result, err := method(context.Background(), json.RawMessage(c.params))
			if err != nil || result == nil {
				t.Fatalf("result = %v, err = %v", result, err)
			}
			if fake.method != c.call || !reflect.DeepEqual(fake.params, c.want) {
				t.Errorf("called %s(%+v), want %s(%+v)", fake.method, fake.params, c.call, c.want)
			}
		})
	}
}

func TestRegisterPropagatesMethodErrors(t *testing.T) {
	sentinel := fmt.Errorf("%w: wrapped", domain.ErrNotFound)
	handler := Register(&fakeCommands{demo: true, err: sentinel})
	for _, c := range c3Calls {
		if _, err := handler[c.method](context.Background(), json.RawMessage(c.params)); !errors.Is(err, sentinel) {
			t.Errorf("%s error = %v, want the method's error", c.method, err)
		}
	}
}

func TestRegisterOmitsDemoInjectOutsideDemoMode(t *testing.T) {
	if _, ok := Register(&fakeCommands{demo: false})["demo.inject"]; ok {
		t.Fatal("demo.inject registered outside demo mode")
	}
}

func TestBindRejectsParamsOfTheWrongType(t *testing.T) {
	method := Register(&fakeCommands{})["messages.send"]
	if _, err := method(context.Background(), json.RawMessage(`{"text":42}`)); !errors.Is(err, errInvalidParams) {
		t.Fatalf("error = %v, want errInvalidParams", err)
	}
}

func TestCode(t *testing.T) {
	cases := []struct {
		err           error
		code, message string
	}{
		{errInvalidParams, "bad_request", "invalid params"},
		{fmt.Errorf("%w: text is required", app.ErrBadRequest), "bad_request", "bad request: text is required"},
		{fmt.Errorf("lookup: %w", domain.ErrNotFound), "not_found", "not found"},
		{app.ErrUnknownMethod, "unknown_method", "unknown method"},
		{errors.New("private message text"), "internal", "internal error"},
	}
	for _, c := range cases {
		if code, message := Code(c.err); code != c.code || message != c.message {
			t.Errorf("Code(%v) = %q, %q; want %q, %q", c.err, code, message, c.code, c.message)
		}
	}
}

// TestServeWithCode runs the table through rpc framing to prove Register and
// Code compose: a not-found error reaches the wire as not_found.
func TestServeWithCode(t *testing.T) {
	var output bytes.Buffer
	input := `{"id":1,"method":"conversations.markRead","params":{"conversationId":"missing"}}` + "\n"
	fake := &fakeCommands{err: domain.ErrNotFound}
	if err := rpc.Serve(context.Background(), strings.NewReader(input), rpc.NewStream(&output), Register(fake), Code); err != nil {
		t.Fatal(err)
	}
	var frame struct {
		ID    int `json:"id"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	line, _ := bufio.NewReader(&output).ReadBytes('\n')
	if err := json.Unmarshal(line, &frame); err != nil || frame.ID != 1 || frame.Error.Code != "not_found" {
		t.Fatalf("frame = %s (%v)", line, err)
	}
}
