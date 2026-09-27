package clean_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

func exactDriverPlan(t *testing.T) *clean.CategoryPlan {
	t.Helper()
	plan, err := clean.CompileExactCategoryPlan([]string{clean.CategorySupersededDisplayDrivers})
	if err != nil {
		t.Fatalf("compile exact driver plan: %v", err)
	}
	return &plan
}

func driverCandidate(name string, bytes int64) clean.ServicingDriverPackage {
	return clean.ServicingDriverPackage{
		PublishedName: name,
		OriginalName:  "nv_dispi.inf",
		Provider:      "NVIDIA",
		DriverDate:    "01/01/2025",
		DriverVersion: "32.0.15.1000",
		Bytes:         bytes,
		Outcome:       clean.DriverPackageOutcomeCandidate,
	}
}

func readyDriverAnalysis(packages ...clean.ServicingDriverPackage) clean.DriverStoreAnalysisResult {
	return clean.DriverStoreAnalysisResult{Outcome: clean.ServicingOutcomeReady, Packages: packages}
}

func onlyDriverOp(t *testing.T, result clean.Result) clean.ServicingOperation {
	t.Helper()
	if len(result.Candidates) != 0 || len(result.Deleted) != 0 || len(result.OptInCandidates) != 0 {
		t.Fatalf("driver servicing produced file work: %#v", result)
	}
	if result.Totals.CandidateBytes != 0 || result.Totals.AffectedBytes != 0 || result.Totals.OptInReclaimableBytes != 0 {
		t.Fatalf("driver package bytes leaked into deletion totals: %#v", result.Totals)
	}
	if len(result.ServicingOperations) != 1 {
		t.Fatalf("servicing operations = %#v, want 1", result.ServicingOperations)
	}
	op := result.ServicingOperations[0]
	if op.Category != clean.CategorySupersededDisplayDrivers || op.PlannedAction != clean.PlannedActionInvokeWindowsServicing {
		t.Fatalf("op = %#v", op)
	}
	return op
}

func executeDrivers(t *testing.T, gateway *fakeServicingGateway, allowServicing bool, confirmed []clean.ServicingDriverPackage) clean.Result {
	t.Helper()
	return clean.Execute(context.Background(), clean.Options{
		Validator:               pathsafe.Validator{},
		Plan:                    exactDriverPlan(t),
		AllowServicing:          allowServicing,
		ServicingGateway:        gateway,
		ConfirmedDriverPackages: confirmed,
	})
}

// sentDriverResult is a helper response: the request reached the helper.
func sentDriverResult(outcome clean.ServicingOutcome, reason string, reports ...clean.ServicingDriverPackage) clean.DriverPackageCleanupResult {
	return clean.DriverPackageCleanupResult{Outcome: outcome, Reason: reason, Packages: reports, RequestSent: true}
}

func report(name, outcome string) clean.ServicingDriverPackage {
	return clean.ServicingDriverPackage{PublishedName: name, Outcome: outcome}
}

func driverOutcomeList(op clean.ServicingOperation) string {
	outcomes := make([]string, 0, len(op.DriverPackages))
	for _, pkg := range op.DriverPackages {
		outcomes = append(outcomes, pkg.PublishedName+"="+pkg.Outcome)
	}
	return strings.Join(outcomes, ",")
}

func requestNames(req clean.DriverPackageCleanupRequest) string {
	names := make([]string, 0, len(req.Packages))
	for _, pkg := range req.Packages {
		names = append(names, pkg.PublishedName)
	}
	return strings.Join(names, ",")
}

func TestDriverPackagesDryRunAnalysisListsCandidatesWithoutElevation(t *testing.T) {
	gateway := &fakeServicingGateway{driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem11.inf", 250))}
	result := clean.DryRun(context.Background(), clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactDriverPlan(t),
		ServicingGateway: gateway,
	})
	op := onlyDriverOp(t, result)
	if op.Outcome != clean.ServicingOutcomeReady || op.Capability != clean.ServicingCapabilityAnalyzeDriverStore {
		t.Fatalf("op = %#v, want ready analyze_driver_store", op)
	}
	if op.ReclaimablePackages != 2 || !op.CleanupRecommended || op.PackageBytes == nil || *op.PackageBytes != 350 {
		t.Fatalf("op counts = %#v", op)
	}
	if len(op.DriverPackages) != 2 || op.DriverPackages[0].Outcome != clean.DriverPackageOutcomeCandidate {
		t.Fatalf("driver packages = %#v", op.DriverPackages)
	}
	if gateway.driverAnalyzeCalls != 1 || gateway.calls != 0 || gateway.execCalls != 0 || gateway.driverExecCalls != 0 {
		t.Fatalf("gateway calls: driverAnalyze=%d winsxs=%d exec=%d driverExec=%d", gateway.driverAnalyzeCalls, gateway.calls, gateway.execCalls, gateway.driverExecCalls)
	}
}

