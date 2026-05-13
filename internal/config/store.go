package config

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

// ConfigStore persists route configurations to a JSON file
// and provides thread-safe CRUD operations.
type ConfigStore struct {
	mu       sync.RWMutex
	path     string
	routes   map[string]*RouteConfig
	pathToID map[string]string
}

// NewConfigStore creates a new ConfigStore that persists data to the given file path.
func NewConfigStore(path string) *ConfigStore {
	return &ConfigStore{
		path:     path,
		routes:   make(map[string]*RouteConfig),
		pathToID: make(map[string]string),
	}
}

// Load reads the JSON file from disk and populates the store.
func (s *ConfigStore) Load(ctx context.Context) error {
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
		return fmt.Errorf("read config file: %w", err)
	}

	var routes []RouteConfig
	if err := json.Unmarshal(data, &routes); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	s.routes = make(map[string]*RouteConfig, len(routes))
	s.pathToID = make(map[string]string, len(routes))
	for i := range routes {
		r := &routes[i]
		s.routes[r.ID] = r
		s.pathToID[r.Path] = r.ID
	}
	return nil
}

// Save persists the current routes to the JSON file.
func (s *ConfigStore) Save(ctx context.Context) error {
	s.mu.RLock()
	routes := make([]*RouteConfig, 0, len(s.routes))
	for _, r := range s.routes {
		routes = append(routes, r)
	}
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

	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// List returns a snapshot of all route configurations.
func (s *ConfigStore) List() []*RouteConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*RouteConfig, 0, len(s.routes))
	for _, r := range s.routes {
		out = append(out, r)
	}
	return out
}

// Get retrieves a route by its ID.
func (s *ConfigStore) Get(id string) (*RouteConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.routes[id]
	return r, ok
}

// GetByPath retrieves a route by its HTTP path.
func (s *ConfigStore) GetByPath(path string) (*RouteConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.pathToID[path]
	if !ok {
		return nil, false
	}
	r, ok := s.routes[id]
	return r, ok
}

// Add inserts a new route configuration.
func (s *ConfigStore) Add(ctx context.Context, rc *RouteConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-ctx.Done():
		return fmt.Errorf("add cancelled: %w", ctx.Err())
	default:
	}

	if _, exists := s.pathToID[rc.Path]; exists {
		return fmt.Errorf("route with path %q already exists", rc.Path)
	}

	rc.ID = generateID()
	rc.CreatedAt = time.Now()
	rc.UpdatedAt = rc.CreatedAt

	s.routes[rc.ID] = rc
	s.pathToID[rc.Path] = rc.ID
	return nil
}

// Update modifies an existing route configuration.
func (s *ConfigStore) Update(ctx context.Context, id string, updateFn func(*RouteConfig)) (*RouteConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("update cancelled: %w", ctx.Err())
	default:
	}

	rc, ok := s.routes[id]
	if !ok {
		return nil, fmt.Errorf("route %q not found", id)
	}

	updateFn(rc)
	rc.UpdatedAt = time.Now()
	return rc, nil
}

// Delete removes a route by its ID.
func (s *ConfigStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-ctx.Done():
		return fmt.Errorf("delete cancelled: %w", ctx.Err())
	default:
	}

	rc, ok := s.routes[id]
	if !ok {
		return fmt.Errorf("route %q not found", id)
	}

	delete(s.routes, id)
	delete(s.pathToID, rc.Path)
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
