// internal/tui/screens/result.go
package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/profile"
	"snow_white/internal/tuitypes"
)

var (
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

type resultScreen struct {
	state       *tuitypes.AppState
	saveForm    *huh.Form
	saveName    string
	saveConfirm bool
	phase       string // "saving" | "done"
}

func NewResult(state *tuitypes.AppState) tea.Model {
	s := &resultScreen{state: state, phase: "done"}

	if state.FinalErr == nil {
		s.phase = "saving"
		s.saveForm = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Save this connection as a profile? (password stored in plaintext)").
					Value(&s.saveConfirm),
				huh.NewInput().
					Title("Profile name").
					Value(&s.saveName).
					Validate(func(v string) error {
						if s.saveConfirm && v == "" {
							return fmt.Errorf("name is required")
						}
						return nil
					}),
			),
		)
	}
	return s
}

func (s *resultScreen) Init() tea.Cmd {
	if s.saveForm != nil {
		return s.saveForm.Init()
	}
	return nil
}

func (s *resultScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.phase == "saving" && s.saveForm != nil {
		form, cmd := s.saveForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.saveForm = f
		}
		if s.saveForm.State == huh.StateCompleted {
			if s.saveConfirm && s.saveName != "" {
				src := s.state.Source
				newProfile := profile.Profile{
					Name:     s.saveName,
					Host:     src.Host,
					Port:     src.Port,
					User:     src.User,
					Password: src.Password,
					DBName:   src.DBName,
					SSLMode:  src.SSLMode,
				}
				all := append(s.state.Profiles, newProfile)
				_ = profile.Save(all)
			}
			s.phase = "done"
			return s, nil
		}
		return s, cmd
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "ctrl+c", "enter":
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *resultScreen) View() string {
	if s.phase == "saving" && s.saveForm != nil {
		return s.saveForm.View()
	}

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