func TestDriverPackagesAnalysisRejectsInvalidOrDuplicateNames(t *testing.T) {
	for name, packages := range map[string][]clean.ServicingDriverPackage{
		"path-like": {driverCandidate(`..\oem10.inf`, 1)},
		"inbox":     {driverCandidate("nv_dispi.inf", 1)},
		"duplicate": {driverCandidate("oem10.inf", 1), driverCandidate("OEM10.INF", 1)},
		"negative":  {driverCandidate("oem10.inf", -1)},
		"empty":     nil,
	} {
		t.Run(name, func(t *testing.T) {
			gateway := &fakeServicingGateway{driverAnalysis: readyDriverAnalysis(packages...)}
			op := onlyDriverOp(t, clean.DryRun(context.Background(), clean.Options{
				Validator: pathsafe.Validator{}, Plan: exactDriverPlan(t), ServicingGateway: gateway,
			}))
			if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonAnalysisOutputInvalid || len(op.DriverPackages) != 0 {
				t.Fatalf("op = %#v, want failed analysis_output_invalid", op)
			}
		})
	}
}

func TestDriverPackagesDryRunWithoutGatewayFailsClosed(t *testing.T) {
	op := onlyDriverOp(t, clean.DryRun(context.Background(), clean.Options{Validator: pathsafe.Validator{}, Plan: exactDriverPlan(t)}))
	if op.Outcome != clean.ServicingOutcomeSkipped || op.Reason != clean.ServicingReasonUnsupportedPlatform {
		t.Fatalf("op = %#v, want skipped unsupported_platform", op)
	}
}

func TestDriverPackagesNeverSelectedByAllToken(t *testing.T) {
	gateway := &fakeServicingGateway{driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 1))}
	result := clean.DryRun(context.Background(), clean.Options{
		Validator:                     pathsafe.Validator{},
		OptIn:                         []string{"all"},
		ServicingGateway:              gateway,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
	})
	for _, op := range result.ServicingOperations {
		if op.Category == clean.CategorySupersededDisplayDrivers {
			t.Fatalf("all token analyzed driver packages: %#v", op)
		}
	}
	if gateway.driverAnalyzeCalls != 0 {
		t.Fatalf("driver analysis calls = %d, want 0 for the all token", gateway.driverAnalyzeCalls)
	}
}

func TestDriverPackagesExecuteWithoutAllowServicingSkipsBeforeAnalysis(t *testing.T) {
	gateway := &fakeServicingGateway{driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 1))}
	op := onlyDriverOp(t, executeDrivers(t, gateway, false, nil))
	if op.Outcome != clean.ServicingOutcomeSkipped || op.Reason != clean.ServicingReasonNotAuthorized {
		t.Fatalf("op = %#v, want skipped windows_servicing_not_authorized", op)
	}
	if gateway.driverAnalyzeCalls != 0 || gateway.driverExecCalls != 0 {
		t.Fatalf("unauthorized run touched the gateway: analyze=%d exec=%d", gateway.driverAnalyzeCalls, gateway.driverExecCalls)
	}
}

