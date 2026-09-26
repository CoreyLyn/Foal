package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// Superseded display driver Clean TUI workflow tests (ADR 0036). Analysis and
// execution are injected through the runServicingAnalysisFn and
// runExactCleanSelection seams, so no test inspects the real driver store,
// launches UAC, or removes a package.

func newDriverWorkflowModel(t *testing.T) *eagerCleanModel {
	t.Helper()
	standard := clean.CleanupCategorySummary{
		Identifier:               clean.OpportunityCategoryUserTemp,
		Label:                    "User temp",
		ReportCategory:           clean.ReportCategoryUserEssentials,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionMoveToRecycleBin,
	}
	drivers := clean.CleanupCategorySummary{
		Identifier:               clean.CategorySupersededDisplayDrivers,
		Label:                    "Superseded display drivers",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionInvokeWindowsServicing,
		SelectionPolicy:          clean.CategorySelectionPolicyExactOnly,
	}
	model := newEagerCleanModelFromSummaries([]clean.CleanupCategorySummary{standard, drivers}, 100, 40)
	model.generation = 1
	model.finished = true
	for i := range model.rows {
		if !model.rows[i].Servicing {
			model.rows[i].State = clean.CategoryPreviewEmpty
			model.rows[i].Selected = false
		}
	}
	return &model
}

func readyDriverOperation() clean.ServicingOperation {
	bytes := int64(3 << 30)
	return clean.ServicingOperation{
		Category:            clean.CategorySupersededDisplayDrivers,
		PlannedAction:       clean.PlannedActionInvokeWindowsServicing,
		Capability:          clean.ServicingCapabilityAnalyzeDriverStore,
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 2,
		CleanupRecommended:  true,
		PackageBytes:        &bytes,
		DriverPackages: []clean.ServicingDriverPackage{
			{PublishedName: "oem10.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", Bytes: 1 << 30, Outcome: clean.DriverPackageOutcomeCandidate},
			{PublishedName: "oem11.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", Bytes: 2 << 30, Outcome: clean.DriverPackageOutcomeCandidate},
		},
	}
}

func driverPackageNames(packages []clean.ServicingDriverPackage) []string {
	names := make([]string, 0, len(packages))
	for _, pkg := range packages {
		names = append(names, pkg.PublishedName)
	}
	return names
}

func TestDriverRowInspectionIsReadOnlyAndDisclosesSize(t *testing.T) {
	model := newDriverWorkflowModel(t)
	idx := servicingRowIndex(t, model)
	model.cursor = idx
	if detail := eagerServicingFocusedDetailBody(model.rows[idx]); !strings.Contains(detail, "no administrator consent") {
		t.Fatalf("analysis_required detail = %q, want read-only inspection wording", detail)
	}

	runServicingAnalysis(t, model, readyDriverOperation())
	row := model.rows[idx]
	if row.ServicingState != clean.ServicingRowReady || row.ServicingPackageBytes != 3<<30 {
		t.Fatalf("row = %#v, want ready with measured bytes", row)
	}
	if !stringSlicesEqual(driverPackageNames(row.ServicingDriverPackages), []string{"oem10.inf", "oem11.inf"}) ||
		row.ServicingDriverPackages[0].OriginalName != "nv_dispi.inf" {
		t.Fatalf("row packages = %#v, want the disclosed identities", row.ServicingDriverPackages)
	}
	if label := eagerServicingRowLabel(row); !strings.Contains(label, "2 package(s)") || !strings.Contains(label, cleanFormatBytes(3<<30)) {
		t.Fatalf("ready label = %q, want package count and size", label)
	}
	if detail := eagerServicingFocusedDetailBody(row); !strings.Contains(detail, "cannot be rolled back") {
		t.Fatalf("ready detail = %q, want rollback disclosure", detail)
	}
}

func TestDriverConfirmationDisclosesRollbackNotComponentStore(t *testing.T) {
	model := newDriverWorkflowModel(t)
	runServicingAnalysis(t, model, readyDriverOperation())
	idx := servicingRowIndex(t, model)
	model.cursor = idx
	model.toggleFocusedSelection()
	if !model.rows[idx].Selected {
		t.Fatal("ready driver row must be selectable")
	}
	model.handleKey("enter")
	if model.phase != eagerPhaseConfirmation {
		t.Fatalf("phase = %v, want confirmation", model.phase)
	}
	content := model.content()
	assertNoPath(t, content)
	for _, want := range []string{
		"Windows servicing · 1 categories · 2 package(s) · " + cleanFormatBytes(3<<30),
		confirmationServicingAuthorizationLine,
		confirmationServicingUACLine,
		confirmationDriverNonInterruptLine,
		confirmationDriverRollbackLine,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{confirmationServicingNoRestartLine, confirmationServicingBytesLine, "/NoRestart"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("driver-only confirmation contains component-store disclosure %q:\n%s", unwanted, content)
		}
	}
}

