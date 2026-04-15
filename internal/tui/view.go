package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Edcko/Mneme/internal/store"
	"github.com/Edcko/Mneme/internal/version"
	"github.com/charmbracelet/lipgloss"
)

// ─── Logo ────────────────────────────────────────────────────────────────────

func renderLogo(version string) string {
	logoText := []string{
		`███████ ███    ██  ██████  ██████   █████  ███    ███ `,
		`██      ████   ██ ██       ██   ██ ██   ██ ████  ████ `,
		`█████   ██ ██  ██ ██   ███ ██████  ███████ ██ ████ ██ `,
		`██      ██  ██ ██ ██    ██ ██   ██ ██   ██ ██  ██  ██ `,
		`███████ ██   ████  ██████  ██   ██ ██   ██ ██      ██ `,
	}

	frameStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorOverlay).
		Padding(0, 1).
		MarginBottom(1)

	// Gradient colors for the rows
	colors := []lipgloss.Color{
		colorMauve,    // Top (Pinkish)
		colorLavender, // Middle-top
		colorBlue,     // Middle
		colorTeal,     // Middle-bottom
		colorGreen,    // Bottom (Cyan/Greenish)
	}

	accentStyle := lipgloss.NewStyle().Foreground(colorLavender).Bold(true)
	taglineStyle := lipgloss.NewStyle().Foreground(colorSubtext).Italic(true)

	var b strings.Builder

	// Header line inside box (Cyber-Elephant Terminal)
	b.WriteString(accentStyle.Render(" 🐘 SYSTEM ONLINE ") + strings.Repeat(" ", 32) + accentStyle.Render(" MEM: OK 100% ") + "\n\n")

	// Logo body with gradient (logoText and colors are always the same length)
	for i, line := range logoText {
		b.WriteString(" " + lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(line) + "\n")
	}
	b.WriteString("\n")

	// Footer inside box
	b.WriteString(taglineStyle.Render(" > mneme " + version + " — An elephant never forgets"))

	return frameStyle.Render(b.String()) + "\n"
}

// ─── View (main router) ─────────────────────────────────────────────────────

func (m Model) View() string {
	var content string

	switch m.Screen {
	case ScreenDashboard:
		content = m.viewDashboard()
	case ScreenSearch:
		content = m.viewSearch()
	case ScreenSearchResults:
		content = m.viewSearchResults()
	case ScreenRecent:
		content = m.viewRecent()
	case ScreenObservationDetail:
		content = m.viewObservationDetail()
	case ScreenTimeline:
		content = m.viewTimeline()
	case ScreenSessions:
		content = m.viewSessions()
	case ScreenSessionDetail:
		content = m.viewSessionDetail()
	case ScreenSetup:
		content = m.viewSetup()
	case ScreenGraph:
		content = m.viewGraph()
	case ScreenGraphDetail:
		content = m.viewGraphDetail()
	case ScreenGraphSearch:
		content = m.viewGraphSearch()
	default:
		content = "Unknown screen"
	}

	// Show error if present
	if m.ErrorMsg != "" {
		content += "\n" + errorStyle.Render("Error: "+m.ErrorMsg)
	}

	return appStyle.Render(content)
}

// ─── Dashboard ───────────────────────────────────────────────────────────────

