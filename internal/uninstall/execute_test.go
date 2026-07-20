package uninstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

// fakeUninstallerRunner records every Run call in order and returns the
// queued result. When the queue is exhausted it returns a default success
// so a test that under-configures still terminates rather than panicking.
type fakeUninstallerRunner struct {
	calls   []fakeRunnerCall
	results []UninstallerRunResult
	errs    []error
	callIdx int
}

type fakeRunnerCall struct {
	Command string
	Mode    string // set by the test via the call context, not by the runner
}

func (f *fakeUninstallerRunner) Run(_ context.Context, command string) (UninstallerRunResult, error) {
	f.calls = append(f.calls, fakeRunnerCall{Command: command})
	if f.callIdx < len(f.errs) && f.errs[f.callIdx] != nil {
		err := f.errs[f.callIdx]
		f.callIdx++
		return UninstallerRunResult{}, err
	}
	var result UninstallerRunResult
	if f.callIdx < len(f.results) {
		result = f.results[f.callIdx]
	}
	f.callIdx++
	return result, nil
}

// fakeProcessDetector returns configurable states per app name.
type fakeProcessDetector struct {
	states map[string]ProcessState
	err    error
}

func (f fakeProcessDetector) IsRunning(_ context.Context, appName string) (ProcessState, error) {
	if f.err != nil {
		return ProcessState{State: ProcessStateUnknown, Message: f.err.Error()}, f.err
	}
	if state, ok := f.states[appName]; ok {
		return state, nil
	}
	return ProcessState{State: ProcessStateIdle}, nil
}

// fakeHistoryRecorder captures the session and items for assertions.
type fakeHistoryRecorder struct {
	sessions []fakeHistorySession
}

type fakeHistorySession struct {
	Session history.SessionRecord
	Items   []history.ItemRecord
}

func (f *fakeHistoryRecorder) Record(_ context.Context, session history.SessionRecord, items []history.ItemRecord) error {
	f.sessions = append(f.sessions, fakeHistorySession{
		Session: session,
		Items:   append([]history.ItemRecord(nil), items...),
	})
	return nil
}

// stubDiscovery replaces the three discovery vars with fakes that return the
// given applications and no leftover/orphaned evidence. Tests call this to
// avoid touching the real Windows registry or filesystem.
func stubDiscovery(t *testing.T, apps []ApplicationEvidence) {
	t.Helper()
	stubDiscoveryWithLeftovers(t, apps, nil)
}

// stubDiscoveryWithLeftovers replaces the discovery vars with fakes that
// return the given applications and leftover evidence. The leftover evidence
// flows through the review classifier so app_owned+under_user_profile signals
// become PossibleLeftovers (the Confirmed leftover path set source).
func stubDiscoveryWithLeftovers(t *testing.T, apps []ApplicationEvidence, leftovers []LeftoverEvidence) {
	t.Helper()
	originalDiscover := discoverUninstallEvidence
	discoverUninstallEvidence = func() DiscoveryResult {
		return DiscoveryResult{
			Evidence: Evidence{Applications: apps},
			Sources:  []EvidenceSource{{Source: "windows_registry_uninstall_keys:HKLM64", Status: "reported"}},
		}
	}
	t.Cleanup(func() { discoverUninstallEvidence = originalDiscover })

	originalLeftover := discoverLeftoverEvidence
	discoverLeftoverEvidence = func([]ApplicationEvidence) LeftoverDiscoveryResult {
		return LeftoverDiscoveryResult{
			Leftovers: leftovers,
			Source:    EvidenceSource{Source: "known_leftover_locations", Status: "reported"},
		}
	}
	t.Cleanup(func() { discoverLeftoverEvidence = originalLeftover })

	stubOrphanedResidue(t, OrphanedResidueDiscoveryResult{
		Source: EvidenceSource{Source: orphanedResidueSource, Status: "reported"},
	})
}

// fakeRecycleBinAdapter records every MoveToRecycleBin call and never
// actually deletes anything. Tests inject this so no real Recycle Bin API
// is invoked and no real files are deleted. An optional err is returned for
// every call when set (to exercise the delete_failed skip path).
type fakeRecycleBinAdapter struct {
	calls []string
	err   error
}

func (f *fakeRecycleBinAdapter) MoveToRecycleBin(path string) error {
	f.calls = append(f.calls, path)
	if f.err != nil {
		return f.err
	}
	return nil
}

// appOwnedLeftover builds LeftoverEvidence with the signals required for
// app_owned classification (app_name_match + under_user_profile). The path
// must be a real directory on disk for pathsafe.Validator.ValidateDeletePath
// to pass (it calls Lstat).
func appOwnedLeftover(app, path string) LeftoverEvidence {
	return LeftoverEvidence{
		Path:    path,
		App:     app,
		Signals: []string{"app_name_match", "under_user_profile"},
	}
}

