// Command oma-messenger-service is OmaMessenger's helper. Omarchy's shell
// starts it and talks JSON-RPC to it over stdin and stdout; it keeps the
// message database and runs the connectors for each messaging service.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/clipboard"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/telegram"
	"github.com/timlittle/omamessenger/backend/internal/connector/whatsapp"
	"github.com/timlittle/omamessenger/backend/internal/doctor"
	"github.com/timlittle/omamessenger/backend/internal/notify"
	"github.com/timlittle/omamessenger/backend/internal/server"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// providers lists every messaging service this helper can add an account
// for. Adding a service means adding its Provider here.
func providers() []connector.Provider {
	return []connector.Provider{telegram.Provider{Version: helperVersion}, whatsapp.Provider{}}
}

// mediaCacheLimit is how much downloaded media the helper keeps before it
// drops the least recently used: 1 GiB.
const mediaCacheLimit = 1 << 30

// outgoingRetention is how long a pending or failed message's outgoing
// attachment is kept for a retry before it is swept regardless of the
// area's size: long enough to retry after being offline for a few days,
// short enough that an attachment nobody ever retries does not sit
// there forever.
const outgoingRetention = 7 * 24 * time.Hour

// outgoingSizeLimit is how much the outgoing media area may hold before
// the least recently used pending or failed attachments are swept to
// make room: generous for a handful of large attachments awaiting
// retry, far below the 1 GiB downloaded media is allowed, since nothing
// here should ever need to hold as much.
const outgoingSizeLimit = 256 << 20

// outgoingSweepInterval is how often the outgoing media area is swept
// for expired or oversized copies while the helper runs, on top of once
// at startup.
const outgoingSweepInterval = time.Hour

// notify.Desktop is the only Notifier the helper wires; app can't import
// notify (see .golangci.yml), so the pairing is checked here instead.
var _ app.Notifier = notify.Desktop{}

// main runs the helper until the UI disconnects or it receives SIGTERM.
func main() { // coverage-ignore: process entry point; run is tested
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	streams := streams{in: os.Stdin, out: os.Stdout, errOut: os.Stderr}
	if err := run(ctx, streams, os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "OmaMessenger helper:", err)
		os.Exit(1)
	}
}

// streams are the protocol input and output and the log output.
type streams struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

// run reads the configuration and serves the UI until it disconnects or ctx
// is cancelled; both are a clean exit. "doctor" as the first argument
// runs the health checks instead (see runDoctor).
func run(ctx context.Context, s streams, args []string, env func(string) string) error {
	if len(args) > 0 && args[0] == "doctor" {
		return runDoctor(ctx, args[1:], env, s)
	}

	cfg, err := resolveConfig(args, env)
	if err != nil {
		return err
	}

	if cfg.version {
		_, err := fmt.Fprintln(s.out, helperVersion)
		return err
	}

	return serve(ctx, cfg, s)
}

// runDoctor runs the helper's own health checks against the data
// directory the remaining args or env resolve to, printing one safe line
// per check to s.out. It returns an error once any check found a
// problem, so the process exits non-zero; the command palette's "Run
// health check" instead reaches the same checks in process, through the
// helper.doctor method, while the helper is already running.
func runDoctor(ctx context.Context, args []string, env func(string) string, s streams) error {
	cfg, err := resolveConfig(args, env)
	if err != nil {
		return err
	}

	db, err := store.Open(ctx, cfg.dbPath)
	if err != nil {
		return fmt.Errorf("doctor: open database: %w", err)
	}
	defer db.Close()

	commands, _ := app.New(app.Deps{
		Store: db, Cache: cache.New(filepath.Join(cfg.dataDir, "media"), mediaCacheLimit),
		DataDir: cfg.dataDir, DBPath: cfg.dbPath,
		ExecutableName: filepath.Base(os.Args[0]), HelperVersion: helperVersion,
	})

	report, err := commands.Doctor(ctx)
	if err != nil {
		return fmt.Errorf("doctor: %w", err)
	}

	printDoctorReport(s.out, report)
	if problems := report.Problems(); problems > 0 {
		return fmt.Errorf("doctor: found %d problem(s)", problems)
	}

	return nil
}

// printDoctorReport writes one line per check: whether it passed, its
// name and its safe detail text.
func printDoctorReport(w io.Writer, report doctor.Report) {
	for _, c := range report.Checks {
		mark := "ok  "
		if !c.OK {
			mark = "FAIL"
		}

		fmt.Fprintf(w, "%s %s: %s\n", mark, c.Name, c.Detail)
	}
}

