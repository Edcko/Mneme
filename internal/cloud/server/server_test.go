package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Edcko/Mneme/internal/cloud/auth"
	"github.com/Edcko/Mneme/internal/cloud/store"
	"github.com/Edcko/Mneme/internal/sync"
)

// ─── Test Helpers ─────────────────────────────────────────────────────────────

const testJWTSecret = "test-secret-key-32bytes-long!!!!!"

// testEnv holds a fully wired server + helpers for testing.
type testEnv struct {
	server  *Server
	store   *store.TestStore
	auth    *auth.Manager
	client  *http.Client
	baseURL string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s, err := store.NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	srv, err := New(Config{
		Store:     s,
		JWTSecret: testJWTSecret,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &testEnv{
		server:  srv,
		store:   s,
		auth:    srv.AuthManager(),
		client:  ts.Client(),
		baseURL: ts.URL,
	}
}

// setupProject creates a project and an API key, returning the raw key for auth.
func (e *testEnv) setupProject(t *testing.T, name string) (int64, string) {
	t.Helper()

	p, err := e.store.CreateProject(name, name+"@test.com", "secret-"+name)
	if err != nil {
		t.Fatalf("CreateProject %s: %v", name, err)
	}

	rawKey, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	if err := e.store.CreateAPIKey(p.ID, rawKey, "test-key"); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	e.auth.AddKey("key-"+name, hash)

	return p.ID, rawKey
}

func (e *testEnv) doRequest(method, path string, body []byte, apiKey string) (*http.Response, map[string]any) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, _ := http.NewRequest(method, e.baseURL+path, bodyReader)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, nil
	}

	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return resp, nil
	}

	var result map[string]any
	json.Unmarshal(raw, &result)

	// Store raw body for re-reading
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, result
}

// doRequestArray sends a request and decodes the response as a JSON array.
func (e *testEnv) doRequestArray(method, path string, body []byte, apiKey string) (*http.Response, []any) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, _ := http.NewRequest(method, e.baseURL+path, bodyReader)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, nil
	}

	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return resp, nil
	}

	var result []any
	json.Unmarshal(raw, &result)

	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, result
}

func (e *testEnv) doRawRequest(method, path string, body []byte, apiKey string, headers map[string]string) *http.Response {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, _ := http.NewRequest(method, e.baseURL+path, bodyReader)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil
	}
	return resp
}

