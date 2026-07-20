package cli

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/uninstall"
)

// sampleUninstallPreview returns a deterministic multi-app review covering
// official, portable, admin-required, and non-selectable classes.
func sampleUninstallPreview() uninstall.Result {
	return uninstall.WithReviewSections(uninstall.Result{
		Status: "preview",
		Applications: []uninstall.Application{
			{
				Name:         "Official App",
				Version:      "1.0.0",
				Publisher:    "Vendor",
				PlannedClass: uninstall.PlannedClassOfficialUninstaller,
				Evidence:     []string{"windows_registry_uninstall_keys:HKCU"},
				Confidence:   "high",
				Ownership:    "app_owned",
			},
			{
				Name:          "Admin App",
				Version:       "2.0.0",
				Publisher:     "Vendor",
				PlannedClass:  uninstall.PlannedClassOfficialUninstaller,
				RequiresAdmin: true,
				Evidence:      []string{"windows_registry_uninstall_keys:HKLM64"},
				Confidence:    "high",
				Ownership:     "app_owned",
			},
			{
				Name:         "Portable App",
				Version:      "3.0.0",
				Publisher:    "Vendor",
				PlannedClass: uninstall.PlannedClassPortableDirectoryRemoval,
				Evidence:     []string{"start_menu_shortcut"},
				Confidence:   "medium",
				Ownership:    "app_owned",
			},
			{
				Name:         "Hard Exclusion App",
				Version:      "0.0.1",
				Publisher:    "Foal",
				PlannedClass: uninstall.PlannedClassHardExclusion,
				Evidence:     []string{"windows_registry_uninstall_keys:HKCU"},
				Confidence:   "high",
				Ownership:    "app_owned",
			},
		},
		PossibleLeftovers: []uninstall.LeftoverCandidate{
			{
				App:        "Official App",
				Path:       `C:\Users\test\AppData\Local\OfficialApp\Cache`,
				Ownership:  "app_owned",
				Confidence: "high",
			},
			{
				App:        "Official App",
				Path:       `C:\Users\test\AppData\Roaming\Shared`,
				Ownership:  "shared",
				Confidence: "low",
			},
			{
				App:        "Admin App",
				Path:       `C:\ProgramData\AdminApp\Logs`,
				Ownership:  "app_owned",
				Confidence: "high",
			},
		},
		Execution: uninstall.ExecutionPolicy{
			Allowed: false,
			Actions: []string{},
			Reason:  "preview-only until confirmed execute",
		},
	})
}

func openUninstallTUI(t *testing.T) (rootModel, tea.Cmd) {
	t.Helper()
	original := reviewUninstall
	reviewUninstall = sampleUninstallPreview
	t.Cleanup(func() { reviewUninstall = original })

	model := newRootModel()
	next, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	model = next.(rootModel)
	// Uninstall is the second menu item (index 1).
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(rootModel), cmd
}

func loadUninstallTUI(t *testing.T) rootModel {
	t.Helper()
	model, cmd := openUninstallTUI(t)
	if cmd == nil {
		t.Fatal("opening Uninstall must return a preview load command")
	}
	if !strings.Contains(model.content(), "Loading uninstall preview...") {
		t.Fatalf("uninstall TUI should show a loading state first:\n%s", model.content())
	}
	loaded, ok := cmd().(uninstallPreviewLoadedMsg)
	if !ok {
		t.Fatalf("load command produced %T, want uninstallPreviewLoadedMsg", cmd())
	}
	next, _ := model.Update(loaded)
	return next.(rootModel)
}

func TestUninstallTUIOpensMultiSelectPreview(t *testing.T) {
	model := loadUninstallTUI(t)

	if model.screen != screenUninstallPreview {
		t.Fatalf("screen = %v, want screenUninstallPreview", model.screen)
	}

	content := model.content()
	for _, want := range []string{
		"Uninstall TUI",
		"Preview-only: select apps, then confirm to run.",
		"Official App",
		"Admin App",
		"Portable App",
		"Hard Exclusion App",
		"(hard-exclusion, not selectable)",
		"Process-stop authorized: false",
		"Permanent authorized: false",
		"Selected: 0",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("preview missing %q:\n%s", want, content)
		}
	}
}

