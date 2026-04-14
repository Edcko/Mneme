// Package store implements the server-side persistence layer for Mneme cloud sync.
//
// It uses PostgreSQL for multi-tenant storage of manifests and chunks.
// The package defines a Store interface so the HTTP transport layer depends
// on the abstraction, not the PostgreSQL implementation directly.
// Tests use an SQLite-backed double (TestStore) with compatible SQL.
//
// Architecture alignment:
//   - Local SQLite  → internal/store      (source of truth, single-user)
//   - Cloud PG      → internal/cloud/store (replication, multi-tenant)
//   - HTTP contract → internal/cloud/server (handlers, not here)
package store

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Edcko/Mneme/internal/sync"
)

// ─── Domain Types ────────────────────────────────────────────────────────────

// Project represents a tenant in the cloud store. Each project isolates its
// manifests, chunks, and sync log from every other project.
type Project struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	OwnerEmail string `json:"owner_email"`
	APISecret  string `json:"-"` // never serialized to JSON responses
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// APIKey authenticates requests to the cloud sync API.
// An API key belongs to a project and may have a human-readable label.
type APIKey struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	KeyHash   string `json:"-"` // SHA-256 of the raw key
	Label     string `json:"label,omitempty"`
	CreatedAt string `json:"created_at"`
}

// CloudManifest is the server-side representation of a sync manifest.
// Unlike the local Manifest (which is an append-only list), the cloud
// manifest is stored as a row with the full chunk list serialized as JSONB.
// This lets PostgreSQL handle concurrent appends via SELECT FOR UPDATE.
type CloudManifest struct {
	ID        int64             `json:"id"`
	ProjectID int64             `json:"project_id"`
	Version   int               `json:"version"`
	Chunks    []sync.ChunkEntry `json:"chunks"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

// CloudChunk stores a compressed chunk of sync data.
// The chunk_id is content-addressed (SHA-256 prefix), matching the local
// sync engine's convention. The data field holds the gzipped JSONL payload.
type CloudChunk struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	ChunkID   string `json:"chunk_id"` // content-addressed 8-char hex
	Data      []byte `json:"-"`        // gzipped payload, never in JSON
	SizeBytes int    `json:"size_bytes"`
	CreatedAt string `json:"created_at"`
}

// SyncLogEntry records every push/pull operation for observability and debugging.
type SyncLogEntry struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	Direction  string `json:"direction"` // "push" or "pull"
	ChunkID    string `json:"chunk_id,omitempty"`
	EntryCount int    `json:"entry_count"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// ─── Store Interface ─────────────────────────────────────────────────────────

// CloudStore defines the server-side operations needed by the HTTP transport.
// The concrete PostgreSQL implementation satisfies this interface.
// Tests use a compatible SQLite-backed implementation.
type CloudStore interface {
	// Close releases all database resources.
	Close() error

	// Project operations

	// CreateProject creates a new tenant project. Returns the project with its
	// assigned ID. Returns ErrDuplicate if the project name already exists.
	CreateProject(name, ownerEmail, apiSecret string) (*Project, error)

	// GetProjectByName looks up a project by its unique name.
	GetProjectByName(name string) (*Project, error)

	// GetProjectByID looks up a project by its primary key.
	GetProjectByID(id int64) (*Project, error)

	// Auth operations

	// Authenticate validates an API key and returns the associated project.
	// Returns ErrUnauthorized if the key is invalid.
	Authenticate(rawKey string) (*Project, error)

	// CreateAPIKey stores a new API key hash for a project.
	// The keyHash must be the SHA-256 hex digest of the raw key.
	// Returns ErrDuplicate if the key hash already exists.
	CreateAPIKey(ctx context.Context, projectID int64, keyHash, label string) error

	// Manifest operations

	// GetManifest returns the current manifest for a project.
	// Returns an empty manifest (Version=1, no chunks) if none exists.
	GetManifest(projectID int64) (*CloudManifest, error)

	// AppendManifest atomically appends chunk entries to the project's manifest.
	// Uses SELECT FOR UPDATE (or equivalent) to serialize concurrent writers.
	// Returns the updated manifest.
	AppendManifest(projectID int64, entries []sync.ChunkEntry) (*CloudManifest, error)

	// Chunk operations

	// PutChunk stores a gzipped chunk for a project. The chunkID must be the
	// content-addressed hash (8-char hex prefix). If the chunk already exists
	// for this project, it is a no-op (idempotent).
	// The data parameter should already be gzipped.
	PutChunk(projectID int64, chunkID string, data []byte) error

	// GetChunk retrieves a gzipped chunk by its content-addressed ID.
	// Returns ErrNotFound if the chunk does not exist for this project.
	GetChunk(projectID int64, chunkID string) (*CloudChunk, error)

	// HasChunk returns true if the chunk exists for the given project.
	HasChunk(projectID int64, chunkID string) (bool, error)

	// Sync logging

	// LogSync records a push or pull operation for observability.
	LogSync(projectID int64, direction, chunkID string, entryCount int, remoteAddr string) error

	// ListSyncLog returns recent sync log entries for a project.
	ListSyncLog(projectID int64, limit int) ([]SyncLogEntry, error)
}

// ─── Errors ──────────────────────────────────────────────────────────────────

// Sentinel errors for the cloud store.
var (
	ErrDuplicate    = fmt.Errorf("cloud/store: duplicate")
	ErrNotFound     = fmt.Errorf("cloud/store: not found")
	ErrUnauthorized = fmt.Errorf("cloud/store: unauthorized")
)

// ─── PGStore (PostgreSQL implementation) ─────────────────────────────────────

// PGConfig holds PostgreSQL connection parameters.
type PGConfig struct {
	// DSN is the PostgreSQL data source name
	// (e.g. "postgres://user:pass@host:5432/mneme?sslmode=disable").
	DSN string
}

// PGStore implements CloudStore backed by PostgreSQL.
type PGStore struct {
	db *sql.DB
}

// NewPGStore connects to PostgreSQL, runs migrations, and returns a ready-to-use store.
func NewPGStore(cfg PGConfig) (*PGStore, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("cloud/store: DSN is required")
	}

	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("cloud/store: open: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("cloud/store: ping: %w", err)
	}

	s := &PGStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("cloud/store: migrate: %w", err)
	}

	return s, nil
}

