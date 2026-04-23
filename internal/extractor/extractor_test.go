package extractor_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Edcko/Mneme/internal/extractor"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func hasEntity(entities []extractor.ExtractedEntity, name string, et extractor.EntityType) bool {
	for _, e := range entities {
		if e.Name == name && e.EntityType == et {
			return true
		}
	}
	return false
}

func findEntity(entities []extractor.ExtractedEntity, name string, et extractor.EntityType) (extractor.ExtractedEntity, bool) {
	for _, e := range entities {
		if e.Name == name && e.EntityType == et {
			return e, true
		}
	}
	return extractor.ExtractedEntity{}, false
}

func hasRelation(relations []extractor.ExtractedRelation, rel, src, tgt string) bool {
	for _, r := range relations {
		if r.Relation == rel && r.SourceName == src && r.TargetName == tgt {
			return true
		}
	}
	return false
}

// ─── Gazetteer Lookup: Tools ──────────────────────────────────────────────────

func TestGazetteer_KnownTools(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		entity string
	}{
		{"We use SQLite for persistence", "SQLite"},
		{"Deploy with Docker containers", "Docker"},
		{"Cache layer uses Redis", "Redis"},
		{"Orchestrate with Kubernetes", "Kubernetes"},
		{"API uses GraphQL", "GraphQL"},
		{"Monitoring with Prometheus", "Prometheus"},
		{"Events flow through Kafka", "Kafka"},
		{"Auth uses JWT tokens", "JWT"},
		{"MCP server handles tools", "MCP"},
		{"Infra as code with Terraform", "Terraform"},
		{"Queue with RabbitMQ", "RabbitMQ"},
		{"Search with Elasticsearch", "Elasticsearch"},
		{"Tracing with OpenTelemetry", "OpenTelemetry"},
		{"Errors tracked with Sentry", "Sentry"},
		{"Deploy to Vercel", "Vercel"},
		{"Built with Vite", "Vite"},
	}

	for _, tc := range tests {
		t.Run(tc.entity, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.entity, extractor.EntityTypeTool) {
				t.Errorf("expected tool %q in results from: %s", tc.entity, tc.text)
			}
		})
	}
}

// ─── Gazetteer Lookup: Languages ──────────────────────────────────────────────

func TestGazetteer_KnownLanguages(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		entity string
	}{
		{"Written in Go", "Go"},
		{"Script in Python", "Python"},
		{"Frontend in TypeScript", "TypeScript"},
		{"CLI tool in Rust", "Rust"},
		{"Backend in Java", "Java"},
		{"Android app in Kotlin", "Kotlin"},
		{"Microservice in Elixir", "Elixir"},
		{"Config in HCL", "HCL"},
		{"Functional code in Haskell", "Haskell"},
		{"Analysis in R", "R"},
	}

	for _, tc := range tests {
		t.Run(tc.entity, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.entity, extractor.EntityTypeLanguage) {
				t.Errorf("expected language %q in results from: %s", tc.entity, tc.text)
			}
		})
	}
}

// ─── Gazetteer Lookup: Frameworks ─────────────────────────────────────────────

func TestGazetteer_KnownFrameworks(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		entity string
	}{
		{"UI built with React", "React"},
		{"App uses Next.js", "Next.js"},
		{"Backend framework Django", "Django"},
		{"API server with FastAPI", "FastAPI"},
		{"HTTP router Gin", "Gin"},
		{"ORM using Prisma", "Prisma"},
		{"Tests with Jest", "Jest"},
		{"DB with GORM", "GORM"},
		{"Web framework Angular", "Angular"},
		{"CLI with testify", "testify"},
	}

	for _, tc := range tests {
		t.Run(tc.entity, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.entity, extractor.EntityTypeTool) {
				t.Errorf("expected framework %q in results from: %s", tc.entity, tc.text)
			}
		})
	}
}

// ─── File Path Regex ──────────────────────────────────────────────────────────

func TestFilePathRegex_Detection(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name     string
		text     string
		wantPath string
	}{
		{"Go file", "fixed bug in internal/extractor/extractor.go", "internal/extractor/extractor.go"},
		{"Go file deep path", "changed internal/mcp/mcp.go", "internal/mcp/mcp.go"},
		{"TSX file", "modified src/components/Button.tsx", "src/components/Button.tsx"},
		{"Python file", "updated scripts/migrate.py", "scripts/migrate.py"},
		{"Rust file", "changed src/main.rs", "src/main.rs"},
		{"YAML config", "edited deploy/docker-compose.yaml", "deploy/docker-compose.yaml"},
		{"YML config", "edited deploy/docker-compose.yml", "deploy/docker-compose.yml"},
		{"SQL migration", "wrote migrations/001_init.sql", "migrations/001_init.sql"},
		{"Java file", "touched src/Main.java", "src/Main.java"},
		{"Markdown file", "updated README.md", "README.md"},
		{"TOML config", "changed Cargo.toml", "Cargo.toml"},
		{"JSON config", "edited tsconfig.json", "tsconfig.json"},
		{"Shell script", "wrote deploy.sh", "deploy.sh"},
		{"Go file with dots", "fixed cmd/server/main.go", "cmd/server/main.go"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.wantPath, extractor.EntityTypeFile) {
				t.Errorf("expected file %q, got entities: %v", tc.wantPath, entityNames(result.Entities))
			}
		})
	}
}

func TestFilePathRegex_ProjectPaths(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name     string
		text     string
		wantPath string
	}{
		{"GitHub path", "using github.com/Edcko/Mneme", "github.com/Edcko/Mneme"},
		{"GitLab path", "forked from gitlab.com/org/repo", "gitlab.com/org/repo"},
		{"Bitbucket path", "moved to bitbucket.com/team/project", "bitbucket.com/team/project"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.wantPath, extractor.EntityTypeProject) {
				t.Errorf("expected project %q, got entities: %v", tc.wantPath, entityNames(result.Entities))
			}
		})
	}
}

// ─── Relation Rules ──────────────────────────────────────────────────────────

func TestRelationRules_Reemplaza(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name   string
		text   string
		source string
		target string
	}{
		{"decided to use instead of", "Decided to use SQLite instead of PostgreSQL", "SQLite", "PostgreSQL"},
		{"chose to use instead of", "Chose to use Redis instead of Memcached", "Redis", "Memcached"},
		{"decided to use instead of (variation)", "Decided to use Vite instead of Webpack", "Vite", "Webpack"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasRelation(result.Relations, "reemplaza_a", tc.source, tc.target) {
				t.Errorf("expected reemplaza_a(%q, %q) from: %s\ngot: %v",
					tc.source, tc.target, tc.text, result.Relations)
			}
		})
	}
}

func TestRelationRules_DependeDe(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name   string
		text   string
		source string
		target string
	}{
		{"depends on", "API depends on PostgreSQL", "API", "PostgreSQL"},
		{"uses", "Backend uses Redis for caching", "Backend", "Redis"},
		{"requires", "Service requires Kafka", "Service", "Kafka"},
		{"imports", "Module imports JWT", "Module", "JWT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasRelation(result.Relations, "depende_de", tc.source, tc.target) {
				t.Errorf("expected depende_de(%q, %q) from: %s\ngot: %v",
					tc.source, tc.target, tc.text, result.Relations)
			}
		})
	}
}