func TestDriverPackagesExecuteSendsFreshCandidatesAndMergesOutcomes(t *testing.T) {
	observed := int64(4096)
	result := sentDriverResult(clean.ServicingOutcomeCompleted, "",
		report("oem10.inf", clean.DriverPackageOutcomeRemoved),
		report("oem11.inf", clean.DriverPackageOutcomeInUse),
		report("oem12.inf", clean.DriverPackageOutcomeRemoved))
	result.ObservedFreeBytes = &observed
	gateway := &fakeServicingGateway{
		driverAnalysis:   readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem11.inf", 200), driverCandidate("oem12.inf", 300)),
		driverExecResult: result,
	}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))

	req := gateway.lastDriverExecReq
	if req.Category != clean.CategorySupersededDisplayDrivers || req.Capability != clean.ServicingCapabilityExecuteDriverPackageCleanup ||
		requestNames(req) != "oem10.inf,oem11.inf,oem12.inf" {
		t.Fatalf("request = %#v", req)
	}
	if first := req.Packages[0]; first.OriginalName != "nv_dispi.inf" || first.Provider != "NVIDIA" || first.DriverVersion != "32.0.15.1000" || first.DriverDate != "01/01/2025" {
		t.Fatalf("request is not identity-bound: %#v", first)
	}
	if op.Outcome != clean.ServicingOutcomeCompleted || op.Capability != clean.ServicingCapabilityExecuteDriverPackageCleanup || op.ReclaimablePackages != 3 {
		t.Fatalf("op = %#v, want completed", op)
	}
	if got := driverOutcomeList(op); got != "oem10.inf=removed,oem11.inf=in_use,oem12.inf=removed" {
		t.Fatalf("package outcomes = %s", got)
	}
	if op.PackageBytes == nil || *op.PackageBytes != 400 {
		t.Fatalf("package bytes = %v, want removed bytes 400", op.PackageBytes)
	}
	if op.ObservedFreeBytes == nil || *op.ObservedFreeBytes != observed || op.RestartRequired {
		t.Fatalf("observed/restart = %#v", op)
	}
}

func TestDriverPackagesExecuteBindsCandidatesToConfirmedIdentities(t *testing.T) {
	gateway := &fakeServicingGateway{
		driverAnalysis:   readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem12.inf", 300)),
		driverExecResult: sentDriverResult(clean.ServicingOutcomeCompleted, "", report("oem10.inf", clean.DriverPackageOutcomeRemoved)),
	}
	reused := driverCandidate("oem12.inf", 300)
	reused.DriverVersion = "31.0.15.9999" // oem12.inf now names a different package
	confirmed := []clean.ServicingDriverPackage{driverCandidate("OEM10.INF", 100), driverCandidate("oem11.inf", 200), reused}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, confirmed))
	if got := requestNames(gateway.lastDriverExecReq); got != "oem10.inf" {
		t.Fatalf("request packages = %q, want only the confirmed identity", got)
	}
	if op.Outcome != clean.ServicingOutcomeCompleted {
		t.Fatalf("op = %#v", op)
	}
	if got := driverOutcomeList(op); got != "oem10.inf=removed,oem11.inf=not_eligible,oem12.inf=not_eligible" {
		t.Fatalf("package outcomes = %s", got)
	}
}

func TestDriverPackagesExecuteWithoutConfirmedMatchIsNoWork(t *testing.T) {
	for name, confirmed := range map[string][]clean.ServicingDriverPackage{
		"absent":    {driverCandidate("oem99.inf", 1)},
		"empty set": {},
	} {
		t.Run(name, func(t *testing.T) {
			gateway := &fakeServicingGateway{driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100))}
			op := onlyDriverOp(t, executeDrivers(t, gateway, true, confirmed))
			if op.Outcome != clean.ServicingOutcomeNoWork || gateway.driverExecCalls != 0 {
				t.Fatalf("op = %#v exec calls = %d, want no_work without helper", op, gateway.driverExecCalls)
			}
			if len(confirmed) > 0 && driverOutcomeList(op) != "oem99.inf=not_eligible" {
				t.Fatalf("packages = %s, want the confirmed package recorded not_eligible", driverOutcomeList(op))
			}
		})
	}
}

func TestDriverPackagesExecuteReportedFailureFailsOperation(t *testing.T) {
	gateway := &fakeServicingGateway{
		driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem11.inf", 200), driverCandidate("oem12.inf", 300)),
		driverExecResult: sentDriverResult(clean.ServicingOutcomeFailed, clean.ServicingReasonCleanupFailed,
			report("oem10.inf", clean.DriverPackageOutcomeRemoved),
			report("oem11.inf", clean.DriverPackageOutcomeFailed),
			report("oem12.inf", clean.DriverPackageOutcomeInUse)),
	}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))
	if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonCleanupFailed {
		t.Fatalf("op = %#v, want failed windows_servicing_cleanup_failed", op)
	}
	if got := driverOutcomeList(op); got != "oem10.inf=removed,oem11.inf=failed,oem12.inf=in_use" {
		t.Fatalf("package outcomes = %s", got)
	}
	if op.PackageBytes == nil || *op.PackageBytes != 100 {
		t.Fatalf("package bytes = %v, want the removed package only", op.PackageBytes)
	}
}

