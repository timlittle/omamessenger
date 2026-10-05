// Package app coordinates normalized conversations, storage, connectors and
// user-facing events. Commands serves the UI; Ingest receives connector
// updates. Neither contains service protocol code.
package app

import (
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

var (
	ErrBadRequest    = errors.New("bad request")
	ErrUnknownMethod = errors.New("unknown method")
	errNoDispatcher  = errors.New("connector dispatcher is unavailable")
)

// Config holds the application's dependencies and mode.
type Config struct {
	Repo     Repository
	Notifier Notifier
	Clock    connector.Clock
	Emit     Emit
	Version  string
	Demo     bool
	// DemoInject and SetChatter are used only when Demo is set.
	DemoInject DemoInjector
	SetChatter func(enabled bool)
}

// New builds the two halves of the application, which share state and the
// event stream. Notifications, previews and demo chatter start enabled.
// Commands needs a Dispatcher before it can send; see AttachDispatcher.
func New(cfg Config) (*Commands, *Ingest) {
	if cfg.Clock == nil {
		cfg.Clock = connector.RealClock{}
	}
	pub := &publisher{repo: cfg.Repo, emit: cfg.Emit}
	sess := newSession()
	cmd := &Commands{
		repo: cfg.Repo, pub: pub, session: sess, clock: cfg.Clock, version: cfg.Version,
		demo: cfg.Demo, demoInject: cfg.DemoInject, setChatter: cfg.SetChatter,
	}
	ingest := &Ingest{repo: cfg.Repo, pub: pub, session: sess, notifier: cfg.Notifier, clock: cfg.Clock}
	return cmd, ingest
}