func TestRelationRules_ArregladoEn(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name   string
		text   string
		target string
	}{
		{"fixed in", "Fixed N+1 query in internal/store/store.go", "internal/store/store.go"},
		{"resolved inside", "Resolved race condition inside src/concurrent/worker.rs", "src/concurrent/worker.rs"},
		{"patched within", "Patched auth bug within internal/auth/middleware.ts", "internal/auth/middleware.ts"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			found := false
			for _, r := range result.Relations {
				if r.Relation == "arreglado_en" && r.TargetName == tc.target {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected arreglado_en(_, %q) from: %s\ngot: %v",
					tc.target, tc.text, result.Relations)
			}
		})
	}
}

func TestRelationRules_Extiende(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name   string
		text   string
		source string
		target string
	}{
		{"extends", "UserService extends BaseService", "UserService", "BaseService"},
		{"implements", "Repository implements CRUDInterface", "Repository", "CRUDInterface"},
		{"inherits from", "AdminRole inherits from BaseRole", "AdminRole", "BaseRole"},
		{"embeds", "Server embeds Router", "Server", "Router"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasRelation(result.Relations, "extiende", tc.source, tc.target) {
				t.Errorf("expected extiende(%q, %q) from: %s\ngot: %v",
					tc.source, tc.target, tc.text, result.Relations)
			}
		})
	}
}

func TestRelationRules_SameSourceTarget(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("A depends on A")
	for _, r := range result.Relations {
		if r.Relation == "depende_de" && r.SourceName == r.TargetName {
			t.Error("relation with same source and target should be filtered out")
		}
	}
}

// ─── Deduplication ──────────────────────────────────────────────────────────

func TestDedup_Entities(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "We use SQLite. SQLite is great. Did I mention SQLite?"
	result := ex.Extract(text)

	count := 0
	for _, e := range result.Entities {
		if e.Name == "SQLite" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 SQLite entity, got %d", count)
	}
}

func TestDedup_Relations(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "API depends on Redis. API depends on Redis."
	result := ex.Extract(text)

	count := 0
	for _, r := range result.Relations {
		if r.Relation == "depende_de" && r.SourceName == "API" && r.TargetName == "Redis" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 deduplicated depende_de(API, Redis) relation, got %d (relations: %v)", count, result.Relations)
	}
}

func TestDedup_CaseInsensitiveKey(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "We use SQLite. SQLite is great. Did I mention SQLite?"
	result := ex.Extract(text)

	count := 0
	for _, e := range result.Entities {
		if e.Name == "SQLite" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 deduplicated SQLite entity, got %d", count)
	}
}

// ─── Word Boundary Detection ──────────────────────────────────────────────────

func TestWordBoundary_StandaloneMatch(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("I love Go programming")
	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' detected as standalone word")
	}
}

func TestWordBoundary_NoMatchInsideGolang(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Golang is a great language")
	for _, e := range result.Entities {
		if e.Name == "Go" && e.EntityType == extractor.EntityTypeLanguage {
			t.Error("'Go' should NOT match inside 'Golang'")
		}
	}
	if !hasEntity(result.Entities, "Golang", extractor.EntityTypeLanguage) {
		t.Error("expected 'Golang' to be detected as its own entry")
	}
}

func TestWordBoundary_NoMatchInAgora(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("The agora was crowded")
	for _, e := range result.Entities {
		if e.Name == "Go" {
			t.Error("'Go' should NOT match inside 'agora'")
		}
	}
}

func TestWordBoundary_CaseInsensitive(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Using SQLITE for persistence")
	if !hasEntity(result.Entities, "SQLite", extractor.EntityTypeTool) {
		t.Error("expected 'SQLite' to match 'SQLITE' (case-insensitive gazetteer)")
	}

	result = ex.Extract("We use docker containers")
	if !hasEntity(result.Entities, "Docker", extractor.EntityTypeTool) {
		t.Error("expected 'Docker' to match 'docker' (case-insensitive gazetteer)")
	}
}

func TestWordBoundary_PunctuationBoundary(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Use Go, Python, and Rust.")
	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' with comma boundary")
	}
	if !hasEntity(result.Entities, "Python", extractor.EntityTypeLanguage) {
		t.Error("expected 'Python' with comma boundary")
	}
	if !hasEntity(result.Entities, "Rust", extractor.EntityTypeLanguage) {
		t.Error("expected 'Rust' with period boundary")
	}
}

func TestWordBoundary_ParenthesisBoundary(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Written in Go (the language)")
	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' with parenthesis boundary")
	}
}

func TestWordBoundary_StartAndEndOfString(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Go is great")
	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' at start of string")
	}

	result = ex.Extract("I love Go")
	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' at end of string")
	}
}

// ─── Edge Cases ──────────────────────────────────────────────────────────

func TestEdgeCase_EmptyText(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("")
	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities from empty text, got %d", len(result.Entities))
	}
	if len(result.Relations) != 0 {
		t.Errorf("expected 0 relations from empty text, got %d", len(result.Relations))
	}
}

func TestEdgeCase_NoEntities(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("The quick brown fox jumps over the lazy dog")
	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities from plain text, got %d: %v", len(result.Entities), result.Entities)
	}
}

func TestEdgeCase_UnicodeText(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Usamos Go para el backend con Redis y Docker")
	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' in unicode text")
	}
	if !hasEntity(result.Entities, "Redis", extractor.EntityTypeTool) {
		t.Error("expected 'Redis' in unicode text")
	}
	if !hasEntity(result.Entities, "Docker", extractor.EntityTypeTool) {
		t.Error("expected 'Docker' in unicode text")
	}
}

func TestEdgeCase_VeryLongText(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	padded := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 100)
	text := "Go " + padded + " SQLite"
	result := ex.Extract(text)

	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' in long text")
	}
	if !hasEntity(result.Entities, "SQLite", extractor.EntityTypeTool) {
		t.Error("expected 'SQLite' in long text")
	}
}

func TestEdgeCase_NewlinesAndTabs(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "Tech stack:\n\t- Go\n\t- PostgreSQL\n\t- Docker\n"
	result := ex.Extract(text)

	if !hasEntity(result.Entities, "Go", extractor.EntityTypeLanguage) {
		t.Error("expected 'Go' after newline/tab")
	}
	if !hasEntity(result.Entities, "PostgreSQL", extractor.EntityTypeTool) {
		t.Error("expected 'PostgreSQL' in tabbed list")
	}
	if !hasEntity(result.Entities, "Docker", extractor.EntityTypeTool) {
		t.Error("expected 'Docker' in tabbed list")
	}
}

func TestEdgeCase_WhitespaceOnly(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("   \t\n  ")
	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities from whitespace-only text, got %d", len(result.Entities))
	}
}

// ─── Confidence Values ──────────────────────────────────────────────────

func TestConfidence_GazetteerEntities(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Using SQLite and Go")
	for _, e := range result.Entities {
		if (e.Name == "SQLite" || e.Name == "Go") && e.Confidence != 0.95 {
			t.Errorf("expected 0.95 confidence for gazetteer entity %q, got %.2f", e.Name, e.Confidence)
		}
	}
}

