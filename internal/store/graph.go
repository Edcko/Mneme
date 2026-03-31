// Package store — graph.go
//
// Knowledge graph layer for Mneme. Adds entities, relations (with bi-temporal
// validity), and community groupings on top of the existing observations store.
// All storage is SQLite — zero extra dependencies.
//
// Design goals:
//   - 100% backward compatible: all original 15 MCP tools unchanged
//   - Bi-temporal relations: t_valid (when it became true) + t_invalid (when superseded)
//   - BFS graph traversal via recursive CTEs (max depth 10, cycle-safe)
//   - Label-propagation community detection in pure SQL
//   - Entity deduplication via FTS5 similarity match

package store

import (
	"fmt"
	"strings"
)

// ─── Types ───────────────────────────────────────────────────────────────────

// EntityType classifies what kind of thing an entity represents.
type EntityType string

const (
	EntityTypePerson   EntityType = "person"
	EntityTypeProject  EntityType = "project"
	EntityTypeFile     EntityType = "file"
	EntityTypeTool     EntityType = "tool"
	EntityTypeConcept  EntityType = "concept"
	EntityTypeLanguage EntityType = "language"
)

// Entity is a named node in the knowledge graph.
type Entity struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	EntityType EntityType `json:"entity_type"`
	Summary    *string    `json:"summary,omitempty"`
	Project    *string    `json:"project,omitempty"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
}

// Relation is a directed, bi-temporally-tracked edge between two entities.
// TInvalid == nil means the relation is currently active.
type Relation struct {
	ID            int64   `json:"id"`
	SourceID      int64   `json:"source_id"`
	SourceName    string  `json:"source_name"`
	Relation      string  `json:"relation"`
	TargetID      int64   `json:"target_id"`
	TargetName    string  `json:"target_name"`
	ObservationID *int64  `json:"observation_id,omitempty"`
	TValid        string  `json:"t_valid"`
	TInvalid      *string `json:"t_invalid,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

// GraphNode wraps an Entity with its BFS traversal depth from the seed.
type GraphNode struct {
	Entity
	Depth int `json:"depth"`
}

// GraphResult is the output of a BFS traversal.
type GraphResult struct {
	Seed      Entity      `json:"seed"`
	Nodes     []GraphNode `json:"nodes"`
	Relations []Relation  `json:"relations"`
}

// Community is a cluster of strongly connected entities.
type Community struct {
	ID        int64    `json:"id"`
	Summary   *string  `json:"summary,omitempty"`
	Project   *string  `json:"project,omitempty"`
	Members   []Entity `json:"members,omitempty"`
	UpdatedAt string   `json:"updated_at"`
}

// ─── Schema Migration ─────────────────────────────────────────────────────────

// migrateGraph adds the graph tables to the database. Called from migrate().
// All statements use IF NOT EXISTS so they're safe to run on every startup.
func (s *Store) migrateGraph() error {
	schema := `
		CREATE TABLE IF NOT EXISTS entities (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT    NOT NULL,
			entity_type TEXT    NOT NULL,
			summary     TEXT,
			project     TEXT,
			created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_entities_name    ON entities(name COLLATE NOCASE);
		CREATE INDEX IF NOT EXISTS idx_entities_type    ON entities(entity_type);
		CREATE INDEX IF NOT EXISTS idx_entities_project ON entities(project);
		CREATE INDEX IF NOT EXISTS idx_entities_name_project ON entities(name COLLATE NOCASE, project);

		CREATE VIRTUAL TABLE IF NOT EXISTS entities_fts USING fts5(
			name,
			summary,
			entity_type,
			project,
			content='entities',
			content_rowid='id'
		);

		CREATE TABLE IF NOT EXISTS relations (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id      INTEGER NOT NULL REFERENCES entities(id),
			relation       TEXT    NOT NULL,
			target_id      INTEGER NOT NULL REFERENCES entities(id),
			observation_id INTEGER REFERENCES observations(id),
			t_valid        TEXT    NOT NULL DEFAULT (datetime('now')),
			t_invalid      TEXT,
			created_at     TEXT    NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_rel_source       ON relations(source_id);
		CREATE INDEX IF NOT EXISTS idx_rel_target       ON relations(target_id);
		CREATE INDEX IF NOT EXISTS idx_rel_source_type  ON relations(source_id, relation);
		CREATE INDEX IF NOT EXISTS idx_rel_pair         ON relations(source_id, target_id, relation);

		CREATE TABLE IF NOT EXISTS communities (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			summary    TEXT,
			project    TEXT,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS community_members (
			community_id INTEGER NOT NULL REFERENCES communities(id),
			entity_id    INTEGER NOT NULL REFERENCES entities(id),
			PRIMARY KEY (community_id, entity_id)
		);

		CREATE INDEX IF NOT EXISTS idx_community_entity ON community_members(entity_id);
	`
	if _, err := s.execHook(s.db, schema); err != nil {
		return fmt.Errorf("graph schema: %w", err)
	}

	// FTS5 triggers for entities — keeps the virtual table in sync.
	if err := s.ensureEntityFTSTriggers(); err != nil {
		return err
	}

	return nil
}

