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
	"syscall"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/notify"
	"github.com/timlittle/omamessenger/backend/internal/server"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

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
// is cancelled; both are a clean exit.
func run(ctx context.Context, s streams, args []string, env func(string) string) error {
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

	registry := &accountRegistry{db: db, dir: filepath.Join(cfg.dataDir, "telegram"), connect: telegramConnector}
	commands, ingest, manager, err := wire(ctx, db, srv, registry)
	if err != nil {
		return err
	}

	// Serve before the connectors start, so the UI sees their first events.
	// On every return, stop the connectors and finish requests in progress
	// before the database closes.
	srv.Start(ctx, stdio{s.in, s.out}, commands)
	defer func() {
		cancel()
		srv.Wait()
		manager.Wait()
	}()

	if err := manager.Start(ctx, db, ingest); err != nil && ctx.Err() == nil {
		return fmt.Errorf("start connectors: %w", err)
	}

	logger.Printf("OmaMessenger helper %s started", helperVersion)
	srv.Wait()

	return nil
}

// wire builds the application around db and srv, starting with the
// accounts already saved. Test builds add the fake connectors; see
// fake.go.
func wire(ctx context.Context, db *store.Store, srv *server.Server, registry *accountRegistry) (*app.Commands, *app.Ingest, *connector.Manager, error) {
	deps := app.Deps{Store: db, Notifier: notify.Desktop{}, Publisher: srv, Accounts: registry}

	connectors, injector := fakeConnectors()
	if injector != nil {
		deps.Fake = injector
	}

	saved, err := registry.saved(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	manager, err := connector.NewManager(append(connectors, saved...)...)
	if err != nil {
		return nil, nil, nil, err
	}

	registry.manager = manager
	deps.Dispatcher, deps.SignIn, deps.History = manager, manager, manager
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
