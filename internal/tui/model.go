// Package tui implements the Bubbletea terminal UI for Mneme.
//
// Following the Gentleman Bubbletea patterns:
// - Screen constants as iota
// - Single Model struct holds ALL state
// - Update() with type switch
// - Per-screen key handlers returning (tea.Model, tea.Cmd)
// - Vim keys (j/k) for navigation
// - PrevScreen for back navigation
package tui

import (
	"github.com/Edcko/Mneme/internal/setup"
	"github.com/Edcko/Mneme/internal/store"
	"github.com/Edcko/Mneme/internal/version"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── Screens ─────────────────────────────────────────────────────────────────

type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenSearch
	ScreenSearchResults
	ScreenRecent
	ScreenObservationDetail
	ScreenTimeline
	ScreenSessions
	ScreenSessionDetail
	ScreenSetup
	ScreenGraph
	ScreenGraphDetail
	ScreenGraphSearch
)

// ─── Custom Messages ─────────────────────────────────────────────────────────

type updateCheckMsg struct {
	result version.CheckResult
}

type statsLoadedMsg struct {
	stats *store.Stats
	err   error
}

type searchResultsMsg struct {
	results []store.SearchResult
	query   string
	err     error
}

type recentObservationsMsg struct {
	observations []store.Observation
	err          error
}

type observationDetailMsg struct {
	observation *store.Observation
	err         error
}

type timelineMsg struct {
	timeline *store.TimelineResult
	err      error
}

type recentSessionsMsg struct {
	sessions []store.SessionSummary
	err      error
}

type sessionObservationsMsg struct {
	observations []store.Observation
	err          error
}

type entitiesLoadedMsg struct {
	entities []store.Entity
	err      error
}

type entityDetailLoadedMsg struct {
	entity    *store.Entity
	relations []store.Relation
	err       error
}

type graphSearchResultsMsg struct {
	entities []store.Entity
	query    string
	err      error
}

type setupInstallMsg struct {
	result *setup.Result
	err    error
}

// ─── Model ───────────────────────────────────────────────────────────────────

type Model struct {
	store      *store.Store
	Version    string
	Screen     Screen
	PrevScreen Screen
	Width      int
	Height     int
	Cursor     int
	Scroll     int

	// Update notification
	UpdateStatus version.CheckStatus
	UpdateMsg    string

	// Error display
	ErrorMsg string

	// Dashboard
	Stats *store.Stats

	// Search
	SearchInput   textinput.Model
	SearchQuery   string
	SearchResults []store.SearchResult

	// Recent observations
	RecentObservations []store.Observation

	// Observation detail
	SelectedObservation *store.Observation
	DetailScroll        int

	// Timeline
	Timeline *store.TimelineResult

	// Sessions
	Sessions            []store.SessionSummary
	SelectedSessionIdx  int
	SessionObservations []store.Observation
	SessionDetailScroll int

	// Setup
	SetupAgents           []setup.Agent
	SetupResult           *setup.Result
	SetupError            string
	SetupDone             bool
	SetupInstalling       bool
	SetupInstallingName   string // agent name being installed (for display)
	SetupAllowlistPrompt  bool   // true = showing y/n prompt for allowlist
	SetupAllowlistApplied bool   // true = allowlist was added successfully
	SetupAllowlistError   string // error message if allowlist injection failed
	SetupSpinner          spinner.Model

	// Graph — entity list
	GraphEntities []store.Entity

	// Graph — entity detail
	SelectedEntity     *store.Entity
	EntityRelations    []store.Relation
	EntityDetailScroll int

	// Graph — search
	GraphSearchInput textinput.Model
	GraphSearchQuery string
}

// New creates a new TUI model connected to the given store.
func New(s *store.Store, version string) Model {
	ti := textinput.New()
	ti.Placeholder = "Search memories..."
	ti.CharLimit = 256
	ti.Width = 60

	gsi := textinput.New()
	gsi.Placeholder = "Search entities..."
	gsi.CharLimit = 256
	gsi.Width = 60

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorLavender)

	return Model{
		store:            s,
		Version:          version,
		Screen:           ScreenDashboard,
		SearchInput:      ti,
		GraphSearchInput: gsi,
		SetupSpinner:     sp,
	}
}

// Init loads initial data (stats for the dashboard).
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadStats(m.store),
		checkForUpdate(m.Version),
		tea.EnterAltScreen,
	)
}

// ─── Commands (data loading) ─────────────────────────────────────────────────

func checkForUpdate(v string) tea.Cmd {
	return func() tea.Msg {
		return updateCheckMsg{result: version.CheckLatest(v)}
	}
}

func loadStats(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		stats, err := s.Stats()
		return statsLoadedMsg{stats: stats, err: err}
	}
}

func searchMemories(s *store.Store, query string) tea.Cmd {
	return func() tea.Msg {
		results, err := s.Search(query, store.SearchOptions{Limit: 50})
		return searchResultsMsg{results: results, query: query, err: err}
	}
}

func loadRecentObservations(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		obs, err := s.AllObservations("", "", 50)
		return recentObservationsMsg{observations: obs, err: err}
	}
}

func loadObservationDetail(s *store.Store, id int64) tea.Cmd {
	return func() tea.Msg {
		obs, err := s.GetObservation(id)
		return observationDetailMsg{observation: obs, err: err}
	}
}

func loadTimeline(s *store.Store, obsID int64) tea.Cmd {
	return func() tea.Msg {
		tl, err := s.Timeline(obsID, 10, 10)
		return timelineMsg{timeline: tl, err: err}
	}
}

func loadRecentSessions(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		sessions, err := s.AllSessions("", 50)
		return recentSessionsMsg{sessions: sessions, err: err}
	}
}

func loadSessionObservations(s *store.Store, sessionID string) tea.Cmd {
	return func() tea.Msg {
		obs, err := s.SessionObservations(sessionID, 200)
		return sessionObservationsMsg{observations: obs, err: err}
	}
}

func installAgent(agentName string) tea.Cmd {
	return func() tea.Msg {
		result, err := installAgentFn(agentName)
		return setupInstallMsg{result: result, err: err}
	}
}

var installAgentFn = setup.Install
var addClaudeCodeAllowlistFn = setup.AddClaudeCodeAllowlist

func loadEntities(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		entities, err := s.ListEntities("", "", 100)
		return entitiesLoadedMsg{entities: entities, err: err}
	}
}

func loadEntityDetail(s *store.Store, id int64) tea.Cmd {
	return func() tea.Msg {
		entity, err := s.GetEntityByID(id)
		if err != nil {
			return entityDetailLoadedMsg{err: err}
		}
		relations, err := s.GetEntityRelations(id, true)
		if err != nil {
			return entityDetailLoadedMsg{err: err}
		}
		return entityDetailLoadedMsg{entity: entity, relations: relations, err: nil}
	}
}

func searchGraphEntities(s *store.Store, query string) tea.Cmd {
	return func() tea.Msg {
		entities, err := s.SearchEntities(query, "", "", 50)
		return graphSearchResultsMsg{entities: entities, query: query, err: err}
	}
}
