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

// outgoingOrphanGrace is how long a newly stored outgoing file is left
// alone before Sweep will consider removing it as an orphan - one whose
// id matches no message at all, such as an image pasted from the
// clipboard but never sent - so a file just attached has time to become
// a real message's copy before the first sweep might otherwise see it
// as abandoned. A pending or failed message's own copy is never swept
// regardless of its age; see cache.Outgoing.
const outgoingOrphanGrace = time.Hour

// outgoingSizeLimit is how much the outgoing media area is expected to
// hold: generous for a handful of large attachments awaiting a retry,
// far below the 1 GiB downloaded media is allowed. Nothing is evicted
// to enforce it; going over it only raises a doctor warning, since a
// pending or failed message's attachment is kept until it is sent or
// the user deletes it (see cache.Outgoing and docs/decisions.md).
const outgoingSizeLimit = 256 << 20

// outgoingSweepInterval is how often the outgoing media area is swept
// for orphaned copies while the helper runs, on top of once at startup.
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
	caches := newMediaCaches(cfg.dataDir, db)
	commands, ingest, manager, err := wire(ctx, wireDeps{
		db: db, srv: srv, registry: registry, caches: caches, logger: logger,
		dataDir: cfg.dataDir, dbPath: cfg.dbPath,
	})
	if err != nil {
		return err
	}

	// Serve before the connectors start, so the UI sees their first events.
	// On every return, stop the connectors and the reminder and retry
	// schedulers and finish requests in progress before the database closes.
	srv.Start(ctx, stdio{s.in, s.out}, commands)

	var background sync.WaitGroup
	defer func() {
		cancel()
		srv.Wait()
		manager.Wait()
		background.Wait()
	}()

	runBackground(ctx, &background, commands, caches.outgoing, logger)

	if err := manager.Start(ctx, db, ingest); err != nil && ctx.Err() == nil {
		return fmt.Errorf("start connectors: %w", err)
	}

	logger.Printf("OmaMessenger helper %s started", helperVersion)
	srv.Wait()

	return nil
}

// mediaCaches are the two directories the helper keeps media in:
// downloaded media, pruned once it grows past a limit, and outgoing
// attachments, kept until their message is sent or deleted.
type mediaCaches struct {
	downloaded *cache.Cache
	outgoing   *cache.Outgoing
}

// newMediaCaches builds both of the helper's media areas under dir,
// the outgoing one backed by db to tell a real pending or failed
// message's copy from an orphan Sweep may remove.
func newMediaCaches(dir string, db *store.Store) mediaCaches {
	exists := func(ctx context.Context, id string) (bool, error) { return db.MessageExists(ctx, id) }

	return mediaCaches{
		downloaded: cache.New(filepath.Join(dir, "media"), mediaCacheLimit),
		outgoing:   cache.NewOutgoing(filepath.Join(dir, "media", "outgoing"), outgoingOrphanGrace, outgoingSizeLimit, exists),
	}
}

// runBackground starts the helper's own background work - the reminder
// and retry schedulers, and the outgoing media area's sweep - each in
// the given WaitGroup, so serve's own deferred cleanup waits for all of
// them to stop.
func runBackground(ctx context.Context, wg *sync.WaitGroup, commands *app.Commands, outgoing *cache.Outgoing, logger *log.Logger) {
	wg.Go(func() { commands.RunReminders(ctx) })
	wg.Go(func() { commands.RunRetries(ctx) })
	wg.Go(func() { outgoing.RunSweeper(ctx, outgoingSweepInterval, logger) })
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
	deps.Dispatcher, deps.SignIn, deps.History, deps.Media, deps.Refresher, deps.Organizer, deps.Reactor, deps.Deleter, deps.Members = manager, manager, manager, manager, manager, manager, manager, manager, manager
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
