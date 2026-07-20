//go:build windows

package servicing

import (
	"errors"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// analyzeComponentStoreArgs is the fixed, user-input-free DISM argument array
// for read-only component-store analysis (ADR 0029). It is launched directly as
// an argument vector — never a shell command string — and is equivalent to
// `DISM /Online /Cleanup-Image /AnalyzeComponentStore /English /NoRestart`.
var analyzeComponentStoreArgs = []string{
	"/Online",
	"/Cleanup-Image",
	"/AnalyzeComponentStore",
	"/English",
	"/NoRestart",
}

// dismExecutableName is the only executable the helper will launch, resolved
// exclusively from the Windows system directory.
const dismExecutableName = "Dism.exe"

// resolveSystemDISM returns the absolute path to the system DISM executable,
// obtained only through the Windows system-directory API, after validating that
// it is an ordinary, non-reparse file. It never consults PATH, %WINDIR%, other
// environment variables, the current directory, or a caller-supplied path.
func resolveSystemDISM() (string, error) {
	dir, err := systemDirectory()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, dismExecutableName)
	if err := validateOrdinaryExecutable(path); err != nil {
		return "", err
	}
	return path, nil
}

// systemDirectory returns the Windows system directory (for example
// C:\Windows\System32) via GetSystemDirectory. The path is never taken from the
// environment or the current directory.
func systemDirectory() (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("servicing: empty system directory")
	}
	return dir, nil
}

// validateOrdinaryExecutable confirms path names an existing ordinary file that
// is neither a directory nor a reparse point (symlink, junction, or mount
// point), so PATH, environment, or CWD substitution cannot redirect the launch
// to another target. It reads only file attributes, never file contents.
func validateOrdinaryExecutable(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return errors.New("servicing: system DISM path is a directory")
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("servicing: system DISM path is a reparse point")
	}
	return nil
}

// runComponentStoreAnalysis resolves and launches the system DISM directly with
// the fixed analysis argument array, captures bounded output, and classifies it
// fail-closed. It never uses PATH, a shell, cmd.exe, PowerShell, or a
// caller-supplied argument, and returns a path-free, byte-free result.
func runComponentStoreAnalysis() clean.ServicingAnalysisResult {
	dismPath, err := resolveSystemDISM()
	if err != nil {
		return failResult(clean.ServicingReasonToolUnavailable)
	}
	// Re-validate the ordinary non-reparse identity immediately before launch, so
	// a race after resolution cannot swap the target.
	if err := validateOrdinaryExecutable(dismPath); err != nil {
		return failResult(clean.ServicingReasonToolUnavailable)
	}

	// Construct the command explicitly from the absolute path so exec performs no
	// PATH lookup, and pin the working directory to the system directory rather
	// than an inherited CWD.
	cmd := &exec.Cmd{
		Path: dismPath,
		Args: append([]string{dismPath}, analyzeComponentStoreArgs...),
		Dir:  filepath.Dir(dismPath),
	}
	var out boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// Failure to launch DISM at all (not a nonzero exit).
			return failResult(clean.ServicingReasonToolUnavailable)
		}
	}
	return classifyComponentStoreAnalysis(exitCode, out.String())
}

// boundedBuffer captures at most maxAnalysisOutputBytes of DISM output. Excess
// is discarded so pathological output cannot exhaust memory; truncated output
// simply fails the strict parser closed.
type boundedBuffer struct {
	data []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := maxAnalysisOutputBytes - len(b.data)
	if remaining > 0 {
		if len(p) <= remaining {
			b.data = append(b.data, p...)
		} else {
			b.data = append(b.data, p[:remaining]...)
		}
	}
	// Report all bytes consumed so the child process is never blocked on a full
	// buffer; the excess is intentionally dropped.
	return len(p), nil
}

func (b *boundedBuffer) String() string { return string(b.data) }
