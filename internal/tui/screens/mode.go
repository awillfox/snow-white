// internal/tui/screens/mode.go
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tuitypes"
)

type modeScreen struct {
	state  *tuitypes.AppState
	cursor int
}

func NewMode(state *tuitypes.AppState) tea.Model {
	return modeScreen{state: state}
}

func (s modeScreen) Init() tea.Cmd { return nil }

func (s modeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch k.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < 1 {
			s.cursor++
		}
	case "enter", " ":
		if s.cursor == 0 {
			s.state.Mode = tuitypes.ModeClone
			return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenTarget} }
		}
		s.state.Mode = tuitypes.ModeDump
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenDumpOutput} }
	}
	return s, nil
}

func (s modeScreen) View() string {
	items := []string{"Clone to another server", "Dump to file / stdout"}
	out := titleStyle.Render("What would you like to do?") + "\n\n"
	for i, item := range items {
		if i == s.cursor {
			out += selectedStyle.Render("▶ "+item) + "\n"
		} else {
			out += unselectedStyle.Render("  "+item) + "\n"
		}
	}
	out += "\n" + hintStyle.Render("↑/↓ navigate • enter select")
	return out
}