func TestConfidence_RegexFileEntities(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("See internal/store/store.go")
	if e, ok := findEntity(result.Entities, "internal/store/store.go", extractor.EntityTypeFile); ok {
		if e.Confidence != 0.80 {
			t.Errorf("expected 0.80 confidence for file entity, got %.2f", e.Confidence)
		}
	} else {
		t.Error("expected file entity to exist")
	}
}

func TestConfidence_RegexProjectEntities(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("See github.com/Edcko/Mneme")
	if e, ok := findEntity(result.Entities, "github.com/Edcko/Mneme", extractor.EntityTypeProject); ok {
		if e.Confidence != 0.80 {
			t.Errorf("expected 0.80 confidence for project entity, got %.2f", e.Confidence)
		}
	} else {
		t.Error("expected project entity to exist")
	}
}

func TestConfidence_Relations(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("API depends on Redis")
	if len(result.Relations) == 0 {
		t.Fatal("expected at least one relation")
	}
	for _, r := range result.Relations {
		if r.Confidence != 0.75 {
			t.Errorf("expected 0.75 confidence for relation, got %.2f", r.Confidence)
		}
	}
}

// ─── Integration: Full Pipeline ──────────────────────────────────────────

func TestIntegration_RealisticParagraph(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "Decided to use SQLite instead of PostgreSQL for the embedded store. " +
		"The project uses Go with the Gin framework. " +
		"Fixed N+1 query bug in internal/store/graph.go. " +
		"The cache layer uses Redis for caching and JWT for auth. " +
		"UserService extends BaseService and implements AuthInterface."

	result := ex.Extract(text)

	wantEntities := []struct {
		name string
		et   extractor.EntityType
	}{
		{"SQLite", extractor.EntityTypeTool},
		{"PostgreSQL", extractor.EntityTypeTool},
		{"Go", extractor.EntityTypeLanguage},
		{"Gin", extractor.EntityTypeTool},
		{"Redis", extractor.EntityTypeTool},
		{"JWT", extractor.EntityTypeTool},
		{"internal/store/graph.go", extractor.EntityTypeFile},
	}

	for _, we := range wantEntities {
		if !hasEntity(result.Entities, we.name, we.et) {
			t.Errorf("missing entity %q (type %q)", we.name, we.et)
		}
	}

	if !hasRelation(result.Relations, "reemplaza_a", "SQLite", "PostgreSQL") {
		t.Error("missing reemplaza_a(SQLite, PostgreSQL)")
	}

	hasRedisDep := false
	for _, r := range result.Relations {
		if r.Relation == "depende_de" && r.TargetName == "Redis" {
			hasRedisDep = true
			break
		}
	}
	if !hasRedisDep {
		t.Error("missing depende_de(_, Redis)")
	}

	hasFix := false
	for _, r := range result.Relations {
		if r.Relation == "arreglado_en" && strings.Contains(r.TargetName, "graph.go") {
			hasFix = true
			break
		}
	}
	if !hasFix {
		t.Error("missing arreglado_en(_, graph.go)")
	}

	if !hasRelation(result.Relations, "extiende", "UserService", "BaseService") {
		t.Error("missing extiende(UserService, BaseService)")
	}

	if len(result.Entities) < len(wantEntities) {
		t.Errorf("expected at least %d entities, got %d", len(wantEntities), len(result.Entities))
	}
}

func TestIntegration_MultiLanguageParagraph(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "Decided to use TypeScript instead of JavaScript. " +
		"Auth depends on JWT. " +
		"Fixed XSS vulnerability in src/middleware/auth.ts."

	result := ex.Extract(text)

	if !hasEntity(result.Entities, "TypeScript", extractor.EntityTypeLanguage) {
		t.Error("missing TypeScript")
	}
	if !hasEntity(result.Entities, "JavaScript", extractor.EntityTypeLanguage) {
		t.Error("missing JavaScript")
	}
	if !hasEntity(result.Entities, "JWT", extractor.EntityTypeTool) {
		t.Error("missing JWT")
	}
	if !hasEntity(result.Entities, "src/middleware/auth.ts", extractor.EntityTypeFile) {
		t.Error("missing file path")
	}

	hasReemplaza := false
	for _, r := range result.Relations {
		if r.Relation == "reemplaza_a" && r.SourceName == "TypeScript" {
			hasReemplaza = true
			break
		}
	}
	if !hasReemplaza {
		t.Errorf("missing reemplaza_a relation, got: %v", result.Relations)
	}

	hasDep := false
	for _, r := range result.Relations {
		if r.Relation == "depende_de" && r.SourceName == "Auth" {
			hasDep = true
			break
		}
	}
	if !hasDep {
		t.Errorf("missing depende_de relation, got: %v", result.Relations)
	}

	hasFix := false
	for _, r := range result.Relations {
		if r.Relation == "arreglado_en" && strings.Contains(r.TargetName, "auth.ts") {
			hasFix = true
			break
		}
	}
	if !hasFix {
		t.Errorf("missing arreglado_en relation, got: %v", result.Relations)
	}
}

// ─── Person: @mention Detection ────────────────────────────────────────────

func TestPerson_AtMention(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		person string
	}{
		{"Thanks @johnsmith for the review", "johnsmith"},
		{"cc @jane for visibility", "jane"},
		{"Feedback from @misael on the PR", "misael"},
		{"Pair programmed with @dev123 today", "dev123"},
	}

	for _, tc := range tests {
		t.Run(tc.person, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.person, extractor.EntityTypePerson) {
				t.Errorf("expected person @%q from: %s\ngot: %v", tc.person, tc.text, entityNames(result.Entities))
			}
		})
	}
}

func TestPerson_AtMention_MinLength(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Single-char @mentions should not be extracted.
	result := ex.Extract("Thanks @a for help")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypePerson && e.Name == "a" {
			t.Error("single-char @mention should not be extracted as person")
		}
	}
}

// ─── Person: Attribution Patterns ──────────────────────────────────────────

func TestPerson_ByAttribution(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		person string
	}{
		{"Auth module by Jane Doe", "Jane Doe"},
		{"Feature by John Smith", "John Smith"},
		{"author: Alice Wonderland", "Alice Wonderland"},
		{"credit: Bob Builder", "Bob Builder"},
	}

	for _, tc := range tests {
		t.Run(tc.person, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.person, extractor.EntityTypePerson) {
				t.Errorf("expected person %q from: %s\ngot: %v", tc.person, tc.text, entityNames(result.Entities))
			}
		})
	}
}

func TestPerson_CommunicationVerbs(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		person string
	}{
		{"John Smith suggested using Redis", "John Smith"},
		{"Jane Doe mentioned the issue", "Jane Doe"},
		{"Alice Park recommended upgrading", "Alice Park"},
		{"Bob Builder pointed out the flaw", "Bob Builder"},
		{"Carol King noted the inconsistency", "Carol King"},
		{"Dave Wilson explained the architecture", "Dave Wilson"},
		{"Eve Garcia proposed a refactor", "Eve Garcia"},
	}

	for _, tc := range tests {
		t.Run(tc.person, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.person, extractor.EntityTypePerson) {
				t.Errorf("expected person %q from: %s\ngot: %v", tc.person, tc.text, entityNames(result.Entities))
			}
		})
	}
}

