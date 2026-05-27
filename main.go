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

	initial := &tuitypes.AppState{Profiles: profiles}
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
			Progress:  nil,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "dump failed:", err)
			os.Exit(1)
		}
	}
}
