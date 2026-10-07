// Package fake provides scripted connectors with seeded conversations, so
// tests can drive the whole helper and UI without a WhatsApp or Telegram
// account. The helper includes it only when built with the fake tag.
package fake

import (
	"context"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Errors returned by fake connectors.
var (
	ErrNotRunning          = errors.New("fake connector is not running")
	ErrAlreadyRunning      = errors.New("fake connector is already running")
	ErrUnknownConversation = errors.New("not a fake conversation")
)

// Suite is the set of fake accounts one helper runs, and lets a test
// deliver a message as if someone had sent it.
type Suite struct {
	connectors []*Connector
}

// New creates the fake accounts.
func New() *Suite {
	return newSuite(scripts)
}

// NewDemo creates the small, curated set of fake accounts the README demo
// recording uses, instead of the fuller fixture New gives every test.
func NewDemo() *Suite {
	return newSuite(demoScripts)
}

// newSuite builds a Suite from a list of account scripts.
func newSuite(accounts []accountScript) *Suite {
	s := &Suite{}
	for _, script := range accounts {
		s.connectors = append(s.connectors, newConnector(script))
	}

	return s
}

// Connectors returns one connector per fake account.
func (s *Suite) Connectors() []connector.Connector {
	out := make([]connector.Connector, len(s.connectors))
	for i, c := range s.connectors {
		out[i] = c
	}

	return out
}

// Inject delivers a scripted message into a fake conversation at once, as
// if someone had just sent it.
func (s *Suite) Inject(ctx context.Context, conversationRemoteID string) (domain.Message, error) {
	for _, c := range s.connectors {
		if _, ok := c.script.find(conversationRemoteID); ok {
			return c.inject(ctx, conversationRemoteID)
		}
	}

	return domain.Message{}, fmt.Errorf("fake: inject into %q: %w", conversationRemoteID, ErrUnknownConversation)
}
