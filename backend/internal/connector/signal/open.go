package signal

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// linkDeps bundles openDevice's own three callbacks into one
// parameter, since otherwise it would carry more arguments than the
// project's own limit on a function's parameter list allows.
type linkDeps struct {
	handler   func(events.SignalEvent) bool
	report    func(connector.AuthStep)
	provision provisioner
}

// openDevice opens the account's session database in dir, linking it
// by QR code first if it has never linked, and returns the
// connected-phase client, reporting whether this call just completed a
// fresh link.
func openDevice(ctx context.Context, dir, accountID string, deps linkDeps) (device, bool, error) {
	session, err := openSession(ctx, dir, accountID)
	if err != nil {
		return nil, false, fmt.Errorf("signal: open session: %w", err)
	}

	if aci, ok := session.linkedACI(); ok {
		dev, err := loadLinkedDevice(ctx, session, aci, deps.handler)
		if err == nil {
			return dev, false, nil
		}
		// The remembered account identifier no longer has a usable
		// session row; fall through and link again.
	}

	return linkAndLoadDevice(ctx, session, deps)
}

// loadLinkedDevice loads the session row already linked as aci and
// wraps it as a connected-phase client.
func loadLinkedDevice(ctx context.Context, session *sessionStore, aci uuid.UUID, handler func(events.SignalEvent) bool) (device, error) {
	dev, err := session.DeviceByACI(ctx, aci)
	if err != nil {
		return nil, fmt.Errorf("signal: load session: %w", err)
	}
	if dev == nil {
		return nil, fmt.Errorf("signal: no session row for the linked account")
	}

	return newSigDevice(dev, session, handler), nil
}

// linkAndLoadDevice links a new device by QR code, remembers which
// account it linked as, and wraps the result as a connected-phase
// client.
func linkAndLoadDevice(ctx context.Context, session *sessionStore, deps linkDeps) (device, bool, error) {
	data, err := link(ctx, session, deps.provision, deps.report)
	if err != nil {
		return nil, false, err
	}

	if err := session.saveLinkedACI(data.ACI); err != nil {
		return nil, false, fmt.Errorf("signal: save linked account: %w", err)
	}

	dev, err := session.DeviceByACI(ctx, data.ACI)
	if err != nil {
		return nil, false, fmt.Errorf("signal: load linked session: %w", err)
	}
	if dev == nil {
		return nil, false, fmt.Errorf("signal: no session row after linking")
	}

	return newSigDevice(dev, session, deps.handler), true, nil
}
