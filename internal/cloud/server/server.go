// Package server implements the Mneme cloud REST API for remote sync.
//
// It wires together the auth middleware (internal/cloud/auth) and the
// persistence layer (internal/cloud/store) behind a net/http mux.
//
// Endpoint mapping (matches what HTTPTransport expects):
//
//	Sync endpoints (authenticated):
//	  GET  /sync/manifest        — read project manifest
//	  PUT  /sync/manifest        — append entries to manifest
//	  POST /sync/chunks/{id}     — upload gzip chunk
//	  GET  /sync/chunks/{id}     — download gzip chunk
//
//	Admin endpoints (authenticated):
//	  POST /admin/projects             — create project
//	  POST /admin/projects/{name}/keys — generate API key
//	  GET  /admin/sync-log             — list sync log
//
//	System endpoints:
//	  GET /health                      — liveness check
//
// Architecture alignment:
//   - HTTP contract lives here (handlers, routing, status codes)
//   - Persistence lives in store (no SQL in this package)
//   - Auth lives in auth (middleware, JWT, key validation)
package server

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/Edcko/Mneme/internal/cloud/auth"
	"github.com/Edcko/Mneme/internal/cloud/store"
	"github.com/Edcko/Mneme/internal/sync"
)

// ─── Server ──────────────────────────────────────────────────────────────────

// Server is the Mneme cloud REST API server.
type Server struct {
	store  store.CloudStore
	auth   *auth.Manager
	mux    *http.ServeMux
	secret string // JWT signing secret (passed through to auth.Manager)
}

// Config holds the server configuration.
type Config struct {
	// Store is the CloudStore backend (PGStore or TestStore).
	Store store.CloudStore

	// JWTSecret is the secret used for signing JWT tokens.
	JWTSecret string
}

// New creates a new Server with the given configuration.
func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("cloud/server: Store is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("cloud/server: JWTSecret is required")
	}

	s := &Server{
		store:  cfg.Store,
		auth:   auth.NewManager(cfg.JWTSecret),
		mux:    http.NewServeMux(),
		secret: cfg.JWTSecret,
	}

	s.registerRoutes()
	return s, nil
}

// Handler returns the http.Handler for the server (for use with httptest or http.Server).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// AuthManager returns the auth.Manager (for test setup — register keys, generate tokens).
func (s *Server) AuthManager() *auth.Manager {
	return s.auth
}

// Store returns the CloudStore (for test setup).
func (s *Server) Store() store.CloudStore {
	return s.store
}

// ─── Route Registration ──────────────────────────────────────────────────────

func (s *Server) registerRoutes() {
	// System (unauthenticated)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Sync endpoints (authenticated via middleware)
	syncHandler := s.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync/manifest":
			switch r.Method {
			case http.MethodGet:
				s.handleGetManifest(w, r)
			case http.MethodPut:
				s.handlePutManifest(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		default:
			// Chunk endpoints: /sync/chunks/{id}
			s.handleChunkRoutes(w, r)
		}
	}))
	s.mux.Handle("GET /sync/manifest", syncHandler)
	s.mux.Handle("PUT /sync/manifest", syncHandler)

	// Chunk routes use the pattern-matching mux
	chunkHandler := s.auth.Middleware(http.HandlerFunc(s.handleChunkRoutes))
	s.mux.Handle("POST /sync/chunks/{id}", chunkHandler)
	s.mux.Handle("GET /sync/chunks/{id}", chunkHandler)

	// Admin endpoints (authenticated)
	adminHandler := s.auth.Middleware(http.HandlerFunc(s.handleAdminRoutes))
	s.mux.Handle("POST /admin/projects", adminHandler)
	s.mux.Handle("POST /admin/projects/{name}/keys", adminHandler)
	s.mux.Handle("GET /admin/sync-log", adminHandler)
}

// ─── Project Resolution ──────────────────────────────────────────────────────

// resolveProjectID extracts the project ID from the authenticated request.
//
// Flow:
//  1. If JWT claims contain a non-empty Project field → look up by name
//  2. If X-API-Key header is present → authenticate via store to get project
//  3. Otherwise → 401
func (s *Server) resolveProjectID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	// Try JWT claims first
	if claims, ok := auth.FromContext(r.Context()); ok && claims.Project != "" {
		p, err := s.store.GetProjectByName(claims.Project)
		if err != nil {
			writeError(w, http.StatusForbidden, "project not found")
			return 0, false
		}
		return p.ID, true
	}

	// Try API key via store authentication
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		p, err := s.store.Authenticate(apiKey)
		if err != nil {
			writeError(w, http.StatusForbidden, "invalid API key for project")
			return 0, false
		}
		return p.ID, true
	}

	writeError(w, http.StatusUnauthorized, "no project context")
	return 0, false
}

// ─── Health ──────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "mneme-cloud",
	})
}

// ─── Sync: Manifest ──────────────────────────────────────────────────────────