func (m Model) viewDashboard() string {
	var b strings.Builder

	// Logo header
	b.WriteString(renderLogo(m.Version))
	b.WriteString("\n")

	// Update notification
	if m.UpdateMsg != "" {
		bannerStyle := updateBannerStyle
		if m.UpdateStatus == version.StatusCheckFailed {
			bannerStyle = errorStyle
		}
		b.WriteString(bannerStyle.Render(m.UpdateMsg))
		b.WriteString("\n\n")
	}

	// Stats card
	if m.Stats != nil {
		statsContent := fmt.Sprintf(
			"%s %s\n%s %s\n%s %s\n%s %s",
			statNumberStyle.Render(fmt.Sprintf("%d", m.Stats.TotalSessions)),
			statLabelStyle.Render("sessions"),
			statNumberStyle.Render(fmt.Sprintf("%d", m.Stats.TotalObservations)),
			statLabelStyle.Render("observations"),
			statNumberStyle.Render(fmt.Sprintf("%d", m.Stats.TotalPrompts)),
			statLabelStyle.Render("prompts"),
			statNumberStyle.Render(fmt.Sprintf("%d", len(m.Stats.Projects))),
			statLabelStyle.Render("projects"),
		)
		b.WriteString(statCardStyle.Render(statsContent))
		b.WriteString("\n")

		if len(m.Stats.Projects) > 0 {
			b.WriteString(titleStyle.Render("  Projects"))
			b.WriteString("\n")

			limit := 5
			for i, p := range m.Stats.Projects {
				if i >= limit {
					break
				}
				b.WriteString(listItemStyle.Render("• " + p))
				b.WriteString("\n")
			}

			if len(m.Stats.Projects) > limit {
				remaining := len(m.Stats.Projects) - limit
				b.WriteString(fmt.Sprintf("    %s\n", timestampStyle.Render(fmt.Sprintf("...and %d more projects", remaining))))
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString(statCardStyle.Render("Loading stats..."))
		b.WriteString("\n")
	}

	// Menu
	b.WriteString(titleStyle.Render("  Actions"))
	b.WriteString("\n")

	for i, item := range dashboardMenuItems {
		if i == m.Cursor {
			b.WriteString(menuSelectedStyle.Render("▸ " + item))
		} else {
			b.WriteString(menuItemStyle.Render("  " + item))
		}
		b.WriteString("\n")
	}

	// Help
	b.WriteString(helpStyle.Render("\n  j/k navigate • enter select • s search • q quit"))

	return b.String()
}

// ─── Search ──────────────────────────────────────────────────────────────────

func (m Model) viewSearch() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Search Memories"))
	b.WriteString("\n\n")

	b.WriteString(searchInputStyle.Render(m.SearchInput.View()))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("  Type a query and press enter • esc go back"))

	return b.String()
}

// ─── Search Results ──────────────────────────────────────────────────────────

func (m Model) viewSearchResults() string {
	var b strings.Builder

	resultCount := len(m.SearchResults)
	header := fmt.Sprintf("  Search: %q — %d result", m.SearchQuery, resultCount)
	if resultCount != 1 {
		header += "s"
	}
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	if resultCount == 0 {
		b.WriteString(noResultsStyle.Render("No memories found. Try a different query."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  / new search • esc back"))
		return b.String()
	}

	visibleItems := (m.Height - 10) / 2 // 2 lines per observation item
	if visibleItems < 3 {
		visibleItems = 3
	}

	end := m.Scroll + visibleItems
	if end > resultCount {
		end = resultCount
	}

	for i := m.Scroll; i < end; i++ {
		r := m.SearchResults[i]
		b.WriteString(m.renderObservationListItem(i, r.ID, r.Type, r.Title, r.Content, r.CreatedAt, r.Project))
	}

	// Scroll indicator
	if resultCount > visibleItems {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("showing %d-%d of %d", m.Scroll+1, end, resultCount))))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter detail • t timeline • / search • esc back"))

	return b.String()
}

// ─── Recent Observations ─────────────────────────────────────────────────────

func (m Model) viewRecent() string {
	var b strings.Builder

	count := len(m.RecentObservations)
	header := fmt.Sprintf("  Recent Observations — %d total", count)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	if count == 0 {
		b.WriteString(noResultsStyle.Render("No observations yet."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc back"))
		return b.String()
	}

	visibleItems := (m.Height - 8) / 2 // 2 lines per observation item
	if visibleItems < 3 {
		visibleItems = 3
	}

	end := m.Scroll + visibleItems
	if end > count {
		end = count
	}

	for i := m.Scroll; i < end; i++ {
		o := m.RecentObservations[i]
		b.WriteString(m.renderObservationListItem(i, o.ID, o.Type, o.Title, o.Content, o.CreatedAt, o.Project))
	}

	if count > visibleItems {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("showing %d-%d of %d", m.Scroll+1, end, count))))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter detail • t timeline • esc back"))

	return b.String()
}

