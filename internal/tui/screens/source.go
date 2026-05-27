// internal/tui/screens/source.go
package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"snow_white/internal/engine"
	"snow_white/internal/tuitypes"
)

type sourceScreen struct {
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

func NewSource(state *tuitypes.AppState) tea.Model {
	s := &sourceScreen{
		state:    state,
		host:     state.Source.Host,
		port:     orDefault(state.Source.Port, "5432"),
		user:     state.Source.User,
		password: state.Source.Password,
		dbname:   state.Source.DBName,
		ssl:      state.Source.SSLMode == "require",
	}
	s.form = buildConnForm("Source server", &s.host, &s.port, &s.user, &s.password, &s.dbname, &s.ssl)
	return s
}

func (s *sourceScreen) Init() tea.Cmd { return s.form.Init() }

func (s *sourceScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			s.form = buildConnForm("Source server", &s.host, &s.port, &s.user, &s.password, &s.dbname, &s.ssl)
			return s, s.form.Init()
		}
		s.state.Source = cfg
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenMode} }
	}
	return s, cmd
}

func (s *sourceScreen) View() string {
	out := s.form.View()
	if s.probeErr != "" {
		out += "\n" + probeErrStyle.Render("Connection failed: "+s.probeErr)
	}
	return out
}

func buildConnForm(title string, host, port, user, password, dbname *string, ssl *bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title(title+" — Host").Value(host).Validate(nonEmpty("host")),
			huh.NewInput().Title("Port").Value(port).Validate(nonEmpty("port")),
			huh.NewInput().Title("User").Value(user).Validate(nonEmpty("user")),
			huh.NewInput().Title("Password").EchoMode(huh.EchoModePassword).Value(password),
			huh.NewInput().Title("Database").Value(dbname).Validate(nonEmpty("database")),
			huh.NewConfirm().Title("Enable SSL?").Value(ssl),
		),
	)
}

func nonEmpty(field string) func(string) error {
	return func(s string) error {
		if s == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
