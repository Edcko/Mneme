// Package autosync provides background push/pull synchronization for Mneme.
//
// Manager runs a goroutine that periodically pushes local changes to a remote
// server and pulls remote changes back, using the existing sync engine and
// HTTPTransport. It integrates with the store's sync_state table for backoff
// and lease-based coordination, and implements SyncStatusProvider for the
// server's /sync/status endpoint.
package autosync

import (
	"context"
	"fmt"
	"log"
	"math"
	stdsync "sync"
	"time"

	"github.com/Edcko/Mneme/internal/config"
	"github.com/Edcko/Mneme/internal/store"
	"github.com/Edcko/Mneme/internal/sync"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// Status represents the current state of the autosync manager.
type Status struct {
	Phase               string     `json:"phase"`
	LastError           string     `json:"last_error,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	BackoffUntil        *time.Time `json:"backoff_until,omitempty"`
	LastSyncAt          *time.Time `json:"last_sync_at,omitempty"`
}

// Config holds the configuration for creating a Manager.
type Config struct {
	// ServerURL is the remote sync server base URL.
	ServerURL string

	// APIKey authenticates with the remote server.
	APIKey string

	// Interval controls how often background sync runs. Defaults to 5 minutes.
	Interval time.Duration

	// CreatedBy is the username used to attribute exported chunks.
	CreatedBy string

	// Project is the project filter for exports. Empty means all projects.
	Project string
}

// ─── Seams for testing ────────────────────────────────────────────────────────

// These variables are overridden in tests to avoid real side effects.
var (
	newHTTPTransport = func(cfg sync.HTTPTransportConfig) (*sync.HTTPTransport, error) {
		return sync.NewHTTPTransport(cfg)
	}

	newSyncer = func(s *store.Store, transport sync.Transport) *sync.Syncer {
		return sync.NewWithTransport(s, transport)
	}

	getUsername = sync.GetUsername

	// nowFunc returns the current time. Overridable in tests.
	nowFunc = time.Now

	// backoffDuration computes the backoff delay for a given number of
	// consecutive failures. Uses exponential backoff capped at 1 hour.
	backoffDuration = func(failures int) time.Duration {
		if failures <= 0 {
			return 0
		}
		seconds := math.Pow(2, float64(failures))
		d := time.Duration(seconds) * time.Second
		if d > time.Hour {
			d = time.Hour
		}
		return d
	}
)

// ─── Manager ──────────────────────────────────────────────────────────────────

// Manager orchestrates background push/pull sync with a remote server.
//
// It runs a single goroutine that:
//  1. Waits for the configured interval (or a dirty notification)
//  2. Acquires a sync lease to coordinate with other potential runners
//  3. Pushes local changes (Export)
//  4. Pulls remote changes (Import)
//  5. Releases the lease and reports status
//
// On failure, it marks the sync state as degraded with exponential backoff.
type Manager struct {
	store     *store.Store
	syncer    *sync.Syncer
	cfg       Config
	targetKey string

	mu     stdsync.RWMutex
	status Status

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// dirty is signaled by NotifyDirty to trigger an immediate sync cycle.
	dirty chan struct{}

	// startedMu guards the started flag.
	startedMu stdsync.Mutex
	started   bool
}

// New creates a new autosync Manager.
//
// It builds an HTTPTransport from the config, constructs a Syncer via
// NewWithTransport, and prepares internal state. Call Start() to begin
// background sync, and Stop() for graceful shutdown.
func New(s *store.Store, cfg Config) (*Manager, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("autosync: ServerURL is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("autosync: APIKey is required")
	}
	if cfg.Interval <= 0 {
		cfg.Interval = config.DefaultSyncInterval
	}
	if cfg.CreatedBy == "" {
		cfg.CreatedBy = getUsername()
	}

	transport, err := newHTTPTransport(sync.HTTPTransportConfig{
		BaseURL: cfg.ServerURL,
		APIKey:  cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("autosync: create transport: %w", err)
	}

	syncer := newSyncer(s, transport)

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		store:     s,
		syncer:    syncer,
		cfg:       cfg,
		targetKey: store.DefaultSyncTargetKey,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		dirty:     make(chan struct{}, 1),
		status: Status{
			Phase: store.SyncLifecycleIdle,
		},
	}, nil
}

// Start begins the background sync loop. Returns an error if already started.
func (m *Manager) Start() error {
	m.startedMu.Lock()
	if m.started {
		m.startedMu.Unlock()
		return fmt.Errorf("autosync: already started")
	}
	m.started = true
	m.startedMu.Unlock()

	go m.run()
	return nil
}

// Stop gracefully shuts down the background sync loop.
// It cancels the internal context and waits for the goroutine to finish.
func (m *Manager) Stop() {
	m.cancel()
	<-m.done
}

// NotifyDirty signals that local data has changed and a sync should run soon.
// This is called by the server's onWrite callback after local writes.
// Non-blocking: if a dirty signal is already pending, this is a no-op.
func (m *Manager) NotifyDirty() {
	select {
	case m.dirty <- struct{}{}:
	default:
		// Already pending — no need to queue another.
	}
}

// Status returns the current autosync status.
// Implements server.SyncStatusProvider.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// SyncStatusProvider compatibility — the server package calls Status() via interface.
// The Manager directly satisfies the Status() method shape.

// ─── Background loop ──────────────────────────────────────────────────────────

func (m *Manager) run() {
	defer close(m.done)

	m.setStatus(Status{Phase: store.SyncLifecycleIdle})

	// Initial sync on start.
	m.syncCycle()

	timer := time.NewTimer(m.cfg.Interval)
	defer timer.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.dirty:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			m.syncCycle()
			timer.Reset(m.cfg.Interval)
		case <-timer.C:
			m.syncCycle()
			timer.Reset(m.cfg.Interval)
		}
	}
}

// ─── Sync cycle ───────────────────────────────────────────────────────────────

func (m *Manager) syncCycle() {
	// Check backoff from store before attempting.
	state, err := m.store.GetSyncState(m.targetKey)
	if err != nil {
		m.recordFailure(fmt.Sprintf("read sync state: %v", err), 1)
		return
	}

	if state.BackoffUntil != nil {
		backoffTime, parseErr := time.Parse(time.RFC3339, *state.BackoffUntil)
		if parseErr == nil && nowFunc().Before(backoffTime) {
			// Still in backoff window — skip this cycle.
			m.mu.Lock()
			m.status.Phase = store.SyncLifecycleDegraded
			bt := backoffTime
			m.status.BackoffUntil = &bt
			m.status.ConsecutiveFailures = state.ConsecutiveFailures
			if state.LastError != nil {
				m.status.LastError = *state.LastError
			}
			m.mu.Unlock()
			return
		}
	}

	// Acquire lease to prevent concurrent sync runs.
	owner := fmt.Sprintf("autosync-%d", nowFunc().UnixNano())
	acquired, err := m.store.AcquireSyncLease(m.targetKey, owner, 2*m.cfg.Interval, nowFunc())
	if err != nil {
		m.recordFailure(fmt.Sprintf("acquire lease: %v", err), 1)
		return
	}
	if !acquired {
		// Another runner holds the lease — skip.
		return
	}
	defer func() {
		if releaseErr := m.store.ReleaseSyncLease(m.targetKey, owner); releaseErr != nil {
			log.Printf("[autosync] release lease: %v", releaseErr)
		}
	}()

	m.setStatus(Status{Phase: store.SyncLifecycleRunning})

	// Push: export local → remote.
	exportResult, err := m.syncer.Export(m.cfg.CreatedBy, m.cfg.Project)
	if err != nil {
		m.recordFailure(fmt.Sprintf("push: %v", err), state.ConsecutiveFailures+1)
		return
	}

	if exportResult != nil && !exportResult.IsEmpty {
		log.Printf("[autosync] pushed chunk %s (%d obs, %d sessions, %d prompts)",
			exportResult.ChunkID,
			exportResult.ObservationsExported,
			exportResult.SessionsExported,
			exportResult.PromptsExported,
		)
	}

	// Pull: import remote → local.
	importResult, err := m.syncer.Import()
	if err != nil {
		m.recordFailure(fmt.Sprintf("pull: %v", err), state.ConsecutiveFailures+1)
		return
	}

	if importResult != nil && importResult.ChunksImported > 0 {
		log.Printf("[autosync] pulled %d chunks (%d obs, %d sessions, %d prompts)",
			importResult.ChunksImported,
			importResult.ObservationsImported,
			importResult.SessionsImported,
			importResult.PromptsImported,
		)
	}

	// Success — mark healthy.
	if err := m.store.MarkSyncHealthy(m.targetKey); err != nil {
		log.Printf("[autosync] mark healthy: %v", err)
	}

	now := nowFunc()
	m.setStatus(Status{
		Phase:      store.SyncLifecycleHealthy,
		LastSyncAt: &now,
	})
}

// ─── Failure handling ─────────────────────────────────────────────────────────

func (m *Manager) recordFailure(message string, failures int) {
	backoff := backoffDuration(failures)
	backoffUntil := nowFunc().Add(backoff)

	if err := m.store.MarkSyncFailure(m.targetKey, message, backoffUntil); err != nil {
		log.Printf("[autosync] mark failure: %v", err)
	}

	m.setStatus(Status{
		Phase:               store.SyncLifecycleDegraded,
		LastError:           message,
		ConsecutiveFailures: failures,
		BackoffUntil:        &backoffUntil,
	})

	log.Printf("[autosync] sync failed (attempt %d, backoff %v): %s", failures, backoff, message)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (m *Manager) setStatus(s Status) {
	m.mu.Lock()
	m.status = s
	m.mu.Unlock()
}

// ─── Constructor from config ──────────────────────────────────────────────────

// NewFromConfig creates a Manager using the application's config package.
// It reads the default remote and constructs the autosync Config from it.
// Returns an error if no remotes are configured or the default remote is missing.
func NewFromConfig(s *store.Store, cfg *config.Config) (*Manager, error) {
	remoteName := cfg.Sync.DefaultRemote
	if remoteName == "" {
		// Pick the first remote if no default is set.
		for name := range cfg.Sync.Remotes {
			remoteName = name
			break
		}
	}

	if remoteName == "" {
		return nil, fmt.Errorf("autosync: no remotes configured")
	}

	remote, ok := cfg.Sync.Remotes[remoteName]
	if !ok {
		return nil, fmt.Errorf("autosync: remote %q not found", remoteName)
	}

	return New(s, Config{
		ServerURL: remote.ServerURL,
		APIKey:    remote.APIKey,
		Interval:  cfg.Sync.SyncInterval,
	})
}
