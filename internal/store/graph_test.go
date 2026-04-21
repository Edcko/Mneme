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

	// Same name + type + project → same ID, updated summary.
	id2, err := s.UpsertEntity("Go", store.EntityTypeLanguage, "fast compiled language by Google", "mneme")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same ID on dedup: got %d and %d", id1, id2)
	}
}

func TestUpsertEntity_DifferentTypesDontCollide(t *testing.T) {
	s := newGraphStore(t)

	// "Go" as language
	id1, err := s.UpsertEntity("Go", store.EntityTypeLanguage, "compiled language", "mneme")
	if err != nil {
		t.Fatalf("UpsertEntity language: %v", err)
	}

	// Same name, different type — must NOT dedup.
	id2, err := s.UpsertEntity("Go", store.EntityTypeTool, "board game tool", "mneme")
	if err != nil {
		t.Fatalf("UpsertEntity tool: %v", err)
	}

	if id1 == id2 {
		t.Errorf("different entity_types must get different IDs, got same %d", id1)
	}

	// Both must exist.
	entities, err := s.ListEntities("", "mneme", 10)
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities (different types), got %d", len(entities))
	}
}

func TestUpsertEntity_SemanticMatch(t *testing.T) {
	s := newGraphStore(t)

	// Insert "React" as tool.
	id1, err := s.UpsertEntity("React", store.EntityTypeTool, "UI library", "mneme")
	if err != nil {
		t.Fatalf("UpsertEntity React: %v", err)
	}

	// "React.js" should semantically match "React" (same type, same project).
	id2, err := s.UpsertEntity("React.js", store.EntityTypeTool, "UI framework", "mneme")
	if err != nil {
		t.Fatalf("UpsertEntity React.js: %v", err)
	}

	if id1 != id2 {
		t.Errorf("semantic match should merge React.js with React: got %d and %d", id1, id2)
	}

	// Summary must be updated.
	e, err := s.GetEntityByID(id1)
	if err != nil {
		t.Fatalf("GetEntityByID: %v", err)
	}
	if e.Summary == nil || *e.Summary != "UI framework" {
		t.Errorf("expected summary updated to 'UI framework', got %v", e.Summary)
	}
}

func TestUpsertEntity_SemanticMatch_NoCrossType(t *testing.T) {
	s := newGraphStore(t)

	// "React" as tool.
	toolID, _ := s.UpsertEntity("React", store.EntityTypeTool, "UI library", "mneme")

	// "React" as concept — must NOT match the tool entity via semantic dedup.
	conceptID, _ := s.UpsertEntity("React", store.EntityTypeConcept, "UI pattern", "mneme")

	if toolID == conceptID {
		t.Errorf("semantic match must not cross entity_types: got same ID %d", toolID)
	}
}

func TestUpsertEntity_SemanticMatch_BelowThreshold(t *testing.T) {
	s := newGraphStore(t)

	// "Redis" as tool.
	id1, _ := s.UpsertEntity("Redis", store.EntityTypeTool, "in-memory store", "mneme")

	// "React" is not similar enough to "Redis" → must create separate entity.
	id2, _ := s.UpsertEntity("React", store.EntityTypeTool, "UI framework", "mneme")

	if id1 == id2 {
		t.Errorf("dissimilar names must not merge: got same ID %d", id1)
	}
}