// mkdirTempLeftover creates a real directory under t.TempDir() so the real
// pathsafe.Validator can Lstat it. The directory is automatically cleaned up
// by the testing framework.
func mkdirTempLeftover(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir leftover %q: %v", dir, err)
	}
	return dir
}

func TestExecutePrefersQuietUninstallerAndDoesNotRunInteractive(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                        "Quiet App",
		QuietUninstallCommand:       `MsiExec.exe /X{A} /qn`,
		InteractiveUninstallCommand: `MsiExec.exe /X{A}`,
		Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Quiet App"},
		UninstallerRunner: runner,
	})

	if result.Status != StatusExecuteOK {
		t.Fatalf("status = %q, want %q", result.Status, StatusExecuteOK)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (quiet only; no interactive fallback on success)", len(runner.calls))
	}
	if runner.calls[0].Command != `MsiExec.exe /X{A} /qn` {
		t.Fatalf("runner call[0] = %q, want quiet command", runner.calls[0].Command)
	}
	if len(result.Applications) != 1 {
		t.Fatalf("applications = %d, want 1", len(result.Applications))
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if app.CommandMode != CommandModeQuiet {
		t.Fatalf("command mode = %q, want %q", app.CommandMode, CommandModeQuiet)
	}
	if app.LeftoverPaths != nil {
		t.Fatalf("leftover paths = %#v, want nil (not deleted in this slice)", app.LeftoverPaths)
	}
	if result.Totals.UninstalledCount != 1 {
		t.Fatalf("uninstalled count = %d, want 1", result.Totals.UninstalledCount)
	}
}

func TestExecuteFallsBackToInteractiveWhenQuietFails(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                        "Stubborn App",
		QuietUninstallCommand:       `MsiExec.exe /X{B} /qn`,
		InteractiveUninstallCommand: `"C:\Program Files\Stubborn App\uninstall.exe"`,
		Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{
			{ExitCode: 1603, Stderr: "quiet uninstall failed"}, // quiet fails
			{ExitCode: 0}, // interactive succeeds
		},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Stubborn App"},
		UninstallerRunner: runner,
	})

	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %d, want 2 (quiet then interactive)", len(runner.calls))
	}
	if runner.calls[0].Command != `MsiExec.exe /X{B} /qn` {
		t.Fatalf("call[0] = %q, want quiet", runner.calls[0].Command)
	}
	if runner.calls[1].Command != `"C:\Program Files\Stubborn App\uninstall.exe"` {
		t.Fatalf("call[1] = %q, want interactive", runner.calls[1].Command)
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q (interactive succeeded)", app.Result, ResultUninstalled)
	}
	if app.CommandMode != CommandModeInteractive {
		t.Fatalf("command mode = %q, want %q", app.CommandMode, CommandModeInteractive)
	}
}

func TestExecuteDoesNotFallBackToInteractiveWhenQuietCanceled(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                        "Cancellable App",
		QuietUninstallCommand:       `MsiExec.exe /X{C} /qn`,
		InteractiveUninstallCommand: `MsiExec.exe /X{C}`,
		Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{
			{ExitCode: -1, Canceled: true}, // quiet canceled
		},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Cancellable App"},
		UninstallerRunner: runner,
	})

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (canceled quiet must not start interactive)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultCanceled {
		t.Fatalf("result = %q, want %q", app.Result, ResultCanceled)
	}
	if app.LeftoverPaths != nil {
		t.Fatalf("leftover paths = %#v, want nil (canceled uninstaller must not delete leftovers)", app.LeftoverPaths)
	}
}

func TestExecuteRunsInteractiveWhenNoQuietCommand(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                        "Interactive Only App",
		InteractiveUninstallCommand: `"C:\Program Files\App\uninstall.exe"`,
		Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Interactive Only App"},
		UninstallerRunner: runner,
	})

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (interactive only)", len(runner.calls))
	}
	if runner.calls[0].Command != `"C:\Program Files\App\uninstall.exe"` {
		t.Fatalf("call[0] = %q, want interactive command", runner.calls[0].Command)
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if app.CommandMode != CommandModeInteractive {
		t.Fatalf("command mode = %q, want %q", app.CommandMode, CommandModeInteractive)
	}
}

