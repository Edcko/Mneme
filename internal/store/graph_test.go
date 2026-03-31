package store_test

import (
	"os"
	"testing"

	"github.com/Edcko/Mneme/internal/store"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newGraphStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(store.Config{
		DataDir:              dir,
		MaxObservationLength: 50000,
		MaxContextResults:    20,
		MaxSearchResults:     20,
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		os.RemoveAll(dir)
	})
	return s
}

// ─── Entity Tests ─────────────────────────────────────────────────────────────

func TestUpsertEntity_CreateNew(t *testing.T) {
	s := newGraphStore(t)

	id, err := s.UpsertEntity("SQLite", store.EntityTypeTool, "embedded SQL database", "mneme")
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestUpsertEntity_Dedup(t *testing.T) {
	s := newGraphStore(t)

	id1, err := s.UpsertEntity("Go", store.EntityTypeLanguage, "compiled language", "mneme")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Same name + project → same ID, updated summary.
	id2, err := s.UpsertEntity("Go", store.EntityTypeLanguage, "fast compiled language by Google", "mneme")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same ID on dedup: got %d and %d", id1, id2)
	}
}

func TestUpsertEntity_CaseInsensitive(t *testing.T) {
	s := newGraphStore(t)

	id1, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")
	id2, _ := s.UpsertEntity("sqlite", store.EntityTypeTool, "", "mneme")
	id3, _ := s.UpsertEntity("SQLITE", store.EntityTypeTool, "", "mneme")

	if id1 != id2 || id2 != id3 {
		t.Errorf("case-insensitive dedup failed: %d %d %d", id1, id2, id3)
	}
}

func TestGetEntityByName(t *testing.T) {
	s := newGraphStore(t)

	s.UpsertEntity("Docker", store.EntityTypeTool, "container runtime", "mneme")

	e, err := s.GetEntityByName("Docker", "mneme")
	if err != nil {
		t.Fatalf("GetEntityByName: %v", err)
	}
	if e.Name != "Docker" {
		t.Errorf("expected name Docker, got %s", e.Name)
	}
	if e.EntityType != store.EntityTypeTool {
		t.Errorf("expected type tool, got %s", e.EntityType)
	}
}

func TestListEntities_TypeFilter(t *testing.T) {
	s := newGraphStore(t)

	s.UpsertEntity("Go", store.EntityTypeLanguage, "", "mneme")
	s.UpsertEntity("Python", store.EntityTypeLanguage, "", "mneme")
	s.UpsertEntity("Docker", store.EntityTypeTool, "", "mneme")

	entities, err := s.ListEntities(string(store.EntityTypeLanguage), "mneme", 10)
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 language entities, got %d", len(entities))
	}
}

func TestSearchEntities_FTS(t *testing.T) {
	s := newGraphStore(t)

	s.UpsertEntity("SQLite", store.EntityTypeTool, "embedded relational database", "mneme")
	s.UpsertEntity("PostgreSQL", store.EntityTypeTool, "advanced open source database", "mneme")
	s.UpsertEntity("Go", store.EntityTypeLanguage, "compiled systems language", "mneme")

	results, err := s.SearchEntities("database", "", "mneme", 10)
	if err != nil {
		t.Fatalf("SearchEntities: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected ≥2 results for 'database', got %d", len(results))
	}
}

// ─── Relation Tests ───────────────────────────────────────────────────────────

func TestAddRelation_Basic(t *testing.T) {
	s := newGraphStore(t)

	srcID, _ := s.UpsertEntity("Mneme", store.EntityTypeProject, "", "mneme")
	tgtID, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")

	relID, err := s.AddRelation(srcID, tgtID, "usa", nil)
	if err != nil {
		t.Fatalf("AddRelation: %v", err)
	}
	if relID <= 0 {
		t.Errorf("expected positive relation ID, got %d", relID)
	}
}

func TestAddRelation_Invalidates_Previous(t *testing.T) {
	s := newGraphStore(t)

	srcID, _ := s.UpsertEntity("Mneme", store.EntityTypeProject, "", "mneme")
	oldTgt, _ := s.UpsertEntity("PostgreSQL", store.EntityTypeTool, "", "mneme")
	newTgt, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")

	// Create initial relation.
	s.AddRelation(srcID, oldTgt, "usa_db", nil)

	// Add conflicting relation — should invalidate the old one.
	s.AddRelation(srcID, newTgt, "usa_db", nil)

	// Old relation should be invalidated.
	oldRels, _ := s.GetRelationHistory(srcID, oldTgt, "usa_db")
	if len(oldRels) == 0 {
		t.Fatal("expected old relation in history")
	}
	if oldRels[0].TInvalid == nil {
		t.Errorf("expected old relation to be invalidated")
	}

	// New relation should be active.
	newRels, _ := s.GetEntityRelations(srcID, true)
	found := false
	for _, r := range newRels {
		if r.TargetID == newTgt && r.Relation == "usa_db" {
			found = true
		}
	}
	if !found {
		t.Errorf("new active relation not found")
	}
}