func (s *Store) ensureEntityFTSTriggers() error {
	// Check whether the insert trigger already exists.
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='entities_fts_insert'`,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("graph triggers check: %w", err)
	}
	if n > 0 {
		return nil // already created
	}

	triggers := `
		CREATE TRIGGER entities_fts_insert AFTER INSERT ON entities BEGIN
			INSERT INTO entities_fts(rowid, name, summary, entity_type, project)
			VALUES (new.id, new.name, new.summary, new.entity_type, new.project);
		END;

		CREATE TRIGGER entities_fts_delete AFTER DELETE ON entities BEGIN
			INSERT INTO entities_fts(entities_fts, rowid, name, summary, entity_type, project)
			VALUES ('delete', old.id, old.name, old.summary, old.entity_type, old.project);
		END;

		CREATE TRIGGER entities_fts_update AFTER UPDATE ON entities BEGIN
			INSERT INTO entities_fts(entities_fts, rowid, name, summary, entity_type, project)
			VALUES ('delete', old.id, old.name, old.summary, old.entity_type, old.project);
			INSERT INTO entities_fts(rowid, name, summary, entity_type, project)
			VALUES (new.id, new.name, new.summary, new.entity_type, new.project);
		END;
	`
	if _, err := s.execHook(s.db, triggers); err != nil {
		return fmt.Errorf("graph triggers: %w", err)
	}
	return nil
}

// ─── Entity Operations ────────────────────────────────────────────────────────

// UpsertEntity creates or updates an entity by (name, project).
// If an entity with the same name exists in the same project, its summary is
// updated and the existing ID is returned. Otherwise a new entity is created.
func (s *Store) UpsertEntity(name string, entityType EntityType, summary, project string) (int64, error) {
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("entity name cannot be empty")
	}

	tx, err := s.beginTxHook()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Try to find existing entity with the same name in the same project (case-insensitive).
	var existingID int64
	err = tx.QueryRow(`
		SELECT id FROM entities
		WHERE name = ? COLLATE NOCASE
		  AND ifnull(project, '') = ifnull(?, '')
		LIMIT 1
	`, name, project).Scan(&existingID)

	if err == nil {
		// Entity exists — update summary if provided.
		if summary != "" {
			if _, err := tx.Exec(`
				UPDATE entities
				SET summary    = ?,
				    updated_at = datetime('now')
				WHERE id = ?
			`, summary, existingID); err != nil {
				return 0, fmt.Errorf("update entity summary: %w", err)
			}
		}
		if err := s.commitHook(tx); err != nil {
			return 0, err
		}
		return existingID, nil
	}

	// Create new entity.
	var proj *string
	if project != "" {
		proj = &project
	}
	var sum *string
	if summary != "" {
		sum = &summary
	}

	res, err := tx.Exec(`
		INSERT INTO entities (name, entity_type, summary, project)
		VALUES (?, ?, ?, ?)
	`, name, string(entityType), sum, proj)
	if err != nil {
		return 0, fmt.Errorf("insert entity: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := s.commitHook(tx); err != nil {
		return 0, err
	}
	return id, nil
}

// GetEntityByID returns an entity by its primary key.
func (s *Store) GetEntityByID(id int64) (*Entity, error) {
	row := s.db.QueryRow(`
		SELECT id, name, entity_type, summary, project, created_at, updated_at
		FROM entities WHERE id = ?
	`, id)
	return scanEntity(row)
}

// GetEntityByName returns the first entity matching name + project (case-insensitive).
func (s *Store) GetEntityByName(name, project string) (*Entity, error) {
	row := s.db.QueryRow(`
		SELECT id, name, entity_type, summary, project, created_at, updated_at
		FROM entities
		WHERE name = ? COLLATE NOCASE
		  AND ifnull(project, '') = ifnull(?, '')
		LIMIT 1
	`, name, project)
	return scanEntity(row)
}

// SearchEntities performs FTS5 search across entity names and summaries.
func (s *Store) SearchEntities(query, entityType, project string, limit int) ([]Entity, error) {
	if limit <= 0 {
		limit = 20
	}

	sanitized := sanitizeFTS(query)
	if sanitized == "" {
		return nil, nil
	}

	args := []any{sanitized}
	filters := ""

	if entityType != "" {
		filters += " AND e.entity_type = ?"
		args = append(args, entityType)
	}
	if project != "" {
		filters += " AND ifnull(e.project, '') = ?"
		args = append(args, project)
	}
	args = append(args, limit)

	rows, err := s.queryHook(s.db, `
		SELECT e.id, e.name, e.entity_type, e.summary, e.project, e.created_at, e.updated_at
		FROM entities e
		JOIN entities_fts f ON e.id = f.rowid
		WHERE entities_fts MATCH ?`+filters+`
		ORDER BY f.rank
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	defer rows.Close()

	var results []Entity
	for rows.Next() {
		e, err := scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *e)
	}
	return results, rows.Err()
}