func TestSemanticDedup_RelationsPointCorrectly(t *testing.T) {
	s := newGraphStore(t)

	// Pre-create "React" and "MyApp".
	reactID, _ := s.UpsertEntity("React", store.EntityTypeTool, "UI library", "mneme")
	appID, _ := s.UpsertEntity("MyApp", store.EntityTypeProject, "web app", "mneme")

	// "React.js" should resolve to the existing "React" entity via semantic match.
	semanticID, err := s.UpsertEntity("React.js", store.EntityTypeTool, "frontend framework", "mneme")
	if err != nil {
		t.Fatalf("UpsertEntity React.js: %v", err)
	}
	if semanticID != reactID {
		t.Fatalf("React.js should resolve to React ID %d, got %d", reactID, semanticID)
	}

	// Add relation: MyApp usa React.
	_, err = s.AddRelation(appID, semanticID, "usa", nil)
	if err != nil {
		t.Fatalf("AddRelation: %v", err)
	}

	// Verify relation target is the React entity (not a new one).
	rels, err := s.GetEntityRelations(appID, true)
	if err != nil {
		t.Fatalf("GetEntityRelations: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(rels))
	}
	if rels[0].TargetID != reactID {
		t.Errorf("relation target should be React ID %d, got %d", reactID, rels[0].TargetID)
	}
	if rels[0].TargetName != "React" {
		t.Errorf("relation target name should be 'React', got %s", rels[0].TargetName)
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

// ─── Count and Reindex Helpers ─────────────────────────────────────────────────

func TestCountEntities(t *testing.T) {
	s := newGraphStore(t)

	// Initially zero
	count, err := s.CountEntities("")
	if err != nil {
		t.Fatalf("CountEntities: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 entities, got %d", count)
	}

	s.UpsertEntity("Go", store.EntityTypeLanguage, "backend lang", "proj-a")
	s.UpsertEntity("React", store.EntityTypeConcept, "ui library", "proj-a")
	s.UpsertEntity("SQLite", store.EntityTypeTool, "embedded db", "proj-b")

	// Count all
	count, err = s.CountEntities("")
	if err != nil {
		t.Fatalf("CountEntities: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 entities, got %d", count)
	}

	// Count by project
	countProj, err := s.CountEntities("proj-a")
	if err != nil {
		t.Fatalf("CountEntities proj-a: %v", err)
	}
	if countProj != 2 {
		t.Fatalf("expected 2 entities for proj-a, got %d", countProj)
	}
}

func TestCountRelations(t *testing.T) {
	s := newGraphStore(t)

	count, err := s.CountRelations()
	if err != nil {
		t.Fatalf("CountRelations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 relations, got %d", count)
	}

	srcID, _ := s.UpsertEntity("Go", store.EntityTypeLanguage, "", "p")
	tgtID, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "p")
	s.AddRelation(srcID, tgtID, "usa", nil)

	count, err = s.CountRelations()
	if err != nil {
		t.Fatalf("CountRelations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 relation, got %d", count)
	}
}

func TestListObservationsForReindex(t *testing.T) {
	s := newGraphStore(t)

	// Seed observations
	s.CreateSession("s-ri", "ri-proj", "/tmp")
	s.AddObservation(store.AddObservationParams{
		SessionID: "s-ri", Type: "note", Title: "first",
		Content: "first content about Go", Project: "ri-proj", Scope: "project",
	})
	s.AddObservation(store.AddObservationParams{
		SessionID: "s-ri", Type: "note", Title: "second",
		Content: "second content about React", Project: "ri-proj", Scope: "project",
	})
	s.CreateSession("s-ri2", "other-proj", "/tmp")
	s.AddObservation(store.AddObservationParams{
		SessionID: "s-ri2", Type: "note", Title: "third",
		Content: "third content about Python", Project: "other-proj", Scope: "project",
	})

	// List all — paginated
	all, err := s.ListObservationsForReindex("", 0, 10)
	if err != nil {
		t.Fatalf("ListObservationsForReindex: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 observations, got %d", len(all))
	}
	// Should be ordered by ID ASC
	if all[0].Title != "first" {
		t.Fatalf("expected first observation ordered by ID, got %q", all[0].Title)
	}

	// Filter by project
	projOnly, err := s.ListObservationsForReindex("ri-proj", 0, 10)
	if err != nil {
		t.Fatalf("ListObservationsForReindex filtered: %v", err)
	}
	if len(projOnly) != 2 {
		t.Fatalf("expected 2 observations for ri-proj, got %d", len(projOnly))
	}

	// Pagination
	page1, err := s.ListObservationsForReindex("", 0, 2)
	if err != nil {
		t.Fatalf("ListObservationsForReindex page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 on page1, got %d", len(page1))
	}
	page2, err := s.ListObservationsForReindex("", 2, 2)
	if err != nil {
		t.Fatalf("ListObservationsForReindex page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 on page2, got %d", len(page2))
	}
}

// ─── Noise Cleanup Tests ─────────────────────────────────────────────────────

func TestCleanupNoiseConcepts_Basic(t *testing.T) {
	s := newGraphStore(t)

	// Seed: noise concepts + valid concepts + non-concept entities.
	s.UpsertEntity("the", store.EntityTypeConcept, "", "mneme")    // stopword → noise
	s.UpsertEntity("and", store.EntityTypeConcept, "", "mneme")    // stopword → noise
	s.UpsertEntity("result", store.EntityTypeConcept, "", "mneme") // generic → noise
	s.UpsertEntity("run", store.EntityTypeConcept, "", "mneme")    // generic → noise
	s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")    // tool → keep
	s.UpsertEntity("Go", store.EntityTypeLanguage, "", "mneme")    // language → keep
	s.UpsertEntity("CQRS", store.EntityTypeConcept, "", "mneme")   // gazetteer concept → keep

	result, err := s.CleanupNoiseConcepts("")
	if err != nil {
		t.Fatalf("CleanupNoiseConcepts: %v", err)
	}

	if result.EntitiesDeleted != 4 {
		t.Errorf("expected 4 entities deleted (the, and, result, run), got %d", result.EntitiesDeleted)
	}

	// Verify remaining entities are the valid ones.
	entities, _ := s.ListEntities("", "mneme", 20)
	if len(entities) != 3 {
		t.Errorf("expected 3 remaining entities (SQLite, Go, CQRS), got %d", len(entities))
		for _, e := range entities {
			t.Logf("  remaining: %s (%s)", e.Name, e.EntityType)
		}
	}
}

func TestCleanupNoiseConcepts_WithRelations(t *testing.T) {
	s := newGraphStore(t)

	// Mneme → usa → the (noise target)
	// Mneme → usa → SQLite (valid target)
	mneme, _ := s.UpsertEntity("Mneme", store.EntityTypeProject, "", "mneme")
	theID, _ := s.UpsertEntity("the", store.EntityTypeConcept, "", "mneme")
	sqliteID, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")

	s.AddRelation(mneme, theID, "usa", nil)
	s.AddRelation(mneme, sqliteID, "usa", nil)

	result, err := s.CleanupNoiseConcepts("")
	if err != nil {
		t.Fatalf("CleanupNoiseConcepts: %v", err)
	}

	if result.EntitiesDeleted != 1 {
		t.Errorf("expected 1 entity deleted, got %d", result.EntitiesDeleted)
	}
	if result.RelationsDeleted != 1 {
		t.Errorf("expected 1 relation deleted (Mneme→the), got %d", result.RelationsDeleted)
	}

	// Mneme→SQLite relation must survive.
	rels, _ := s.GetEntityRelations(mneme, true)
	if len(rels) != 1 || rels[0].TargetID != sqliteID {
		t.Errorf("expected Mneme→SQLite relation to survive, got %d relations", len(rels))
	}
}

func TestCleanupNoiseConcepts_ProjectFilter(t *testing.T) {
	s := newGraphStore(t)

	// "the" in project A → should be deleted
	s.UpsertEntity("the", store.EntityTypeConcept, "", "proj-a")
	// "the" in project B → should survive when filtering by proj-a
	s.UpsertEntity("the", store.EntityTypeConcept, "", "proj-b")

	result, err := s.CleanupNoiseConcepts("proj-a")
	if err != nil {
		t.Fatalf("CleanupNoiseConcepts: %v", err)
	}

	if result.EntitiesDeleted != 1 {
		t.Errorf("expected 1 entity deleted (proj-a only), got %d", result.EntitiesDeleted)
	}

	// proj-b "the" must still exist.
	entities, _ := s.ListEntities(string(store.EntityTypeConcept), "proj-b", 10)
	if len(entities) != 1 {
		t.Errorf("expected proj-b 'the' to survive, got %d entities", len(entities))
	}
}

func TestCleanupNoiseConcepts_NoNoise(t *testing.T) {
	s := newGraphStore(t)

	s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")
	s.UpsertEntity("Go", store.EntityTypeLanguage, "", "mneme")

	result, err := s.CleanupNoiseConcepts("")
	if err != nil {
		t.Fatalf("CleanupNoiseConcepts: %v", err)
	}

	if result.EntitiesDeleted != 0 {
		t.Errorf("expected 0 entities deleted, got %d", result.EntitiesDeleted)
	}
	if result.RelationsDeleted != 0 {
		t.Errorf("expected 0 relations deleted, got %d", result.RelationsDeleted)
	}
}

func TestCleanupNoiseConcepts_OnlyConceptsAffected(t *testing.T) {
	s := newGraphStore(t)

	// "the" as a tool entity (NOT concept) → should survive.
	s.UpsertEntity("the", store.EntityTypeTool, "", "mneme")

	result, err := s.CleanupNoiseConcepts("")
	if err != nil {
		t.Fatalf("CleanupNoiseConcepts: %v", err)
	}

	if result.EntitiesDeleted != 0 {
		t.Errorf("expected 0 entities deleted (tool 'the' is not a concept), got %d", result.EntitiesDeleted)
	}
}

func TestCleanupNoiseConcepts_CommunityMembersCleaned(t *testing.T) {
	s := newGraphStore(t)

	// Create a cluster where one entity is noise.
	mneme, _ := s.UpsertEntity("Mneme", store.EntityTypeProject, "", "mneme")
	theID, _ := s.UpsertEntity("the", store.EntityTypeConcept, "", "mneme")
	sqliteID, _ := s.UpsertEntity("SQLite", store.EntityTypeTool, "", "mneme")

	s.AddRelation(mneme, theID, "usa", nil)
	s.AddRelation(mneme, sqliteID, "usa", nil)

	// Build communities first.
	s.RebuildCommunities("mneme")

	// Cleanup noise.
	result, err := s.CleanupNoiseConcepts("")
	if err != nil {
		t.Fatalf("CleanupNoiseConcepts: %v", err)
	}

	if result.EntitiesDeleted != 1 {
		t.Errorf("expected 1 entity deleted, got %d", result.EntitiesDeleted)
	}

	// Communities still exist (Mneme + SQLite still connected).
	communities, _ := s.GetCommunities("mneme", 10)
	if len(communities) == 0 {
		t.Error("expected communities to survive after cleanup")
	}
}
