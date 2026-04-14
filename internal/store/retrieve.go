// Package store — retrieve.go
//
// Composite retrieval layer for Mneme. Combines observation search (FTS5)
// with knowledge graph context (entities + relations) into a single
// structured response. One query → rich context, less friction for agents.

package store

import (
	"fmt"
	"strings"
)

// ─── Types ───────────────────────────────────────────────────────────────────

// RetrieveParams holds the input for a composite retrieval query.
type RetrieveParams struct {
	Query   string `json:"query"`
	Project string `json:"project,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Limit   int    `json:"limit,omitempty"` // per-source limit (default: 5)
}

// RetrieveResult is the combined output of a composite retrieval.
type RetrieveResult struct {
	Query        string           `json:"query"`
	Project      string           `json:"project,omitempty"`
	Observations []SearchResult   `json:"observations"`
	Entities     []EntityResult   `json:"entities"`
	GraphEdges   []RelationResult `json:"graph_edges"`
	Total        int              `json:"total"` // combined count of all items
}

// EntityResult wraps an entity with a relevance indicator and its active relations.
type EntityResult struct {
	Entity    Entity     `json:"entity"`
	MatchType string     `json:"match_type"` // "fts" = full-text match, "related" = connected via graph
	Relations []Relation `json:"relations,omitempty"`
}

// RelationResult is a deduplicated graph edge relevant to the query context.
type RelationResult struct {
	SourceName string `json:"source_name"`
	SourceType string `json:"source_type"`
	Relation   string `json:"relation"`
	TargetName string `json:"target_name"`
	TargetType string `json:"target_type"`
	ValidSince string `json:"valid_since"`
}

// ─── Composite Retrieval ─────────────────────────────────────────────────────

// CompositeRetrieve performs a unified search across observations and the
// knowledge graph. It runs two independent searches (FTS5 on observations,
// FTS5 on entities) and then enriches matched entities with their active
// relations. The result is a structured bundle an agent can use directly
// without needing to orchestrate multiple tool calls.
//
// Design: v1 keeps it simple — two parallel FTS5 lookups + relation enrichment.
// No ranking fusion, no cross-source scoring. Results are returned grouped by
// source so the consumer can decide what matters.
func (s *Store) CompositeRetrieve(p RetrieveParams) (*RetrieveResult, error) {
	// Normalize inputs.
	project, _ := NormalizeProject(p.Project)
	scope := normalizeScope(p.Scope)

	limit := p.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	result := &RetrieveResult{
		Query:   p.Query,
		Project: project,
	}

	// 1. Search observations via FTS5.
	obsResults, err := s.Search(p.Query, SearchOptions{
		Project: project,
		Scope:   scope,
		Limit:   limit,
	})
	if err != nil {
		// Non-fatal: observation search failure should not block graph results.
		obsResults = nil
	}
	result.Observations = obsResults

	// 2. Search entities via FTS5.
	entityResults, err := s.SearchEntities(p.Query, "", project, limit)
	if err != nil {
		// Non-fatal: graph search failure should not block observation results.
		entityResults = nil
	}

	// 3. Enrich matched entities with their active relations.
	seenEntityIDs := make(map[int64]bool)
	for _, e := range entityResults {
		if seenEntityIDs[e.ID] {
			continue
		}
		seenEntityIDs[e.ID] = true

		er := EntityResult{
			Entity:    e,
			MatchType: "fts",
		}

		// Load active relations for this entity.
		rels, err := s.GetEntityRelations(e.ID, true)
		if err == nil && len(rels) > 0 {
			er.Relations = rels

			// Extract graph edges from these relations for the flat edge list.
			for _, r := range rels {
				edge := RelationResult{
					Relation:   r.Relation,
					ValidSince: r.TValid,
				}
				if r.SourceID == e.ID {
					edge.SourceName = e.Name
					edge.SourceType = string(e.EntityType)
					// Resolve target entity type.
					edge.TargetName = r.TargetName
					edge.TargetType = resolveEntityType(s, r.TargetID)
				} else {
					edge.TargetName = e.Name
					edge.TargetType = string(e.EntityType)
					edge.SourceName = r.SourceName
					edge.SourceType = resolveEntityType(s, r.SourceID)
				}
				result.GraphEdges = append(result.GraphEdges, edge)
			}
		}

		result.Entities = append(result.Entities, er)
	}

	// 4. Compute totals.
	result.Total = len(result.Observations) + len(result.Entities) + len(result.GraphEdges)

	return result, nil
}

// FormatRetrieveResult renders a RetrieveResult as human-readable text
// suitable for returning via MCP tool responses.
func FormatRetrieveResult(r *RetrieveResult) string {
	if r.Total == 0 {
		return fmt.Sprintf("No context found for query: %q", r.Query)
	}

	var b strings.Builder

	fmt.Fprintf(&b, "## Composite context for: %q", r.Query)
	if r.Project != "" {
		fmt.Fprintf(&b, " (project: %s)", r.Project)
	}
	b.WriteString("\n\n")

	// Observations section.
	if len(r.Observations) > 0 {
		fmt.Fprintf(&b, "### Relevant Memories (%d)\n\n", len(r.Observations))
		for i, obs := range r.Observations {
			projectDisplay := ""
			if obs.Project != nil {
				projectDisplay = fmt.Sprintf(" | project: %s", *obs.Project)
			}
			preview := truncate(obs.Content, 200)
			fmt.Fprintf(&b, "%d. **%s** [%s]%s\n   %s\n   %s | scope: %s\n\n",
				i+1, obs.Title, obs.Type, projectDisplay,
				preview, obs.CreatedAt, obs.Scope)
		}
	}

	// Entities section.
	if len(r.Entities) > 0 {
		fmt.Fprintf(&b, "### Related Entities (%d)\n\n", len(r.Entities))
		for _, er := range r.Entities {
			summary := ""
			if er.Entity.Summary != nil {
				summary = " — " + truncate(*er.Entity.Summary, 100)
			}
			fmt.Fprintf(&b, "- **%s** (%s)%s", er.Entity.Name, er.Entity.EntityType, summary)
			if len(er.Relations) > 0 {
				b.WriteString(" → ")
				for j, rel := range er.Relations {
					if j > 0 {
						b.WriteString(", ")
					}
					// Show the "other side" of the relation.
					if rel.SourceID == er.Entity.ID {
						fmt.Fprintf(&b, "[%s] %s", rel.Relation, rel.TargetName)
					} else {
						fmt.Fprintf(&b, "[%s] %s", rel.Relation, rel.SourceName)
					}
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Graph edges section (flat, deduplicated view).
	if len(r.GraphEdges) > 0 {
		fmt.Fprintf(&b, "### Knowledge Graph Edges (%d)\n\n", len(r.GraphEdges))
		for _, edge := range r.GraphEdges {
			fmt.Fprintf(&b, "- %s (%s) -[%s]-> %s (%s)  [since %s]\n",
				edge.SourceName, edge.SourceType,
				edge.Relation,
				edge.TargetName, edge.TargetType,
				edge.ValidSince)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "---\nTotal: %d memories, %d entities, %d edges\n",
		len(r.Observations), len(r.Entities), len(r.GraphEdges))

	return b.String()
}

// resolveEntityType looks up an entity's type by ID. Returns "" on error.
func resolveEntityType(s *Store, entityID int64) string {
	e, err := s.GetEntityByID(entityID)
	if err != nil {
		return ""
	}
	return string(e.EntityType)
}
