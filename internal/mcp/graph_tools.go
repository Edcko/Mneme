// graph_tools.go — Knowledge graph MCP tools for Mneme.
//
// Six new tools that expose the graph layer to AI agents:
//
//	mem_graph_search          — BFS traversal from a named entity
//	mem_entities              — list/filter known entities
//	mem_relations             — relations for a specific entity
//	mem_relation_history      — bi-temporal history of a relation
//	mem_invalidate            — mark a relation as no longer valid
//	mem_rebuild_communities   — recompute connected components via union-find
//
// All existing 15 tools are unchanged.

package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/Edcko/Mneme/internal/extractor"
	"github.com/Edcko/Mneme/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// registerGraphTools adds the six graph tools to the MCP server.
// Called from registerTools() in mcp.go — all six are in the "graph" profile.
func registerGraphTools(srv *mcpserver.MCPServer, s *store.Store, allowlist map[string]bool) {
	// ─── mem_graph_search ────────────────────────────────────────────────
	if shouldRegister("mem_graph_search", allowlist) {
		srv.AddTool(
			mcp.NewTool("mem_graph_search",
				mcp.WithTitleAnnotation("Graph Search"),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithDescription(
					"Explore the knowledge graph starting from a named entity. "+
						"Returns related entities and active relations up to N hops away. "+
						"Use this to discover how concepts, tools, and files are connected.",
				),
				mcp.WithString("entity",
					mcp.Required(),
					mcp.Description("Name of the seed entity (e.g. 'SQLite', 'store.go', 'auth middleware')"),
				),
				mcp.WithString("project",
					mcp.Description("Filter traversal to a specific project"),
				),
				mcp.WithNumber("depth",
					mcp.Description("Max hops from the seed entity (default: 3, max: 10)"),
				),
			),
			handleGraphSearch(s),
		)
	}

	// ─── mem_entities ────────────────────────────────────────────────────
	if shouldRegister("mem_entities", allowlist) {
		srv.AddTool(
			mcp.NewTool("mem_entities",
				mcp.WithTitleAnnotation("List Entities"),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithDescription(
					"List known entities in the knowledge graph. "+
						"Optionally filter by entity type (tool, file, language, project, person, concept) "+
						"and/or project. Supports free-text search.",
				),
				mcp.WithString("query",
					mcp.Description("Free-text search across entity names and summaries"),
				),
				mcp.WithString("type",
					mcp.Description("Entity type filter: tool, file, language, project, person, concept"),
				),
				mcp.WithString("project",
					mcp.Description("Filter by project"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Max results (default: 20)"),
				),
			),
			handleEntities(s),
		)
	}

	// ─── mem_relations ───────────────────────────────────────────────────
	if shouldRegister("mem_relations", allowlist) {
		srv.AddTool(
			mcp.NewTool("mem_relations",
				mcp.WithTitleAnnotation("Entity Relations"),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithDescription(
					"Show all relations for a named entity — both as source and target. "+
						"By default returns only currently active relations (t_invalid IS NULL). "+
						"Set include_history=true to see superseded relations too.",
				),
				mcp.WithString("entity",
					mcp.Required(),
					mcp.Description("Entity name to inspect"),
				),
				mcp.WithString("project",
					mcp.Description("Project scope for entity lookup"),
				),
				mcp.WithBoolean("include_history",
					mcp.Description("Include invalidated (historical) relations (default: false)"),
				),
			),
			handleRelations(s),
		)
	}

	// ─── mem_relation_history ────────────────────────────────────────────
	if shouldRegister("mem_relation_history", allowlist) {
		srv.AddTool(
			mcp.NewTool("mem_relation_history",
				mcp.WithTitleAnnotation("Relation History"),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithDescription(
					"Bi-temporal history of a specific relation between two entities. "+
						"Shows when each fact became true (t_valid) and when it was superseded (t_invalid). "+
						"Use this to understand how a relationship evolved over time.",
				),
				mcp.WithString("source",
					mcp.Required(),
					mcp.Description("Source entity name"),
				),
				mcp.WithString("relation",
					mcp.Required(),
					mcp.Description("Relation type (e.g. 'depende_de', 'usa', 'reemplaza_a')"),
				),
				mcp.WithString("target",
					mcp.Required(),
					mcp.Description("Target entity name"),
				),
				mcp.WithString("project",
					mcp.Description("Project scope for entity lookup"),
				),
			),
			handleRelationHistory(s),
		)
	}

	// ─── mem_invalidate ──────────────────────────────────────────────────
	if shouldRegister("mem_invalidate", allowlist) {
		srv.AddTool(
			mcp.NewTool("mem_invalidate",
				mcp.WithTitleAnnotation("Invalidate Relation"),
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithDescription(
					"Mark a relation as no longer valid (sets t_invalid = now). "+
						"The relation is preserved for historical queries — it is NOT deleted. "+
						"Use this when a fact has changed: e.g. 'project now uses PostgreSQL instead of SQLite'.",
				),
				mcp.WithNumber("relation_id",
					mcp.Required(),
					mcp.Description("ID of the relation to invalidate (from mem_relations output)"),
				),
			),
			handleInvalidate(s),
		)
	}

	// ─── mem_rebuild_communities ────────────────────────────────────────
	if shouldRegister("mem_rebuild_communities", allowlist) {
		srv.AddTool(
			mcp.NewTool("mem_rebuild_communities",
				mcp.WithTitleAnnotation("Rebuild Communities"),
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithDescription(
					"Recompute community groupings using union-find connected components. "+
						"Communities are clusters of entities connected by active relations. "+
						"Run this periodically after significant graph changes. "+
						"Only communities with 2+ members are stored.",
				),
				mcp.WithString("project",
					mcp.Required(),
					mcp.Description("Project scope for community rebuild"),
				),
			),
			handleRebuildCommunities(s),
		)
	}
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func handleGraphSearch(s *store.Store) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		entityName := strArgGraph(req, "entity", "")
		if entityName == "" {
			return mcp.NewToolResultError("entity name is required"), nil
		}

		project := strArgGraph(req, "project", "")
		depth := intArgGraph(req, "depth", 3)
		if depth > 10 {
			depth = 10
		}

		// Resolve entity by name.
		entity, err := s.GetEntityByName(entityName, project)
		if err != nil {
			// Try FTS search as fallback.
			candidates, searchErr := s.SearchEntities(entityName, "", project, 1)
			if searchErr != nil || len(candidates) == 0 {
				return mcp.NewToolResultError(fmt.Sprintf("entity %q not found in project %q", entityName, project)), nil
			}
			entity = &candidates[0]
		}

		result, err := s.GraphBFS(entity.ID, depth, project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("graph traversal failed: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Graph: %s (%s)\n\n", result.Seed.Name, result.Seed.EntityType))

		if len(result.Nodes) == 0 {
			sb.WriteString("No connected entities found within ")
			sb.WriteString(fmt.Sprintf("%d hops.\n", depth))
		} else {
			sb.WriteString(fmt.Sprintf("**%d connected entities** (max depth: %d):\n\n", len(result.Nodes), depth))
			for _, n := range result.Nodes {
				summary := ""
				if n.Summary != nil {
					summary = " — " + *n.Summary
				}
				sb.WriteString(fmt.Sprintf("  [depth %d] %s (%s)%s\n", n.Depth, n.Name, n.EntityType, summary))
			}
		}

		if len(result.Relations) > 0 {
			sb.WriteString(fmt.Sprintf("\n**%d active relations**:\n\n", len(result.Relations)))
			for _, r := range result.Relations {
				sb.WriteString(fmt.Sprintf("  ID:%d  %s -[%s]-> %s  (since %s)\n",
					r.ID, r.SourceName, r.Relation, r.TargetName, r.TValid))
			}
		}

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func handleEntities(s *store.Store) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := strArgGraph(req, "query", "")
		entityType := strArgGraph(req, "type", "")
		project := strArgGraph(req, "project", "")
		limit := intArgGraph(req, "limit", 20)

		var (
			entities []store.Entity
			err      error
		)

		if query != "" {
			entities, err = s.SearchEntities(query, entityType, project, limit)
		} else {
			entities, err = s.ListEntities(entityType, project, limit)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list entities failed: %v", err)), nil
		}

		if len(entities) == 0 {
			return mcp.NewToolResultText("No entities found."), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Entities (%d)\n\n", len(entities)))
		for _, e := range entities {
			proj := ""
			if e.Project != nil {
				proj = " [" + *e.Project + "]"
			}
			summary := ""
			if e.Summary != nil {
				summary = "\n    " + *e.Summary
			}
			sb.WriteString(fmt.Sprintf("  ID:%d  %s (%s)%s%s\n", e.ID, e.Name, e.EntityType, proj, summary))
		}

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func handleRelations(s *store.Store) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		entityName := strArgGraph(req, "entity", "")
		if entityName == "" {
			return mcp.NewToolResultError("entity name is required"), nil
		}

		project := strArgGraph(req, "project", "")
		includeHistory := boolArgGraph(req, "include_history", false)

		entity, err := s.GetEntityByName(entityName, project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("entity %q not found", entityName)), nil
		}

		relations, err := s.GetEntityRelations(entity.ID, !includeHistory)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get relations failed: %v", err)), nil
		}

		if len(relations) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No relations found for %q.", entityName)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Relations for: %s (%s)\n\n", entity.Name, entity.EntityType))

		active := 0
		for _, r := range relations {
			if r.TInvalid == nil {
				active++
			}
		}
		sb.WriteString(fmt.Sprintf("Active: %d | Total: %d\n\n", active, len(relations)))

		for _, r := range relations {
			status := "active"
			invalidAt := ""
			if r.TInvalid != nil {
				status = "superseded"
				invalidAt = " → " + *r.TInvalid
			}
			sb.WriteString(fmt.Sprintf(
				"  ID:%-4d  %s -[%s]-> %s  [%s]  valid: %s%s\n",
				r.ID, r.SourceName, r.Relation, r.TargetName,
				status, r.TValid, invalidAt,
			))
		}

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func handleRelationHistory(s *store.Store) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sourceName := strArgGraph(req, "source", "")
		targetName := strArgGraph(req, "target", "")
		relation := strArgGraph(req, "relation", "")
		project := strArgGraph(req, "project", "")

		if sourceName == "" || targetName == "" || relation == "" {
			return mcp.NewToolResultError("source, relation, and target are all required"), nil
		}

		sourceEntity, err := s.GetEntityByName(sourceName, project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("source entity %q not found", sourceName)), nil
		}
		targetEntity, err := s.GetEntityByName(targetName, project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("target entity %q not found", targetName)), nil
		}

		history, err := s.GetRelationHistory(sourceEntity.ID, targetEntity.ID, relation)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get relation history failed: %v", err)), nil
		}

		if len(history) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf(
				"No history found for: %s -[%s]-> %s", sourceName, relation, targetName,
			)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Relation History: %s -[%s]-> %s\n\n",
			sourceName, relation, targetName))
		sb.WriteString(fmt.Sprintf("%d record(s):\n\n", len(history)))

		for _, r := range history {
			status := "ACTIVE"
			invalidAt := "—"
			if r.TInvalid != nil {
				status = "superseded"
				invalidAt = *r.TInvalid
			}
			sb.WriteString(fmt.Sprintf(
				"  ID:%-4d  [%s]  valid_from: %s  invalid_at: %s\n",
				r.ID, status, r.TValid, invalidAt,
			))
		}

		return mcp.NewToolResultText(sb.String()), nil
	}
}

