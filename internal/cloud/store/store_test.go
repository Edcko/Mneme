package store

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"testing"

	ssync "github.com/Edcko/Mneme/internal/sync"
)

// newTestStore creates a TestStore, failing the test if setup errors.
func newTestStore(t *testing.T) *TestStore {
	t.Helper()
	s, err := NewTestStore()
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ─── Project CRUD ────────────────────────────────────────────────────────────

func TestCreateProject(t *testing.T) {
	s := newTestStore(t)

	p, err := s.CreateProject("mneme", "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID <= 0 {
		t.Error("expected non-zero project ID")
	}
	if p.Name != "mneme" {
		t.Errorf("name = %q, want %q", p.Name, "mneme")
	}
	if p.OwnerEmail != "admin@example.com" {
		t.Errorf("owner_email = %q, want %q", p.OwnerEmail, "admin@example.com")
	}
	if p.CreatedAt == "" {
		t.Error("expected non-empty created_at")
	}
}

func TestCreateProjectDuplicate(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateProject("mneme", "a@b.com", "secret")
	if err != nil {
		t.Fatalf("first CreateProject: %v", err)
	}

	_, err = s.CreateProject("mneme", "c@d.com", "other")
	if err != ErrDuplicate {
		t.Errorf("second CreateProject error = %v, want ErrDuplicate", err)
	}
}

func TestGetProjectByName(t *testing.T) {
	s := newTestStore(t)

	created, err := s.CreateProject("test-project", "user@test.com", "abc")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	found, err := s.GetProjectByName("test-project")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("ID = %d, want %d", found.ID, created.ID)
	}
	if found.APISecret != "abc" {
		t.Errorf("api_secret = %q, want %q", found.APISecret, "abc")
	}
}

func TestGetProjectByNameNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetProjectByName("nonexistent")
	if err != ErrNotFound {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestGetProjectByID(t *testing.T) {
	s := newTestStore(t)

	created, err := s.CreateProject("by-id-test", "u@e.com", "pass")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	found, err := s.GetProjectByID(created.ID)
	if err != nil {
		t.Fatalf("GetProjectByID: %v", err)
	}
	if found.Name != "by-id-test" {
		t.Errorf("name = %q, want %q", found.Name, "by-id-test")
	}
}

func TestGetProjectByIDNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetProjectByID(99999)
	if err != ErrNotFound {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// ─── Auth ────────────────────────────────────────────────────────────────────

func TestAuthenticate(t *testing.T) {
	s := newTestStore(t)

	p, err := s.CreateProject("auth-test", "auth@test.com", "projsecret")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Register an API key for the project
	if err := s.CreateAPIKey(p.ID, "my-api-key-123", "test-key"); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Authenticate with the key
	authProject, err := s.Authenticate("my-api-key-123")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if authProject.ID != p.ID {
		t.Errorf("project ID = %d, want %d", authProject.ID, p.ID)
	}
	if authProject.Name != "auth-test" {
		t.Errorf("project name = %q, want %q", authProject.Name, "auth-test")
	}
}

func TestAuthenticateInvalid(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Authenticate("nonexistent-key")
	if err != ErrUnauthorized {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestAuthenticateWrongKey(t *testing.T) {
	s := newTestStore(t)

	p, err := s.CreateProject("wrong-key", "w@test.com", "s")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := s.CreateAPIKey(p.ID, "correct-key", "label"); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	_, err = s.Authenticate("wrong-key")
	if err != ErrUnauthorized {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

// ─── Manifest Operations ────────────────────────────────────────────────────

func TestGetManifestEmpty(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("manifest-test", "m@t.com", "s")

	m, err := s.GetManifest(p.ID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("version = %d, want 1", m.Version)
	}
	if len(m.Chunks) != 0 {
		t.Errorf("chunks = %d entries, want 0", len(m.Chunks))
	}
}

func TestAppendManifestCreatesOnFirst(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("manifest-create", "m@t.com", "s")

	entries := []ssync.ChunkEntry{
		{ID: "a1b2c3d4", CreatedBy: "alice", CreatedAt: "2025-01-01T00:00:00Z", Sessions: 1, Memories: 5, Prompts: 2},
	}

	m, err := s.AppendManifest(p.ID, entries)
	if err != nil {
		t.Fatalf("AppendManifest: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("version = %d, want 1", m.Version)
	}
	if len(m.Chunks) != 1 {
		t.Fatalf("chunks = %d entries, want 1", len(m.Chunks))
	}
	if m.Chunks[0].ID != "a1b2c3d4" {
		t.Errorf("chunk ID = %q, want %q", m.Chunks[0].ID, "a1b2c3d4")
	}
	if m.Chunks[0].Memories != 5 {
		t.Errorf("memories = %d, want 5", m.Chunks[0].Memories)
	}
}

func TestAppendManifestAppendsToExisting(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("manifest-append", "m@t.com", "s")

	// First append
	first := []ssync.ChunkEntry{
		{ID: "aaaa1111", CreatedBy: "alice", Sessions: 1, Memories: 3},
	}
	m1, err := s.AppendManifest(p.ID, first)
	if err != nil {
		t.Fatalf("first AppendManifest: %v", err)
	}
	if m1.Version != 1 {
		t.Errorf("first version = %d, want 1", m1.Version)
	}

	// Second append
	second := []ssync.ChunkEntry{
		{ID: "bbbb2222", CreatedBy: "bob", Sessions: 2, Memories: 7},
	}
	m2, err := s.AppendManifest(p.ID, second)
	if err != nil {
		t.Fatalf("second AppendManifest: %v", err)
	}
	if m2.Version != 2 {
		t.Errorf("second version = %d, want 2", m2.Version)
	}
	if len(m2.Chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(m2.Chunks))
	}
	if m2.Chunks[0].ID != "aaaa1111" {
		t.Errorf("chunk[0].ID = %q, want %q", m2.Chunks[0].ID, "aaaa1111")
	}
	if m2.Chunks[1].ID != "bbbb2222" {
		t.Errorf("chunk[1].ID = %q, want %q", m2.Chunks[1].ID, "bbbb2222")
	}
}

func TestAppendManifestMultipleEntries(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("multi-append", "m@t.com", "s")

	entries := []ssync.ChunkEntry{
		{ID: "chunk-1", CreatedBy: "alice", Memories: 1},
		{ID: "chunk-2", CreatedBy: "alice", Memories: 2},
		{ID: "chunk-3", CreatedBy: "alice", Memories: 3},
	}
	m, err := s.AppendManifest(p.ID, entries)
	if err != nil {
		t.Fatalf("AppendManifest: %v", err)
	}
	if len(m.Chunks) != 3 {
		t.Errorf("chunks = %d, want 3", len(m.Chunks))
	}
}

func TestGetManifestAfterAppend(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("get-after-append", "m@t.com", "s")

	entries := []ssync.ChunkEntry{
		{ID: "abc123", CreatedBy: "test", Memories: 10},
	}
	if _, err := s.AppendManifest(p.ID, entries); err != nil {
		t.Fatalf("AppendManifest: %v", err)
	}

	// GetManifest should now return the appended data
	m, err := s.GetManifest(p.ID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(m.Chunks) != 1 {
		t.Errorf("chunks = %d, want 1", len(m.Chunks))
	}
	if m.Chunks[0].ID != "abc123" {
		t.Errorf("chunk ID = %q, want %q", m.Chunks[0].ID, "abc123")
	}
}

func TestManifestIsolation(t *testing.T) {
	s := newTestStore(t)

	p1, _ := s.CreateProject("proj-a", "a@t.com", "s")
	p2, _ := s.CreateProject("proj-b", "b@t.com", "s")

	s.AppendManifest(p1.ID, []ssync.ChunkEntry{{ID: "a-chunk", Memories: 1}})
	s.AppendManifest(p2.ID, []ssync.ChunkEntry{{ID: "b-chunk", Memories: 2}})

	m1, _ := s.GetManifest(p1.ID)
	m2, _ := s.GetManifest(p2.ID)

	if len(m1.Chunks) != 1 || m1.Chunks[0].ID != "a-chunk" {
		t.Errorf("proj-a manifest = %+v, expected a-chunk only", m1.Chunks)
	}
	if len(m2.Chunks) != 1 || m2.Chunks[0].ID != "b-chunk" {
		t.Errorf("proj-b manifest = %+v, expected b-chunk only", m2.Chunks)
	}
}

// ─── Chunk Operations ────────────────────────────────────────────────────────

func TestPutAndGetChunk(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("chunk-test", "c@t.com", "s")

	data := gzipChunk(t, []byte(`{"sessions":[],"observations":[],"prompts":[]}`))

	if err := s.PutChunk(p.ID, "abcd1234", data); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	chunk, err := s.GetChunk(p.ID, "abcd1234")
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if chunk.ChunkID != "abcd1234" {
		t.Errorf("chunk_id = %q, want %q", chunk.ChunkID, "abcd1234")
	}
	if chunk.SizeBytes != len(data) {
		t.Errorf("size_bytes = %d, want %d", chunk.SizeBytes, len(data))
	}
	if !bytes.Equal(chunk.Data, data) {
		t.Error("data mismatch")
	}
}

func TestPutChunkIdempotent(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("chunk-idem", "c@t.com", "s")

	data1 := gzipChunk(t, []byte(`{"v":1}`))
	data2 := gzipChunk(t, []byte(`{"v":2}`))

	if err := s.PutChunk(p.ID, "same-id", data1); err != nil {
		t.Fatalf("first PutChunk: %v", err)
	}
	// Second put with same chunk_id should be no-op (ON CONFLICT DO NOTHING)
	if err := s.PutChunk(p.ID, "same-id", data2); err != nil {
		t.Fatalf("second PutChunk: %v", err)
	}

	// Should still have the first data
	chunk, err := s.GetChunk(p.ID, "same-id")
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if !bytes.Equal(chunk.Data, data1) {
		t.Error("expected first data to be preserved (idempotent)")
	}
}

func TestGetChunkNotFound(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("chunk-nf", "c@t.com", "s")

	_, err := s.GetChunk(p.ID, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestHasChunk(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("chunk-has", "c@t.com", "s")

	data := gzipChunk(t, []byte(`{}`))

	exists, err := s.HasChunk(p.ID, "abc123")
	if err != nil {
		t.Fatalf("HasChunk before put: %v", err)
	}
	if exists {
		t.Error("expected chunk to not exist before PutChunk")
	}

	if err := s.PutChunk(p.ID, "abc123", data); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	exists, err = s.HasChunk(p.ID, "abc123")
	if err != nil {
		t.Fatalf("HasChunk after put: %v", err)
	}
	if !exists {
		t.Error("expected chunk to exist after PutChunk")
	}
}

func TestChunkIsolation(t *testing.T) {
	s := newTestStore(t)

	p1, _ := s.CreateProject("chunk-proj-a", "a@t.com", "s")
	p2, _ := s.CreateProject("chunk-proj-b", "b@t.com", "s")

	data := gzipChunk(t, []byte(`{}`))

	s.PutChunk(p1.ID, "shared-id", data)

	// p2 should not see p1's chunk
	has, err := s.HasChunk(p2.ID, "shared-id")
	if err != nil {
		t.Fatalf("HasChunk: %v", err)
	}
	if has {
		t.Error("project B should not see project A's chunk")
	}

	// p2 can create its own chunk with the same ID
	if err := s.PutChunk(p2.ID, "shared-id", data); err != nil {
		t.Fatalf("PutChunk for p2: %v", err)
	}
}

// ─── Sync Log ────────────────────────────────────────────────────────────────

func TestLogSync(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("sync-log", "s@t.com", "s")

	err := s.LogSync(p.ID, "push", "abc123", 5, "127.0.0.1")
	if err != nil {
		t.Fatalf("LogSync: %v", err)
	}

	entries, err := s.ListSyncLog(p.ID, 10)
	if err != nil {
		t.Fatalf("ListSyncLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Direction != "push" {
		t.Errorf("direction = %q, want %q", entries[0].Direction, "push")
	}
	if entries[0].ChunkID != "abc123" {
		t.Errorf("chunk_id = %q, want %q", entries[0].ChunkID, "abc123")
	}
	if entries[0].EntryCount != 5 {
		t.Errorf("entry_count = %d, want 5", entries[0].EntryCount)
	}
	if entries[0].RemoteAddr != "127.0.0.1" {
		t.Errorf("remote_addr = %q, want %q", entries[0].RemoteAddr, "127.0.0.1")
	}
}

func TestListSyncLogOrder(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("sync-order", "s@t.com", "s")

	s.LogSync(p.ID, "push", "chunk-1", 1, "")
	s.LogSync(p.ID, "pull", "chunk-2", 2, "")
	s.LogSync(p.ID, "push", "chunk-3", 3, "")

	entries, err := s.ListSyncLog(p.ID, 10)
	if err != nil {
		t.Fatalf("ListSyncLog: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	// Most recent first (SQLite autoincrement preserves order)
	if entries[0].ChunkID != "chunk-3" {
		t.Errorf("first entry chunk = %q, want %q", entries[0].ChunkID, "chunk-3")
	}
	if entries[2].ChunkID != "chunk-1" {
		t.Errorf("last entry chunk = %q, want %q", entries[2].ChunkID, "chunk-1")
	}
}

func TestListSyncLogLimit(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("sync-limit", "s@t.com", "s")

	for i := 0; i < 10; i++ {
		s.LogSync(p.ID, "push", fmt.Sprintf("chunk-%d", i), i, "")
	}

	entries, err := s.ListSyncLog(p.ID, 3)
	if err != nil {
		t.Fatalf("ListSyncLog: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("entries = %d, want 3 (limited)", len(entries))
	}
}

func TestSyncLogIsolation(t *testing.T) {
	s := newTestStore(t)

	p1, _ := s.CreateProject("log-proj-a", "a@t.com", "s")
	p2, _ := s.CreateProject("log-proj-b", "b@t.com", "s")

	s.LogSync(p1.ID, "push", "a-chunk", 1, "")

	entries, _ := s.ListSyncLog(p2.ID, 10)
	if len(entries) != 0 {
		t.Errorf("project B should have no sync log entries, got %d", len(entries))
	}
}

func TestLogSyncInvalidDirection(t *testing.T) {
	s := newTestStore(t)

	p, _ := s.CreateProject("sync-invalid", "s@t.com", "s")

	err := s.LogSync(p.ID, "invalid", "", 0, "")
	if err == nil {
		t.Error("expected error for invalid direction")
	}
}

// ─── DecompressChunk Helper ──────────────────────────────────────────────────

func TestDecompressChunk(t *testing.T) {
	original := ssync.ChunkData{
		Sessions:     nil,
		Observations: nil,
		Prompts:      nil,
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gzData := gzipChunk(t, raw)

	result, err := DecompressChunk(gzData)
	if err != nil {
		t.Fatalf("DecompressChunk: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestDecompressChunkRawJSON(t *testing.T) {
	// Test that DecompressChunk handles raw (non-gzipped) JSON gracefully
	raw := []byte(`{"sessions":[],"observations":[],"prompts":[]}`)

	result, err := DecompressChunk(raw)
	if err != nil {
		t.Fatalf("DecompressChunk raw JSON: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ─── Interface Compliance ────────────────────────────────────────────────────

func TestTestStoreImplementsCloudStore(t *testing.T) {
	// Compile-time check that TestStore satisfies CloudStore
	var _ CloudStore = (*TestStore)(nil)
}

func TestPGStoreImplementsCloudStore(t *testing.T) {
	// Compile-time check that PGStore satisfies CloudStore
	var _ CloudStore = (*PGStore)(nil)
}

// ─── Hash Key Helper ─────────────────────────────────────────────────────────

func TestHashKeyDeterministic(t *testing.T) {
	h1 := hashKey("my-api-key")
	h2 := hashKey("my-api-key")
	if h1 != h2 {
		t.Errorf("hash not deterministic: %s != %s", h1, h2)
	}

	h3 := hashKey("different-key")
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// gzipChunk compresses data for chunk storage tests.
func gzipChunk(t *testing.T, data []byte) []byte {
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
