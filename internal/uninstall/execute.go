package uninstall

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/history"
)

// Execute status and mode constants. These mirror the stable vocabulary used
// by Clean and Purge so JSON consumers can rely on one shape across commands.
const (
	StatusExecuteOK       = "ok"
	StatusExecuteError    = "error"
	StatusExecuteCanceled = "canceled"

	ModeExecute = "execute"
	ModePreview = "preview"
)

// Action and result stable values for per-app outcomes.
const (
	// ActionOfficialUninstaller is the action Foal attempted: it ran the
	// registry-advertised uninstall command for this app.
	ActionOfficialUninstaller = "official_uninstaller"
	// ActionSkipped means Foal did not attempt any uninstaller for this app.
	ActionSkipped = "skipped"

	// ResultUninstalled means the uninstaller completed successfully.
	ResultUninstalled = "uninstalled"
	// ResultSkipped means the app was skipped before any uninstaller ran.
	ResultSkipped = "skipped"
	// ResultFailed means the uninstaller ran but reported failure.
	ResultFailed = "failed"
	// ResultCanceled means the uninstaller was interrupted by context cancel.
	ResultCanceled = "canceled"

	// CommandModeQuiet is the quiet uninstall command path.
	CommandModeQuiet = "quiet"
	// CommandModeInteractive is the interactive fallback command path.
	CommandModeInteractive = "interactive"
)

// Stable skip reason codes (JSON contract). These are the documented reasons
// that appear in AppOutcome.SkippedReason when an app is not uninstalled.
const (
	// SkipProcessRunningStopNotAuthorized: the app is running and the caller
	// did not authorize process stopping. Foal never kills a process without
	// that authorization; the app is skipped with this stable reason instead.
	SkipProcessRunningStopNotAuthorized = "process_running_stop_not_authorized"
	// SkipHardExclusion: the app is on the Uninstall hard exclusion denylist
	// (including Foal itself) and Foal never offers it for execution.
	SkipHardExclusion = "hard_exclusion"
	// SkipNotExecutable: the app has no uninstall command and no install
	// location, so Foal cannot plan any execution for it.
	SkipNotExecutable = "not_executable"
	// SkipPortableRemovalNotSupported: the app has no uninstall command but
	// has an install location, which would map to Portable directory removal.
	// That path belongs to #294 and is intentionally not executed in this
	// slice; the app is skipped rather than force-deleted.
	SkipPortableRemovalNotSupported = "portable_removal_not_supported"
	// SkipUninstallCommandMissing: the app classified as official_uninstaller
	// but neither a quiet nor an interactive command is present. This is a
	// defensive guard; classification should prevent this case.
	SkipUninstallCommandMissing = "uninstall_command_missing"
	// SkipAppNotFound: the caller selected an app name that does not match
	// any discovered installed application.
	SkipAppNotFound = "app_not_found"
	// SkipUnsupportedPlatform: Foal only mutates on Windows. On other
	// platforms execution is refused and every selected app is skipped.
	SkipUnsupportedPlatform = "unsupported_platform"
)

// Failed/canceled runner outcome reason codes.
const (
	// ReasonUninstallerFailed: the uninstaller runner returned a non-zero
	// exit code. Leftovers are NOT deleted on this outcome.
	ReasonUninstallerFailed = "uninstaller_failed"
	// ReasonUninstallerCanceled: the uninstaller was interrupted by context
	// cancellation. Leftovers are NOT deleted on this outcome.
	ReasonUninstallerCanceled = "uninstaller_canceled"
	// ReasonUninstallerRunError: the runner could not start or complete the
	// uninstaller process (e.g. the command string could not be parsed).
	ReasonUninstallerRunError = "uninstaller_run_error"
)

