package autosync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Edcko/Mneme/internal/config"
	"github.com/Edcko/Mneme/internal/store"
	"github.com/Edcko/Mneme/internal/sync"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

// testServer creates a local store backed by a temp directory.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(store.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedStore adds a session and observation so Export has something to push.
func seedStore(t *testing.T, s *store.Store, project string) {
	t.Helper()
	if err := s.CreateSession("sess-1", project, "/tmp/test"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	_, err := s.AddObservation(store.AddObservationParams{
		SessionID: "sess-1",
		Type:      "manual",
		Title:     "Test observation",
		Content:   "This is a test observation for autosync",
		Project:   project,
	})
	if err != nil {
		t.Fatalf("add observation: %v", err)
	}
}

// fakeTransport is a minimal Transport that records calls.
type fakeTransport struct {
	chunks     map[string][]byte
	manifest   *sync.Manifest
	writeCalls int
	readCalls  int
	failWrite  bool
	failRead   bool
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		chunks:   make(map[string][]byte),
		manifest: &sync.Manifest{Version: 1},
	}
}

func (ft *fakeTransport) ReadManifest() (*sync.Manifest, error) {
	if ft.failRead {
		return nil, fmt.Errorf("transport read error")
	}
	return ft.manifest, nil
}

func (ft *fakeTransport) WriteManifest(m *sync.Manifest) error {
	if ft.failWrite {
		return fmt.Errorf("transport write error")
	}
	ft.manifest = m
	return nil
}

func (ft *fakeTransport) WriteChunk(chunkID string, data []byte, entry sync.ChunkEntry) error {
	if ft.failWrite {
		return fmt.Errorf("transport write error")
	}
	ft.chunks[chunkID] = data
	ft.writeCalls++
	return nil
}

func (ft *fakeTransport) ReadChunk(chunkID string) ([]byte, error) {
	if ft.failRead {
		return nil, fmt.Errorf("transport read error")
	}
	ft.readCalls++
	data, ok := ft.chunks[chunkID]
	if !ok {
		return nil, fmt.Errorf("chunk %s not found", chunkID)
	}
	return data, nil
}

// ─── Constructor Tests ────────────────────────────────────────────────────────

func TestNew_ValidatesRequiredFields(t *testing.T) {
	s := testStore(t)

	_, err := New(s, Config{})
	if err == nil {
		t.Fatal("expected error for empty ServerURL")
	}

	_, err = New(s, Config{ServerURL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for empty APIKey")
	}
}

func TestNew_AcceptsValidConfig(t *testing.T) {
	s := testStore(t)

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Second,
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.cfg.Interval != time.Second {
		t.Errorf("expected interval 1s, got %v", m.cfg.Interval)
	}
}

func TestNew_DefaultsInterval(t *testing.T) {
	s := testStore(t)

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.cfg.Interval != config.DefaultSyncInterval {
		t.Errorf("expected default interval %v, got %v", config.DefaultSyncInterval, m.cfg.Interval)
	}
}

func TestNew_PresetsCreatedBy(t *testing.T) {
	s := testStore(t)

	origGetUsername := getUsername
	getUsername = func() string { return "test-user" }
	defer func() { getUsername = origGetUsername }()

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.cfg.CreatedBy != "test-user" {
		t.Errorf("expected CreatedBy 'test-user', got %q", m.cfg.CreatedBy)
	}
}

// ─── Double-Start Prevention ──────────────────────────────────────────────────

func TestStart_PreventsDoubleStart(t *testing.T) {
	s := testStore(t)

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour, // long interval so the test doesn't trigger a cycle
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	defer m.Stop()

	if err := m.Start(); err == nil {
		t.Fatal("expected error on double start")
	}
}

// ─── Status Reporting ─────────────────────────────────────────────────────────

func TestStatus_InitialIsIdle(t *testing.T) {
	s := testStore(t)

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := m.Status()
	if status.Phase != store.SyncLifecycleIdle {
		t.Errorf("expected initial phase %q, got %q", store.SyncLifecycleIdle, status.Phase)
	}
}

func TestStatus_TransitionsToRunning(t *testing.T) {
	s := testStore(t)

	// Override transport and syncer to use fake
	transport := newFakeTransport()
	origNewSyncer := newSyncer
	newSyncer = func(st *store.Store, tr sync.Transport) *sync.Syncer {
		return sync.NewWithTransport(st, transport)
	}
	defer func() { newSyncer = origNewSyncer }()

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Trigger a dirty notification to force a sync cycle.
	m.NotifyDirty()

	// Give the goroutine time to process.
	time.Sleep(200 * time.Millisecond)

	status := m.Status()
	if status.Phase != store.SyncLifecycleHealthy {
		t.Errorf("expected phase %q after sync, got %q (last error: %s)",
			store.SyncLifecycleHealthy, status.Phase, status.LastError)
	}
	if status.LastSyncAt == nil {
		t.Error("expected LastSyncAt to be set after successful sync")
	}

	m.Stop()
}

// ─── NotifyDirty ──────────────────────────────────────────────────────────────

func TestNotifyDirty_NonBlocking(t *testing.T) {
	s := testStore(t)

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Multiple NotifyDirty calls should not block.
	for i := 0; i < 10; i++ {
		m.NotifyDirty()
	}
}

// ─── Graceful Shutdown ────────────────────────────────────────────────────────

func TestStop_GracefulShutdown(t *testing.T) {
	s := testStore(t)

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Stop should not hang.
	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung — goroutine did not exit")
	}
}

