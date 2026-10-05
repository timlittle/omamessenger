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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Stdin, os.Stdout, os.Stderr, os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "OmaMessenger helper:", err)
		os.Exit(1)
	}
}

func run(parent context.Context, stdin io.Reader, stdout, stderr io.Writer, args []string, env func(string) string) error {
	cfg, err := resolveConfig(args, env)
	if err != nil {
		return err
	}
	if cfg.Version {
		_, err := fmt.Fprintln(stdout, helperVersion)
		return err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	clock := connector.RealClock{}
	var connectors []connector.Connector
	if cfg.Demo {
		connectors = demo.New(clock, rand.New(rand.NewSource(cfg.Seed)), cfg.Chatter)
	}
	service := app.New(db, nil, notify.Desktop{}, clock)
	service.Demo = cfg.Demo
	service.SetChatter = demo.SetChatter
	if cfg.Demo {
		service.DemoInject = demo.NewInjector(connectors...)
	}
	manager := &connector.Manager{Store: db, Sink: service, Clock: clock, Connectors: connectors}
	service.Manager = manager
	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("start connectors: %w", err)
	}
	defer manager.Wait()
	stream := rpc.NewStream(stdout)
	service.Emit = func(name string, data any) { _ = stream.Emit(name, data) }
	log.New(stderr, "", 0).Println("OmaMessenger helper started")
	if err := rpc.Serve(ctx, stdin, stream, api.Register(service), api.Code); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("serve local API: %w", err)
	}
	cancel()
	manager.Wait()
	return nil
}