func TestInvalidateRelation(t *testing.T) {
	s := newGraphStore(t)

	srcID, _ := s.UpsertEntity("App", store.EntityTypeProject, "", "mneme")
	tgtID, _ := s.UpsertEntity("Redis", store.EntityTypeTool, "", "mneme")

	relID, _ := s.AddRelation(srcID, tgtID, "usa", nil)

	// Should be active initially.
	rels, _ := s.GetEntityRelations(srcID, true)
	if len(rels) != 1 {
		t.Fatalf("expected 1 active relation, got %d", len(rels))
	}

	// Invalidate it.
	if err := s.InvalidateRelation(relID); err != nil {
		t.Fatalf("InvalidateRelation: %v", err)
	}

	// Should no longer appear as active.
	active, _ := s.GetEntityRelations(srcID, true)
	if len(active) != 0 {
		t.Errorf("expected 0 active relations after invalidation, got %d", len(active))
	}

	// But should appear in full history.
	all, _ := s.GetEntityRelations(srcID, false)
	if len(all) != 1 {
		t.Errorf("expected 1 historical relation, got %d", len(all))
	}
	if all[0].TInvalid == nil {
		t.Errorf("expected t_invalid to be set")
	}
}

func TestGetRelationHistory_BiTemporal(t *testing.T) {
	s := newGraphStore(t)

	srcID, _ := s.UpsertEntity("Service", store.EntityTypeProject, "", "mneme")
	v1, _ := s.UpsertEntity("v1.0", store.EntityTypeConcept, "", "mneme")
	v2, _ := s.UpsertEntity("v2.0", store.EntityTypeConcept, "", "mneme")
	v3, _ := s.UpsertEntity("v3.0", store.EntityTypeConcept, "", "mneme")

	s.AddRelation(srcID, v1, "en_version", nil)
	s.AddRelation(srcID, v2, "en_version", nil)
	s.AddRelation(srcID, v3, "en_version", nil)

	// v1 and v2 should be in history as invalidated.
	h1, _ := s.GetRelationHistory(srcID, v1, "en_version")
	if len(h1) == 0 || h1[0].TInvalid == nil {
		t.Errorf("v1 relation should be invalidated in history")
	}

	h2, _ := s.GetRelationHistory(srcID, v2, "en_version")
	if len(h2) == 0 || h2[0].TInvalid == nil {
		t.Errorf("v2 relation should be invalidated in history")
	}

	// v3 should be the active one.
	active, _ := s.GetEntityRelations(srcID, true)
	if len(active) != 1 || active[0].TargetID != v3 {
		t.Errorf("v3 should be the only active relation")
	}
}

// ─── Graph BFS Tests ──────────────────────────────────────────────────────────

func TestGraphBFS_SingleHop(t *testing.T) {
	s := newGraphStore(t)

	mneme, _ := s.UpsertEntity("Mneme", store.EntityTypeProject, "", "mneme")
	sqlite, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")
	goLang, _ := s.UpsertEntity("Go", store.EntityTypeLanguage, "", "mneme")

	s.AddRelation(mneme, sqlite, "usa", nil)
	s.AddRelation(mneme, goLang, "escrito_en", nil)

	result, err := s.GraphBFS(mneme, 1, "mneme")
	if err != nil {
		t.Fatalf("GraphBFS: %v", err)
	}

	if result.Seed.ID != mneme {
		t.Errorf("expected seed ID %d, got %d", mneme, result.Seed.ID)
	}
	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 connected nodes at depth 1, got %d", len(result.Nodes))
	}
	if len(result.Relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(result.Relations))
	}
}