func handleInvalidate(s *store.Store) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := int64(intArgGraph(req, "relation_id", 0))
		if id == 0 {
			return mcp.NewToolResultError("relation_id is required"), nil
		}

		if err := s.InvalidateRelation(id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalidate failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(
			"Relation %d invalidated. It remains in history but is no longer active.", id,
		)), nil
	}
}

func handleRebuildCommunities(s *store.Store) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project := strArgGraph(req, "project", "")
		if project == "" {
			return mcp.NewToolResultError("project is required"), nil
		}

		if err := s.RebuildCommunities(project); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("rebuild communities failed: %v", err)), nil
		}

		communities, err := s.GetCommunities(project, 1000)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("fetch communities failed: %v", err)), nil
		}

		totalMembers := 0
		for _, c := range communities {
			totalMembers += len(c.Members)
		}

		if len(communities) == 0 {
			return mcp.NewToolResultText(
				fmt.Sprintf("Communities rebuilt for project %q. No multi-entity communities found.", project),
			), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Communities rebuilt for: %s\n\n", project))
		sb.WriteString(fmt.Sprintf("**%d communities** with **%d total entities**:\n\n", len(communities), totalMembers))

		for i, c := range communities {
			sb.WriteString(fmt.Sprintf("  Community %d (%d members):\n", i+1, len(c.Members)))
			for _, m := range c.Members {
				proj := ""
				if m.Project != nil {
					proj = " [" + *m.Project + "]"
				}
				sb.WriteString(fmt.Sprintf("    - %s (%s)%s\n", m.Name, m.EntityType, proj))
			}
			sb.WriteString("\n")
		}

		return mcp.NewToolResultText(sb.String()), nil
	}
}

