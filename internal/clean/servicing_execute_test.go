package clean_test

import (
	"context"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func executeWinSxS(t *testing.T, ctx context.Context, allowServicing bool, gateway *fakeServicingGateway) clean.Result {
	t.Helper()
	opts := clean.Options{
		Validator:      pathsafe.Validator{},
		Plan:           exactWinSxSPlan(t),
		AllowServicing: allowServicing,
	}
	if gateway != nil {
		opts.ServicingGateway = gateway
	}
	return clean.Execute(ctx, opts)
}

// TestServicingExecuteAuthorizedSuccess proves an authorized run drives the
// composite gateway and records a completed operation with no file mutation.
func TestServicingExecuteAuthorizedSuccess(t *testing.T) {
	exit0 := 0
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{
		Outcome:             clean.ServicingOutcomeCompleted,
		ReclaimablePackages: 4,
		CleanupRecommended:  true,
		ExitCode:            &exit0,
	}}
	result := executeWinSxS(t, context.Background(), true, gateway)
	op := onlyServicingOp(t, result)
	if op.Outcome != clean.ServicingOutcomeCompleted || op.ReclaimablePackages != 4 || !op.CleanupRecommended {
		t.Fatalf("completed op = %#v", op)
	}
	if op.Capability != clean.ServicingCapabilityExecuteComponentStoreCleanup {
		t.Fatalf("capability = %q, want execute_component_store_cleanup", op.Capability)
	}
	if op.ExitCode == nil || *op.ExitCode != 0 || op.RestartRequired || op.CancelRequested {
		t.Fatalf("completed op exit/restart/cancel = %#v", op)
	}
	if gateway.execCalls != 1 {
		t.Fatalf("execute gateway calls = %d, want 1", gateway.execCalls)
	}
	if gateway.calls != 0 {
		t.Fatalf("execute must not call read-only analysis: %d", gateway.calls)
	}
	if gateway.lastExecReq.Category != clean.CategoryWinSxSComponentStore ||
		gateway.lastExecReq.Capability != clean.ServicingCapabilityExecuteComponentStoreCleanup {
		t.Fatalf("execute request = %#v", gateway.lastExecReq)
	}
}

// TestServicingExecuteRestartRequired proves DISM exit 3010 is completed with a
// restart requirement preserved.
func TestServicingExecuteRestartRequired(t *testing.T) {
	exit3010 := 3010
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{
		Outcome:             clean.ServicingOutcomeCompleted,
		ReclaimablePackages: 4,
		CleanupRecommended:  true,
		ExitCode:            &exit3010,
		RestartRequired:     true,
	}}
	op := onlyServicingOp(t, executeWinSxS(t, context.Background(), true, gateway))
	if op.Outcome != clean.ServicingOutcomeCompleted || !op.RestartRequired {
		t.Fatalf("restart op = %#v", op)
	}
	if op.ExitCode == nil || *op.ExitCode != 3010 {
		t.Fatalf("restart op exit = %#v", op.ExitCode)
	}
}

// TestServicingExecuteFailureRestart proves DISM exit 3017 is a failure with a
// restart requirement.
func TestServicingExecuteFailureRestart(t *testing.T) {
	exit3017 := 3017
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{
		Outcome:             clean.ServicingOutcomeFailed,
		Reason:              clean.ServicingReasonCleanupFailed,
		ReclaimablePackages: 4,
		CleanupRecommended:  true,
		ExitCode:            &exit3017,
		RestartRequired:     true,
	}}
	op := onlyServicingOp(t, executeWinSxS(t, context.Background(), true, gateway))
	if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonCleanupFailed || !op.RestartRequired {
		t.Fatalf("failure restart op = %#v", op)
	}
	if op.ExitCode == nil || *op.ExitCode != 3017 {
		t.Fatalf("failure restart op exit = %#v", op.ExitCode)
	}
}

// TestServicingExecuteNoWork proves a fresh analysis reporting no work yields a
// no_work outcome without a reason and without mutation.
func TestServicingExecuteNoWork(t *testing.T) {
	exit0 := 0
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{
		Outcome:             clean.ServicingOutcomeNoWork,
		ReclaimablePackages: 0,
		CleanupRecommended:  false,
		ExitCode:            &exit0,
	}}
	op := onlyServicingOp(t, executeWinSxS(t, context.Background(), true, gateway))
	if op.Outcome != clean.ServicingOutcomeNoWork || op.Reason != "" || op.ReclaimablePackages != 0 {
		t.Fatalf("no_work op = %#v", op)
	}
}