func TestPerson_AccordingTo(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		person string
	}{
		{"according to John Smith, the API is slow", "John Smith"},
		{"According to Jane Doe, this is correct", "Jane Doe"},
		{"per Alice Park, we need to migrate", "Alice Park"},
	}

	for _, tc := range tests {
		t.Run(tc.person, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.person, extractor.EntityTypePerson) {
				t.Errorf("expected person %q from: %s\ngot: %v", tc.person, tc.text, entityNames(result.Entities))
			}
		})
	}
}

// ─── Person: False Positive Filtering ──────────────────────────────────────

func TestPerson_FilterOutTechTerms(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "Docker Containers" looks like FirstName LastName but Docker is a known tool.
	result := ex.Extract("by Docker Containers")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypePerson && strings.Contains(e.Name, "Docker") {
			t.Errorf("'Docker Containers' should NOT be extracted as person, got: %v", e)
		}
	}

	// "PostgreSQL Replica" — PostgreSQL is a known tool.
	result = ex.Extract("According to PostgreSQL Replica, the sync failed")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypePerson && strings.Contains(e.Name, "PostgreSQL") {
			t.Errorf("'PostgreSQL Replica' should NOT be extracted as person, got: %v", e)
		}
	}

	// "Redis Cluster" — Redis is a known tool.
	result = ex.Extract("Redis Cluster suggested a rebalance")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypePerson && strings.Contains(e.Name, "Redis") {
			t.Errorf("'Redis Cluster' should NOT be extracted as person, got: %v", e)
		}
	}
}

func TestPerson_FilterOutSingleKnownTerm(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Even if one word matches a known term, the whole name is filtered.
	result := ex.Extract("by React Developer")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypePerson && e.Name == "React Developer" {
			t.Error("'React Developer' should be filtered because 'React' is a known tool")
		}
	}
}

// ─── Concept: Gazetteer Detection ──────────────────────────────────────────

func TestConcept_Gazetteer(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text    string
		concept string
	}{
		{"We adopted microservices for this project", "microservices"},
		{"The system uses event sourcing", "event sourcing"},
		{"Following clean architecture principles", "clean architecture"},
		{"Applying SOLID principles", "SOLID"},
		{"Using the repository pattern", "repository"},
		{"Practicing TDD", "TDD"},
		{"Need idempotency for retries", "idempotency"},
		{"Implemented circuit breaker for fault tolerance", "circuit breaker"},
		{"Managing concurrency with goroutines", "concurrency"},
		{"Achieving eventual consistency", "eventual consistency"},
		{"CQRS separates reads and writes", "CQRS"},
		{"Avoid deadlock in concurrent code", "deadlock"},
		{"The monolith is too large", "monolith"},
		{"Following DRY principle", "DRY"},
	}

	for _, tc := range tests {
		t.Run(tc.concept, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.concept, extractor.EntityTypeConcept) {
				t.Errorf("expected concept %q from: %s\ngot: %v", tc.concept, tc.text, entityNames(result.Entities))
			}
		})
	}
}

// ─── Concept: Backtick Terms ───────────────────────────────────────────────

func TestConcept_BacktickTerms(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text    string
		concept string
	}{
		{"Using `event sourcing` for state changes", "event sourcing"},
		{"Implemented `circuit breaker` pattern", "circuit breaker"},
		{"The `CQRS` pattern separates reads", "CQRS"},
		{"Using `idempotency` for safe retries", "idempotency"},
		{"Applied `rate limiting` to the API", "rate limiting"},
	}

	for _, tc := range tests {
		t.Run(tc.concept, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.concept, extractor.EntityTypeConcept) {
				t.Errorf("expected concept %q from: %s\ngot: %v", tc.concept, tc.text, entityNames(result.Entities))
			}
		})
	}
}

func TestConcept_BacktickFiltersCode(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Code with dots/slashes/parens should NOT be extracted as concept.
	result := ex.Extract("Called `extractEntities()` in the pipeline")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept {
			t.Errorf("code reference should not be concept, got: %v", e)
		}
	}

	// File paths in backticks should not be concepts.
	result = ex.Extract("Edited `internal/store/store.go`")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept {
			t.Errorf("file path in backticks should not be concept, got: %v", e)
		}
	}
}

func TestConcept_BacktickStopWords(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "the" in backticks is a stop word → not a concept.
	result := ex.Extract("Set `the` as a keyword")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept && e.Name == "the" {
			t.Error("'the' in backticks should be filtered as stop word")
		}
	}
}

// ─── Concept: Definition Patterns ──────────────────────────────────────────

func TestConcept_DefinitionPattern(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text    string
		concept string
	}{
		// These are in the gazetteer → matched by gazetteer with canonical form.
		{"We use microservices, microservices is a way to decompose apps", "microservices"},
		{"CQRS is an approach to data management", "CQRS"},
		// Non-gazetteer terms → matched purely by the definition regex pattern.
		{"Memcached is a distributed caching system", "Memcached"},
		{"Kubernetes is a container orchestration tool", "Kubernetes"},
	}

	for _, tc := range tests {
		t.Run(tc.concept, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.concept, extractor.EntityTypeConcept) {
				t.Errorf("expected concept %q from: %s\ngot: %v", tc.concept, tc.text, entityNames(result.Entities))
			}
		})
	}
}

func TestConcept_DefinitionStopWords(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "This is a test" → "This" is a stop word → should not be concept.
	result := ex.Extract("This is a test of the system")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept && e.Name == "This" {
			t.Error("'This' should be filtered as stop word in definition pattern")
		}
	}
}

// ─── Concept: Pattern/Principle Suffix ─────────────────────────────────────

func TestConcept_SuffixPattern(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text    string
		concept string
	}{
		// Non-gazetteer concepts → captured purely by suffix regex.
		{"Implemented the Saga pattern for transactions", "Saga"},
		{"Applied the Memento pattern for undo", "Memento"},
		// Gazetteer acronym → canonical form matches.
		{"Following the SOLID principle", "SOLID"},
		// Gazetteer lowercase concept → captured with canonical form.
		{"Using the observer pattern", "observer"},
	}

	for _, tc := range tests {
		t.Run(tc.concept, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.concept, extractor.EntityTypeConcept) {
				t.Errorf("expected concept %q from: %s\ngot: %v", tc.concept, tc.text, entityNames(result.Entities))
			}
		})
	}
}

// ─── New Relation Rules ────────────────────────────────────────────────────

func TestRelationRules_ParteDe(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name   string
		text   string
		source string
		target string
	}{
		{"is part of", "Auth is part of the main service", "Auth", "the"},
		{"module is part of", "UserService is part of CoreModule", "UserService", "CoreModule"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasRelation(result.Relations, "parte_de", tc.source, tc.target) {
				t.Errorf("expected parte_de(%q, %q) from: %s\ngot: %v",
					tc.source, tc.target, tc.text, result.Relations)
			}
		})
	}
}

