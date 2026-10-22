package whatsapp

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestResolveUndecryptable_ReportsAndForgetsAMarkedMessage confirms a
// marked message is reported found exactly once: resolving it a second
// time, or resolving a message never marked, reports not found.
func TestResolveUndecryptable_ReportsAndForgetsAMarkedMessage(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	c.markUndecryptable("conv", "msg")

	if !c.resolveUndecryptable("conv", "msg") {
		t.Error("resolveUndecryptable on a marked message = false, want true")
	}
	if c.resolveUndecryptable("conv", "msg") {
		t.Error("resolveUndecryptable on an already-resolved message = true, want false")
	}
	if c.resolveUndecryptable("conv", "never-marked") {
		t.Error("resolveUndecryptable on a message never marked = true, want false")
	}
}

// TestMarkUndecryptable_EvictsTheOldestMessageOnceFull confirms a
// sender whose messages never decrypt, or who names an endless stream
// of ids, cannot grow c.undecryptable forever: once
// maxUndecryptableTargets messages are tracked, the oldest is forgotten
// to make room for a new one.
func TestMarkUndecryptable_EvictsTheOldestMessageOnceFull(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	c.markUndecryptable("conv", "first")

	for i := 0; i < maxUndecryptableTargets; i++ {
		c.markUndecryptable("conv", "filler"+string(rune(i)))
	}

	if len(c.undecryptable) > maxUndecryptableTargets {
		t.Fatalf("tracked %d messages, want at most %d", len(c.undecryptable), maxUndecryptableTargets)
	}
	if c.resolveUndecryptable("conv", "first") {
		t.Error("the oldest message was not evicted once the cache filled up")
	}
}
