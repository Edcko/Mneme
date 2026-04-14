// Package store includes a SQLite-backed TestStore for testing without PostgreSQL.
//
// TestStore implements the CloudStore interface using SQLite in-memory,
// with SQL dialect compatible with both SQLite and PostgreSQL semantics.
// It translates PG-specific patterns ($1 params, RETURNING, SELECT FOR UPDATE)
// to SQLite equivalents.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	stdsync "sync"
	"time"

	ssync "github.com/Edcko/Mneme/internal/sync"
	_ "modernc.org/sqlite"
)

// TestStore implements CloudStore backed by SQLite in-memory.
// Use NewTestStore to create one.
type TestStore struct {
	db *sql.DB
	// mu protects manifest append operations (SQLite has no SELECT FOR UPDATE).
	mu stdsync.Mutex
}

// NewTestStore creates an in-memory SQLite-backed CloudStore for testing.
func NewTestStore() (*TestStore, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("test store open: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("test store ping: %w", err)
	}

	s := &TestStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("test store migrate: %w", err)
	}

	return s, nil
}

// NewTestStoreWithPath creates an SQLite-backed CloudStore at a file path
// (useful for debugging test failures).
func NewTestStoreWithPath(path string) (*TestStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("test store open: %w", err)
	}

	s := &TestStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("test store migrate: %w", err)
	}

	return s, nil
}