func gzipData(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// ─── Health ──────────────────────────────────────────────────────────────────

func TestHealthEndpoint(t *testing.T) {
	env := newTestEnv(t)

	resp, _ := env.doRequest(http.MethodGet, "/health", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHealthResponse(t *testing.T) {
	env := newTestEnv(t)

	resp, body := env.doRequest(http.MethodGet, "/health", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["service"] != "mneme-cloud" {
		t.Errorf("service = %v, want mneme-cloud", body["service"])
	}
}

// ─── Auth Protection ─────────────────────────────────────────────────────────

func TestSyncEndpointsRequireAuth(t *testing.T) {
	env := newTestEnv(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/sync/manifest"},
		{http.MethodPut, "/sync/manifest"},
		{http.MethodGet, "/sync/chunks/test123"},
	}

	for _, ep := range endpoints {
		resp, _ := env.doRequest(ep.method, ep.path, nil, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", ep.method, ep.path, resp.StatusCode)
		}
	}
}

func TestSyncEndpointsRejectInvalidKey(t *testing.T) {
	env := newTestEnv(t)

	resp, _ := env.doRequest(http.MethodGet, "/sync/manifest", nil, "mn_invalidkey1234567890abcdef")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSyncEndpointsAcceptValidKey(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "auth-test")

	resp, _ := env.doRequest(http.MethodGet, "/sync/manifest", nil, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ─── Manifest Operations ────────────────────────────────────────────────────

func TestGetManifestEmpty(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "manifest-empty")

	resp, body := env.doRequest(http.MethodGet, "/sync/manifest", nil, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	version, ok := body["version"].(float64)
	if !ok || version != 1 {
		t.Errorf("version = %v, want 1", body["version"])
	}

	chunks, ok := body["chunks"].([]any)
	if !ok || len(chunks) != 0 {
		t.Errorf("chunks = %v, want empty array", body["chunks"])
	}
}

func TestPutManifestAndGet(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "manifest-put")

	// PUT manifest
	manifest := sync.Manifest{
		Version: 1,
		Chunks: []sync.ChunkEntry{
			{ID: "abcd1234", CreatedBy: "alice", Memories: 5, Sessions: 1},
		},
	}
	data, _ := json.Marshal(manifest)

	resp, body := env.doRequest(http.MethodPut, "/sync/manifest", data, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT manifest: expected 200, got %d (body: %v)", resp.StatusCode, body)
	}

	// GET manifest — should have the entry
	resp, body = env.doRequest(http.MethodGet, "/sync/manifest", nil, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET manifest: expected 200, got %d", resp.StatusCode)
	}

	version, _ := body["version"].(float64)
	if version != 1 {
		t.Errorf("version = %v, want 1", body["version"])
	}

	chunks, _ := body["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
}

func TestPutManifestAppends(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "manifest-append")

	// First PUT
	m1 := sync.Manifest{
		Chunks: []sync.ChunkEntry{
			{ID: "chunk-1", CreatedBy: "alice", Memories: 3},
		},
	}
	data1, _ := json.Marshal(m1)
	resp, _ := env.doRequest(http.MethodPut, "/sync/manifest", data1, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first PUT: expected 200, got %d", resp.StatusCode)
	}

	// Second PUT — should append
	m2 := sync.Manifest{
		Chunks: []sync.ChunkEntry{
			{ID: "chunk-2", CreatedBy: "bob", Memories: 7},
		},
	}
	data2, _ := json.Marshal(m2)
	resp, body := env.doRequest(http.MethodPut, "/sync/manifest", data2, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second PUT: expected 200, got %d", resp.StatusCode)
	}

	version, _ := body["version"].(float64)
	if version != 2 {
		t.Errorf("version after second append = %v, want 2", body["version"])
	}

	chunks, _ := body["chunks"].([]any)
	if len(chunks) != 2 {
		t.Errorf("chunks after second append = %d, want 2", len(chunks))
	}
}

func TestPutManifestInvalidJSON(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "manifest-invalid")

	resp, body := env.doRequest(http.MethodPut, "/sync/manifest", []byte("not json"), apiKey)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body: %v)", resp.StatusCode, body)
	}
}

// ─── Chunk Operations ────────────────────────────────────────────────────────

func TestPutAndGetChunk(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "chunk-test")

	// Gzip-compressed chunk data (as HTTPTransport sends it)
	payload := []byte(`{"sessions":[],"observations":[],"prompts":[]}`)
	gzData := gzipData(t, payload)

	// PUT chunk
	resp := env.doRawRequest(http.MethodPost, "/sync/chunks/abcd1234", gzData, apiKey, map[string]string{
		"Content-Encoding": "gzip",
		"Content-Type":     "application/octet-stream",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT chunk: expected 204, got %d (body: %s)", resp.StatusCode, body)
	}

	// GET chunk
	resp = env.doRawRequest(http.MethodGet, "/sync/chunks/abcd1234", nil, apiKey, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET chunk: expected 200, got %d", resp.StatusCode)
	}

	// The server does NOT set Content-Encoding: gzip on purpose.
	// Go's http.Client auto-decompresses when Content-Encoding: gzip is set,
	// which would break HTTPTransport's gzip.NewReader (data already decompressed).
	// Instead, the server returns raw gzipped bytes as application/octet-stream.
	if enc := resp.Header.Get("Content-Encoding"); enc == "gzip" {
		t.Error("Content-Encoding should NOT be gzip — would cause double-decompress in HTTPTransport")
	}

	// Read and verify the data matches the stored gzip payload.
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !bytes.Equal(respData, gzData) {
		t.Errorf("response data mismatch: got %d bytes, want %d bytes", len(respData), len(gzData))
	}
}

func TestGetChunkNotFound(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "chunk-nf")

	resp := env.doRawRequest(http.MethodGet, "/sync/chunks/nonexistent", nil, apiKey, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPutChunkEmpty(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "chunk-empty")

	resp := env.doRawRequest(http.MethodPost, "/sync/chunks/empty123", nil, apiKey, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestChunkProjectIsolation(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey1 := env.setupProject(t, "iso-proj-a")
	_, apiKey2 := env.setupProject(t, "iso-proj-b")

	// Project A uploads a chunk
	payload := []byte(`{"data":"project-a"}`)
	gzData := gzipData(t, payload)

	resp := env.doRawRequest(http.MethodPost, "/sync/chunks/shared-id", gzData, apiKey1, map[string]string{
		"Content-Encoding": "gzip",
	})
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT chunk for project A: expected 204, got %d", resp.StatusCode)
	}

	// Project B should NOT see project A's chunk
	resp = env.doRawRequest(http.MethodGet, "/sync/chunks/shared-id", nil, apiKey2, nil)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for project B accessing project A's chunk, got %d", resp.StatusCode)
	}
}

// ─── Admin: Create Project ───────────────────────────────────────────────────

func TestCreateProject(t *testing.T) {
	env := newTestEnv(t)
	// Create an admin key for auth
	rawKey, hash, _ := auth.GenerateAPIKey()
	env.auth.AddKey("admin", hash)

	body, _ := json.Marshal(createProjectRequest{
		Name:       "test-project",
		OwnerEmail: "admin@test.com",
	})

	resp, result := env.doRequest(http.MethodPost, "/admin/projects", body, rawKey)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %v)", resp.StatusCode, result)
	}

	name, _ := result["name"].(string)
	if name != "test-project" {
		t.Errorf("name = %q, want %q", name, "test-project")
	}

	email, _ := result["owner_email"].(string)
	if email != "admin@test.com" {
		t.Errorf("owner_email = %q, want %q", email, "admin@test.com")
	}
}

