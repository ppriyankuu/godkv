// Package store contains the core storage engine of our distributed key-value system.
//
// This store:
//   - Keeps data in memory (fast reads/writes)
//   - Persists every write to disk using a Write-Ahead Log (WAL)
//   - Periodically creates full snapshots to speed up recovery
//
// Big idea:
//
//  1. WAL (Write-Ahead Log)
//     Every write is first written to disk before updating memory.
//     If the process crashes, we replay the WAL to rebuild the state.
//     This is how real databases like PostgreSQL and MySQL stay safe.
//
//  2. Snapshot
//     Instead of replaying the entire WAL from the beginning of time,
//     we sometimes save the full in-memory state to disk.
//     After that, we only need to replay newer WAL entries.
//
//  3. Concurrency
//     We use sync.RWMutex so:
//     - Many readers can read at the same time
//     - Only one writer can write at a time
//     This pattern works well for read-heavy systems.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Value represents one stored record in the key-value store.
//
// It contains:
//   - The actual data
//   - A vector clock (used to detect version conflicts between nodes)
//   - A tombstone flag (used for soft deletes in distributed replication)
//   - A timestamp for tie-breaking conflicts
//
// Why tombstone?
// In distributed systems, deletes must also be replicated.
// If we just removed the key, other nodes would not know it was deleted.
// So we mark it as deleted instead.
type Value struct {
	Data      string      `json:"data"`
	Clock     VectorClock `json:"clock"`      // Version information for conflict detection
	Tombstone bool        `json:"tombstone"`  // Marks a soft delete
	UpdatedAt time.Time   `json:"updated_at"` // Used as tie-breaker in conflicts
}

// Store is the main storage object.
//
// It is safe for concurrent use.
//
// Fields:
//   - mu: mutex to protect access to the map
//   - data: in-memory key-value storage
//   - wal: write-ahead log for durability
//   - dataDir: folder where snapshot and WAL are stored
//   - nodeID: unique ID of this node (used in vector clocks)
type Store struct {
	mu      sync.RWMutex
	data    map[string]Value
	wal     *WAL
	dataDir string
	nodeID  string
}

// New creates or opens a Store.
//
// Startup process:
//
// 1) Create the data directory (if it doesn't exist)
// 2) Load the latest snapshot into memory
// 3) Open the WAL file
// 4) Replay WAL entries written after the snapshot
//
// After this finishes, the store is fully rebuilt in memory.
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

// Put stores or updates a key.
//
// Steps:
//  1. Lock for writing
//  2. Increment this node's vector clock
//  3. Write the operation to the WAL (disk first!)
//  4. Update the in-memory map
//
// Important rule:
//
//	We ALWAYS write to WAL before changing memory.
//	This guarantees crash safety.
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

// Get returns the value for a key.
//
// If the key does not exist OR
// if it was deleted (tombstone),
// it returns (Value{}, false).
//
// This hides tombstones from normal reads.
func (s *Store) Get(key string) (Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[key]
	if !ok || v.Tombstone {
		return Value{}, false
	}
	return v, true
}

// GetRaw returns the stored Value exactly as it exists,
// including tombstones.
//
// This is used internally for replication so that
// deletes can be propagated across nodes.
func (s *Store) GetRaw(key string) (Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// Delete performs a soft delete.
//
// Instead of removing the key from memory,
// we store a new Value with Tombstone = true.
//
// Why?
// Because deletes must also replicate to other nodes.
// A tombstone tells other replicas: "this key was deleted".
//
// Just like Put:
//   - We increment vector clock
//   - We write to WAL first
//   - Then update memory
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

// ApplyRemote applies an update received from another node.
//
// This is part of replication.
//
// We compare vector clocks to decide:
//
//   - If incoming is older → ignore it
//   - If incoming is newer → accept it
//   - If both are concurrent (true conflict):
//     use UpdatedAt as a tie-breaker
//
// This approach is:
//
//	"Vector clocks for causality + last-write-wins for conflicts"
//
// In a real production system, we might return conflicts
// to the application instead of auto-resolving.
func (s *Store) ApplyRemote(key string, incoming Value) (applied bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.data[key]
	if ok {
		rel := incoming.Clock.Compare(existing.Clock)
		switch rel {
		case ConcurrentClocks:
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

// Keys returns all keys that are NOT tombstoned.
//
// We do not expose deleted keys to users.
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

// Snapshot saves the entire in-memory state to disk.
//
// Steps:
//  1. Copy in-memory map (while holding read lock)
//  2. Write it to a temporary file
//  3. Atomically rename it to snapshot.json
//  4. Truncate the WAL (since snapshot now contains everything)
//
// Why atomic rename?
// If we crash during write, the old snapshot remains safe.
//
// After snapshot:
//
//	Recovery is much faster because we replay fewer WAL entries.
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

// loadSnapshot loads snapshot.json (if it exists)
// and restores it into memory.
//
// If no snapshot exists, this is not an error.
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

// replayWAL reads all WAL entries
// and applies them to the in-memory map.
//
// Important:
// We DO NOT re-write them to the WAL again.
// We are only rebuilding memory.
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

// Close closes the WAL file.
// Call this during shutdown.
func (s *Store) Close() error {
	return s.wal.close()
}
