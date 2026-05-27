// internal/tui/screens/start.go
package screens

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/tuitypes"
)

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	errStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	hintStyle       = lipgloss.NewStyle().Faint(true)
	selectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	unselectedStyle = lipgloss.NewStyle().Faint(true)
	probeErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

type startScreen struct {
	missingBins []string
	ready       bool
}

func NewStart() tea.Model {
	s := startScreen{}
	for _, bin := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(bin); err != nil {
			s.missingBins = append(s.missingBins, bin)
		}
	}
	s.ready = len(s.missingBins) == 0
	return s
}

func (s startScreen) Init() tea.Cmd {
	if s.ready {
		return func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenProfile} }
	}
	return nil
}

func (s startScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "q" {
		return s, tea.Quit
	}
	return s, nil
}

func (s startScreen) View() string {
	if s.ready {
		return titleStyle.Render("snow_white") + "\n" + hintStyle.Render("Starting…")
	}
	out := titleStyle.Render("snow_white") + "\n\n"
	out += errStyle.Render("Missing required binaries:") + "\n"
	for _, b := range s.missingBins {
		out += fmt.Sprintf("  • %s\n", b)
	}
	out += "\n" + hintStyle.Render("Install the PostgreSQL client tools, then re-run.")
	out += "\n" + hintStyle.Render("  Ubuntu/Debian: sudo apt install postgresql-client")
	out += "\n" + hintStyle.Render("  macOS:         brew install libpq")
	out += "\n\n" + hintStyle.Render("Press q to quit.")
	return out
}
