// internal/engine/dump.go
package engine

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"snow_white/internal/tuitypes"
)

// DumpRequest holds everything the dump engine needs.
type DumpRequest struct {
	SourceDSN string
	Dest      io.Writer
	Options   tuitypes.CloneOptions
	Progress  func(line string)
}

// Dump runs pg_dump and writes output to req.Dest.
func Dump(req DumpRequest) error {
	args := []string{"-Fc", "--no-password"}
	switch req.Options {
	case tuitypes.CloneSchemaOnly:
		args = append(args, "-s")
	case tuitypes.CloneDataOnly:
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