func TestExecuteSkipsRunningAppWhenStopNotAuthorized(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Running App",
		QuietUninstallCommand: `MsiExec.exe /X{D} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{}
	detector := fakeProcessDetector{
		states: map[string]ProcessState{
			"Running App": {State: ProcessStateRunning, Message: "running.exe"},
		},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Running App"},
		UninstallerRunner: runner,
		ProcessDetector:   detector,
	})

	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (running app must not be uninstalled without stop auth)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != SkipProcessRunningStopNotAuthorized {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, SkipProcessRunningStopNotAuthorized)
	}
	if result.Totals.SkippedCount != 1 || result.Totals.UninstalledCount != 0 {
		t.Fatalf("totals = %#v, want 1 skipped 0 uninstalled", result.Totals)
	}
}

func TestExecuteProceedsWithRunningAppWhenStopAuthorized(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Running App",
		QuietUninstallCommand: `MsiExec.exe /X{E} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	detector := fakeProcessDetector{
		states: map[string]ProcessState{
			"Running App": {State: ProcessStateRunning, Message: "running.exe"},
		},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:          []string{"Running App"},
		AllowStopProcesses: true,
		UninstallerRunner:  runner,
		ProcessDetector:    detector,
	})

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (stop authorized proceeds with uninstaller)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
}

func TestExecuteSkipsHardExclusion(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Foal",
		QuietUninstallCommand: `MsiExec.exe /X{FOAL} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Foal"},
		UninstallerRunner: runner,
	})

	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (hard exclusion must never run)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != SkipHardExclusion {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, SkipHardExclusion)
	}
}

func TestExecuteSkipsPortableRemovalAsScopeLimit(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:            "Portable App",
		InstallLocation: `C:\Apps\PortableApp`,
		Sources:         []string{"windows_registry_uninstall_keys:HKCU"},
	}})
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Portable App"},
		UninstallerRunner: runner,
	})

	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (portable removal is out of scope)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != SkipPortableRemovalNotSupported {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, SkipPortableRemovalNotSupported)
	}
}

func TestExecuteSkipsNotExecutableApp(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:    "Bare App",
		Sources: []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Bare App"},
		UninstallerRunner: runner,
	})

	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != SkipNotExecutable {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, SkipNotExecutable)
	}
}

func TestExecuteSkipsAppNotFound(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Real App",
		QuietUninstallCommand: `MsiExec.exe /X{F} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Nonexistent App"},
		UninstallerRunner: runner,
	})

	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != SkipAppNotFound {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, SkipAppNotFound)
	}
}

func TestExecuteBatchContinuesAfterFailure(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{
		{
			Name:                  "Failing App",
			QuietUninstallCommand: `MsiExec.exe /X{G} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		},
		{
			Name:                  "Success App",
			QuietUninstallCommand: `MsiExec.exe /X{H} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		},
	})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{
			{ExitCode: 1603, Stderr: "failed"}, // Failing App quiet fails
			// No interactive fallback for Failing App (no interactive command)
			{ExitCode: 0}, // Success App quiet succeeds
		},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Failing App", "Success App"},
		UninstallerRunner: runner,
	})

	if len(result.Applications) != 2 {
		t.Fatalf("applications = %d, want 2 (batch must continue after failure)", len(result.Applications))
	}
	// stableSelection sorts alphabetically: "Failing App" before "Success App"
	if result.Applications[0].Name != "Failing App" || result.Applications[0].Result != ResultFailed {
		t.Fatalf("app[0] = %#v, want Failing App/failed", result.Applications[0])
	}
	if result.Applications[1].Name != "Success App" || result.Applications[1].Result != ResultUninstalled {
		t.Fatalf("app[1] = %#v, want Success App/uninstalled", result.Applications[1])
	}
	if result.Totals.FailedCount != 1 || result.Totals.UninstalledCount != 1 {
		t.Fatalf("totals = %#v, want 1 failed 1 uninstalled", result.Totals)
	}
}

func TestExecuteFailedUninstallerDoesNotDeleteLeftovers(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                        "Failed App",
		QuietUninstallCommand:       `MsiExec.exe /X{I} /qn`,
		InteractiveUninstallCommand: `MsiExec.exe /X{I}`,
		Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{
			{ExitCode: 1603}, // quiet fails
			{ExitCode: 1603}, // interactive also fails
		},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Failed App"},
		UninstallerRunner: runner,
	})

	app := result.Applications[0]
	if app.Result != ResultFailed {
		t.Fatalf("result = %q, want %q", app.Result, ResultFailed)
	}
	if app.LeftoverPaths != nil {
		t.Fatalf("leftover paths = %#v, want nil (failed uninstaller must not delete leftovers)", app.LeftoverPaths)
	}
	if app.SkippedReason != ReasonUninstallerFailed {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, ReasonUninstallerFailed)
	}
}

