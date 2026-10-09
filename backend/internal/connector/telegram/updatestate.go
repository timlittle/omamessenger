package telegram

// updatestate.go persists gotd's update-manager state (pts/qts/seq/date,
// each channel's own pts, and the access hashes the manager needs to
// address a channel or a user again) to a file in the account's own
// directory, the same place its session and credentials already live.
// Without this, updates.Manager keeps that state only in memory, so a
// helper restart always starts it from nothing: the manager treats a
// blank state as "forget", fetches only the server's current position,
// and every update from the gap while the helper was not running is
// lost for good. With it saved, updates.Manager calls
// updates.getDifference against the saved position on every start and
// dispatches whatever happened in between through the same handlers a
// live update reaches, exactly once.
//
// A single JSON file was chosen over a table in the shared SQLite store
// because this state belongs to gotd's own update manager, not to
// anything the rest of the helper reads: it is written on nearly every
// incoming update, so giving it its own file, read and rewritten whole
// the same way gotd's own session.FileStorage already works, needs no
// migration and cannot contend with the store's other tables.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/gotd/td/telegram/updates"
)

// errUpdateStateMissing reports a pts, qts, date or seq update for an
// account whose state was never saved with SetState, matching what
// updates.Manager expects these setters to return when that happens.
var errUpdateStateMissing = errors.New("telegram: update state: account state not found")

// updateStateFile is the on-disk shape of one account's saved update
// state: gotd keys everything by the Telegram user id passed to
// updates.Manager.Run, so the maps keep that shape even though this
// file in practice only ever holds the one account it belongs to.
type updateStateFile struct {
	States      map[int64]updates.State   `json:"states"`
	ChannelPts  map[int64]map[int64]int   `json:"channelPts"`
	ChannelHash map[int64]map[int64]int64 `json:"channelAccessHashes"`
	UserHash    map[int64]map[int64]int64 `json:"userAccessHashes"`
}

// emptyUpdateStateFile returns update state with nothing saved yet.
func emptyUpdateStateFile() updateStateFile {
	return updateStateFile{
		States:      map[int64]updates.State{},
		ChannelPts:  map[int64]map[int64]int{},
		ChannelHash: map[int64]map[int64]int64{},
		UserHash:    map[int64]map[int64]int64{},
	}
}

// fillMissingMaps replaces any nil map left by decoding JSON that
// omitted an empty field, so every other method can assume they exist.
func (f *updateStateFile) fillMissingMaps() {
	if f.States == nil {
		f.States = map[int64]updates.State{}
	}
	if f.ChannelPts == nil {
		f.ChannelPts = map[int64]map[int64]int{}
	}
	if f.ChannelHash == nil {
		f.ChannelHash = map[int64]map[int64]int64{}
	}
	if f.UserHash == nil {
		f.UserHash = map[int64]map[int64]int64{}
	}
}

// updateStorage is a gotd update manager's state storage and access-hash
// stores, all three backed by the one file at path.
type updateStorage struct {
	path string

	mu   sync.Mutex
	data updateStateFile
}

var (
	_ updates.StateStorage        = (*updateStorage)(nil)
	_ updates.ChannelAccessHasher = (*updateStorage)(nil)
	_ updates.UserAccessHasher    = (*updateStorage)(nil)
)

// updateStatePath is where an account's update-manager state lives.
func updateStatePath(dir, accountID string) string {
	return filepath.Join(dir, accountID+".updates.json")
}

// newUpdateStorage returns update storage backed by path, loading
// whatever an earlier run already saved there, or starting empty when
// nothing has been saved yet. A file that cannot be read or parsed -
// left truncated by a crash mid-write, say - is discarded rather than
// returned as an error: failing here would stop this account's Run from
// ever starting again, where starting fresh just costs one resync, the
// same recovery updates.Manager already performs for a gap it cannot
// otherwise bridge.
func newUpdateStorage(path string) (*updateStorage, error) {
	s := &updateStorage{path: path, data: emptyUpdateStateFile()}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		log.Printf("telegram: update state unreadable, starting fresh and resyncing")

		return s, nil //nolint:nilerr // deliberate: see the doc comment above, starting fresh just costs one resync
	}

	if err := json.Unmarshal(raw, &s.data); err != nil {
		log.Printf("telegram: update state corrupt, discarding and resyncing")
		s.data = emptyUpdateStateFile()

		return s, nil //nolint:nilerr // deliberate: see the doc comment above, starting fresh just costs one resync
	}
	s.data.fillMissingMaps()

	return s, nil
}

// GetState returns the saved pts, qts, date and seq for userID.
func (s *updateStorage) GetState(_ context.Context, userID int64) (updates.State, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.data.States[userID]

	return state, ok, nil
}

// SetState replaces userID's saved state and forgets its channels, the
// same reset updates.Manager expects of a fresh forced resync.
func (s *updateStorage) SetState(_ context.Context, userID int64, state updates.State) error {
	return s.update(func(d *updateStateFile) error {
		d.States[userID] = state
		d.ChannelPts[userID] = map[int64]int{}

		return nil
	})
}

// SetPts updates userID's saved pts.
func (s *updateStorage) SetPts(_ context.Context, userID int64, pts int) error {
	return s.updateState(userID, func(state *updates.State) { state.Pts = pts })
}

// SetQts updates userID's saved qts.
func (s *updateStorage) SetQts(_ context.Context, userID int64, qts int) error {
	return s.updateState(userID, func(state *updates.State) { state.Qts = qts })
}

