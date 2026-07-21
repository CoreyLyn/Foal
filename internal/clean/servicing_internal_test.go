package clean

import (
	"context"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/history"
)

type capturingServicingRecorder struct {
	session history.SessionRecord
	items   []history.ItemRecord
}

func (r *capturingServicingRecorder) Record(_ context.Context, session history.SessionRecord, items []history.ItemRecord) error {
	r.session = session
	r.items = append(r.items, items...)
	return nil
}

func internalServicingOperations() []ServicingOperation {
	exit0 := 0
	exit3017 := 3017
	return []ServicingOperation{
		{Category: "winsxs_component_store", PlannedAction: PlannedActionInvokeWindowsServicing, Capability: ServicingCapabilityExecuteComponentStoreCleanup, ReclaimablePackages: 4, CleanupRecommended: true, Outcome: ServicingOutcomeCompleted, ExitCode: &exit0},
		{Category: "winsxs_component_store", PlannedAction: PlannedActionInvokeWindowsServicing, Capability: ServicingCapabilityExecuteComponentStoreCleanup, Outcome: ServicingOutcomeSkipped, Reason: ServicingReasonNotAuthorized},
		{Category: "winsxs_component_store", PlannedAction: PlannedActionInvokeWindowsServicing, Capability: ServicingCapabilityExecuteComponentStoreCleanup, ReclaimablePackages: 4, CleanupRecommended: true, Outcome: ServicingOutcomeFailed, Reason: ServicingReasonCleanupFailed, ExitCode: &exit3017, RestartRequired: true},
		{Category: "winsxs_component_store", PlannedAction: PlannedActionInvokeWindowsServicing, Capability: ServicingCapabilityExecuteComponentStoreCleanup, Outcome: ServicingOutcomeCanceled, Reason: ServicingReasonContextCanceled},
	}
}

// TestTotalsCountServicingWithoutBytes proves servicing operations contribute a
// count only and never enter file arrays or byte totals.
func TestTotalsCountServicingWithoutBytes(t *testing.T) {
	result := Result{
		Mode:                "execute",
		ServicingOperations: internalServicingOperations(),
	}
	got := totals(result)
	if got.ServicingOperationCount != 4 {
		t.Fatalf("servicing count = %d, want 4", got.ServicingOperationCount)
	}
	if got.AffectedBytes != 0 || got.RecycleBinMovedBytes != 0 || got.PermanentlyDeletedBytes != 0 ||
		got.CandidateBytes != 0 || got.OptInAffectedBytes != 0 {
		t.Fatalf("servicing contributed bytes: %#v", got)
	}
	if got.CandidateCount != 0 || got.DeletedCount != 0 {
		t.Fatalf("servicing entered file arrays: %#v", got)
	}
}

// TestRecordHistorySessionProjectsServicingSeparatelyFromItems verifies the four
// required History projections are stored on the session (not as file items) and
// that the aggregate carries only a servicing count.
func TestRecordHistorySessionProjectsServicingSeparatelyFromItems(t *testing.T) {
	result := Result{
		Mode:                "execute",
		ServicingOperations: internalServicingOperations(),
	}
	result.Totals = totals(result)
	recorder := &capturingServicingRecorder{}
	now := time.Now()
	recordHistorySession(context.Background(), Options{HistoryRecorder: recorder}, result, now, now)

	if len(recorder.items) != 0 {
		t.Fatalf("servicing must not become file items: %#v", recorder.items)
	}
	if got := recorder.session.Aggregate.ServicingOperationCount; got != 4 {
		t.Fatalf("aggregate servicing count = %d, want 4", got)
	}
	if got := len(recorder.session.ServicingOperations); got != 4 {
		t.Fatalf("session servicing records = %d, want 4", got)
	}
	success := recorder.session.ServicingOperations[0]
	if success.Outcome != string(ServicingOutcomeCompleted) || success.ReclaimablePackages != 4 ||
		!success.CleanupRecommended || success.ExitCode == nil || *success.ExitCode != 0 || success.RestartRequired {
		t.Fatalf("success record lost fields: %#v", success)
	}
	skip := recorder.session.ServicingOperations[1]
	if skip.Outcome != string(ServicingOutcomeSkipped) || skip.Reason != ServicingReasonNotAuthorized || skip.ExitCode != nil {
		t.Fatalf("authorization skip record lost fields: %#v", skip)
	}
	fail := recorder.session.ServicingOperations[2]
	if fail.Outcome != string(ServicingOutcomeFailed) || fail.ExitCode == nil || *fail.ExitCode != 3017 || !fail.RestartRequired {
		t.Fatalf("failure record lost fields: %#v", fail)
	}
	cancel := recorder.session.ServicingOperations[3]
	if cancel.Outcome != string(ServicingOutcomeCanceled) || cancel.Reason != ServicingReasonContextCanceled {
		t.Fatalf("cancellation record lost fields: %#v", cancel)
	}
}

