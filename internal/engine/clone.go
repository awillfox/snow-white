// internal/engine/clone.go
package engine

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// CloneOptions controls what pg_dump includes.
type CloneOptions int

const (
	CloneBoth       CloneOptions = iota // schema + data
	CloneSchemaOnly                     // -s flag
	CloneDataOnly                       // -a flag
)

// CloneRequest holds everything the clone engine needs.
type CloneRequest struct {
	SourceDSN string
	TargetDSN string
	Options   CloneOptions
	// Progress is called with each line of stderr from pg_dump or pg_restore.
	// Called from a goroutine; must be safe to call concurrently.
	Progress func(line string)
}

// Clone streams pg_dump -Fc output directly into pg_restore via an io.Pipe.
// Snapshots the target tables before starting; calls DropNewTables on failure.
// Returns dropped table names and any error.
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

func buildDumpCmd(dsn string, opts CloneOptions) *exec.Cmd {
	args := []string{"-Fc", "--no-password"}
	switch opts {
	case CloneSchemaOnly:
		args = append(args, "-s")
	case CloneDataOnly:
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
