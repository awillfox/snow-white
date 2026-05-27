// internal/tui/screens/options.go
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tuitypes"
)

var cloneOptionLabels = []string{
	"Schema + data (full clone)",
	"Schema only",
	"Data only",
}

type optionsScreen struct {
	state  *tuitypes.AppState
	cursor int
}

func NewOptions(state *tuitypes.AppState) tea.Model {
	return optionsScreen{state: state}
}

func (s optionsScreen) Init() tea.Cmd { return nil }

func (s optionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if s.cursor < len(cloneOptionLabels)-1 {
			s.cursor++
		}
	case "enter", " ":
		s.state.Options = tuitypes.CloneOptions(s.cursor)
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenProgress} }
	}
	return s, nil
}

func (s optionsScreen) View() string {
	out := titleStyle.Render("What should be copied?") + "\n\n"
	for i, label := range cloneOptionLabels {
		if i == s.cursor {
			out += selectedStyle.Render("▶ "+label) + "\n"
		} else {
			out += unselectedStyle.Render("  "+label) + "\n"
		}
	}
	out += "\n" + hintStyle.Render("↑/↓ navigate • enter select")
	return out
}
