# snow_white — PostgreSQL Cloner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an interactive CLI PostgreSQL cloner with a Bubbletea TUI wizard that clones or dumps a PostgreSQL database, with connection profile management and cleanup on failure.

**Architecture:** Single Bubbletea program with a screen-router model advancing through wizard screens; a pure-Go engine layer orchestrates `pg_dump -Fc | pg_restore` streaming via `io.Pipe`; for dump-to-stdout the TUI exits first and `main.go` runs the dump directly after `program.Run()` returns.

**Tech Stack:** Go 1.26.3, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/huh`, `github.com/charmbracelet/bubbles` (spinner), `github.com/charmbracelet/lipgloss`, `github.com/jackc/pgx/v5`, `gopkg.in/yaml.v3`

---

## File Map

| File | Responsibility |
|---|---|
| `main.go` | Entry point, launch Bubbletea, post-TUI stdout dump |
| `internal/profile/model.go` | `Profile` struct |
| `internal/profile/store.go` | Load/save `~/.snow_white/profiles.yaml` |
| `internal/engine/probe.go` | pgx connection test |
| `internal/engine/cleanup.go` | Table snapshot + drop-new tables on failure |
| `internal/engine/clone.go` | `pg_dump -Fc \| pg_restore` pipe + progress |
| `internal/engine/dump.go` | `pg_dump` to `io.Writer` with progress |
| `internal/tui/state.go` | `AppState`, `ConnConfig`, screen enum, Bubbletea messages |
| `internal/tui/app.go` | Top-level Bubbletea model, screen router |
| `internal/tui/screens/start.go` | Binary detection, welcome |
| `internal/tui/screens/profile.go` | Profile list selector |
| `internal/tui/screens/source.go` | Huh form: source connection |
| `internal/tui/screens/mode.go` | Clone vs Dump-only selector |
| `internal/tui/screens/target.go` | Huh form: target connection |
| `internal/tui/screens/options.go` | Schema / data / both selector |
| `internal/tui/screens/dump_output.go` | Stdout vs file path input |
| `internal/tui/screens/progress.go` | Spinner + rolling last-8 lines of stderr |
| `internal/tui/screens/result.go` | Success/error + cleanup summary |

---

## Task 1: Initialize module and install dependencies

**Files:**
- Modify: `go.mod`
- Create: `go.sum` (auto-generated)

- [ ] **Step 1: Initialize git and install all dependencies**

```bash
cd /home/nate/Dev/snow_white
git init
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/huh@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/jackc/pgx/v5@latest
go get gopkg.in/yaml.v3@latest
go mod tidy
```

- [ ] **Step 2: Verify go.mod has all requires**

```bash
cat go.mod
```

Expected: Six `require` entries for bubbletea, huh, bubbles, lipgloss, pgx/v5, yaml.v3.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize module with dependencies"
```

---

## Task 2: Profile model

**Files:**
- Create: `internal/profile/model.go`
- Create: `internal/profile/model_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/profile/model_test.go
package profile_test

import (
	"testing"

	"snow_white/internal/profile"
)

func TestProfileSSLMode(t *testing.T) {
	p := profile.Profile{SSLMode: "require"}
	if p.SSLMode != "require" {
		t.Errorf("expected 'require', got %q", p.SSLMode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/profile/ -run TestProfileSSLMode -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Create the model**

```go
// internal/profile/model.go
package profile

// Profile holds saved connection details.
// SSLMode is "require" or "disable" (maps to the SSL toggle in the TUI).
// Password is stored in plaintext — users are warned before saving.
type Profile struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"ssl_mode"`
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/profile/ -run TestProfileSSLMode -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/profile/model.go internal/profile/model_test.go
git commit -m "feat: add profile model"
```

---

## Task 3: Profile store

