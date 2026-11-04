package config

import (
	"errors"
	"fmt"
	"sync"

	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// Store is the concurrency-safe owner of the profile configuration (the
// ProfileStore persisted as config.json) and of the active profile name.
//
// net/http serves each request on its own goroutine, so every handler that
// reads or changes profiles goes through a Store: reads return deep copies
// taken under a read lock, and writes run as transactions under the write
// lock — the callback mutates a private copy, the copy is saved, and only then
// does it replace the live data. A failed callback or save leaves both memory
// and disk unchanged. The underlying maps are never handed out.
//
// An ephemeral Store (NewEphemeralStore) applies changes in memory only and
// never touches the disk; boot falls back to one when config.json is unreadable.
//
// Environment overrides (OpenStore) are kept apart from the persisted data:
// Effective applies them on top of a profile for the live providers, and they
// are never saved.
type Store struct {
	mu         sync.RWMutex
	data       ProfileStore
	configHome string
	ephemeral  bool
	env        PathSettings // immutable after construction
}

// NewStore returns a Store persisted to config.json in configHome, holding a
// private deep copy of data.
func NewStore(configHome string, data ProfileStore) *Store {
	return &Store{data: cloneProfileStore(data), configHome: configHome}
}

// NewEphemeralStore returns a Store that is never written to disk.
func NewEphemeralStore(data ProfileStore) *Store {
	return &Store{data: cloneProfileStore(data), ephemeral: true}
}

// Ephemeral reports whether the store never persists its changes.
func (s *Store) Ephemeral() bool { return s.ephemeral }

// ActiveProfileName returns the name of the active profile.
func (s *Store) ActiveProfileName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.ActiveProfile
}

// Snapshot returns a deep copy of the persisted configuration.
func (s *Store) Snapshot() ProfileStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProfileStore(s.data)
}

// Profile returns a deep copy of the named profile as persisted.
func (s *Store) Profile(name string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.data.Profiles[name]
	return cloneProfile(p), ok
}

// ActiveProfile returns the active profile name and a deep copy of it.
func (s *Store) ActiveProfile() (string, Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.data.Profiles[s.data.ActiveProfile]
	return s.data.ActiveProfile, cloneProfile(p), ok
}

// ProfileNames returns the names of every profile, in no particular order.
func (s *Store) ProfileNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.data.Profiles))
	for name := range s.data.Profiles {
		names = append(names, name)
	}
	return names
}

// CCAccounts returns a copy of the tracked Claude Code accounts.
func (s *Store) CCAccounts() []CCAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]CCAccount(nil), s.data.CCAccounts...)
}

// Update runs fn on a private copy of the configuration under the write lock,
// persists the copy, and commits it. When fn or the save fails nothing changes.
func (s *Store) Update(fn func(*ProfileStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateLocked(fn)
}

// UpdateActive runs fn on a private copy of the active profile under the write
// lock and commits it like Update. Holding the lock for the whole callback
// guarantees the profile fn changes is still the active one when it is saved,
// even if a profile switch is requested concurrently; fn may therefore call
// into the provider registry (the lock order is always Store, then Registry).
func (s *Store) UpdateActive(fn func(name string, p *Profile) error) error {
	return s.Update(func(st *ProfileStore) error {
		name := st.ActiveProfile
		p, ok := st.Profiles[name]
		if !ok {
			return fmt.Errorf("active profile %q not found", name)
		}
		if err := fn(name, &p); err != nil {
			return err
		}
		st.Profiles[name] = p
		return nil
	})
}

// SwitchActive makes name the active profile. apply receives the next profile
// (with environment overrides applied, see Effective) and must bring the live providers in line with it (Registry.SwitchProfile);
// it runs under the write lock, so no other profile change interleaves. When
// apply fails nothing changes. When the save fails, apply is called again with
// the previous profile to restore the live providers and the error is returned
// (joined with the restore error if that fails too).
func (s *Store) SwitchActive(name string, apply func(next Profile) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next, ok := s.data.Profiles[name]
	if !ok {
		return usererr.New(fmt.Sprintf("profile %q not found", name))
	}
	prevName := s.data.ActiveProfile
	if err := apply(s.Effective(next)); err != nil {
		return err
	}
	err := s.updateLocked(func(st *ProfileStore) error {
		st.ActiveProfile = name
		return nil
	})
	if err != nil {
		if prev, ok := s.data.Profiles[prevName]; ok {
			if rerr := apply(s.Effective(prev)); rerr != nil {
				// The live providers may now follow the unsaved profile while
				// the store still names the previous one: report both.
				return errors.Join(err, fmt.Errorf("restore profile %q: %w", prevName, rerr))
			}
		}
		return err
	}
	return nil
}

// AddCCAccount adds a Claude Code account with a unique slug ID and persists it.
func (s *Store) AddCCAccount(label, path string) (CCAccount, error) {
	var acc CCAccount
	err := s.Update(func(st *ProfileStore) error {
		var err error
		acc, err = addCCAccount(st, label, path)
		return err
	})
	return acc, err
}

// UpdateCCAccount changes an account's label, path and enabled flag.
func (s *Store) UpdateCCAccount(id, label, path string, enabled bool) error {
	return s.Update(func(st *ProfileStore) error {
		return updateCCAccount(st, id, label, path, enabled)
	})
}

// RemoveCCAccount removes an account; the last account cannot be removed.
func (s *Store) RemoveCCAccount(id string) error {
	return s.Update(func(st *ProfileStore) error {
		return removeCCAccount(st, id)
	})
}

// SeedDefaultCCAccount persists a single "Personal" account pointing at path
// when no account exists yet. It is a no-op otherwise.
func (s *Store) SeedDefaultCCAccount(path string) error {
	s.mu.RLock()
	seeded := len(s.data.CCAccounts) > 0
	s.mu.RUnlock()
	if seeded {
		return nil
	}
	return s.Update(func(st *ProfileStore) error {
		SeedDefaultAccount(st, path)
		return nil
	})
}

// updateLocked is Update without locking; the caller holds s.mu for writing.
func (s *Store) updateLocked(fn func(*ProfileStore) error) error {
	next := cloneProfileStore(s.data)
	if next.Profiles == nil {
		next.Profiles = make(map[string]Profile)
	}
	if err := fn(&next); err != nil {
		return err
	}
	if !s.ephemeral {
		if err := SaveProfileStore(s.configHome, &next); err != nil {
			return err
		}
	}
	s.data = next
	return nil
}
