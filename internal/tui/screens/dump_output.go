// internal/tui/screens/dump_output.go
package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"snow_white/internal/tuitypes"
)

type dumpOutputScreen struct {
	state    *tuitypes.AppState
	form     *huh.Form
	dest     string
	filePath string
}

func NewDumpOutput(state *tuitypes.AppState) tea.Model {
	s := &dumpOutputScreen{state: state, dest: "file"}
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Dump destination").
				Options(
					huh.NewOption("File on disk", "file"),
					huh.NewOption("Standard output (stdout)", "stdout"),
				).
				Value(&s.dest),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("File path").
				Value(&s.filePath).
				Validate(func(v string) error {
					if s.dest == "file" && v == "" {
						return fmt.Errorf("file path is required")
					}
					return nil
				}),
		),
	)
	return s
}

func (s *dumpOutputScreen) Init() tea.Cmd { return s.form.Init() }

func (s *dumpOutputScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := s.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		s.form = f
	}
	if s.form.State == huh.StateCompleted {
		if s.dest == "stdout" {
			s.state.DumpDest = tuitypes.DumpToStdout
		} else {
			s.state.DumpDest = tuitypes.DumpToFile
			s.state.DumpFile = s.filePath
		}
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenCloneOptions} }
	}
	return s, cmd
}

func (s *dumpOutputScreen) View() string {
	return s.form.View()
}