// ListEntities returns entities filtered by type and/or project.
func (s *Store) ListEntities(entityType, project string, limit int) ([]Entity, error) {
	if limit <= 0 {
		limit = 50
	}

	args := []any{}
	where := "1=1"

	if entityType != "" {
		where += " AND entity_type = ?"
		args = append(args, entityType)
	}
	if project != "" {
		where += " AND ifnull(project, '') = ?"
		args = append(args, project)
	}
	args = append(args, limit)

	rows, err := s.queryHook(s.db, `
		SELECT id, name, entity_type, summary, project, created_at, updated_at
		FROM entities
		WHERE `+where+`
		ORDER BY updated_at DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}
	defer rows.Close()

	var results []Entity
	for rows.Next() {
		e, err := scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *e)
	}
	return results, rows.Err()
}

// ─── Relation Operations ──────────────────────────────────────────────────────

// AddRelation creates a new directed relation between two entities.
// If an active relation of the same type already exists between these entities,
// the old one is invalidated (t_invalid = now) before the new one is inserted.
// This implements the bi-temporal "supersede" pattern from Zep/Graphiti.
func (s *Store) AddRelation(sourceID, targetID int64, relation string, observationID *int64) (int64, error) {
	tx, err := s.beginTxHook()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Invalidate all active outgoing relations of the same type from this source.
	// Semantics: "Project now uses X" supersedes "Project used Y" for the same relation.
	if _, err := tx.Exec(`
		UPDATE relations
		SET t_invalid = datetime('now')
		WHERE source_id = ?
		  AND relation  = ?
		  AND t_invalid IS NULL
	`, sourceID, relation); err != nil {
		return 0, fmt.Errorf("invalidate old relation: %w", err)
	}

	// Insert new relation.
	res, err := tx.Exec(`
		INSERT INTO relations (source_id, relation, target_id, observation_id, t_valid)
		VALUES (?, ?, ?, ?, datetime('now'))
	`, sourceID, relation, targetID, observationID)
	if err != nil {
		return 0, fmt.Errorf("insert relation: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := s.commitHook(tx); err != nil {
		return 0, err
	}
	return id, nil
}

// InvalidateRelation marks a relation as no longer active (t_invalid = now).
func (s *Store) InvalidateRelation(id int64) error {
	_, err := s.execHook(s.db, `
		UPDATE relations SET t_invalid = datetime('now') WHERE id = ? AND t_invalid IS NULL
	`, id)
	return err
}

// GetEntityRelations returns relations for an entity (as source or target).
// activeOnly = true filters to only currently valid relations (t_invalid IS NULL).
func (s *Store) GetEntityRelations(entityID int64, activeOnly bool) ([]Relation, error) {
	filter := ""
	if activeOnly {
		filter = "AND r.t_invalid IS NULL"
	}

	rows, err := s.queryHook(s.db, `
		SELECT r.id,
		       r.source_id, src.name,
		       r.relation,
		       r.target_id, tgt.name,
		       r.observation_id,
		       r.t_valid, r.t_invalid,
		       r.created_at
		FROM relations r
		JOIN entities src ON src.id = r.source_id
		JOIN entities tgt ON tgt.id = r.target_id
		WHERE (r.source_id = ? OR r.target_id = ?)
		`+filter+`
		ORDER BY r.t_valid DESC
	`, entityID, entityID)
	if err != nil {
		return nil, fmt.Errorf("get entity relations: %w", err)
	}
	defer rows.Close()

	return scanRelations(rows)
}

// GetRelationHistory returns all historical states of a relation between two
// entities (active + invalidated), ordered by t_valid descending.
// This is the bi-temporal "what was true when" query.
func (s *Store) GetRelationHistory(sourceID, targetID int64, relation string) ([]Relation, error) {
	rows, err := s.queryHook(s.db, `
		SELECT r.id,
		       r.source_id, src.name,
		       r.relation,
		       r.target_id, tgt.name,
		       r.observation_id,
		       r.t_valid, r.t_invalid,
		       r.created_at
		FROM relations r
		JOIN entities src ON src.id = r.source_id
		JOIN entities tgt ON tgt.id = r.target_id
		WHERE r.source_id = ?
		  AND r.target_id = ?
		  AND r.relation  = ?
		ORDER BY r.t_valid DESC
	`, sourceID, targetID, relation)
	if err != nil {
		return nil, fmt.Errorf("get relation history: %w", err)
	}
	defer rows.Close()

	return scanRelations(rows)
}

// ─── Graph Traversal ──────────────────────────────────────────────────────────

// GraphBFS performs a breadth-first traversal starting from seedEntityID,
// following active relations (t_invalid IS NULL) up to maxDepth hops.
// Cycle detection uses path-string tracking — no node is visited twice.
func (s *Store) GraphBFS(seedEntityID int64, maxDepth int, project string) (*GraphResult, error) {
	if maxDepth <= 0 || maxDepth > 10 {
		maxDepth = 5
	}

	seed, err := s.GetEntityByID(seedEntityID)
	if err != nil {
		return nil, fmt.Errorf("seed entity %d not found: %w", seedEntityID, err)
	}

	// Args order must match SQL placeholders exactly:
	// 1. SELECT ?   → seedEntityID (anchor row)
	// 2. ',' || ? || ','  → seedEntityID (initial path)
	// 3. WHERE bfs.depth < ?  → maxDepth
	// 4,5. project filters (if set)
	// 6. WHERE b.entity_id != ?  → seedEntityID (exclude seed from results)
	projectFilter := ""
	args := []any{seedEntityID, seedEntityID, maxDepth}
	if project != "" {
		projectFilter = "AND (src.project IS NULL OR src.project = ? OR tgt.project IS NULL OR tgt.project = ?)"
		args = append(args, project, project)
	}

	// Recursive CTE: BFS with cycle detection via comma-separated path string.
	// Using UNION (not UNION ALL) prevents visiting the same node twice.
	rows, err := s.queryHook(s.db, `
		WITH RECURSIVE bfs(entity_id, depth, path) AS (
			SELECT ?, 0, ',' || ? || ','
			UNION
			SELECT
				CASE WHEN r.source_id = bfs.entity_id THEN r.target_id
				     ELSE r.source_id END,
				bfs.depth + 1,
				bfs.path || CASE WHEN r.source_id = bfs.entity_id THEN r.target_id
				                 ELSE r.source_id END || ','
			FROM relations r
			JOIN bfs ON (r.source_id = bfs.entity_id OR r.target_id = bfs.entity_id)
			JOIN entities src ON src.id = r.source_id
			JOIN entities tgt ON tgt.id = r.target_id
			WHERE bfs.depth < ?
			  AND r.t_invalid IS NULL
			  AND bfs.path NOT LIKE '%,' || CASE WHEN r.source_id = bfs.entity_id
			                                     THEN r.target_id
			                                     ELSE r.source_id END || ',%'
			  `+projectFilter+`
		)
		SELECT DISTINCT e.id, e.name, e.entity_type, e.summary, e.project,
		                e.created_at, e.updated_at,
		                MIN(b.depth) as depth
		FROM bfs b
		JOIN entities e ON e.id = b.entity_id
		WHERE b.entity_id != ?
		GROUP BY e.id
		ORDER BY depth, e.name
	`, append(args, seedEntityID)...)
	if err != nil {
		return nil, fmt.Errorf("graph BFS: %w", err)
	}
	defer rows.Close()

	var nodes []GraphNode
	nodeIDs := []int64{seedEntityID}
	for rows.Next() {
		var n GraphNode
		var summary, project *string
		if err := rows.Scan(
			&n.ID, &n.Name, &n.EntityType, &summary, &project,
			&n.CreatedAt, &n.UpdatedAt, &n.Depth,
		); err != nil {
			return nil, err
		}
		n.Summary = summary
		n.Project = project
		nodes = append(nodes, n)
		nodeIDs = append(nodeIDs, n.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch all active relations between the traversed nodes.
	relations, err := s.relationsAmongNodes(nodeIDs)
	if err != nil {
		return nil, err
	}

	return &GraphResult{
		Seed:      *seed,
		Nodes:     nodes,
		Relations: relations,
	}, nil
}

// relationsAmongNodes returns all active relations where both endpoints are in nodeIDs.
func (s *Store) relationsAmongNodes(nodeIDs []int64) ([]Relation, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(nodeIDs))
	placeholders = placeholders[:len(placeholders)-1]

	// Build args slice: nodeIDs twice (for source and target IN clauses)
	args := make([]any, 0, len(nodeIDs)*2)
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	for _, id := range nodeIDs {
		args = append(args, id)
	}

	rows, err := s.queryHook(s.db, `
		SELECT r.id,
		       r.source_id, src.name,
		       r.relation,
		       r.target_id, tgt.name,
		       r.observation_id,
		       r.t_valid, r.t_invalid,
		       r.created_at
		FROM relations r
		JOIN entities src ON src.id = r.source_id
		JOIN entities tgt ON tgt.id = r.target_id
		WHERE r.source_id IN (`+placeholders+`)
		  AND r.target_id IN (`+placeholders+`)
		  AND r.t_invalid IS NULL
		ORDER BY r.t_valid DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("relations among nodes: %w", err)
	}
	defer rows.Close()

	return scanRelations(rows)
}

