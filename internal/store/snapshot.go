package store

import (
	"encoding/json"
	"os"
)

// Snapshots provide a compact, point-in-time backup of the entire store.
// so no need to replay the whole log on restart.
type SnapshotManager struct {
	path string
}

func NewSnapshotManager(path string) *SnapshotManager {
	return &SnapshotManager{path: path}
}

func (s *SnapshotManager) Save(entries []*Entry) error {
	// convert the in-memory KV entries to JSON
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	// write to a tmp file
	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	// rename it to the actual snapshot path
	// ensures the old snapshot is swapped only after the
	// new one is fully written.
	return os.Rename(tempPath, s.path)
}

func (s *SnapshotManager) Load() ([]*Entry, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []*Entry
	err = json.Unmarshal(data, &entries)
	return entries, err
}
