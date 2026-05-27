# snow_white — Interactive CLI PostgreSQL Cloner: Design Spec

**Date:** 2026-05-27  
**Status:** Approved

---

## Overview

`snow_white` is an interactive CLI tool written in Go that clones or dumps a PostgreSQL database. It presents a rich TUI wizard (Bubbletea + Huh) that guides the user through entering source and target server credentials, selecting an operation mode, and monitoring progress. Connection profiles can be saved and reused across runs.

---

## User-Facing Behavior

- Invoked with no arguments: `snow_white`
- Detects `pg_dump` / `pg_restore` on PATH at startup; exits with install instructions if absent
- Wizard-style screen flow — no flags or subcommands
- Passwords masked in input; plaintext warning shown before saving to profile

---

## Screen Flow

```
StartScreen
  └─> ProfileScreen         (list saved profiles or "new connection")
        └─> SourceForm      (host, port, user, password, dbname, SSL toggle: maps to sslmode=require/disable)
              └─> ModeScreen        (Clone | Dump-only)
                    ├─[Clone]─> TargetForm       (same fields as source, or pick profile)
                    │              └─> CloneOptionsScreen   (schema / data / both)
                    │                     └─> ProgressScreen ──> ResultScreen
                    └─[Dump]──> DumpOutputScreen  (stdout | file path)
                                   └─> CloneOptionsScreen
                                          └─> ProgressScreen ──> ResultScreen
```

Screens advance forward and can navigate back. All state is carried in a single `AppState` struct passed through the Bubbletea model.

---

## Package Structure

```
snow_white/
├── main.go                      # entry point, launches Bubbletea app
├── go.mod
│
├── internal/
│   ├── tui/
│   │   ├── app.go               # top-level model, screen router, Update/View
│   │   ├── state.go             # AppState, ConnConfig structs
│   │   └── screens/
│   │       ├── start.go         # binary check, welcome
│   │       ├── profile.go       # list + select saved profiles
│   │       ├── source.go        # Huh form: source connection fields
│   │       ├── target.go        # Huh form: target connection fields
│   │       ├── mode.go          # clone vs dump-only selector
│   │       ├── options.go       # schema/data/both selector
│   │       ├── dump.go          # stdout vs file path input
│   │       ├── progress.go      # spinner + rolling last-8 lines of pg output
│   │       └── result.go        # success/error + cleanup summary
│   │
│   ├── engine/
│   │   ├── probe.go             # pgx test-connection before operation
│   │   ├── clone.go             # pg_dump stdout → io.Pipe → pg_restore stdin
│   │   ├── dump.go              # pg_dump → stdout or file
│   │   └── cleanup.go           # drops partially-written tables on target on failure
│   │
│   └── profile/
│       ├── model.go             # Profile struct
│       └── store.go             # read/write ~/.snow_white/profiles.yaml
│
└── docs/
    └── superpowers/specs/
        └── 2026-05-27-psql-cloner-design.md
```

**Boundaries:**
- `tui` calls `engine` — never invokes `pg_dump` directly
- `engine` receives resolved `ConnConfig` — never reads profiles
- `profile` is pure file I/O — no Bubbletea dependency

---

## Engine & Data Flow

### Connection Probing (`engine/probe.go`)
Opens a pgx connection to verify credentials before starting any operation. Typed errors are returned to the TUI and shown inline on the relevant form screen so the user can correct and retry without restarting.

### Clone Path (`engine/clone.go`)
```
pg_dump -Fc (source) ──stdout──> io.Pipe ──stdin──> pg_restore (target)
```
- `pg_dump` runs in custom format (`-Fc`) so `pg_restore` can use `--single-transaction` and `--clean --if-exists`
- `pg_dump` flags derived from `ConnConfig` + clone options (`-s` schema-only, `-a` data-only, neither for full)
- `pg_restore` invoked with `--clean --if-exists --single-transaction` to make failures atomic where PostgreSQL supports it
- Both processes' stderr captured line-by-line via goroutine → `ProgressMsg{line}` → Bubbletea
- Progress screen keeps a rolling buffer of the last 8 stderr lines beneath a spinner
- Non-zero exit from either process → `DoneMsg{err}` → triggers cleanup

### Dump Path (`engine/dump.go`)
Same `pg_dump` invocation; stdout directed to `os.File` (user-supplied path) or `/dev/stdout`. In stdout mode, the TUI tears down completely before `pg_dump` begins streaming so the two do not write to the terminal simultaneously; progress display is suppressed in this mode. Stderr captured identically for progress display in file mode.

### Cleanup (`engine/cleanup.go`)
`--single-transaction` handles most failures atomically via PostgreSQL's rollback. For cases where it does not apply (e.g., DDL-heavy restores, partitioned tables), the engine snapshots the target's table list via pgx *before* starting pg_restore, then on failure drops any tables present in the post-failure list that were not in the pre-run snapshot. This avoids the broken "compare source vs target" approach (source may be unreachable at cleanup time, and pre-existing target tables would be wrongly flagged). Cleanup status is shown on the result screen.

---

## Profile Storage

- File: `~/.snow_white/profiles.yaml`
- Format: YAML list of `Profile` structs (name, host, port, user, password, dbname, ssl_mode)
- Passwords stored as plaintext — warning displayed in TUI before saving
- Missing or corrupt file treated as empty profile list (non-fatal)

---

## Error Handling

| Scenario | Behavior |
|---|---|
| `pg_dump`/`pg_restore` not on PATH | Detected at startup, shown on StartScreen with install instructions |
| Connection failure | Shown inline on form screen; user can correct and retry |
| Mid-clone process failure | `DoneMsg{err}` → cleanup goroutine → result screen shows what was cleaned |
| Profile file missing/corrupt | Treated as empty list, not fatal |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/huh` | Form inputs with validation and password masking |
| `github.com/charmbracelet/lipgloss` | Styling |
| `github.com/jackc/pgx/v5` | Connection probing |
| `gopkg.in/yaml.v3` | Profile store |

External requirements: `pg_dump`, `pg_restore` (PostgreSQL client tools) must be on PATH.

---

## Out of Scope (v1)

- Password encryption in profile store
- Selective table filtering
- Schema mapping (clone schema A → schema B on target)
- Progress as percentage (pg_dump does not expose row counts)
- CLI flags / non-interactive mode