// TestAggregateAndGroupSelectionExcludeExactOnly proves the `all` token and each
// group token skip exact-selection-only categories while still expanding
// standard opt-ins.
func TestAggregateAndGroupSelectionExcludeExactOnly(t *testing.T) {
	entries := []categoryCatalogEntry{
		{definition: categoryDefinition("standard_dev", "Standard dev", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, PlannedActionDeletePermanently), resolverKind: categoryResolverDeveloperCache},
		{definition: categoryDefinition("standard_app", "Standard app", ReportCategoryApplications, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, PlannedActionDeletePermanently), resolverKind: categoryResolverApplicationCache},
		{definition: categoryDefinition("standard_cli", "Standard cli", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, PlannedActionDeletePermanently), cliAgentProduct: true},
		exactOnlyEntry("exact_dev", ReportCategoryDeveloperTools, categoryResolverDeveloperCache, false),
		exactOnlyEntry("exact_app", ReportCategoryApplications, categoryResolverApplicationCache, false),
		exactOnlyEntry("exact_cli", ReportCategoryDeveloperTools, categoryResolverDeveloperCache, true),
		exactOnlyEntry("exact_servicing", ReportCategorySystem, categoryResolverNonExecutable, false),
	}

	if got := aggregateSelectableCategoryIDsFrom(entries); containsAny(got, "exact_dev", "exact_app", "exact_cli", "exact_servicing") {
		t.Fatalf("all-token expansion included exact-only categories: %v", got)
	} else if !containsAll(got, "standard_dev", "standard_app", "standard_cli") {
		t.Fatalf("all-token expansion dropped standard opt-ins: %v", got)
	}
	if got := developerToolsOptInCategoryIDsFrom(entries); containsAny(got, "exact_dev") {
		t.Fatalf("dev-caches included exact-only: %v", got)
	}
	if got := applicationCachesOptInCategoryIDsFrom(entries); containsAny(got, "exact_app") {
		t.Fatalf("app-caches included exact-only: %v", got)
	}
	if got := cliAgentCategoryIDsFrom(entries); containsAny(got, "exact_cli") {
		t.Fatalf("cli-agents included exact-only: %v", got)
	}
}

// TestServicingObservationProjectsAndStaysOutOfTotals proves the post-mutation
// free-space observation projects into History and the aggregate helper, yet
// never becomes a byte total; measured-zero and absent observations contribute
// nothing to the presentation aggregate.
func TestServicingObservationProjectsAndStaysOutOfTotals(t *testing.T) {
	positive := int64(2048)
	zero := int64(0)
	ops := []ServicingOperation{
		{Category: "winsxs_component_store", Capability: ServicingCapabilityExecuteComponentStoreCleanup, Outcome: ServicingOutcomeCompleted, ObservedFreeBytes: &positive},
		{Category: "winsxs_component_store", Capability: ServicingCapabilityExecuteComponentStoreCleanup, Outcome: ServicingOutcomeCompleted, ObservedFreeBytes: &zero},
		{Category: "winsxs_component_store", Capability: ServicingCapabilityExecuteComponentStoreCleanup, Outcome: ServicingOutcomeCompleted},
	}

	records := servicingHistoryRecords(ops)
	if records[0].ObservedFreeBytes == nil || *records[0].ObservedFreeBytes != 2048 {
		t.Fatalf("positive observation not projected to history: %#v", records[0].ObservedFreeBytes)
	}
	if records[1].ObservedFreeBytes == nil || *records[1].ObservedFreeBytes != 0 {
		t.Fatalf("measured zero must project distinct from nil: %#v", records[1].ObservedFreeBytes)
	}
	if records[2].ObservedFreeBytes != nil {
		t.Fatalf("absent observation must stay nil: %#v", records[2].ObservedFreeBytes)
	}

	total, present := ServicingObservedFreeBytes(ops)
	if !present || total != 2048 {
		t.Fatalf("aggregate = (%d, %v), want (2048, true): zero and nil must contribute nothing", total, present)
	}

	got := totals(Result{Mode: "execute", ServicingOperations: ops})
	if got.AffectedBytes != 0 || got.RecycleBinMovedBytes != 0 || got.PermanentlyDeletedBytes != 0 ||
		got.CandidateBytes != 0 || got.OptInAffectedBytes != 0 {
		t.Fatalf("observation leaked into byte totals: %#v", got)
	}
}

func exactOnlyEntry(id string, report ReportCategory, kind categoryResolverKind, cliAgent bool) categoryCatalogEntry {
	def := categoryDefinition(id, id, report, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable, PlannedActionInvokeWindowsServicing)
	def.SelectionPolicy = CategorySelectionPolicyExactOnly
	return categoryCatalogEntry{definition: def, resolverKind: kind, cliAgentProduct: cliAgent}
}

func containsAny(ids []string, wanted ...string) bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	for _, w := range wanted {
		if set[w] {
			return true
		}
	}
	return false
}

func containsAll(ids []string, wanted ...string) bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	for _, w := range wanted {
		if !set[w] {
			return false
		}
	}
	return true
}