// TestServicingExecuteElevationOutcomes proves UAC denial is a skip and elevation
// failure / helper / tool failures are failures with their stable reasons.
func TestServicingExecuteElevationOutcomes(t *testing.T) {
	cases := []struct {
		name        string
		result      clean.ServicingExecuteResult
		wantOutcome clean.ServicingOutcome
		wantReason  string
	}{
		{"elevation denied", clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeSkipped, Reason: clean.ServicingReasonElevationDenied}, clean.ServicingOutcomeSkipped, clean.ServicingReasonElevationDenied},
		{"elevation failed", clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonElevationFailed}, clean.ServicingOutcomeFailed, clean.ServicingReasonElevationFailed},
		{"helper failed", clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonHelperFailed}, clean.ServicingOutcomeFailed, clean.ServicingReasonHelperFailed},
		{"tool unavailable", clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonToolUnavailable}, clean.ServicingOutcomeFailed, clean.ServicingReasonToolUnavailable},
		{"analysis failed", clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonAnalysisFailed}, clean.ServicingOutcomeFailed, clean.ServicingReasonAnalysisFailed},
		{"analysis output invalid", clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonAnalysisOutputInvalid}, clean.ServicingOutcomeFailed, clean.ServicingReasonAnalysisOutputInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &fakeServicingGateway{execResult: tc.result}
			op := onlyServicingOp(t, executeWinSxS(t, context.Background(), true, gateway))
			if op.Outcome != tc.wantOutcome || op.Reason != tc.wantReason {
				t.Fatalf("op = %#v, want outcome=%q reason=%q", op, tc.wantOutcome, tc.wantReason)
			}
			if op.ExitCode != nil {
				t.Fatalf("pre-DISM failure must have no exit code: %#v", op)
			}
		})
	}
}

// TestServicingExecuteNotAuthorized proves missing --allow-servicing skips with a
// stable reason and never calls the gateway (no UAC).
func TestServicingExecuteNotAuthorized(t *testing.T) {
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{
		Outcome: clean.ServicingOutcomeCompleted,
	}}
	op := onlyServicingOp(t, executeWinSxS(t, context.Background(), false, gateway))
	if op.Outcome != clean.ServicingOutcomeSkipped || op.Reason != clean.ServicingReasonNotAuthorized {
		t.Fatalf("unauthorized op = %#v", op)
	}
	if op.Capability != clean.ServicingCapabilityExecuteComponentStoreCleanup {
		t.Fatalf("unauthorized capability = %q", op.Capability)
	}
	if op.ExitCode != nil {
		t.Fatalf("not-authorized skip must have no exit code: %#v", op)
	}
	if gateway.execCalls != 0 || gateway.calls != 0 {
		t.Fatalf("gateway called without authorization: exec=%d analyze=%d", gateway.execCalls, gateway.calls)
	}
}

// TestServicingExecuteAuthorizedNoGateway proves an authorized run with no wired
// gateway fails closed with unsupported_platform and no UAC.
func TestServicingExecuteAuthorizedNoGateway(t *testing.T) {
	op := onlyServicingOp(t, executeWinSxS(t, context.Background(), true, nil))
	if op.Outcome != clean.ServicingOutcomeSkipped || op.Reason != clean.ServicingReasonUnsupportedPlatform {
		t.Fatalf("no-gateway execute op = %#v", op)
	}
	if op.ExitCode != nil {
		t.Fatalf("no-gateway op must have no exit code: %#v", op)
	}
}

// TestServicingExecutePreStartCancel proves a run canceled before servicing
// begins records a pre-mutation cancellation and never calls the gateway.
func TestServicingExecutePreStartCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{Outcome: clean.ServicingOutcomeCompleted}}
	op := onlyServicingOp(t, executeWinSxS(t, ctx, true, gateway))
	if op.Outcome != clean.ServicingOutcomeCanceled || op.Reason != clean.ServicingReasonContextCanceled {
		t.Fatalf("pre-start cancel op = %#v", op)
	}
	if op.CancelRequested {
		t.Fatalf("pre-start cancel must not set cancel_requested: %#v", op)
	}
	if gateway.execCalls != 0 {
		t.Fatalf("gateway called after pre-start cancellation: %d", gateway.execCalls)
	}
}

