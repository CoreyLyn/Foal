package uninstall

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
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
	// SkipElevationRequiredNotGranted: the app likely needs administrator
	// rights (machine-wide / HKLM install) and the injectable ElevationPort
	// did not grant elevation for this batch. Foal fail-closes by skipping
	// the app with this stable reason rather than attempting an uninstaller
	// that would fail with an opaque permission error. Uninstall-only
	// (ADR 0028); Clean/Purge never request elevation.
	SkipElevationRequiredNotGranted = "elevation_required_not_granted"
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

// Leftover action and result stable values. These describe the actual
// per-path leftover deletion outcome after a successful uninstaller.
const (
	// ActionLeftoverRecycleBin is the actual action taken for a leftover
	// path that was moved to the Recycle Bin.
	ActionLeftoverRecycleBin = "recycle_bin"
	// ActionLeftoverSkipped means the leftover path was not deleted.
	ActionLeftoverSkipped = "skipped"

	// ResultLeftoverDeleted means the leftover path was moved to the Recycle
	// Bin successfully.
	ResultLeftoverDeleted = "deleted"
	// ResultLeftoverSkipped means the leftover path was not deleted.
	ResultLeftoverSkipped = "skipped"
)

// Leftover skip reason codes (JSON contract). These are the documented
// reasons that appear in LeftoverPathOutcome.Reason when a leftover path is
// not deleted. They mirror pathsafe.Reason codes so consumers can rely on
// one vocabulary across Clean, Purge, and Uninstall.
const (
	// SkipLeftoverProtected: the path is protected by a user-defined
	// Protection rule (deny-only) and was skipped, never force-deleted.
	SkipLeftoverProtected = "protected_path"
	// SkipLeftoverMissing: the path does not exist anymore (already cleaned
	// by the uninstaller or the user). It is a revalidated subset skip, not
	// an error.
	SkipLeftoverMissing = "stat_failed"
	// SkipLeftoverReparse: the path is a reparse point and cannot be cleaned
	// by default.
	SkipLeftoverReparse = "reparse_point"
	// SkipLeftoverHardlink: the file has multiple hardlinks and cannot be
	// cleaned by default.
	SkipLeftoverHardlink = "hardlink_path"
	// SkipLeftoverPermission: permission was denied while validating or
	// moving the path.
	SkipLeftoverPermission = "permission_denied"
	// SkipLeftoverUnsupported: the target cannot be moved to the Recycle Bin.
	SkipLeftoverUnsupported = "unsupported_target"
	// SkipLeftoverDeleteFailed: the Recycle Bin adapter returned an error
	// that does not map to a more specific code.
	SkipLeftoverDeleteFailed = "delete_failed"
	// SkipLeftoverContextCanceled: the context was canceled before the path
	// could be processed.
	SkipLeftoverContextCanceled = "context_canceled"
	// SkipLeftoverProtectionNotLoaded: the Protection configuration could not
	// be loaded; leftover deletion is skipped entirely as a fail-closed
	// measure. This is recorded per-path when no Validator is available.
	SkipLeftoverProtectionNotLoaded = "protection_not_loaded"
	// SkipLeftoverUnknown is a defensive code for paths whose outcome was not
	// reported by the delete executor. It should not occur in practice.
	SkipLeftoverUnknown = "leftover_outcome_missing"
)

// ProtectionLoadIssue describes a Protection configuration load failure.
// When non-nil on ExecuteOptions, Execute fail-closes before any mutation:
// no uninstaller is invoked and no leftover is deleted. This mirrors
// Clean/Purge fail-closed behavior on Protection load errors.
type ProtectionLoadIssue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