func TestExecuteRecordsHistoryWithPerAppOutcomes(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{
		{
			Name:                  "History App",
			QuietUninstallCommand: `MsiExec.exe /X{J} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		},
		{
			Name:    "Bare App",
			Sources: []string{"windows_registry_uninstall_keys:HKLM64"},
		},
	})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	recorder := &fakeHistoryRecorder{}

	Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"History App", "Bare App"},
		UninstallerRunner: runner,
		HistoryRecorder:   recorder,
	})

	if len(recorder.sessions) != 1 {
		t.Fatalf("history sessions = %d, want 1", len(recorder.sessions))
	}
	session := recorder.sessions[0]
	if !strings.HasPrefix(session.Session.ID, "uninstall-") {
		t.Fatalf("session ID = %q, want uninstall- prefix", session.Session.ID)
	}
	if session.Session.Command.Command != "uninstall" {
		t.Fatalf("command = %q, want uninstall", session.Session.Command.Command)
	}
	if session.Session.Mode != ModeExecute {
		t.Fatalf("mode = %q, want %q", session.Session.Mode, ModeExecute)
	}
	if session.Session.Aggregate.CandidateCount != 2 {
		t.Fatalf("candidate count = %d, want 2", session.Session.Aggregate.CandidateCount)
	}
	if session.Session.Aggregate.DeletedCount != 1 {
		t.Fatalf("deleted count = %d, want 1 (History App uninstalled)", session.Session.Aggregate.DeletedCount)
	}
	if session.Session.Aggregate.SkippedCount != 1 {
		t.Fatalf("skipped count = %d, want 1 (Bare App skipped)", session.Session.Aggregate.SkippedCount)
	}
	if len(session.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(session.Items))
	}
	// stableSelection sorts alphabetically: "Bare App" before "History App"
	if session.Items[0].Result != ResultSkipped {
		t.Fatalf("item[0] result = %q, want %q (Bare App)", session.Items[0].Result, ResultSkipped)
	}
	if session.Items[0].SkippedReason == nil || session.Items[0].SkippedReason.Code != SkipNotExecutable {
		t.Fatalf("item[0] skipped reason = %#v, want %q", session.Items[0].SkippedReason, SkipNotExecutable)
	}
	if session.Items[1].Result != ResultUninstalled {
		t.Fatalf("item[1] result = %q, want %q (History App)", session.Items[1].Result, ResultUninstalled)
	}
}

func TestExecuteNonWindowsSkipsAllWithUnsupportedPlatform(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Any App",
		QuietUninstallCommand: `MsiExec.exe /X{K} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{}

	originalGOOS := runtimeGOOS
	runtimeGOOS = "linux"
	t.Cleanup(func() { runtimeGOOS = originalGOOS })

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Any App"},
		UninstallerRunner: runner,
	})

	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (non-Windows must not mutate)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != SkipUnsupportedPlatform {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, SkipUnsupportedPlatform)
	}
}

func TestExecuteDefaultProcessDetectorIsUsedWhenNoneInjected(t *testing.T) {
	// This test verifies the wiring path: when ProcessDetector is nil, Execute
	// constructs the default detector and calls it without panicking. We do
	// not assert on the real process list; we only verify the app proceeds
	// (idle or unknown -> proceed) and the runner is called.
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Detector Wiring App",
		QuietUninstallCommand: `MsiExec.exe /X{L} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Detector Wiring App"},
		UninstallerRunner: runner,
		// ProcessDetector intentionally nil: Execute must use the default.
	})

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (default detector must not block idle app)", len(runner.calls))
	}
	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
}

func TestExecuteRunnerErrorRecordsRunError(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Broken Command App",
		QuietUninstallCommand: `not-a-real-command`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		errs: []error{errors.New("command parse failed")},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Broken Command App"},
		UninstallerRunner: runner,
	})

	app := result.Applications[0]
	if app.Result != ResultFailed {
		t.Fatalf("result = %q, want %q", app.Result, ResultFailed)
	}
	if app.SkippedReason != ReasonUninstallerRunError {
		t.Fatalf("skipped reason = %q, want %q", app.SkippedReason, ReasonUninstallerRunError)
	}
}

func TestExecuteSelectionIsDeduplicatedAndSorted(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Dedup App",
		QuietUninstallCommand: `MsiExec.exe /X{M} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Dedup App", "Dedup App", "  Dedup App  "},
		UninstallerRunner: runner,
	})

	if len(result.Applications) != 1 {
		t.Fatalf("applications = %d, want 1 (selection must dedupe)", len(result.Applications))
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.calls))
	}
}

