package sync

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── test helpers ───────────────────────────────────────────────────────────

// testServer creates an httptest.Server that behaves like the remote sync API.
// It stores manifest and chunks in memory.
func testServer(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()

	var mu sync.Mutex
	manifest := []byte("{}")
	chunks := make(map[string][]byte)

	mux := http.NewServeMux()

	// Auth middleware.
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if apiKey != "" && r.Header.Get("X-API-Key") != apiKey {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("GET /sync/manifest", auth(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(manifest)
	}))

	mux.HandleFunc("PUT /sync/manifest", auth(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		manifest = data
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /sync/chunks/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		chunkID := r.PathValue("id")
		if chunkID == "" {
			http.Error(w, "missing chunk id", http.StatusBadRequest)
			return
		}

		// Decompress gzip payload and store raw.
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "invalid gzip", http.StatusBadRequest)
				return
			}
			defer gr.Close()
			reader = gr
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}

		mu.Lock()
		defer mu.Unlock()
		chunks[chunkID] = data
		w.WriteHeader(http.StatusCreated)
	}))

	mux.HandleFunc("GET /sync/chunks/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		chunkID := r.PathValue("id")
		mu.Lock()
		data, ok := chunks[chunkID]
		mu.Unlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Gzip compress the response.
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		gw.Write(data)
		gw.Close()

		// Send raw gzip bytes — do NOT set Content-Encoding: gzip
		// because Go's http.Client auto-decompresses that header,
		// which would cause double-decompression in ReadChunk.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(buf.Bytes())
	}))

	return httptest.NewServer(mux)
}

func newTestHTTPTransport(t *testing.T, serverURL, apiKey string) *HTTPTransport {
	t.Helper()
	ht, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL:    serverURL,
		APIKey:     apiKey,
		MaxRetries: 1,
		RetryDelay: 10 * time.Millisecond,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	return ht
}

// ─── NewHTTPTransport validation ─────────────────────────────────────────────

func TestNewHTTPTransportValidates(t *testing.T) {
	t.Run("empty base URL", func(t *testing.T) {
		_, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: ""})
		if err == nil || !strings.Contains(err.Error(), "BaseURL is required") {
			t.Fatalf("expected BaseURL required error, got %v", err)
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		_, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: "://bad"})
		if err == nil || !strings.Contains(err.Error(), "invalid BaseURL") {
			t.Fatalf("expected invalid BaseURL error, got %v", err)
		}
	})

	t.Run("valid URL", func(t *testing.T) {
		ht, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: "http://localhost:8080"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ht == nil {
			t.Fatal("expected non-nil transport")
		}
	})

	t.Run("defaults applied", func(t *testing.T) {
		ht, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: "http://localhost"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ht.maxRetries != 3 {
			t.Fatalf("expected default maxRetries=3, got %d", ht.maxRetries)
		}
		if ht.retryDelay != 500*time.Millisecond {
			t.Fatalf("expected default retryDelay=500ms, got %v", ht.retryDelay)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		ht, err := NewHTTPTransport(HTTPTransportConfig{
			BaseURL:    "https://sync.example.com",
			APIKey:     "mykey",
			MaxRetries: 5,
			RetryDelay: 100 * time.Millisecond,
			Timeout:    10 * time.Second,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ht.apiKey != "mykey" {
			t.Fatalf("apiKey mismatch: got %q", ht.apiKey)
		}
		if ht.maxRetries != 5 {
			t.Fatalf("maxRetries mismatch: got %d", ht.maxRetries)
		}
	})

	t.Run("custom HTTP client", func(t *testing.T) {
		custom := &http.Client{Timeout: 99 * time.Second}
		ht, err := NewHTTPTransport(HTTPTransportConfig{
			BaseURL:    "http://localhost",
			HTTPClient: custom,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ht.client != custom {
			t.Fatal("expected custom HTTP client to be used")
		}
	})
}

// ─── ReadManifest ────────────────────────────────────────────────────────────

func TestHTTPTransportReadManifestEmpty(t *testing.T) {
	srv := testServer(t, "test-key")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "test-key")
	m, err := ht.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if m.Version != 0 {
		t.Fatalf("expected Version=0 for empty JSON, got %d", m.Version)
	}
	if len(m.Chunks) != 0 {
		t.Fatalf("expected no chunks, got %d", len(m.Chunks))
	}
}

func TestHTTPTransportReadManifestWithChunks(t *testing.T) {
	srv := testServer(t, "test-key")
	defer srv.Close()

	// Pre-write a manifest via the transport.
	ht := newTestHTTPTransport(t, srv.URL, "test-key")

	want := &Manifest{
		Version: 1,
		Chunks: []ChunkEntry{
			{ID: "abc12345", CreatedBy: "alice", CreatedAt: "2025-06-01T00:00:00Z", Sessions: 1, Memories: 2, Prompts: 3},
		},
	}

	if err := ht.WriteManifest(want); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := ht.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if len(got.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got.Chunks))
	}
	if got.Chunks[0].ID != "abc12345" {
		t.Fatalf("chunk ID mismatch: got %q", got.Chunks[0].ID)
	}
	if got.Chunks[0].Memories != 2 {
		t.Fatalf("memories mismatch: got %d", got.Chunks[0].Memories)
	}
}

func TestHTTPTransportReadManifestUnauthorized(t *testing.T) {
	srv := testServer(t, "correct-key")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "wrong-key")
	_, err := ht.ReadManifest()
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

// ─── WriteManifest ───────────────────────────────────────────────────────────

func TestHTTPTransportWriteManifestRoundtrip(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "")

	want := &Manifest{
		Version: 1,
		Chunks: []ChunkEntry{
			{ID: "deadbeef", CreatedBy: "bob", CreatedAt: "2025-07-01T12:00:00Z", Sessions: 3, Memories: 5, Prompts: 1},
			{ID: "cafebabe", CreatedBy: "alice", CreatedAt: "2025-07-02T08:00:00Z", Sessions: 2, Memories: 4, Prompts: 0},
		},
	}

	if err := ht.WriteManifest(want); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := ht.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if len(got.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got.Chunks))
	}
	if got.Chunks[0].ID != "deadbeef" || got.Chunks[1].ID != "cafebabe" {
		t.Fatalf("chunk IDs mismatch: got %+v", got.Chunks)
	}
}

