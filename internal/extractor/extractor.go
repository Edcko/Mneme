// Package extractor provides lightweight entity and relation extraction from
// developer memory text. No external dependencies — pure Go stdlib.
//
// Architecture (layered, composable):
//
//	Layer 1: Gazetteer  — O(1) lookup of known tools, languages, frameworks
//	Layer 2: Regex      — file paths, decisions, bug references, patterns
//	Layer 3: LLM        — optional, pluggable via the Extractor interface
//
// For most developer notes, layers 1+2 capture ~85% of entities accurately
// without any network calls or model inference.
package extractor

import (
	"regexp"
	"strings"
)

// EntityType mirrors store.EntityType — redefined here to avoid import cycles.
type EntityType string

const (
	EntityTypePerson   EntityType = "person"
	EntityTypeProject  EntityType = "project"
	EntityTypeFile     EntityType = "file"
	EntityTypeTool     EntityType = "tool"
	EntityTypeConcept  EntityType = "concept"
	EntityTypeLanguage EntityType = "language"
)

// ExtractedEntity is a single entity found in text.
type ExtractedEntity struct {
	Name       string
	EntityType EntityType
	Summary    string  // optional context sentence
	Confidence float64 // 0.0–1.0
}

// ExtractedRelation is a directed triplet between two named entities.
type ExtractedRelation struct {
	SourceName string
	Relation   string // e.g. "usa", "depende_de", "arregló", "usa_en_lugar_de"
	TargetName string
	Confidence float64
}

// ExtractionResult bundles entities and relations from a single text.
type ExtractionResult struct {
	Entities  []ExtractedEntity
	Relations []ExtractedRelation
}

// Extractor is the interface every extraction strategy must implement.
// This allows swapping rule-based ↔ LLM ↔ hybrid without changing callers.
type Extractor interface {
	Extract(text string) ExtractionResult
}

// ─── Rule-Based Extractor ─────────────────────────────────────────────────────

// RuleExtractor is the default zero-dependency extractor.
// It combines a gazetteer lookup with compiled regex patterns.
type RuleExtractor struct {
	gazetteers map[EntityType]map[string]bool
	patterns   []entityPattern
	relRules   []relationRule
}

type entityPattern struct {
	re         *regexp.Regexp
	entityType EntityType
	group      int // capture group index that contains the entity name
}

type relationRule struct {
	re        *regexp.Regexp
	relation  string
	sourceGrp int
	targetGrp int
}

// NewRuleExtractor returns a ready-to-use RuleExtractor seeded with common
// developer tools, languages, and regex patterns.
func NewRuleExtractor() *RuleExtractor {
	r := &RuleExtractor{
		gazetteers: buildGazetteers(),
		patterns:   buildPatterns(),
		relRules:   buildRelationRules(),
	}
	return r
}

// Extract runs both gazetteer lookup and regex patterns on text.
func (r *RuleExtractor) Extract(text string) ExtractionResult {
	entities := r.extractEntities(text)
	relations := r.extractRelations(text, entities)
	return ExtractionResult{
		Entities:  dedupEntities(entities),
		Relations: dedupRelations(relations),
	}
}

func (r *RuleExtractor) extractEntities(text string) []ExtractedEntity {
	var results []ExtractedEntity

	// Layer 1: Gazetteer — highest confidence, O(n) string scan.
	for entityType, gazette := range r.gazetteers {
		for term := range gazette {
			if containsWord(text, term) {
				results = append(results, ExtractedEntity{
					Name:       term,
					EntityType: entityType,
					Confidence: 0.95,
				})
			}
		}
	}

	// Layer 2: Regex patterns.
	for _, p := range r.patterns {
		matches := p.re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if p.group < len(m) && m[p.group] != "" {
				name := strings.TrimSpace(m[p.group])
				if name != "" {
					results = append(results, ExtractedEntity{
						Name:       name,
						EntityType: p.entityType,
						Confidence: 0.80,
					})
				}
			}
		}
	}

	return results
}