// ─── Entity indexing hook (called after mem_save) ─────────────────────────────

// IndexObservationEntities extracts entities and relations from observation
// content and indexes them into the knowledge graph.
// obsID may be 0 when called from contexts that don't have a single observation
// ID (e.g. mem_capture_passive which saves multiple at once).
// Designed to run in a goroutine — all errors are silently swallowed to avoid
// breaking the primary mem_save flow.
func IndexObservationEntities(s *store.Store, obsID int64, content, project string) {
	ex := extractor.NewRuleExtractor()
	result := ex.Extract(content)

	if len(result.Entities) == 0 {
		return
	}

	// Upsert entities and track their IDs for relation linking.
	// Composite key (name\0type) prevents collisions when the same name
	// appears with different entity types.  Name-only keys are set only
	// once (first type wins) so that relation resolution still works.
	entityIDs := make(map[string]int64, len(result.Entities)*2)
	for _, e := range result.Entities {
		id, err := s.UpsertEntity(e.Name, store.EntityType(e.EntityType), e.Summary, project)
		if err == nil {
			compositeKey := strings.ToLower(e.Name) + "\x00" + strings.ToLower(string(e.EntityType))
			entityIDs[compositeKey] = id
			nameKey := strings.ToLower(e.Name)
			if _, exists := entityIDs[nameKey]; !exists {
				entityIDs[nameKey] = id
			}
		}
	}

	// Insert relations — auto-create missing endpoints as concept entities.
	// Regex rules capture raw text tokens (e.g. "API", "Backend") that may not
	// be gazetteer entities.  Without backfill, every such relation is silently
	// dropped, which is why reindex produced 0 relations on real data.
	//
	// Filter: skip endpoints that are noise (stopwords, punctuation, very short)
	// to avoid polluting the graph with concepts like "and", "the", "for", "-".
	var obsIDPtr *int64
	if obsID > 0 {
		obsIDPtr = &obsID
	}
	for _, r := range result.Relations {
		// Skip relations with noise endpoints.
		if extractor.IsNoiseConcept(r.SourceName) || extractor.IsNoiseConcept(r.TargetName) {
			continue
		}

		srcID, srcOK := entityIDs[strings.ToLower(r.SourceName)]
		tgtID, tgtOK := entityIDs[strings.ToLower(r.TargetName)]
		if !srcOK {
			id, err := s.UpsertEntity(r.SourceName, store.EntityTypeConcept, "", project)
			if err == nil {
				srcID = id
				srcOK = true
				entityIDs[strings.ToLower(r.SourceName)] = id
			}
		}
		if !tgtOK {
			id, err := s.UpsertEntity(r.TargetName, store.EntityTypeConcept, "", project)
			if err == nil {
				tgtID = id
				tgtOK = true
				entityIDs[strings.ToLower(r.TargetName)] = id
			}
		}
		if !srcOK || !tgtOK {
			continue
		}
		s.AddRelation(srcID, tgtID, r.Relation, obsIDPtr) //nolint:errcheck
	}
}

// ─── Argument Helpers ─────────────────────────────────────────────────────────

func strArgGraph(req mcp.CallToolRequest, key, def string) string {
	args := req.GetArguments()
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func intArgGraph(req mcp.CallToolRequest, key string, def int) int {
	args := req.GetArguments()
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	return def
}

func boolArgGraph(req mcp.CallToolRequest, key string, def bool) bool {
	args := req.GetArguments()
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}