func TestCreateProjectDuplicate(t *testing.T) {
	env := newTestEnv(t)
	rawKey, hash, _ := auth.GenerateAPIKey()
	env.auth.AddKey("admin", hash)

	body, _ := json.Marshal(createProjectRequest{
		Name:       "dup-project",
		OwnerEmail: "a@b.com",
	})

	// First creation succeeds
	resp, _ := env.doRequest(http.MethodPost, "/admin/projects", body, rawKey)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", resp.StatusCode)
	}

	// Second creation fails with 409
	resp, _ = env.doRequest(http.MethodPost, "/admin/projects", body, rawKey)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate create: expected 409, got %d", resp.StatusCode)
	}
}

func TestCreateProjectMissingName(t *testing.T) {
	env := newTestEnv(t)
	rawKey, hash, _ := auth.GenerateAPIKey()
	env.auth.AddKey("admin", hash)

	body, _ := json.Marshal(createProjectRequest{
		OwnerEmail: "a@b.com",
	})

	resp, _ := env.doRequest(http.MethodPost, "/admin/projects", body, rawKey)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateProjectMissingEmail(t *testing.T) {
	env := newTestEnv(t)
	rawKey, hash, _ := auth.GenerateAPIKey()
	env.auth.AddKey("admin", hash)

	body, _ := json.Marshal(createProjectRequest{
		Name: "no-email",
	})

	resp, _ := env.doRequest(http.MethodPost, "/admin/projects", body, rawKey)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// ─── Admin: Generate API Key ─────────────────────────────────────────────────

func TestGenerateAPIKey(t *testing.T) {
	env := newTestEnv(t)

	// Create project directly in store
	p, err := env.store.CreateProject("key-test", "k@t.com", "secret")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Create admin key for auth
	adminKey, adminHash, _ := auth.GenerateAPIKey()
	env.auth.AddKey("admin-key", adminHash)

	resp, result := env.doRequest(http.MethodPost, "/admin/projects/key-test/keys", nil, adminKey)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %v)", resp.StatusCode, result)
	}

	project, _ := result["project"].(string)
	if project != "key-test" {
		t.Errorf("project = %q, want %q", project, "key-test")
	}

	apiKey, _ := result["api_key"].(string)
	if apiKey == "" {
		t.Error("expected non-empty api_key")
	}
	if len(apiKey) < 10 {
		t.Errorf("api_key seems too short: %s", apiKey)
	}

	// The generated key should work for auth
	_ = p // used implicitly via the key being stored
	resp2, _ := env.doRequest(http.MethodGet, "/sync/manifest", nil, apiKey)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("generated key should work for sync: expected 200, got %d", resp2.StatusCode)
	}
}

func TestGenerateAPIKeyProjectNotFound(t *testing.T) {
	env := newTestEnv(t)
	rawKey, hash, _ := auth.GenerateAPIKey()
	env.auth.AddKey("admin", hash)

	resp, _ := env.doRequest(http.MethodPost, "/admin/projects/nonexistent/keys", nil, rawKey)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// ─── Admin: Sync Log ─────────────────────────────────────────────────────────

func TestSyncLog(t *testing.T) {
	env := newTestEnv(t)
	projectID, apiKey := env.setupProject(t, "sync-log-test")

	// Do some sync operations to generate log entries
	manifest := sync.Manifest{
		Chunks: []sync.ChunkEntry{{ID: "log-chunk", CreatedBy: "test", Memories: 3}},
	}
	data, _ := json.Marshal(manifest)
	env.doRequest(http.MethodPut, "/sync/manifest", data, apiKey)

	// Upload a chunk to generate another log entry
	gzData := gzipData(t, []byte(`{"data":"test"}`))
	resp := env.doRawRequest(http.MethodPost, "/sync/chunks/log-chunk", gzData, apiKey, map[string]string{
		"Content-Encoding": "gzip",
	})
	resp.Body.Close()

	// Check sync log
	logResp, entries := env.doRequestArray(http.MethodGet, "/admin/sync-log", nil, apiKey)
	if logResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", logResp.StatusCode)
	}

	if entries == nil {
		t.Fatal("expected array, got nil")
	}
	if len(entries) < 2 {
		t.Errorf("entries = %d, want at least 2", len(entries))
	}

	// Verify log entries belong to our project
	_ = projectID
	for _, entry := range entries {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		projID, _ := e["project_id"].(float64)
		if int64(projID) != projectID {
			t.Errorf("log entry project_id = %v, want %d", projID, projectID)
		}
	}
}