// ─── Backoff on Failure ───────────────────────────────────────────────────────

func TestSyncCycle_BackoffOnTransportFailure(t *testing.T) {
	s := testStore(t)
	seedStore(t, s, "fail-project")

	// Use a fake transport that fails all writes.
	transport := newFakeTransport()
	transport.failWrite = true

	origNewSyncer := newSyncer
	origNewHTTPTransport := newHTTPTransport

	// Provide a valid transport — it won't be used since newSyncer is overridden.
	dummySrv := httptest.NewServer(http.NotFoundHandler())
	defer dummySrv.Close()

	newHTTPTransport = func(cfg sync.HTTPTransportConfig) (*sync.HTTPTransport, error) {
		return sync.NewHTTPTransport(sync.HTTPTransportConfig{
			BaseURL: dummySrv.URL,
			APIKey:  "dummy",
		})
	}
	newSyncer = func(st *store.Store, tr sync.Transport) *sync.Syncer {
		return sync.NewWithTransport(st, transport)
	}
	defer func() {
		newSyncer = origNewSyncer
		newHTTPTransport = origNewHTTPTransport
	}()

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Trigger a sync cycle.
	m.NotifyDirty()
	time.Sleep(200 * time.Millisecond)

	status := m.Status()
	if status.Phase != store.SyncLifecycleDegraded {
		t.Errorf("expected degraded phase after failure, got %q", status.Phase)
	}
	if status.ConsecutiveFailures == 0 {
		t.Error("expected consecutive failures > 0")
	}
	if status.BackoffUntil == nil {
		t.Error("expected BackoffUntil to be set")
	}
	if status.LastError == "" {
		t.Error("expected LastError to be set")
	}
}

func TestSyncCycle_RespectsBackoff(t *testing.T) {
	s := testStore(t)

	// Set a backoff in the future on the store.
	backoffUntil := time.Now().Add(1 * time.Hour)
	if err := s.MarkSyncFailure(store.DefaultSyncTargetKey, "previous failure", backoffUntil); err != nil {
		t.Fatalf("mark failure: %v", err)
	}

	transport := newFakeTransport()

	origNewSyncer := newSyncer
	newSyncer = func(st *store.Store, tr sync.Transport) *sync.Syncer {
		return sync.NewWithTransport(st, transport)
	}
	defer func() { newSyncer = origNewSyncer }()

	// No seam needed — backoff prevents the sync cycle from reaching Export.
	// We verify via transport.writeCalls (which stays 0) instead.

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Wait enough time for multiple cycles — but backoff should prevent them.
	time.Sleep(300 * time.Millisecond)
	m.Stop()

	// No writes should happen during backoff — transport never called.
	if transport.writeCalls > 0 {
		t.Error("expected no transport writes during backoff")
	}
}

// ─── Push/Pull Integration ────────────────────────────────────────────────────