// serve opens the database, wires the application and serves the UI.
func serve(ctx context.Context, cfg config, s streams) error {
	db, err := store.Open(ctx, cfg.dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := log.New(s.errOut, "", 0)
	srv := server.New(helperVersion, logger)

	registry := newAccountRegistry(db, cfg.dataDir, providers())
	caches := mediaCaches{
		downloaded: cache.New(filepath.Join(cfg.dataDir, "media"), mediaCacheLimit),
		outgoing:   cache.NewOutgoing(filepath.Join(cfg.dataDir, "media", "outgoing"), outgoingRetention, outgoingSizeLimit),
	}
	commands, ingest, manager, err := wire(ctx, wireDeps{
		db: db, srv: srv, registry: registry, caches: caches, logger: logger,
		dataDir: cfg.dataDir, dbPath: cfg.dbPath,
	})
	if err != nil {
		return err
	}

	// Serve before the connectors start, so the UI sees their first events.
	// On every return, stop the connectors and the reminder scheduler and
	// finish requests in progress before the database closes.
	srv.Start(ctx, stdio{s.in, s.out}, commands)

	var background sync.WaitGroup
	defer func() {
		cancel()
		srv.Wait()
		manager.Wait()
		background.Wait()
	}()

	background.Go(func() { commands.RunReminders(ctx) })
	background.Go(func() { caches.outgoing.RunSweeper(ctx, outgoingSweepInterval, logger) })

	if err := manager.Start(ctx, db, ingest); err != nil && ctx.Err() == nil {
		return fmt.Errorf("start connectors: %w", err)
	}

	logger.Printf("OmaMessenger helper %s started", helperVersion)
	srv.Wait()

	return nil
}

// mediaCaches are the two directories the helper keeps media in:
// downloaded media, pruned once it grows past a limit, and outgoing
// attachments, kept until their message is no longer worth retrying.
type mediaCaches struct {
	downloaded *cache.Cache
	outgoing   *cache.Outgoing
}

// wireDeps are wire's own inputs, grouped into one struct so adding one,
// such as the logger diagnostics needed, does not keep growing wire's
// own argument list.
type wireDeps struct {
	db       *store.Store
	srv      *server.Server
	registry *accountRegistry
	caches   mediaCaches
	logger   *log.Logger
	dataDir  string
	dbPath   string
}

// wire builds the application around d's database and server, starting
// with the accounts already saved. Test builds add the fake connectors;
// see fake.go.
func wire(ctx context.Context, d wireDeps) (*app.Commands, *app.Ingest, *connector.Manager, error) {
	notifier := notify.Desktop{Click: func(conversationID string) {
		d.srv.Publish(ctx, app.EventNotificationClicked, app.NotificationClicked{ConversationID: conversationID})
	}}
	deps := app.Deps{
		Store: d.db, Notifier: notifier, Publisher: d.srv, Accounts: d.registry, Cache: d.caches.downloaded,
		Outgoing: d.caches.outgoing, Clipboard: clipboard.Wayland{}, Logger: d.logger,
		DataDir: d.dataDir, DBPath: d.dbPath, ExecutableName: filepath.Base(os.Args[0]), HelperVersion: helperVersion,
	}

	connectors, injector := fakeConnectors()
	if injector != nil {
		deps.Fake = injector
	}

	saved, err := d.registry.saved(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	manager, err := connector.NewManager(append(connectors, saved...)...)
	if err != nil {
		return nil, nil, nil, err
	}

	d.registry.manager = manager
	deps.Dispatcher, deps.SignIn, deps.History, deps.Media, deps.Refresher, deps.Organizer, deps.Reactor, deps.Voter, deps.Deleter, deps.Members = manager, manager, manager, manager, manager, manager, manager, manager, manager, manager
	commands, ingest := app.New(deps)

	return commands, ingest, manager, nil
}

// stdio joins the helper's stdin and stdout into one connection.
type stdio struct {
	io.Reader
	io.Writer
}

// Close closes stdin when it can be closed, which unblocks a pending read
// in tests. A blocked read on a real pipe is not interrupted, but the
// process is exiting by then.
func (s stdio) Close() error {
	if c, ok := s.Reader.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