func TestUninstallTUIMultiSelectAndConfirmDisclosures(t *testing.T) {
	model := loadUninstallTUI(t)

	// Space toggles focused Official App; down to Admin, toggle; down to
	// Portable, toggle; hard exclusion must stay unselected.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})
	// Attempt to select hard exclusion (one more down + space): must not select.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})

	// Authorize process-stop and permanent.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'p', Text: "p"})

	preview := model.content()
	if !strings.Contains(preview, "Selected: 3") {
		t.Fatalf("expected 3 selected apps:\n%s", preview)
	}
	if !strings.Contains(preview, "Process-stop authorized: true") {
		t.Fatalf("process-stop toggle not reflected:\n%s", preview)
	}
	if !strings.Contains(preview, "Permanent authorized: true") {
		t.Fatalf("permanent toggle not reflected:\n%s", preview)
	}

	// First enter opens confirmation (non-mutating).
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.uninstall.phase != uninstallPhaseConfirmation {
		t.Fatalf("phase = %v, want confirmation", model.uninstall.phase)
	}

	confirm := model.content()
	for _, want := range []string{
		"Confirm uninstall",
		"Official App",
		"plan: Official uninstaller invocation",
		"Admin App",
		"requires admin: true",
		"Portable App",
		"plan: Portable directory removal",
		"Confirmed leftover path set: 2 path(s)", // Official + Admin high-confidence app_owned only
		"Process-stop authorization: true",
		"Permanent authorization: true",
		"Portable directory removal is authorized",
		"Applications likely requiring administrator rights",
		"Admin App",
		"shared Execute",
	} {
		if !strings.Contains(confirm, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, confirm)
		}
	}
	// Hard exclusion must not appear in the selected confirmation list as a plan row.
	if strings.Contains(confirm, "Hard Exclusion App") {
		t.Fatalf("hard exclusion must not be in confirmation selection:\n%s", confirm)
	}
	// Shared/low-confidence leftover must not inflate the confirmed count (2 not 3).
	if strings.Contains(confirm, "Confirmed leftover path set: 3") {
		t.Fatalf("confirmed leftover count must exclude non-app_owned/high leftovers:\n%s", confirm)
	}
}

func TestUninstallTUIPreviewWithoutConfirmDoesNotExecute(t *testing.T) {
	originalExec := runUninstallTUIExecute
	runUninstallTUIExecute = func(ctx context.Context, selection []string, allowStop, allowPermanent bool) uninstall.ExecuteResult {
		t.Fatal("preview/browse without confirmation must not call shared Execute")
		return uninstall.ExecuteResult{}
	}
	t.Cleanup(func() { runUninstallTUIExecute = originalExec })

	model := loadUninstallTUI(t)
	// Select and toggle authorizations, browse, back out — never confirm-run.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})   // open confirmation
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"}) // back to preview
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"}) // back to menu

	if model.screen != screenMenu {
		t.Fatalf("screen = %v, want screenMenu after back", model.screen)
	}
	if model.uninstall.executionStarted {
		t.Fatal("execution must not have started without confirmation enter")
	}
}

func TestUninstallTUIAuthorizationHandoffToSharedExecute(t *testing.T) {
	var capturedSelection []string
	var capturedStop, capturedPermanent bool
	var callCount int

	originalExec := runUninstallTUIExecute
	runUninstallTUIExecute = func(ctx context.Context, selection []string, allowStop, allowPermanent bool) uninstall.ExecuteResult {
		callCount++
		capturedSelection = append([]string(nil), selection...)
		capturedStop = allowStop
		capturedPermanent = allowPermanent
		return uninstall.ExecuteResult{
			Status: uninstall.StatusExecuteOK,
			Mode:   uninstall.ModeExecute,
			Applications: []uninstall.AppOutcome{{
				Name:         "Official App",
				PlannedClass: uninstall.PlannedClassOfficialUninstaller,
				Action:       uninstall.ActionOfficialUninstaller,
				Result:       uninstall.ResultUninstalled,
			}},
			Totals: uninstall.ExecuteTotals{
				SelectedCount:    1,
				UninstalledCount: 1,
			},
		}
	}
	t.Cleanup(func() { runUninstallTUIExecute = originalExec })

	model := loadUninstallTUI(t)
	// Select Official App only; authorize process-stop but not permanent.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})

	// Enter confirmation, then enter again to run.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("confirm enter must return an execute command")
	}
	if model.uninstall.phase != uninstallPhaseExecuting {
		t.Fatalf("phase = %v, want executing", model.uninstall.phase)
	}

	msg := cmd()
	executed, ok := msg.(uninstallExecutedMsg)
	if !ok {
		t.Fatalf("execute cmd produced %T, want uninstallExecutedMsg", msg)
	}
	next, _ = model.Update(executed)
	model = next.(rootModel)

	if callCount != 1 {
		t.Fatalf("shared Execute call count = %d, want 1", callCount)
	}
	if len(capturedSelection) != 1 || capturedSelection[0] != "Official App" {
		t.Fatalf("selection handoff = %#v, want [Official App]", capturedSelection)
	}
	if !capturedStop {
		t.Fatal("AllowStopProcesses handoff = false, want true (CLI --allow-stop-processes equivalent)")
	}
	if capturedPermanent {
		t.Fatal("AllowPermanent handoff = true, want false (default off, CLI equivalence)")
	}
	if model.uninstall.phase != uninstallPhaseResult {
		t.Fatalf("phase = %v, want result", model.uninstall.phase)
	}

	result := model.content()
	for _, want := range []string{
		"Status: complete",
		"Official App",
		"uninstalled: 1",
		"plan: Official uninstaller invocation",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("result missing %q:\n%s", want, result)
		}
	}
}

