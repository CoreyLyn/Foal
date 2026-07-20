package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/uninstall"
)

func TestUninstallDefaultRemainsPreviewOnly(t *testing.T) {
	original := reviewUninstall
	reviewUninstall = func() uninstall.Result {
		return uninstall.WithReviewSections(uninstall.Result{
			Status: "preview",
			Applications: []uninstall.Application{{
				Name:       "Example App",
				Evidence:   []string{"windows_registry_uninstall_keys:HKLM64"},
				Confidence: "medium",
				Ownership:  "unknown",
			}},
			Execution: uninstall.ExecutionPolicy{
				Allowed: false,
				Actions: []string{},
				Reason:  "uninstall is preview-only",
			},
		})
	}
	t.Cleanup(func() { reviewUninstall = original })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "preview" {
		t.Fatalf("status = %v, want preview (no --execute)", result["status"])
	}
}

func TestUninstallExecuteWithoutExecuteFlagStaysPreview(t *testing.T) {
	original := reviewUninstall
	reviewUninstall = func() uninstall.Result {
		return uninstall.WithReviewSections(uninstall.Result{
			Status:       "preview",
			Applications: []uninstall.Application{},
			Execution: uninstall.ExecutionPolicy{
				Allowed: false,
				Actions: []string{},
				Reason:  "uninstall is preview-only",
			},
		})
	}
	t.Cleanup(func() { reviewUninstall = original })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--select", "Example App", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "preview" {
		t.Fatalf("status = %v, want preview (--select without --execute stays preview)", result["status"])
	}
}

func TestUninstallExecuteCallsExecuteWithSelection(t *testing.T) {
	disableHistoryRecording(t)
	original := executeUninstall
	var captured uninstall.ExecuteOptions
	executeUninstall = func(_ context.Context, opts uninstall.ExecuteOptions) uninstall.ExecuteResult {
		captured = opts
		return uninstall.ExecuteResult{
			Status:       uninstall.StatusExecuteOK,
			Mode:         uninstall.ModeExecute,
			Applications: []uninstall.AppOutcome{},
		}
	}
	t.Cleanup(func() { executeUninstall = original })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--execute", "--select", "Example App", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if len(captured.Selection) != 1 || captured.Selection[0] != "Example App" {
		t.Fatalf("selection = %#v, want [Example App]", captured.Selection)
	}
	var env struct {
		Command string                  `json:"command"`
		Result  uninstall.ExecuteResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.Command != "uninstall" || env.Result.Mode != uninstall.ModeExecute {
		t.Fatalf("envelope = %#v", env)
	}
}

func TestUninstallExecuteAcceptsBareNameAsSelection(t *testing.T) {
	disableHistoryRecording(t)
	original := executeUninstall
	var captured uninstall.ExecuteOptions
	executeUninstall = func(_ context.Context, opts uninstall.ExecuteOptions) uninstall.ExecuteResult {
		captured = opts
		return uninstall.ExecuteResult{
			Status: uninstall.StatusExecuteOK,
			Mode:   uninstall.ModeExecute,
		}
	}
	t.Cleanup(func() { executeUninstall = original })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--execute", "Bare Name App", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if len(captured.Selection) != 1 || captured.Selection[0] != "Bare Name App" {
		t.Fatalf("selection = %#v, want [Bare Name App] (bare name shorthand)", captured.Selection)
	}
}

func TestUninstallExecuteWiresAllowStopProcessesFlag(t *testing.T) {
	disableHistoryRecording(t)
	original := executeUninstall
	var captured uninstall.ExecuteOptions
	executeUninstall = func(_ context.Context, opts uninstall.ExecuteOptions) uninstall.ExecuteResult {
		captured = opts
		return uninstall.ExecuteResult{
			Status: uninstall.StatusExecuteOK,
			Mode:   uninstall.ModeExecute,
		}
	}
	t.Cleanup(func() { executeUninstall = original })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--execute", "--allow-stop-processes", "--select", "App", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if !captured.AllowStopProcesses {
		t.Fatal("AllowStopProcesses not set from --allow-stop-processes")
	}
}

