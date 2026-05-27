// internal/tuitypes/types.go
package tuitypes

import "snow_white/internal/profile"

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

type OperationMode int

const (
	ModeClone OperationMode = iota
	ModeDump
)

type CloneOptions int

const (
	CloneBoth CloneOptions = iota
	CloneSchemaOnly
	CloneDataOnly
)

type DumpDest int

const (
	DumpToFile DumpDest = iota
	DumpToStdout
)

type ConnConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (c ConnConfig) DSN() string {
	return "postgres://" + c.User + ":" + c.Password +
		"@" + c.Host + ":" + c.Port + "/" + c.DBName +
		"?sslmode=" + c.SSLMode
}

type AppState struct {
	Profiles  []profile.Profile
	Source    ConnConfig
	Target    ConnConfig
	Mode      OperationMode
	Options   CloneOptions
	DumpDest  DumpDest
	DumpFile  string

	Completed bool
	FinalErr  error
	Dropped   []string
}

type NavigateMsg struct{ To Screen }
type ProgressMsg struct{ Line string }
type DoneMsg struct{ Err error }
type CleanupMsg struct{ Dropped []string }
