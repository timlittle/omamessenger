//go:build fake

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestRun_ServesTheDemoAndStopsAtEndOfInput(t *testing.T) {
	t.Parallel()

	in, client, stderr, done := startInProcess(t, t.Context())
	exercise(t, client)
	_ = in.Close()

	if err := await(t, done); err != nil {
		t.Fatalf("run = %v", err)
	}

	assertNoContent(t, stderr.String())
	if !strings.Contains(stderr.String(), "OmaMessenger helper "+helperVersion+" started") {
		t.Errorf("startup line missing from %q", stderr.String())
	}
}

func TestRun_StopsWhenCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	_, client, _, done := startInProcess(t, ctx)
	call(t, client, "hello", nil, nil)
	cancel()

	if err := await(t, done); err != nil {
		t.Fatalf("run after cancel = %v", err)
	}
}

func TestRun_ReportsStartupErrors(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	s := streams{in: strings.NewReader(""), out: io.Discard, errOut: io.Discard}
	if err := run(t.Context(), s, []string{"--data-dir", filepath.Join(file, "data")}, lookup(nil)); err == nil {
		t.Error("run with an unusable data directory succeeded")
	}

	if err := run(t.Context(), s, []string{"--bogus"}, lookup(nil)); err == nil {
		t.Error("run accepted an unknown flag")
	}
}

func TestRun_PrintsVersionWithoutTouchingData(t *testing.T) {
	t.Parallel()

	var out, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "never-created")
	s := streams{in: strings.NewReader(""), out: &out, errOut: &stderr}

	err := run(t.Context(), s, []string{"--version"}, lookup(map[string]string{"HOME": missing}))
	if err != nil || strings.TrimSpace(out.String()) != helperVersion || stderr.Len() != 0 {
		t.Fatalf("--version: out %q, stderr %q, %v", out.String(), stderr.String(), err)
	}

	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("--version created %s", missing)
	}
}

func TestHelperVersion_MatchesThePin(t *testing.T) {
	t.Parallel()

	pin, err := os.ReadFile(filepath.Join("..", "helper-version"))
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(string(pin)); got != helperVersion {
		t.Fatalf("helper-version is %q but the helper reports %q; change both together", got, helperVersion)
	}
}

func TestBinary_ServesAndExitsAtEndOfInput(t *testing.T) {
	t.Parallel()

	cmd, stdin, client, stderr := startBinary(t)
	exercise(t, client)
	_ = stdin.Close()

	waitExit(t, cmd, stderr)
	assertNoContent(t, stderr.String())
}

func TestBinary_ExitsCleanlyOnSIGTERM(t *testing.T) {
	t.Parallel()

	cmd, _, client, stderr := startBinary(t)
	call(t, client, "hello", nil, nil)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	waitExit(t, cmd, stderr)
}

// exercise runs a short session through the protocol: the full stack of
// server, application, store and fake connectors.
func exercise(t *testing.T, client *jsonrpc2.Conn) {
	t.Helper()

	var hello struct {
		Version string `json:"version"`
	}
	call(t, client, "hello", nil, &hello)
	if hello.Version != helperVersion {
		t.Fatalf("hello = %+v", hello)
	}

	sam := waitForConversation(t, client, "Sam (spotty signal)")

	var sent domain.Message
	call(t, client, "messages.send", map[string]string{"conversationId": sam.ID, "text": "hello from the test"}, &sent)
	call(t, client, "fake.inject", map[string]string{"conversationId": sam.ID}, nil)
	call(t, client, "settings.apply", map[string]bool{"notifications": false}, nil)
}

// waitForConversation polls until the fakes have seeded the conversation.
func waitForConversation(t *testing.T, client *jsonrpc2.Conn, title string) domain.Conversation {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var list []domain.Conversation
		call(t, client, "conversations.list", map[string]string{"query": title}, &list)

		if len(list) > 0 {
			return list[0]
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("conversation %q never appeared", title)

	return domain.Conversation{}
}

// call makes a request, failing the test on an error.
func call(t *testing.T, client *jsonrpc2.Conn, method string, params, result any) {
	t.Helper()

	if err := client.Call(t.Context(), method, params, result); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

// seedContent is user content the helper must never write to its log.
var seedContent = []string{"Mum", "Climbing Crew", "Ben Okafor", "Call me when", "https://example.com/tickets", "hello from the test"}

// assertNoContent fails if the log holds message content or a data path.
func assertNoContent(t *testing.T, log string) {
	t.Helper()

	for _, secret := range seedContent {
		if strings.Contains(log, secret) {
			t.Errorf("log contains user content %q:\n%s", secret, log)
		}
	}

	if strings.Contains(log, ".db") {
		t.Errorf("log exposes a database path:\n%s", log)
	}
}

// startInProcess runs the helper on pipes, so the wiring counts towards
// coverage. done receives run's result.
func startInProcess(t *testing.T, ctx context.Context) (io.Closer, *jsonrpc2.Conn, *lockedBuffer, chan error) {
	t.Helper()

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	stderr := &lockedBuffer{}
	done := make(chan error, 1)
	args := []string{"--data-dir", filepath.Join(t.TempDir(), "data")}

	go func() {
		done <- run(ctx, streams{in: inR, out: outW, errOut: stderr}, args, lookup(nil))
		_ = outW.Close()
	}()

	client := jsonrpc2.NewConn(t.Context(), jsonrpc2.NewPlainObjectStream(pipe{outR, inW}), noEvents{})
	t.Cleanup(func() { _ = client.Close() })

	return inW, client, stderr, done
}

// await returns run's result, failing if it takes over five seconds.
func await(t *testing.T, done chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return")
		return nil
	}
}

// startBinary builds the test helper, with its fake connectors, and runs it.
func startBinary(t *testing.T) (*exec.Cmd, io.Closer, *jsonrpc2.Conn, *lockedBuffer) {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "oma-messenger-service")
	if out, err := exec.Command("go", "build", "-tags", "fake", "-buildvcs=false", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(binary, "--data-dir", filepath.Join(t.TempDir(), "data"))
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

	client := jsonrpc2.NewConn(t.Context(), jsonrpc2.NewPlainObjectStream(pipe{stdout, stdin}), noEvents{})
	t.Cleanup(func() { _ = client.Close() })

	return cmd, stdin, client, stderr
}

// waitExit fails unless the helper exits cleanly within five seconds.
func waitExit(t *testing.T, cmd *exec.Cmd, stderr *lockedBuffer) {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper exit: %v\n%s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not exit")
	}
}

// pipe joins a reader and a writer into a client connection.
type pipe struct {
	io.Reader
	io.WriteCloser
}

// noEvents ignores the helper's notifications.
type noEvents struct{}

func (noEvents) Handle(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request) {}

// lockedBuffer is a bytes.Buffer safe for the helper and test to share.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}
