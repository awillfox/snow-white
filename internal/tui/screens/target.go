// internal/tui/screens/target.go
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"snow_white/internal/engine"
	"snow_white/internal/tuitypes"
)

type targetScreen struct {
	state    *tuitypes.AppState
	form     *huh.Form
	host     string
	port     string
	user     string
	password string
	dbname   string
	ssl      bool
	probeErr string
}

func NewTarget(state *tuitypes.AppState) tea.Model {
	s := &targetScreen{
		state:    state,
		host:     state.Target.Host,
		port:     orDefault(state.Target.Port, "5432"),
		user:     state.Target.User,
		password: state.Target.Password,
		dbname:   state.Target.DBName,
		ssl:      state.Target.SSLMode == "require",
	}
	s.form = buildConnForm("Target server", &s.host, &s.port, &s.user, &s.password, &s.dbname, &s.ssl)
	return s
}

func (s *targetScreen) Init() tea.Cmd { return s.form.Init() }

func (s *targetScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := s.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		s.form = f
	}
	if s.form.State == huh.StateCompleted {
		sslMode := "disable"
		if s.ssl {
			sslMode = "require"
		}
		cfg := tuitypes.ConnConfig{
			Host:     s.host,
			Port:     s.port,
			User:     s.user,
			Password: s.password,
			DBName:   s.dbname,
			SSLMode:  sslMode,
		}
		if err := engine.Probe(cfg.DSN()); err != nil {
			s.probeErr = err.Error()
			s.form = buildConnForm("Target server", &s.host, &s.port, &s.user, &s.password, &s.dbname, &s.ssl)
			return s, s.form.Init()
		}
		s.state.Target = cfg
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenCloneOptions} }
	}
	return s, cmd
}

func (s *targetScreen) View() string {
	out := s.form.View()
	if s.probeErr != "" {
		out += "\n" + probeErrStyle.Render("Connection failed: "+s.probeErr)
	}
	return out
}
