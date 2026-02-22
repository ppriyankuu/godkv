package store

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

// The WAL (Write-Ahead Log) is an append-only file where every mutation is
// durably recorded BEFORE it is applied to the in-memory store.
//
// Interview explanation:
//   WALs are the backbone of crash safety in databases.  Because writes are
//   sequential (append-only), they are very fast even on spinning disks.
//   On restart we read the WAL from top to bottom and re-apply every entry,
//   leaving the store in the exact state it was before the crash.

const (
	opPut    = "PUT"
	opDelete = "DELETE"
)

type walEntry struct {
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value Value  `json:"value"`
}

// WAL is a simple append-only log backed by a single file.
// Each entry is a newline-delimited JSON object (NDJSON) which makes it
// trivial to read back line-by-line.
type WAL struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func newWAL(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{file: f, path: path}, nil
}

// append serialises entry as JSON and fsync-writes it.
// fsync (Sync) forces the OS to flush its write buffer to physical media —
// without it a crash could lose the entry even though Write returned nil.
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
	return w.file.Sync() // flush to disk — this is the "D" in ACID
}

// readAll scans the WAL file from the beginning and returns all entries.
func (w *WAL) readAll() ([]walEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Seek to start for reading.
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
			// Corrupt entry — skip it.  In a production system we'd stop here
			// and alert an operator rather than silently skip.
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// truncate empties the WAL after a snapshot has been taken.
// We use O_TRUNC rather than deleting because re-opening is simpler.
func (w *WAL) truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.file.Truncate(0); err != nil {
		return err
	}
	_, err := w.file.Seek(0, 0)
	return err
}

func (w *WAL) close() error {
	return w.file.Close()
}