// Close releases the database connection.
func (s *TestStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying *sql.DB for test assertions that need raw SQL.
func (s *TestStore) DB() *sql.DB { return s.db }

func (s *TestStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT    NOT NULL UNIQUE,
		owner_email TEXT    NOT NULL,
		api_secret  TEXT    NOT NULL,
		created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
		updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS api_keys (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		key_hash   TEXT    NOT NULL UNIQUE,
		label      TEXT    NOT NULL DEFAULT '',
		created_at TEXT    NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);

	CREATE TABLE IF NOT EXISTS manifests (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
		version    INTEGER NOT NULL DEFAULT 1,
		chunks     TEXT    NOT NULL DEFAULT '[]',
		created_at TEXT    NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS chunks (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		chunk_id   TEXT    NOT NULL,
		data       BLOB    NOT NULL,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		created_at TEXT    NOT NULL DEFAULT (datetime('now')),
		UNIQUE(project_id, chunk_id)
	);
	CREATE INDEX IF NOT EXISTS idx_chunks_project ON chunks(project_id);

	CREATE TABLE IF NOT EXISTS sync_log (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		direction   TEXT    NOT NULL CHECK (direction IN ('push', 'pull')),
		chunk_id    TEXT    NOT NULL DEFAULT '',
		entry_count INTEGER NOT NULL DEFAULT 0,
		remote_addr TEXT    NOT NULL DEFAULT '',
		created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_sync_log_project ON sync_log(project_id);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	return nil
}

// ─── TestStore Project Operations ────────────────────────────────────────────

func (s *TestStore) CreateProject(name, ownerEmail, apiSecret string) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`
		INSERT INTO projects (name, owner_email, api_secret)
		VALUES (?, ?, ?)
		RETURNING id, name, owner_email, api_secret, created_at, updated_at
	`, name, ownerEmail, apiSecret).Scan(
		&p.ID, &p.Name, &p.OwnerEmail, &p.APISecret, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if isDuplicate(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *TestStore) GetProjectByName(name string) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`
		SELECT id, name, owner_email, api_secret, created_at, updated_at
		FROM projects WHERE name = ?
	`, name).Scan(&p.ID, &p.Name, &p.OwnerEmail, &p.APISecret, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *TestStore) GetProjectByID(id int64) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`
		SELECT id, name, owner_email, api_secret, created_at, updated_at
		FROM projects WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.OwnerEmail, &p.APISecret, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ─── TestStore Auth Operations ───────────────────────────────────────────────

func (s *TestStore) Authenticate(rawKey string) (*Project, error) {
	keyHash := hashKey(rawKey)

	var p Project
	err := s.db.QueryRow(`
		SELECT p.id, p.name, p.owner_email, p.api_secret, p.created_at, p.updated_at
		FROM projects p
		JOIN api_keys ak ON ak.project_id = p.id
		WHERE ak.key_hash = ?
	`, keyHash).Scan(&p.ID, &p.Name, &p.OwnerEmail, &p.APISecret, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateAPIKey stores a new API key hash for a project.
// The keyHash must be the SHA-256 hex digest of the raw key.
// Returns ErrDuplicate if the key hash already exists.
func (s *TestStore) CreateAPIKey(ctx context.Context, projectID int64, keyHash, label string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (project_id, key_hash, label)
		VALUES (?, ?, ?)
	`, projectID, keyHash, label)
	if err != nil {
		if isDuplicate(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

// ─── TestStore Manifest Operations ───────────────────────────────────────────

func (s *TestStore) GetManifest(projectID int64) (*CloudManifest, error) {
	var m CloudManifest
	var chunksJSON string

	err := s.db.QueryRow(`
		SELECT id, project_id, version, chunks, created_at, updated_at
		FROM manifests WHERE project_id = ?
	`, projectID).Scan(&m.ID, &m.ProjectID, &m.Version, &chunksJSON, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		return &CloudManifest{
			ProjectID: projectID,
			Version:   1,
			Chunks:    []ssync.ChunkEntry{},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(chunksJSON), &m.Chunks); err != nil {
		return nil, fmt.Errorf("unmarshal chunks: %w", err)
	}
	return &m, nil
}

func (s *TestStore) AppendManifest(projectID int64, entries []ssync.ChunkEntry) (*CloudManifest, error) {
	// SQLite has no SELECT FOR UPDATE, so use a mutex to serialize appends.
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var m CloudManifest
	var chunksJSON string
	err = tx.QueryRow(`
		SELECT id, project_id, version, chunks, created_at, updated_at
		FROM manifests WHERE project_id = ?
	`, projectID).Scan(&m.ID, &m.ProjectID, &m.Version, &chunksJSON, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		m.ProjectID = projectID
		m.Version = 1
		m.Chunks = entries

		chunksToStore, err := json.Marshal(entries)
		if err != nil {
			return nil, fmt.Errorf("marshal chunks: %w", err)
		}

		err = tx.QueryRow(`
			INSERT INTO manifests (project_id, version, chunks)
			VALUES (?, ?, ?)
			RETURNING id, created_at, updated_at
		`, projectID, m.Version, string(chunksToStore)).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("insert manifest: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("select manifest: %w", err)
	} else {
		if err := json.Unmarshal([]byte(chunksJSON), &m.Chunks); err != nil {
			return nil, fmt.Errorf("unmarshal chunks: %w", err)
		}
		m.Chunks = append(m.Chunks, entries...)
		m.Version++

		chunksToStore, err := json.Marshal(m.Chunks)
		if err != nil {
			return nil, fmt.Errorf("marshal chunks: %w", err)
		}

		_, err = tx.Exec(`
			UPDATE manifests
			SET version = ?, chunks = ?, updated_at = datetime('now')
			WHERE id = ?
		`, m.Version, string(chunksToStore), m.ID)
		if err != nil {
			return nil, fmt.Errorf("update manifest: %w", err)
		}
		m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &m, nil
}

// ─── TestStore Chunk Operations ──────────────────────────────────────────────

func (s *TestStore) PutChunk(projectID int64, chunkID string, data []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO chunks (project_id, chunk_id, data, size_bytes)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id, chunk_id) DO NOTHING
	`, projectID, chunkID, data, len(data))
	if err != nil {
		return fmt.Errorf("put chunk: %w", err)
	}
	return nil
}

func (s *TestStore) GetChunk(projectID int64, chunkID string) (*CloudChunk, error) {
	var c CloudChunk
	err := s.db.QueryRow(`
		SELECT id, project_id, chunk_id, data, size_bytes, created_at
		FROM chunks WHERE project_id = ? AND chunk_id = ?
	`, projectID, chunkID).Scan(&c.ID, &c.ProjectID, &c.ChunkID, &c.Data, &c.SizeBytes, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *TestStore) HasChunk(projectID int64, chunkID string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`
		SELECT 1 FROM chunks WHERE project_id = ? AND chunk_id = ?
	`, projectID, chunkID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ─── TestStore Sync Log ──────────────────────────────────────────────────────

func (s *TestStore) LogSync(projectID int64, direction, chunkID string, entryCount int, remoteAddr string) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_log (project_id, direction, chunk_id, entry_count, remote_addr)
		VALUES (?, ?, ?, ?, ?)
	`, projectID, direction, chunkID, entryCount, remoteAddr)
	return err
}

func (s *TestStore) ListSyncLog(projectID int64, limit int) ([]SyncLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, project_id, direction, chunk_id, entry_count, remote_addr, created_at
		FROM sync_log WHERE project_id = ?
		ORDER BY id DESC LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []SyncLogEntry
	for rows.Next() {
		var e SyncLogEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Direction, &e.ChunkID, &e.EntryCount, &e.RemoteAddr, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
