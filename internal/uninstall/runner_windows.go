//go:build windows

package uninstall

import (
	"bytes"
	"context"
	"os/exec"
)

// defaultUninstallerRunner runs a registry uninstall command string via
// cmd /c on Windows. The Windows UninstallString / QuietUninstallString
// values are shell command lines that may contain quotes, environment
// variables, and special characters; cmd /c is the documented interpreter
// for those strings. The runner respects context cancellation: a canceled
// context terminates the child process and surfaces as Canceled=true.
type defaultUninstallerRunner struct{}

// Run executes one uninstall command string. Stdout and stderr are captured
// for diagnostics; they are truncated by the caller when attached to an
// AppOutcome. A non-zero exit code is returned as a normal result (not an
// error) so the caller can classify it as a failure rather than a run error.
func (defaultUninstallerRunner) Run(ctx context.Context, command string) (UninstallerRunResult, error) {
	if command == "" {
		return UninstallerRunResult{}, errEmptyUninstallCommand
	}
	cmd := exec.CommandContext(ctx, "cmd", "/c", command)
	var stdout, stderr bytes.Buffer
	// Cap captured output so a chatty uninstaller cannot exhaust memory. The
	// caller truncates again when building the detail string.
	stdout.Grow(4096)
	stderr.Grow(4096)
	cmd.Stdout = &limitedWriter{w: &stdout, limit: 64 * 1024}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: 64 * 1024}

	err := cmd.Run()
	if ctx.Err() != nil {
		return UninstallerRunResult{
			ExitCode: -1,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			Canceled: true,
		}, nil
	}
	if err != nil {
		// A non-zero exit code reaches here as an *exec.ExitError; surface
		// the code and let the caller classify. Other errors (command not
		// found, parse failure) are returned as the error value so the
		// caller records ReasonUninstallerRunError.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return UninstallerRunResult{
				ExitCode: exitErr.ExitCode(),
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
			}, nil
		}
		return UninstallerRunResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}, err
	}
	return UninstallerRunResult{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// limitedWriter caps the number of bytes written to the underlying writer.
// Additional writes are silently dropped so a chatty uninstaller cannot
// exhaust memory. The limit is per-stream; the caller truncates again.
type limitedWriter struct {
	w     *bytes.Buffer
	limit int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.w.Len() >= lw.limit {
		return len(p), nil
	}
	remaining := lw.limit - lw.w.Len()
	if len(p) <= remaining {
		return lw.w.Write(p)
	}
	lw.w.Write(p[:remaining])
	return len(p), nil
}

var errEmptyUninstallCommand = newEmptyUninstallCommandError()

func newEmptyUninstallCommandError() error {
	// Defined as a value rather than errors.New so callers can compare with
	// errors.Is if they need to distinguish this from other errors.
	return &uninstallCommandError{message: "uninstall command string is empty"}
}

type uninstallCommandError struct{ message string }

func (e *uninstallCommandError) Error() string { return e.message }