func TestDriverExecutionFreezesDisclosedPackageSet(t *testing.T) {
	model := newDriverWorkflowModel(t)
	runServicingAnalysis(t, model, readyDriverOperation())
	idx := servicingRowIndex(t, model)
	model.cursor = idx
	model.toggleFocusedSelection()

	var gotPackages []clean.ServicingDriverPackage
	var gotServicing bool
	orig := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, selected []string, _, allowServicing bool, _ clean.ProgressReporter, confirmed []clean.ServicingDriverPackage) clean.Result {
		gotServicing = allowServicing
		gotPackages = append([]clean.ServicingDriverPackage(nil), confirmed...)
		observed := int64(2 << 30)
		return clean.Result{Status: "ok", Mode: "execute", ServicingOperations: []clean.ServicingOperation{{
			Category:          clean.CategorySupersededDisplayDrivers,
			PlannedAction:     clean.PlannedActionInvokeWindowsServicing,
			Capability:        clean.ServicingCapabilityExecuteDriverPackageCleanup,
			Outcome:           clean.ServicingOutcomeCompleted,
			ObservedFreeBytes: &observed,
		}}}
	}
	t.Cleanup(func() { runExactCleanSelection = orig })

	model.handleKey("enter")
	_, cmd := model.handleKey("enter")
	if cmd == nil {
		t.Fatal("second enter should hand off execution")
	}
	if !stringSlicesEqual(driverPackageNames(model.frozenDriverPackages), []string{"oem10.inf", "oem11.inf"}) {
		t.Fatalf("frozen driver packages = %#v", model.frozenDriverPackages)
	}
	driveExecutionToResult(t, model, cmd)
	if !gotServicing || !stringSlicesEqual(driverPackageNames(gotPackages), []string{"oem10.inf", "oem11.inf"}) ||
		gotPackages[1].Provider != "NVIDIA" {
		t.Fatalf("handoff servicing=%v packages=%#v", gotServicing, gotPackages)
	}
	content := model.content()
	if !strings.Contains(content, "Superseded display drivers: observed free-space increase ≈ "+cleanFormatBytes(2<<30)) {
		t.Fatalf("result must label the driver observation:\n%s", content)
	}
}

func TestExecuteExactCleanSelectionCmdPreservesConfirmedSetNilness(t *testing.T) {
	var received []clean.ServicingDriverPackage
	orig := runExactCleanSelection
	runExactCleanSelection = func(_ context.Context, _ []string, _, _ bool, _ clean.ProgressReporter, confirmed []clean.ServicingDriverPackage) clean.Result {
		received = confirmed
		return clean.Result{Status: "ok", Mode: "execute"}
	}
	t.Cleanup(func() { runExactCleanSelection = orig })

	for _, tc := range []struct {
		name      string
		confirmed []clean.ServicingDriverPackage
	}{
		{"no driver row", nil},
		{"empty confirmed set", []clean.ServicingDriverPackage{}},
	} {
		msg := executeExactCleanSelectionCmd(context.Background(), []string{clean.CategorySupersededDisplayDrivers}, false, true, tc.confirmed)()
		started, ok := msg.(eagerExactExecutionStartedMsg)
		if !ok {
			t.Fatalf("%s: message = %T", tc.name, msg)
		}
		<-started.stream.result
		if (received == nil) != (tc.confirmed == nil) {
			t.Fatalf("%s: handoff nil=%v, want nil=%v; an empty confirmed set must never widen to every fresh candidate",
				tc.name, received == nil, tc.confirmed == nil)
		}
	}
}

func TestSelectedDriverPackagesNilWhenDriverRowUnselected(t *testing.T) {
	model := newDriverWorkflowModel(t)
	runServicingAnalysis(t, model, readyDriverOperation())
	if got := model.selectedDriverPackages(); got != nil {
		t.Fatalf("unselected driver row packages = %#v, want nil", got)
	}
}

func TestHelpDocumentsSupersededDisplayDrivers(t *testing.T) {
	help := helpText()
	for _, want := range []string{
		"winsxs_component_store, superseded-display-drivers,",
		"foal clean --dry-run --opt-in superseded-display-drivers",
		"foal clean --execute --opt-in superseded-display-drivers --allow-servicing",
		"never forced, never files",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}