// LeftoverPathOutcome records the actual per-path leftover deletion outcome
// for one path in the Confirmed leftover path set. Populated only after a
// successful uninstaller (ResultUninstalled); each entry records whether the
// path was moved to the Recycle Bin or skipped with a stable reason.
type LeftoverPathOutcome struct {
	Path string `json:"path"`
	// Action is the actual action taken: "recycle_bin" (deleted via Recycle
	// Bin) or "skipped" (not deleted).
	Action string `json:"action"`
	// Result is the terminal outcome: "deleted" or "skipped".
	Result string `json:"result"`
	// Reason is a stable skip code when Result is "skipped".
	Reason string `json:"reason,omitempty"`
	// Detail is a human-readable explanation.
	Detail string `json:"detail,omitempty"`
}

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

	// Validator is the deny-only Protection rule validator used to revalidate
	// each leftover path immediately before deletion. Production loads it
	// from clean.LoadProtectionConfiguration(); tests inject a real
	// pathsafe.Validator built from t.TempDir() paths. When nil and leftover
	// deletion would run, Execute fail-closes by skipping every leftover
	// path with SkipLeftoverProtectionNotLoaded (a missing validator means
	// Protection rules are not loaded).
	Validator pathsafe.Validator

	// ProtectionLoadError fail-closes the entire execute when the Protection
	// configuration could not be loaded. When non-nil Execute returns
	// StatusExecuteError without invoking any uninstaller or deleting any
	// leftover, matching Clean/Purge fail-closed behavior. The CLI sets
	// this from clean.LoadProtectionConfiguration().LoadError.
	ProtectionLoadError *ProtectionLoadIssue

	// RecycleBinAdapter moves leftover paths to the Recycle Bin. When nil
	// Execute uses the default Windows adapter (shell32 SHFileOperationW).
	// Tests inject a fake adapter so no real Recycle Bin API is ever
	// invoked and no real files are deleted.
	RecycleBinAdapter delete.Adapter

	// ElevationPort is the injectable seam through which Uninstall Execute
	// MAY request UAC when a selected app needs admin (ADR 0028). When nil
	// Execute does not request UAC itself and proceeds with current process
	// privileges (preserving machine-wide uninstall capability when Foal is
	// already elevated). Uninstall-only; Clean/Purge never wire this field.
	// Tests inject a fake so no real UAC prompt is ever triggered.
	ElevationPort ElevationPort
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

// ElevationPort is the injectable seam through which Uninstall Execute MAY
// request Windows administrator consent (UAC) when a selected app needs it.
// It is Uninstall-ONLY (ADR 0028): Clean, Purge, and every other command stay
// non-elevating and never call this port. Tests inject a fake so no real UAC
// prompt is ever triggered.
//
// RequestElevation is called at most once per Execute batch, and only when at
// least one selected app likely requires admin (HKLM / machine-wide source).
// When granted is false the caller skips admin-required apps with the stable
// SkipElevationRequiredNotGranted reason; non-admin apps proceed unaffected.
//
// When no port is wired (nil) Execute does not request UAC itself and proceeds
// with the current process privileges: a user running Foal elevated can still
// uninstall machine-wide apps, matching pre-elevation-port capability. A
// future slice may wire a real Windows UAC request; tests inject a fake that
// grants or denies to exercise both paths without real UAC.
type ElevationPort interface {
	RequestElevation(ctx context.Context) (granted bool, err error)
}

// ElevationOutcome records whether Uninstall Execute requested and obtained
// administrator consent for the batch. It is Uninstall-only metadata
// (ADR 0028): Clean/Purge results never carry elevation state.
type ElevationOutcome struct {
	// Requested is true when at least one selected app likely required admin.
	// When false, no admin-required app was selected and no port was called.
	Requested bool `json:"requested"`
	// Granted is true when admin-required apps may proceed. When Requested is
	// true and Granted is false, admin-required apps were skipped with
	// SkipElevationRequiredNotGranted.
	Granted bool `json:"granted"`
	// Reason is a stable, human-readable explanation of the outcome.
	Reason string `json:"reason,omitempty"`
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
	// Elevation records the Uninstall-only elevation outcome for the batch
	// (ADR 0028). It is absent (zero-value) on Clean/Purge results, which
	// never request elevation. Makes batch and elevation outcomes clear in
	// result metadata.
	Elevation ElevationOutcome `json:"elevation"`
	ElapsedMS int64            `json:"elapsed_ms"`
	Message   string           `json:"message,omitempty"`
}