// ─── WriteChunk / ReadChunk ──────────────────────────────────────────────────

func TestHTTPTransportChunkRoundtrip(t *testing.T) {
	srv := testServer(t, "secret")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "secret")

	payload := []byte(`{"sessions":[{"id":"s1"}],"observations":[{"id":1}],"prompts":[]}`)
	entry := ChunkEntry{ID: "aabbccdd", CreatedBy: "charlie"}

	if err := ht.WriteChunk("aabbccdd", payload, entry); err != nil {
		t.Fatalf("WriteChunk: %v", err)
	}

	got, err := ht.ReadChunk("aabbccdd")
	if err != nil {
		t.Fatalf("ReadChunk: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("chunk data mismatch:\ngot  %q\nwant %q", got, payload)
	}
}

func TestHTTPTransportChunkLargePayload(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "")

	// Simulate a ~64KB chunk.
	var buf bytes.Buffer
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&buf, `{"observations":[{"id":%d,"title":"obs-%d","content":"%s"}]}`, i, i, strings.Repeat("x", 50))
	}
	payload := buf.Bytes()

	if err := ht.WriteChunk("bigchunk", payload, ChunkEntry{ID: "bigchunk"}); err != nil {
		t.Fatalf("WriteChunk large: %v", err)
	}

	got, err := ht.ReadChunk("bigchunk")
	if err != nil {
		t.Fatalf("ReadChunk large: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("large chunk data mismatch: got %d bytes, want %d bytes", len(got), len(payload))
	}
}

func TestHTTPTransportReadChunkNotFound(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "")
	_, err := ht.ReadChunk("nonexistent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

// ─── Chunk data integrity ────────────────────────────────────────────────────

func TestHTTPTransportChunkJSONRoundtrip(t *testing.T) {
	srv := testServer(t, "key123")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "key123")

	original := map[string]any{
		"sessions": []map[string]any{
			{"id": "s1", "project": "proj", "directory": "/tmp", "started_at": "2025-01-01 00:00:00"},
		},
		"observations": []map[string]any{
			{"id": 1, "session_id": "s1", "type": "decision", "title": "test", "content": "data", "scope": "project", "created_at": "2025-01-01 00:00:00", "updated_at": "2025-01-01 00:00:00"},
		},
		"prompts": []map[string]any{
			{"id": 1, "session_id": "s1", "content": "hello", "created_at": "2025-01-01 00:00:00"},
		},
	}

	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := ht.WriteChunk("deadbeef", payload, ChunkEntry{ID: "deadbeef"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ht.ReadChunk("deadbeef")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatalf("unmarshal roundtrip: %v", err)
	}

	sessions, ok := result["sessions"].([]any)
	if !ok || len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %v", result["sessions"])
	}
}

// ─── Auth ────────────────────────────────────────────────────────────────────

func TestHTTPTransportAPIKeyRequired(t *testing.T) {
	srv := testServer(t, "required-key")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "wrong-key")
	err := ht.WriteManifest(&Manifest{Version: 1})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestHTTPTransportNoAPIKeyWhenServerExpectsNone(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "")
	err := ht.WriteManifest(&Manifest{Version: 1})
	if err != nil {
		t.Fatalf("expected no error without API key, got %v", err)
	}
}

// ─── Retry / Error handling ──────────────────────────────────────────────────

func TestHTTPTransportRetryOnServerErrors(t *testing.T) {
	var attemptCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount <= 2 {
			http.Error(w, "transient error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ht, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL:    srv.URL,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}

	err = ht.WriteManifest(&Manifest{Version: 1})
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if attemptCount != 3 {
		t.Fatalf("expected 3 attempts, got %d", attemptCount)
	}
}

func TestHTTPTransportRetryExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "always fails", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ht, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL:    srv.URL,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}

	err = ht.WriteManifest(&Manifest{Version: 1})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	// The error should mention the 500 server error.
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected server error in message, got %v", err)
	}
}