func TestUninstallExecuteDoesNotSetAllowStopProcessesByDefault(t *testing.T) {
	disableHistoryRecording(t)
	original := executeUninstall
	var captured uninstall.ExecuteOptions
	executeUninstall = func(_ context.Context, opts uninstall.ExecuteOptions) uninstall.ExecuteResult {
		captured = opts
		return uninstall.ExecuteResult{
			Status: uninstall.StatusExecuteOK,
			Mode:   uninstall.ModeExecute,
		}
	}
	t.Cleanup(func() { executeUninstall = original })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--execute", "--select", "App", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if captured.AllowStopProcesses {
		t.Fatal("AllowStopProcesses = true, want false (default off; separate authorization)")
	}
}

func TestUninstallExecuteRejectsDryRunAndExecuteTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--dry-run", "--execute"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "either --dry-run or --execute") {
		t.Fatalf("stderr = %q, want either --dry-run or --execute", stderr.String())
	}
}

func TestUninstallExecuteRejectsUnknownOption(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--bogus"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown uninstall option") {
		t.Fatalf("stderr = %q, want unknown uninstall option", stderr.String())
	}
}

func TestUninstallExecuteJSONErrorEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--bogus", "--json"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	var env struct {
		Command string `json:"command"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("invalid error JSON: %v\n%s", err, stderr.String())
	}
	if env.Command != "uninstall" || env.Error == nil || env.Error.Code != "invalid_uninstall_invocation" {
		t.Fatalf("error envelope = %#v", env)
	}
}

func TestUninstallExecuteRecordsHistory(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	originalRecorder := newHistoryRecorder
	newHistoryRecorder = func() (history.Recorder, error) {
		return history.NewFileRecorder(historyDir), nil
	}
	t.Cleanup(func() { newHistoryRecorder = originalRecorder })

	// Use the real executeUninstall (uninstall.Execute). The selected app
	// name is intentionally not in the registry so Execute skips it with
	// app_not_found before reaching the process detector or runner. This
	// verifies the CLI wires a non-nil HistoryRecorder and Execute writes
	// the session; the unit test in internal/uninstall covers per-app
	// outcome semantics with fakes.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--execute", "--select", "Definitely Not Installed App 12345", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	query := history.NewFileQuery(historyDir)
	hist := query.Recent(context.Background())
	if len(hist.Sessions) != 1 {
		t.Fatalf("history sessions = %d, want 1", len(hist.Sessions))
	}
	sess := hist.Sessions[0]
	if sess.Command.Command != "uninstall" {
		t.Fatalf("command = %q, want uninstall", sess.Command.Command)
	}
	if !strings.HasPrefix(sess.ID, "uninstall-") {
		t.Fatalf("session id = %q, want uninstall- prefix", sess.ID)
	}
	if sess.Mode != uninstall.ModeExecute {
		t.Fatalf("mode = %q, want %q", sess.Mode, uninstall.ModeExecute)
	}
}

func TestUninstallExecuteHumanOutputRendersSummary(t *testing.T) {
	disableHistoryRecording(t)
	original := executeUninstall
	executeUninstall = func(_ context.Context, opts uninstall.ExecuteOptions) uninstall.ExecuteResult {
		return uninstall.ExecuteResult{
			Status: uninstall.StatusExecuteOK,
			Mode:   uninstall.ModeExecute,
			Applications: []uninstall.AppOutcome{{
				Name:        "Human App",
				Result:      uninstall.ResultUninstalled,
				Action:      uninstall.ActionOfficialUninstaller,
				CommandMode: uninstall.CommandModeQuiet,
			}},
			Totals: uninstall.ExecuteTotals{
				SelectedCount:    1,
				UninstalledCount: 1,
			},
		}
	}
	t.Cleanup(func() { executeUninstall = original })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"uninstall", "--execute", "--select", "Human App"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Foal uninstall",
		"Execution complete",
		"Selected: 1",
		"uninstalled: 1",
		"Human App",
		"Leftover deletion uses the Recycle Bin",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestHelpDocumentsUninstallExecuteAndStopProcesses(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	out := stdout.String()
	for _, want := range []string{
		"foal uninstall",
		"--execute",
		"--select",
		"--allow-stop-processes",
		"Leftover deletion uses the Recycle Bin",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}

func TestUninstallCommandDescriptionMentionsExecute(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "official uninstallers") {
		t.Fatalf("help missing 'official uninstallers' in uninstall description:\n%s", out)
	}
}