// ExecuteOptions configures Uninstall execution. The CLI and TUI build this
// from parsed flags and pass it to Execute; tests inject fakes for the
// runner, process detector, and history recorder so no real vendor
// uninstaller is ever invoked in tests.
type ExecuteOptions struct {
	// Selection is the exact set of application display names the user
	// confirmed for uninstall. Matching is case-insensitive and trims
	// surrounding whitespace; names not present in fresh discovery are
	// skipped with SkipAppNotFound.
	Selection []string

	// AllowStopProcesses is the separate per-run authorization for process
	// stopping. It is distinct from execute authorization: --execute alone
	// never stops or kills a process. When false (the default) a running app
	// is skipped with SkipProcessRunningStopNotAuthorized. When true Foal
	// proceeds to run the uninstaller; actual process termination is a
	// future concern and is not performed in this slice.
	AllowStopProcesses bool

	// UninstallerRunner runs one uninstall command string. When nil Execute
	// uses the default Windows runner (cmd /c). Tests inject a fake.
	UninstallerRunner UninstallerRunner

	// ProcessDetector reports whether an application is currently running.
	// When nil Execute uses the default Windows detector (Toolhelp32
	// snapshot). Tests inject a fake.
	ProcessDetector ProcessDetector

	// HistoryRecorder optionally records the uninstall session and per-app
	// outcomes. When nil no history is written (tests that do not assert
	// history leave this empty).
	HistoryRecorder history.Recorder

	// CommandParameters identify the invocation in History.
	CommandParameters history.CommandParameters
}

// UninstallerRunner runs one uninstall command string and returns its
// outcome. Implementations must respect context cancellation: a canceled
// context surfaces as Canceled=true rather than an error. The runner must
// not kill the process except in response to context cancellation.
type UninstallerRunner interface {
	Run(ctx context.Context, command string) (UninstallerRunResult, error)
}

// UninstallerRunResult is the outcome of one uninstaller invocation.
type UninstallerRunResult struct {
	// ExitCode is the process exit code. 0 means success.
	ExitCode int
	// Stdout captures the uninstaller's standard output, truncated for safety.
	Stdout string
	// Stderr captures the uninstaller's standard error, truncated for safety.
	Stderr string
	// Canceled is true when the context was canceled mid-run.
	Canceled bool
}

// ProcessState is the three-state running-application check result. It
// mirrors Clean's running-application detection vocabulary: "running" means
// Foal must not proceed without process-stop authorization, "idle" means
// Foal may proceed, and "unknown" means Foal proceeds cautiously (the
// uninstaller itself will report failure if files are locked).
type ProcessState struct {
	State   string
	Message string
}

// ProcessState values.
const (
	ProcessStateRunning = "running"
	ProcessStateIdle    = "idle"
	ProcessStateUnknown = "unknown"
)

// ProcessDetector reports whether an application is currently running. The
// default Windows implementation snapshots running processes via Toolhelp32.
type ProcessDetector interface {
	IsRunning(ctx context.Context, appName string) (ProcessState, error)
}

// ExecuteResult is the JSON-contract read model for Uninstall execution. It
// is distinct from the preview Result so preview and execute contracts can
// evolve independently.
type ExecuteResult struct {
	Status       string          `json:"status"`
	Mode         string          `json:"mode"`
	Applications []AppOutcome    `json:"applications"`
	Totals       ExecuteTotals   `json:"totals"`
	Execution    ExecutionPolicy `json:"execution"`
	ElapsedMS    int64           `json:"elapsed_ms"`
	Message      string          `json:"message,omitempty"`
}

// AppOutcome records what happened for one selected application.
type AppOutcome struct {
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	// PlannedClass is the preview-time classification (official_uninstaller,
	// portable_directory_removal, not_executable, hard_exclusion).
	PlannedClass string `json:"planned_class"`
	// Action is what Foal attempted: "official_uninstaller" or "skipped".
	Action string `json:"action"`
	// Result is the terminal outcome: "uninstalled", "skipped", "failed",
	// or "canceled".
	Result string `json:"result"`
	// AttemptedCommand is the command string Foal ran, when any.
	AttemptedCommand string `json:"attempted_command,omitempty"`
	// CommandMode is "quiet" or "interactive" for the attempted command.
	CommandMode string `json:"command_mode,omitempty"`
	// SkippedReason is a stable code when Result is "skipped" or "failed".
	SkippedReason string `json:"skipped_reason,omitempty"`
	// Detail is a human-readable explanation of the outcome.
	Detail string `json:"detail,omitempty"`
	// LeftoverPaths planned for this app. Empty in this slice; leftover
	// deletion ships in #292 and is never performed when the uninstaller
	// fails or is canceled.
	LeftoverPaths []string `json:"leftover_paths,omitempty"`
}

// ExecuteTotals aggregates per-app outcomes for the result.
type ExecuteTotals struct {
	SelectedCount    int `json:"selected_count"`
	UninstalledCount int `json:"uninstalled_count"`
	SkippedCount     int `json:"skipped_count"`
	FailedCount      int `json:"failed_count"`
	CanceledCount    int `json:"canceled_count"`
}

