// internal/tui/screens/profile.go
package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/profile"
	"snow_white/internal/tuitypes"
)

type profileScreen struct {
	state    *tuitypes.AppState
	items    []string
	profiles []profile.Profile
	cursor   int
}

func NewProfile(state *tuitypes.AppState) tea.Model {
	items := make([]string, 0, len(state.Profiles)+1)
	for _, p := range state.Profiles {
		items = append(items, p.Name)
	}
	items = append(items, "New connection")
	return profileScreen{
		state:    state,
		items:    items,
		profiles: state.Profiles,
	}
}

func (s profileScreen) Init() tea.Cmd { return nil }

func (s profileScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if s.cursor < len(s.items)-1 {
			s.cursor++
		}
	case "enter", " ":
		if s.cursor < len(s.profiles) {
			p := s.profiles[s.cursor]
			s.state.Source = tuitypes.ConnConfig{
				Host:     p.Host,
				Port:     p.Port,
				User:     p.User,
				Password: p.Password,
				DBName:   p.DBName,
				SSLMode:  p.SSLMode,
			}
		}
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenSource} }
	}
	return s, nil
}

func (s profileScreen) View() string {
	out := titleStyle.Render("Select a connection profile") + "\n\n"
	for i, item := range s.items {
		if i == s.cursor {
			out += selectedStyle.Render(fmt.Sprintf("▶ %s", item)) + "\n"
		} else {
			out += unselectedStyle.Render(fmt.Sprintf("  %s", item)) + "\n"
		}
	}
	out += "\n" + hintStyle.Render("↑/↓ navigate • enter select")
	return out
}
