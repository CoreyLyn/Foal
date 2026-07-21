//go:build windows

package servicing

import (
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

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

// startComponentCleanupArgs is the fixed, user-input-free DISM argument array
// for confirmed component-store cleanup (ADR 0029), equivalent to
// `DISM /Online /Cleanup-Image /StartComponentCleanup /English /NoRestart`.
// /ResetBase, /SPSuperseded, /Remove-Package, and any custom argument are
// deliberately absent and cannot be added over the wire.
var startComponentCleanupArgs = []string{
	"/Online",
	"/Cleanup-Image",
	"/StartComponentCleanup",
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
	exitCode, output, ok := runSystemDISM(analyzeComponentStoreArgs)
	if !ok {
		return failResult(clean.ServicingReasonToolUnavailable)
	}
	return classifyComponentStoreAnalysis(exitCode, output)
}

// runComponentStoreCleanup is the composite execute capability: it performs a
// fresh AnalyzeComponentStore, enforces the guard, and starts
// StartComponentCleanup only when analysis is ready (positive reclaimable
// packages explicitly recommended) — all within this one elevated helper
// session, so preview analysis never authorizes mutation and no inter-process
// analysis-to-mutation gap exists. A non-ready analysis is projected as the
// composite outcome (no_work or failed) with no mutation.
func runComponentStoreCleanup() clean.ServicingExecuteResult {
	analysis := runComponentStoreAnalysis()
	proceed, projected := projectAnalysisForExecute(analysis)
	if !proceed {
		return projected
	}
	// Sample Windows-volume free space immediately around StartComponentCleanup
	// only, so the observation excludes the preceding fresh analysis and every
	// earlier Clean action group (ADR 0029). A read failure simply yields no
	// observation.
	beforeFree, beforeOK := volumeFreeBytes()
	exitCode, _, ok := runSystemDISM(startComponentCleanupArgs)
	if !ok {
		return failExecuteResult(clean.ServicingReasonToolUnavailable)
	}
	afterFree, afterOK := volumeFreeBytes()
	var observed *int64
	if beforeOK && afterOK && afterFree >= beforeFree {
		delta := int64(afterFree - beforeFree)
		observed = &delta
	}
	return cleanupResultFromExit(analysis, exitCode, observed)
}

// getDiskFreeSpaceEx is resolved from kernel32 directly, mirroring
// internal/status; the x/sys wrapper is not relied on so this stays available
// across sys versions.
var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// volumeFreeBytes returns the total free bytes on the volume that hosts the
// Windows system directory. ok is false when the volume free space could not be
// read at all, in which case the caller attaches no free-space observation. It
// is used only to observe disk change around a completed cleanup; it never
// gates or authorizes the mutation.
func volumeFreeBytes() (freeBytes uint64, ok bool) {
	dir, err := systemDirectory()
	if err != nil {
		return 0, false
	}
	pathPtr, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, false
	}
	var availableToCaller, totalBytes, totalFree uint64
	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&availableToCaller)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if ret == 0 {
		return 0, false
	}
	return totalFree, true
}

// runSystemDISM resolves the system DISM, re-validates its ordinary non-reparse
// identity immediately before launch, and runs it directly with the fixed
// argument array. It returns the DISM exit code and bounded output; ok is false
// only when DISM could not be resolved or launched at all (distinct from a
// non-zero exit). It never uses PATH, %WINDIR%, environment overrides, CWD, a
// shell, cmd.exe, PowerShell, or a caller-supplied argument.
func runSystemDISM(args []string) (exitCode int, output string, ok bool) {
	dismPath, err := resolveSystemDISM()
	if err != nil {
		return 0, "", false
	}
	// Re-validate the ordinary non-reparse identity immediately before launch, so
	// a race after resolution cannot swap the target.
	if err := validateOrdinaryExecutable(dismPath); err != nil {
		return 0, "", false
	}

	// Construct the command explicitly from the absolute path so exec performs no
	// PATH lookup, and pin the working directory to the system directory rather
	// than an inherited CWD.
	cmd := &exec.Cmd{
		Path: dismPath,
		Args: append([]string{dismPath}, args...),
		Dir:  filepath.Dir(dismPath),
	}
	var out boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return exitErr.ExitCode(), out.String(), true
		}
		// Failure to launch DISM at all (not a non-zero exit).
		return 0, "", false
	}
	return 0, out.String(), true
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
