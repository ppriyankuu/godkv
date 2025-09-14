package store

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

// WAL -> Write-Ahead Log
// every change to the KV store is first written to this log on disk
// before being applied to the in-memory map. This is to allow rebuilding
// of the state by replaying the WAL in case the node crashes.

type WAL struct {
	file *os.File   // the actual file on disk where operations are logged
	mu   sync.Mutex // for thread safety
	path string     // the log file path
}

func NewWAL(path string) (*WAL, error) {
	// opens/creates a log file in "append-only" mode.
	// meaning each operation gets added to the end of the file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return &WAL{
		file: file,
		path: path,
	}, nil
}

func (w *WAL) Append(entry *Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// serializes the KV operation to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// append it to the WAL file
	_, err = w.file.Write(append(data, '\n'))
	if err != nil {
		return err
	}

	return w.file.Sync()
}

func (w *WAL) Replay(fn func(*Entry)) error {
	file, err := os.Open(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	// read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry Entry
		// parses each JSON log entry back into 'ENTRY'
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip corrupted files
		}
		// calls the callback func to re-apply it to the store
		fn(&entry)
	}

	return scanner.Err()
}

func (w *WAL) Close() error {
	return w.file.Close()
}
