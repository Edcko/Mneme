package extractor_test

import (
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
