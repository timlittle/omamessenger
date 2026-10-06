package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func lookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestResolveConfig(t *testing.T) {
	home := map[string]string{"HOME": "/home/test"}
	tests := []struct {
		name                               string
		args                               []string
		env                                map[string]string
		wantDB, wantDataDir                string
		wantSeed                           int64
		wantDemo, wantChatter, wantVersion bool
		wantErr                            bool
	}{
		{name: "default", env: home, wantDB: "/home/test/.local/share/omamessenger/messages.db", wantDataDir: "/home/test/.local/share/omamessenger", wantChatter: true},
		{name: "xdg wins over home", env: map[string]string{"HOME": "/home/test", "XDG_DATA_HOME": "/data"}, wantDB: "/data/omamessenger/messages.db", wantDataDir: "/data/omamessenger", wantChatter: true},
		{name: "demo uses demo.db", args: []string{"--demo", "--seed", "7"}, env: map[string]string{"XDG_DATA_HOME": "/data"}, wantDB: "/data/omamessenger/demo.db", wantDataDir: "/data/omamessenger", wantSeed: 7, wantDemo: true, wantChatter: true},
		{name: "data-dir is used as given", args: []string{"--data-dir", "/tmp/oma"}, env: map[string]string{}, wantDB: "/tmp/oma/messages.db", wantDataDir: "/tmp/oma", wantChatter: true},
		{name: "db overrides data-dir", args: []string{"--demo", "--no-chatter", "--data-dir", "/tmp/oma", "--db", "/tmp/custom.db"}, env: map[string]string{}, wantDB: "/tmp/custom.db", wantDataDir: "/tmp/oma", wantDemo: true},
		{name: "version", args: []string{"--version"}, env: home, wantDB: "/home/test/.local/share/omamessenger/messages.db", wantDataDir: "/home/test/.local/share/omamessenger", wantChatter: true, wantVersion: true},
		{name: "missing home", wantErr: true},
		{name: "unknown flag", args: []string{"--unknown"}, env: home, wantErr: true},
		{name: "invalid seed", args: []string{"--seed", "x"}, env: home, wantErr: true},
		{name: "extra argument", args: []string{"extra"}, env: home, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := resolveConfig(tt.args, lookup(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DBPath != tt.wantDB || cfg.DataDir != tt.wantDataDir || cfg.Demo != tt.wantDemo || cfg.Chatter != tt.wantChatter || cfg.Version != tt.wantVersion {
				t.Fatalf("config=%+v", cfg)
			}
			if tt.wantSeed != 0 && cfg.Seed != tt.wantSeed || cfg.Seed == 0 {
				t.Fatalf("seed = %d, want %d (or a non-zero default)", cfg.Seed, tt.wantSeed)
			}
		})
	}
	if _, err := resolveConfig(nil, nil); err == nil {
		t.Fatal("nil environment lookup should fail")
	}
}

type frame struct {
	ID     int             `json:"id"`
	Event  string          `json:"event"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

// client speaks the C3 protocol to a running helper.
type client struct {
	t       *testing.T
	in      io.Writer
	frames  chan frame
	readErr chan error
	mu      sync.Mutex
	nextID  int
}

func newClient(t *testing.T, in io.Writer, out io.Reader) *client {
	c := &client{t: t, in: in, frames: make(chan frame, 4096), readErr: make(chan error, 1)}
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		for scanner.Scan() {
			var value frame
			if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
				c.readErr <- fmt.Errorf("bad frame %q: %w", scanner.Text(), err)
				return
			}
			c.frames <- value
		}
		c.readErr <- scanner.Err()
	}()
	return c
}

// call sends one request and returns its response, skipping events.
func (c *client) call(method, params string) frame {
	c.t.Helper()
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	if _, err := fmt.Fprintf(c.in, `{"id":%d,"method":%q,"params":%s}`+"\n", id, method, params); err != nil {
		c.t.Fatalf("send %s: %v", method, err)
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case value := <-c.frames:
			if value.Event == "" && value.ID == id {
				return value
			}
		case err := <-c.readErr:
			c.t.Fatalf("helper output ended during %s: %v", method, err)
		case <-deadline.C:
			c.t.Fatalf("timed out waiting for %s", method)
		}
	}
}

// ok calls a method, requires a result, and decodes it into into (if non-nil).
func (c *client) ok(method, params string, into any) {
	c.t.Helper()
	response := c.call(method, params)
	if response.Error != nil || response.Result == nil {
		c.t.Fatalf("%s %s: error %+v", method, params, response.Error)
	}
	if into != nil {
		if err := json.Unmarshal(response.Result, into); err != nil {
			c.t.Fatalf("%s result %s: %v", method, response.Result, err)
		}
	}
}

func (c *client) wantError(method, params, code string) {
	c.t.Helper()
	if response := c.call(method, params); response.Error == nil || response.Error.Code != code {
		c.t.Fatalf("%s %s: error %+v, want %s", method, params, response.Error, code)
	}
}

type conversation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Unread int    `json:"unread"`
}

type message struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// conversationsByTitle waits for the demo seed and indexes it by title.
func (c *client) conversationsByTitle() map[string]conversation {
	c.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var list []conversation
		c.ok("conversations.list", `{}`, &list)
		if len(list) == 11 {
			byTitle := map[string]conversation{}
			for _, conv := range list {
				byTitle[conv.Title] = conv
			}
			return byTitle
		}
		if time.Now().After(deadline) {
			c.t.Fatalf("demo seed has %d conversations, want 11", len(list))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForStatus polls a conversation until the newest message has status.
func (c *client) waitForStatus(conversationID, messageID, status string) {
	c.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var page struct {
			Messages []message `json:"messages"`
		}
		c.ok("messages.list", fmt.Sprintf(`{"conversationId":%q}`, conversationID), &page)
		for _, m := range page.Messages {
			if m.ID == messageID && m.Status == status {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.t.Fatalf("message %s never reached %s", messageID, status)
}

// exerciseProtocol calls every C3 method once with valid params, plus the
// not_found and bad_request paths. It is the full-stack test for the method
// table: real store, app, demo connectors and rpc framing.
func exerciseProtocol(t *testing.T, c *client) {
	var hello struct {
		Protocol int    `json:"protocol"`
		Version  string `json:"version"`
		Demo     bool   `json:"demo"`
	}
	c.ok("hello", `{}`, &hello)
	if hello.Protocol != 1 || hello.Version != helperVersion || !hello.Demo {
		t.Fatalf("hello = %+v", hello)
	}
	var accounts []struct{ ID string }
	c.ok("accounts.list", `{}`, &accounts)
	if len(accounts) != 3 {
		t.Fatalf("accounts = %+v, want 3", accounts)
	}
	chats := c.conversationsByTitle()
	mum, sam := chats["Mum"], chats["Sam (spotty signal)"]

	var page struct {
		Messages []message `json:"messages"`
		HasMore  bool      `json:"hasMore"`
	}
	c.ok("messages.list", fmt.Sprintf(`{"conversationId":%q,"limit":5}`, mum.ID), &page)
	if len(page.Messages) != 5 || !page.HasMore {
		t.Fatalf("Mum first page = %d messages, hasMore %t", len(page.Messages), page.HasMore)
	}
	var sent message
	c.ok("messages.send", fmt.Sprintf(`{"conversationId":%q,"text":"hello from the test"}`, sam.ID), &sent)
	if sent.Status != "pending" {
		t.Fatalf("sent status = %q, want pending", sent.Status)
	}
	c.waitForStatus(sam.ID, sent.ID, "failed") // Sam's first attempt always fails
	c.ok("messages.retry", fmt.Sprintf(`{"messageId":%q}`, sent.ID), nil)
	c.ok("conversations.markRead", fmt.Sprintf(`{"conversationId":%q}`, chats["Alex Chen"].ID), nil)
	c.ok("conversations.setMuted", fmt.Sprintf(`{"conversationId":%q,"muted":true}`, mum.ID), nil)
	var contacts []struct{ RemoteID, Name string }
	c.ok("contacts.list", `{"accountId":"wa-personal","query":"ben"}`, &contacts)
	if len(contacts) != 1 || contacts[0].Name != "Ben Okafor" {
		t.Fatalf("contacts = %+v", contacts)
	}
	var opened conversation
	c.ok("conversations.open", fmt.Sprintf(`{"accountId":"wa-personal","contactId":%q}`, contacts[0].RemoteID), &opened)
	if opened.Title != "Ben Okafor" {
		t.Fatalf("opened = %+v", opened)
	}
	c.ok("ui.setFocus", fmt.Sprintf(`{"conversationId":%q,"windowActive":true}`, mum.ID), nil)
	c.ok("settings.apply", `{"notifications":false,"notificationPreview":false,"demoChatter":false}`, nil)
	c.ok("demo.inject", fmt.Sprintf(`{"conversationId":%q}`, mum.ID), nil)

	c.wantError("messages.list", `{"conversationId":"missing"}`, "not_found")
	c.wantError("messages.send", fmt.Sprintf(`{"conversationId":%q,"text":"  "}`, mum.ID), "bad_request")
	c.wantError("no.such.method", `{}`, "unknown_method")
}

// seedContent is user content the helper must never write to stderr.
var seedContent = []string{"Mum", "Climbing Crew", "Ben Okafor", "Call me when you're on your way", "https://example.com/tickets", "hello from the test"}

func assertNoContent(t *testing.T, stderr string) {
	t.Helper()
	for _, secret := range seedContent {
		if strings.Contains(stderr, secret) {
			t.Errorf("stderr contains user content %q:\n%s", secret, stderr)
		}
	}
	if strings.Contains(stderr, ".db") {
		t.Errorf("stderr exposes a database path:\n%s", stderr)
	}
}

func buildHelper(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "oma-messenger-service")
	build := exec.Command("go", "build", "-mod=vendor", "-buildvcs=false", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return binary
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// startBinary runs the built helper in demo mode on a fresh data dir.
func startBinary(t *testing.T, binary string) (*exec.Cmd, io.WriteCloser, *client, *lockedBuffer) {
	t.Helper()
	cmd := exec.Command(binary, "--demo", "--no-chatter", "--seed", "1", "--data-dir", filepath.Join(t.TempDir(), "data"))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, stdin, newClient(t, stdin, stdout), stderr
}

func waitExit(t *testing.T, cmd *exec.Cmd, stderr *lockedBuffer, why string) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper exit after %s: %v\n%s", why, err, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("helper did not exit within 2s of %s", why)
	}
}

func TestBuiltHelperServesEveryMethodAndExitsOnEOF(t *testing.T) {
	cmd, stdin, c, stderr := startBinary(t, buildHelper(t))
	exerciseProtocol(t, c)
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	waitExit(t, cmd, stderr, "stdin EOF")
	assertNoContent(t, stderr.String())
}

func TestBuiltHelperExitsCleanlyOnSIGTERM(t *testing.T) {
	cmd, _, c, stderr := startBinary(t, buildHelper(t))
	c.ok("hello", `{}`, nil)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitExit(t, cmd, stderr, "SIGTERM")
}

// startInProcess runs run() on pipes so the wiring counts toward coverage.
func startInProcess(t *testing.T, ctx context.Context) (*io.PipeWriter, *client, *lockedBuffer, chan error) {
	t.Helper()
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	stderr := &lockedBuffer{}
	done := make(chan error, 1)
	args := []string{"--demo", "--no-chatter", "--seed", "1", "--data-dir", filepath.Join(t.TempDir(), "data")}
	go func() {
		done <- run(ctx, ioStreams{in: inReader, out: outWriter, errOut: stderr}, args, lookup(nil))
		outWriter.Close()
	}()
	return inWriter, newClient(t, inWriter, outReader), stderr, done
}

func awaitRun(t *testing.T, done chan error, why string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("run did not return after %s", why)
		return nil
	}
}

func TestRunServesEveryMethodAndStopsOnEOF(t *testing.T) {
	in, c, stderr, done := startInProcess(t, context.Background())
	exerciseProtocol(t, c)
	in.Close()
	if err := awaitRun(t, done, "stdin EOF"); err != nil {
		t.Fatalf("run after EOF = %v", err)
	}
	assertNoContent(t, stderr.String())
	if !strings.Contains(stderr.String(), "OmaMessenger helper "+helperVersion+" started (demo: true)") {
		t.Errorf("startup line missing: %q", stderr.String())
	}
}

func TestRunStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in, c, _, done := startInProcess(t, ctx)
	defer in.Close()
	c.ok("hello", `{}`, nil)
	cancel()
	if err := awaitRun(t, done, "cancel"); err != nil {
		t.Fatalf("run after cancel = %v, want nil", err)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// TestRunReturnsWhenOutputBreaks is the regression test for a shutdown
// ordering bug: when the UI side of stdout went away, run waited for the
// connectors before canceling them and never returned.
func TestRunReturnsWhenOutputBreaks(t *testing.T) {
	done := make(chan error, 1)
	args := []string{"--demo", "--no-chatter", "--data-dir", filepath.Join(t.TempDir(), "data")}
	input := strings.NewReader(`{"id":1,"method":"hello","params":{}}` + "\n")
	go func() {
		done <- run(context.Background(), ioStreams{in: input, out: brokenWriter{}, errOut: io.Discard}, args, lookup(nil))
	}()
	if err := awaitRun(t, done, "a broken stdout"); err == nil {
		t.Fatal("run with a broken stdout returned nil, want the write error")
	}
}

func TestRunReportsStartupErrors(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	streams := ioStreams{in: strings.NewReader(""), out: io.Discard, errOut: io.Discard}
	if err := run(context.Background(), streams, []string{"--data-dir", filepath.Join(blocker, "data")}, lookup(nil)); err == nil || !strings.Contains(err.Error(), "open database") {
		t.Errorf("unusable data dir: %v", err)
	}
	if err := run(context.Background(), streams, []string{"--bogus"}, lookup(nil)); err == nil {
		t.Error("unknown flag was accepted")
	}
}

func TestVersionPrintsWithoutOpeningData(t *testing.T) {
	var out, stderr bytes.Buffer
	streams := ioStreams{in: strings.NewReader(""), out: &out, errOut: &stderr}
	err := run(context.Background(), streams, []string{"--version"}, lookup(map[string]string{"HOME": "/definitely/not/created"}))
	if err != nil || strings.TrimSpace(out.String()) != helperVersion || stderr.Len() != 0 {
		t.Fatalf("version: out=%q stderr=%q err=%v", out.String(), stderr.String(), err)
	}
	if _, err := os.Stat("/definitely/not/created"); !os.IsNotExist(err) {
		t.Fatalf("version unexpectedly used data dir: %v", err)
	}
}

// TestHelperVersionMatchesPin keeps the compiled-in version equal to the
// helper-version pin that the installer downloads and checks against.
func TestHelperVersionMatchesPin(t *testing.T) {
	pin, err := os.ReadFile(filepath.Join("..", "helper-version"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(pin)); got != helperVersion {
		t.Fatalf("helper-version pins %q but the helper reports %q; change both together", got, helperVersion)
	}
}