// TestServicingExecutePostStartCancelPreservesOutcome proves a cancellation that
// arrives after cleanup starts is recorded without replacing the actual outcome.
func TestServicingExecutePostStartCancelPreservesOutcome(t *testing.T) {
	exit0 := 0
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{
		Outcome:             clean.ServicingOutcomeCompleted,
		ReclaimablePackages: 4,
		CleanupRecommended:  true,
		ExitCode:            &exit0,
		CancelRequested:     true,
	}}
	op := onlyServicingOp(t, executeWinSxS(t, context.Background(), true, gateway))
	if op.Outcome != clean.ServicingOutcomeCompleted || !op.CancelRequested {
		t.Fatalf("post-start cancel op = %#v", op)
	}
}

// TestServicingExecuteReadyFromMutationFailsClosed proves a read-only ready
// outcome returned from the mutation capability is downgraded to a stable
// failure, so ambiguous state is never read as success.
func TestServicingExecuteReadyFromMutationFailsClosed(t *testing.T) {
	gateway := &fakeServicingGateway{execResult: clean.ServicingExecuteResult{
		Outcome:             clean.ServicingOutcomeReady,
		ReclaimablePackages: 4,
		CleanupRecommended:  true,
	}}
	op := onlyServicingOp(t, executeWinSxS(t, context.Background(), true, gateway))
	if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonHelperFailed {
		t.Fatalf("ready-from-mutation op = %#v", op)
	}
}

// orderedServicingGateway records the moment servicing runs into a shared
// ordered call list so a mixed run can assert servicing runs last.
type orderedServicingGateway struct {
	collab *orderedCollaborators
	result clean.ServicingExecuteResult
}

func (g *orderedServicingGateway) AnalyzeComponentStore(context.Context, clean.ServicingAnalysisRequest) clean.ServicingAnalysisResult {
	return clean.ServicingAnalysisResult{Outcome: clean.ServicingOutcomeSkipped, Reason: clean.ServicingReasonHelperFailed}
}

func (g *orderedServicingGateway) ExecuteComponentStoreCleanup(context.Context, clean.ServicingExecuteRequest) clean.ServicingExecuteResult {
	g.collab.calls = append(g.collab.calls, orderedCall{kind: "servicing"})
	return g.result
}

// TestServicingExecuteRunsAfterRecycleBinAndPermanent proves the mixed execution
// order is Recycle Bin, then Permanent deletion, then Windows servicing last.
func TestServicingExecuteRunsAfterRecycleBinAndPermanent(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "recycle.tmp", "rbin")
	permanentPath := writeTestFile(t, root, "permanent.tmp", "perm")
	collab := &orderedCollaborators{}
	exit0 := 0
	gateway := &orderedServicingGateway{
		collab: collab,
		result: clean.ServicingExecuteResult{
			Outcome:             clean.ServicingOutcomeCompleted,
			ReclaimablePackages: 4,
			CleanupRecommended:  true,
			ExitCode:            &exit0,
		},
	}
	opts := mixedActionOpts(t, root, recyclePath, permanentPath, true)
	opts.OptIn = []string{clean.CategoryWinSxSComponentStore}
	opts.AllowServicing = true
	opts.RecycleBinAdapter = collab
	opts.PermanentRemover = collab
	opts.ServicingGateway = gateway

	result := clean.Execute(context.Background(), opts)

	if len(collab.calls) != 3 {
		t.Fatalf("calls = %#v, want recycle, permanent, servicing", collab.calls)
	}
	if collab.calls[0].kind != "recycle" || collab.calls[1].kind != "permanent" || collab.calls[2].kind != "servicing" {
		t.Fatalf("order = %#v, want recycle, permanent, servicing", collab.calls)
	}
	if len(result.Deleted) != 2 {
		t.Fatalf("deleted = %#v, want recycle + permanent", result.Deleted)
	}
	if len(result.ServicingOperations) != 1 || result.ServicingOperations[0].Outcome != clean.ServicingOutcomeCompleted {
		t.Fatalf("servicing operations = %#v", result.ServicingOperations)
	}
	// Servicing never enters file arrays or byte totals.
	if result.Totals.ServicingOperationCount != 1 {
		t.Fatalf("servicing count = %d, want 1", result.Totals.ServicingOperationCount)
	}
}
