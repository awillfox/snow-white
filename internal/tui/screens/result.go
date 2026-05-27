// internal/tui/screens/result.go
package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/tuitypes"
)

var (
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

type resultScreen struct {
	state *tuitypes.AppState
}

func NewResult(state *tuitypes.AppState) tea.Model {
	return resultScreen{state: state}
}

func (s resultScreen) Init() tea.Cmd { return nil }

func (s resultScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "ctrl+c", "enter":
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s resultScreen) View() string {
	var out strings.Builder

	if s.state.FinalErr != nil {
		out.WriteString(errStyle.Render("Operation failed") + "\n\n")
		out.WriteString(fmt.Sprintf("%s\n", s.state.FinalErr))
		if len(s.state.Dropped) > 0 {
			out.WriteString("\n" + warnStyle.Render("Cleaned up partial tables:") + "\n")
			for _, t := range s.state.Dropped {
				out.WriteString(fmt.Sprintf("  • %s\n", t))
			}
		} else {
			out.WriteString("\n" + hintStyle.Render("No tables needed cleanup.") + "\n")
		}
	} else {
		out.WriteString(successStyle.Render("Done!") + "\n\n")
		switch {
		case s.state.Mode == tuitypes.ModeDump && s.state.DumpDest == tuitypes.DumpToFile:
			out.WriteString(fmt.Sprintf("Dump saved to: %s\n", s.state.DumpFile))
		case s.state.Mode == tuitypes.ModeDump:
			out.WriteString("Dump written to stdout.\n")
		default:
			out.WriteString(fmt.Sprintf("Cloned %s → %s\n", s.state.Source.DBName, s.state.Target.DBName))
		}
	}

	out.WriteString("\n" + hintStyle.Render("Press enter or q to exit."))
	return out.String()
}
