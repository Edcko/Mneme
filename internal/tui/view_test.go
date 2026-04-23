package tui

import (
	"strings"
	"testing"

	"github.com/Edcko/Mneme/internal/setup"
	"github.com/Edcko/Mneme/internal/store"
	"github.com/Edcko/Mneme/internal/version"
)

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{name: "unchanged", in: "short", max: 10, want: "short"},
		{name: "replaces newlines", in: "a\nb", max: 10, want: "a b"},
		{name: "truncated", in: "abcdefghijklmnopqrstuvwxyz", max: 5, want: "abcde..."},
		{name: "spanish accents", in: "Decisión de arquitectura", max: 8, want: "Decisión..."},
		{name: "emoji", in: "🐛🔧🚀✨🎉💡", max: 3, want: "🐛🔧🚀..."},
		{name: "mixed ascii and multibyte", in: "café☕latte", max: 5, want: "café☕..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateStr(tt.in, tt.max)
			if got != tt.want {
				t.Fatalf("truncateStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderObservationListItem(t *testing.T) {
	m := New(nil, "")
	m.Cursor = 1
	project := "engram"

	line := m.renderObservationListItem(
		1,
		42,
		"bugfix",
		"Title here",
		"content line 1\ncontent line 2",
		"2026-01-01",
		&project,
	)

	if !strings.Contains(line, "▸") {
		t.Fatal("selected item should include cursor marker")
	}
	if !strings.Contains(line, "Title here") {
		t.Fatal("line should include title")
	}
	if !strings.Contains(line, "content line 1 content line 2") {
		t.Fatal("content preview should be rendered on second line")
	}
	if !strings.Contains(line, "engram") {
		t.Fatal("project label should be rendered when project is set")
	}
}

func TestViewRouterAndErrorRendering(t *testing.T) {
	m := New(nil, "")
	m.Screen = Screen(999)
	m.ErrorMsg = "boom"

	out := m.View()
	if !strings.Contains(out, "Unknown screen") {
		t.Fatal("unknown screen fallback text missing")
	}
	if !strings.Contains(out, "Error: boom") {
		t.Fatal("error message should be appended to view")
	}
}

func TestViewSearchResultsAndScrollIndicator(t *testing.T) {
	m := New(nil, "")
	m.Screen = ScreenSearchResults
	m.Height = 14
	m.SearchQuery = "needle"
	m.SearchResults = []store.SearchResult{
		{Observation: store.Observation{ID: 1, Type: "bugfix", Title: "one", Content: "a", CreatedAt: "2026-01-01"}},
		{Observation: store.Observation{ID: 2, Type: "bugfix", Title: "two", Content: "b", CreatedAt: "2026-01-01"}},
		{Observation: store.Observation{ID: 3, Type: "bugfix", Title: "three", Content: "c", CreatedAt: "2026-01-01"}},
		{Observation: store.Observation{ID: 4, Type: "bugfix", Title: "four", Content: "d", CreatedAt: "2026-01-01"}},
	}

	out := m.viewSearchResults()
	if !strings.Contains(out, "Search: \"needle\"") {
		t.Fatal("search header missing")
	}
	if !strings.Contains(out, "showing 1-3 of 4") {
		t.Fatal("scroll indicator missing for overflowing list")
	}

	m.SearchResults = nil
	out = m.viewSearchResults()
	if !strings.Contains(out, "No memories found") {
		t.Fatal("empty result state missing")
	}
}

func TestViewSetupBranches(t *testing.T) {
	m := New(nil, "")
	m.Screen = ScreenSetup

	m.SetupInstalling = true
	m.SetupInstallingName = "opencode"
	out := m.viewSetup()
	if !strings.Contains(out, "Installing opencode plugin") {
		t.Fatal("installing state should render progress line")
	}

	m.SetupInstalling = false
	m.SetupDone = true
	m.SetupResult = &setup.Result{Agent: "opencode", Destination: "/tmp/plugins", Files: 2}
	out = m.viewSetup()
	if !strings.Contains(out, "Installed opencode plugin") {
		t.Fatal("success state should render install result")
	}
	if !strings.Contains(out, "Next Steps") {
		t.Fatal("success state should render post-install instructions")
	}

	m.SetupResult = nil
	m.SetupError = "permission denied"
	out = m.viewSetup()
	if !strings.Contains(out, "Installation failed") {
		t.Fatal("error state should render failure message")
	}
}

func TestViewDashboardSearchAndRecent(t *testing.T) {
	m := New(nil, "")
	m.Cursor = 1
	m.Stats = &store.Stats{
		TotalSessions:     3,
		TotalObservations: 7,
		TotalPrompts:      2,
		Projects:          []string{"a", "b", "c", "d", "e", "f"},
	}

	out := m.viewDashboard()
	if !strings.Contains(out, "mneme") || !strings.Contains(out, "Actions") {
		t.Fatal("dashboard should include header and actions")
	}
	if !strings.Contains(out, "...and 1 more projects") {
		t.Fatal("dashboard should show overflow projects indicator")
	}

	m.UpdateStatus = version.StatusUpdateAvailable
	m.UpdateMsg = "Update available: 1.10.7 -> 1.10.8"
	out = m.viewDashboard()
	if !strings.Contains(out, "Update available") {
		t.Fatal("dashboard should render update banner")
	}

	m.UpdateStatus = version.StatusCheckFailed
	m.UpdateMsg = "Could not check for updates: GitHub took too long to respond."
	out = m.viewDashboard()
	if !strings.Contains(out, "Could not check for updates") {
		t.Fatal("dashboard should render update failure banner")
	}

	m.Stats = nil
	out = m.viewDashboard()
	if !strings.Contains(out, "Loading stats") {
		t.Fatal("dashboard should render loading state when stats are nil")
	}

	m.Screen = ScreenSearch
	out = m.viewSearch()
	if !strings.Contains(out, "Search Memories") {
		t.Fatal("search view should render title")
	}

	m.Height = 14
	m.RecentObservations = []store.Observation{
		{ID: 1, Type: "bugfix", Title: "one", Content: "a", CreatedAt: "2026-01-01"},
		{ID: 2, Type: "bugfix", Title: "two", Content: "b", CreatedAt: "2026-01-01"},
		{ID: 3, Type: "bugfix", Title: "three", Content: "c", CreatedAt: "2026-01-01"},
		{ID: 4, Type: "bugfix", Title: "four", Content: "d", CreatedAt: "2026-01-01"},
	}
	out = m.viewRecent()
	if !strings.Contains(out, "Recent Observations") {
		t.Fatal("recent view should render title")
	}
	if !strings.Contains(out, "showing 1-3 of 4") {
		t.Fatal("recent view should render scroll indicator when needed")
	}

	m.RecentObservations = nil
	out = m.viewRecent()
	if !strings.Contains(out, "No observations yet") {
		t.Fatal("recent view should render empty state")
	}

	// Force minimum visible items branch
	m.Height = 8
	m.RecentObservations = []store.Observation{{ID: 1, Type: "bugfix", Title: "one", Content: "a", CreatedAt: "2026-01-01"}}
	out = m.viewRecent()
	if !strings.Contains(out, "Recent Observations") {
		t.Fatal("recent view should still render when height is very small")
	}
}

func TestViewObservationDetailTimelineSessionsAndSessionDetail(t *testing.T) {
	m := New(nil, "")
	m.Height = 22

	out := m.viewObservationDetail()
	if !strings.Contains(out, "Loading") {
		t.Fatal("detail view should render loading state when observation is nil")
	}

	tool := "bash"
	project := "engram"
	m.SelectedObservation = &store.Observation{
		ID:        42,
		Type:      "decision",
		Title:     "Architecture decision",
		SessionID: "session-1",
		CreatedAt: "2026-01-01",
		ToolName:  &tool,
		Project:   &project,
		Content:   strings.Repeat("line\n", 20),
	}
	m.DetailScroll = 99
	out = m.viewObservationDetail()
	if !strings.Contains(out, "Observation #42") || !strings.Contains(out, "Content") {
		t.Fatal("detail view should render metadata and content section")
	}
	if !strings.Contains(out, "line") {
		t.Fatal("detail view should render content lines")
	}

	out = m.viewTimeline()
	if !strings.Contains(out, "Loading") {
		t.Fatal("timeline should render loading state when nil")
	}

	m.Timeline = &store.TimelineResult{
		Focus:        store.Observation{ID: 42, Type: "decision", Title: "focus", Content: "focus content"},
		Before:       []store.TimelineEntry{{ID: 40, Type: "bugfix", Title: "before title"}},
		After:        []store.TimelineEntry{{ID: 43, Type: "pattern", Title: "after title"}},
		SessionInfo:  &store.Session{ID: "session-1", Project: "engram"},
		TotalInRange: 3,
	}
	out = m.viewTimeline()
	if !strings.Contains(out, "Timeline") || !strings.Contains(out, "Before") || !strings.Contains(out, "After") {
		t.Fatal("timeline should render focus and before/after sections")
	}

	m.Sessions = nil
	out = m.viewSessions()
	if !strings.Contains(out, "No sessions yet") {
		t.Fatal("sessions view should render empty state")
	}

	summary := "session summary"
	m.Height = 14
	m.Sessions = []store.SessionSummary{
		{ID: "s1", Project: "engram", StartedAt: "2026-01-01", Summary: &summary, ObservationCount: 2},
		{ID: "s2", Project: "engram", StartedAt: "2026-01-02", ObservationCount: 1},
		{ID: "s3", Project: "engram", StartedAt: "2026-01-03", ObservationCount: 1},
		{ID: "s4", Project: "engram", StartedAt: "2026-01-04", ObservationCount: 1},
		{ID: "s5", Project: "engram", StartedAt: "2026-01-05", ObservationCount: 1},
		{ID: "s6", Project: "engram", StartedAt: "2026-01-06", ObservationCount: 1},
		{ID: "s7", Project: "engram", StartedAt: "2026-01-07", ObservationCount: 1},
	}
	out = m.viewSessions()
	if !strings.Contains(out, "Sessions") || !strings.Contains(out, "showing 1-6 of 7") {
		t.Fatal("sessions view should render list and scroll indicator")
	}

	// Force minimum visible items branch
	m.Height = 2
	out = m.viewSessions()
	if !strings.Contains(out, "Sessions") {
		t.Fatal("sessions view should render when height is very small")
	}

	m.SelectedSessionIdx = 99
	out = m.viewSessionDetail()
	if !strings.Contains(out, "Session not found") {
		t.Fatal("session detail should guard invalid index")
	}

	m.SelectedSessionIdx = 0
	m.SessionObservations = nil
	out = m.viewSessionDetail()
	if !strings.Contains(out, "No observations in this session") {
		t.Fatal("session detail should render empty observations state")
	}

	m.Height = 16
	m.SessionObservations = []store.Observation{
		{ID: 1, Type: "bugfix", Title: "one", Content: "a", CreatedAt: "2026-01-01"},
		{ID: 2, Type: "bugfix", Title: "two", Content: "b", CreatedAt: "2026-01-01"},
		{ID: 3, Type: "bugfix", Title: "three", Content: "c", CreatedAt: "2026-01-01"},
		{ID: 4, Type: "bugfix", Title: "four", Content: "d", CreatedAt: "2026-01-01"},
	}
	out = m.viewSessionDetail()
	if !strings.Contains(out, "Observations (4)") {
		t.Fatal("session detail should show observations heading")
	}
}

func TestViewRouterCoversAllScreens(t *testing.T) {
	m := New(nil, "")
	m.Stats = &store.Stats{}
	m.SearchResults = []store.SearchResult{{Observation: store.Observation{ID: 1, Type: "bugfix", Title: "t", Content: "c", CreatedAt: "now"}}}
	m.SearchQuery = "q"
	m.RecentObservations = []store.Observation{{ID: 1, Type: "bugfix", Title: "t", Content: "c", CreatedAt: "now"}}
	m.SelectedObservation = &store.Observation{ID: 1, Type: "bugfix", Title: "t", Content: "c", CreatedAt: "now", SessionID: "s1"}
	m.Timeline = &store.TimelineResult{Focus: store.Observation{ID: 1, Type: "bugfix", Title: "t", Content: "c"}, TotalInRange: 1}
	m.Sessions = []store.SessionSummary{{ID: "s1", Project: "engram", StartedAt: "now", ObservationCount: 1}}
	m.SelectedSessionIdx = 0
	m.SessionObservations = []store.Observation{{ID: 1, Type: "bugfix", Title: "t", Content: "c", CreatedAt: "now"}}
	m.SetupAgents = []setup.Agent{{Name: "opencode", Description: "OpenCode", InstallDir: "/tmp"}}
	m.GraphEntities = []store.Entity{{ID: 1, Name: "React", EntityType: store.EntityTypeTool, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"}}
	m.SelectedEntity = &store.Entity{ID: 1, Name: "React", EntityType: store.EntityTypeTool, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"}
	m.Height = 20

	tests := []struct {
		screen Screen
		want   string
	}{
		{screen: ScreenDashboard, want: "Actions"},
		{screen: ScreenSearch, want: "Search Memories"},
		{screen: ScreenSearchResults, want: "Search:"},
		{screen: ScreenRecent, want: "Recent Observations"},
		{screen: ScreenObservationDetail, want: "Observation #"},
		{screen: ScreenTimeline, want: "Timeline"},
		{screen: ScreenSessions, want: "Sessions"},
		{screen: ScreenSessionDetail, want: "Session:"},
		{screen: ScreenSetup, want: "Setup"},
		{screen: ScreenGraph, want: "Knowledge Graph"},
		{screen: ScreenGraphDetail, want: "Entity #"},
		{screen: ScreenGraphSearch, want: "Search Knowledge Graph"},
	}

	for _, tt := range tests {
		m.Screen = tt.screen
		out := m.View()
		if !strings.Contains(out, tt.want) {
			t.Fatalf("screen %v output missing %q", tt.screen, tt.want)
		}
	}
}

func TestViewSetupRemainingBranches(t *testing.T) {
	m := New(nil, "")
	m.Screen = ScreenSetup
	m.SetupAgents = []setup.Agent{
		{Name: "claude-code", Description: "Claude Code", InstallDir: "/tmp/claude"},
		{Name: "opencode", Description: "OpenCode", InstallDir: "/tmp/opencode"},
	}

	out := m.viewSetup()
	if !strings.Contains(out, "Select an agent to set up") || !strings.Contains(out, "Install to") {
		t.Fatal("setup selection mode should render options and install paths")
	}

	m.SetupInstalling = true
	m.SetupInstallingName = "claude-code"
	out = m.viewSetup()
	if !strings.Contains(out, "Running claude plugin marketplace add + install") {
		t.Fatal("setup installing should render claude-code specific progress text")
	}

	m.SetupInstalling = false
	m.SetupDone = true
	m.SetupError = ""
	m.SetupResult = &setup.Result{Agent: "claude-code", Destination: "/tmp/claude", Files: 0}
	out = m.viewSetup()
	if !strings.Contains(out, "Verify with: claude plugin list") {
		t.Fatal("setup success for claude-code should render next steps")
	}

	m.SetupResult = nil
	m.SetupError = ""
	out = m.viewSetup()
	if !strings.Contains(out, "enter/esc back to dashboard") {
		t.Fatal("setup done without result/error should still render return help")
	}
}

func TestViewSetupAllowlistPrompt(t *testing.T) {
	t.Run("renders allowlist prompt", func(t *testing.T) {
		m := New(nil, "")
		m.Screen = ScreenSetup
		m.SetupAllowlistPrompt = true
		m.SetupResult = &setup.Result{Agent: "claude-code", Destination: "claude plugin system"}

		out := m.viewSetup()
		if !strings.Contains(out, "Installed claude-code plugin") {
			t.Fatal("prompt should show install success")
		}
		if !strings.Contains(out, "Permissions Allowlist") {
			t.Fatal("prompt should show allowlist heading")
		}
		if !strings.Contains(out, "settings.json") {
			t.Fatal("prompt should mention settings.json")
		}
		if !strings.Contains(out, "[y] Yes") || !strings.Contains(out, "[n] No") {
			t.Fatal("prompt should show y/n options")
		}
	})

	t.Run("renders applied state", func(t *testing.T) {
		m := New(nil, "")
		m.Screen = ScreenSetup
		m.SetupDone = true
		m.SetupResult = &setup.Result{Agent: "claude-code", Destination: "claude plugin system"}
		m.SetupAllowlistApplied = true

		out := m.viewSetup()
		if !strings.Contains(out, "tools added to allowlist") {
			t.Fatal("should show allowlist success")
		}
	})

	t.Run("renders error state", func(t *testing.T) {
		m := New(nil, "")
		m.Screen = ScreenSetup
		m.SetupDone = true
		m.SetupResult = &setup.Result{Agent: "claude-code", Destination: "claude plugin system"}
		m.SetupAllowlistError = "permission denied"

		out := m.viewSetup()
		if !strings.Contains(out, "Allowlist update failed") {
			t.Fatal("should show allowlist error")
		}
		if !strings.Contains(out, "permission denied") {
			t.Fatal("should show error message")
		}
	})
}

// ─── Graph View Tests ─────────────────────────────────────────────────────────

func TestViewGraphEntityList(t *testing.T) {
	m := New(nil, "")
	m.Height = 14

	t.Run("empty state", func(t *testing.T) {
		m.GraphEntities = nil
		m.GraphSearchQuery = ""
		out := m.viewGraph()
		if !strings.Contains(out, "Knowledge Graph") {
			t.Fatal("graph view should render title")
		}
		if !strings.Contains(out, "No entities yet") {
			t.Fatal("graph view should render empty state")
		}
	})

	t.Run("empty search results", func(t *testing.T) {
		m.GraphEntities = nil
		m.GraphSearchQuery = "nonexistent"
		out := m.viewGraph()
		if !strings.Contains(out, "No entities found") {
			t.Fatal("graph search should render no results state")
		}
	})

	t.Run("entity list with items", func(t *testing.T) {
		m.GraphEntities = []store.Entity{
			{ID: 1, Name: "React", EntityType: store.EntityTypeTool, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 2, Name: "TypeScript", EntityType: store.EntityTypeLanguage, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
		}
		m.GraphSearchQuery = ""
		out := m.viewGraph()
		if !strings.Contains(out, "React") || !strings.Contains(out, "TypeScript") {
			t.Fatal("graph view should list entity names")
		}
	})

	t.Run("entities sorted by type then name", func(t *testing.T) {
		m.GraphEntities = []store.Entity{
			{ID: 1, Name: "React", EntityType: store.EntityTypeTool, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 2, Name: "Alice", EntityType: store.EntityTypePerson, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 3, Name: "Go", EntityType: store.EntityTypeLanguage, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 4, Name: "TypeScript", EntityType: store.EntityTypeLanguage, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 5, Name: "Angular", EntityType: store.EntityTypeTool, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
		}
		m.GraphSearchQuery = ""
		out := m.viewGraph()

		// Order should be: language (Go, TypeScript), person (Alice), tool (Angular, React).
		goIdx := strings.Index(out, "Go")
		tsIdx := strings.Index(out, "TypeScript")
		aliceIdx := strings.Index(out, "Alice")
		angularIdx := strings.Index(out, "Angular")
		reactIdx := strings.Index(out, "React")

		if goIdx >= tsIdx {
			t.Fatal("Go should appear before TypeScript (same type, alphabetical)")
		}
		if tsIdx >= aliceIdx {
			t.Fatal("TypeScript (language) should appear before Alice (person)")
		}
		if aliceIdx >= angularIdx {
			t.Fatal("Alice (person) should appear before Angular (tool)")
		}
		if angularIdx >= reactIdx {
			t.Fatal("Angular should appear before React (same type, alphabetical)")
		}
	})

	t.Run("search header", func(t *testing.T) {
		m.GraphSearchQuery = "react"
		m.GraphEntities = []store.Entity{
			{ID: 1, Name: "React", EntityType: store.EntityTypeTool, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
		}
		out := m.viewGraph()
		if !strings.Contains(out, `Graph Search: "react"`) {
			t.Fatal("graph search view should show query in header")
		}
	})

	t.Run("scroll indicator", func(t *testing.T) {
		m.Height = 12 // visibleItems = 12-8 = 4
		m.GraphSearchQuery = ""
		m.GraphEntities = []store.Entity{
			{ID: 1, Name: "A", EntityType: store.EntityTypeConcept, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 2, Name: "B", EntityType: store.EntityTypeConcept, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 3, Name: "C", EntityType: store.EntityTypeConcept, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 4, Name: "D", EntityType: store.EntityTypeConcept, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 5, Name: "E", EntityType: store.EntityTypeConcept, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
			{ID: 6, Name: "F", EntityType: store.EntityTypeConcept, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"},
		}
		out := m.viewGraph()
		if !strings.Contains(out, "showing") {
			t.Fatal("graph view should show scroll indicator when items overflow")
		}
	})
}

func TestViewGraphDetail(t *testing.T) {
	m := New(nil, "")
	m.Height = 20

	t.Run("loading state", func(t *testing.T) {
		m.SelectedEntity = nil
		out := m.viewGraphDetail()
		if !strings.Contains(out, "Loading") {
			t.Fatal("graph detail should render loading state when entity is nil")
		}
	})

	t.Run("entity detail with metadata", func(t *testing.T) {
		summary := "A JavaScript library for building UIs"
		project := "webapp"
		m.SelectedEntity = &store.Entity{
			ID:         1,
			Name:       "React",
			EntityType: store.EntityTypeTool,
			Summary:    &summary,
			Project:    &project,
			CreatedAt:  "2026-01-01 12:00:00",
			UpdatedAt:  "2026-01-02 12:00:00",
		}
		out := m.viewGraphDetail()
		if !strings.Contains(out, "Entity #1") {
			t.Fatal("graph detail should show entity ID")
		}
		if !strings.Contains(out, "React") {
			t.Fatal("graph detail should show entity name")
		}
		if !strings.Contains(out, "tool") {
			t.Fatal("graph detail should show entity type")
		}
		if !strings.Contains(out, "JavaScript library") {
			t.Fatal("graph detail should show summary")
		}
		if !strings.Contains(out, "webapp") {
			t.Fatal("graph detail should show project")
		}
	})

	t.Run("entity detail without relations", func(t *testing.T) {
		m.SelectedEntity = &store.Entity{ID: 2, Name: "Go", EntityType: store.EntityTypeLanguage, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"}
		m.EntityRelations = nil
		out := m.viewGraphDetail()
		if !strings.Contains(out, "Relations (0)") {
			t.Fatal("graph detail should show zero relations")
		}
		if !strings.Contains(out, "No active relations") {
			t.Fatal("graph detail should show no relations message")
		}
	})

	t.Run("entity detail with relations", func(t *testing.T) {
		m.SelectedEntity = &store.Entity{ID: 1, Name: "React", EntityType: store.EntityTypeTool, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"}
		m.EntityRelations = []store.Relation{
			{ID: 1, SourceID: 1, SourceName: "React", Relation: "uses", TargetID: 2, TargetName: "TypeScript"},
			{ID: 2, SourceID: 3, SourceName: "Alice", Relation: "works_with", TargetID: 1, TargetName: "React"},
		}
		out := m.viewGraphDetail()
		if !strings.Contains(out, "Relations (2)") {
			t.Fatal("graph detail should show relation count")
		}
		if !strings.Contains(out, "TypeScript") || !strings.Contains(out, "Alice") {
			t.Fatal("graph detail should show related entity names")
		}
		if !strings.Contains(out, "→") || !strings.Contains(out, "←") {
			t.Fatal("graph detail should show directional arrows")
		}
	})
}

func TestViewGraphSearch(t *testing.T) {
	m := New(nil, "")
	m.Screen = ScreenGraphSearch

	out := m.viewGraphSearch()
	if !strings.Contains(out, "Search Knowledge Graph") {
		t.Fatal("graph search should render title")
	}
	if !strings.Contains(out, "Type a query") {
		t.Fatal("graph search should show help text")
	}
}

func TestRenderEntityListItem(t *testing.T) {
	m := New(nil, "")
	m.Cursor = 0
	project := "webapp"
	summary := "A cool tool"

	e := store.Entity{
		ID:         42,
		Name:       "React",
		EntityType: store.EntityTypeTool,
		Summary:    &summary,
		Project:    &project,
		CreatedAt:  "2026-01-01",
		UpdatedAt:  "2026-01-01",
	}

	line := m.renderEntityListItem(0, e)
	if !strings.Contains(line, "▸") {
		t.Fatal("selected item should include cursor marker")
	}
	if !strings.Contains(line, "React") {
		t.Fatal("item should include entity name")
	}
	if !strings.Contains(line, "webapp") {
		t.Fatal("item should include project")
	}
	if !strings.Contains(line, "cool tool") {
		t.Fatal("item should include summary preview")
	}

	// Not selected
	line = m.renderEntityListItem(1, e)
	if strings.Contains(line, "▸") {
		t.Fatal("non-selected item should not include cursor marker")
	}
}
