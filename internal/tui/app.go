// internal/tui/app.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tui/screens"
	"snow_white/internal/tuitypes"
)

// App is the top-level Bubbletea model.
type App struct {
	State   tuitypes.AppState
	screen  tuitypes.Screen
	current tea.Model
}

func New(state tuitypes.AppState) App {
	a := App{State: state, screen: tuitypes.ScreenStart}
	a.current = screens.NewStart()
	return a
}

func (a App) Init() tea.Cmd {
	return a.current.Init()
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuitypes.NavigateMsg:
		return a.navigate(m.To)
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}
	next, cmd := a.current.Update(msg)
	a.current = next
	return a, cmd
}

func (a App) View() string {
	return a.current.View()
}

func (a App) navigate(to tuitypes.Screen) (App, tea.Cmd) {
	a.screen = to
	switch to {
	case tuitypes.ScreenProfile:
		a.current = screens.NewProfile(&a.State)
	case tuitypes.ScreenSource:
		a.current = screens.NewSource(&a.State)
	case tuitypes.ScreenMode:
		a.current = screens.NewMode(&a.State)
	case tuitypes.ScreenTarget:
		a.current = screens.NewTarget(&a.State)
	case tuitypes.ScreenCloneOptions:
		a.current = screens.NewOptions(&a.State)
	case tuitypes.ScreenDumpOutput:
		a.current = screens.NewDumpOutput(&a.State)
	case tuitypes.ScreenProgress:
		a.current = screens.NewProgress(&a.State)
	case tuitypes.ScreenResult:
		a.current = screens.NewResult(&a.State)
	}
	return a, a.current.Init()
}