// ─── Community Detection ──────────────────────────────────────────────────────

// RebuildCommunities recomputes connected components using label propagation
// and stores them in the communities + community_members tables.
// This is meant to be called periodically, not on every mem_save.
func (s *Store) RebuildCommunities(project string) error {
	tx, err := s.beginTxHook()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Load all entities and active edges for this project, then run
	// union-find in Go. SQLite does not support aggregates in recursive CTEs,
	// so this is more reliable than pure SQL label propagation.
	components, err := s.connectedComponents(project)
	if err != nil {
		return fmt.Errorf("community detection: %w", err)
	}

	// Clear existing communities for this project.
	if project != "" {
		if _, err := tx.Exec(`
			DELETE FROM community_members
			WHERE community_id IN (SELECT id FROM communities WHERE ifnull(project,'') = ?)
		`, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM communities WHERE ifnull(project,'') = ?`, project); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`DELETE FROM community_members`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM communities`); err != nil {
			return err
		}
	}

	// Insert new communities (only those with > 1 member are interesting).
	var proj *string
	if project != "" {
		proj = &project
	}
	for _, members := range components {
		if len(members) < 2 {
			continue
		}
		res, err := tx.Exec(`
			INSERT INTO communities (project, updated_at)
			VALUES (?, datetime('now'))
		`, proj)
		if err != nil {
			return fmt.Errorf("insert community: %w", err)
		}
		cid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, eid := range members {
			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO community_members (community_id, entity_id) VALUES (?, ?)
			`, cid, eid); err != nil {
				return fmt.Errorf("insert community member: %w", err)
			}
		}
	}

	return s.commitHook(tx)
}

// GetCommunities returns all communities for a project, including their members.
func (s *Store) GetCommunities(project string, limit int) ([]Community, error) {
	if limit <= 0 {
		limit = 20
	}

	args := []any{}
	where := "1=1"
	if project != "" {
		where = "ifnull(c.project,'') = ?"
		args = append(args, project)
	}
	args = append(args, limit)

	rows, err := s.queryHook(s.db, `
		SELECT c.id, c.summary, c.project, c.updated_at
		FROM communities c
		WHERE `+where+`
		ORDER BY c.updated_at DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("get communities: %w", err)
	}
	defer rows.Close()

	var communities []Community
	for rows.Next() {
		var c Community
		var summary, proj *string
		if err := rows.Scan(&c.ID, &summary, &proj, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Summary = summary
		c.Project = proj
		communities = append(communities, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load members for each community.
	for i := range communities {
		members, err := s.communityMembers(communities[i].ID)
		if err != nil {
			return nil, err
		}
		communities[i].Members = members
	}

	return communities, nil
}

// connectedComponents runs union-find in Go over all entities and active
// relations for a project. Returns a map of component_id → []entity_id.
// Component IDs are the minimum entity ID in each component (arbitrary but stable).
func (s *Store) connectedComponents(project string) (map[int64][]int64, error) {
	// Load entity IDs.
	entityFilter := "1=1"
	eArgs := []any{}
	if project != "" {
		entityFilter = "ifnull(project,'') = ?"
		eArgs = append(eArgs, project)
	}
	eRows, err := s.queryHook(s.db, `SELECT id FROM entities WHERE `+entityFilter, eArgs...)
	if err != nil {
		return nil, err
	}
	defer eRows.Close()

	parent := make(map[int64]int64)
	for eRows.Next() {
		var id int64
		if err := eRows.Scan(&id); err != nil {
			return nil, err
		}
		parent[id] = id
	}
	if err := eRows.Err(); err != nil {
		return nil, err
	}

	// union-find helpers.
	var find func(int64) int64
	find = func(x int64) int64 {
		if parent[x] != x {
			parent[x] = find(parent[x]) // path compression
		}
		return parent[x]
	}
	union := func(a, b int64) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}

	// Load active edges.
	rFilter := "r.t_invalid IS NULL"
	rArgs := []any{}
	if project != "" {
		rFilter += " AND (ifnull(src.project,'') = ? OR ifnull(tgt.project,'') = ?)"
		rArgs = append(rArgs, project, project)
	}
	rRows, err := s.queryHook(s.db, `
		SELECT r.source_id, r.target_id
		FROM relations r
		JOIN entities src ON src.id = r.source_id
		JOIN entities tgt ON tgt.id = r.target_id
		WHERE `+rFilter, rArgs...)
	if err != nil {
		return nil, err
	}
	defer rRows.Close()

	for rRows.Next() {
		var src, tgt int64
		if err := rRows.Scan(&src, &tgt); err != nil {
			return nil, err
		}
		union(src, tgt)
	}
	if err := rRows.Err(); err != nil {
		return nil, err
	}

	// Group entity IDs by their root component.
	components := make(map[int64][]int64)
	for id := range parent {
		root := find(id)
		components[root] = append(components[root], id)
	}
	return components, nil
}

func (s *Store) communityMembers(communityID int64) ([]Entity, error) {
	rows, err := s.queryHook(s.db, `
		SELECT e.id, e.name, e.entity_type, e.summary, e.project, e.created_at, e.updated_at
		FROM entities e
		JOIN community_members cm ON cm.entity_id = e.id
		WHERE cm.community_id = ?
		ORDER BY e.name
	`, communityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Entity
	for rows.Next() {
		e, err := scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *e)
	}
	return members, rows.Err()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

type entityScanner interface {
	Scan(dest ...any) error
}

func scanEntity(row entityScanner) (*Entity, error) {
	var e Entity
	var summary, project *string
	if err := row.Scan(
		&e.ID, &e.Name, &e.EntityType, &summary, &project,
		&e.CreatedAt, &e.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan entity: %w", err)
	}
	e.Summary = summary
	e.Project = project
	return &e, nil
}

type rowsScanner interface {
	Scan(dest ...any) error
}

func scanEntityRow(rows rowsScanner) (*Entity, error) {
	var e Entity
	var summary, project *string
	if err := rows.Scan(
		&e.ID, &e.Name, &e.EntityType, &summary, &project,
		&e.CreatedAt, &e.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan entity row: %w", err)
	}
	e.Summary = summary
	e.Project = project
	return &e, nil
}

func scanRelations(rows rowScanner) ([]Relation, error) {
	var results []Relation
	for rows.Next() {
		var r Relation
		var obsID *int64
		var tInvalid *string
		if err := rows.Scan(
			&r.ID,
			&r.SourceID, &r.SourceName,
			&r.Relation,
			&r.TargetID, &r.TargetName,
			&obsID,
			&r.TValid, &tInvalid,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan relation: %w", err)
		}
		r.ObservationID = obsID
		r.TInvalid = tInvalid
		results = append(results, r)
	}
	return results, rows.Err()
}