// ─── Observation Detail ──────────────────────────────────────────────────────

func (m Model) viewObservationDetail() string {
	var b strings.Builder

	if m.SelectedObservation == nil {
		b.WriteString(headerStyle.Render("  Observation Detail"))
		b.WriteString("\n")
		b.WriteString(noResultsStyle.Render("Loading..."))
		return b.String()
	}

	obs := m.SelectedObservation

	header := fmt.Sprintf("  Observation #%d", obs.ID)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Metadata rows
	b.WriteString(fmt.Sprintf("%s %s\n",
		detailLabelStyle.Render("Type:"),
		typeBadgeStyle.Render(obs.Type)))

	b.WriteString(fmt.Sprintf("%s %s\n",
		detailLabelStyle.Render("Title:"),
		detailValueStyle.Bold(true).Render(obs.Title)))

	b.WriteString(fmt.Sprintf("%s %s\n",
		detailLabelStyle.Render("Session:"),
		idStyle.Render(obs.SessionID)))

	b.WriteString(fmt.Sprintf("%s %s\n",
		detailLabelStyle.Render("Created:"),
		timestampStyle.Render(localTime(obs.CreatedAt))))

	if obs.ToolName != nil {
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Tool:"),
			detailValueStyle.Render(*obs.ToolName)))
	}

	if obs.Project != nil {
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Project:"),
			projectStyle.Render(*obs.Project)))
	}

	// Content section
	b.WriteString("\n")
	b.WriteString(sectionHeadingStyle.Render("  Content"))
	b.WriteString("\n")

	// Wrap content based on terminal width
	wrapWidth := m.Width - 6 // basic padding
	if wrapWidth < 20 {
		wrapWidth = 20
	}
	wrappedContent := detailContentStyle.Width(wrapWidth).Render(obs.Content)

	// Split wrapped content into lines
	contentLines := strings.Split(wrappedContent, "\n")
	maxLines := m.Height - 16
	if maxLines < 5 {
		maxLines = 5
	}

	// Clamp scroll
	maxScroll := len(contentLines) - maxLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.DetailScroll > maxScroll {
		m.DetailScroll = maxScroll
	}

	end := m.DetailScroll + maxLines
	if end > len(contentLines) {
		end = len(contentLines)
	}

	for i := m.DetailScroll; i < end; i++ {
		b.WriteString(contentLines[i])
		b.WriteString("\n")
	}

	if len(contentLines) > maxLines {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("line %d-%d of %d", m.DetailScroll+1, end, len(contentLines)))))
	}

	b.WriteString(helpStyle.Render("\n  j/k scroll • t timeline • esc back"))

	return b.String()
}

// ─── Timeline ────────────────────────────────────────────────────────────────