func TestHTTPTransportConnectionRefused(t *testing.T) {
	// Use a port that's almost certainly not listening.
	ht, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL:    "http://127.0.0.1:1",
		MaxRetries: 1,
		RetryDelay: 10 * time.Millisecond,
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}

	_, err = ht.ReadManifest()
	if err == nil {
		t.Fatal("expected connection refused error")
	}
}

// ─── Full sync flow via HTTP ─────────────────────────────────────────────────

func TestHTTPTransportFullSyncFlow(t *testing.T) {
	srv := testServer(t, "flow-key")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "flow-key")

	// Step 1: Read empty manifest.
	m, err := ht.ReadManifest()
	if err != nil {
		t.Fatalf("read empty manifest: %v", err)
	}
	if len(m.Chunks) != 0 {
		t.Fatalf("expected empty manifest, got %d chunks", len(m.Chunks))
	}

	// Step 2: Write a chunk.
	chunkData := []byte(`{"sessions":[{"id":"s1"}],"observations":[],"prompts":[]}`)
	if err := ht.WriteChunk("abc12345", chunkData, ChunkEntry{ID: "abc12345", CreatedBy: "alice"}); err != nil {
		t.Fatalf("write chunk: %v", err)
	}

	// Step 3: Read the chunk back.
	got, err := ht.ReadChunk("abc12345")
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if string(got) != string(chunkData) {
		t.Fatalf("chunk roundtrip mismatch")
	}

	// Step 4: Write manifest with the chunk entry.
	m.Chunks = append(m.Chunks, ChunkEntry{
		ID:        "abc12345",
		CreatedBy: "alice",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Sessions:  1,
	})
	if err := ht.WriteManifest(m); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	// Step 5: Read manifest back — should have 1 chunk.
	m2, err := ht.ReadManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(m2.Chunks) != 1 || m2.Chunks[0].ID != "abc12345" {
		t.Fatalf("manifest roundtrip mismatch: %+v", m2.Chunks)
	}
}

// ─── URL path escaping ───────────────────────────────────────────────────────

func TestHTTPTransportChunkIDSafePaths(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "")

	// Normal hex chunk ID should work fine.
	payload := []byte("test data")
	if err := ht.WriteChunk("a1b2c3d4", payload, ChunkEntry{ID: "a1b2c3d4"}); err != nil {
		t.Fatalf("write chunk: %v", err)
	}

	got, err := ht.ReadChunk("a1b2c3d4")
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("data mismatch")
	}
}

// ─── Interface compliance ────────────────────────────────────────────────────

func TestHTTPTransportImplementsInterface(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()

	ht := newTestHTTPTransport(t, srv.URL, "")

	// This function accepts a Transport — compile-time check passed above,
	// but let's verify runtime too.
	var _ Transport = ht
}

// ─── TruncateBody helper ─────────────────────────────────────────────────────

func TestTruncateBody(t *testing.T) {
	short := []byte("hello")
	if got := truncateBody(short); got != "hello" {
		t.Fatalf("short body mismatch: got %q", got)
	}

	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	got := truncateBody(long)
	if len(got) != 256+3 { // 256 + "..."
		t.Fatalf("expected truncated body len 259, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ... suffix, got %q", got)
	}
}

// ─── Integration: Syncer with HTTPTransport ──────────────────────────────────

func TestSyncerWithHTTPTransportIntegration(t *testing.T) {
	srv := testServer(t, "integ-key")
	defer srv.Close()

	s := newTestStore(t)
	seedStoreForSync(t, s)

	ht, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL:    srv.URL,
		APIKey:     "integ-key",
		MaxRetries: 1,
		RetryDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}

	sy := NewWithTransport(s, ht)

	// Export via HTTP transport.
	result, err := sy.Export("alice", "proj-a")
	if err != nil {
		t.Fatalf("export via HTTP: %v", err)
	}
	if result.IsEmpty {
		t.Fatal("expected non-empty export")
	}
	if result.ChunkID == "" {
		t.Fatal("expected chunk ID")
	}

	// Import via HTTP transport into a fresh store.
	dstStore := newTestStore(t)
	dstHT, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL:    srv.URL,
		APIKey:     "integ-key",
		MaxRetries: 1,
		RetryDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport dst: %v", err)
	}
	dstSyncer := NewWithTransport(dstStore, dstHT)

	importResult, err := dstSyncer.Import()
	if err != nil {
		t.Fatalf("import via HTTP: %v", err)
	}
	if importResult.ChunksImported != 1 {
		t.Fatalf("expected 1 chunk imported, got %d", importResult.ChunksImported)
	}
	if importResult.SessionsImported != 1 || importResult.ObservationsImported != 1 || importResult.PromptsImported != 1 {
		t.Fatalf("unexpected import counts: %+v", importResult)
	}

	// Second import should skip (already synced).
	importAgain, err := dstSyncer.Import()
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if importAgain.ChunksImported != 0 || importAgain.ChunksSkipped != 1 {
		t.Fatalf("expected skip on second import, got %+v", importAgain)
	}
}