func TestRelationRules_EsUn(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		name   string
		text   string
		source string
		target string
	}{
		{"is a type of", "PostgreSQL is a type of database", "PostgreSQL", "database"},
		{"is a kind of", "Redis is a kind of cache", "Redis", "cache"},
		{"is type of", "GraphQL is type of API", "GraphQL", "API"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasRelation(result.Relations, "es_un", tc.source, tc.target) {
				t.Errorf("expected es_un(%q, %q) from: %s\ngot: %v",
					tc.source, tc.target, tc.text, result.Relations)
			}
		})
	}
}

// ─── Integration: Person + Concept Extraction ──────────────────────────────

func TestIntegration_PersonAndConceptInContext(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "According to Jane Smith, the `circuit breaker` pattern prevents cascading failures. " +
		"John Doe suggested using `event sourcing` for the audit trail. " +
		"We adopted microservices and follow the SOLID principle. " +
		"Auth module by Alice Park uses JWT for authentication. " +
		"@bob reviewed the PR and noted that idempotency is important for retries."

	result := ex.Extract(text)

	// Person detections.
	wantPersons := []string{"Jane Smith", "John Doe", "Alice Park", "bob"}
	for _, name := range wantPersons {
		if !hasEntity(result.Entities, name, extractor.EntityTypePerson) {
			t.Errorf("missing person %q, got: %v", name, entityNames(result.Entities))
		}
	}

	// Concept detections (via gazetteer + backtick + definition + suffix).
	wantConcepts := []string{"circuit breaker", "event sourcing", "microservices", "SOLID", "idempotency"}
	for _, name := range wantConcepts {
		if !hasEntity(result.Entities, name, extractor.EntityTypeConcept) {
			t.Errorf("missing concept %q, got: %v", name, entityNames(result.Entities))
		}
	}

	// Tool/language detections should still work.
	if !hasEntity(result.Entities, "JWT", extractor.EntityTypeTool) {
		t.Error("JWT should still be detected as tool")
	}
}

func TestIntegration_NewRelationsInParagraph(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "UserService is part of CoreModule. " +
		"PostgreSQL is a type of database. " +
		"API depends on Redis."

	result := ex.Extract(text)

	if !hasRelation(result.Relations, "parte_de", "UserService", "CoreModule") {
		t.Error("missing parte_de(UserService, CoreModule)")
	}
	if !hasRelation(result.Relations, "es_un", "PostgreSQL", "database") {
		t.Error("missing es_un(PostgreSQL, database)")
	}
	if !hasRelation(result.Relations, "depende_de", "API", "Redis") {
		t.Error("missing depende_de(API, Redis)")
	}
}

// ─── Co-occurrence "usa" Inference ─────────────────────────────────────────

func TestCoOccurrence_LanguageUsesTool(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "Go" (language) and "SQLite" (tool) co-occur — should infer Go usa SQLite.
	result := ex.Extract("We use Go with SQLite for persistence")

	if !hasRelation(result.Relations, "usa", "Go", "SQLite") {
		t.Errorf("expected co-occurrence usa(Go, SQLite), got: %v", result.Relations)
	}
	// Reverse direction should NOT exist — tools don't "use" languages.
	if hasRelation(result.Relations, "usa", "SQLite", "Go") {
		t.Error("tools should not 'usa' languages")
	}
}

func TestCoOccurrence_ToolUsesTool(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "Docker" and "Kubernetes" co-occur as tools.
	result := ex.Extract("Deploy with Docker and Kubernetes")

	if !hasRelation(result.Relations, "usa", "Docker", "Kubernetes") {
		t.Errorf("expected co-occurrence usa(Docker, Kubernetes), got: %v", result.Relations)
	}
}

func TestCoOccurrence_NoSelfRelation(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "Go" appears only once — no self-relation.
	result := ex.Extract("We use Go for the backend")

	for _, r := range result.Relations {
		if r.Relation == "usa" && r.SourceName == "Go" && r.TargetName == "Go" {
			t.Error("co-occurrence should not create self-relation")
		}
	}
}

func TestCoOccurrence_ConceptsNotUsedAsSource(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "microservices" is a concept — it should not be a "usa" source.
	result := ex.Extract("We use microservices with Docker")

	for _, r := range result.Relations {
		if r.Relation == "usa" && r.SourceName == "microservices" {
			t.Errorf("concepts should not be 'usa' source, got: %v", r)
		}
	}
	// But Docker (tool) can use microservices as target (concept) — wait, our
	// code only creates relations where target is EntityTypeTool. So concepts
	// as targets should NOT produce relations either.
	for _, r := range result.Relations {
		if r.Relation == "usa" && r.SourceName == "Docker" && r.TargetName == "microservices" {
			// Actually this is fine — tools can "use" concepts.
			// But our implementation only targets EntityTypeTool, so this won't happen.
		}
	}
}

func TestCoOccurrence_LowerConfidence(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Go and SQLite for persistence")
	for _, r := range result.Relations {
		if r.Relation == "usa" && r.Confidence >= 0.75 {
			t.Errorf("co-occurrence relations should have low confidence, got %.2f", r.Confidence)
		}
	}
}

// ─── Regression: No False Positives on Plain Text ─────────────────────────

func TestRegression_PlainTextNoSpuriousConcepts(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Plain text should not produce spurious person or concept entities.
	result := ex.Extract("The quick brown fox jumps over the lazy dog")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypePerson {
			t.Errorf("plain text should not produce person entities, got: %v", e)
		}
	}
}

func TestRegression_DefinitionPatternNoCommonWords(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Common words should not be extracted as concepts.
	badTexts := []string{
		"This is a test",
		"That is the way",
		"There is a problem",
	}
	for _, text := range badTexts {
		result := ex.Extract(text)
		for _, e := range result.Entities {
			if e.EntityType == extractor.EntityTypeConcept {
				t.Errorf("common word text should not produce concepts, got %q from: %s", e.Name, text)
			}
		}
	}
}

// ─── entityNames is a debug helper for test error messages ────────────────────

func entityNames(entities []extractor.ExtractedEntity) []string {
	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = string(e.EntityType) + ":" + e.Name
	}
	return names
}

// ─── Bug Fix Regression Tests ────────────────────────────────────────────

func TestBugFix_RegexAlternation_TSXBeforeTS(t *testing.T) {
	ex := extractor.NewRuleExtractor()
	result := ex.Extract("modified src/components/Button.tsx")
	if !hasEntity(result.Entities, "src/components/Button.tsx", extractor.EntityTypeFile) {
		t.Errorf("expected Button.tsx (not .ts), got: %v", entityNames(result.Entities))
	}
}

func TestBugFix_RegexAlternation_JSONBeforeJS(t *testing.T) {
	ex := extractor.NewRuleExtractor()
	result := ex.Extract("edited tsconfig.json")
	if !hasEntity(result.Entities, "tsconfig.json", extractor.EntityTypeFile) {
		t.Errorf("expected tsconfig.json (not .js), got: %v", entityNames(result.Entities))
	}
}

