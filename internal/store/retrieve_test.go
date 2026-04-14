package store

import (
	"strings"
	"testing"
)

// ─── Composite Retrieve Tests ─────────────────────────────────────────────────

func TestCompositeRetrieve_ReturnsObservationsAndEntities(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Save an observation about auth middleware.
	_, err := s.AddObservation(AddObservationParams{
		SessionID: "s1",
		Type:      "decision",
		Title:     "Auth middleware architecture",
		Content:   "Decided to use JWT middleware for auth validation in the API gateway",
		Project:   "engram",
		Scope:     "project",
	})
	if err != nil {
		t.Fatalf("add observation: %v", err)
	}

	// Create a graph entity related to the same domain.
	// Use summary text that will match the FTS query.
	s.UpsertEntity("JWT", EntityTypeTool, "token library for auth middleware", "engram")

	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "JWT auth",
		Project: "engram",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	// Should find at least one observation.
	if len(result.Observations) == 0 {
		t.Fatalf("expected observations, got 0")
	}
	if !strings.Contains(result.Observations[0].Title, "Auth middleware") {
		t.Fatalf("expected auth middleware observation, got %q", result.Observations[0].Title)
	}

	// Should find graph entities (JWT entity has "JWT" in name and "auth" in summary).
	if len(result.Entities) == 0 {
		t.Fatalf("expected entities, got 0")
	}

	entityNames := make(map[string]bool)
	for _, e := range result.Entities {
		entityNames[e.Entity.Name] = true
	}
	if !entityNames["JWT"] {
		t.Fatalf("expected JWT entity in results, got entities: %v", entityNames)
	}
}

func TestCompositeRetrieve_EnrichesEntitiesWithRelations(t *testing.T) {
	s := newTestStore(t)

	// Create entities and a relation between them.
	appID, _ := s.UpsertEntity("MyApp", EntityTypeProject, "web application", "engram")
	sqliteID, _ := s.UpsertEntity("SQLite", EntityTypeTool, "embedded database", "engram")

	_, err := s.AddRelation(appID, sqliteID, "usa", nil)
	if err != nil {
		t.Fatalf("AddRelation: %v", err)
	}

	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "MyApp",
		Project: "engram",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	// Find the MyApp entity in results.
	var appEntity *EntityResult
	for i := range result.Entities {
		if result.Entities[i].Entity.Name == "MyApp" {
			appEntity = &result.Entities[i]
			break
		}
	}
	if appEntity == nil {
		t.Fatalf("expected MyApp in entity results")
	}

	// Should have active relations loaded.
	if len(appEntity.Relations) == 0 {
		t.Fatalf("expected MyApp to have relations, got 0")
	}

	foundSQLite := false
	for _, r := range appEntity.Relations {
		if r.TargetName == "SQLite" && r.Relation == "usa" {
			foundSQLite = true
		}
	}
	if !foundSQLite {
		t.Fatalf("expected MyApp -[usa]-> SQLite relation")
	}

	// Graph edges should also be populated.
	if len(result.GraphEdges) == 0 {
		t.Fatalf("expected graph edges, got 0")
	}
	foundEdge := false
	for _, e := range result.GraphEdges {
		if e.SourceName == "MyApp" && e.TargetName == "SQLite" && e.Relation == "usa" {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected MyApp -> SQLite edge in graph edges")
	}
}

func TestCompositeRetrieve_EmptyResult(t *testing.T) {
	s := newTestStore(t)

	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "absolutely nothing matches this query xyz123",
		Project: "nonexistent-project",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	if len(result.Observations) != 0 {
		t.Fatalf("expected 0 observations, got %d", len(result.Observations))
	}
	if len(result.Entities) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(result.Entities))
	}
	if result.Total != 0 {
		t.Fatalf("expected total=0, got %d", result.Total)
	}

	// Formatting should produce a "no context" message.
	text := FormatRetrieveResult(result)
	if !strings.Contains(text, "No context found") {
		t.Fatalf("expected empty result message, got %q", text)
	}
}

func TestCompositeRetrieve_ScopeFilter(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Save project-scoped observation.
	_, err := s.AddObservation(AddObservationParams{
		SessionID: "s1",
		Type:      "decision",
		Title:     "Project auth",
		Content:   "Keep auth middleware in project memory scope test",
		Project:   "engram",
		Scope:     "project",
	})
	if err != nil {
		t.Fatalf("add project observation: %v", err)
	}

	// Save personal-scoped observation.
	_, err = s.AddObservation(AddObservationParams{
		SessionID: "s1",
		Type:      "decision",
		Title:     "Personal auth note",
		Content:   "Personal regex trick for auth validation scope test",
		Project:   "engram",
		Scope:     "personal",
	})
	if err != nil {
		t.Fatalf("add personal observation: %v", err)
	}

	// Search with project scope.
	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "auth scope test",
		Project: "engram",
		Scope:   "project",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	for _, obs := range result.Observations {
		if obs.Scope != "project" {
			t.Fatalf("expected only project-scope observations, got scope=%q for %q", obs.Scope, obs.Title)
		}
	}
}

