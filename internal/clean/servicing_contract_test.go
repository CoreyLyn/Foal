package clean_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// TestCatalogAcceptsInvokeWindowsServicingAction proves the shared vocabulary is
// action-neutral: a category may declare invoke_windows_servicing, and the
// servicing action is not treated as a deletion.
func TestCatalogAcceptsInvokeWindowsServicingAction(t *testing.T) {
	catalog, err := clean.NewCleanupCategoryCatalog([]clean.CleanupCategoryDefinition{{
		Identifier:               "sample_servicing",
		Label:                    "Sample servicing",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionInvokeWindowsServicing,
		SelectionPolicy:          clean.CategorySelectionPolicyExactOnly,
	}})
	if err != nil {
		t.Fatalf("servicing category rejected: %v", err)
	}
	summary, ok := catalog.Summary("sample_servicing")
	if !ok {
		t.Fatal("servicing summary missing")
	}
	if summary.PlannedAction != clean.PlannedActionInvokeWindowsServicing {
		t.Fatalf("planned action = %q, want invoke_windows_servicing", summary.PlannedAction)
	}
	if summary.SelectionPolicy != clean.CategorySelectionPolicyExactOnly {
		t.Fatalf("selection policy = %q, want exact-selection-only", summary.SelectionPolicy)
	}
	if clean.PlannedActionIsDeletion(clean.PlannedActionInvokeWindowsServicing) {
		t.Fatal("invoke_windows_servicing must not be treated as a deletion")
	}
	if !clean.PlannedActionIsDeletion(clean.PlannedActionMoveToRecycleBin) ||
		!clean.PlannedActionIsDeletion(clean.PlannedActionDeletePermanently) {
		t.Fatal("deletion actions must report as deletions")
	}
	if !clean.ExactSelectionOnlyCategory(summary) {
		t.Fatal("summary must report exact-selection-only")
	}
	if clean.InitiallySelectedCategory(summary) {
		t.Fatal("exact-selection-only servicing category must start unselected")
	}
	if clean.PlannedActionLabel(clean.PlannedActionInvokeWindowsServicing) != "Windows servicing" {
		t.Fatalf("servicing label = %q", clean.PlannedActionLabel(clean.PlannedActionInvokeWindowsServicing))
	}
}

// TestCatalogFailsClosedOnPlannedActionAndSelectionPolicy locks the fail-closed
// registration contract for planned actions and selection policy.
func TestCatalogFailsClosedOnPlannedActionAndSelectionPolicy(t *testing.T) {
	cases := []struct {
		name       string
		definition clean.CleanupCategoryDefinition
	}{
		{
			name: "executable missing planned action",
			definition: clean.CleanupCategoryDefinition{
				Identifier: "missing", Label: "Missing", ReportCategory: clean.ReportCategorySystem,
				Eligibility: clean.CategoryEligibilityOptIn, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			},
		},
		{
			name: "executable unknown planned action",
			definition: clean.CleanupCategoryDefinition{
				Identifier: "unknown", Label: "Unknown", ReportCategory: clean.ReportCategorySystem,
				Eligibility: clean.CategoryEligibilityOptIn, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
				PlannedAction: "invoke_something_else",
			},
		},
		{
			name: "unknown selection policy",
			definition: clean.CleanupCategoryDefinition{
				Identifier: "bad_policy", Label: "Bad policy", ReportCategory: clean.ReportCategorySystem,
				Eligibility: clean.CategoryEligibilityOptIn, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
				PlannedAction: clean.PlannedActionMoveToRecycleBin, SelectionPolicy: "sometimes",
			},
		},
		{
			name: "exact-only on default category",
			definition: clean.CleanupCategoryDefinition{
				Identifier: "default_exact", Label: "Default exact", ReportCategory: clean.ReportCategoryUserEssentials,
				Eligibility: clean.CategoryEligibilityDefault, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
				PlannedAction: clean.PlannedActionMoveToRecycleBin, SelectionPolicy: clean.CategorySelectionPolicyExactOnly,
			},
		},
		{
			name: "selection policy on non-executable",
			definition: clean.CleanupCategoryDefinition{
				Identifier: "boundary_policy", Label: "Boundary policy", ReportCategory: clean.ReportCategorySystem,
				Eligibility: clean.CategoryEligibilityPermissionBoundary, RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
				SelectionPolicy: clean.CategorySelectionPolicyExactOnly,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := clean.NewCleanupCategoryCatalog([]clean.CleanupCategoryDefinition{tc.definition}); err == nil {
				t.Fatal("NewCleanupCategoryCatalog() error = nil, want fail-closed rejection")
			}
		})
	}
}

// TestProductionCatalogRegistersNoServicingCategory guards the #305 boundary:
// this enabling refactor must not register any production servicing category.
func TestProductionCatalogRegistersNoServicingCategory(t *testing.T) {
	for _, summary := range clean.CanonicalCleanupCategoryCatalog().Summaries() {
		if summary.PlannedAction == clean.PlannedActionInvokeWindowsServicing {
			t.Fatalf("production catalog unexpectedly registers servicing category %q", summary.Identifier)
		}
		if summary.SelectionPolicy == clean.CategorySelectionPolicyExactOnly {
			t.Fatalf("production catalog unexpectedly registers exact-selection-only category %q", summary.Identifier)
		}
	}
}

