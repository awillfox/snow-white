// internal/tui/screens/progress.go
package screens

import (
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/engine"
	"snow_white/internal/tuitypes"
)

const maxLines = 8

type progressScreen struct {
	state   *tuitypes.AppState
	spinner spinner.Model
	lines   []string
	msgCh   chan tea.Msg
}

func NewProgress(state *tuitypes.AppState) tea.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	return &progressScreen{
		state:   state,
		spinner: sp,
		msgCh:   make(chan tea.Msg, 64),
	}
}

func (s *progressScreen) Init() tea.Cmd {
	// Stdout-dump: quit TUI immediately; main.go handles the actual dump.
	if s.state.Mode == tuitypes.ModeDump && s.state.DumpDest == tuitypes.DumpToStdout {
		s.state.Completed = true
		return tea.Quit
	}

	go s.runEngine()
	return tea.Batch(s.spinner.Tick, s.waitMsg())
}

func (s *progressScreen) runEngine() {
	progress := func(line string) {
		s.msgCh <- tuitypes.ProgressMsg{Line: line}
	}

	var err error
	var dropped []string

	if s.state.Mode == tuitypes.ModeClone {
		dropped, err = engine.Clone(engine.CloneRequest{
			SourceDSN: s.state.Source.DSN(),
			TargetDSN: s.state.Target.DSN(),
			Options:   s.state.Options,
			Progress:  progress,
		})
	} else {
		f, openErr := os.Create(s.state.DumpFile)
		if openErr != nil {
			s.msgCh <- tuitypes.DoneMsg{Err: openErr}
			return
		}
		defer f.Close()
		err = engine.Dump(engine.DumpRequest{
			SourceDSN: s.state.Source.DSN(),
			Dest:      f,
			Options:   s.state.Options,
			Progress:  progress,
		})
	}

	s.msgCh <- tuitypes.CleanupMsg{Dropped: dropped}
	s.msgCh <- tuitypes.DoneMsg{Err: err}
}

func (s *progressScreen) waitMsg() tea.Cmd {
	return func() tea.Msg {
		return <-s.msgCh
	}
}

func (s *progressScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuitypes.ProgressMsg:
		s.lines = append(s.lines, m.Line)
		if len(s.lines) > maxLines {
			s.lines = s.lines[len(s.lines)-maxLines:]
		}
		return s, s.waitMsg()

	case tuitypes.CleanupMsg:
		s.state.Dropped = m.Dropped
		return s, s.waitMsg()

	case tuitypes.DoneMsg:
		s.state.FinalErr = m.Err
		s.state.Completed = true
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenResult} }

	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *progressScreen) View() string {
	out := titleStyle.Render("Running…") + " " + s.spinner.View() + "\n\n"
	for _, line := range s.lines {
		out += hintStyle.Render(line) + "\n"
	}
	return out
}