func TestExecuteEmptySelectionProducesEmptyResult(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Unselected App",
		QuietUninstallCommand: `MsiExec.exe /X{N} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{},
		UninstallerRunner: runner,
	})

	if len(result.Applications) != 0 {
		t.Fatalf("applications = %d, want 0", len(result.Applications))
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
	if result.Totals.SelectedCount != 0 {
		t.Fatalf("totals = %#v, want 0 selected", result.Totals)
	}
}

func TestExecuteExecutionPolicyReportsAuthorizedActions(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Policy App",
		QuietUninstallCommand: `MsiExec.exe /X{O} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Policy App"},
		UninstallerRunner: runner,
	})

	if !result.Execution.Allowed {
		t.Fatal("execution.allowed = false, want true (execute is authorized)")
	}
	found := false
	for _, action := range result.Execution.Actions {
		if action == ActionOfficialUninstaller {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("execution actions = %#v, want %q present", result.Execution.Actions, ActionOfficialUninstaller)
	}
}

func TestExecuteCaseInsensitiveSelection(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Case App",
		QuietUninstallCommand: `MsiExec.exe /X{P} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"case app"},
		UninstallerRunner: runner,
	})

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (selection is case-insensitive)", len(runner.calls))
	}
	if result.Applications[0].Name != "Case App" {
		t.Fatalf("app name = %q, want Case App", result.Applications[0].Name)
	}
}

// TestExecuteDeletesLeftoversToRecycleBinAfterSuccess verifies that on a
// successful uninstaller Foal freezes the Confirmed leftover path set from
// PossibleLeftovers, revalidates each path, and moves it to the Recycle Bin
// via the injected adapter. The fake adapter records calls without deleting,
// so the real temp directories survive for assertion.
func TestExecuteDeletesLeftoversToRecycleBinAfterSuccess(t *testing.T) {
	leftoverPath := mkdirTempLeftover(t, "AppData")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                  "Leftover App",
			QuietUninstallCommand: `MsiExec.exe /X{Q} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{appOwnedLeftover("Leftover App", leftoverPath)},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	adapter := &fakeRecycleBinAdapter{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Leftover App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
	})

	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if len(app.LeftoverPaths) != 1 || app.LeftoverPaths[0] != leftoverPath {
		t.Fatalf("leftover paths = %#v, want [%q]", app.LeftoverPaths, leftoverPath)
	}
	if len(app.LeftoverOutcomes) != 1 {
		t.Fatalf("leftover outcomes = %d, want 1", len(app.LeftoverOutcomes))
	}
	outcome := app.LeftoverOutcomes[0]
	if outcome.Action != ActionLeftoverRecycleBin || outcome.Result != ResultLeftoverDeleted {
		t.Fatalf("outcome = %#v, want recycle_bin/deleted", outcome)
	}
	if len(adapter.calls) != 1 || adapter.calls[0] != leftoverPath {
		t.Fatalf("adapter calls = %#v, want [%q]", adapter.calls, leftoverPath)
	}
}