func TestUninstallTUIExecuteOptionsShapeMatchesCLI(t *testing.T) {
	// Assert runUninstallTUIExecute builds the same ExecuteOptions shape the
	// CLI uses (selection, allow flags, TUI provenance). Stub executeUninstall
	// (shared seam) rather than runUninstallTUIExecute so we see real options.
	var captured uninstall.ExecuteOptions
	original := executeUninstall
	executeUninstall = func(_ context.Context, opts uninstall.ExecuteOptions) uninstall.ExecuteResult {
		captured = opts
		return uninstall.ExecuteResult{
			Status: uninstall.StatusExecuteOK,
			Mode:   uninstall.ModeExecute,
		}
	}
	t.Cleanup(func() { executeUninstall = original })

	// Drive the real handoff helper (what the confirmed TUI run calls).
	_ = runUninstallTUIExecute(context.Background(), []string{"App A", "App B"}, true, true)

	if len(captured.Selection) != 2 || captured.Selection[0] != "App A" || captured.Selection[1] != "App B" {
		t.Fatalf("Selection = %#v, want [App A App B]", captured.Selection)
	}
	if !captured.AllowStopProcesses {
		t.Fatal("AllowStopProcesses = false, want true")
	}
	if !captured.AllowPermanent {
		t.Fatal("AllowPermanent = false, want true")
	}
	if captured.CommandParameters.Command != "uninstall" {
		t.Fatalf("Command = %q, want uninstall", captured.CommandParameters.Command)
	}
	if captured.CommandParameters.Surface != "tui" {
		t.Fatalf("Surface = %q, want tui", captured.CommandParameters.Surface)
	}
	if captured.CommandParameters.SelectionMode != "exact" {
		t.Fatalf("SelectionMode = %q, want exact", captured.CommandParameters.SelectionMode)
	}
	if len(captured.CommandParameters.SelectedCategories) != 2 {
		t.Fatalf("SelectedCategories = %#v, want 2 app names", captured.CommandParameters.SelectedCategories)
	}
}

func TestUninstallTUIConfirmWithoutSelectionDoesNotExecute(t *testing.T) {
	originalExec := runUninstallTUIExecute
	runUninstallTUIExecute = func(ctx context.Context, selection []string, allowStop, allowPermanent bool) uninstall.ExecuteResult {
		t.Fatal("empty selection must not call shared Execute")
		return uninstall.ExecuteResult{}
	}
	t.Cleanup(func() { runUninstallTUIExecute = originalExec })

	model := loadUninstallTUI(t)
	// Enter with zero selection must stay in preview.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.uninstall.phase != uninstallPhasePreview {
		t.Fatalf("phase = %v, want preview when nothing selected", model.uninstall.phase)
	}
	if model.uninstall.executionStarted {
		t.Fatal("execution must not start with empty selection")
	}
}

func TestUninstallTUIPortableWithoutPermanentDisclosesSkip(t *testing.T) {
	model := loadUninstallTUI(t)
	// Focus Portable App (index 2) and select it; leave permanent off.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})

	preview := model.content()
	if !strings.Contains(preview, "requires permanent authorization") {
		t.Fatalf("preview should note portable needs permanent auth:\n%s", preview)
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	confirm := model.content()
	if !strings.Contains(confirm, "Portable directory removal is NOT authorized") {
		t.Fatalf("confirmation must disclose portable not authorized:\n%s", confirm)
	}
	if !strings.Contains(confirm, "Permanent authorization: false") {
		t.Fatalf("confirmation must show permanent false:\n%s", confirm)
	}
}

func TestUninstallTUIMenuDescriptionMentionsSharedExecute(t *testing.T) {
	content := newRootModel().content()
	if !strings.Contains(content, "Multi-select installed apps") {
		t.Fatalf("main menu Uninstall description missing multi-select wording:\n%s", content)
	}
	if !strings.Contains(content, "shared Uninstall execute") {
		t.Fatalf("main menu Uninstall description missing shared execute wording:\n%s", content)
	}
}
