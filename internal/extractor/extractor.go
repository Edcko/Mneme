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
	gazetteers   map[EntityType]map[string]bool
	knownTermSet map[string]bool // lowercase set of all gazetteer terms for person filtering
	patterns     []entityPattern
	relRules     []relationRule
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
	r.knownTermSet = r.buildKnownTermSet()
	return r
}

// buildKnownTermSet creates a lowercase lookup of all gazetteer terms,
// used to filter false-positive person detections.
func (r *RuleExtractor) buildKnownTermSet() map[string]bool {
	set := make(map[string]bool)
	for _, gazette := range r.gazetteers {
		for term := range gazette {
			// Add the full term (handles multi-word entries like "clean architecture")
			set[strings.ToLower(term)] = true
			// Also add individual words (catches "Docker" in "Docker Containers")
			for _, word := range strings.Fields(term) {
				set[strings.ToLower(word)] = true
			}
		}
	}
	return set
}

// isKnownTerm checks if any word in name matches a known gazetteer term.
// Used to prevent tech terms from being classified as person names.
func (r *RuleExtractor) isKnownTerm(name string) bool {
	for _, word := range strings.Fields(name) {
		if r.knownTermSet[strings.ToLower(word)] {
			return true
		}
	}
	return false
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

// conceptStopWords prevents common English words from being extracted as
// concepts. Covers articles, conjunctions, prepositions, pronouns, verbs,
// and other high-frequency/low-information words that pollute communities.
var conceptStopWords = map[string]bool{
	// Articles
	"a": true, "an": true, "the": true,
	// Conjunctions
	"and": true, "but": true, "or": true, "nor": true, "so": true, "yet": true,
	// Prepositions
	"for": true, "to": true, "of": true, "in": true, "on": true, "at": true,
	"by": true, "with": true, "from": true, "as": true, "into": true,
	"through": true, "during": true, "before": true, "after": true,
	"above": true, "below": true, "between": true, "under": true,
	"over": true, "up": true, "down": true, "out": true, "off": true,
	"about": true, "against": true, "without": true, "within": true,
	"along": true, "across": true, "behind": true, "beyond": true,
	// Pronouns
	"it": true, "its": true, "he": true, "him": true, "his": true,
	"she": true, "her": true, "we": true, "us": true, "our": true,
	"they": true, "them": true, "their": true, "you": true, "your": true,
	"my": true, "me": true, "this": true, "that": true, "these": true,
	"those": true, "who": true, "whom": true, "which": true, "what": true,
	// Common verbs (auxiliary / high-frequency)
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "can": true,
	"shall": true, "must": true,
	// Adverbs / temporal
	"not": true, "no": true, "now": true, "then": true, "than": true,
	"too": true, "very": true, "just": true, "also": true, "only": true,
	"here": true, "there": true, "when": true, "where": true, "why": true,
	"how": true, "once": true, "still": true, "even": true, "well": true,
	// Determiners / quantifiers
	"every": true, "some": true, "any": true, "each": true, "both": true,
	"another": true, "other": true, "such": true, "same": true,
	"all": true, "most": true, "more": true, "much": true, "many": true,
	"few": true, "own": true,
	// Generic low-value terms — vague/common words that pollute communities
	// when extracted as isolated concept tokens by regex or backfill.
	// Excludes terms that are also gazetteer entries (e.g. "Make", "Go", "R").
	"start": true, "result": true, "step": true,
	"package": true, "project": true,
	"thing": true, "example": true, "point": true,
	"way": true, "end": true, "work": true, "back": true,
	"use": true, "need": true, "try": true,
	"set": true, "get": true, "run": true, "add": true, "put": true,
	"new": true, "old": true,
	"first": true, "next": true, "last": true,
}

// IsNoiseConcept reports whether name is too generic or low-value to be
// stored as a concept entity. Used by the extractor AND by MCP backfill
// to keep the graph clean.
//
// Checks: empty, pure punctuation/symbols, exact stopword match.
// Does NOT reject short names like "Go", "R", "C#" — those are legitimate
// gazetteer entities that appear in relation endpoints.
//
// Exported because graph_tools.go needs it for relation-endpoint filtering.
func IsNoiseConcept(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))

	// Empty or whitespace-only.
	if lower == "" {
		return true
	}

	// Pure punctuation / symbols (e.g. "-", "_", "--", "==", "+").
	allPunct := true
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			allPunct = false
			break
		}
	}
	if allPunct {
		return true
	}

	// Pure numeric tokens (e.g. "5", "7", "42") — never useful as concepts.
	if isPureDigits(lower) {
		return true
	}

	// CLI flags/options (e.g. "-V", "-h", "--verbose", "--foo").
	if strings.HasPrefix(name, "-") {
		return true
	}

	// Single placeholder letters (x, y, z) — math/code variable placeholders.
	// Does NOT filter legitimate short entities like "Go", "R", "C".
	if len(lower) == 1 && (lower == "x" || lower == "y" || lower == "z") {
		return true
	}

	// Exact stopword match.
	if conceptStopWords[lower] {
		return true
	}

	return false
}

// isNoiseConceptStrict adds a short-token guard on top of IsNoiseConcept.
// Used only inside regex concept extraction — single-char or two-char
// tokens from backticks or definition patterns are almost never useful
// concepts (exceptions like "Go" are handled by the gazetteer layer).
func isNoiseConceptStrict(name string) bool {
	if IsNoiseConcept(name) {
		return true
	}
	if len(strings.TrimSpace(name)) < 3 {
		return true
	}
	return false
}