**Files:**
- Create: `internal/profile/store.go`
- Create: `internal/profile/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/profile/store_test.go
package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"snow_white/internal/profile"
)

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	profiles, err := profile.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty slice, got %d profiles", len(profiles))
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	input := []profile.Profile{
		{Name: "dev", Host: "localhost", Port: "5432", User: "pg", Password: "pw", DBName: "db1", SSLMode: "disable"},
	}
	if err := profile.Save(input); err != nil {
		t.Fatal(err)
	}

	loaded, err := profile.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(loaded))
	}
	if loaded[0].Name != "dev" {
		t.Errorf("expected name 'dev', got %q", loaded[0].Name)
	}
	if loaded[0].Password != "pw" {
		t.Errorf("expected password 'pw', got %q", loaded[0].Password)
	}
}

func TestLoadCorruptFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".snow_white", "profiles.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not: valid: yaml: [[["), 0600); err != nil {
		t.Fatal(err)
	}

	profiles, err := profile.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty on corrupt file, got %d", len(profiles))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/profile/ -v
```

Expected: FAIL — `Load` and `Save` not defined.

- [ ] **Step 3: Implement the store**

```go
// internal/profile/store.go
package profile

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".snow_white", "profiles.yaml"), nil
}

func Load() ([]Profile, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var profiles []Profile
	if err := yaml.Unmarshal(data, &profiles); err != nil {
		return []Profile{}, nil // corrupt file → treat as empty
	}
	return profiles, nil
}

func Save(profiles []Profile) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(profiles)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/profile/ -v
```

Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/store.go internal/profile/store_test.go
git commit -m "feat: add profile store with YAML load/save"
```

---

## Task 4: Shared TUI state types and messages

**Files:**
- Create: `internal/tui/state.go`

No test needed — these are pure data types used across all TUI screens.

- [ ] **Step 1: Create state.go**

```go
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
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/tui/
```

Expected: exits 0 with no output.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/state.go
git commit -m "feat: add tui state types and bubbletea messages"
```

---

## Task 5: Engine — connection probe

**Files:**
- Create: `internal/engine/probe.go`
- Create: `internal/engine/probe_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/engine/probe_test.go
package engine_test

import (
	"testing"

	"snow_white/internal/engine"
)

func TestProbeInvalidHost(t *testing.T) {
	err := engine.Probe("postgres://bad:bad@127.0.0.1:9999/nodb?sslmode=disable")
	if err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/engine/ -run TestProbeInvalidHost -v
```

Expected: FAIL — `engine.Probe` not defined.

- [ ] **Step 3: Implement probe**

```go
// internal/engine/probe.go
package engine

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Probe opens and pings a PostgreSQL connection. Returns a descriptive error on failure.
func Probe(dsn string) error {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/engine/ -run TestProbeInvalidHost -v
```

Expected: PASS (error returned, not panicked).

- [ ] **Step 5: Commit**

```bash
git add internal/engine/probe.go internal/engine/probe_test.go
git commit -m "feat: add engine connection probe"
```

---

## Task 6: Engine — cleanup (snapshot + drop new tables)

**Files:**
- Create: `internal/engine/cleanup.go`
- Create: `internal/engine/cleanup_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/engine/cleanup_test.go
package engine_test

import (
	"testing"

	"snow_white/internal/engine"
)

func TestNewTablesDiff(t *testing.T) {
	before := map[string]struct{}{
		"users": {},
		"posts": {},
	}
	after := map[string]struct{}{
		"users":    {},
		"posts":    {},
		"comments": {},
	}

	toDelete := engine.DiffTables(before, after)
	if len(toDelete) != 1 {
		t.Fatalf("expected 1 new table, got %v", toDelete)
	}
	if toDelete[0] != "comments" {
		t.Errorf("expected 'comments', got %q", toDelete[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/engine/ -run TestNewTablesDiff -v
```

Expected: FAIL — `engine.DiffTables` not defined.

- [ ] **Step 3: Implement cleanup**