// SetDate updates userID's saved date.
func (s *updateStorage) SetDate(_ context.Context, userID int64, date int) error {
	return s.updateState(userID, func(state *updates.State) { state.Date = date })
}

// SetSeq updates userID's saved seq.
func (s *updateStorage) SetSeq(_ context.Context, userID int64, seq int) error {
	return s.updateState(userID, func(state *updates.State) { state.Seq = seq })
}

// SetDateSeq updates userID's saved date and seq together.
func (s *updateStorage) SetDateSeq(_ context.Context, userID int64, date, seq int) error {
	return s.updateState(userID, func(state *updates.State) { state.Date, state.Seq = date, seq })
}

// updateState applies mutate to userID's saved state and persists the
// result, reporting errUpdateStateMissing when SetState has never been
// called for userID, matching updates.StateStorage's documented
// contract for these setters.
func (s *updateStorage) updateState(userID int64, mutate func(*updates.State)) error {
	return s.update(func(d *updateStateFile) error {
		state, ok := d.States[userID]
		if !ok {
			return errUpdateStateMissing
		}

		mutate(&state)
		d.States[userID] = state

		return nil
	})
}

// GetChannelPts returns the saved pts for one of userID's channels.
func (s *updateStorage) GetChannelPts(_ context.Context, userID, channelID int64) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channels, ok := s.data.ChannelPts[userID]
	if !ok {
		return 0, false, nil
	}

	pts, found := channels[channelID]

	return pts, found, nil
}

// SetChannelPts updates the saved pts for one of userID's channels.
func (s *updateStorage) SetChannelPts(_ context.Context, userID, channelID int64, pts int) error {
	return s.update(func(d *updateStateFile) error {
		channels, ok := d.ChannelPts[userID]
		if !ok {
			return errUpdateStateMissing
		}

		channels[channelID] = pts

		return nil
	})
}

// ForEachChannels calls f for every channel saved for userID, with its
// last known pts.
func (s *updateStorage) ForEachChannels(ctx context.Context, userID int64, f func(ctx context.Context, channelID int64, pts int) error) error {
	s.mu.Lock()
	channels, ok := s.data.ChannelPts[userID]
	if !ok {
		s.mu.Unlock()

		return errUpdateStateMissing
	}
	copied := make(map[int64]int, len(channels))
	for id, pts := range channels {
		copied[id] = pts
	}
	s.mu.Unlock()

	for id, pts := range copied {
		if err := f(ctx, id, pts); err != nil {
			return err
		}
	}

	return nil
}

// GetChannelAccessHash returns the access hash userID has observed for
// channelID, so the manager can address it again without asking
// Telegram first.
func (s *updateStorage) GetChannelAccessHash(_ context.Context, userID, channelID int64) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hashes, ok := s.data.ChannelHash[userID]
	if !ok {
		return 0, false, nil
	}

	hash, found := hashes[channelID]

	return hash, found, nil
}

// SetChannelAccessHash records the access hash userID observed for
// channelID.
func (s *updateStorage) SetChannelAccessHash(_ context.Context, userID, channelID, accessHash int64) error {
	return s.update(func(d *updateStateFile) error {
		hashes, ok := d.ChannelHash[userID]
		if !ok {
			hashes = map[int64]int64{}
			d.ChannelHash[userID] = hashes
		}
		hashes[channelID] = accessHash

		return nil
	})
}

// GetUserAccessHash returns the access hash userID has observed for
// targetUserID.
func (s *updateStorage) GetUserAccessHash(_ context.Context, userID, targetUserID int64) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hashes, ok := s.data.UserHash[userID]
	if !ok {
		return 0, false, nil
	}

	hash, found := hashes[targetUserID]

	return hash, found, nil
}

// SetUserAccessHash records the access hash userID observed for
// targetUserID.
func (s *updateStorage) SetUserAccessHash(_ context.Context, userID, targetUserID, accessHash int64) error {
	return s.update(func(d *updateStateFile) error {
		hashes, ok := d.UserHash[userID]
		if !ok {
			hashes = map[int64]int64{}
			d.UserHash[userID] = hashes
		}
		hashes[targetUserID] = accessHash

		return nil
	})
}

// update applies mutate to the stored state under lock and writes the
// result to disk, so every change the update manager makes survives a
// restart; mutate's own error, when it returns one, is reported without
// writing anything.
func (s *updateStorage) update(mutate func(*updateStateFile) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := mutate(&s.data); err != nil {
		return err
	}

	raw, err := json.Marshal(s.data)
	if err != nil {
		return fmt.Errorf("telegram: save update state: %w", err)
	}

	if err := writeFileAtomically(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("telegram: save update state: %w", err)
	}

	return nil
}

// writeFileAtomically writes data to a temporary file beside path,
// fsyncs it so its content is actually on disk, and only then renames
// it over path. This is written on nearly every incoming update, so a
// crash or power loss mid-write must never leave path holding a
// truncated or half-written file: the rename either lands in full or
// not at all, and path keeps whatever it held before until it does.
func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	part := path + ".tmp"

	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		return errors.Join(err, f.Close(), removeIfPresent(part))
	}

	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close(), removeIfPresent(part))
	}

	if err := f.Close(); err != nil {
		return errors.Join(err, removeIfPresent(part))
	}

	if err := os.Rename(part, path); err != nil {
		return errors.Join(err, removeIfPresent(part))
	}

	return nil
}

// removeIfPresent removes path, treating one already gone as removed:
// cleanup after a failed atomic write must not mask the write's own
// error by failing itself just because there was nothing left to clean.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