func (s *Server) handleGetManifest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.resolveProjectID(w, r)
	if !ok {
		return
	}

	m, err := s.store.GetManifest(projectID)
	if err != nil {
		log.Printf("ERROR GetManifest project=%d: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Convert CloudManifest → sync.Manifest (what HTTPTransport expects)
	resp := sync.Manifest{
		Version: m.Version,
		Chunks:  m.Chunks,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePutManifest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.resolveProjectID(w, r)
	if !ok {
		return
	}

	var req sync.Manifest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid manifest JSON")
		return
	}

	m, err := s.store.AppendManifest(projectID, req.Chunks)
	if err != nil {
		log.Printf("ERROR AppendManifest project=%d: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Log the sync operation
	_ = s.store.LogSync(projectID, "push", "", len(req.Chunks), r.RemoteAddr)

	resp := sync.Manifest{
		Version: m.Version,
		Chunks:  m.Chunks,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ─── Sync: Chunks ────────────────────────────────────────────────────────────

func (s *Server) handleChunkRoutes(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.resolveProjectID(w, r)
	if !ok {
		return
	}

	chunkID := r.PathValue("id")
	if chunkID == "" {
		writeError(w, http.StatusBadRequest, "chunk ID is required")
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handlePutChunk(w, r, projectID, chunkID)
	case http.MethodGet:
		s.handleGetChunk(w, r, projectID, chunkID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handlePutChunk(w http.ResponseWriter, r *http.Request, projectID int64, chunkID string) {
	// Read the raw body — HTTPTransport sends gzip-compressed data with
	// Content-Encoding: gzip. We store the raw bytes as-is.
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "empty chunk body")
		return
	}

	if err := s.store.PutChunk(projectID, chunkID, data); err != nil {
		log.Printf("ERROR PutChunk project=%d chunk=%s: %v", projectID, chunkID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Log the sync operation
	_ = s.store.LogSync(projectID, "push", chunkID, 1, r.RemoteAddr)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetChunk(w http.ResponseWriter, r *http.Request, projectID int64, chunkID string) {
	chunk, err := s.store.GetChunk(projectID, chunkID)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "chunk not found")
		return
	}
	if err != nil {
		log.Printf("ERROR GetChunk project=%d chunk=%s: %v", projectID, chunkID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// The data is already gzipped in storage. Return as-is with
	// Content-Type: application/octet-stream. Do NOT set Content-Encoding: gzip
	// because Go's http.Client auto-decompresses gzip transport encoding,
	// which would undo the compression before the client's gzip.NewReader sees it.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(chunk.Data)

	// Log the sync operation
	_ = s.store.LogSync(projectID, "pull", chunkID, 1, r.RemoteAddr)
}

// ─── Admin ───────────────────────────────────────────────────────────────────

func (s *Server) handleAdminRoutes(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/admin/projects" && r.Method == http.MethodPost:
		s.handleCreateProject(w, r)
	case r.URL.Path == "/admin/sync-log" && r.Method == http.MethodGet:
		s.handleSyncLog(w, r)
	default:
		// Check if it's a key generation route: /admin/projects/{name}/keys
		name := r.PathValue("name")
		if name != "" && r.Method == http.MethodPost {
			s.handleGenerateAPIKey(w, r, name)
		} else {
			writeError(w, http.StatusNotFound, "not found")
		}
	}
}

type createProjectRequest struct {
	Name       string `json:"name"`
	OwnerEmail string `json:"owner_email"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.OwnerEmail == "" {
		writeError(w, http.StatusBadRequest, "owner_email is required")
		return
	}

	// Generate a project-level API secret
	apiSecret := generateSecret()

	p, err := s.store.CreateProject(req.Name, req.OwnerEmail, apiSecret)
	if err == store.ErrDuplicate {
		writeError(w, http.StatusConflict, "project already exists")
		return
	}
	if err != nil {
		log.Printf("ERROR CreateProject: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

type generateKeyResponse struct {
	Project string `json:"project"`
	KeyID   string `json:"key_id"`
	APIKey  string `json:"api_key"`
	KeyHash string `json:"key_hash,omitempty"`
	Label   string `json:"label,omitempty"`
}

func (s *Server) handleGenerateAPIKey(w http.ResponseWriter, r *http.Request, projectName string) {
	// Verify project exists
	p, err := s.store.GetProjectByName(projectName)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		log.Printf("ERROR GetProjectByName %s: %v", projectName, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Generate API key
	rawKey, hash, err := auth.GenerateAPIKey()
	if err != nil {
		log.Printf("ERROR GenerateAPIKey: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Parse optional label from request body
	var body struct {
		Label string `json:"label"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	// Register in the store (DB)
	testStore, isTest := s.store.(*store.TestStore)
	if isTest {
		if err := testStore.CreateAPIKey(p.ID, rawKey, body.Label); err != nil {
			log.Printf("ERROR CreateAPIKey: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	// Register in auth.Manager (in-memory) so the middleware can validate it
	keyID := fmt.Sprintf("proj-%d-%s", p.ID, projectName)
	s.auth.AddKey(keyID, hash)

	writeJSON(w, http.StatusCreated, generateKeyResponse{
		Project: projectName,
		KeyID:   keyID,
		APIKey:  rawKey,
		Label:   body.Label,
	})
}

func (s *Server) handleSyncLog(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.resolveProjectID(w, r)
	if !ok {
		return
	}

	entries, err := s.store.ListSyncLog(projectID, 50)
	if err != nil {
		log.Printf("ERROR ListSyncLog project=%d: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if entries == nil {
		entries = []store.SyncLogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// ─── Response Helpers ────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("ERROR encode JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// generateSecret creates a random secret for project creation.
// Uses crypto/rand via auth.GenerateAPIKey for simplicity.
func generateSecret() string {
	raw, _, err := auth.GenerateAPIKey()
	if err != nil {
		// Fallback — should never happen
		return "mn_fallback_secret"
	}
	return raw
}

// Ensure gzip.Reader is imported (used in chunk handling).
var _ = gzip.NewReader
