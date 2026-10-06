// Package demo provides scripted connectors with seeded conversations, so
// the UI can be developed and tried without a WhatsApp or Telegram account.
package demo

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync/atomic"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Errors returned by demo connectors.
var (
	ErrNotRunning          = errors.New("demo connector is not running")
	ErrAlreadyRunning      = errors.New("demo connector is already running")
	ErrUnknownConversation = errors.New("not a demo conversation")
)

// Suite is the set of demo accounts one helper runs, with the controls the
// demo UI uses: background chatter and injected messages.
type Suite struct {
	connectors []*Connector
	chatter    atomic.Bool
}

// New creates the demo accounts. seed makes background chatter repeatable;
// chatter turns it on.
func New(seed uint64, chatter bool) *Suite {
	s := &Suite{}
	s.chatter.Store(chatter)

	for i, script := range scripts {
		rng := rand.New(rand.NewPCG(seed, uint64(i)))
		s.connectors = append(s.connectors, newConnector(script, &s.chatter, rng))
	}

	return s
}

// Connectors returns one connector per demo account.
func (s *Suite) Connectors() []connector.Connector {
	out := make([]connector.Connector, len(s.connectors))
	for i, c := range s.connectors {
		out[i] = c
	}

	return out
}

// SetChatter turns background chatter on or off without a restart.
func (s *Suite) SetChatter(enabled bool) {
	s.chatter.Store(enabled)
}

// Inject delivers a scripted message into a demo conversation at once, as
// if someone had just sent it.
func (s *Suite) Inject(ctx context.Context, conversationRemoteID string) (domain.Message, error) {
	for _, c := range s.connectors {
		if _, ok := c.script.find(conversationRemoteID); ok {
			return c.inject(ctx, conversationRemoteID)
		}
	}

	return domain.Message{}, fmt.Errorf("demo: inject into %q: %w", conversationRemoteID, ErrUnknownConversation)
}