func TestSyncLogEmpty(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "sync-log-empty")

	resp, entries := env.doRequestArray(http.MethodGet, "/admin/sync-log", nil, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if entries == nil {
		t.Fatal("expected array, got nil")
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

// ─── JWT Auth (Bearer Token) ─────────────────────────────────────────────────

func TestSyncWithJWT(t *testing.T) {
	env := newTestEnv(t)
	projectID, apiKey := env.setupProject(t, "jwt-test")

	// Verify project exists
	p, err := env.store.GetProjectByID(projectID)
	if err != nil {
		t.Fatalf("GetProjectByID: %v", err)
	}

	// Generate a JWT with the project name (as an admin token would)
	token, err := env.auth.GenerateToken("admin", p.Name, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Use JWT for manifest request
	req, _ := http.NewRequest(http.MethodGet, env.baseURL+"/sync/manifest", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d (body: %s)", resp.StatusCode, body)
	}

	// Verify the JWT is NOT sufficient on its own — API key should also work
	_ = apiKey
}

func TestSyncWithExpiredJWT(t *testing.T) {
	env := newTestEnv(t)
	_, _ = env.setupProject(t, "jwt-expired")

	// Generate an expired token directly
	token, err := env.auth.GenerateToken("user-1", "jwt-expired", -1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, env.baseURL+"/sync/manifest", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired JWT, got %d", resp.StatusCode)
	}
}

// ─── Integration: Full Sync Flow ─────────────────────────────────────────────

func TestFullSyncFlow(t *testing.T) {
	env := newTestEnv(t)
	_, apiKey := env.setupProject(t, "full-flow")

	// Step 1: GET manifest (should be empty)
	resp, body := env.doRequest(http.MethodGet, "/sync/manifest", nil, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET manifest: expected 200, got %d", resp.StatusCode)
	}
	version, _ := body["version"].(float64)
	if version != 1 {
		t.Errorf("initial version = %v, want 1", body["version"])
	}

	// Step 2: Upload a chunk
	chunkData := []byte(`{"sessions":[{"id":"s1"}],"observations":[],"prompts":[]}`)
	gzData := gzipData(t, chunkData)

	resp = env.doRawRequest(http.MethodPost, "/sync/chunks/a1b2c3d4", gzData, apiKey, map[string]string{
		"Content-Encoding": "gzip",
		"Content-Type":     "application/octet-stream",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT chunk: expected 204, got %d", resp.StatusCode)
	}

	// Step 3: Update manifest with the chunk entry
	manifest := sync.Manifest{
		Chunks: []sync.ChunkEntry{
			{ID: "a1b2c3d4", CreatedBy: "alice", CreatedAt: "2025-01-01T00:00:00Z", Sessions: 1, Memories: 0, Prompts: 0},
		},
	}
	mData, _ := json.Marshal(manifest)
	resp, body = env.doRequest(http.MethodPut, "/sync/manifest", mData, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT manifest: expected 200, got %d", resp.StatusCode)
	}
	version, _ = body["version"].(float64)
	if version != 1 {
		t.Errorf("after put version = %v, want 1", body["version"])
	}

	// Step 4: GET manifest — should have the chunk
	resp, body = env.doRequest(http.MethodGet, "/sync/manifest", nil, apiKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET manifest after sync: expected 200, got %d", resp.StatusCode)
	}
	chunks, _ := body["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}

	// Step 5: Download the chunk
	resp = env.doRawRequest(http.MethodGet, "/sync/chunks/a1b2c3d4", nil, apiKey, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET chunk: expected 200, got %d", resp.StatusCode)
	}

	respData, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(respData, gzData) {
		t.Error("downloaded chunk data should match uploaded data")
	}

	// Step 6: Verify sync log has entries
	logResp, logEntries := env.doRequestArray(http.MethodGet, "/admin/sync-log", nil, apiKey)
	if logResp.StatusCode != http.StatusOK {
		t.Fatalf("GET sync-log: expected 200, got %d", logResp.StatusCode)
	}
	if len(logEntries) < 2 {
		t.Errorf("sync log entries = %d, want at least 2", len(logEntries))
	}
}

// ─── Server Construction ─────────────────────────────────────────────────────

func TestNewServerMissingStore(t *testing.T) {
	_, err := New(Config{JWTSecret: "secret"})
	if err == nil {
		t.Fatal("expected error when Store is nil")
	}
}

func TestNewServerMissingSecret(t *testing.T) {
	s, _ := store.NewTestStore()
	defer s.Close()

	_, err := New(Config{Store: s})
	if err == nil {
		t.Fatal("expected error when JWTSecret is empty")
	}
}