func TestDriverPackagesExecuteRejectsInconsistentHelperReports(t *testing.T) {
	negative := int64(-1)
	zero := int64(0)
	withObserved := func(res clean.DriverPackageCleanupResult, observed *int64) clean.DriverPackageCleanupResult {
		res.ObservedFreeBytes = observed
		return res
	}
	for name, tc := range map[string]struct {
		result clean.DriverPackageCleanupResult
		want   string
	}{
		"missing report": {
			sentDriverResult(clean.ServicingOutcomeFailed, clean.ServicingReasonCleanupFailed, report("oem10.inf", clean.DriverPackageOutcomeRemoved)),
			"oem10.inf=removed,oem11.inf=unknown",
		},
		"duplicate report": {
			sentDriverResult(clean.ServicingOutcomeCompleted, "",
				report("oem10.inf", clean.DriverPackageOutcomeRemoved), report("oem10.inf", clean.DriverPackageOutcomeFailed), report("oem11.inf", clean.DriverPackageOutcomeRemoved)),
			"oem10.inf=removed,oem11.inf=removed",
		},
		"unrequested package": {
			sentDriverResult(clean.ServicingOutcomeCompleted, "",
				report("oem10.inf", clean.DriverPackageOutcomeRemoved), report("oem11.inf", clean.DriverPackageOutcomeRemoved), report("oem42.inf", clean.DriverPackageOutcomeRemoved)),
			"oem10.inf=removed,oem11.inf=removed,oem42.inf=removed",
		},
		"unknown outcome value": {
			sentDriverResult(clean.ServicingOutcomeCompleted, "",
				report("oem10.inf", "deleted"), report("oem11.inf", clean.DriverPackageOutcomeRemoved)),
			"oem10.inf=unknown,oem11.inf=removed",
		},
		"outcome contradicts reports": {
			sentDriverResult(clean.ServicingOutcomeCompleted, "",
				report("oem10.inf", clean.DriverPackageOutcomeRemoved), report("oem11.inf", clean.DriverPackageOutcomeFailed)),
			"oem10.inf=removed,oem11.inf=failed",
		},
		"skipped after the request was sent": {
			sentDriverResult(clean.ServicingOutcomeSkipped, clean.ServicingReasonElevationDenied),
			"oem10.inf=unknown,oem11.inf=unknown",
		},
		"negative observation": {
			withObserved(sentDriverResult(clean.ServicingOutcomeCompleted, "",
				report("oem10.inf", clean.DriverPackageOutcomeRemoved), report("oem11.inf", clean.DriverPackageOutcomeRemoved)), &negative),
			"oem10.inf=removed,oem11.inf=removed",
		},
		"observation without removal": {
			withObserved(sentDriverResult(clean.ServicingOutcomeNoWork, "",
				report("oem10.inf", clean.DriverPackageOutcomeNotEligible), report("oem11.inf", clean.DriverPackageOutcomeInUse)), &zero),
			"oem10.inf=not_eligible,oem11.inf=in_use",
		},
		"exchange failed after send": {
			sentDriverResult(clean.ServicingOutcomeFailed, clean.ServicingReasonHelperFailed,
				report("oem10.inf", clean.DriverPackageOutcomeUnknown), report("oem11.inf", clean.DriverPackageOutcomeUnknown)),
			"oem10.inf=unknown,oem11.inf=unknown",
		},
	} {
		t.Run(name, func(t *testing.T) {
			gateway := &fakeServicingGateway{
				driverAnalysis:   readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem11.inf", 200)),
				driverExecResult: tc.result,
			}
			op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))
			if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonHelperFailed {
				t.Fatalf("op = %#v, want failed windows_servicing_helper_failed", op)
			}
			if got := driverOutcomeList(op); got != tc.want {
				t.Fatalf("package outcomes = %s, want %s", got, tc.want)
			}
			if op.ObservedFreeBytes != nil {
				t.Fatalf("observed free bytes kept from an inconsistent report: %d", *op.ObservedFreeBytes)
			}
		})
	}
}