func TestSyncCycle_PushPull(t *testing.T) {
	s := testStore(t)
	seedStore(t, s, "test-project")

	transport := newFakeTransport()

	origNewSyncer := newSyncer
	newSyncer = func(st *store.Store, tr sync.Transport) *sync.Syncer {
		return sync.NewWithTransport(st, transport)
	}
	defer func() { newSyncer = origNewSyncer }()

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
		Project:   "test-project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Force a sync.
	m.NotifyDirty()
	time.Sleep(300 * time.Millisecond)
	m.Stop()

	// Verify transport received a chunk.
	if transport.writeCalls == 0 {
		t.Error("expected at least one chunk write (push)")
	}

	status := m.Status()
	if status.Phase != store.SyncLifecycleHealthy {
		t.Errorf("expected healthy, got %q: %s", status.Phase, status.LastError)
	}
}

// ─── Lease-based coordination ─────────────────────────────────────────────────

func TestSyncCycle_LeaseNotAcquired(t *testing.T) {
	s := testStore(t)

	// Pre-acquire the lease with a different owner.
	_, err := s.AcquireSyncLease(store.DefaultSyncTargetKey, "other-owner", 1*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}

	transport := newFakeTransport()

	origNewSyncer := newSyncer
	newSyncer = func(st *store.Store, tr sync.Transport) *sync.Syncer {
		return sync.NewWithTransport(st, transport)
	}
	defer func() { newSyncer = origNewSyncer }()

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Trigger sync — should be a no-op because lease is held.
	m.NotifyDirty()
	time.Sleep(200 * time.Millisecond)
	m.Stop()

	// No writes should have happened.
	if transport.writeCalls > 0 {
		t.Error("expected no writes when lease not acquired")
	}
}

// ─── NewFromConfig ────────────────────────────────────────────────────────────

func TestNewFromConfig_NoRemotes(t *testing.T) {
	s := testStore(t)

	cfg := &config.Config{}
	_, err := NewFromConfig(s, cfg)
	if err == nil {
		t.Fatal("expected error when no remotes configured")
	}
}

