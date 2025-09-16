package store

import (
	"fmt"
	"sync"
	"time"
)

// storage engine for a single node
// define a 'store' that holds key-value data in memory.

// to prevent data loss during program crashes,
// 2 common db techniques are used:

// Write-Ahead Log (WAL): every change (put, delete) is first written to a
// log file on disk before it's applied to the in-memory map. if the server
// restarts, it can "replay" this log to reconstruct its state

// Snapshots: periodically the entire in-memory data is saved to a "snapshot" file.
// on the next restart, the program can load the latest snapshot and then replay
// only the WAL entires that occured after the snaptshot.

// Version represents the data's version using a Vector Clock
// and a physical timestamp.
type Version struct {
	Clock     map[string]int64 `json:"clock"`
	Timestamp int64            `json:"timestamp"`
}

// Entry is the atomic unit of storage.
// the 'Deleted' field signifies deletion without removing the key
// necessary for replicating deletes correctly
type Entry struct {
	Key     string  `json:"key"`
	Value   string  `json:"value"`
	Version Version `json:"version"`
	Deleted bool    `json:"deleted"`
}

// Store is the main storage engine for a node
type Store struct {
	// in-memory map for fast O(1) key lookups
	data map[string]*Entry
	// for thread-safety
	mu sync.RWMutex
	// WAL struct
	wal    *WAL
	nodeID string
	// Snapshot struct
	snapMgr *SnapshotManager
	// tracks operational counts
	metrics *Metrics
}

type Metrics struct {
	Reads   int64
	Writes  int64
	Deletes int64
}

// NewStore initializes the store
// the startup process:
// 1. initialize WAL and Snapshot manager
// 2. load the most recent snapshot into memory
// 3. replay the WAL to apply any changes made after the snapshot was taken
func NewStore(nodeId string, walPath string) (*Store, error) {
	wal, err := NewWAL(walPath)
	if err != nil {
		return nil, err
	}

	snapMgr := NewSnapshotManager(fmt.Sprintf("%s.snapshot", walPath))

	store := &Store{
		data:    make(map[string]*Entry),
		nodeID:  nodeId,
		wal:     wal,
		snapMgr: snapMgr,
		metrics: &Metrics{},
	}

	// load the snapshort first
	if err := store.loadSnapshot(); err != nil {
		fmt.Printf("Failed to load snapshot: %v\n", err)
	}

	// replay WAL
	if err := store.replayWAL(); err != nil {
		return nil, fmt.Errorf("failed to replay WAL: %w", err)
	}

	return store, nil
}

// Put inserts or updates a key
// follows the WAL-first principle
// 1. acquire a write lock to ensure exclusive access
// 2. write the new entry to the on-disk WAL
// 3. only if the WAL write succeeds, update the in-memory map
func (s *Store) Put(key, value string, version *Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if version == nil {
		version = s.generateVersion()
	}

	entry := &Entry{
		Key:     key,
		Value:   value,
		Version: *version,
		Deleted: false,
	}

	// write to WAL first
	if err := s.wal.Append(entry); err != nil {
		return err
	}

	s.data[key] = entry
	s.metrics.Writes++
	return nil
}

// Get retrives a key. It uses a read lock to allow concurrent reads
// returns a value only if the key exists and its 'Deleted' flag is false
func (s *Store) Get(key string) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.data[key]
	if exists && !entry.Deleted {
		s.metrics.Reads++
		return entry, true
	}

	return nil, false
}

// Delete performs a "soft-delete"
// (only changes the flag to true, and clears the value)
// the operation is also written to the WAL to ensure the deletion
// is replicable
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	version := s.generateVersion()
	entry := &Entry{
		Key:     key,
		Value:   "",
		Version: *version,
		Deleted: true,
	}

	if err := s.wal.Append(entry); err != nil {
		return err
	}

	s.data[key] = entry
	s.metrics.Deletes++
	return nil
}

// generateVersion creates a version for a new write originating no this node
// the verctor clock is initialized with only this node's ID and current timestamp
func (s *Store) generateVersion() *Version {
	return &Version{
		Clock:     map[string]int64{s.nodeID: time.Now().UnixNano()},
		Timestamp: time.Now().UnixNano(),
	}
}

// loadSnapshot deserializes the snapshot file and
// populates the in-memory data map
// this is called only during initialization, so no lock is required.
func (s *Store) loadSnapshot() error {
	entries, err := s.snapMgr.Load()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		s.data[entry.Key] = entry
	}

	return nil
}

// replayWAL iterates through records and applies them to the 'data' map
// overwrites any older state loaded from a snapshot
func (s *Store) replayWAL() error {
	return s.wal.Replay(func(entry *Entry) {
		s.data[entry.Key] = entry
	})
}

// Snapshot creates a point-in-time snapshot of the current data
// it uses a read lock to prevent modification during the copy processes
// and then writes the copied data to a snapshot file
func (s *Store) Snapshot() error {
	s.mu.RLock()
	entries := make([]*Entry, 0, len(s.data))
	for _, entry := range s.data {
		entries = append(entries, entry)
	}
	s.mu.RUnlock()

	return s.snapMgr.Save(entries)
}

// GetMetrics provide thread-safe access to the store's operational metrics
func (s *Store) GetMetrics() *Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &Metrics{
		Reads:   s.metrics.Reads,
		Writes:  s.metrics.Writes,
		Deletes: s.metrics.Deletes,
	}
}
