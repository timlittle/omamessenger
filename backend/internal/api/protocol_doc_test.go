package api

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/tools/docscheck/markdown"
)

var updateProtocol = flag.Bool("update", false, "rewrite docs/PROTOCOL.md")

// MethodDesc documents one C3 method. Params and Result are zero values of
// the Go types the method decodes and returns; docs/PROTOCOL.md is rendered
// from them, so the reference cannot drift from the code.
type MethodDesc struct {
	Name     string
	Params   any
	Result   any
	Errors   []string
	DemoOnly bool
}

// EventDesc documents one C3 event.
type EventDesc struct {
	Name string
	Data string
	When string
}

// Methods lists every C3 method in protocol order. TestDescriptorsMatchRegister
// keeps it equal to the Register table.
func Methods() []MethodDesc {
	return []MethodDesc{
		{Name: "hello", Params: struct{}{}, Result: app.HelloResult{}},
		{Name: "accounts.list", Params: struct{}{}, Result: []domain.Account{}},
		{Name: "conversations.list", Params: app.ConversationsListParams{}, Result: []domain.Conversation{}},
		{Name: "messages.list", Params: app.MessagesListParams{}, Result: app.MessagesListResult{}, Errors: []string{"bad_request", "not_found"}},
		{Name: "messages.send", Params: app.SendMessageParams{}, Result: domain.Message{}, Errors: []string{"bad_request", "not_found"}},
		{Name: "messages.retry", Params: app.RetryParams{}, Result: domain.Message{}, Errors: []string{"bad_request", "not_found"}},
		{Name: "conversations.markRead", Params: app.ConversationParams{}, Result: struct{}{}, Errors: []string{"not_found"}},
		{Name: "conversations.setMuted", Params: app.SetMutedParams{}, Result: domain.Conversation{}, Errors: []string{"not_found"}},
		{Name: "conversations.open", Params: app.OpenConversationParams{}, Result: domain.Conversation{}, Errors: []string{"bad_request", "not_found"}},
		{Name: "contacts.list", Params: app.ContactsListParams{}, Result: []domain.Contact{}, Errors: []string{"bad_request", "not_found"}},
		{Name: "ui.setFocus", Params: app.FocusParams{}, Result: struct{}{}, Errors: []string{"not_found"}},
		{Name: "settings.apply", Params: app.SettingsParams{}, Result: struct{}{}},
		{Name: "demo.inject", Params: app.InjectParams{}, Result: domain.Message{}, Errors: []string{"not_found"}, DemoOnly: true},
	}
}

// Events lists every C3 event the helper emits.
func Events() []EventDesc {
	return []EventDesc{
		{Name: "account.updated", Data: "Account", When: "an account's status or detail changes"},
		{Name: "conversation.updated", Data: "Conversation", When: "a conversation is created or its preview, unread count, muting or title changes"},
		{Name: "message.added", Data: "Message", When: "any new message is stored, incoming or outgoing"},
		{Name: "message.updated", Data: "Message", When: "a delivery status changes in a way domain.StatusAdvances allows"},
		{Name: "typing", Data: "{conversationId, name, active}", When: "a connector reports a typing indicator"},
		{Name: "unread.changed", Data: "{total}", When: "the unread total across unmuted conversations changes"},
	}
}

const protocolHeader = `# Helper protocol

<!-- Generated from backend/internal/api/protocol_doc_test.go. After changing the protocol run: go test ./backend/internal/api -run TestProtocolDocCurrent -args -update -->

The helper and the UI exchange one JSON object per line. The helper reads requests on stdin and writes responses and events to stdout; stderr carries logs only. Lines are at most 1 MiB.

- Request: ` + "`" + `{"id":<int>,"method":"<name>","params":{...}}` + "`" + `
- Response: ` + "`" + `{"id":<int>,"result":<value>}` + "`" + ` or ` + "`" + `{"id":<int>,"error":{"code":"...","message":"..."}}` + "`" + `
- Event: ` + "`" + `{"event":"<name>","data":{...}}` + "`" + `

Error codes are ` + "`bad_request`, `not_found`, `unknown_method` and `internal`" + `. Any method can answer ` + "`internal`" + `; its message never carries internal details. Shapes below are the JSON fields of the Go types in ` + "`backend/internal/app` and `backend/internal/domain`" + `.
`

// renderProtocol renders docs/PROTOCOL.md from the descriptors.
func renderProtocol(methods []MethodDesc, events []EventDesc) string {
	var b strings.Builder
	b.WriteString(protocolHeader)
	b.WriteString("\n## Methods\n\n| method | params | result | errors |\n|---|---|---|---|\n")
	for _, m := range methods {
		name := "`" + m.Name + "`"
		if m.DemoOnly {
			name += " (demo only)"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", name, describe(m.Params), describe(m.Result), strings.Join(m.Errors, ", "))
	}
	b.WriteString("\n## Events\n\n| event | data | sent when |\n|---|---|---|\n")
	for _, e := range events {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", e.Name, e.Data, e.When)
	}
	return b.String()
}

// describe renders a Go value's JSON shape: named structs as
// "Name {field, ...}", anonymous structs as "{field, ...}", slices as "T[]".
func describe(v any) string {
	return "`" + shape(reflect.TypeOf(v), true) + "`"
}

func shape(t reflect.Type, withFields bool) string {
	switch {
	case t.Kind() == reflect.Slice:
		return shape(t.Elem(), false) + "[]"
	case t.Kind() != reflect.Struct:
		return t.Name()
	case t.NumField() == 0:
		return "{}"
	case !withFields:
		return t.Name()
	}
	var fields []string
	for i := 0; i < t.NumField(); i++ {
		if tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
			fields = append(fields, tag)
		}
	}
	if t.Name() == "" {
		return "{" + strings.Join(fields, ", ") + "}"
	}
	return t.Name() + " {" + strings.Join(fields, ", ") + "}"
}

func repoFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", name)
}

func TestProtocolDocCurrent(t *testing.T) {
	path := repoFile(t, "docs/PROTOCOL.md")
	want := renderProtocol(Methods(), Events())
	if *updateProtocol {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("docs/PROTOCOL.md is missing or stale (%v); run go test ./backend/internal/api -run TestProtocolDocCurrent -args -update", err)
	}
}

// TestContractC3MatchesAPI keeps the C3 method and event tables in
// docs/TASKS.md equal to the protocol the code implements.
func TestContractC3MatchesAPI(t *testing.T) {
	tasks, err := os.ReadFile(repoFile(t, "docs/TASKS.md"))
	if err != nil {
		t.Fatal(err)
	}
	tables := markdown.TableNames(markdown.Section(string(tasks), "### C3"))
	var methods, events []string
	for _, m := range Methods() {
		methods = append(methods, m.Name)
	}
	for _, e := range Events() {
		events = append(events, e.Name)
	}
	for kind, want := range map[string][]string{"method": methods, "event": events} {
		got := append([]string(nil), tables[kind]...)
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("C3 %s table = %v, code = %v", kind, got, want)
		}
	}
}

func TestDescribeShapes(t *testing.T) {
	cases := map[string]any{
		"`{}`":        struct{}{},
		"`Account[]`": []domain.Account{},
		"`{conversationId, muted}`": struct {
			ConversationID string `json:"conversationId"`
			Muted          bool   `json:"muted"`
			skip           int
		}{},
		"`HelloResult {protocol, version, demo, unreadTotal}`": app.HelloResult{},
		"`string`": "",
	}
	for want, v := range cases {
		if got := describe(v); got != want {
			t.Errorf("describe(%T) = %s, want %s", v, got, want)
		}
	}
}