// isPureDigits reports whether s consists entirely of ASCII digits.
func isPureDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
				if name == "" {
					continue
				}

				// Filter: person names that overlap with known tech terms.
				if p.entityType == EntityTypePerson && r.isKnownTerm(name) {
					continue
				}

				// Filter: noise concepts (stopwords, punctuation, short tokens).
				if p.entityType == EntityTypeConcept {
					if isNoiseConceptStrict(name) {
						continue
					}
				}

				results = append(results, ExtractedEntity{
					Name:       name,
					EntityType: p.entityType,
					Confidence: 0.80,
				})
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

	// Infer implicit "usa" relations when entities co-occur in the same text.
	// Heuristic: languages "use" tools, tools "use" other tools.
	// Only creates relations between already-extracted entities — no noise.
	for i, e1 := range entities {
		if e1.EntityType != EntityTypeLanguage && e1.EntityType != EntityTypeTool {
			continue
		}
		for j, e2 := range entities {
			if i == j {
				continue
			}
			if e2.EntityType == EntityTypeTool {
				results = append(results, ExtractedRelation{
					SourceName: e1.Name,
					Relation:   "usa",
					TargetName: e2.Name,
					Confidence: 0.45,
				})
			}
		}
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

	concepts := []string{
		// Architecture & Design
		"microservices", "monolith", "CQRS", "event sourcing",
		"clean architecture", "hexagonal architecture",
		"domain-driven design", "SOA",
		// Design Principles
		"SOLID", "DRY", "KISS", "YAGNI",
		// Design Patterns
		"singleton", "factory", "observer", "strategy", "adapter",
		"decorator", "proxy", "facade", "mediator", "repository",
		"unit of work",
		// Methods & Practices
		"TDD", "BDD", "CI/CD", "DevOps", "GitOps",
		// Distributed Systems
		"idempotency", "eventual consistency", "CAP theorem",
		"ACID", "consensus",
		// General CS
		"concurrency", "parallelism", "memoization", "polymorphism",
		"encapsulation", "abstraction", "composition", "immutability",
		"recursion", "backpressure", "circuit breaker",
		"rate limiting", "deadlock",
	}

	gazette := map[EntityType]map[string]bool{
		EntityTypeTool:     {},
		EntityTypeLanguage: {},
		EntityTypeConcept:  {},
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
	for _, c := range concepts {
		gazette[EntityTypeConcept][c] = true
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

		// ─── Person patterns ───────────────────────────────────────

		// Person: @username mentions (GitHub/social style)
		{
			re:         regexp.MustCompile(`@(\w{2,})`),
			entityType: EntityTypePerson,
			group:      1,
		},
		// Person: attribution — "by FirstName LastName", "author: FirstName LastName"
		{
			re:         regexp.MustCompile(`(?i)(?:by|author\s*:|credit\s*:)\s+([A-Z][a-z]+\s+[A-Z][a-z]+)`),
			entityType: EntityTypePerson,
			group:      1,
		},
		// Person: communication verbs — "FirstName LastName said/mentioned/..."
		{
			re: regexp.MustCompile(
				`([A-Z][a-z]+\s+[A-Z][a-z]+)\s+(?:said|mentioned|suggested|recommended|pointed\s+out|noted|explained|proposed|asked|reported)`,
			),
			entityType: EntityTypePerson,
			group:      1,
		},
		// Person: attribution — "according to FirstName LastName", "per FirstName LastName"
		{
			re:         regexp.MustCompile(`(?i)(?:according\s+to|per)\s+([A-Z][a-z]+\s+[A-Z][a-z]+)`),
			entityType: EntityTypePerson,
			group:      1,
		},

		// ─── Concept patterns ──────────────────────────────────────

		// Concept: backtick-enclosed terms — `event sourcing`, `CQRS`
		// Only letters and spaces to avoid code/file references.
		{
			re:         regexp.MustCompile("`([a-zA-Z][a-zA-Z ]+[a-zA-Z])`"),
			entityType: EntityTypeConcept,
			group:      1,
		},
		// Concept: definition — "X is a/an/the ..."
		// Uses inline (?i:...) for keyword matching only, keeping [A-Z] case-sensitive.
		{
			re: regexp.MustCompile(
				`\b([A-Z][a-zA-Z]+(?:\s+[a-z]+){0,2}?)\s+(?i:is)\s+(?i:a|an|the)\s+`,
			),
			entityType: EntityTypeConcept,
			group:      1,
		},
		// Concept: suffix — "the X pattern/principle/algorithm/..."
		// Single-word capture for precision; multi-word concepts are in gazetteer.
		// Uses inline (?i:...) for keyword matching, keeping [A-Z] case-sensitive.
		{
			re: regexp.MustCompile(
				`\b([A-Z][a-zA-Z]+)\s+(?i:pattern|principle|algorithm|architecture|paradigm|methodology)\b`,
			),
			entityType: EntityTypeConcept,
			group:      1,
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
		// "X is part of Y" → parte_de
		{
			re: regexp.MustCompile(
				`(?i)([A-Za-z0-9_.+-]+)\s+is\s+part\s+of\s+([A-Za-z0-9_.+-]+)`,
			),
			relation:  "parte_de",
			sourceGrp: 1,
			targetGrp: 2,
		},
		// "X is a type/kind of Y" → es_un
		{
			re: regexp.MustCompile(
				`(?i)([A-Za-z0-9_.+-]+)\s+is\s+(?:a\s+)?(?:type|kind)\s+of\s+([A-Za-z0-9_.+-]+)`,
			),
			relation:  "es_un",
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