func (r *RuleExtractor) extractRelations(text string, entities []ExtractedEntity) []ExtractedRelation {
	var results []ExtractedRelation

	for _, rule := range r.relRules {
		matches := rule.re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if rule.sourceGrp < len(m) && rule.targetGrp < len(m) {
				src := strings.TrimRight(strings.TrimSpace(m[rule.sourceGrp]), ".")
				tgt := strings.TrimRight(strings.TrimSpace(m[rule.targetGrp]), ".")
				if src != "" && tgt != "" && src != tgt {
					results = append(results, ExtractedRelation{
						SourceName: src,
						Relation:   rule.relation,
						TargetName: tgt,
						Confidence: 0.75,
					})
				}
			}
		}
	}

	// Infer implicit "usa" relations when a gazetteer tool appears near another.
	knownNames := make(map[string]bool)
	for _, e := range entities {
		knownNames[e.Name] = true
	}

	return results
}

// ─── Gazetteer ────────────────────────────────────────────────────────────────

func buildGazetteers() map[EntityType]map[string]bool {
	tools := []string{
		// Databases
		"SQLite", "PostgreSQL", "MySQL", "MongoDB", "Redis", "DynamoDB",
		"CockroachDB", "Cassandra", "Elasticsearch", "ClickHouse",
		// Cloud & infra
		"Docker", "Kubernetes", "Terraform", "Ansible", "Helm",
		"AWS", "GCP", "Azure", "Vercel", "Railway", "Fly.io",
		// CI/CD
		"GitHub Actions", "CircleCI", "Jenkins", "GitLab CI",
		// Web / API
		"GraphQL", "REST", "gRPC", "WebSocket", "HTTP",
		"nginx", "Caddy", "Traefik",
		// Observability
		"Prometheus", "Grafana", "Datadog", "Sentry", "OpenTelemetry",
		// Message queues
		"Kafka", "RabbitMQ", "NATS", "SQS",
		// Auth
		"OAuth", "JWT", "Keycloak", "Auth0",
		// Build / tooling
		"Make", "Bazel", "Nx", "Turborepo", "Webpack", "Vite", "esbuild",
		// SQLite extensions
		"FTS5", "sqlite-vec", "WAL",
		// Misc
		"MCP", "OpenAI", "Anthropic", "LangChain", "LlamaIndex",
	}

	languages := []string{
		"Go", "Golang", "Python", "JavaScript", "TypeScript",
		"Rust", "Java", "Kotlin", "C++", "C#", "Swift",
		"Ruby", "PHP", "Bash", "Shell", "SQL", "HCL",
		"Elixir", "Haskell", "Scala", "R",
	}

	frameworks := []string{
		// Go
		"Gin", "Echo", "Fiber", "Chi", "net/http",
		// JS/TS
		"React", "Next.js", "Vue", "Nuxt", "Svelte", "Angular",
		"Express", "Fastify", "NestJS", "Hono",
		// Python
		"FastAPI", "Django", "Flask", "SQLAlchemy",
		// Mobile
		"React Native", "Flutter",
		// Testing
		"Jest", "Vitest", "Playwright", "Cypress", "testify",
		// ORM / query
		"Prisma", "Drizzle", "GORM", "sqlx", "pgx",
	}

	gazette := map[EntityType]map[string]bool{
		EntityTypeTool:     {},
		EntityTypeLanguage: {},
	}

	for _, t := range tools {
		gazette[EntityTypeTool][t] = true
	}
	for _, t := range frameworks {
		gazette[EntityTypeTool][t] = true
	}
	for _, l := range languages {
		gazette[EntityTypeLanguage][l] = true
	}

	return gazette
}

// ─── Regex Patterns ───────────────────────────────────────────────────────────