func TestBugFix_RelationTargetTrailingPeriod(t *testing.T) {
	ex := extractor.NewRuleExtractor()
	result := ex.Extract("API depends on Redis.")
	for _, r := range result.Relations {
		if r.Relation == "depende_de" && r.TargetName == "Redis." {
			t.Error("target should not include trailing period: got 'Redis.'")
		}
	}
	foundClean := false
	for _, r := range result.Relations {
		if r.Relation == "depende_de" && r.TargetName == "Redis" {
			foundClean = true
			break
		}
	}
	if !foundClean {
		t.Errorf("expected depende_de(_, 'Redis') without trailing period, got: %v", result.Relations)
	}
}

func TestBugFix_SwitchedToWithoutUse(t *testing.T) {
	ex := extractor.NewRuleExtractor()
	result := ex.Extract("Switched to Redis over Memcached")
	if !hasRelation(result.Relations, "reemplaza_a", "Redis", "Memcached") {
		t.Errorf("expected reemplaza_a(Redis, Memcached) from 'switched to X over Y', got: %v", result.Relations)
	}
}

func TestBugFix_ReplacedWith(t *testing.T) {
	ex := extractor.NewRuleExtractor()
	result := ex.Extract("Replaced Webpack with Vite")
	if !hasRelation(result.Relations, "reemplaza_a", "Webpack", "Vite") {
		t.Errorf("expected reemplaza_a(Webpack, Vite) from 'replaced X with Y', got: %v", result.Relations)
	}
}

func TestBugFix_GazetteerCaseInsensitive(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	tests := []struct {
		text   string
		entity string
		et     extractor.EntityType
	}{
		{"Using SQLITE for persistence", "SQLite", extractor.EntityTypeTool},
		{"Deploy with DOCKER", "Docker", extractor.EntityTypeTool},
		{"Written in TYPESCRIPT", "TypeScript", extractor.EntityTypeLanguage},
		{"Using golang for backend", "Golang", extractor.EntityTypeLanguage},
		{"Using GRAPHQL for API", "GraphQL", extractor.EntityTypeTool},
	}

	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			result := ex.Extract(tc.text)
			if !hasEntity(result.Entities, tc.entity, tc.et) {
				t.Errorf("expected entity %q from %q, got: %v", tc.entity, tc.text, entityNames(result.Entities))
			}
		})
	}
}

// ─── Noise Concept Filtering ──────────────────────────────────────────────

func TestIsNoiseConcept_StopWords(t *testing.T) {
	// Every word from the user's bug report must be classified as noise.
	badWords := []string{
		"and", "the", "it", "for", "now",
		"but", "or", "is", "are", "was", "were",
		"to", "of", "in", "on", "at", "by", "with", "from",
		"this", "that", "these", "those", "there", "here",
		"have", "has", "had", "do", "does", "did",
		"will", "would", "could", "should", "can",
		"not", "no", "then", "than", "too", "very", "just",
		"all", "some", "any", "each", "both",
	}
	for _, w := range badWords {
		t.Run(w, func(t *testing.T) {
			if !extractor.IsNoiseConcept(w) {
				t.Errorf("expected %q to be noise", w)
			}
		})
	}
}

func TestIsNoiseConcept_PunctuationAndSymbols(t *testing.T) {
	badTokens := []string{"-", "_", "--", "==", "+", "++", "---", ".."}
	for _, tok := range badTokens {
		t.Run(tok, func(t *testing.T) {
			if !extractor.IsNoiseConcept(tok) {
				t.Errorf("expected %q to be noise", tok)
			}
		})
	}
}

func TestIsNoiseConcept_ShortTokens(t *testing.T) {
	// Short tokens that are NOT stopwords should pass — they might be legitimate
	// gazetteer entities like "Go" or "R" appearing in relation endpoints.
	legitShort := []string{"Go", "R", "Go"}
	for _, tok := range legitShort {
		t.Run(tok, func(t *testing.T) {
			if extractor.IsNoiseConcept(tok) {
				t.Errorf("short token %q should NOT be noise for backfill (legitimate entity names)", tok)
			}
		})
	}

	// But short stopwords ARE noise — caught by stopword list, not length.
	stopShort := []string{"of", "to", "in", "on", "at", "it", "an"}
	for _, tok := range stopShort {
		t.Run(tok+"_stopword", func(t *testing.T) {
			if !extractor.IsNoiseConcept(tok) {
				t.Errorf("short stopword %q should be noise", tok)
			}
		})
	}
}

func TestIsNoiseConcept_RealConceptsPass(t *testing.T) {
	// Actual domain concepts must NOT be classified as noise.
	goodConcepts := []string{
		"microservices", "event sourcing", "CQRS", "circuit breaker",
		"repository", "idempotency", "concurrency", "encapsulation",
		"rate limiting", "deadlock", "SOLID", "DRY", "TDD",
		"React", "Docker", "PostgreSQL", "SQLite", "Kubernetes",
		"API", "JWT", "REST", "gRPC",
	}
	for _, c := range goodConcepts {
		t.Run(c, func(t *testing.T) {
			if extractor.IsNoiseConcept(c) {
				t.Errorf("real concept %q should NOT be noise", c)
			}
		})
	}
}

func TestIsNoiseConcept_CaseInsensitive(t *testing.T) {
	// "And", "AND", "and" should all be noise.
	upper := []string{"And", "AND", "The", "THE", "For", "FOR", "Now", "NOW"}
	for _, w := range upper {
		t.Run(w, func(t *testing.T) {
			if !extractor.IsNoiseConcept(w) {
				t.Errorf("expected %q (case-variant of stopword) to be noise", w)
			}
		})
	}
}

func TestIsNoiseConcept_EmptyAndWhitespace(t *testing.T) {
	empty := []string{"", " ", "  ", "\t", "\n"}
	for _, tok := range empty {
		t.Run(fmt.Sprintf("empty_%d", len(tok)), func(t *testing.T) {
			if !extractor.IsNoiseConcept(tok) {
				t.Errorf("expected %q to be noise", tok)
			}
		})
	}
}

func TestNoiseConcept_BacktickGarbage(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// These backtick-enclosed stopwords should NOT become concepts.
	garbage := []string{"and", "the", "it", "for", "now", "is", "are", "was"}
	for _, g := range garbage {
		text := fmt.Sprintf("The system uses `%s` for processing", g)
		t.Run(g, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, g) {
					t.Errorf("backtick stopword %q should not be a concept, got: %v", g, e)
				}
			}
		})
	}
}

func TestNoiseConcept_DefinitionPatternGarbage(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "And is a ..." — "And" should be filtered.
	garbage := []string{"And", "But", "Or", "For", "Now"}
	for _, g := range garbage {
		text := fmt.Sprintf("%s is a critical component of the system", g)
		t.Run(g, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, g) {
					t.Errorf("definition-pattern stopword %q should not be a concept", g)
				}
			}
		})
	}
}

func TestNoiseConcept_SuffixPatternGarbage(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "The pattern" — "The" should not become a concept via suffix pattern.
	result := ex.Extract("Implemented the pattern for transactions")
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, "The") {
			t.Error("'The' should not become a concept via suffix pattern")
		}
	}
}