func TestDriverPackagesExecuteBoundsRequestSize(t *testing.T) {
	var candidates, reports []clean.ServicingDriverPackage
	for i := 1; i <= 300; i++ {
		name := fmt.Sprintf("oem%d.inf", i)
		candidates = append(candidates, driverCandidate(name, 1))
		if i <= 256 {
			reports = append(reports, report(name, clean.DriverPackageOutcomeRemoved))
		}
	}
	gateway := &fakeServicingGateway{
		driverAnalysis:   readyDriverAnalysis(candidates...),
		driverExecResult: sentDriverResult(clean.ServicingOutcomeCompleted, "", reports...),
	}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))
	if len(gateway.lastDriverExecReq.Packages) != 256 || op.Outcome != clean.ServicingOutcomeCompleted || op.ReclaimablePackages != 300 {
		t.Fatalf("request size = %d op = %s/%s reclaimable = %d", len(gateway.lastDriverExecReq.Packages), op.Outcome, op.Reason, op.ReclaimablePackages)
	}
	if len(op.DriverPackages) != 300 || op.DriverPackages[255].Outcome != clean.DriverPackageOutcomeRemoved ||
		op.DriverPackages[256].Outcome != clean.DriverPackageOutcomeCandidate {
		t.Fatalf("packages beyond the request limit must stay unattempted candidates: %#v", op.DriverPackages[254:258])
	}
}

func TestDriverPackagesExecuteHelperSkipKeepsCandidatesUnattempted(t *testing.T) {
	gateway := &fakeServicingGateway{
		driverAnalysis:   readyDriverAnalysis(driverCandidate("oem10.inf", 100)),
		driverExecResult: clean.DriverPackageCleanupResult{Outcome: clean.ServicingOutcomeSkipped, Reason: clean.ServicingReasonElevationDenied},
	}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))
	if op.Outcome != clean.ServicingOutcomeSkipped || op.Reason != clean.ServicingReasonElevationDenied {
		t.Fatalf("op = %#v, want skipped elevation denied", op)
	}
	if len(op.DriverPackages) != 1 || op.DriverPackages[0].Outcome != clean.DriverPackageOutcomeCandidate {
		t.Fatalf("packages = %#v, want unattempted candidate", op.DriverPackages)
	}
	if op.ObservedFreeBytes != nil {
		t.Fatalf("observed free bytes = %v, want none without removal", *op.ObservedFreeBytes)
	}
}

func TestDriverPackagesExecuteFreshAnalysisFailureStopsBeforeHelper(t *testing.T) {
	gateway := &fakeServicingGateway{driverAnalysis: clean.DriverStoreAnalysisResult{Outcome: clean.ServicingOutcomeFailed, Reason: clean.ServicingReasonAnalysisFailed}}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))
	if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonAnalysisFailed || gateway.driverExecCalls != 0 {
		t.Fatalf("op = %#v exec calls = %d", op, gateway.driverExecCalls)
	}
}