```go
// internal/engine/cleanup.go
package engine

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SnapshotTables returns all BASE TABLE names in the public schema of the given database.
func SnapshotTables(dsn string) (map[string]struct{}, error) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("snapshot connect: %w", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables[name] = struct{}{}
	}
	return tables, rows.Err()
}

// DiffTables returns table names present in after but not in before.
func DiffTables(before, after map[string]struct{}) []string {
	var news []string
	for t := range after {
		if _, existed := before[t]; !existed {
			news = append(news, t)
		}
	}
	return news
}

// DropNewTables connects to the target, snapshots current tables, drops anything
// not present in the pre-run snapshot. Returns the dropped table names.
func DropNewTables(dsn string, before map[string]struct{}) ([]string, error) {
	after, err := SnapshotTables(dsn)
	if err != nil {
		return nil, fmt.Errorf("post-failure snapshot: %w", err)
	}

	toDelete := DiffTables(before, after)
	if len(toDelete) == 0 {
		return nil, nil
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("cleanup connect: %w", err)
	}
	defer conn.Close(ctx)

	var dropped []string
	for _, table := range toDelete {
		if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %q CASCADE`, table)); err != nil {
			return dropped, fmt.Errorf("drop %q: %w", table, err)
		}
		dropped = append(dropped, table)
	}
	return dropped, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/engine/ -run TestNewTablesDiff -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/cleanup.go internal/engine/cleanup_test.go
git commit -m "feat: add engine cleanup (table snapshot + drop on failure)"
```

---

## Task 7: Engine — clone (pg_dump | pg_restore pipe)

**Files:**
- Create: `internal/engine/clone.go`

No unit test — requires live PostgreSQL. The TUI smoke test in Task 20 covers integration.

- [ ] **Step 1: Create clone.go**

```go
// internal/engine/clone.go
package engine

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"

	"snow_white/internal/tui"
)

// CloneRequest holds everything the clone engine needs.
type CloneRequest struct {
	SourceDSN string
	TargetDSN string
	Options   tui.CloneOptions
	// Progress is called with each line of stderr from pg_dump or pg_restore.
	// Called from a goroutine; must be safe to call concurrently.
	Progress func(line string)
}

// Clone streams pg_dump -Fc output directly into pg_restore via an io.Pipe.
// Snapshots the target tables before starting; calls DropNewTables on failure.
// Returns a non-nil error if either process exits non-zero.
func Clone(req CloneRequest) (dropped []string, err error) {
	before, err := SnapshotTables(req.TargetDSN)
	if err != nil {
		return nil, fmt.Errorf("pre-clone snapshot: %w", err)
	}

	pr, pw := io.Pipe()

	dumpCmd := buildDumpCmd(req.SourceDSN, req.Options)
	dumpCmd.Stdout = pw

	restoreCmd := buildRestoreCmd(req.TargetDSN)
	restoreCmd.Stdin = pr

	dumpStderr, _ := dumpCmd.StderrPipe()
	restoreStderr, _ := restoreCmd.StderrPipe()

	go scanLines(dumpStderr, req.Progress)
	go scanLines(restoreStderr, req.Progress)

	if err := dumpCmd.Start(); err != nil {
		return nil, fmt.Errorf("pg_dump start: %w", err)
	}
	if err := restoreCmd.Start(); err != nil {
		_ = dumpCmd.Process.Kill()
		return nil, fmt.Errorf("pg_restore start: %w", err)
	}

	dumpErr := dumpCmd.Wait()
	pw.Close()
	restoreErr := restoreCmd.Wait()

	if dumpErr != nil || restoreErr != nil {
		dropped, _ = DropNewTables(req.TargetDSN, before)
		if dumpErr != nil {
			return dropped, fmt.Errorf("pg_dump: %w", dumpErr)
		}
		return dropped, fmt.Errorf("pg_restore: %w", restoreErr)
	}
	return nil, nil
}

func buildDumpCmd(dsn string, opts tui.CloneOptions) *exec.Cmd {
	args := []string{"-Fc", "--no-password"}
	switch opts {
	case tui.CloneSchemaOnly:
		args = append(args, "-s")
	case tui.CloneDataOnly:
		args = append(args, "-a")
	}
	args = append(args, dsn)
	cmd := exec.Command("pg_dump", args...)
	cmd.Env = os.Environ()
	return cmd
}

func buildRestoreCmd(dsn string) *exec.Cmd {
	args := []string{
		"--no-password",
		"--clean",
		"--if-exists",
		"--single-transaction",
		"-d", dsn,
	}
	cmd := exec.Command("pg_restore", args...)
	cmd.Env = os.Environ()
	return cmd
}