// Execute runs the confirmed Uninstall mutation for selected traditional
// desktop applications through the shared Uninstall Execute seam. It is the
// only mutation path: the CLI parses selection and authorizations then calls
// Execute.
//
// Pipeline (per ADR 0027 and docs/plan/uninstall-execution-spec.md):
//  1. Authorize: --execute is required (enforced by the CLI; Execute itself
//     runs mutation unconditionally but is only reached when the CLI parsed
//     --execute).
//  2. Platform gate: non-Windows is refused; every selected app is skipped
//     with SkipUnsupportedPlatform.
//  3. Fresh discovery: Review() rediscovers installed applications so
//     execution never trusts a stale preview path.
//  4. Per selected app in stable order: hard exclusion -> planned class
//     check -> process-stop gate -> Official uninstaller invocation
//     (quiet preferred, then interactive) -> outcome.
//  5. Leftover deletion is NOT performed in this slice (#292). A failed or
//     canceled uninstaller never deletes leftovers and never falls back to
//     Portable directory removal (#294).
//  6. History records the session and per-app outcomes.
//
// Cancellation stops remaining apps without rollback. A canceled uninstaller
// is recorded as ResultCanceled with no leftover deletion.
func Execute(ctx context.Context, opts ExecuteOptions) ExecuteResult {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()

	result := ExecuteResult{
		Status:       StatusExecuteOK,
		Mode:         ModeExecute,
		Applications: []AppOutcome{},
		Execution: ExecutionPolicy{
			Allowed: true,
			Actions: []string{ActionOfficialUninstaller},
			Reason:  "uninstall execution authorized; official uninstaller invocation only; leftover deletion is not performed in this slice",
		},
	}

	// Platform gate: Uninstall mutation is Windows-only. On other platforms
	// every selected app is skipped with a stable reason so the contract is
	// honest without faking mutation.
	if runtimeGOOS != "windows" {
		for _, name := range stableSelection(opts.Selection) {
			result.Applications = append(result.Applications, AppOutcome{
				Name:          name,
				Action:        ActionSkipped,
				Result:        ResultSkipped,
				SkippedReason: SkipUnsupportedPlatform,
				Detail:        "uninstall execution is Windows-only; no mutation performed on this platform",
			})
		}
		result.Totals = computeExecuteTotals(result.Applications)
		result.ElapsedMS = time.Since(start).Milliseconds()
		recordHistorySession(ctx, opts, result, start, time.Now())
		return result
	}

	// Fresh discovery so execution never trusts stale preview paths.
	review := Review()
	appsByLowerName := indexApplicationsByName(review.Applications)

	for _, selectedName := range stableSelection(opts.Selection) {
		outcome := executeOneApp(ctx, opts, selectedName, appsByLowerName)
		result.Applications = append(result.Applications, outcome)
		if ctx.Err() != nil {
			// Record the cancel on the running app, then stop scheduling more.
			result.Status = StatusExecuteCanceled
			result.Message = "uninstall execution canceled; completed work is not rolled back"
			break
		}
	}

	// If context was canceled before the loop processed any app, the status
	// stays OK with an empty selection. Promote to canceled when at least one
	// app was canceled and nothing else completed.
	if ctx.Err() != nil && result.Status != StatusExecuteCanceled {
		result.Status = StatusExecuteCanceled
		result.Message = "uninstall execution canceled; completed work is not rolled back"
	}

	result.Totals = computeExecuteTotals(result.Applications)
	result.ElapsedMS = time.Since(start).Milliseconds()
	recordHistorySession(ctx, opts, result, start, time.Now())
	return result
}

