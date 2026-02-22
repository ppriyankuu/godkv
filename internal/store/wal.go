package store

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

// WAL (Write-Ahead Log)
//
// The WAL is a file where we record every change BEFORE
// we update the in-memory map.
//
// Why?
// If the process crashes, memory is lost.
// But the WAL file stays on disk.
//
// When the server restarts:
//   1) We read the snapshot (if any)
//   2) Then replay all WAL entries
//   3) The store becomes exactly what it was before the crash
//
// Important idea:
// The WAL is append-only.
// We only add to the end of the file.
// This is fast because disks are very good at sequential writes.
//
// This is the same basic idea used by real databases.

// These define the type of operation stored in the WAL.
const (
	opPut    = "PUT"
	opDelete = "DELETE"
)

// walEntry represents one line in the WAL file.
//
// Each entry stores:
//   - The operation (PUT or DELETE)
//   - The key
//   - The full Value (including vector clock and tombstone)
//
// We store the full Value so recovery is simple.
// During replay we just restore it directly into memory.
type walEntry struct {
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value Value  `json:"value"`
}

// WAL represents the write-ahead log file.
//
// Fields:
//   - mu: ensures only one goroutine writes at a time
//   - file: the open file handle
//   - path: file location (used for truncate/reopen logic)
type WAL struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// newWAL opens (or creates) the WAL file.
//
// Flags:
//
//	O_CREATE → create file if it does not exist
//	O_RDWR   → open for read and write
//	O_APPEND → always write at the end of file
//
// We use O_APPEND to guarantee we never overwrite old entries.
func newWAL(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{file: f, path: path}, nil
}

// append writes a new entry to the WAL.
//
// Steps:
//  1. Lock (only one writer allowed)
//  2. Convert entry to JSON
//  3. Add newline (so each entry is one line)
//  4. Write to file
//  5. Call Sync() to flush to disk
//
// Why Sync() is important:
//
//	Write() only writes to OS buffer.
//	Sync() forces the OS to flush data to physical disk.
//
// Without Sync(), a sudden crash (power loss)
// could lose the last write even though Write() succeeded.
//
// This is what makes the WAL durable.
func (w *WAL) append(entry walEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if _, err := w.file.Write(data); err != nil {
		return err
	}
	return w.file.Sync() // ensures data is physically written to disk
}

// readAll reads the entire WAL file from the beginning.
//
// Used during startup to replay operations.
//
// Steps:
//  1. Seek to beginning of file
//  2. Read line by line
//  3. Parse each JSON line
//  4. Return all entries in order
//
// Important:
// Entries must be applied in the same order they were written.
// Order matters because later writes override earlier ones.
func (w *WAL) readAll() ([]walEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Move file pointer to beginning before reading.
	if _, err := w.file.Seek(0, 0); err != nil {
		return nil, err
	}

	var entries []walEntry
	scanner := bufio.NewScanner(w.file)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var e walEntry
		if err := json.Unmarshal(line, &e); err != nil {
			// If one line is corrupted, we skip it.
			// In a real production system, we would likely stop
			// and raise an alert instead of silently skipping.
			continue
		}
		entries = append(entries, e)
	}

	return entries, scanner.Err()
}

// truncate clears the WAL file.
//
// When do we call this?
// After taking a snapshot.
//
// Why?
// Because the snapshot already contains all data.
// So old WAL entries are no longer needed.
//
// Instead of deleting the file,
// we truncate it (set size to 0).
// This keeps the file handle open and simplifies logic.
func (w *WAL) truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.file.Truncate(0); err != nil {
		return err
	}

	// Move file pointer back to start.
	_, err := w.file.Seek(0, 0)
	return err
}

// close closes the WAL file.
// Should be called during graceful shutdown.
func (w *WAL) close() error {
	return w.file.Close()
}