func scanLines(r io.Reader, fn func(string)) {
	if r == nil || fn == nil {
		return
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fn(scanner.Text())
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/engine/
```

Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/engine/clone.go
git commit -m "feat: add engine clone (pg_dump -Fc | pg_restore streaming)"
```

---

## Task 8: Engine — dump (pg_dump to file or writer)

**Files:**
- Create: `internal/engine/dump.go`

- [ ] **Step 1: Create dump.go**

```go
// internal/engine/dump.go
package engine

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"snow_white/internal/tui"
)

// DumpRequest holds everything the dump engine needs.
type DumpRequest struct {
	SourceDSN string
	Dest      io.Writer    // os.Stdout for stdout mode, *os.File for file mode
	Options   tui.CloneOptions
	// Progress is called with each stderr line. May be nil (stdout mode suppresses progress).
	Progress func(line string)
}

// Dump runs pg_dump and writes output to req.Dest.
func Dump(req DumpRequest) error {
	args := []string{"-Fc", "--no-password"}
	switch req.Options {
	case tui.CloneSchemaOnly:
		args = append(args, "-s")
	case tui.CloneDataOnly:
		args = append(args, "-a")
	}
	args = append(args, req.SourceDSN)

	cmd := exec.Command("pg_dump", args...)
	cmd.Stdout = req.Dest
	cmd.Env = os.Environ()

	stderr, _ := cmd.StderrPipe()
	go scanLines(stderr, req.Progress)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pg_dump start: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/engine/
```

Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/engine/dump.go
git commit -m "feat: add engine dump (pg_dump to writer)"
```

---

## Task 9: TUI app shell (screen router)

**Files:**
- Create: `internal/tui/app.go`

- [ ] **Step 1: Create app.go**

```go
// internal/tui/app.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tui/screens"
)

// App is the top-level Bubbletea model. It owns the AppState and routes
// messages and rendering to the currently active screen.
type App struct {
	State   AppState
	screen  Screen
	current tea.Model
}

// New creates an App starting at ScreenStart.
func New(state AppState) App {
	a := App{State: state, screen: ScreenStart}
	a.current = screens.NewStart()
	return a
}

func (a App) Init() tea.Cmd {
	return a.current.Init()
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case NavigateMsg:
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

func (a App) navigate(to Screen) (App, tea.Cmd) {
	a.screen = to
	switch to {
	case ScreenProfile:
		a.current = screens.NewProfile(&a.State)
	case ScreenSource:
		a.current = screens.NewSource(&a.State)
	case ScreenMode:
		a.current = screens.NewMode(&a.State)
	case ScreenTarget:
		a.current = screens.NewTarget(&a.State)
	case ScreenCloneOptions:
		a.current = screens.NewOptions(&a.State)
	case ScreenDumpOutput:
		a.current = screens.NewDumpOutput(&a.State)
	case ScreenProgress:
		a.current = screens.NewProgress(&a.State)
	case ScreenResult:
		a.current = screens.NewResult(&a.State)
	}
	return a, a.current.Init()
}
```

- [ ] **Step 2: Verify it compiles (screens package stubs needed first)**

Create a minimal stub file so the compiler is happy. Real screen implementations come in Tasks 10–18.

```go
// internal/tui/screens/stubs.go  (TEMPORARY — delete in Task 18)
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tui"  // avoid import cycle: screens imports tui state, not tui package
)
```

Wait — there's a circular import risk: `tui` (app.go) imports `tui/screens`, and `tui/screens` would import `tui` for `AppState`. To break this, screens should import only `tui` for the state types.

But `tui/app.go` is in package `tui` and `tui/screens/*.go` are in package `screens`. The screens need `AppState` from package `tui`. This creates a cycle: `tui` → `screens` → `tui`.

**Fix:** Move `AppState`, `ConnConfig`, and all message types to a separate package `internal/tui/state` (no dependency on screens or app). Both `tui` (app.go) and `screens` import from `internal/tui/state`.

**Refactor plan for state types:**
- Rename `internal/tui/state.go` → `internal/tuitypes/types.go`, package `tuitypes`
- Update `internal/tui/app.go` to import `tuitypes`
- All screens import `tuitypes`

Apply this refactor now before writing any screens.

- [ ] **Step 3: Rename tui/state.go to tuitypes package**

Move the file:

```bash
mkdir -p /home/nate/Dev/snow_white/internal/tuitypes
```

Create `internal/tuitypes/types.go` with the same content as `internal/tui/state.go` but change `package tui` to `package tuitypes`:

```go
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
```

- [ ] **Step 4: Delete the old state.go and update engine/clone.go imports**

```bash
rm /home/nate/Dev/snow_white/internal/tui/state.go
```

Update `internal/engine/clone.go` and `internal/engine/dump.go` to import `snow_white/internal/tuitypes` instead of `snow_white/internal/tui`.

In `internal/engine/clone.go`, change:
```go
import (
    ...
    "snow_white/internal/tui"
)
```
to:
```go
import (
    ...
    "snow_white/internal/tuitypes"
)
```

And replace all `tui.CloneOptions`, `tui.CloneSchemaOnly`, `tui.CloneDataOnly` with `tuitypes.CloneOptions`, `tuitypes.CloneSchemaOnly`, `tuitypes.CloneDataOnly`.

Same fix in `internal/engine/dump.go`.

- [ ] **Step 5: Rewrite app.go with correct imports**

```go
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
```

- [ ] **Step 6: Create a minimal screens stub so app.go compiles**

```go
// internal/tui/screens/stub.go  — TEMPORARY, replaced by Tasks 10-18
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tuitypes"
)

type stub struct{ label string }

func (s stub) Init() tea.Cmd                           { return nil }
func (s stub) Update(tea.Msg) (tea.Model, tea.Cmd)    { return s, nil }
func (s stub) View() string                            { return s.label }

func NewStart() tea.Model                              { return stub{"[start]"} }
func NewProfile(s *tuitypes.AppState) tea.Model        { return stub{"[profile]"} }
func NewSource(s *tuitypes.AppState) tea.Model         { return stub{"[source]"} }
func NewMode(s *tuitypes.AppState) tea.Model           { return stub{"[mode]"} }
func NewTarget(s *tuitypes.AppState) tea.Model         { return stub{"[target]"} }
func NewOptions(s *tuitypes.AppState) tea.Model        { return stub{"[options]"} }
func NewDumpOutput(s *tuitypes.AppState) tea.Model     { return stub{"[dump_output]"} }
func NewProgress(s *tuitypes.AppState) tea.Model       { return stub{"[progress]"} }
func NewResult(s *tuitypes.AppState) tea.Model         { return stub{"[result]"} }
```

- [ ] **Step 7: Verify everything compiles**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 8: Commit**

```bash
git add internal/tuitypes/ internal/tui/app.go internal/tui/screens/stub.go internal/engine/clone.go internal/engine/dump.go
git commit -m "feat: add tui app shell + screen router; move state to tuitypes package"
```

---

## Task 10: Start screen

**Files:**
- Create: `internal/tui/screens/start.go`
- Delete the `NewStart` stub from `stub.go`

The start screen checks that `pg_dump` and `pg_restore` are on PATH, then navigates to the profile screen.

- [ ] **Step 1: Create start.go**

```go
// internal/tui/screens/start.go
package screens

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/tuitypes"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	hintStyle  = lipgloss.NewStyle().Faint(true)
)

type startScreen struct {
	missingBins []string
	ready       bool
}

func NewStart() tea.Model {
	s := startScreen{}
	for _, bin := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(bin); err != nil {
			s.missingBins = append(s.missingBins, bin)
		}
	}
	s.ready = len(s.missingBins) == 0
	return s
}

func (s startScreen) Init() tea.Cmd {
	if s.ready {
		return func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenProfile} }
	}
	return nil
}

func (s startScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "q" {
		return s, tea.Quit
	}
	return s, nil
}

func (s startScreen) View() string {
	if s.ready {
		return titleStyle.Render("snow_white") + "\n" + hintStyle.Render("Starting…")
	}
	out := titleStyle.Render("snow_white") + "\n\n"
	out += errStyle.Render("Missing required binaries:") + "\n"
	for _, b := range s.missingBins {
		out += fmt.Sprintf("  • %s\n", b)
	}
	out += "\n" + hintStyle.Render("Install the PostgreSQL client tools, then re-run.")
	out += "\n" + hintStyle.Render("  Ubuntu/Debian: sudo apt install postgresql-client")
	out += "\n" + hintStyle.Render("  macOS:         brew install libpq")
	out += "\n\n" + hintStyle.Render("Press q to quit.")
	return out
}
```

- [ ] **Step 2: Remove NewStart from stub.go**

Edit `internal/tui/screens/stub.go` and delete the `NewStart` function line.

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/start.go internal/tui/screens/stub.go
git commit -m "feat: add start screen with binary detection"
```

---

## Task 11: Profile screen

**Files:**
- Create: `internal/tui/screens/profile.go`
- Remove `NewProfile` stub from `stub.go`

Displays a list: saved profiles + "New connection". Arrow keys to navigate, enter to select.

- [ ] **Step 1: Create profile.go**

```go
// internal/tui/screens/profile.go
package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/profile"
	"snow_white/internal/tuitypes"
)

var (
	selectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	unselectedStyle = lipgloss.NewStyle().Faint(true)
)

type profileScreen struct {
	state    *tuitypes.AppState
	items    []string // display names: profile names + "New connection"
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
			// Fill AppState from selected profile
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
```

- [ ] **Step 2: Remove NewProfile from stub.go**

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/profile.go internal/tui/screens/stub.go
git commit -m "feat: add profile selection screen"
```

---

## Task 12: Source form screen (Huh form)

**Files:**
- Create: `internal/tui/screens/source.go`
- Remove `NewSource` stub from `stub.go`

Huh form embedded in a Bubbletea model. On completion, probes the connection; shows inline error if probe fails.

- [ ] **Step 1: Create source.go**

```go
// internal/tui/screens/source.go
package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/engine"
	"snow_white/internal/tuitypes"
)

var probeErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

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
			// Reset form so user can correct credentials
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

// buildConnForm creates a reusable Huh form for connection fields.
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
```

- [ ] **Step 2: Remove NewSource from stub.go**

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/source.go internal/tui/screens/stub.go
git commit -m "feat: add source connection form with inline probe validation"
```

---

## Task 13: Mode screen (clone vs dump)

**Files:**
- Create: `internal/tui/screens/mode.go`
- Remove `NewMode` stub from `stub.go`

- [ ] **Step 1: Create mode.go**

```go
// internal/tui/screens/mode.go
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tuitypes"
)

type modeScreen struct {
	state  *tuitypes.AppState
	cursor int
}

func NewMode(state *tuitypes.AppState) tea.Model {
	return modeScreen{state: state}
}

func (s modeScreen) Init() tea.Cmd { return nil }

func (s modeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if s.cursor < 1 {
			s.cursor++
		}
	case "enter", " ":
		if s.cursor == 0 {
			s.state.Mode = tuitypes.ModeClone
			return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenTarget} }
		}
		s.state.Mode = tuitypes.ModeDump
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenDumpOutput} }
	}
	return s, nil
}

func (s modeScreen) View() string {
	items := []string{"Clone to another server", "Dump to file / stdout"}
	out := titleStyle.Render("What would you like to do?") + "\n\n"
	for i, item := range items {
		if i == s.cursor {
			out += selectedStyle.Render("▶ "+item) + "\n"
		} else {
			out += unselectedStyle.Render("  "+item) + "\n"
		}
	}
	out += "\n" + hintStyle.Render("↑/↓ navigate • enter select")
	return out
}
```

- [ ] **Step 2: Remove NewMode from stub.go**

- [ ] **Step 3: Compile check**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/mode.go internal/tui/screens/stub.go
git commit -m "feat: add mode selection screen (clone vs dump)"
```

---

## Task 14: Target form screen

**Files:**
- Create: `internal/tui/screens/target.go`
- Remove `NewTarget` stub from `stub.go`

Same structure as source.go. Probes the target connection before navigating to options.

- [ ] **Step 1: Create target.go**

```go
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
```

- [ ] **Step 2: Remove NewTarget from stub.go**

- [ ] **Step 3: Compile check**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/target.go internal/tui/screens/stub.go
git commit -m "feat: add target connection form"
```

---

## Task 15: Clone options screen (schema / data / both)

**Files:**
- Create: `internal/tui/screens/options.go`
- Remove `NewOptions` stub from `stub.go`

- [ ] **Step 1: Create options.go**

```go
// internal/tui/screens/options.go
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/tuitypes"
)

var cloneOptionLabels = []string{
	"Schema + data (full clone)",
	"Schema only",
	"Data only",
}

type optionsScreen struct {
	state  *tuitypes.AppState
	cursor int
}

func NewOptions(state *tuitypes.AppState) tea.Model {
	return optionsScreen{state: state}
}

func (s optionsScreen) Init() tea.Cmd { return nil }

func (s optionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if s.cursor < len(cloneOptionLabels)-1 {
			s.cursor++
		}
	case "enter", " ":
		s.state.Options = tuitypes.CloneOptions(s.cursor)
		return s, func() tea.Msg { return tuitypes.NavigateMsg{To: tuitypes.ScreenProgress} }
	}
	return s, nil
}

func (s optionsScreen) View() string {
	out := titleStyle.Render("What should be copied?") + "\n\n"
	for i, label := range cloneOptionLabels {
		if i == s.cursor {
			out += selectedStyle.Render("▶ "+label) + "\n"
		} else {
			out += unselectedStyle.Render("  "+label) + "\n"
		}
	}
	out += "\n" + hintStyle.Render("↑/↓ navigate • enter select")
	return out
}
```

- [ ] **Step 2: Remove NewOptions from stub.go**

- [ ] **Step 3: Compile check**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/options.go internal/tui/screens/stub.go
git commit -m "feat: add clone options screen"
```

---

## Task 16: Dump output screen (stdout vs file)

**Files:**
- Create: `internal/tui/screens/dump_output.go`
- Remove `NewDumpOutput` stub from `stub.go`

- [ ] **Step 1: Create dump_output.go**

```go
// internal/tui/screens/dump_output.go
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"snow_white/internal/tuitypes"
)

type dumpOutputScreen struct {
	state    *tuitypes.AppState
	form     *huh.Form
	dest     string // "stdout" or "file"
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
```

Note: add `"fmt"` to the import block in dump_output.go.

- [ ] **Step 2: Remove NewDumpOutput from stub.go**

- [ ] **Step 3: Compile check**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/dump_output.go internal/tui/screens/stub.go
git commit -m "feat: add dump output screen (stdout vs file path)"
```

---

## Task 17: Progress screen (spinner + rolling stderr buffer)

**Files:**
- Create: `internal/tui/screens/progress.go`
- Remove `NewProgress` stub from `stub.go`

The progress screen starts the engine in `Init()` via a goroutine that sends `ProgressMsg` and `DoneMsg` back through channels read by Bubbletea commands. For dump-to-stdout mode it quits the TUI immediately (engine runs in `main.go`).

- [ ] **Step 1: Create progress.go**

```go
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
	// Stdout-dump: quit the TUI immediately; main.go handles the actual dump.
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

// waitMsg returns a Cmd that blocks until the next message arrives on msgCh.
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
```

- [ ] **Step 2: Remove NewProgress from stub.go**

- [ ] **Step 3: Compile check**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/progress.go internal/tui/screens/stub.go
git commit -m "feat: add progress screen with spinner and rolling stderr buffer"
```

---

## Task 18: Result screen + remove stub file

**Files:**
- Create: `internal/tui/screens/result.go`
- Delete: `internal/tui/screens/stub.go`

- [ ] **Step 1: Create result.go**

```go
// internal/tui/screens/result.go
package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"snow_white/internal/tuitypes"
)

var (
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

type resultScreen struct {
	state *tuitypes.AppState
}

func NewResult(state *tuitypes.AppState) tea.Model {
	return resultScreen{state: state}
}

func (s resultScreen) Init() tea.Cmd { return nil }

func (s resultScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "ctrl+c", "enter":
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s resultScreen) View() string {
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
			out.WriteString(fmt.Sprintf(
				"Cloned %s → %s\n",
				s.state.Source.DBName,
				s.state.Target.DBName,
			))
		}
	}

	out.WriteString("\n" + hintStyle.Render("Press enter or q to exit."))
	return out.String()
}
```

- [ ] **Step 2: Delete stub.go (all real screens are now implemented)**

```bash
rm /home/nate/Dev/snow_white/internal/tui/screens/stub.go
```

- [ ] **Step 3: Compile check**

```bash
go build ./...
```

Expected: exits 0. No stub warnings.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/screens/result.go
git rm internal/tui/screens/stub.go
git commit -m "feat: add result screen; remove screen stubs (all screens implemented)"
```

---

## Task 19: Save profile prompt in result screen + main.go

**Files:**
- Modify: `internal/tui/screens/result.go` (add save prompt)
- Create: `main.go`

- [ ] **Step 1: Add save-profile prompt to result screen**

On success, before showing the result, ask the user if they want to save the source connection as a profile. Add a `saving` state and a `huh.Form` inside `resultScreen`.

Replace the result screen with this version that includes the save-profile flow:

```go
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

	// Offer profile save on success if source is not already saved
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
```

- [ ] **Step 2: Create main.go**

```go
// main.go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"snow_white/internal/engine"
	"snow_white/internal/profile"
	appui "snow_white/internal/tui"
	"snow_white/internal/tuitypes"
)

func main() {
	profiles, _ := profile.Load()

	initial := tuitypes.AppState{Profiles: profiles}
	app := appui.New(initial)

	p := tea.NewProgram(app, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	final, ok := finalModel.(appui.App)
	if !ok {
		return
	}

	// Dump-to-stdout: TUI has exited; run pg_dump now so it writes to clean stdout.
	if final.State.Mode == tuitypes.ModeDump &&
		final.State.DumpDest == tuitypes.DumpToStdout &&
		final.State.Completed {

		if err := engine.Dump(engine.DumpRequest{
			SourceDSN: final.State.Source.DSN(),
			Dest:      os.Stdout,
			Options:   final.State.Options,
			Progress:  nil, // no progress in stdout mode
		}); err != nil {
			fmt.Fprintln(os.Stderr, "dump failed:", err)
			os.Exit(1)
		}
	}
}
```

- [ ] **Step 3: Full compile check**

```bash
go build ./...
```

Expected: exits 0, binary `snow_white` or via `go run .`.

- [ ] **Step 4: Commit**

```bash
git add main.go internal/tui/screens/result.go
git commit -m "feat: add main.go with stdout-dump handoff; add save-profile prompt in result screen"
```

---

## Task 20: Smoke test — run the binary

- [ ] **Step 1: Build the binary**

```bash
go build -o snow_white .
```

Expected: `snow_white` binary created, exits 0.

- [ ] **Step 2: Verify binary detection works when pg_dump is missing**

```bash
PATH="" ./snow_white
```

Expected: start screen shows missing binaries message with install instructions (not a panic).

- [ ] **Step 3: Run all unit tests**

```bash
go test ./... -v
```

Expected: all tests PASS. (Clone/dump integration tests require a live PostgreSQL — skip with `go test ./internal/profile/... ./internal/engine/ -run 'TestProbeInvalidHost|TestNewTablesDiff|TestLoad|TestSave' -v`)

- [ ] **Step 4: Run with a real PostgreSQL (manual)**

With a running PostgreSQL instance:
```bash
./snow_white
```

Walk through: profile screen → source form (fill in your local PG creds) → mode: clone → target form → options → watch progress screen → verify result screen.

- [ ] **Step 5: Commit final state**

```bash
git add .
git commit -m "chore: verify build and tests pass; snow_white v1 complete"
```

---

## Out of Scope (v1)

- Password encryption in profile store
- Selective table filtering
- Schema mapping (source schema → different target schema)
- Progress percentage (pg_dump does not expose row counts)
- CLI flags / non-interactive mode
- Parallel pg_restore (`--jobs`)