// TestExecuteLeftoverCeilingNeverExpandsBeyondConfirmedSet verifies that
// Foal deletes ONLY paths from the frozen Confirmed leftover path set and
// never adds new paths. A second directory that looks like app data but was
// not in the confirmed set must never be touched by the adapter.
func TestExecuteLeftoverCeilingNeverExpandsBeyondConfirmedSet(t *testing.T) {
	confirmedPath := mkdirTempLeftover(t, "Confirmed")
	// unconfirmedPath exists on disk and looks like app data but is NOT in
	// the PossibleLeftovers set. It must never be passed to the adapter.
	unconfirmedPath := mkdirTempLeftover(t, "Unconfirmed")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                  "Ceiling App",
			QuietUninstallCommand: `MsiExec.exe /X{R} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{appOwnedLeftover("Ceiling App", confirmedPath)},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	adapter := &fakeRecycleBinAdapter{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Ceiling App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
	})

	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if len(app.LeftoverPaths) != 1 || app.LeftoverPaths[0] != confirmedPath {
		t.Fatalf("leftover paths = %#v, want only [%q] (ceiling must not expand)", app.LeftoverPaths, confirmedPath)
	}
	if len(adapter.calls) != 1 || adapter.calls[0] != confirmedPath {
		t.Fatalf("adapter calls = %#v, want only [%q] (unconfirmed path must not be touched)", adapter.calls, confirmedPath)
	}
	for _, call := range adapter.calls {
		if call == unconfirmedPath {
			t.Fatalf("adapter was called with unconfirmed path %q (ceiling violated)", unconfirmedPath)
		}
	}
}

// TestExecuteFailedUninstallerSkipsLeftoverDeletion verifies that a failed
// uninstaller never triggers leftover deletion. The adapter must not be
// called and LeftoverPaths/LeftoverOutcomes must be nil.
func TestExecuteFailedUninstallerSkipsLeftoverDeletion(t *testing.T) {
	leftoverPath := mkdirTempLeftover(t, "ShouldNotBeDeleted")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                        "Fail App",
			QuietUninstallCommand:       `MsiExec.exe /X{S} /qn`,
			InteractiveUninstallCommand: `MsiExec.exe /X{S}`,
			Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{appOwnedLeftover("Fail App", leftoverPath)},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{
			{ExitCode: 1603}, // quiet fails
			{ExitCode: 1603}, // interactive also fails
		},
	}
	adapter := &fakeRecycleBinAdapter{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Fail App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
	})

	app := result.Applications[0]
	if app.Result != ResultFailed {
		t.Fatalf("result = %q, want %q", app.Result, ResultFailed)
	}
	if app.LeftoverPaths != nil {
		t.Fatalf("leftover paths = %#v, want nil (failed uninstaller must not freeze leftover set)", app.LeftoverPaths)
	}
	if app.LeftoverOutcomes != nil {
		t.Fatalf("leftover outcomes = %#v, want nil (failed uninstaller must not delete leftovers)", app.LeftoverOutcomes)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("adapter calls = %#v, want 0 (failed uninstaller must not delete leftovers)", adapter.calls)
	}
}

// TestExecuteCanceledUninstallerSkipsLeftoverDeletion verifies that a
// canceled uninstaller never triggers leftover deletion.
func TestExecuteCanceledUninstallerSkipsLeftoverDeletion(t *testing.T) {
	leftoverPath := mkdirTempLeftover(t, "ShouldNotBeDeleted")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                        "Cancel App",
			QuietUninstallCommand:       `MsiExec.exe /X{T} /qn`,
			InteractiveUninstallCommand: `MsiExec.exe /X{T}`,
			Sources:                     []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{appOwnedLeftover("Cancel App", leftoverPath)},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: -1, Canceled: true}},
	}
	adapter := &fakeRecycleBinAdapter{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Cancel App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
	})

	app := result.Applications[0]
	if app.Result != ResultCanceled {
		t.Fatalf("result = %q, want %q", app.Result, ResultCanceled)
	}
	if app.LeftoverPaths != nil {
		t.Fatalf("leftover paths = %#v, want nil (canceled uninstaller must not freeze leftover set)", app.LeftoverPaths)
	}
	if app.LeftoverOutcomes != nil {
		t.Fatalf("leftover outcomes = %#v, want nil (canceled uninstaller must not delete leftovers)", app.LeftoverOutcomes)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("adapter calls = %#v, want 0 (canceled uninstaller must not delete leftovers)", adapter.calls)
	}
}

// TestExecuteProtectionSuppressesLeftoverTargets verifies that a path
// protected by a user-defined Protection rule is skipped, never
// force-deleted. The deny-only validator removes the protected path from the
// deletion set; a sibling unprotected path is still deleted.
func TestExecuteProtectionSuppressesLeftoverTargets(t *testing.T) {
	protectedPath := mkdirTempLeftover(t, "Protected")
	freePath := mkdirTempLeftover(t, "Free")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                  "Protect App",
			QuietUninstallCommand: `MsiExec.exe /X{U} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{
			appOwnedLeftover("Protect App", protectedPath),
			appOwnedLeftover("Protect App", freePath),
		},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	adapter := &fakeRecycleBinAdapter{}
	validator := pathsafe.NewValidator([]string{protectedPath})

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Protect App"},
		UninstallerRunner: runner,
		Validator:         validator,
		RecycleBinAdapter: adapter,
	})

	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if len(app.LeftoverPaths) != 2 {
		t.Fatalf("leftover paths = %#v, want 2 (frozen set includes both)", app.LeftoverPaths)
	}
	if len(app.LeftoverOutcomes) != 2 {
		t.Fatalf("leftover outcomes = %d, want 2", len(app.LeftoverOutcomes))
	}
	// The free path must be deleted; the protected path must be skipped.
	freeDeleted := false
	protectedSkipped := false
	for _, outcome := range app.LeftoverOutcomes {
		if outcome.Path == freePath && outcome.Result == ResultLeftoverDeleted {
			freeDeleted = true
		}
		if outcome.Path == protectedPath && outcome.Result == ResultLeftoverSkipped && outcome.Reason == SkipLeftoverProtected {
			protectedSkipped = true
		}
	}
	if !freeDeleted {
		t.Fatalf("free path %q was not deleted (outcomes = %#v)", freePath, app.LeftoverOutcomes)
	}
	if !protectedSkipped {
		t.Fatalf("protected path %q was not skipped with protected_path (outcomes = %#v)", protectedPath, app.LeftoverOutcomes)
	}
	if len(adapter.calls) != 1 || adapter.calls[0] != freePath {
		t.Fatalf("adapter calls = %#v, want only [%q] (protected path must not reach the adapter)", adapter.calls, freePath)
	}
}