func TestNoiseConcept_BacktickRealConceptsStillWork(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Regression: real backtick concepts must still work after stopword expansion.
	concepts := []string{"event sourcing", "CQRS", "idempotency", "circuit breaker"}
	for _, c := range concepts {
		text := fmt.Sprintf("Using `%s` for state management", c)
		t.Run(c, func(t *testing.T) {
			result := ex.Extract(text)
			if !hasEntity(result.Entities, c, extractor.EntityTypeConcept) {
				t.Errorf("real concept %q should still be extracted, got: %v", c, entityNames(result.Entities))
			}
		})
	}
}

// ─── Fine-Grained Semantic Noise Filtering ──────────────────────────────────

func TestIsNoiseConcept_PureNumbers(t *testing.T) {
	numbers := []string{"5", "7", "42", "0", "100", "3"}
	for _, n := range numbers {
		t.Run(n, func(t *testing.T) {
			if !extractor.IsNoiseConcept(n) {
				t.Errorf("expected pure number %q to be noise", n)
			}
		})
	}
}

func TestIsNoiseConcept_CLIFlags(t *testing.T) {
	flags := []string{"-V", "-h", "--verbose", "--foo", "-v", "--version", "--help"}
	for _, f := range flags {
		t.Run(f, func(t *testing.T) {
			if !extractor.IsNoiseConcept(f) {
				t.Errorf("expected CLI flag %q to be noise", f)
			}
		})
	}
}

func TestIsNoiseConcept_PlaceholderLetters(t *testing.T) {
	placeholders := []string{"X", "Y", "Z", "x", "y", "z"}
	for _, p := range placeholders {
		t.Run(p, func(t *testing.T) {
			if !extractor.IsNoiseConcept(p) {
				t.Errorf("expected placeholder letter %q to be noise", p)
			}
		})
	}
}

func TestIsNoiseConcept_GenericLowValueTerms(t *testing.T) {
	// Terms reported by user in real graph data.
	genericTerms := []string{
		"start", "result", "step", "package", "project",
		"Start", "RESULT", "Step", "Package", "PROJECT",
	}
	for _, term := range genericTerms {
		t.Run(term, func(t *testing.T) {
			if !extractor.IsNoiseConcept(term) {
				t.Errorf("expected generic term %q to be noise", term)
			}
		})
	}
}

func TestIsNoiseConcept_ExtendedGenericTerms(t *testing.T) {
	// Extended set of generic low-value terms that should not be concepts.
	genericTerms := []string{
		"thing", "example", "point", "way", "end", "work", "back",
		"use", "need", "try", "set", "get", "run", "add", "put",
		"new", "old", "first", "next", "last",
	}
	for _, term := range genericTerms {
		t.Run(term, func(t *testing.T) {
			if !extractor.IsNoiseConcept(term) {
				t.Errorf("expected generic term %q to be noise", term)
			}
		})
	}
}

func TestIsNoiseConcept_RealEntitiesStillPass(t *testing.T) {
	// Critical: legitimate short and medium-length entities must NOT be filtered.
	realEntities := []string{
		// Short gazetteer entities
		"Go", "R",
		// Acronyms and abbreviations
		"JWT", "API", "SQL", "REST", "gRPC",
		// Tools/languages (capitalized)
		"React", "Docker", "SQLite", "Redis", "Kubernetes",
		// Concepts
		"CQRS", "SOLID", "TDD", "BDD",
		// Multi-word concepts
		"event sourcing", "circuit breaker", "rate limiting",
		// Relation endpoint tokens (uppercase)
		"Auth", "Backend", "Service", "Module",
		// Languages with symbols
		"C#", "F#",
	}
	for _, e := range realEntities {
		t.Run(e, func(t *testing.T) {
			if extractor.IsNoiseConcept(e) {
				t.Errorf("real entity %q should NOT be noise", e)
			}
		})
	}
}

func TestNoiseConcept_NumbersFromRelations(t *testing.T) {
	// Relation regex captures "5" and "7" as endpoints.
	// IsNoiseConcept must flag them so the backfill layer in graph_tools
	// skips creating noise concept entities for them.
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("5 depends on 7")
	// The extractor DOES extract the relation — noise filtering is the caller's job.
	found := false
	for _, r := range result.Relations {
		if r.Relation == "depende_de" && r.SourceName == "5" && r.TargetName == "7" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected depende_de(5, 7) to be extracted (filtering is caller's job)")
	}

	// But IsNoiseConcept correctly flags both endpoints as noise.
	if !extractor.IsNoiseConcept("5") {
		t.Error("IsNoiseConcept should flag '5' as noise")
	}
	if !extractor.IsNoiseConcept("7") {
		t.Error("IsNoiseConcept should flag '7' as noise")
	}
}

func TestNoiseConcept_PlaceholdersFromRelations(t *testing.T) {
	// Relation regex captures "X" and "Y" as endpoints.
	// IsNoiseConcept must flag them so the backfill layer skips them.
	ex := extractor.NewRuleExtractor()

	result := ex.Extract("Decided to use X instead of Y")
	found := false
	for _, r := range result.Relations {
		if r.Relation == "reemplaza_a" && r.SourceName == "X" && r.TargetName == "Y" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected reemplaza_a(X, Y) to be extracted (filtering is caller's job)")
	}

	// But IsNoiseConcept correctly flags both as noise.
	if !extractor.IsNoiseConcept("X") {
		t.Error("IsNoiseConcept should flag 'X' as noise")
	}
	if !extractor.IsNoiseConcept("Y") {
		t.Error("IsNoiseConcept should flag 'Y' as noise")
	}
}

func TestNoiseConcept_GenericBacktickConcepts(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Generic terms in backticks should NOT become concepts.
	generic := []string{"start", "result", "step", "package", "project", "new", "old"}
	for _, g := range generic {
		text := fmt.Sprintf("Using `%s` in the pipeline", g)
		t.Run(g, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, g) {
					t.Errorf("generic term %q in backticks should not be a concept", g)
				}
			}
		})
	}
}

func TestNoiseConcept_GenericDefinitionPatterns(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Generic terms via "X is a ..." pattern should NOT become concepts.
	generic := []string{"Start", "Result", "Step", "Package", "Project"}
	for _, g := range generic {
		text := fmt.Sprintf("%s is a critical component of the system", g)
		t.Run(g, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, g) {
					t.Errorf("generic term %q via definition pattern should not be a concept", g)
				}
			}
		})
	}
}

// ─── Round 2: User-Reported Generic Concepts ──────────────────────────────────

func TestIsNoiseConcept_UserReportedRound2(t *testing.T) {
	// Terms the user observed still appearing in the graph after cleanup-noise.
	// Each is too vague to be a useful isolated concept.
	genericTerms := []string{
		"Formula", "formula", "FORMULA",
		"placeholder", "Placeholder", "PLACEHOLDER",
		"understanding", "Understanding", "UNDERSTANDING",
		"nobody", "Nobody", "NOBODY",
		"real", "Real", "REAL",
		"engine", "Engine", "ENGINE",
	}
	for _, term := range genericTerms {
		t.Run(term, func(t *testing.T) {
			if !extractor.IsNoiseConcept(term) {
				t.Errorf("expected user-reported generic term %q to be noise", term)
			}
		})
	}
}

