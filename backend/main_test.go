//go:build fake

package main

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// stuckAfter is how long a wait lasts before the test calls the helper
// stuck. It only guards against hanging: every wait ends on an event, and
// a shared CI runner building the helper under the race detector in
// parallel can take far longer than a developer machine.
const stuckAfter = time.Minute

func TestRun_ServesTheDemoAndStopsAtEndOfInput(t *testing.T) {
	t.Parallel()

	in, client, stderr, done, updates := startInProcess(t, t.Context())
	exercise(t, client, updates)
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
	_, client, _, done, _ := startInProcess(t, ctx)
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

// TestRun_DoctorReportsOnAFreshInstall confirms the doctor subcommand
// runs standalone, without starting the server or any connector, and
// prints a line naming the database check on a data directory it has
// just created for itself. It does not assert the overall exit status:
// whether a notification service answers on this machine's session bus
// is outside the test's control, and must not make the test flaky.
func TestRun_DoctorReportsOnAFreshInstall(t *testing.T) {
	t.Parallel()

	var out, stderr bytes.Buffer
	s := streams{in: strings.NewReader(""), out: &out, errOut: &stderr}
	dir := filepath.Join(t.TempDir(), "data")

	_ = run(t.Context(), s, []string{"doctor", "--data-dir", dir}, lookup(nil))

	if !strings.Contains(out.String(), "Database: open and up to date") {
		t.Errorf("doctor output = %q, want a passing database check", out.String())
	}
	assertNoContent(t, out.String())
}

// TestRun_DoctorExitsWithAnErrorOnProblems confirms a looser-than-expected
// database file makes the doctor subcommand fail, with the failing check
// named in its output.
func TestRun_DoctorExitsWithAnErrorOnProblems(t *testing.T) {
	t.Parallel()

	var out, stderr bytes.Buffer
	s := streams{in: strings.NewReader(""), out: &out, errOut: &stderr}
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "messages.db")
	if err := os.WriteFile(dbPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	err := run(t.Context(), s, []string{"doctor", "--data-dir", dir}, lookup(nil))
	if err == nil {
		t.Fatal("doctor over a world-readable database file succeeded")
	}

	if !strings.Contains(out.String(), "FAIL Data permissions") {
		t.Errorf("doctor output = %q, want a failing \"Data permissions\" line", out.String())
	}
}

func TestHelperVersion_MatchesThePin(t *testing.T) {
	t.Parallel()

	pin, err := os.ReadFile(filepath.Join("..", "helper-version"))
	if err != nil {
		t.Fatal(err)
	}

	// helper-version carries a trailing "# x-release-please-version"
	// comment that release-please's generic updater matches on.
	line, _, _ := strings.Cut(string(pin), "#")
	if got := strings.TrimSpace(line); got != helperVersion {
		t.Fatalf("helper-version is %q but the helper reports %q; change both together", got, helperVersion)
	}
}

func TestBinary_ServesAndExitsAtEndOfInput(t *testing.T) {
	t.Parallel()

	cmd, stdin, client, stderr, updates := startBinary(t)
	exercise(t, client, updates)
	_ = stdin.Close()

	waitExit(t, cmd, stderr)
	assertNoContent(t, stderr.String())
}

func TestBinary_ExitsCleanlyOnSIGTERM(t *testing.T) {
	t.Parallel()

	cmd, _, client, stderr, _ := startBinary(t)
	call(t, client, "hello", nil, nil)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	waitExit(t, cmd, stderr)
}

// exercise runs a short session through the protocol: the full stack of
// server, application, store and fake connectors. updates carries the
// account.updated notifications the helper sends as it connects each
// fake account.
func exercise(t *testing.T, client *jsonrpc2.Conn, updates chan domain.Account) {
	t.Helper()

	var hello struct {
		Version string `json:"version"`
	}
	call(t, client, "hello", nil, &hello)
	if hello.Version != helperVersion {
		t.Fatalf("hello = %+v", hello)
	}

	// "Sam (spotty signal)" belongs to wa-personal. The fake connector
	// seeds all of an account's conversations synchronously before it
	// reports that account connected, so waiting for this one
	// notification is the signal the conversation is in the store —
	// not a fixed delay or a conversations.list poll.
	awaitAccountStatus(t, updates, "wa-personal", domain.AccountConnected)
	sam := findConversation(t, client, "Sam (spotty signal)")

	var sent domain.Message
	call(t, client, "messages.send", map[string]string{"conversationId": sam.ID, "text": "hello from the test"}, &sent)
	call(t, client, "fake.inject", map[string]string{"conversationId": sam.ID}, nil)
	call(t, client, "settings.apply", map[string]bool{"notifications": false}, nil)
}

// findConversation looks up a conversation by title, failing the test if
// it is not there.
func findConversation(t *testing.T, client *jsonrpc2.Conn, title string) domain.Conversation {
	t.Helper()

	var list []domain.Conversation
	call(t, client, "conversations.list", map[string]string{"query": title}, &list)
	if len(list) == 0 {
		t.Fatalf("conversation %q not found", title)
	}

	return list[0]
}

// awaitAccountStatus waits for an account.updated notification reporting
// accountID at status, failing the test after stuckAfter. A notification
// for a different account or an earlier status is skipped rather than
// treated as a failure: an account reports connecting before connected,
// and the helper runs several fake accounts at once.
func awaitAccountStatus(t *testing.T, updates chan domain.Account, accountID, status string) {
	t.Helper()

	deadline := time.After(stuckAfter)
	for {
		select {
		case account := <-updates:
			if account.ID == accountID && account.Status == status {
				return
			}
		case <-deadline:
			t.Fatalf("account %q never reported %q", accountID, status)
		}
	}
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
// coverage. done receives run's result; updates receives every
// account.updated notification the helper sends.
func startInProcess(t *testing.T, ctx context.Context) (io.Closer, *jsonrpc2.Conn, *lockedBuffer, chan error, chan domain.Account) {
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

	updates := make(chan domain.Account, 32)
	client := jsonrpc2.NewConn(ctx, jsonrpc2.NewPlainObjectStream(pipe{outR, inW}), &accountUpdates{updates: updates})
	t.Cleanup(func() { _ = client.Close() })

	return inW, client, stderr, done, updates
}

// await returns run's result, failing if it takes over stuckAfter.
func await(t *testing.T, done chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(stuckAfter):
		t.Fatal("run did not return")
		return nil
	}
}

// startBinary builds the test helper, with its fake connectors, and runs
// it. updates receives every account.updated notification the helper
// sends.
func startBinary(t *testing.T) (*exec.Cmd, io.Closer, *jsonrpc2.Conn, *lockedBuffer, chan domain.Account) {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "oma-messenger-service")
	build := exec.CommandContext(t.Context(), "go", "build", "-tags", "fake", "-buildvcs=false", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.CommandContext(t.Context(), binary, "--data-dir", filepath.Join(t.TempDir(), "data"))
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

	updates := make(chan domain.Account, 32)
	client := jsonrpc2.NewConn(t.Context(), jsonrpc2.NewPlainObjectStream(pipe{stdout, stdin}), &accountUpdates{updates: updates})
	t.Cleanup(func() { _ = client.Close() })

	return cmd, stdin, client, stderr, updates
}

// waitExit fails unless the helper exits cleanly within stuckAfter.
func waitExit(t *testing.T, cmd *exec.Cmd, stderr *lockedBuffer) {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper exit: %v\n%s", err, stderr.String())
		}
	case <-time.After(stuckAfter):
		t.Fatal("helper did not exit")
	}
}

// pipe joins a reader and a writer into a client connection.
type pipe struct {
	io.Reader
	io.WriteCloser
}

// accountUpdates forwards every account.updated notification's account to
// updates, so a test can wait for a specific account's status instead of
// polling the store for data that status change already guarantees is
// there. Any other notification is dropped.
type accountUpdates struct {
	updates chan domain.Account
}

// Handle decodes an account.updated notification's params and forwards
// the account; a notification this test does not care about, or one
// whose params do not decode, is dropped rather than failing the test.
func (a *accountUpdates) Handle(_ context.Context, _ *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Method != app.EventAccountUpdated || req.Params == nil {
		return
	}

	var account domain.Account
	if err := json.Unmarshal(*req.Params, &account); err != nil {
		return
	}

	a.updates <- account
}

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