func TestDriverPackagesHistoryRecordsPackagesWithoutPaths(t *testing.T) {
	recorder := &recordingHistoryRecorder{}
	gateway := &fakeServicingGateway{
		driverAnalysis:   readyDriverAnalysis(driverCandidate("oem10.inf", 100)),
		driverExecResult: sentDriverResult(clean.ServicingOutcomeCompleted, "", report("oem10.inf", clean.DriverPackageOutcomeRemoved)),
	}
	clean.Execute(context.Background(), clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactDriverPlan(t),
		AllowServicing:   true,
		ServicingGateway: gateway,
		HistoryRecorder:  recorder,
	})
	if len(recorder.sessions) != 1 || len(recorder.sessions[0].ServicingOperations) != 1 {
		t.Fatalf("history sessions = %#v", recorder.sessions)
	}
	record := recorder.sessions[0].ServicingOperations[0]
	if len(record.DriverPackages) != 1 || record.DriverPackages[0].PublishedName != "oem10.inf" ||
		record.DriverPackages[0].Outcome != clean.DriverPackageOutcomeRemoved || record.PackageBytes == nil || *record.PackageBytes != 100 {
		t.Fatalf("history record = %#v", record)
	}
	if len(recorder.items) != 0 {
		t.Fatalf("servicing produced path items: %#v", recorder.items)
	}
	encoded, _ := json.Marshal(record)
	if strings.Contains(string(encoded), `\`) || strings.Contains(strings.ToLower(string(encoded)), "driverstore") {
		t.Fatalf("history record carries a path: %s", encoded)
	}
}

func TestComponentStoreServicingJSONOmitsDriverFields(t *testing.T) {
	encoded, err := json.Marshal(clean.ServicingOperation{
		Category:      clean.CategoryWinSxSComponentStore,
		PlannedAction: clean.PlannedActionInvokeWindowsServicing,
		Capability:    clean.ServicingCapabilityAnalyzeComponentStore,
		Outcome:       clean.ServicingOutcomeNoWork,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "driver_packages") || strings.Contains(string(encoded), "package_bytes") {
		t.Fatalf("component-store record gained driver fields: %s", encoded)
	}
}

func TestDriverPackageOperationLinesDiscloseImpactAndPackages(t *testing.T) {
	bytes := int64(300)
	lines := clean.ServicingOperationLines([]clean.ServicingOperation{{
		Category:       clean.CategorySupersededDisplayDrivers,
		Capability:     clean.ServicingCapabilityAnalyzeDriverStore,
		Outcome:        clean.ServicingOutcomeReady,
		DriverPackages: []clean.ServicingDriverPackage{driverCandidate("oem10.inf", 100), driverCandidate("oem11.inf", 200)},
		PackageBytes:   &bytes,
	}})
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Superseded display drivers: 2 package(s) ready for removal (300 bytes)", "Roll Back Driver", "oem10.inf · nv_dispi.inf · NVIDIA · 32.0.15.1000 (01/01/2025) · 100 bytes"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("lines missing %q:\n%s", want, joined)
		}
	}
}

// observingDriverGateway records the progress stream at the moments the
// non-elevated analysis and the elevated removal actually run.
type observingDriverGateway struct {
	fakeServicingGateway
	onAnalyze func()
	onExec    func()
}

func (g *observingDriverGateway) AnalyzeDriverStore(ctx context.Context) clean.DriverStoreAnalysisResult {
	if g.onAnalyze != nil {
		g.onAnalyze()
	}
	return g.fakeServicingGateway.AnalyzeDriverStore(ctx)
}

func (g *observingDriverGateway) ExecuteDriverPackageCleanup(ctx context.Context, req clean.DriverPackageCleanupRequest) clean.DriverPackageCleanupResult {
	if g.onExec != nil {
		g.onExec()
	}
	return g.fakeServicingGateway.ExecuteDriverPackageCleanup(ctx, req)
}

// TestDriverServicingStaysOpenUntilHelperReturns proves a superseded display
// driver category is not projected as skipped just because resolve queued no
// file candidates. The row stays open through analysis and removal; the real
// outcome is reported only after the helper returns, and before the run's
// completion marker.
func TestDriverServicingStaysOpenUntilHelperReturns(t *testing.T) {
	var events []clean.ExecutionProgress
	gateway := &observingDriverGateway{}
	gateway.driverAnalysis = readyDriverAnalysis(driverCandidate("oem10.inf", 100))
	gateway.driverExecResult = sentDriverResult(clean.ServicingOutcomeCompleted, "", report("oem10.inf", clean.DriverPackageOutcomeRemoved))
	assertStillOpen := func(when string) {
		t.Helper()
		for _, event := range events {
			if event.CompletedCategory == clean.CategorySupersededDisplayDrivers {
				t.Fatalf("driver category completed %s; events=%#v", when, events)
			}
		}
	}
	gateway.onAnalyze = func() { assertStillOpen("before analysis") }
	gateway.onExec = func() { assertStillOpen("before removal") }

	result := clean.Execute(context.Background(), clean.Options{
		Validator:        pathsafe.Validator{},
		Plan:             exactDriverPlan(t),
		AllowServicing:   true,
		ServicingGateway: gateway,
		ProgressReporter: func(event clean.ExecutionProgress) { events = append(events, event) },
	})
	if op := onlyDriverOp(t, result); op.Outcome != clean.ServicingOutcomeCompleted {
		t.Fatalf("op = %#v, want completed", op)
	}

	active, completed, completePhase := -1, -1, -1
	for i, event := range events {
		if event.Phase == clean.ExecutionPhaseComplete && completePhase < 0 {
			completePhase = i
		}
		if event.Phase == clean.ExecutionPhaseServicingOperations &&
			event.ActiveCategory == clean.CategorySupersededDisplayDrivers &&
			event.CompletedCategory == "" && active < 0 {
			active = i
		}
		if event.CompletedCategory != clean.CategorySupersededDisplayDrivers {
			continue
		}
		if completed >= 0 {
			t.Fatalf("duplicate driver completion: %#v", events)
		}
		completed = i
		if event.CompletedState != clean.CategoryExecutionCleaned {
			t.Fatalf("completion state = %q, want cleaned", event.CompletedState)
		}
		if event.Phase != clean.ExecutionPhaseServicingOperations {
			t.Fatalf("completion phase = %q, want windows servicing", event.Phase)
		}
	}
	if active < 0 || completed < 0 || completePhase < 0 || active >= completed || completed >= completePhase {
		t.Fatalf("progress order active=%d completed=%d complete=%d events=%#v", active, completed, completePhase, events)
	}
}
