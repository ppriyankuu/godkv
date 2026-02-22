// Package store implements the core storage engine: an in-memory key-value
// store backed by a Write-Ahead Log (WAL) for crash safety and periodic
// snapshots for fast recovery.
//
// How it works (interview-ready explanation):
//
//  1. All writes go to the WAL first (disk) before being applied in-memory.
//     If the process crashes mid-write, on restart we replay the WAL to
//     reconstruct state — this is the same technique used by PostgreSQL/MySQL.
//
//  2. Snapshots periodically capture the full in-memory state to disk so
//     WAL replay starts from the snapshot, not from the beginning of time.
//     Without snapshots the WAL would grow unbounded and recovery would be slow.
//
//  3. sync.RWMutex gives us concurrent reads (multiple goroutines can hold
//     RLock simultaneously) while serialising writes (Lock is exclusive).
//     This is the standard Go pattern for a read-heavy cache.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Value wraps a stored value with metadata needed for distributed consistency.
type Value struct {
	Data      string      `json:"data"`
	Clock     VectorClock `json:"clock"`     // version vector for conflict detection
	Tombstone bool        `json:"tombstone"` // soft-delete marker (needed so deletes replicate)
	UpdatedAt time.Time   `json:"updated_at"`
}

// Store is the primary storage abstraction.  It is safe for concurrent use.
type Store struct {
	mu      sync.RWMutex
	data    map[string]Value
	wal     *WAL
	dataDir string
	nodeID  string
}

// New opens (or creates) a store rooted at dataDir.
// On first open it replays any existing WAL entries on top of the last snapshot.
func New(dataDir, nodeID string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	s := &Store{
		data:    make(map[string]Value),
		dataDir: dataDir,
		nodeID:  nodeID,
	}

	// Step 1: load snapshot (if any) into memory.
	if err := s.loadSnapshot(); err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	// Step 2: open WAL and replay any entries written after the last snapshot.
	wal, err := newWAL(filepath.Join(dataDir, "wal.log"))
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
	}
	s.wal = wal

	if err := s.replayWAL(); err != nil {
		return nil, fmt.Errorf("replay wal: %w", err)
	}

	return s, nil
}

// ─── Public API ───────────────────────────────────────────────────────────────

// Put stores a value.  The caller supplies a VectorClock; if nil a fresh one
// is created stamped with nodeID.
func (s *Store) Put(key, data string, clock VectorClock) (Value, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if clock == nil {
		clock = make(VectorClock)
	}
	clock.Increment(s.nodeID) // bump our own counter on every write

	v := Value{
		Data:      data,
		Clock:     clock,
		Tombstone: false,
		UpdatedAt: time.Now().UTC(),
	}

	// WAL-first: persist before mutating memory.
	entry := walEntry{Op: opPut, Key: key, Value: v}
	if err := s.wal.append(entry); err != nil {
		return Value{}, fmt.Errorf("wal append: %w", err)
	}

	s.data[key] = v
	return v, nil
}

// Get returns the value for key.  Returns (Value{}, false) if not found or deleted.
func (s *Store) Get(key string) (Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[key]
	if !ok || v.Tombstone {
		return Value{}, false
	}
	return v, true
}

// GetRaw returns the raw Value including tombstones — used internally for
// replication and read repair so we can propagate deletes.
func (s *Store) GetRaw(key string) (Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// Delete soft-deletes a key by writing a tombstone.  The tombstone is
// replicated to followers so they also remove the key.
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.data[key]
	clock := make(VectorClock)
	if ok {
		clock = existing.Clock.Copy()
	}
	clock.Increment(s.nodeID)

	v := Value{
		Clock:     clock,
		Tombstone: true,
		UpdatedAt: time.Now().UTC(),
	}

	entry := walEntry{Op: opDelete, Key: key, Value: v}
	if err := s.wal.append(entry); err != nil {
		return fmt.Errorf("wal append: %w", err)
	}

	s.data[key] = v
	return nil
}

// ApplyRemote applies a value received from a peer during replication.
// It uses the vector clock to decide whether to accept or discard the update.
// This is the core of "last-write-wins with vector clocks."
func (s *Store) ApplyRemote(key string, incoming Value) (applied bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.data[key]
	if ok {
		rel := incoming.Clock.Compare(existing.Clock)
		switch rel {
		case ConcurrentClocks:
			// True conflict: both clocks are incomparable.  We merge by taking
			// the later wall-clock time as a tiebreaker — a pragmatic choice
			// used by many real systems (Cassandra, Riak).  A production system
			// could surface the conflict to the application instead.
			if incoming.UpdatedAt.Before(existing.UpdatedAt) {
				return false, nil // keep existing
			}
		case Before:
			// Incoming is strictly older — discard it.
			return false, nil
			// After or Equal: incoming wins, fall through.
		}
	}

	entry := walEntry{Op: opPut, Key: key, Value: incoming}
	if err := s.wal.append(entry); err != nil {
		return false, err
	}
	s.data[key] = incoming
	return true, nil
}

// Keys returns a snapshot of all live (non-tombstoned) keys.
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.data))
	for k, v := range s.data {
		if !v.Tombstone {
			keys = append(keys, k)
		}
	}
	return keys
}

// ─── Snapshot ─────────────────────────────────────────────────────────────────

// Snapshot serialises the current in-memory state to disk and truncates the
// WAL.  Call this periodically (e.g. every N writes or on a timer).
func (s *Store) Snapshot() error {
	s.mu.RLock()
	snapshot := make(map[string]Value, len(s.data))
	for k, v := range s.data {
		snapshot[k] = v
	}
	s.mu.RUnlock()

	path := filepath.Join(s.dataDir, "snapshot.json")
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(snapshot); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// Atomic rename: if we crash between Create and Rename the old snapshot
	// is still valid.
	if err := os.Rename(tmp, path); err != nil {
		return err
	}

	// Truncate WAL — everything is now captured in the snapshot.
	return s.wal.truncate()
}

func (s *Store) loadSnapshot() error {
	path := filepath.Join(s.dataDir, "snapshot.json")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil // no snapshot yet — that's fine
	}
	if err != nil {
		return err
	}
	defer f.Close()

	var snapshot map[string]Value
	if err := json.NewDecoder(f).Decode(&snapshot); err != nil {
		return err
	}
	s.data = snapshot
	return nil
}

func (s *Store) replayWAL() error {
	entries, err := s.wal.readAll()
	if err != nil {
		return err
	}
	for _, e := range entries {
		// Apply directly without re-writing to WAL.
		s.data[e.Key] = e.Value
	}
	return nil
}

// Close flushes everything and closes underlying files.
func (s *Store) Close() error {
	return s.wal.close()
}