func (m Model) viewTimeline() string {
	var b strings.Builder

	if m.Timeline == nil {
		b.WriteString(headerStyle.Render("  Timeline"))
		b.WriteString("\n")
		b.WriteString(noResultsStyle.Render("Loading..."))
		return b.String()
	}

	tl := m.Timeline
	header := fmt.Sprintf("  Timeline — Observation #%d (%d total in session)", tl.Focus.ID, tl.TotalInRange)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Session info
	if tl.SessionInfo != nil {
		b.WriteString(fmt.Sprintf("  %s %s  %s %s\n\n",
			detailLabelStyle.Render("Session:"),
			idStyle.Render(tl.SessionInfo.ID),
			detailLabelStyle.Render("Project:"),
			projectStyle.Render(tl.SessionInfo.Project)))
	}

	// Before entries
	if len(tl.Before) > 0 {
		b.WriteString(sectionHeadingStyle.Render("  Before"))
		b.WriteString("\n")
		for _, e := range tl.Before {
			b.WriteString(fmt.Sprintf("  %s %s %s  %s\n",
				timelineConnectorStyle.Render("│"),
				idStyle.Render(fmt.Sprintf("#%-4d", e.ID)),
				typeBadgeStyle.Render(fmt.Sprintf("[%-12s]", e.Type)),
				timelineItemStyle.Render(truncateStr(e.Title, 60))))
		}
		b.WriteString(fmt.Sprintf("  %s\n", timelineConnectorStyle.Render("│")))
	}

	// Focus (highlighted)
	focusContent := fmt.Sprintf("  %s %s  %s\n  %s",
		idStyle.Render(fmt.Sprintf("#%d", tl.Focus.ID)),
		typeBadgeStyle.Render("["+tl.Focus.Type+"]"),
		lipgloss.NewStyle().Bold(true).Foreground(colorLavender).Render(tl.Focus.Title),
		detailContentStyle.Render(truncateStr(tl.Focus.Content, 120)))
	b.WriteString(timelineFocusStyle.Render(focusContent))
	b.WriteString("\n")

	// After entries
	if len(tl.After) > 0 {
		b.WriteString(fmt.Sprintf("  %s\n", timelineConnectorStyle.Render("│")))
		b.WriteString(sectionHeadingStyle.Render("  After"))
		b.WriteString("\n")
		for _, e := range tl.After {
			b.WriteString(fmt.Sprintf("  %s %s %s  %s\n",
				timelineConnectorStyle.Render("│"),
				idStyle.Render(fmt.Sprintf("#%-4d", e.ID)),
				typeBadgeStyle.Render(fmt.Sprintf("[%-12s]", e.Type)),
				timelineItemStyle.Render(truncateStr(e.Title, 60))))
		}
	}

	b.WriteString(helpStyle.Render("\n  j/k scroll • esc back"))

	return b.String()
}

// ─── Sessions ────────────────────────────────────────────────────────────────

func (m Model) viewSessions() string {
	var b strings.Builder

	count := len(m.Sessions)
	header := fmt.Sprintf("  Sessions — %d total", count)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	if count == 0 {
		b.WriteString(noResultsStyle.Render("No sessions yet."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc back"))
		return b.String()
	}

	visibleItems := m.Height - 8
	if visibleItems < 5 {
		visibleItems = 5
	}

	end := m.Scroll + visibleItems
	if end > count {
		end = count
	}

	for i := m.Scroll; i < end; i++ {
		s := m.Sessions[i]
		cursor := "  "
		style := listItemStyle
		if i == m.Cursor {
			cursor = "▸ "
			style = listSelectedStyle
		}

		summary := ""
		if s.Summary != nil {
			summary = truncateStr(*s.Summary, 50)
		}

		line := fmt.Sprintf("%s%s  %s  %s obs  %s",
			cursor,
			projectStyle.Render(fmt.Sprintf("%-20s", s.Project)),
			timestampStyle.Render(localTime(s.StartedAt)),
			statNumberStyle.Render(fmt.Sprintf("%d", s.ObservationCount)),
			style.Render(summary))

		b.WriteString(line)
		b.WriteString("\n")
	}

	if count > visibleItems {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("showing %d-%d of %d", m.Scroll+1, end, count))))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter view session • esc back"))

	return b.String()
}

// ─── Session Detail ──────────────────────────────────────────────────────────

func (m Model) viewSessionDetail() string {
	var b strings.Builder

	if m.SelectedSessionIdx >= len(m.Sessions) {
		b.WriteString(headerStyle.Render("  Session Detail"))
		b.WriteString("\n")
		b.WriteString(noResultsStyle.Render("Session not found."))
		return b.String()
	}

	sess := m.Sessions[m.SelectedSessionIdx]
	header := fmt.Sprintf("  Session: %s — %s", sess.Project, localTime(sess.StartedAt))
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Session metadata
	if sess.Summary != nil {
		b.WriteString(fmt.Sprintf("  %s %s\n\n",
			detailLabelStyle.Render("Summary:"),
			detailValueStyle.Render(*sess.Summary)))
	}

	count := len(m.SessionObservations)
	b.WriteString(sectionHeadingStyle.Render(fmt.Sprintf("  Observations (%d)", count)))
	b.WriteString("\n")

	if count == 0 {
		b.WriteString(noResultsStyle.Render("No observations in this session."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc back"))
		return b.String()
	}

	visibleItems := (m.Height - 12) / 2 // 2 lines per observation item
	if visibleItems < 3 {
		visibleItems = 3
	}

	end := m.SessionDetailScroll + visibleItems
	if end > count {
		end = count
	}

	for i := m.SessionDetailScroll; i < end; i++ {
		o := m.SessionObservations[i]
		b.WriteString(m.renderObservationListItem(i, o.ID, o.Type, o.Title, o.Content, o.CreatedAt, o.Project))
	}

	if count > visibleItems {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("showing %d-%d of %d", m.SessionDetailScroll+1, end, count))))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter detail • t timeline • esc back"))

	return b.String()
}