// TestExecuteProtectionLoadErrorFailsClosed verifies that a Protection
// configuration load error aborts the entire execute before any uninstaller
// invocation or leftover deletion. This matches Clean/Purge fail-closed
// behavior.
func TestExecuteProtectionLoadErrorFailsClosed(t *testing.T) {
	stubDiscovery(t, []ApplicationEvidence{{
		Name:                  "Protect Fail App",
		QuietUninstallCommand: `MsiExec.exe /X{V} /qn`,
		Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
	}})
	runner := &fakeUninstallerRunner{}
	adapter := &fakeRecycleBinAdapter{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection: []string{"Protect Fail App"},
		ProtectionLoadError: &ProtectionLoadIssue{
			Code:    "protection_file_load_failed",
			Message: "protection file could not be read",
		},
		UninstallerRunner: runner,
		RecycleBinAdapter: adapter,
	})

	if result.Status != StatusExecuteError {
		t.Fatalf("status = %q, want %q (fail-closed on Protection load error)", result.Status, StatusExecuteError)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 (no uninstaller on Protection load error)", len(runner.calls))
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("adapter calls = %d, want 0 (no leftover deletion on Protection load error)", len(adapter.calls))
	}
	if len(result.Applications) != 1 {
		t.Fatalf("applications = %d, want 1 (selected app recorded as skipped)", len(result.Applications))
	}
	app := result.Applications[0]
	if app.Result != ResultSkipped {
		t.Fatalf("result = %q, want %q", app.Result, ResultSkipped)
	}
	if app.SkippedReason != "protection_file_load_failed" {
		t.Fatalf("skipped reason = %q, want protection_file_load_failed", app.SkippedReason)
	}
}

// TestExecuteRecordsLeftoverPathHistory verifies that history records one
// ItemRecord per leftover path outcome in addition to the app-level record.
func TestExecuteRecordsLeftoverPathHistory(t *testing.T) {
	leftoverPath := mkdirTempLeftover(t, "HistoryLeftover")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                  "History Leftover App",
			QuietUninstallCommand: `MsiExec.exe /X{W} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{appOwnedLeftover("History Leftover App", leftoverPath)},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	adapter := &fakeRecycleBinAdapter{}
	recorder := &fakeHistoryRecorder{}

	Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"History Leftover App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
		HistoryRecorder:   recorder,
	})

	if len(recorder.sessions) != 1 {
		t.Fatalf("history sessions = %d, want 1", len(recorder.sessions))
	}
	items := recorder.sessions[0].Items
	// One app-level item + one leftover-path item.
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (1 app + 1 leftover path)", len(items))
	}
	// The app-level item has no path; the leftover item carries the path.
	var appItem, leftoverItem *history.ItemRecord
	for i := range items {
		if items[i].Path == "" {
			appItem = &items[i]
		} else if items[i].Path == leftoverPath {
			leftoverItem = &items[i]
		}
	}
	if appItem == nil {
		t.Fatal("app-level history item (empty path) not found")
	}
	if appItem.Result != ResultUninstalled {
		t.Fatalf("app item result = %q, want %q", appItem.Result, ResultUninstalled)
	}
	if leftoverItem == nil {
		t.Fatalf("leftover path item for %q not found in history", leftoverPath)
	}
	if leftoverItem.Rule != "leftover" {
		t.Fatalf("leftover item rule = %q, want leftover", leftoverItem.Rule)
	}
	if leftoverItem.PlannedAction != ActionLeftoverRecycleBin {
		t.Fatalf("leftover item planned action = %q, want %q", leftoverItem.PlannedAction, ActionLeftoverRecycleBin)
	}
	if leftoverItem.Action != ActionLeftoverRecycleBin || leftoverItem.Result != ResultLeftoverDeleted {
		t.Fatalf("leftover item = %#v, want recycle_bin/deleted", leftoverItem)
	}
}

// TestExecuteLeftoverRevalidationSkipsMissingPath verifies that a path in
// the confirmed set that no longer exists (already cleaned by the
// uninstaller) is skipped, not treated as an error. This is the
// "revalidated subset" behavior: Foal may delete only paths that still
// exist and pass checks.
func TestExecuteLeftoverRevalidationSkipsMissingPath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "AlreadyCleaned")
	// Note: missingPath is NOT created on disk, so Lstat fails and
	// pathsafe.Validator rejects it with stat_failed.
	existingPath := mkdirTempLeftover(t, "StillThere")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                  "Revalidate App",
			QuietUninstallCommand: `MsiExec.exe /X{X} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{
			appOwnedLeftover("Revalidate App", missingPath),
			appOwnedLeftover("Revalidate App", existingPath),
		},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	adapter := &fakeRecycleBinAdapter{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Revalidate App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
	})

	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	if len(app.LeftoverPaths) != 2 {
		t.Fatalf("leftover paths = %d, want 2 (frozen set has both)", len(app.LeftoverPaths))
	}
	if len(app.LeftoverOutcomes) != 2 {
		t.Fatalf("leftover outcomes = %d, want 2", len(app.LeftoverOutcomes))
	}
	// The existing path must be deleted; the missing path must be skipped.
	existingDeleted := false
	missingSkipped := false
	for _, outcome := range app.LeftoverOutcomes {
		if outcome.Path == existingPath && outcome.Result == ResultLeftoverDeleted {
			existingDeleted = true
		}
		if outcome.Path == missingPath && outcome.Result == ResultLeftoverSkipped && outcome.Reason == SkipLeftoverMissing {
			missingSkipped = true
		}
	}
	if !existingDeleted {
		t.Fatalf("existing path %q was not deleted", existingPath)
	}
	if !missingSkipped {
		t.Fatalf("missing path %q was not skipped with stat_failed (outcomes = %#v)", missingPath, app.LeftoverOutcomes)
	}
	if len(adapter.calls) != 1 || adapter.calls[0] != existingPath {
		t.Fatalf("adapter calls = %#v, want only [%q] (missing path must not reach adapter)", adapter.calls, existingPath)
	}
}