func TestCompositeRetrieve_LimitDefaultAndClamp(t *testing.T) {
	s := newTestStore(t)

	// Zero limit → default to 5.
	result, err := s.CompositeRetrieve(RetrieveParams{
		Query: "test",
		Limit: 0,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve with limit=0: %v", err)
	}
	// Should not panic, result is just empty.

	// Over-limit → clamp to 20.
	result, err = s.CompositeRetrieve(RetrieveParams{
		Query: "test",
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve with limit=100: %v", err)
	}
	_ = result // just verify no panic
}

func TestCompositeRetrieve_ObservationFailureDoesNotBlockGraph(t *testing.T) {
	s := newTestStore(t)

	// Create graph entities but no observations.
	s.UpsertEntity("Docker", EntityTypeTool, "container runtime", "engram")

	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "Docker container",
		Project: "engram",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	// No observations but should still have entities.
	if len(result.Observations) != 0 {
		t.Fatalf("expected 0 observations, got %d", len(result.Observations))
	}
	if len(result.Entities) == 0 {
		t.Fatalf("expected entities from graph, got 0")
	}
}

func TestCompositeRetrieve_GraphFailureDoesNotBlockObservations(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, err := s.AddObservation(AddObservationParams{
		SessionID: "s1",
		Type:      "decision",
		Title:     "Important decision",
		Content:   "Use Docker for containerization deployment strategy",
		Project:   "engram",
		Scope:     "project",
	})
	if err != nil {
		t.Fatalf("add observation: %v", err)
	}

	// No graph entities exist, but observations should still return.
	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "Docker deployment",
		Project: "engram",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	if len(result.Observations) == 0 {
		t.Fatalf("expected observations, got 0")
	}
	if len(result.Entities) != 0 {
		t.Fatalf("expected 0 entities (no graph data), got %d", len(result.Entities))
	}
}

func TestFormatRetrieveResult_WithAllSections(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Observation about SQLite FTS.
	_, err := s.AddObservation(AddObservationParams{
		SessionID: "s1",
		Type:      "decision",
		Title:     "Use SQLite for FTS",
		Content:   "SQLite FTS5 provides excellent full-text search for memory",
		Project:   "engram",
		Scope:     "project",
	})
	if err != nil {
		t.Fatalf("add observation: %v", err)
	}

	// Graph entity + relation — use summary text containing "SQLite" for FTS match.
	sqliteID, _ := s.UpsertEntity("SQLite", EntityTypeTool, "embedded database with SQLite FTS", "engram")
	fts5ID, _ := s.UpsertEntity("FTS5", EntityTypeTool, "SQLite full-text search extension", "engram")
	s.AddRelation(sqliteID, fts5ID, "incluye", nil)

	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "SQLite FTS",
		Project: "engram",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	text := FormatRetrieveResult(result)

	// Should have all three sections.
	if !strings.Contains(text, "### Relevant Memories") {
		t.Fatalf("expected Memories section in formatted output")
	}
	if !strings.Contains(text, "### Related Entities") {
		t.Fatalf("expected Entities section in formatted output, got:\n%s", text)
	}
	if !strings.Contains(text, "### Knowledge Graph Edges") {
		t.Fatalf("expected Graph Edges section in formatted output")
	}

	// Should include the relation edge.
	if !strings.Contains(text, "SQLite") || !strings.Contains(text, "FTS5") {
		t.Fatalf("expected SQLite and FTS5 in formatted output")
	}

	// Should have totals.
	if !strings.Contains(text, "Total:") {
		t.Fatalf("expected Total line in formatted output")
	}
}

func TestCompositeRetrieve_EntityDeduplication(t *testing.T) {
	s := newTestStore(t)

	// Single entity.
	id, _ := s.UpsertEntity("React", EntityTypeTool, "UI library", "engram")

	// Create a relation so it connects to something.
	otherID, _ := s.UpsertEntity("Redux", EntityTypeTool, "state management", "engram")
	s.AddRelation(id, otherID, "usa", nil)

	result, err := s.CompositeRetrieve(RetrieveParams{
		Query:   "React Redux state",
		Project: "engram",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("CompositeRetrieve: %v", err)
	}

	// Count occurrences of each entity ID — should not have duplicates.
	seenIDs := make(map[int64]int)
	for _, er := range result.Entities {
		seenIDs[er.Entity.ID]++
	}
	for id, count := range seenIDs {
		if count > 1 {
			t.Fatalf("entity %d appears %d times in results — expected dedup", id, count)
		}
	}
}
