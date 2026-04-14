package tui

import tea "github.com/charmbracelet/bubbletea"

// ===========================================
// Route
// ===========================================

// route identifies which page is currently displayed.
type route int

const (
	routeFleetList route = iota
	routeSettings
	routeDepsCheck
)

// ===========================================
// Page Interface
// ===========================================

// Page defines the contract for a TUI page. Each page owns its own
// local state and receives a pointer to the shared model on every
// call so it always sees up-to-date shared data without storing a
// stale copy.
type Page interface {
	// Init is called when the page becomes active. It may return a
	// tea.Cmd to kick off async work.
	Init(m *model) tea.Cmd

	// Update handles a single Bubbletea message and returns any
	// command to execute next.
	Update(m *model, msg tea.Msg) tea.Cmd

	// View renders the page content as a string.
	View(m *model) string
}

// ===========================================
// Route Management
// ===========================================

// ChangeRoute switches the active page. For the fleet list the
// persistent instance is reused (it carries running split-pane
// state). Other pages are constructed fresh on each switch. The
// new page's Init method is called and its command returned.
func (m *model) ChangeRoute(r route) tea.Cmd {
	switch r {
	case routeFleetList:
		m.currentPage = m.fleetPage
	case routeSettings:
		m.currentPage = newSettingsPage()
	case routeDepsCheck:
		m.currentPage = newDepsCheckPage(nil)
	}
	return m.currentPage.Init(m)
}
