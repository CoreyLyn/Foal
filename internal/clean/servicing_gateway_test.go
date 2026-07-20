package clean_test

import (
	"context"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// fakeServicingGateway records calls and returns a canned analysis so core
// tests never launch UAC or DISM.
type fakeServicingGateway struct {
	result  clean.ServicingAnalysisResult
	calls   int
	lastReq clean.ServicingAnalysisRequest
}

func (f *fakeServicingGateway) AnalyzeComponentStore(_ context.Context, req clean.ServicingAnalysisRequest) clean.ServicingAnalysisResult {
	f.calls++
	f.lastReq = req
	return f.result
}

func exactWinSxSPlan(t *testing.T) *clean.CategoryPlan {
	t.Helper()
	plan, err := clean.CompileExactCategoryPlan([]string{clean.CategoryWinSxSComponentStore})
	if err != nil {
		t.Fatalf("compile exact winsxs plan: %v", err)
	}
	return &plan
}

func onlyServicingOp(t *testing.T, result clean.Result) clean.ServicingOperation {
	t.Helper()
	if len(result.Candidates) != 0 || len(result.Deleted) != 0 || len(result.OptInCandidates) != 0 {
		t.Fatalf("servicing analysis produced file work: candidates=%d deleted=%d optin=%d",
			len(result.Candidates), len(result.Deleted), len(result.OptInCandidates))
	}
	if len(result.ServicingOperations) != 1 {
		t.Fatalf("servicing operations = %d, want 1", len(result.ServicingOperations))
	}
	if result.Totals.ServicingOperationCount != 1 {
		t.Fatalf("servicing operation count = %d, want 1", result.Totals.ServicingOperationCount)
	}
	op := result.ServicingOperations[0]
	if op.Category != clean.CategoryWinSxSComponentStore {
		t.Fatalf("op category = %q", op.Category)
	}
	if op.PlannedAction != clean.PlannedActionInvokeWindowsServicing {
		t.Fatalf("op planned action = %q", op.PlannedAction)
	}
	return op
}

func TestServicingDryRunAnalysisReady(t *testing.T) {
	exit0 := 0
	gateway := &fakeServicingGateway{result: clean.ServicingAnalysisResult{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 4,
		CleanupRecommended:  true,
		ExitCode:            &exit0,
	}}
	result := clean.DryRun(context.Background(), clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactWinSxSPlan(t),
		ServicingGateway: gateway,
	})
	op := onlyServicingOp(t, result)
	if op.Outcome != clean.ServicingOutcomeReady || op.ReclaimablePackages != 4 || !op.CleanupRecommended {
		t.Fatalf("ready op = %#v", op)
	}
	if op.Capability != clean.ServicingCapabilityAnalyzeComponentStore {
		t.Fatalf("capability = %q, want analyze_component_store", op.Capability)
	}
	if op.ExitCode == nil || *op.ExitCode != 0 || op.RestartRequired {
		t.Fatalf("ready op exit/restart = %#v", op)
	}
	if gateway.calls != 1 {
		t.Fatalf("gateway calls = %d, want 1", gateway.calls)
	}
	// The request carries only the category and fixed capability — never a path.
	if gateway.lastReq.Category != clean.CategoryWinSxSComponentStore ||
		gateway.lastReq.Capability != clean.ServicingCapabilityAnalyzeComponentStore {
		t.Fatalf("request = %#v", gateway.lastReq)
	}
}

func TestServicingDryRunAnalysisNoWork(t *testing.T) {
	exit0 := 0
	gateway := &fakeServicingGateway{result: clean.ServicingAnalysisResult{
		Outcome:             clean.ServicingOutcomeNoWork,
		ReclaimablePackages: 0,
		CleanupRecommended:  false,
		ExitCode:            &exit0,
	}}
	result := clean.DryRun(context.Background(), clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactWinSxSPlan(t),
		ServicingGateway: gateway,
	})
	op := onlyServicingOp(t, result)
	if op.Outcome != clean.ServicingOutcomeNoWork || op.ReclaimablePackages != 0 || op.CleanupRecommended {
		t.Fatalf("no_work op = %#v", op)
	}
	if op.Reason != "" {
		t.Fatalf("no_work op reason = %q, want empty", op.Reason)
	}
}

func TestServicingDryRunAnalysisFailedInvalidOutput(t *testing.T) {
	gateway := &fakeServicingGateway{result: clean.ServicingAnalysisResult{
		Outcome: clean.ServicingOutcomeFailed,
		Reason:  clean.ServicingReasonAnalysisOutputInvalid,
	}}
	result := clean.DryRun(context.Background(), clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactWinSxSPlan(t),
		ServicingGateway: gateway,
	})
	op := onlyServicingOp(t, result)
	if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonAnalysisOutputInvalid {
		t.Fatalf("failed op = %#v", op)
	}
	if op.ExitCode != nil {
		t.Fatalf("failed-before-exit op must have no exit code: %#v", op)
	}
}