// ─── Setup ───────────────────────────────────────────────────────────────────

func (m Model) viewSetup() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Setup — Install Agent Plugin"))
	b.WriteString("\n")

	// Show spinner while installing
	if m.SetupInstalling {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s Installing %s plugin...\n",
			m.SetupSpinner.View(),
			lipgloss.NewStyle().Bold(true).Foreground(colorLavender).Render(m.SetupInstallingName)))
		b.WriteString("\n")

		switch m.SetupInstallingName {
		case "opencode":
			b.WriteString(timestampStyle.Render("  Copying plugin file to plugins directory"))
		case "claude-code":
			b.WriteString(timestampStyle.Render("  Running claude plugin marketplace add + install"))
		}

		b.WriteString("\n")
		return b.String()
	}

	// Show allowlist prompt after successful claude-code install
	if m.SetupAllowlistPrompt && m.SetupResult != nil {
		successMsg := fmt.Sprintf("Installed %s plugin", m.SetupResult.Agent)
		b.WriteString(fmt.Sprintf("\n  %s %s\n\n",
			lipgloss.NewStyle().Bold(true).Foreground(colorGreen).Render("✓"),
			lipgloss.NewStyle().Bold(true).Foreground(colorGreen).Render(successMsg)))

		b.WriteString(sectionHeadingStyle.Render("  Permissions Allowlist"))
		b.WriteString("\n\n")
		b.WriteString(detailContentStyle.Render("  Add mneme tools to ~/.claude/settings.json allowlist?"))
		b.WriteString("\n")
		b.WriteString(timestampStyle.Render("  This prevents Claude Code from asking permission on every tool call."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  [y] Yes  [n] No"))
		return b.String()
	}

	// Show result after install
	if m.SetupDone {
		if m.SetupError != "" {
			b.WriteString(errorStyle.Render("  ✗ Installation failed: " + m.SetupError))
			b.WriteString("\n\n")
		} else if m.SetupResult != nil {
			successMsg := fmt.Sprintf("Installed %s plugin", m.SetupResult.Agent)
			if m.SetupResult.Files > 0 {
				successMsg += fmt.Sprintf(" (%d files)", m.SetupResult.Files)
			}
			b.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Bold(true).Foreground(colorGreen).Render("✓"),
				lipgloss.NewStyle().Bold(true).Foreground(colorGreen).Render(successMsg)))
			b.WriteString(fmt.Sprintf("  %s %s\n\n",
				detailLabelStyle.Render("Location:"),
				projectStyle.Render(m.SetupResult.Destination)))

			// Post-install instructions
			switch m.SetupResult.Agent {
			case "opencode":
				b.WriteString(sectionHeadingStyle.Render("  Next Steps"))
				b.WriteString("\n")
				b.WriteString(detailContentStyle.Render("1. Restart OpenCode"))
				b.WriteString("\n")
				b.WriteString(detailContentStyle.Render("2. Plugin is auto-loaded from ~/.config/opencode/plugins/"))
				b.WriteString("\n")
				b.WriteString(detailContentStyle.Render("3. Make sure 'mneme' is in your MCP config (opencode.json)"))
				b.WriteString("\n")
			case "claude-code":
				b.WriteString(sectionHeadingStyle.Render("  Next Steps"))
				b.WriteString("\n")
				if m.SetupAllowlistApplied {
					b.WriteString(fmt.Sprintf("  %s %s\n",
						lipgloss.NewStyle().Bold(true).Foreground(colorGreen).Render("✓"),
						detailContentStyle.Render("Mneme tools added to allowlist")))
				} else if m.SetupAllowlistError != "" {
					b.WriteString(fmt.Sprintf("  %s %s\n",
						lipgloss.NewStyle().Bold(true).Foreground(colorRed).Render("✗"),
						detailContentStyle.Render("Allowlist update failed: "+m.SetupAllowlistError)))
					b.WriteString(detailContentStyle.Render("  Add manually to permissions.allow in ~/.claude/settings.json"))
					b.WriteString("\n")
				}
				b.WriteString(detailContentStyle.Render("1. Restart Claude Code — the plugin is active immediately"))
				b.WriteString("\n")
				b.WriteString(detailContentStyle.Render("2. Verify with: claude plugin list"))
				b.WriteString("\n")
			}
		}

		b.WriteString(helpStyle.Render("\n  enter/esc back to dashboard"))
		return b.String()
	}

	// Agent selection
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  Select an agent to set up"))
	b.WriteString("\n\n")

	for i, agent := range m.SetupAgents {
		if i == m.Cursor {
			b.WriteString(menuSelectedStyle.Render("▸ " + agent.Description))
		} else {
			b.WriteString(menuItemStyle.Render("  " + agent.Description))
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("      %s %s\n\n",
			detailLabelStyle.Render("Install to:"),
			timestampStyle.Render(agent.InstallDir)))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter install • esc back"))

	return b.String()
}

