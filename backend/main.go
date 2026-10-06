package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"

	"github.com/timlittle/omamessenger/backend/internal/api"
	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/demo"
	"github.com/timlittle/omamessenger/backend/internal/notify"
	"github.com/timlittle/omamessenger/backend/internal/rpc"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func main() { // coverage-ignore: process entry point; run() is tested
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, ioStreams{in: os.Stdin, out: os.Stdout, errOut: os.Stderr}, os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "OmaMessenger helper:", err)
		os.Exit(1)
	}
}

// ioStreams are the helper's protocol input, protocol output and log output.
type ioStreams struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

// run resolves the configuration and serves the protocol until stdin ends or
// ctx is canceled (SIGTERM), which are both a clean exit.
func run(ctx context.Context, streams ioStreams, args []string, env func(string) string) error {
	cfg, err := resolveConfig(args, env)
	if err != nil {
		return err
	}
	if cfg.Version {
		_, err := fmt.Fprintln(streams.out, helperVersion)
		return err
	}
	return serve(ctx, cfg, streams)
}

func serve(parent context.Context, cfg Config, streams ioStreams) error {
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(parent)
	stream := rpc.NewStream(streams.out)
	commands, manager := wire(cfg, db, stream)
	if err := manager.Start(ctx); err != nil {
		cancel()
		return fmt.Errorf("start connectors: %w", err)
	}
	// Stop the connectors before waiting for them, on every return path.
	defer func() {
		cancel()
		manager.Wait()
	}()
	log.New(streams.errOut, "", 0).Printf("OmaMessenger helper %s started (demo: %t)", helperVersion, cfg.Demo)
	if err := rpc.Serve(ctx, streams.in, stream, api.Register(commands), api.Code); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("serve local API: %w", err)
	}
	return nil
}

// wire builds the application and the connector Manager around db, with
// events written to stream. Only demo connectors exist so far.
func wire(cfg Config, db *store.Store, stream *rpc.Stream) (*app.Commands, *connector.Manager) {
	clock := connector.RealClock{}
	appConfig := app.Config{
		Repo: db, Notifier: notify.Desktop{}, Clock: clock, Version: helperVersion, Demo: cfg.Demo,
		Emit: func(name string, data any) { _ = stream.Emit(name, data) },
	}
	var connectors []connector.Connector
	if cfg.Demo {
		connectors = demo.New(clock, rand.New(rand.NewSource(cfg.Seed)), cfg.Chatter)
		injector := demo.NewInjector(connectors...)
		appConfig.DemoInject = injector
		appConfig.SetChatter = injector.SetChatter
	}
	commands, ingest := app.New(appConfig)
	manager := &connector.Manager{Store: db, Sink: ingest, Clock: clock, Connectors: connectors}
	commands.AttachDispatcher(manager)
	return commands, manager
}
