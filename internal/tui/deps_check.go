package tui

import (
	"fmt"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/version"
	tea "github.com/charmbracelet/bubbletea"
)

// ===========================================
// Update
// ===========================================

// updateDepsCheck handles input on the first-startup dependency check screen.
func (m model) updateDepsCheck(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc", " ":
			m.mode = viewNormal
			m.depsResult = nil
			return m, nil
		case "q", "ctrl+c", "ctrl+q":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// ===========================================
// View
// ===========================================

// viewDepsCheck renders the first-startup dependency check screen.
func (m model) viewDepsCheck() string {
	var b strings.Builder

	logo := "" +
		"  __ _         _\n" +
		" / _| |___ ___| |_\n" +
		"|  _| / -_) -_)  _|\n" +
		"|_| |_\\___\\___|\\___|"
	b.WriteString(renderGradient(logo))
	if version.Version != "" {
		b.WriteString(" " + dimStyle.Render(version.Version))
	}
	b.WriteString("\n\n")

	var lines []string
	lines = append(lines, dialogTitle.Render("Dependency Check"))
	lines = append(lines, "")

	for _, d := range m.depsResult {
		var status, note string
		if d.Found {
			status = statusRunningStyle.Render("found")
		} else {
			status = errorStyle.Render("not found")
		}

		if !d.Required {
			note = dimStyle.Render(" (optional)")
		}

		lines = append(lines, fmt.Sprintf("  %s  %s%s", status, dialogLabel.Render(d.Name), note))

		if !d.Found {
			lines = append(lines, fmt.Sprintf("    Install: %s", dimStyle.Render(d.InstallURL)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, dialogHint.Render("[enter] Continue"))

	b.WriteString(dialogBox.Render(strings.Join(lines, "\n")))
	b.WriteString("\n")

	return b.String()
}