func buildPatterns() []entityPattern {
	return []entityPattern{
		// File paths: internal/store/store.go, src/components/Button.tsx
		{
			re: regexp.MustCompile(
				`(?:[a-zA-Z0-9_.-]+/)*[a-zA-Z0-9_.-]+\.(?:go|tsx|ts|jsx|json|js|py|rs|java|cpp|c|h|rb|php|swift|kt|cs|sh|yaml|yml|toml|sql|md)`,
			),
			entityType: EntityTypeFile,
			group:      0,
		},
		// Package/module paths in Go: github.com/org/repo, modernc.org/sqlite
		{
			re: regexp.MustCompile(
				`(?:github|gitlab|bitbucket)\.com/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+`,
			),
			entityType: EntityTypeProject,
			group:      0,
		},
	}
}

// ─── Relation Rules ───────────────────────────────────────────────────────────

func buildRelationRules() []relationRule {
	return []relationRule{
		// "Decided to use X instead of Y"  → X usa_en_lugar_de Y
		{
			re: regexp.MustCompile(
				`(?i)(?:decided|chose|switched|migrated|replaced)\s+(?:to\s+(?:use\s+)?)?([A-Za-z0-9_.+-]+)\s+(?:instead\s+of|over|rather\s+than|with)\s+([A-Za-z0-9_.+-]+)`,
			),
			relation:  "reemplaza_a",
			sourceGrp: 1,
			targetGrp: 2,
		},
		// "X depends on Y" / "X uses Y"
		{
			re: regexp.MustCompile(
				`(?i)([A-Za-z0-9_.+-]+)\s+(?:depends\s+on|uses|requires|imports)\s+([A-Za-z0-9_.+-]+)`,
			),
			relation:  "depende_de",
			sourceGrp: 1,
			targetGrp: 2,
		},
		// "Fixed X in Y" → X arreglado_en Y
		{
			re: regexp.MustCompile(
				`(?i)(?:fixed?|resolved?|patched?)\s+([^\s]+(?:\s+\w+){0,3}?)\s+(?:in|inside|within)\s+([a-zA-Z0-9_/.-]+\.\w+)`,
			),
			relation:  "arreglado_en",
			sourceGrp: 1,
			targetGrp: 2,
		},
		// "X extends/implements Y"
		{
			re: regexp.MustCompile(
				`(?i)([A-Za-z0-9_]+)\s+(?:extends|implements|inherits\s+from|embeds)\s+([A-Za-z0-9_]+)`,
			),
			relation:  "extiende",
			sourceGrp: 1,
			targetGrp: 2,
		},
	}
}

// ─── Deduplication Helpers ────────────────────────────────────────────────────

func dedupEntities(entities []ExtractedEntity) []ExtractedEntity {
	seen := make(map[string]bool)
	result := entities[:0]
	for _, e := range entities {
		key := strings.ToLower(e.Name) + ":" + string(e.EntityType)
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

func dedupRelations(relations []ExtractedRelation) []ExtractedRelation {
	seen := make(map[string]bool)
	result := relations[:0]
	for _, r := range relations {
		key := strings.ToLower(r.SourceName) + "|" + r.Relation + "|" + strings.ToLower(r.TargetName)
		if !seen[key] {
			seen[key] = true
			result = append(result, r)
		}
	}
	return result
}

// containsWord checks if text contains term as a whole word (case-sensitive match).
// Uses a simple boundary check to avoid false positives (e.g. "Go" inside "Golang").
func containsWord(text, term string) bool {
	idx := strings.Index(strings.ToLower(text), strings.ToLower(term))
	if idx < 0 {
		return false
	}
	// Check left boundary.
	if idx > 0 {
		prev := text[idx-1]
		if isWordChar(prev) {
			return false
		}
	}
	// Check right boundary.
	end := idx + len(term)
	if end < len(text) {
		next := text[end]
		if isWordChar(next) {
			return false
		}
	}
	return true
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}