// ─── Graph — Entity List ─────────────────────────────────────────────────────

func (m Model) viewGraph() string {
	var b strings.Builder

	count := len(m.GraphEntities)
	header := "  Knowledge Graph"
	if m.GraphSearchQuery != "" {
		header = fmt.Sprintf("  Graph Search: %q — %d result", m.GraphSearchQuery, count)
		if count != 1 {
			header += "s"
		}
	} else {
		header = fmt.Sprintf("  Knowledge Graph — %d %s", count, pluralize(count, "entity", "entities"))
	}
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	if count == 0 {
		if m.GraphSearchQuery != "" {
			b.WriteString(noResultsStyle.Render("No entities found. Try a different query."))
		} else {
			b.WriteString(noResultsStyle.Render("No entities yet. They appear as you save observations."))
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  / search • esc back"))
		return b.String()
	}

	visibleItems := m.Height - 8
	if visibleItems < 3 {
		visibleItems = 3
	}

	end := m.Scroll + visibleItems
	if end > count {
		end = count
	}

	for i := m.Scroll; i < end; i++ {
		e := m.GraphEntities[i]
		b.WriteString(m.renderEntityListItem(i, e))
	}

	if count > visibleItems {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("showing %d-%d of %d", m.Scroll+1, end, count))))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter detail • / search • esc back"))

	return b.String()
}

// ─── Graph — Entity Detail ───────────────────────────────────────────────────