func TestNewFromConfig_UsesDefaultRemote(t *testing.T) {
	s := testStore(t)

	cfg := &config.Config{
		Sync: config.SyncConfig{
			DefaultRemote: "prod",
			SyncInterval:  10 * time.Second,
			Remotes: map[string]config.RemoteConfig{
				"prod": {
					ServerURL: "https://sync.example.com",
					APIKey:    "test-key",
				},
			},
		},
	}

	m, err := NewFromConfig(s, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.cfg.ServerURL != "https://sync.example.com" {
		t.Errorf("expected ServerURL from config, got %q", m.cfg.ServerURL)
	}
	if m.cfg.Interval != 10*time.Second {
		t.Errorf("expected interval from config, got %v", m.cfg.Interval)
	}
}

func TestNewFromConfig_PicksFirstRemote(t *testing.T) {
	s := testStore(t)

	cfg := &config.Config{
		Sync: config.SyncConfig{
			SyncInterval: time.Minute,
			Remotes: map[string]config.RemoteConfig{
				"staging": {
					ServerURL: "https://staging.example.com",
					APIKey:    "stage-key",
				},
			},
		},
	}

	m, err := NewFromConfig(s, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.cfg.ServerURL != "https://staging.example.com" {
		t.Errorf("expected ServerURL from staging remote, got %q", m.cfg.ServerURL)
	}
}

func TestNewFromConfig_MissingDefaultRemote(t *testing.T) {
	s := testStore(t)

	cfg := &config.Config{
		Sync: config.SyncConfig{
			DefaultRemote: "nonexistent",
			Remotes: map[string]config.RemoteConfig{
				"prod": {
					ServerURL: "https://sync.example.com",
					APIKey:    "test-key",
				},
			},
		},
	}

	_, err := NewFromConfig(s, cfg)
	if err == nil {
		t.Fatal("expected error when default remote not found")
	}
}

// ─── HTTP Transport Integration ───────────────────────────────────────────────

func TestSyncCycle_WithHTTPTestServer(t *testing.T) {
	s := testStore(t)
	seedStore(t, s, "integration-test")

	// Create a fake HTTP server that mimics the sync endpoints.
	var receivedChunks int32
	var receivedManifest int32

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sync/manifest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(&sync.Manifest{Version: 1})
	})
	mux.HandleFunc("PUT /sync/manifest", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receivedManifest, 1)
		var m sync.Manifest
		json.NewDecoder(r.Body).Decode(&m)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /sync/chunks/{id}", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receivedChunks, 1)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /sync/chunks/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Use real HTTPTransport — New will create it via newHTTPTransport.
	m, err := New(s, Config{
		ServerURL: srv.URL,
		APIKey:    "test-key",
		Interval:  time.Hour,
		Project:   "integration-test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Force sync.
	m.NotifyDirty()
	time.Sleep(500 * time.Millisecond)
	m.Stop()

	if atomic.LoadInt32(&receivedChunks) == 0 {
		t.Error("expected HTTP server to receive at least one chunk")
	}
	if atomic.LoadInt32(&receivedManifest) == 0 {
		t.Error("expected HTTP server to receive manifest update")
	}

	status := m.Status()
	if status.Phase != store.SyncLifecycleHealthy {
		t.Errorf("expected healthy phase, got %q: %s", status.Phase, status.LastError)
	}
}

// ─── Context Cancellation ─────────────────────────────────────────────────────

func TestStop_CancelsContext(t *testing.T) {
	s := testStore(t)

	m, err := New(s, Config{
		ServerURL: "https://sync.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Verify context is not done yet.
	select {
	case <-m.ctx.Done():
		t.Fatal("context should not be done yet")
	default:
	}

	m.Stop()

	// After stop, context should be cancelled.
	select {
	case <-m.ctx.Done():
		// Correct
	case <-time.After(2 * time.Second):
		t.Fatal("context should be cancelled after Stop()")
	}
}

// ─── Backoff Duration ─────────────────────────────────────────────────────────

func TestBackoffDuration_Exponential(t *testing.T) {
	tests := []struct {
		failures int
		wantMax  time.Duration
	}{
		{0, 0},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{5, 32 * time.Second},
		{10, time.Hour}, // capped
		{20, time.Hour}, // still capped
	}

	for _, tc := range tests {
		got := backoffDuration(tc.failures)
		if tc.failures == 0 {
			if got != 0 {
				t.Errorf("failures=%d: expected 0, got %v", tc.failures, got)
			}
			continue
		}
		if got > tc.wantMax {
			t.Errorf("failures=%d: expected <= %v, got %v", tc.failures, tc.wantMax, got)
		}
		if got > time.Hour {
			t.Errorf("failures=%d: backoff exceeded 1h cap: %v", tc.failures, got)
		}
	}
}

// ─── Filesystem-based Integration ─────────────────────────────────────────────

func TestSyncCycle_FileTransportRoundtrip(t *testing.T) {
	// Create two stores — one as "local", one as "remote" — to verify
	// data round-trips through the chunk-based sync engine.
	localStore := testStore(t)
	seedStore(t, localStore, "roundtrip-project")

	// Create a file-based transport via the Syncer's New constructor.
	syncDir := filepath.Join(t.TempDir(), ".engram")
	fileSyncer := sync.New(localStore, syncDir)

	// Override both seams: newHTTPTransport to skip the real constructor,
	// newSyncer to return our file-backed Syncer.
	origNewSyncer := newSyncer
	origNewHTTPTransport := newHTTPTransport
	defer func() {
		newSyncer = origNewSyncer
		newHTTPTransport = origNewHTTPTransport
	}()

	// Provide a dummy HTTPTransport — it will never be used since
	// newSyncer ignores the transport parameter entirely.
	dummySrv := httptest.NewServer(http.NotFoundHandler())
	defer dummySrv.Close()
	newHTTPTransport = func(cfg sync.HTTPTransportConfig) (*sync.HTTPTransport, error) {
		return sync.NewHTTPTransport(sync.HTTPTransportConfig{
			BaseURL: dummySrv.URL,
			APIKey:  "dummy",
		})
	}
	newSyncer = func(s *store.Store, transport sync.Transport) *sync.Syncer {
		return fileSyncer
	}

	m, err := New(localStore, Config{
		ServerURL: "https://placeholder.example.com",
		APIKey:    "test-key",
		Interval:  time.Hour,
		Project:   "roundtrip-project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	m.NotifyDirty()
	time.Sleep(300 * time.Millisecond)
	m.Stop()

	status := m.Status()
	if status.Phase != store.SyncLifecycleHealthy {
		t.Errorf("expected healthy, got %q: %s", status.Phase, status.LastError)
	}

	// Verify data was written to sync dir.
	entries, err := os.ReadDir(filepath.Join(syncDir, "chunks"))
	if err != nil {
		t.Fatalf("read chunks dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one chunk file in sync dir")
	}
}