// TestExecuteSharedStateAndUnknownNeverEnterConfirmedSet verifies that
// leftover evidence classified as shared_state or unknown never enters the
// Confirmed leftover path set and is never deleted. Only app_owned +
// high-confidence paths enter the set.
func TestExecuteSharedStateAndUnknownNeverEnterConfirmedSet(t *testing.T) {
	appOwnedPath := mkdirTempLeftover(t, "AppOwned")
	// sharedStatePath carries the shared_program_data signal, so the review
	// classifier routes it to SharedStateConcerns, not PossibleLeftovers.
	sharedStatePath := mkdirTempLeftover(t, "SharedState")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                  "Shared App",
			QuietUninstallCommand: `MsiExec.exe /X{Y} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{
			appOwnedLeftover("Shared App", appOwnedPath),
			{
				Path:    sharedStatePath,
				App:     "Shared App",
				Signals: []string{"shared_program_data"},
			},
		},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	adapter := &fakeRecycleBinAdapter{}

	result := Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Shared App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
	})

	app := result.Applications[0]
	if app.Result != ResultUninstalled {
		t.Fatalf("result = %q, want %q", app.Result, ResultUninstalled)
	}
	// Only the app_owned path is in the confirmed set.
	if len(app.LeftoverPaths) != 1 || app.LeftoverPaths[0] != appOwnedPath {
		t.Fatalf("leftover paths = %#v, want only [%q] (shared state must not enter confirmed set)", app.LeftoverPaths, appOwnedPath)
	}
	for _, call := range adapter.calls {
		if call == sharedStatePath {
			t.Fatalf("shared state path %q was sent to the adapter (must never be deleted)", sharedStatePath)
		}
	}
}

// TestExecuteRecycleBinAdapterInjectionIsUsedNotRealAPI verifies that the
// injected RecycleBinAdapter is called instead of the real Windows Recycle
// Bin API. The fake adapter records the call and does not delete the file,
// so the real directory must still exist after Execute returns.
func TestExecuteRecycleBinAdapterInjectionIsUsedNotRealAPI(t *testing.T) {
	leftoverPath := mkdirTempLeftover(t, "RealFileNotDeleted")
	stubDiscoveryWithLeftovers(t,
		[]ApplicationEvidence{{
			Name:                  "Inject App",
			QuietUninstallCommand: `MsiExec.exe /X{Z} /qn`,
			Sources:               []string{"windows_registry_uninstall_keys:HKLM64"},
		}},
		[]LeftoverEvidence{appOwnedLeftover("Inject App", leftoverPath)},
	)
	runner := &fakeUninstallerRunner{
		results: []UninstallerRunResult{{ExitCode: 0}},
	}
	adapter := &fakeRecycleBinAdapter{}

	Execute(context.Background(), ExecuteOptions{
		Selection:         []string{"Inject App"},
		UninstallerRunner: runner,
		Validator:         pathsafe.NewValidator(nil),
		RecycleBinAdapter: adapter,
	})

	if len(adapter.calls) != 1 {
		t.Fatalf("adapter calls = %d, want 1", len(adapter.calls))
	}
	// The real directory must still exist (the fake adapter does not delete).
	if info, err := os.Stat(leftoverPath); err != nil || !info.IsDir() {
		t.Fatalf("leftover path %q was deleted by the real Recycle Bin API (must survive when fake adapter is injected)", leftoverPath)
	}
}