// Close releases the database connection.
func (s *PGStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// ─── PG Migrations ───────────────────────────────────────────────────────────

func (s *PGStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id           BIGSERIAL PRIMARY KEY,
		name         TEXT    NOT NULL UNIQUE,
		owner_email  TEXT    NOT NULL,
		api_secret   TEXT    NOT NULL,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS api_keys (
		id          BIGSERIAL    PRIMARY KEY,
		project_id  BIGINT       NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		key_hash    TEXT         NOT NULL UNIQUE,
		label       TEXT         NOT NULL DEFAULT '',
		created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);

	CREATE TABLE IF NOT EXISTS manifests (
		id          BIGSERIAL    PRIMARY KEY,
		project_id  BIGINT       NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
		version     INT          NOT NULL DEFAULT 1,
		chunks      JSONB        NOT NULL DEFAULT '[]',
		created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS chunks (
		id          BIGSERIAL    PRIMARY KEY,
		project_id  BIGINT       NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		chunk_id    TEXT         NOT NULL,
		data        BYTEA        NOT NULL,
		size_bytes  INT          NOT NULL DEFAULT 0,
		created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
		UNIQUE(project_id, chunk_id)
	);
	CREATE INDEX IF NOT EXISTS idx_chunks_project ON chunks(project_id);

	CREATE TABLE IF NOT EXISTS sync_log (
		id           BIGSERIAL    PRIMARY KEY,
		project_id   BIGINT       NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		direction    TEXT         NOT NULL CHECK (direction IN ('push', 'pull')),
		chunk_id     TEXT         NOT NULL DEFAULT '',
		entry_count  INT          NOT NULL DEFAULT 0,
		remote_addr  TEXT         NOT NULL DEFAULT '',
		created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_sync_log_project ON sync_log(project_id);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	return nil
}

// ─── PG Project Operations ───────────────────────────────────────────────────

func (s *PGStore) CreateProject(name, ownerEmail, apiSecret string) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`
		INSERT INTO projects (name, owner_email, api_secret)
		VALUES ($1, $2, $3)
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

func (s *PGStore) GetProjectByName(name string) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`
		SELECT id, name, owner_email, api_secret, created_at, updated_at
		FROM projects WHERE name = $1
	`, name).Scan(&p.ID, &p.Name, &p.OwnerEmail, &p.APISecret, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PGStore) GetProjectByID(id int64) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`
		SELECT id, name, owner_email, api_secret, created_at, updated_at
		FROM projects WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.OwnerEmail, &p.APISecret, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ─── PG Auth Operations ──────────────────────────────────────────────────────

func (s *PGStore) Authenticate(rawKey string) (*Project, error) {
	keyHash := hashKey(rawKey)

	var p Project
	err := s.db.QueryRow(`
		SELECT p.id, p.name, p.owner_email, p.api_secret, p.created_at, p.updated_at
		FROM projects p
		JOIN api_keys ak ON ak.project_id = p.id
		WHERE ak.key_hash = $1
	`, keyHash).Scan(&p.ID, &p.Name, &p.OwnerEmail, &p.APISecret, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ─── PG API Key Operations ────────────────────────────────────────────────────

func (s *PGStore) CreateAPIKey(ctx context.Context, projectID int64, keyHash, label string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (project_id, key_hash, label)
		VALUES ($1, $2, $3)
	`, projectID, keyHash, label)
	if err != nil {
		if isDuplicate(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

// ─── PG Manifest Operations ──────────────────────────────────────────────────

func (s *PGStore) GetManifest(projectID int64) (*CloudManifest, error) {
	var m CloudManifest
	var chunksJSON []byte

	err := s.db.QueryRow(`
		SELECT id, project_id, version, chunks, created_at, updated_at
		FROM manifests WHERE project_id = $1
	`, projectID).Scan(&m.ID, &m.ProjectID, &m.Version, &chunksJSON, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		// Return empty manifest — caller creates on first append
		return &CloudManifest{
			ProjectID: projectID,
			Version:   1,
			Chunks:    []sync.ChunkEntry{},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(chunksJSON, &m.Chunks); err != nil {
		return nil, fmt.Errorf("unmarshal chunks: %w", err)
	}
	return &m, nil
}

func (s *PGStore) AppendManifest(projectID int64, entries []sync.ChunkEntry) (*CloudManifest, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// SELECT FOR UPDATE serializes concurrent appends for this project.
	var m CloudManifest
	var chunksJSON []byte
	err = tx.QueryRow(`
		SELECT id, project_id, version, chunks, created_at, updated_at
		FROM manifests WHERE project_id = $1
		FOR UPDATE
	`, projectID).Scan(&m.ID, &m.ProjectID, &m.Version, &chunksJSON, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		// First manifest for this project — INSERT.
		m.ProjectID = projectID
		m.Version = 1
		m.Chunks = entries
		chunksToStore, err := json.Marshal(entries)
		if err != nil {
			return nil, fmt.Errorf("marshal chunks: %w", err)
		}

		err = tx.QueryRow(`
			INSERT INTO manifests (project_id, version, chunks)
			VALUES ($1, $2, $3)
			RETURNING id, created_at, updated_at
		`, projectID, m.Version, chunksToStore).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("insert manifest: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("select manifest: %w", err)
	} else {
		// Existing manifest — append entries.
		if err := json.Unmarshal(chunksJSON, &m.Chunks); err != nil {
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
			SET version = $1, chunks = $2, updated_at = NOW()
			WHERE id = $3
		`, m.Version, chunksToStore, m.ID)
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

// ─── PG Chunk Operations ────────────────────────────────────────────────────

func (s *PGStore) PutChunk(projectID int64, chunkID string, data []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO chunks (project_id, chunk_id, data, size_bytes)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (project_id, chunk_id) DO NOTHING
	`, projectID, chunkID, data, len(data))
	if err != nil {
		return fmt.Errorf("put chunk: %w", err)
	}
	return nil
}

func (s *PGStore) GetChunk(projectID int64, chunkID string) (*CloudChunk, error) {
	var c CloudChunk
	err := s.db.QueryRow(`
		SELECT id, project_id, chunk_id, data, size_bytes, created_at
		FROM chunks WHERE project_id = $1 AND chunk_id = $2
	`, projectID, chunkID).Scan(&c.ID, &c.ProjectID, &c.ChunkID, &c.Data, &c.SizeBytes, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *PGStore) HasChunk(projectID int64, chunkID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT TRUE FROM chunks WHERE project_id = $1 AND chunk_id = $2
	`, projectID, chunkID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ─── PG Sync Log ─────────────────────────────────────────────────────────────

func (s *PGStore) LogSync(projectID int64, direction, chunkID string, entryCount int, remoteAddr string) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_log (project_id, direction, chunk_id, entry_count, remote_addr)
		VALUES ($1, $2, $3, $4, $5)
	`, projectID, direction, chunkID, entryCount, remoteAddr)
	return err
}

func (s *PGStore) ListSyncLog(projectID int64, limit int) ([]SyncLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, project_id, direction, chunk_id, entry_count, remote_addr, created_at
		FROM sync_log WHERE project_id = $1
		ORDER BY id DESC LIMIT $2
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

// ─── Helpers ─────────────────────────────────────────────────────────────────

// hashKey computes a SHA-256 hex digest of an API key for storage.
func hashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// isDuplicate checks if a database error is a unique constraint violation.
// Handles both PostgreSQL (SQLSTATE 23505) and SQLite error formats.
func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "23505") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}

// DecompressChunk decodes a gzipped chunk payload into ChunkData.
// Shared utility for both PG and test implementations.
func DecompressChunk(gzData []byte) (*sync.ChunkData, error) {
	r, err := gzip.NewReader(strings.NewReader(string(gzData)))
	if err != nil {
		// Maybe it's raw JSON (not gzipped) — try direct unmarshal
		var cd sync.ChunkData
		if jsonErr := json.Unmarshal(gzData, &cd); jsonErr == nil {
			return &cd, nil
		}
		return nil, fmt.Errorf("gzip read: %w", err)
	}
	defer r.Close()

	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip read all: %w", err)
	}

	var cd sync.ChunkData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return nil, fmt.Errorf("unmarshal chunk: %w", err)
	}
	return &cd, nil
}
