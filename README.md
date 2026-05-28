# snow_white

Interactive CLI tool for cloning or dumping a PostgreSQL database. A Bubbletea TUI wizard guides you through entering credentials, selecting an operation mode, and monitoring progress in real time.

## Requirements

The following must be installed before building or running snow_white:

### Go 1.26+

Download from https://go.dev/dl/ or use your package manager:

```bash
# macOS
brew install go

# Debian/Ubuntu
sudo apt install golang-go

# Arch
sudo pacman -S go
```

### PostgreSQL client tools (`pg_dump` / `pg_restore`)

These ship with the PostgreSQL client package — you do **not** need a full server install:

```bash
# macOS
brew install libpq
brew link --force libpq   # puts pg_dump/pg_restore on PATH

# Debian/Ubuntu
sudo apt install postgresql-client

# Arch
sudo pacman -S postgresql-libs

# Windows (WSL)
sudo apt install postgresql-client
```

Verify both are available:

```bash
pg_dump --version
pg_restore --version
```

## Build

```bash
go build -o snow_white .
```

## Usage

```bash
./snow_white
```

No flags or subcommands. The wizard handles everything.

### Screen flow

```
StartScreen      — verifies pg_dump / pg_restore are installed
ProfileScreen    — pick a saved profile or start a new connection
SourceForm       — host, port, user, password, dbname, SSL
ModeScreen       — Clone to another server | Dump to file/stdout
  [Clone] TargetForm → CloneOptionsScreen → ProgressScreen → ResultScreen
  [Dump]  DumpOutputScreen → CloneOptionsScreen → ProgressScreen → ResultScreen
```

### Clone options

- **Schema + Data** — full clone (default)
- **Schema only** — DDL only, no rows
- **Data only** — rows only, assumes schema already exists on target

### Dump output

- **File** — pg_dump output written to a path you specify
- **Stdout** — TUI exits cleanly first, then pg_dump streams directly to stdout

## Profiles

Saved connections are stored in `~/.snow_white/profiles.yaml` (permissions 0600). Passwords are stored in plaintext — a warning is shown before saving. A missing or corrupt profile file is treated as an empty list.

## How it works

**Clone path:** `pg_dump -Fc` (custom format) pipes directly into `pg_restore` via `io.Pipe` — no temp file. Both processes' stderr is captured line-by-line and shown as a rolling 8-line buffer beneath a spinner.

**Failure cleanup:** The target's table list is snapshotted via pgx _before_ `pg_restore` runs. On failure, any tables added during the failed restore are dropped. Pre-existing tables are left untouched.

**Connection probing:** `pgx.Connect` + `Ping` is called against both source and target before any operation starts. Connection errors are shown inline on the form so you can correct them without restarting.

## Development

```bash
go test ./...
go vet ./...
```

## Package structure

```
main.go                    — entry point, Bubbletea program, stdout-dump post-TUI
internal/
  tuitypes/                — shared types: AppState, ConnConfig, Screen, messages
  tui/
    app.go                 — top-level model, screen router
    screens/               — one file per screen
  engine/
    probe.go               — pgx connection check
    clone.go               — pg_dump → io.Pipe → pg_restore
    dump.go                — pg_dump → file or stdout
    cleanup.go             — post-failure table diff and drop
  profile/
    model.go               — Profile struct
    store.go               — ~/.snow_white/profiles.yaml read/write
```

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/huh` | Form inputs with validation and password masking |
| `github.com/charmbracelet/lipgloss` | Styling |
| `github.com/charmbracelet/bubbles` | Spinner |
| `github.com/jackc/pgx/v5` | Connection probing |
| `gopkg.in/yaml.v3` | Profile store |