func (m Model) viewGraphDetail() string {
	var b strings.Builder

	if m.SelectedEntity == nil {
		b.WriteString(headerStyle.Render("  Entity Detail"))
		b.WriteString("\n")
		b.WriteString(noResultsStyle.Render("Loading..."))
		return b.String()
	}

	e := m.SelectedEntity

	b.WriteString(headerStyle.Render(fmt.Sprintf("  Entity #%d", e.ID)))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("%s %s\n",
		detailLabelStyle.Render("Name:"),
		detailValueStyle.Bold(true).Render(e.Name)))

	b.WriteString(fmt.Sprintf("%s %s\n",
		detailLabelStyle.Render("Type:"),
		entityTypeBadgeStyle.Render(string(e.EntityType))))

	if e.Summary != nil {
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Summary:"),
			detailValueStyle.Render(*e.Summary)))
	}

	if e.Project != nil {
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Project:"),
			projectStyle.Render(*e.Project)))
	}

	b.WriteString(fmt.Sprintf("%s %s\n",
		detailLabelStyle.Render("Created:"),
		timestampStyle.Render(localTime(e.CreatedAt))))

	// Relations section
	relCount := len(m.EntityRelations)
	b.WriteString("\n")
	b.WriteString(sectionHeadingStyle.Render(fmt.Sprintf("  Relations (%d)", relCount)))
	b.WriteString("\n")

	if relCount == 0 {
		b.WriteString(noResultsStyle.Render("No active relations for this entity."))
		b.WriteString("\n")
	} else {
		maxRels := m.Height - 14
		if maxRels < 3 {
			maxRels = 3
		}

		maxScroll := relCount - maxRels
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.EntityDetailScroll > maxScroll {
			m.EntityDetailScroll = maxScroll
		}

		end := m.EntityDetailScroll + maxRels
		if end > relCount {
			end = relCount
		}

		for i := m.EntityDetailScroll; i < end; i++ {
			r := m.EntityRelations[i]
			isSource := r.SourceID == e.ID
			var arrow, target string
			if isSource {
				arrow = "→"
				target = r.TargetName
			} else {
				arrow = "←"
				target = r.SourceName
			}
			b.WriteString(fmt.Sprintf("  %s %s %s %s\n",
				timelineConnectorStyle.Render("│"),
				detailValueStyle.Bold(true).Render(arrow),
				typeBadgeStyle.Render(truncateStr(r.Relation, 20)),
				entityNameStyle.Render(target)))
		}

		if relCount > maxRels {
			b.WriteString(fmt.Sprintf("\n  %s",
				timestampStyle.Render(fmt.Sprintf("showing %d-%d of %d relations", m.EntityDetailScroll+1, end, relCount))))
		}
	}

	b.WriteString(helpStyle.Render("\n  j/k scroll • esc back"))

	return b.String()
}

// ─── Graph — Search ──────────────────────────────────────────────────────────

func (m Model) viewGraphSearch() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Search Knowledge Graph"))
	b.WriteString("\n\n")

	b.WriteString(searchInputStyle.Render(m.GraphSearchInput.View()))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("  Type a query and press enter • esc go back"))

	return b.String()
}

// ─── Shared Renderers ────────────────────────────────────────────────────────

func (m Model) renderObservationListItem(index int, id int64, obsType, title, content, createdAt string, project *string) string {
	cursor := "  "
	style := listItemStyle
	if index == m.Cursor {
		cursor = "▸ "
		style = listSelectedStyle
	}

	proj := ""
	if project != nil {
		proj = "  " + projectStyle.Render(*project)
	}

	line := fmt.Sprintf("%s%s %s %s%s  %s\n",
		cursor,
		idStyle.Render(fmt.Sprintf("#%-5d", id)),
		typeBadgeStyle.Render(fmt.Sprintf("[%-12s]", obsType)),
		style.Render(truncateStr(title, 50)),
		proj,
		timestampStyle.Render(localTime(createdAt)))

	// Content preview on second line
	preview := truncateStr(content, 80)
	if preview != "" {
		line += contentPreviewStyle.Render(preview) + "\n"
	}

	return line
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// localTime converts a UTC timestamp string from SQLite to local time for display.
func localTime(utc string) string {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	} {
		if t, err := time.Parse(layout, utc); err == nil {
			return t.UTC().Local().Format("2006-01-02 15:04:05")
		}
	}
	return utc // unparseable — return as-is
}

func truncateStr(s string, max int) string {
	// Remove newlines for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func (m Model) renderEntityListItem(index int, e store.Entity) string {
	cursor := "  "
	style := listItemStyle
	if index == m.Cursor {
		cursor = "▸ "
		style = listSelectedStyle
	}

	proj := ""
	if e.Project != nil {
		proj = "  " + projectStyle.Render(*e.Project)
	}

	summary := ""
	if e.Summary != nil {
		summary = "  " + contentPreviewStyle.Render(truncateStr(*e.Summary, 50))
	}

	line := fmt.Sprintf("%s%s %s %s%s%s  %s\n",
		cursor,
		idStyle.Render(fmt.Sprintf("#%-5d", e.ID)),
		entityTypeBadgeStyle.Render(fmt.Sprintf("[%-10s]", e.EntityType)),
		style.Render(truncateStr(e.Name, 40)),
		proj,
		summary,
		timestampStyle.Render(localTime(e.UpdatedAt)))

	return line
}
