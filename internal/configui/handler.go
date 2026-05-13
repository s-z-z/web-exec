// Package configui provides an HTTP handler and embedded web UI for managing
// route configurations. It exposes a REST API for CRUD operations and serves
// a single-page HTML application from an embedded filesystem.
package configui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/s-z-z/web-exec/internal/config"
	"github.com/s-z-z/web-exec/internal/history"
)

// Handler serves both the REST API and the embedded HTML frontend for
// managing route configurations.
type Handler struct {
	store        *config.ConfigStore
	historyStore *history.HistoryStore
}

// NewHandler creates a new Handler backed by the given ConfigStore and HistoryStore.
func NewHandler(store *config.ConfigStore, historyStore *history.HistoryStore) *Handler {
	return &Handler{store: store, historyStore: historyStore}
}

// ServeHTTP implements http.Handler. It routes requests to either the
// embedded static UI or the REST API.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[configui] %s %s", r.Method, r.URL.Path)

	// Strip the /config prefix so we work with the sub-path only
	path := strings.TrimPrefix(r.URL.Path, "/config")
	path = strings.TrimPrefix(path, "/")
	log.Printf("[configui] stripped path: %q", path)

	// Serve the embedded HTML UI at /config or /config/
	if path == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexHTML)
		return
	}

	// API routes under /config/routes or /config/routes/
	if path == "routes" || strings.HasPrefix(path, "routes/") {
		h.handleAPI(w, r, path)
		return
	}

	// API routes under /config/history or /config/history/
	if path == "history" || strings.HasPrefix(path, "history/") {
		h.handleHistoryAPI(w, r)
		return
	}

	log.Printf("[configui] unmatched path: %q", path)
	http.Error(w, "Not Found", http.StatusNotFound)
}

// handleHistoryAPI handles GET /config/history
func (h *Handler) handleHistoryAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// GET /config/history - list all, or filter by routeId query param
	routeID := r.URL.Query().Get("routeId")
	if routeID != "" {
		records := h.historyStore.ListByRouteID(routeID)
		_ = json.NewEncoder(w).Encode(records)
	} else {
		records := h.historyStore.List()
		_ = json.NewEncoder(w).Encode(records)
	}
}

// handleAPI dispatches API requests for /config/routes/*.
// subPath is the path after stripping /config/ (e.g. "routes" or "routes/abc123").
func (h *Handler) handleAPI(w http.ResponseWriter, r *http.Request, subPath string) {
	w.Header().Set("Content-Type", "application/json")
	log.Printf("[configui] handleAPI subPath=%q method=%s", subPath, r.Method)

	// /config/routes -> list or create
	if subPath == "routes" || subPath == "routes/" {
		switch r.Method {
		case http.MethodGet:
			log.Printf("[configui] -> listRoutes")
			h.listRoutes(w, r)
		case http.MethodPost:
			log.Printf("[configui] -> createRoute")
			h.createRoute(w, r)
		default:
			log.Printf("[configui] -> method not allowed: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	// /config/routes/{id}
	idPart := strings.TrimPrefix(subPath, "routes/")
	if idPart == "" {
		log.Printf("[configui] -> no id in path")
		http.Error(w, `{"error":"Not Found"}`, http.StatusNotFound)
		return
	}
	id := idPart
	log.Printf("[configui] -> route id=%q", id)

	switch r.Method {
	case http.MethodPut:
		log.Printf("[configui] -> updateRoute id=%s", id)
		h.updateRoute(w, r, id)
	case http.MethodDelete:
		log.Printf("[configui] -> deleteRoute id=%s", id)
		h.deleteRoute(w, r, id)
	default:
		log.Printf("[configui] -> method not allowed: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// routeRequest represents the JSON body for creating or updating a route.
type routeRequest struct {
	Path    string `json:"path"`
	WorkDir string `json:"workDir"`
	Script  string `json:"script"`
}

// validate checks that the route request contains valid data.
func (req *routeRequest) validate() error {
	if req.Path == "" {
		return fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(req.Path, "/") {
		return fmt.Errorf("path must start with \"/\"")
	}
	if req.WorkDir == "" {
		return fmt.Errorf("work_dir is required")
	}
	if req.Script == "" {
		return fmt.Errorf("script is required")
	}
	return nil
}

// routeResponse is the JSON shape returned by the API.
type routeResponse struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	WorkDir   string `json:"workDir"`
	Script    string `json:"script"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// newRouteResponse builds a routeResponse from a RouteConfig.
func newRouteResponse(rc *config.RouteConfig) routeResponse {
	return routeResponse{
		ID:        rc.ID,
		Path:      rc.Path,
		WorkDir:   rc.WorkDir,
		Script:    rc.Script,
		CreatedAt: rc.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: rc.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// listRoutes returns all configured routes as JSON.
func (h *Handler) listRoutes(w http.ResponseWriter, r *http.Request) {
	routes := h.store.List()
	resp := make([]routeResponse, len(routes))
	for i, rc := range routes {
		resp[i] = newRouteResponse(rc)
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// createRoute handles POST /config/routes to add a new route.
func (h *Handler) createRoute(w http.ResponseWriter, r *http.Request) {
	var req routeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[configui] createRoute: decode error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	log.Printf("[configui] createRoute: path=%s workDir=%s", req.Path, req.WorkDir)
	if err := req.validate(); err != nil {
		log.Printf("[configui] createRoute: validation error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	rc := &config.RouteConfig{
		Path:    req.Path,
		WorkDir: req.WorkDir,
		Script:  req.Script,
	}

	if err := h.store.Add(r.Context(), rc); err != nil {
		log.Printf("[configui] createRoute: store.Add error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(newRouteResponse(rc))
}

// updateRoute handles PUT /config/routes/{id} to modify an existing route.
func (h *Handler) updateRoute(w http.ResponseWriter, r *http.Request, id string) {
	var req routeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	rc, err := h.store.Update(r.Context(), id, func(existing *config.RouteConfig) {
		existing.Path = req.Path
		existing.WorkDir = req.WorkDir
		existing.Script = req.Script
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(newRouteResponse(rc))
}

// deleteRoute handles DELETE /config/routes/{id} to remove a route.
func (h *Handler) deleteRoute(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.store.Delete(r.Context(), id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

//go:embed static/index.html
var indexHTML []byte
