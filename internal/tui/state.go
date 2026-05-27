// internal/tui/state.go
package tui

import "snow_white/internal/profile"

// Screen identifies which wizard screen is active.
type Screen int

const (
	ScreenStart Screen = iota
	ScreenProfile
	ScreenSource
	ScreenMode
	ScreenTarget
	ScreenCloneOptions
	ScreenDumpOutput
	ScreenProgress
	ScreenResult
)

// OperationMode is the top-level choice: clone or dump.
type OperationMode int

const (
	ModeClone OperationMode = iota
	ModeDump
)

// CloneOptions controls what pg_dump includes.
type CloneOptions int

const (
	CloneBoth CloneOptions = iota // schema + data (default)
	CloneSchemaOnly
	CloneDataOnly
)

// DumpDest is where dump output goes.
type DumpDest int

const (
	DumpToFile DumpDest = iota
	DumpToStdout
)

// ConnConfig holds credentials for one PostgreSQL server.
// SSLMode is "require" or "disable" (driven by a boolean SSL toggle in the UI).
type ConnConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DSN returns a libpq URI suitable for pg_dump, pg_restore, and pgx.
func (c ConnConfig) DSN() string {
	return "postgres://" + c.User + ":" + c.Password +
		"@" + c.Host + ":" + c.Port + "/" + c.DBName +
		"?sslmode=" + c.SSLMode
}

// AppState is the single shared state struct that flows through every screen.
type AppState struct {
	Profiles  []profile.Profile
	Source    ConnConfig
	Target    ConnConfig
	Mode      OperationMode
	Options   CloneOptions
	DumpDest  DumpDest
	DumpFile  string // path; only used when DumpDest == DumpToFile

	// Set by progress screen on completion, read by main.go for stdout-dump path.
	Completed bool
	FinalErr  error
	Dropped   []string // tables cleaned up on failure
}

// Bubbletea messages

// NavigateMsg tells the app router to switch to a new screen.
type NavigateMsg struct{ To Screen }

// ProgressMsg carries one line of stderr from pg_dump or pg_restore.
type ProgressMsg struct{ Line string }

// DoneMsg signals that the engine goroutine has finished (success or error).
type DoneMsg struct{ Err error }

// CleanupMsg reports which tables were dropped during failure cleanup.
type CleanupMsg struct{ Dropped []string }