func TestGraphBFS_MultiHop(t *testing.T) {
	s := newGraphStore(t)

	// Chain: Mneme → SQLite → FTS5 → Trigonometry(concept)
	mneme, _ := s.UpsertEntity("Mneme", store.EntityTypeProject, "", "mneme")
	sqlite, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")
	fts5, _ := s.UpsertEntity("FTS5", store.EntityTypeTool, "", "mneme")
	trig, _ := s.UpsertEntity("Trigonometry", store.EntityTypeConcept, "", "mneme")

	s.AddRelation(mneme, sqlite, "usa", nil)
	s.AddRelation(sqlite, fts5, "incluye", nil)
	s.AddRelation(fts5, trig, "basado_en", nil)

	// depth=3 should reach Trigonometry.
	result, err := s.GraphBFS(mneme, 3, "mneme")
	if err != nil {
		t.Fatalf("GraphBFS depth=3: %v", err)
	}

	nodeNames := make(map[string]int)
	for _, n := range result.Nodes {
		nodeNames[n.Name] = n.Depth
	}

	if nodeNames["SQLite"] != 1 {
		t.Errorf("SQLite should be at depth 1, got %d", nodeNames["SQLite"])
	}
	if nodeNames["FTS5"] != 2 {
		t.Errorf("FTS5 should be at depth 2, got %d", nodeNames["FTS5"])
	}
	if nodeNames["Trigonometry"] != 3 {
		t.Errorf("Trigonometry should be at depth 3, got %d", nodeNames["Trigonometry"])
	}
}

func TestGraphBFS_NoCycles(t *testing.T) {
	s := newGraphStore(t)

	// Cycle: A → B → C → A
	a, _ := s.UpsertEntity("A", store.EntityTypeConcept, "", "mneme")
	b, _ := s.UpsertEntity("B", store.EntityTypeConcept, "", "mneme")
	c, _ := s.UpsertEntity("C", store.EntityTypeConcept, "", "mneme")

	s.AddRelation(a, b, "conecta", nil)
	s.AddRelation(b, c, "conecta", nil)
	s.AddRelation(c, a, "conecta", nil)

	// Should not infinite loop — cycle detection must kick in.
	result, err := s.GraphBFS(a, 5, "mneme")
	if err != nil {
		t.Fatalf("GraphBFS with cycle: %v", err)
	}
	// Should find B and C without duplicates.
	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 unique nodes (B, C), got %d", len(result.Nodes))
	}
}

func TestGraphBFS_InactiveRelationsIgnored(t *testing.T) {
	s := newGraphStore(t)

	src, _ := s.UpsertEntity("Service", store.EntityTypeProject, "", "mneme")
	old, _ := s.UpsertEntity("OldDep", store.EntityTypeTool, "", "mneme")
	current, _ := s.UpsertEntity("NewDep", store.EntityTypeTool, "", "mneme")

	relID, _ := s.AddRelation(src, old, "usa", nil)
	s.InvalidateRelation(relID) // supersede it
	s.AddRelation(src, current, "usa", nil)

	result, err := s.GraphBFS(src, 1, "mneme")
	if err != nil {
		t.Fatalf("GraphBFS: %v", err)
	}

	// Only NewDep should appear — OldDep's relation is inactive.
	if len(result.Nodes) != 1 {
		t.Errorf("expected 1 active node, got %d", len(result.Nodes))
	}
	if result.Nodes[0].Name != "NewDep" {
		t.Errorf("expected NewDep, got %s", result.Nodes[0].Name)
	}
}

// ─── Community Detection Tests ────────────────────────────────────────────────

func TestRebuildCommunities(t *testing.T) {
	s := newGraphStore(t)

	// Cluster 1: Mneme ↔ SQLite ↔ Go
	mneme, _ := s.UpsertEntity("Mneme", store.EntityTypeProject, "", "mneme")
	sqlite, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")
	goLang, _ := s.UpsertEntity("Go", store.EntityTypeLanguage, "", "mneme")

	s.AddRelation(mneme, sqlite, "usa", nil)
	s.AddRelation(mneme, goLang, "escrito_en", nil)

	// Cluster 2: Frontend ↔ React (isolated from cluster 1)
	fe, _ := s.UpsertEntity("Frontend", store.EntityTypeProject, "", "mneme")
	react, _ := s.UpsertEntity("React", store.EntityTypeTool, "", "mneme")
	s.AddRelation(fe, react, "usa", nil)

	if err := s.RebuildCommunities("mneme"); err != nil {
		t.Fatalf("RebuildCommunities: %v", err)
	}

	communities, err := s.GetCommunities("mneme", 10)
	if err != nil {
		t.Fatalf("GetCommunities: %v", err)
	}

	if len(communities) < 2 {
		t.Errorf("expected ≥2 communities, got %d", len(communities))
	}

	// Verify each community has members.
	for _, c := range communities {
		if len(c.Members) == 0 {
			t.Errorf("community %d has no members", c.ID)
		}
	}
}