// executeOneApp runs the per-app pipeline for one selected application name.
// It returns an AppOutcome capturing the terminal state. The function never
// panics on behalf of an injected fake; a fake that panics terminates the
// batch as expected.
func executeOneApp(ctx context.Context, opts ExecuteOptions, selectedName string, appsByLowerName map[string]Application) AppOutcome {
	app, ok := appsByLowerName[strings.ToLower(strings.TrimSpace(selectedName))]
	if !ok {
		return AppOutcome{
			Name:          selectedName,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipAppNotFound,
			Detail:        "selected application name does not match any discovered installed application",
		}
	}

	// Hard exclusion takes precedence over everything else.
	if app.PlannedClass == PlannedClassHardExclusion {
		return AppOutcome{
			Name:          app.Name,
			Version:       app.Version,
			Publisher:     app.Publisher,
			PlannedClass:  app.PlannedClass,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipHardExclusion,
			Detail:        "Foal never offers this application for Uninstall execution",
		}
	}

	// Portable directory removal is intentionally not executed in this slice
	// (#294). Skip with a stable reason rather than force-deleting the tree.
	if app.PlannedClass == PlannedClassPortableDirectoryRemoval {
		return AppOutcome{
			Name:          app.Name,
			Version:       app.Version,
			Publisher:     app.Publisher,
			PlannedClass:  app.PlannedClass,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipPortableRemovalNotSupported,
			Detail:        "portable directory removal is not executed in this slice; use the app's own uninstaller or wait for portable removal execution",
		}
	}

	// Only official_uninstaller proceeds. Not-executable apps are skipped.
	if app.PlannedClass != PlannedClassOfficialUninstaller {
		return AppOutcome{
			Name:          app.Name,
			Version:       app.Version,
			Publisher:     app.Publisher,
			PlannedClass:  app.PlannedClass,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipNotExecutable,
			Detail:        "no uninstall command and no install location; Foal cannot plan any execution for this application",
		}
	}

	// Defensive guard: classification says official_uninstaller but both
	// commands are empty. Skip rather than attempt nothing.
	if app.QuietUninstallCommand == "" && app.InteractiveUninstallCommand == "" {
		return AppOutcome{
			Name:          app.Name,
			Version:       app.Version,
			Publisher:     app.Publisher,
			PlannedClass:  app.PlannedClass,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipUninstallCommandMissing,
			Detail:        "app classified as official_uninstaller but no uninstall command is present",
		}
	}

	// Process-stop gate: if the app is running and the caller did not
	// authorize process stopping, skip with a stable reason. Foal never
	// kills a process without that authorization.
	state := detectProcessState(ctx, opts, app.Name)
	if state.State == ProcessStateRunning && !opts.AllowStopProcesses {
		return AppOutcome{
			Name:          app.Name,
			Version:       app.Version,
			Publisher:     app.Publisher,
			PlannedClass:  app.PlannedClass,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipProcessRunningStopNotAuthorized,
			Detail:        "application is running and process stopping is not authorized; Foal does not kill processes without explicit authorization",
		}
	}

	// Official uninstaller invocation: prefer quiet, then interactive.
	runner := opts.UninstallerRunner
	if runner == nil {
		runner = defaultUninstallerRunner{}
	}

	if app.QuietUninstallCommand != "" {
		outcome := runUninstaller(ctx, runner, app.QuietUninstallCommand, CommandModeQuiet)
		if outcome.Result == ResultUninstalled {
			outcome.Name = app.Name
			outcome.Version = app.Version
			outcome.Publisher = app.Publisher
			outcome.PlannedClass = app.PlannedClass
			outcome.LeftoverPaths = nil // not deleted in this slice
			return outcome
		}
		// Quiet failed or was canceled: fall back to interactive only when
		// the quiet attempt did not indicate cancellation. A canceled quiet
		// run must not start a second interactive process.
		if outcome.Result == ResultCanceled {
			outcome.Name = app.Name
			outcome.Version = app.Version
			outcome.Publisher = app.Publisher
			outcome.PlannedClass = app.PlannedClass
			return outcome
		}
		// Quiet failed: try interactive as the fallback if one exists.
		if app.InteractiveUninstallCommand != "" && app.InteractiveUninstallCommand != app.QuietUninstallCommand {
			interactiveOutcome := runUninstaller(ctx, runner, app.InteractiveUninstallCommand, CommandModeInteractive)
			interactiveOutcome.Name = app.Name
			interactiveOutcome.Version = app.Version
			interactiveOutcome.Publisher = app.Publisher
			interactiveOutcome.PlannedClass = app.PlannedClass
			interactiveOutcome.LeftoverPaths = nil
			return interactiveOutcome
		}
		// No interactive fallback: report the quiet failure.
		outcome.Name = app.Name
		outcome.Version = app.Version
		outcome.Publisher = app.Publisher
		outcome.PlannedClass = app.PlannedClass
		return outcome
	}

	// No quiet command: run interactive directly.
	outcome := runUninstaller(ctx, runner, app.InteractiveUninstallCommand, CommandModeInteractive)
	outcome.Name = app.Name
	outcome.Version = app.Version
	outcome.Publisher = app.Publisher
	outcome.PlannedClass = app.PlannedClass
	outcome.LeftoverPaths = nil
	return outcome
}

