package history

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// HistoryStore persists execution records to a JSON file
// and provides thread-safe operations.
type HistoryStore struct {
	mu      sync.RWMutex
	path    string
	records []ExecRecord
}

// NewHistoryStore creates a new HistoryStore that persists data to the given file path.
func NewHistoryStore(path string) *HistoryStore {
	return &HistoryStore{
		path:    path,
		records: make([]ExecRecord, 0),
	}
}

// Load reads the JSON file from disk and populates the store.
func (s *HistoryStore) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-ctx.Done():
		return fmt.Errorf("load cancelled: %w", ctx.Err())
	default:
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read history file: %w", err)
	}

	var records []ExecRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("unmarshal history: %w", err)
	}

	s.records = records
	return nil
}

// Save persists the current records to the JSON file.
func (s *HistoryStore) Save(ctx context.Context) error {
	s.mu.RLock()
	records := make([]ExecRecord, len(s.records))
	copy(records, s.records)
	s.mu.RUnlock()

	select {
	case <-ctx.Done():
		return fmt.Errorf("save cancelled: %w", ctx.Err())
	default:
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write history file: %w", err)
	}
	return nil
}

// Add inserts a new execution record and persists it.
func (s *HistoryStore) Add(ctx context.Context, record *ExecRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-ctx.Done():
		return fmt.Errorf("add cancelled: %w", ctx.Err())
	default:
	}

	record.ID = generateID()
	record.CreatedAt = time.Now()

	s.records = append([]ExecRecord{*record}, s.records...)

	// Truncate to keep only the last 100 records
	if len(s.records) > 100 {
		s.records = s.records[:100]
	}

	return s.saveLocked()
}

// List returns a snapshot of all records (newest first).
func (s *HistoryStore) List() []ExecRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ExecRecord, len(s.records))
	copy(out, s.records)
	return out
}

// ListByRouteID returns a snapshot of records filtered by routeID (newest first).
func (s *HistoryStore) ListByRouteID(routeID string) []ExecRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []ExecRecord
	for _, r := range s.records {
		if r.RouteID == routeID {
			out = append(out, r)
		}
	}
	return out
}

// saveLocked writes the current records to disk. Must be called with s.mu held.
func (s *HistoryStore) saveLocked() error {
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write history file: %w", err)
	}
	return nil
}

// generateID creates a short random hex string (8 chars).
func generateID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp in hex (unlikely to collide in normal use)
		return fmt.Sprintf("%08x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%08x", b)
}
