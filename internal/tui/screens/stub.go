// internal/tui/screens/stub.go — TEMPORARY, replaced by Tasks 10-18
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tuitypes"
)

type stub struct{ label string }

func (s stub) Init() tea.Cmd                        { return nil }
func (s stub) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s stub) View() string                         { return s.label }

func NewTarget(s *tuitypes.AppState) tea.Model     { return stub{"[target]"} }
func NewOptions(s *tuitypes.AppState) tea.Model    { return stub{"[options]"} }
func NewDumpOutput(s *tuitypes.AppState) tea.Model { return stub{"[dump_output]"} }
func NewProgress(s *tuitypes.AppState) tea.Model   { return stub{"[progress]"} }
func NewResult(s *tuitypes.AppState) tea.Model     { return stub{"[result]"} }