// AppOutcome records what happened for one selected application.
type AppOutcome struct {
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	// PlannedClass is the preview-time classification (official_uninstaller,
	// portable_directory_removal, not_executable, hard_exclusion).
	PlannedClass string `json:"planned_class"`
	// RequiresAdmin reports whether this app likely needed administrator
	// rights (HKLM / machine-wide source). Disclosed per-app so the result
	// makes elevation outcomes clear alongside per-app skip reasons.
	RequiresAdmin bool `json:"requires_admin"`
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
	// LeftoverPaths is the frozen Confirmed leftover path set planned for
	// this app, captured from Possible leftovers (app-owned, high
	// confidence) at execute time. Populated only when the uninstaller
	// reports success; nil when the uninstaller fails, is canceled, or the
	// app is skipped. After success Foal may delete only a revalidated
	// subset of this set and never adds paths beyond it.
	LeftoverPaths []string `json:"leftover_paths,omitempty"`
	// LeftoverOutcomes records the actual per-path leftover deletion
	// outcome for paths in LeftoverPaths. Populated only after a successful
	// uninstaller; each entry records whether the path was moved to the
	// Recycle Bin or skipped with a stable reason (protected, missing,
	// reparse, etc). The number of entries matches LeftoverPaths.
	LeftoverOutcomes []LeftoverPathOutcome `json:"leftover_outcomes,omitempty"`
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
//  2. Protection fail-closed: if ProtectionLoadError is set the entire run
//     is aborted with StatusExecuteError before any uninstaller invocation
//     or leftover deletion (matches Clean/Purge).
//  3. Platform gate: non-Windows is refused; every selected app is skipped
//     with SkipUnsupportedPlatform.
//  4. Fresh discovery: Review() rediscovers installed applications so
//     execution never trusts a stale preview path. The review's
//     PossibleLeftovers (app-owned, high confidence) become the per-app
//     Confirmed leftover path set ceiling.
//  5. Per selected app in stable order: hard exclusion -> planned class
//     check -> process-stop gate -> Official uninstaller invocation
//     (quiet preferred, then interactive) -> on success only, revalidate
//     and delete the confirmed leftover subset via the Recycle Bin.
//  6. Leftover deletion runs ONLY after the uninstaller reports success
//     (ResultUninstalled). A failed or canceled uninstaller never deletes
//     leftovers and never falls back to Portable directory removal (#294).
//     The confirmed set is frozen from Possible leftovers and never
//     expanded by a post-uninstall deep scan.
//  7. History records the session, per-app outcomes, and per-leftover-path
//     outcomes.
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
			Actions: []string{ActionOfficialUninstaller, ActionLeftoverRecycleBin},
			Reason:  "uninstall execution authorized; official uninstaller invocation plus leftover deletion to Recycle Bin for selected apps after success",
		},
	}

	// Protection fail-closed: if the Protection configuration could not be
	// loaded, abort the entire execute before any mutation. No uninstaller
	// is invoked and no leftover is deleted. This matches Clean/Purge.
	if opts.ProtectionLoadError != nil {
		result.Status = StatusExecuteError
		code := opts.ProtectionLoadError.Code
		if code == "" {
			code = "protection_file_load_failed"
		}
		message := opts.ProtectionLoadError.Message
		if message == "" {
			message = "protection configuration could not be loaded"
		}
		result.Message = code + ": " + message
		// Record each selected app as skipped with the protection load
		// failure so consumers see why no work was done.
		for _, name := range stableSelection(opts.Selection) {
			result.Applications = append(result.Applications, AppOutcome{
				Name:          name,
				Action:        ActionSkipped,
				Result:        ResultSkipped,
				SkippedReason: code,
				Detail:        "uninstall execution aborted: " + result.Message,
			})
		}
		result.Totals = computeExecuteTotals(result.Applications)
		result.ElapsedMS = time.Since(start).Milliseconds()
		recordHistorySession(ctx, opts, result, start, time.Now())
		return result
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

	// Fresh discovery so execution never trusts stale preview paths. The
	// review's PossibleLeftovers (app-owned, high confidence) are the
	// ceiling for the per-app Confirmed leftover path set.
	review := Review()
	appsByLowerName := indexApplicationsByName(review.Applications)
	possibleLeftovers := review.PossibleLeftovers

	// Elevation (Uninstall-only, ADR 0028): request administrator consent
	// once per batch when at least one selected app likely needs admin
	// (HKLM / machine-wide source). The port is injectable so tests fake
	// it without real UAC. When elevation is not granted, admin-required
	// apps are skipped with SkipElevationRequiredNotGranted; non-admin apps
	// and cancellation semantics are unaffected. Clean/Purge never reach
	// this code path.
	elevationGranted := requestBatchElevation(ctx, opts, appsByLowerName, &result)

	for _, selectedName := range stableSelection(opts.Selection) {
		outcome := executeOneApp(ctx, opts, selectedName, appsByLowerName, possibleLeftovers, elevationGranted)
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

// requestBatchElevation decides whether admin-required apps may proceed. It
// is the ONLY elevation call site in Foal (ADR 0028): Clean, Purge, and other
// commands never request elevation.
//
// When no selected app requires admin, no request is made and the outcome
// reports Requested=false (non-admin batches never trigger UAC). When at
// least one admin-required app is selected:
//   - nil port (production default): Foal does not request UAC itself in this
//     slice; it proceeds with the current process privileges so a user running
//     Foal elevated can still uninstall machine-wide apps. Granted=true with a
//     reason that makes clear no UAC was prompted.
//   - wired port: the port is called once. A false result or an error skips
//     admin-required apps with SkipElevationRequiredNotGranted; a true result
//     proceeds. Tests inject a fake to exercise both paths without real UAC.
//
// Non-admin apps proceed regardless of the elevation decision. The decision is
// per-batch (one request at most), matching UAC semantics.
func requestBatchElevation(ctx context.Context, opts ExecuteOptions, appsByLowerName map[string]Application, result *ExecuteResult) bool {
	anyAdmin := false
	for _, selectedName := range stableSelection(opts.Selection) {
		app, ok := appsByLowerName[strings.ToLower(strings.TrimSpace(selectedName))]
		if !ok {
			continue
		}
		// Only apps Foal would actually try to execute (official_uninstaller)
		// can need elevation. Hard exclusions, portable-removal, and
		// not-executable apps never reach the uninstaller, so they do not
		// count toward an elevation request.
		if app.RequiresAdmin && app.PlannedClass == PlannedClassOfficialUninstaller {
			anyAdmin = true
			break
		}
	}
	if !anyAdmin {
		result.Elevation = ElevationOutcome{
			Requested: false,
			Granted:   false,
			Reason:    "no selected application requires administrator rights",
		}
		return false
	}

	if opts.ElevationPort == nil {
		result.Elevation = ElevationOutcome{
			Requested: true,
			Granted:   true,
			Reason:    "no elevation port configured; proceeding with current process privileges (Foal did not request UAC)",
		}
		return true
	}

	granted, err := opts.ElevationPort.RequestElevation(ctx)
	if err != nil {
		result.Elevation = ElevationOutcome{
			Requested: true,
			Granted:   false,
			Reason:    "elevation request failed: " + err.Error(),
		}
		return false
	}
	if granted {
		result.Elevation = ElevationOutcome{
			Requested: true,
			Granted:   true,
			Reason:    "elevation granted for admin-required applications",
		}
		return true
	}
	result.Elevation = ElevationOutcome{
		Requested: true,
		Granted:   false,
		Reason:    "elevation denied or unavailable; admin-required applications will be skipped",
	}
	return false
}

// executeOneApp runs the per-app pipeline for one selected application name.
// It returns an AppOutcome capturing the terminal state. The function never
// panics on behalf of an injected fake; a fake that panics terminates the
// batch as expected.
//
// possibleLeftovers is the review's PossibleLeftovers slice, used to freeze
// the Confirmed leftover path set for this app on uninstaller success only.
//
// elevationGranted carries the per-batch elevation decision (ADR 0028). When
// the app likely requires admin and elevation was not granted, the app is
// skipped with SkipElevationRequiredNotGranted before any uninstaller runs.
// Non-admin apps proceed regardless of elevation. This keeps per-app skip
// reasons stable when elevation is denied or unavailable.
func executeOneApp(ctx context.Context, opts ExecuteOptions, selectedName string, appsByLowerName map[string]Application, possibleLeftovers []LeftoverCandidate, elevationGranted bool) AppOutcome {
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

	requiresAdmin := app.RequiresAdmin

	// Hard exclusion takes precedence over everything else.
	if app.PlannedClass == PlannedClassHardExclusion {
		return AppOutcome{
			Name:          app.Name,
			Version:       app.Version,
			Publisher:     app.Publisher,
			PlannedClass:  app.PlannedClass,
			RequiresAdmin: requiresAdmin,
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
			RequiresAdmin: requiresAdmin,
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
			RequiresAdmin: requiresAdmin,
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
			RequiresAdmin: requiresAdmin,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipUninstallCommandMissing,
			Detail:        "app classified as official_uninstaller but no uninstall command is present",
		}
	}

	// Elevation gate (Uninstall-only, ADR 0028): an admin-required app is
	// skipped with a stable reason when elevation was not granted or is
	// unavailable. Foal does not attempt an uninstaller it cannot complete;
	// non-admin apps are unaffected. This keeps per-app skip reasons stable
	// when elevation is denied or unavailable.
	if requiresAdmin && !elevationGranted {
		return AppOutcome{
			Name:          app.Name,
			Version:       app.Version,
			Publisher:     app.Publisher,
			PlannedClass:  app.PlannedClass,
			RequiresAdmin: requiresAdmin,
			Action:        ActionSkipped,
			Result:        ResultSkipped,
			SkippedReason: SkipElevationRequiredNotGranted,
			Detail:        "application likely requires administrator rights and elevation was not granted; Foal does not attempt the uninstaller without it",
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
			RequiresAdmin: requiresAdmin,
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
			outcome.RequiresAdmin = requiresAdmin
			outcome.Detail = "uninstaller completed successfully; revalidating confirmed leftover set for Recycle Bin deletion"
			attachLeftoverOutcomes(ctx, opts, &outcome, possibleLeftovers, app.Name)
			return outcome
		}
		// Quiet failed or was canceled: fall back to interactive only when
		// the quiet attempt did not indicate cancellation. A canceled quiet
		// run must not start a second interactive process. Leftover deletion
		// is skipped on failure or cancel.
		if outcome.Result == ResultCanceled {
			outcome.Name = app.Name
			outcome.Version = app.Version
			outcome.Publisher = app.Publisher
			outcome.PlannedClass = app.PlannedClass
			outcome.RequiresAdmin = requiresAdmin
			return outcome
		}
		// Quiet failed: try interactive as the fallback if one exists.
		if app.InteractiveUninstallCommand != "" && app.InteractiveUninstallCommand != app.QuietUninstallCommand {
			interactiveOutcome := runUninstaller(ctx, runner, app.InteractiveUninstallCommand, CommandModeInteractive)
			interactiveOutcome.Name = app.Name
			interactiveOutcome.Version = app.Version
			interactiveOutcome.Publisher = app.Publisher
			interactiveOutcome.PlannedClass = app.PlannedClass
			interactiveOutcome.RequiresAdmin = requiresAdmin
			if interactiveOutcome.Result == ResultUninstalled {
				interactiveOutcome.Detail = "uninstaller completed successfully; revalidating confirmed leftover set for Recycle Bin deletion"
				attachLeftoverOutcomes(ctx, opts, &interactiveOutcome, possibleLeftovers, app.Name)
			}
			return interactiveOutcome
		}
		// No interactive fallback: report the quiet failure. Leftovers are
		// not deleted on failure.
		outcome.Name = app.Name
		outcome.Version = app.Version
		outcome.Publisher = app.Publisher
		outcome.PlannedClass = app.PlannedClass
		outcome.RequiresAdmin = requiresAdmin
		return outcome
	}

	// No quiet command: run interactive directly.
	outcome := runUninstaller(ctx, runner, app.InteractiveUninstallCommand, CommandModeInteractive)
	outcome.Name = app.Name
	outcome.Version = app.Version
	outcome.Publisher = app.Publisher
	outcome.PlannedClass = app.PlannedClass
	outcome.RequiresAdmin = requiresAdmin
	if outcome.Result == ResultUninstalled {
		outcome.Detail = "uninstaller completed successfully; revalidating confirmed leftover set for Recycle Bin deletion"
		attachLeftoverOutcomes(ctx, opts, &outcome, possibleLeftovers, app.Name)
	}
	return outcome
}

// attachLeftoverOutcomes freezes the Confirmed leftover path set for the app
// from possibleLeftovers and, when the set is non-empty, revalidates and
// deletes a subset via the Recycle Bin. It mutates outcome in place. This
// is called ONLY after the uninstaller reports success (ResultUninstalled);
// the caller must not invoke it on failure or cancel paths.
func attachLeftoverOutcomes(ctx context.Context, opts ExecuteOptions, outcome *AppOutcome, possibleLeftovers []LeftoverCandidate, appName string) {
	confirmed := confirmedLeftoverPaths(possibleLeftovers, appName)
	outcome.LeftoverPaths = confirmed
	outcome.LeftoverOutcomes = deleteLeftovers(ctx, opts, confirmed)
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
		Detail:           "uninstaller completed successfully",
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

// confirmedLeftoverPaths freezes the Confirmed leftover path set for one app
// from the review's Possible leftovers. Only app-owned, high-confidence
// paths whose App field matches the selected app (case-insensitive) enter
// the set. Shared-state, unknown-state, and orphaned residue never enter.
// The returned slice is the frozen upper bound: post-success revalidation
// may delete only a subset and must never add paths beyond it.
//
// This is the "ceiling" from ADR 0027 and the Uninstall execution spec:
// confirmation freezes the upper bound from Possible leftovers disclosed at
// confirmation; after a successful Official uninstaller Foal may delete only
// a revalidated subset of that set and must never add paths that were not
// confirmed.
func confirmedLeftoverPaths(possible []LeftoverCandidate, appName string) []string {
	target := strings.ToLower(strings.TrimSpace(appName))
	if target == "" || len(possible) == 0 {
		return nil
	}
	var confirmed []string
	seen := make(map[string]bool)
	for _, candidate := range possible {
		if strings.ToLower(strings.TrimSpace(candidate.App)) != target {
			continue
		}
		// PossibleLeftovers only contains app_owned+high candidates (the
		// review classifier routes shared_state and unknown to other
		// slices), but we defend against a future classifier change by
		// re-checking ownership and confidence here.
		if candidate.Ownership != "app_owned" || candidate.Confidence != "high" {
			continue
		}
		path := strings.TrimSpace(candidate.Path)
		if path == "" {
			continue
		}
		identity := pathsafe.NormalizePathForIdentity(path)
		if identity == "" || seen[identity] {
			continue
		}
		seen[identity] = true
		confirmed = append(confirmed, path)
	}
	return confirmed
}

// deleteLeftovers revalidates and deletes a revalidated subset of the frozen
// confirmed leftover path set for one app. It runs only after the
// uninstaller reports success (ResultUninstalled). Each path is revalidated
// immediately before deletion via the injected pathsafe.Validator (which
// enforces Protection rules, reparse rejection, hardlink rejection, and
// system-root exclusion). Protected paths are skipped, never force-deleted.
// The Recycle Bin adapter is injectable so tests never invoke the real
// Recycle Bin API. The function never adds paths beyond the confirmed set.
//
// Safety: the fail-closed boundary for missing Protection is in Execute,
// which returns StatusExecuteError when ProtectionLoadError is set before
// any mutation. deleteLeftovers is only reached after that gate passes, so
// opts.Validator is always a loaded validator in production. The defensive
// nil-adapter fallback selects the real Windows adapter, which is never
// exercised in tests (tests inject a fake adapter).
func deleteLeftovers(ctx context.Context, opts ExecuteOptions, confirmed []string) []LeftoverPathOutcome {
	if len(confirmed) == 0 {
		return nil
	}

	adapter := opts.RecycleBinAdapter
	if adapter == nil {
		adapter = delete.WindowsRecycleBinAdapter{}
	}

	candidates := make([]delete.Candidate, 0, len(confirmed))
	for _, path := range confirmed {
		candidates = append(candidates, delete.Candidate{Path: path})
	}

	deleteResult := delete.ExecuteWithValidator(ctx, candidates, adapter, opts.Validator)

	// Map results back to the confirmed-set order so the JSON contract is
	// deterministic and consumers can pair LeftoverPaths[i] with
	// LeftoverOutcomes[i].
	deletedByPath := make(map[string]struct{}, len(deleteResult.Deleted))
	for _, item := range deleteResult.Deleted {
		deletedByPath[item.Path] = struct{}{}
	}
	skippedByPath := make(map[string]LeftoverPathOutcome, len(deleteResult.Skipped))
	for _, item := range deleteResult.Skipped {
		skippedByPath[item.Path] = LeftoverPathOutcome{
			Path:   item.Path,
			Action: ActionLeftoverSkipped,
			Result: ResultLeftoverSkipped,
			Reason: item.Reason.Code,
			Detail: item.Reason.Message,
		}
	}

	outcomes := make([]LeftoverPathOutcome, 0, len(confirmed))
	for _, path := range confirmed {
		if _, ok := deletedByPath[path]; ok {
			outcomes = append(outcomes, LeftoverPathOutcome{
				Path:   path,
				Action: ActionLeftoverRecycleBin,
				Result: ResultLeftoverDeleted,
				Detail: "leftover path moved to the Recycle Bin",
			})
			continue
		}
		if outcome, ok := skippedByPath[path]; ok {
			outcomes = append(outcomes, outcome)
			continue
		}
		// Defensive: delete.ExecuteWithValidator classifies every candidate
		// as either Deleted or Skipped, so this branch should not occur.
		outcomes = append(outcomes, LeftoverPathOutcome{
			Path:   path,
			Action: ActionLeftoverSkipped,
			Result: ResultLeftoverSkipped,
			Reason: SkipLeftoverUnknown,
			Detail: "leftover path outcome was not reported by the delete executor",
		})
	}
	return outcomes
}