// runUninstaller invokes one uninstall command string via the runner and
// translates the result into an AppOutcome. It never deletes leftovers.
func runUninstaller(ctx context.Context, runner UninstallerRunner, command, mode string) AppOutcome {
	runResult, err := runner.Run(ctx, command)
	if err != nil {
		if ctx.Err() != nil {
			return AppOutcome{
				Action:           ActionOfficialUninstaller,
				Result:           ResultCanceled,
				AttemptedCommand: command,
				CommandMode:      mode,
				SkippedReason:    ReasonUninstallerCanceled,
				Detail:           "uninstaller was interrupted by context cancellation; leftovers were not deleted",
			}
		}
		return AppOutcome{
			Action:           ActionOfficialUninstaller,
			Result:           ResultFailed,
			AttemptedCommand: command,
			CommandMode:      mode,
			SkippedReason:    ReasonUninstallerRunError,
			Detail:           "uninstaller runner could not start or complete the command: " + err.Error(),
		}
	}
	if runResult.Canceled || ctx.Err() != nil {
		return AppOutcome{
			Action:           ActionOfficialUninstaller,
			Result:           ResultCanceled,
			AttemptedCommand: command,
			CommandMode:      mode,
			SkippedReason:    ReasonUninstallerCanceled,
			Detail:           "uninstaller was interrupted by context cancellation; leftovers were not deleted",
		}
	}
	if runResult.ExitCode != 0 {
		detail := "uninstaller exited with non-zero status; leftovers were not deleted"
		if runResult.Stderr != "" {
			detail += "; stderr: " + truncateForDetail(runResult.Stderr)
		}
		return AppOutcome{
			Action:           ActionOfficialUninstaller,
			Result:           ResultFailed,
			AttemptedCommand: command,
			CommandMode:      mode,
			SkippedReason:    ReasonUninstallerFailed,
			Detail:           detail,
		}
	}
	return AppOutcome{
		Action:           ActionOfficialUninstaller,
		Result:           ResultUninstalled,
		AttemptedCommand: command,
		CommandMode:      mode,
		Detail:           "uninstaller completed successfully; leftover deletion is not performed in this slice",
	}
}

// detectProcessState queries the injectable ProcessDetector. When no detector
// is wired the default Windows implementation is used. A detector error is
// treated as "unknown" so Foal proceeds cautiously rather than skipping
// every app when detection is temporarily unavailable.
func detectProcessState(ctx context.Context, opts ExecuteOptions, appName string) ProcessState {
	detector := opts.ProcessDetector
	if detector == nil {
		detector = defaultProcessDetector{}
	}
	state, err := detector.IsRunning(ctx, appName)
	if err != nil {
		return ProcessState{State: ProcessStateUnknown, Message: err.Error()}
	}
	return state
}

// indexApplicationsByName builds a case-insensitive lookup from display name
// to the review Application projection. The first app wins on duplicate
// names so the result is deterministic.
func indexApplicationsByName(apps []Application) map[string]Application {
	index := make(map[string]Application, len(apps))
	for _, app := range apps {
		key := strings.ToLower(strings.TrimSpace(app.Name))
		if key == "" {
			continue
		}
		if _, exists := index[key]; !exists {
			index[key] = app
		}
	}
	return index
}

// stableSelection trims, de-duplicates, and sorts the caller's selection so
// the per-app loop is deterministic and resilient to CLI repetition.
func stableSelection(selection []string) []string {
	seen := make(map[string]bool, len(selection))
	out := make([]string, 0, len(selection))
	for _, name := range selection {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func computeExecuteTotals(outcomes []AppOutcome) ExecuteTotals {
	totals := ExecuteTotals{SelectedCount: len(outcomes)}
	for _, outcome := range outcomes {
		switch outcome.Result {
		case ResultUninstalled:
			totals.UninstalledCount++
		case ResultSkipped:
			totals.SkippedCount++
		case ResultFailed:
			totals.FailedCount++
		case ResultCanceled:
			totals.CanceledCount++
		}
	}
	return totals
}

func truncateForDetail(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
