package clean_test

import (
	"context"
	"encoding/json"
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

func executeDrivers(t *testing.T, gateway *fakeServicingGateway, allowServicing bool, confirmed []string) clean.Result {
	t.Helper()
	return clean.Execute(context.Background(), clean.Options{
		Validator:               pathsafe.Validator{},
		Plan:                    exactDriverPlan(t),
		AllowServicing:          allowServicing,
		ServicingGateway:        gateway,
		ConfirmedDriverPackages: confirmed,
	})
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
	gateway := &fakeServicingGateway{
		driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem11.inf", 200), driverCandidate("oem12.inf", 300)),
		driverExecResult: clean.DriverPackageCleanupResult{
			Outcome: clean.ServicingOutcomeCompleted,
			Packages: []clean.ServicingDriverPackage{
				{PublishedName: "oem10.inf", Outcome: clean.DriverPackageOutcomeRemoved},
				{PublishedName: "oem11.inf", Outcome: clean.DriverPackageOutcomeInUse},
				{PublishedName: "oem12.inf", Outcome: clean.DriverPackageOutcomeRemoved},
			},
			ObservedFreeBytes: &observed,
		},
	}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))

	req := gateway.lastDriverExecReq
	if req.Category != clean.CategorySupersededDisplayDrivers || req.Capability != clean.ServicingCapabilityExecuteDriverPackageCleanup ||
		strings.Join(req.Packages, ",") != "oem10.inf,oem11.inf,oem12.inf" {
		t.Fatalf("request = %#v", req)
	}
	if op.Outcome != clean.ServicingOutcomeCompleted || op.Capability != clean.ServicingCapabilityExecuteDriverPackageCleanup {
		t.Fatalf("op = %#v, want completed", op)
	}
	outcomes := []string{op.DriverPackages[0].Outcome, op.DriverPackages[1].Outcome, op.DriverPackages[2].Outcome}
	if strings.Join(outcomes, ",") != "removed,in_use,removed" {
		t.Fatalf("package outcomes = %v", outcomes)
	}
	if op.PackageBytes == nil || *op.PackageBytes != 400 {
		t.Fatalf("package bytes = %v, want removed bytes 400", op.PackageBytes)
	}
	if op.ObservedFreeBytes == nil || *op.ObservedFreeBytes != observed || op.RestartRequired {
		t.Fatalf("observed/restart = %#v", op)
	}
}

func TestDriverPackagesExecuteBoundsFreshCandidatesToConfirmedSet(t *testing.T) {
	gateway := &fakeServicingGateway{
		driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem12.inf", 300)),
		driverExecResult: clean.DriverPackageCleanupResult{
			Outcome:  clean.ServicingOutcomeCompleted,
			Packages: []clean.ServicingDriverPackage{{PublishedName: "oem10.inf", Outcome: clean.DriverPackageOutcomeRemoved}},
		},
	}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, []string{"OEM10.INF", "oem11.inf"}))
	if got := strings.Join(gateway.lastDriverExecReq.Packages, ","); got != "oem10.inf" {
		t.Fatalf("request packages = %q, want only the confirmed fresh candidate", got)
	}
	if op.Outcome != clean.ServicingOutcomeCompleted || len(op.DriverPackages) != 1 {
		t.Fatalf("op = %#v", op)
	}
}

func TestDriverPackagesExecuteEmptyConfirmedIntersectionIsNoWork(t *testing.T) {
	gateway := &fakeServicingGateway{driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100))}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, []string{"oem99.inf"}))
	if op.Outcome != clean.ServicingOutcomeNoWork || gateway.driverExecCalls != 0 {
		t.Fatalf("op = %#v exec calls = %d, want no_work without helper", op, gateway.driverExecCalls)
	}
}

func TestDriverPackagesExecuteFailedOrUnreportedPackageFailsOperation(t *testing.T) {
	gateway := &fakeServicingGateway{
		driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100), driverCandidate("oem11.inf", 200), driverCandidate("oem12.inf", 300)),
		driverExecResult: clean.DriverPackageCleanupResult{
			Outcome: clean.ServicingOutcomeCompleted,
			Packages: []clean.ServicingDriverPackage{
				{PublishedName: "oem10.inf", Outcome: clean.DriverPackageOutcomeRemoved},
				{PublishedName: "oem11.inf", Outcome: clean.DriverPackageOutcomeFailed},
				// oem12 missing from the helper response.
			},
		},
	}
	op := onlyDriverOp(t, executeDrivers(t, gateway, true, nil))
	if op.Outcome != clean.ServicingOutcomeFailed || op.Reason != clean.ServicingReasonCleanupFailed {
		t.Fatalf("op = %#v, want failed windows_servicing_cleanup_failed", op)
	}
	outcomes := []string{op.DriverPackages[0].Outcome, op.DriverPackages[1].Outcome, op.DriverPackages[2].Outcome}
	if strings.Join(outcomes, ",") != "removed,failed,failed" {
		t.Fatalf("package outcomes = %v", outcomes)
	}
	if op.PackageBytes == nil || *op.PackageBytes != 100 {
		t.Fatalf("package bytes = %v, want the removed package only", op.PackageBytes)
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
		driverAnalysis: readyDriverAnalysis(driverCandidate("oem10.inf", 100)),
		driverExecResult: clean.DriverPackageCleanupResult{
			Outcome:  clean.ServicingOutcomeCompleted,
			Packages: []clean.ServicingDriverPackage{{PublishedName: "oem10.inf", Outcome: clean.DriverPackageOutcomeRemoved}},
		},
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