func servicingContractOperations() []clean.ServicingOperation {
	exit0 := 0
	exit3017 := 3017
	return []clean.ServicingOperation{
		{ // success projection
			Category:            "winsxs_component_store",
			PlannedAction:       clean.PlannedActionInvokeWindowsServicing,
			Capability:          clean.ServicingCapabilityExecuteComponentStoreCleanup,
			ReclaimablePackages: 4,
			CleanupRecommended:  true,
			Outcome:             clean.ServicingOutcomeCompleted,
			ExitCode:            &exit0,
		},
		{ // authorization skip projection (no UAC, no DISM run)
			Category:      "winsxs_component_store",
			PlannedAction: clean.PlannedActionInvokeWindowsServicing,
			Capability:    clean.ServicingCapabilityExecuteComponentStoreCleanup,
			Outcome:       clean.ServicingOutcomeSkipped,
			Reason:        clean.ServicingReasonNotAuthorized,
		},
		{ // failure projection with restart-required exit
			Category:            "winsxs_component_store",
			PlannedAction:       clean.PlannedActionInvokeWindowsServicing,
			Capability:          clean.ServicingCapabilityExecuteComponentStoreCleanup,
			ReclaimablePackages: 4,
			CleanupRecommended:  true,
			Outcome:             clean.ServicingOutcomeFailed,
			Reason:              clean.ServicingReasonCleanupFailed,
			ExitCode:            &exit3017,
			RestartRequired:     true,
		},
		{ // pre-mutation cancellation projection
			Category:      "winsxs_component_store",
			PlannedAction: clean.PlannedActionInvokeWindowsServicing,
			Capability:    clean.ServicingCapabilityExecuteComponentStoreCleanup,
			Outcome:       clean.ServicingOutcomeCanceled,
			Reason:        clean.ServicingReasonContextCanceled,
		},
	}
}

// TestServicingOperationResultProjectionPathFreeByteFree verifies the four
// required projections round-trip in Result JSON and carry no path, raw tool
// output, package identifier, or byte estimate.
func TestServicingOperationResultProjectionPathFreeByteFree(t *testing.T) {
	result := clean.Result{
		Status:              "ok",
		Mode:                "execute",
		ServicingOperations: servicingContractOperations(),
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	blob := string(encoded)
	if !strings.Contains(blob, `"servicing_operations"`) {
		t.Fatalf("servicing_operations missing: %s", blob)
	}
	// The category identifier (winsxs_component_store) is a path-free field. What
	// must never appear is a filesystem path, raw DISM output, or package id.
	for _, forbidden := range []string{`C:\\`, `\\WinSxS`, `dism`, `"path"`, `stdout`, `stderr`, `package_id`} {
		if strings.Contains(blob, forbidden) {
			t.Fatalf("servicing projection leaked %q: %s", forbidden, blob)
		}
	}
	// Byte fields must never appear inside a servicing operation.
	for _, op := range servicingContractOperations() {
		opBlob, err := json.Marshal(op)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`bytes`, `"path"`} {
			if strings.Contains(string(opBlob), forbidden) {
				t.Fatalf("servicing operation carries forbidden field %q: %s", forbidden, opBlob)
			}
		}
	}

	// Round-trip preserves stable fields.
	var decoded clean.Result
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.ServicingOperations) != 4 {
		t.Fatalf("round-trip servicing count = %d, want 4", len(decoded.ServicingOperations))
	}
	success := decoded.ServicingOperations[0]
	if success.Outcome != clean.ServicingOutcomeCompleted || success.ReclaimablePackages != 4 ||
		!success.CleanupRecommended || success.ExitCode == nil || *success.ExitCode != 0 || success.RestartRequired {
		t.Fatalf("success projection lost fields: %#v", success)
	}
	skip := decoded.ServicingOperations[1]
	if skip.Outcome != clean.ServicingOutcomeSkipped || skip.Reason != clean.ServicingReasonNotAuthorized || skip.ExitCode != nil {
		t.Fatalf("authorization skip projection lost fields: %#v", skip)
	}
	fail := decoded.ServicingOperations[2]
	if fail.Outcome != clean.ServicingOutcomeFailed || fail.ExitCode == nil || *fail.ExitCode != 3017 || !fail.RestartRequired {
		t.Fatalf("failure projection lost fields: %#v", fail)
	}
	cancel := decoded.ServicingOperations[3]
	if cancel.Outcome != clean.ServicingOutcomeCanceled || cancel.Reason != clean.ServicingReasonContextCanceled || cancel.CancelRequested {
		t.Fatalf("cancellation projection lost fields: %#v", cancel)
	}
}

// TestServicingOperationsPreserveDeletionOnlyResultContract confirms that adding
// servicing operations never changes existing deletion-only serialized output:
// a Result without servicing omits the field and byte totals are untouched.
func TestServicingOperationsPreserveDeletionOnlyResultContract(t *testing.T) {
	result := clean.Result{Status: "ok", Mode: "dry_run"}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "servicing_operations") {
		t.Fatalf("empty servicing must be omitted: %s", encoded)
	}
	if strings.Contains(string(encoded), "servicing_operation_count") {
		t.Fatalf("zero servicing count must be omitted from totals: %s", encoded)
	}
}