// TestServicingDryRunReadyInconsistentFailsClosed proves a ready outcome without
// supporting parsed evidence is downgraded to a stable failure, so localization
// or format drift can never be read as approval.
func TestServicingDryRunReadyInconsistentFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result clean.ServicingAnalysisResult
	}{
		{"zero packages", clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 0, CleanupRecommended: true}},
		{"not recommended", clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 5, CleanupRecommended: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &fakeServicingGateway{result: tc.result}
			result := clean.DryRun(context.Background(), clean.Options{
				Validator:        pathsafe.Validator{},
				Plan:             exactWinSxSPlan(t),
				ServicingGateway: gateway,
			})
			op := onlyServicingOp(t, result)
			if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonAnalysisOutputInvalid {
				t.Fatalf("inconsistent ready op = %#v", op)
			}
		})
	}
}

func TestServicingDryRunNoGatewayUnsupportedPlatform(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		Validator: pathsafe.Validator{},
		Plan:      exactWinSxSPlan(t),
	})
	op := onlyServicingOp(t, result)
	if op.Outcome != clean.ServicingOutcomeSkipped || op.Reason != clean.ServicingReasonUnsupportedPlatform {
		t.Fatalf("no-gateway op = %#v", op)
	}
	if op.ExitCode != nil {
		t.Fatalf("no-gateway op must have no exit code: %#v", op)
	}
}

func TestServicingDryRunPreCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gateway := &fakeServicingGateway{result: clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 4, CleanupRecommended: true}}
	result := clean.DryRun(ctx, clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactWinSxSPlan(t),
		ServicingGateway: gateway,
	})
	if len(result.ServicingOperations) != 1 {
		t.Fatalf("servicing operations = %d, want 1", len(result.ServicingOperations))
	}
	op := result.ServicingOperations[0]
	if op.Outcome != clean.ServicingOutcomeCanceled || op.Reason != clean.ServicingReasonContextCanceled {
		t.Fatalf("canceled op = %#v", op)
	}
	if gateway.calls != 0 {
		t.Fatalf("gateway called after cancellation: %d", gateway.calls)
	}
}

// TestServicingExecuteSkipsNotAuthorized proves execute never opens UAC or
// analyzes: a selected servicing category is a fail-closed not-authorized skip
// and the gateway is never called during execute.
func TestServicingExecuteSkipsNotAuthorized(t *testing.T) {
	gateway := &fakeServicingGateway{result: clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 4, CleanupRecommended: true}}
	result := clean.Execute(context.Background(), clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactWinSxSPlan(t),
		ServicingGateway: gateway,
	})
	if len(result.Deleted) != 0 || len(result.Failed) != 0 {
		t.Fatalf("execute mutated: deleted=%d failed=%d", len(result.Deleted), len(result.Failed))
	}
	if len(result.ServicingOperations) != 1 {
		t.Fatalf("servicing operations = %d, want 1", len(result.ServicingOperations))
	}
	op := result.ServicingOperations[0]
	if op.Outcome != clean.ServicingOutcomeSkipped || op.Reason != clean.ServicingReasonNotAuthorized {
		t.Fatalf("execute skip op = %#v", op)
	}
	if op.Capability != clean.ServicingCapabilityExecuteComponentStoreCleanup {
		t.Fatalf("execute skip capability = %q", op.Capability)
	}
	if op.ExitCode != nil {
		t.Fatalf("not-authorized skip must have no exit code: %#v", op)
	}
	if gateway.calls != 0 {
		t.Fatalf("gateway called during execute: %d", gateway.calls)
	}
}

// TestDefaultDryRunDoesNotAnalyzeServicing proves default Dry-run (no servicing
// opt-in) never contacts the gateway and reports no servicing operations.
func TestDefaultDryRunDoesNotAnalyzeServicing(t *testing.T) {
	gateway := &fakeServicingGateway{result: clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeReady, ReclaimablePackages: 4, CleanupRecommended: true}}
	result := clean.DryRun(context.Background(), clean.Options{
		Validator:                 pathsafe.Validator{},
		Rules:                     []clean.Rule{{ID: "noop", DefaultEnabled: false}},
		ServicingGateway:          gateway,
		DiscoverOpportunities:     func(context.Context) clean.OpportunityDiscoveryResult { return clean.OpportunityDiscoveryResult{} },
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion { return nil },
	})
	if len(result.ServicingOperations) != 0 {
		t.Fatalf("default dry-run produced servicing operations: %#v", result.ServicingOperations)
	}
	if gateway.calls != 0 {
		t.Fatalf("default dry-run called the servicing gateway: %d", gateway.calls)
	}
}

// TestAggregateAndGroupTokensExcludeServicing proves `all` and every group token
// never expand to the exact-selection-only servicing category, so broad cleanup
// automation cannot implicitly analyze WinSxS or request UAC.
func TestAggregateAndGroupTokensExcludeServicing(t *testing.T) {
	for _, token := range []string{"all", "dev-caches", "app-caches", "cli-agents"} {
		plan, invalid := clean.CompileAdditiveCategoryPlan([]string{token})
		if len(invalid) != 0 {
			t.Fatalf("token %q produced invalid names: %v", token, invalid)
		}
		for _, id := range plan.Categories {
			if id == clean.CategoryWinSxSComponentStore {
				t.Fatalf("token %q expanded to the servicing category", token)
			}
		}
	}
	// The exact identifier is still accepted as an opt-in.
	enabled, invalid, _ := clean.NormalizedOptInSet([]string{clean.CategoryWinSxSComponentStore})
	if len(invalid) != 0 || !enabled[clean.CategoryWinSxSComponentStore] {
		t.Fatalf("exact servicing opt-in rejected: enabled=%v invalid=%v", enabled, invalid)
	}
}