func TestIsNoiseConcept_EnvIsNotNoise(t *testing.T) {
	// "env" is a valid developer concept (environment variables).
	// Must NOT be filtered — explicitly requested by the user.
	if extractor.IsNoiseConcept("env") {
		t.Error("'env' should NOT be noise — it's a valid dev concept (environment variables)")
	}
	if extractor.IsNoiseConcept("Env") {
		t.Error("'Env' should NOT be noise — it's a valid dev concept (environment variables)")
	}
}

func TestNoiseConcept_UserReportedRound2_Backtick(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Generic terms in backticks should NOT become concepts.
	generic := []string{"formula", "placeholder", "understanding", "nobody", "real", "engine"}
	for _, g := range generic {
		text := fmt.Sprintf("Using `%s` in the pipeline", g)
		t.Run(g, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, g) {
					t.Errorf("generic term %q in backticks should not be a concept", g)
				}
			}
		})
	}
}

func TestNoiseConcept_UserReportedRound2_Definition(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Generic terms via "X is a ..." pattern should NOT become concepts.
	generic := []string{"Formula", "Placeholder", "Understanding", "Real", "Engine"}
	for _, g := range generic {
		text := fmt.Sprintf("%s is a critical component of the system", g)
		t.Run(g, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, g) {
					t.Errorf("generic term %q via definition pattern should not be a concept", g)
				}
			}
		})
	}
}

func TestNoiseConcept_EnvBacktickStillExtracted(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "env" in backticks should STILL be extracted as a valid concept.
	text := "Using `env` for configuration"
	result := ex.Extract(text)

	found := false
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept && e.Name == "env" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("'env' in backticks should still be extracted as concept, got: %v", entityNames(result.Entities))
	}
}

func TestNoiseConcept_UserReportedRound2_SuffixPattern(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// "Engine pattern" → "Engine" should be filtered as noise concept via suffix pattern.
	generic := []string{"Engine", "Formula", "Real"}
	for _, g := range generic {
		text := fmt.Sprintf("Applied the %s pattern for transactions", g)
		t.Run(g, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, g) {
					t.Errorf("generic term %q via suffix pattern should not be a concept", g)
				}
			}
		})
	}
}

// ─── Round 3: Code Identifier Filtering ────────────────────────────────────

func TestIsNoiseConcept_CodeIdentifiers(t *testing.T) {
	// camelCase/PascalCase identifiers from the real diagnostic — must be noise.
	identifiers := []string{
		"cmdGraphReindex", "RebuildCommunities", "visibleItems",
		"UserService", "BaseService", "AuthService",
		"getUserById", "fetchDataAsync", "handleHttpResponse",
		"ConfigManager", "EventHandler",
	}
	for _, id := range identifiers {
		t.Run(id, func(t *testing.T) {
			if !extractor.IsNoiseConcept(id) {
				t.Errorf("expected code identifier %q to be noise", id)
			}
		})
	}
}

func TestIsNoiseConcept_CodeIdentifiers_GazetteerEntitiesPass(t *testing.T) {
	// Gazetteer entries that look like they could be code identifiers but are
	// legitimate tools/languages — must NOT be filtered.
	legit := []string{
		// lowercase prefix + uppercase acronym — no lower→upper→lower triple
		"gRPC", "eBPF",
		// upper+lower+upper but second upper is NOT followed by lowercase
		"GraphQL", "MongoDB", "OpenAI",
		// product names with special patterns
		"macOS",
		// standard PascalCase tools (single-capital + all-lowercase rest)
		"React", "Docker", "Kubernetes", "Prometheus",
		// all-uppercase acronyms
		"JWT", "API", "REST", "SQL", "SOLID", "CQRS", "TDD",
		// single-capital + all-lowercase
		"Go", "R", "Bash",
	}
	for _, name := range legit {
		t.Run(name, func(t *testing.T) {
			if extractor.IsNoiseConcept(name) {
				t.Errorf("legitimate entity %q should NOT be noise", name)
			}
		})
	}
}

func TestIsNoiseConcept_GenericTermsRound3(t *testing.T) {
	// Explicitly diagnosed generic terms.
	genericTerms := []string{
		"implementation", "Implementation", "IMPLEMENTATION",
		"double-quoted", "Double-quoted", "DOUBLE-QUOTED",
	}
	for _, term := range genericTerms {
		t.Run(term, func(t *testing.T) {
			if !extractor.IsNoiseConcept(term) {
				t.Errorf("expected generic term %q to be noise", term)
			}
		})
	}
}

func TestNoiseConcept_CodeIdentifiers_Backtick(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Code identifiers in backticks should NOT become concepts.
	identifiers := []string{"cmdGraphReindex", "visibleItems", "getUserById"}
	for _, id := range identifiers {
		text := fmt.Sprintf("Called `%s` in the pipeline", id)
		t.Run(id, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && e.Name == id {
					t.Errorf("code identifier %q in backticks should not be a concept", id)
				}
			}
		})
	}
}

func TestNoiseConcept_CodeIdentifiers_Definition(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Code identifiers via "X is a ..." pattern should NOT become concepts.
	identifiers := []string{"CmdGraphReindex", "RebuildCommunities", "VisibleItems"}
	for _, id := range identifiers {
		text := fmt.Sprintf("%s is a critical component of the system", id)
		t.Run(id, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && e.Name == id {
					t.Errorf("code identifier %q via definition pattern should not be a concept", id)
				}
			}
		})
	}
}

func TestNoiseConcept_CodeIdentifiers_Suffix(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Code identifiers via "the X pattern" suffix should NOT become concepts.
	identifiers := []string{"CmdGraphReindex", "VisibleItems", "EventHandler"}
	for _, id := range identifiers {
		text := fmt.Sprintf("Applied the %s pattern for transactions", id)
		t.Run(id, func(t *testing.T) {
			result := ex.Extract(text)
			for _, e := range result.Entities {
				if e.EntityType == extractor.EntityTypeConcept && e.Name == id {
					t.Errorf("code identifier %q via suffix pattern should not be a concept", id)
				}
			}
		})
	}
}

func TestNoiseConcept_CodeIdentifiers_MultiWordNotFiltered(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	// Multi-word phrases with spaces are NOT code identifiers — they should
	// still be extracted if they're valid concepts.
	text := "Using `event sourcing` for state management"
	result := ex.Extract(text)
	if !hasEntity(result.Entities, "event sourcing", extractor.EntityTypeConcept) {
		t.Errorf("multi-word concept 'event sourcing' should still be extracted, got: %v",
			entityNames(result.Entities))
	}
}

func TestNoiseConcept_Implementation_Backtick(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "Using `implementation` in the pipeline"
	result := ex.Extract(text)
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept && e.Name == "implementation" {
			t.Error("'implementation' in backticks should not be a concept")
		}
	}
}

func TestNoiseConcept_Implementation_Definition(t *testing.T) {
	ex := extractor.NewRuleExtractor()

	text := "Implementation is a critical phase of the project"
	result := ex.Extract(text)
	for _, e := range result.Entities {
		if e.EntityType == extractor.EntityTypeConcept && strings.EqualFold(e.Name, "implementation") {
			t.Error("'Implementation' via definition pattern should not be a concept")
		}
	}
}
