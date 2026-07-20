package clean_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

func TestEagerPreviewQueueMatchesScannableCatalog(t *testing.T) {
	queue := clean.EagerPreviewQueue()
	ids := clean.ScannableCategoryIDs()
	if len(queue) != len(ids) {
		t.Fatalf("queue len = %d, scannable = %d", len(queue), len(ids))
	}
	for i, summary := range queue {
		if summary.Identifier != ids[i] {
			t.Fatalf("queue[%d] = %q, scannable = %q", i, summary.Identifier, ids[i])
		}
		switch summary.Eligibility {
		case clean.CategoryEligibilityDefault, clean.CategoryEligibilityOptIn:
		default:
			t.Fatalf("queue entry %q eligibility = %q", summary.Identifier, summary.Eligibility)
		}
		if summary.Identifier == "administrator_only_caches" {
			t.Fatal("permission boundary must not enter the scan queue")
		}
	}
	assertEagerGroupOrder(t, queue)
}

func TestEagerPreviewQueueFromExtendsWithCatalogEntry(t *testing.T) {
	base := clean.CanonicalCleanupCategoryCatalog().Definitions()
	extra := clean.CleanupCategoryDefinition{
		Identifier:               "test_only_eager_category",
		Label:                    "Test-only eager category",
		ReportCategory:           clean.ReportCategoryDeveloperTools,
		Eligibility:              clean.CategoryEligibilityOptIn,
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		PlannedAction:            clean.PlannedActionMoveToRecycleBin,
	}
	definitions := append(append([]clean.CleanupCategoryDefinition{}, base...), extra)
	catalog, err := clean.NewCleanupCategoryCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}

	queue := clean.EagerPreviewQueueFrom(catalog)
	if len(queue) != len(clean.EagerPreviewQueue())+1 {
		t.Fatalf("queue len = %d, want canonical+1", len(queue))
	}
	last := queue[len(queue)-1]
	if last.Identifier != extra.Identifier || last.Label != extra.Label {
		t.Fatalf("extended queue last = %#v, want extra entry", last)
	}
	if len(queue) < 2 {
		t.Fatal("expected multi-entry catalog-derived queue")
	}
}

func TestEagerPreviewQueueExcludesPermissionBoundaryAndReviewOnly(t *testing.T) {
	catalog, err := clean.NewCleanupCategoryCatalog([]clean.CleanupCategoryDefinition{
		{
			Identifier:               "default_one",
			Label:                    "Default one",
			ReportCategory:           clean.ReportCategoryUserEssentials,
			Eligibility:              clean.CategoryEligibilityDefault,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            clean.PlannedActionMoveToRecycleBin,
		},
		{
			Identifier:               "opt_in_one",
			Label:                    "Opt-in one",
			ReportCategory:           clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityOptIn,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
			PlannedAction:            clean.PlannedActionMoveToRecycleBin,
		},
		{
			Identifier:               "admin_notice",
			Label:                    "Admin notice",
			ReportCategory:           clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityPermissionBoundary,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		},
		{
			Identifier:               "review_clue_like",
			Label:                    "Review only",
			ReportCategory:           clean.ReportCategoryDeveloperTools,
			Eligibility:              clean.CategoryEligibilityReviewOnly,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	queue := clean.EagerPreviewQueueFrom(catalog)
	want := []string{"default_one", "opt_in_one"}
	got := make([]string, len(queue))
	for i, summary := range queue {
		got[i] = summary.Identifier
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("queue = %#v, want %#v", got, want)
	}
}

func TestIsTerminalPreviewState(t *testing.T) {
	terminal := []clean.CategoryPreviewState{
		clean.CategoryPreviewComplete,
		clean.CategoryPreviewPartial,
		clean.CategoryPreviewEmpty,
		clean.CategoryPreviewSkipped,
		clean.CategoryPreviewIncomplete,
		clean.CategoryPreviewFailed,
	}
	for _, state := range terminal {
		if !clean.IsTerminalPreviewState(state) {
			t.Fatalf("%q should be terminal", state)
		}
	}
	for _, state := range []clean.CategoryPreviewState{
		clean.CategoryPreviewWaiting,
		clean.CategoryPreviewScanning,
		clean.CategoryPreviewState("bogus"),
	} {
		if clean.IsTerminalPreviewState(state) {
			t.Fatalf("%q should be non-terminal", state)
		}
	}
}

func TestProjectCategoryPreviewCompleteIncludesZeroByteCandidates(t *testing.T) {
	obs := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DefaultCategoryFoalOwnedTempSandboxes,
		Eligibility: clean.CategoryEligibilityDefault,
		Candidates: []clean.CandidatePreview{{
			Path:  `C:\Users\corey\AppData\Local\Temp\foal-empty.tmp`,
			Bytes: 0,
			Rule:  clean.DefaultCategoryFoalOwnedTempSandboxes,
		}},
	})
	if obs.State != clean.CategoryPreviewComplete || obs.CandidateCount != 1 || obs.Bytes != 0 {
		t.Fatalf("zero-byte complete = %#v", obs)
	}
	if obs.ExcludedSiblingCount != 0 {
		t.Fatalf("complete excluded = %d", obs.ExcludedSiblingCount)
	}
	assertObservationPathFree(t, obs, `C:\Users\corey`, `foal-empty.tmp`)
}

func TestProjectCategoryPreviewCompleteAndEmptyArePathFree(t *testing.T) {
	complete := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DefaultCategoryFoalOwnedTempSandboxes,
		Eligibility: clean.CategoryEligibilityDefault,
		Candidates: []clean.CandidatePreview{{
			Path:  `C:\Users\corey\AppData\Local\Temp\foal-secret.tmp`,
			Bytes: 128,
			Rule:  clean.DefaultCategoryFoalOwnedTempSandboxes,
		}},
	})
	if complete.State != clean.CategoryPreviewComplete || complete.CandidateCount != 1 || complete.Bytes != 128 {
		t.Fatalf("complete = %#v", complete)
	}
	assertObservationPathFree(t, complete, `C:\Users\corey`)

	empty := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.OpportunityCategoryCrashDumps,
		Eligibility: clean.CategoryEligibilityOptIn,
	})
	if empty.State != clean.CategoryPreviewEmpty || empty.CandidateCount != 0 || empty.Bytes != 0 {
		t.Fatalf("empty = %#v", empty)
	}
	if empty.ReasonCode != clean.PreviewReasonEmpty {
		t.Fatalf("empty reason = %q", empty.ReasonCode)
	}
	assertObservationPathFree(t, empty, `C:\`)
}

func TestProjectCategoryPreviewPartialPreservesSafeSiblingsOnly(t *testing.T) {
	secret := `C:\Users\private\protected-sibling`
	obs := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DevCacheCategoryPlaywright,
		Eligibility: clean.CategoryEligibilityOptIn,
		OptInCandidates: []clean.OptInCandidate{{
			Path:     `C:\Users\private\safe-sibling`,
			Bytes:    4096,
			Category: clean.DevCacheCategoryPlaywright,
		}},
		SuppressedProtectionPaths: []string{secret},
		Diagnostics: []clean.StructuredIssue{{
			Code:    clean.PreviewReasonInspectionLimit,
			Message: `walk C:\Users\private\huge-sibling: inspection limit exceeded`,
			Path:    `C:\Users\private\huge-sibling`,
		}},
	})
	if obs.State != clean.CategoryPreviewPartial {
		t.Fatalf("state = %q, want partial", obs.State)
	}
	if obs.CandidateCount != 1 || obs.Bytes != 4096 {
		t.Fatalf("safe totals = %d/%d", obs.CandidateCount, obs.Bytes)
	}
	if obs.ExcludedSiblingCount != 2 {
		t.Fatalf("excluded = %d, want 2 (protected + incomplete)", obs.ExcludedSiblingCount)
	}
	if obs.ReasonCode != clean.PreviewReasonProtected {
		t.Fatalf("reason = %q, want protected (priority)", obs.ReasonCode)
	}
	if obs.SafetyNote == "" {
		t.Fatal("playwright partial should retain shared safety note")
	}
	assertObservationPathFree(t, obs, secret, `C:\Users\private`, "safe-sibling", "huge-sibling")
}

func TestProjectCategoryPreviewNoSafeCandidateCannotBePartial(t *testing.T) {
	obs := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:                clean.OpportunityCategoryCrashDumps,
		Eligibility:               clean.CategoryEligibilityOptIn,
		SuppressedProtectionPaths: []string{`C:\Users\corey\AppData\Local\CrashDumps`},
		Diagnostics: []clean.StructuredIssue{{
			Code:    clean.PreviewReasonInspectionLimit,
			Message: `open C:\Users\corey\AppData\Local\CrashDumps\other: limit`,
			Path:    `C:\Users\corey\AppData\Local\CrashDumps\other`,
		}},
	})
	if obs.State == clean.CategoryPreviewPartial {
		t.Fatalf("no-safe must not be partial: %#v", obs)
	}
	if obs.CandidateCount != 0 || obs.Bytes != 0 {
		t.Fatalf("no-safe must not invent bytes: %#v", obs)
	}
	assertObservationPathFree(t, obs, `CrashDumps`, `C:\Users\corey`)
}

func TestProjectCategoryPreviewProtectionProjection(t *testing.T) {
	allProtected := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:                clean.OpportunityCategoryCrashDumps,
		Eligibility:               clean.CategoryEligibilityOptIn,
		SuppressedProtectionPaths: []string{`C:\Users\corey\AppData\Local\CrashDumps`},
	})
	if allProtected.State != clean.CategoryPreviewSkipped {
		t.Fatalf("all-protected state = %q", allProtected.State)
	}
	if allProtected.ReasonCode != clean.PreviewReasonProtected {
		t.Fatalf("all-protected reason = %q", allProtected.ReasonCode)
	}
	if allProtected.ExcludedSiblingCount != 1 || allProtected.CandidateCount != 0 || allProtected.Bytes != 0 {
		t.Fatalf("all-protected projection = %#v", allProtected)
	}
	assertObservationPathFree(t, allProtected, `CrashDumps`, `C:\Users\corey`)

	partialProtected := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DevCacheCategoryNPM,
		Eligibility: clean.CategoryEligibilityOptIn,
		OptInCandidates: []clean.OptInCandidate{{
			Path:     `C:\Users\corey\.npm\_cacache-safe`,
			Bytes:    80,
			Category: clean.DevCacheCategoryNPM,
		}},
		SuppressedProtectionPaths: []string{`C:\Users\corey\.npm\_cacache-protected`},
	})
	if partialProtected.State != clean.CategoryPreviewPartial {
		t.Fatalf("partly protected state = %q", partialProtected.State)
	}
	if partialProtected.CandidateCount != 1 || partialProtected.Bytes != 80 {
		t.Fatalf("partly protected safe totals = %#v", partialProtected)
	}
	if partialProtected.ExcludedSiblingCount != 1 || partialProtected.ReasonCode != clean.PreviewReasonProtected {
		t.Fatalf("partly protected diagnostics = %#v", partialProtected)
	}
	assertObservationPathFree(t, partialProtected, `_cacache`, `C:\Users\corey`)
}

func TestProjectCategoryPreviewDisabledOutcomesPathFree(t *testing.T) {
	skipped := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DevCacheCategoryGo,
		Eligibility: clean.CategoryEligibilityOptIn,
		Skipped: []clean.SkippedItem{{
			Path: `C:\go-cache`,
			Rule: clean.DevCacheCategoryGo,
			Reason: clean.StructuredIssue{
				Code:    clean.PreviewReasonDevToolRunning,
				Message: "Go is running at C:\\go-cache",
				Path:    `C:\go-cache`,
			},
		}},
	})
	if skipped.State != clean.CategoryPreviewSkipped {
		t.Fatalf("skipped state = %q", skipped.State)
	}
	if skipped.ReasonCode != clean.PreviewReasonDevToolRunning || skipped.Bytes != 0 {
		t.Fatalf("skipped projection = %#v", skipped)
	}
	assertObservationPathFree(t, skipped, `C:\go-cache`)

	failed := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.OpportunityCategoryUserTemp,
		Eligibility: clean.CategoryEligibilityOptIn,
		Diagnostics: []clean.StructuredIssue{{
			Code:    clean.PreviewReasonInspectionFailed,
			Message: `open C:\Users\corey\AppData\Local\Temp: access denied`,
			Path:    `C:\Users\corey\AppData\Local\Temp`,
		}},
	})
	if failed.State != clean.CategoryPreviewFailed {
		t.Fatalf("failed state = %q", failed.State)
	}
	if failed.ReasonCode != clean.PreviewReasonInspectionFailed || failed.Bytes != 0 {
		t.Fatalf("failed projection = %#v", failed)
	}
	assertObservationPathFree(t, failed, `C:\Users\corey`, `access denied`)

	canceled := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DevCacheCategoryNPM,
		Eligibility: clean.CategoryEligibilityOptIn,
		Diagnostics: []clean.StructuredIssue{{
			Code:    clean.PreviewReasonContextCanceled,
			Message: "context canceled while reading C:\\npm-cache",
			Path:    `C:\npm-cache`,
		}},
	})
	if canceled.State != clean.CategoryPreviewIncomplete {
		t.Fatalf("canceled state = %q", canceled.State)
	}
	if canceled.ReasonCode != clean.PreviewReasonContextCanceled || canceled.Bytes != 0 {
		t.Fatalf("canceled projection = %#v", canceled)
	}
	assertObservationPathFree(t, canceled, `C:\npm-cache`)

	incomplete := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.OpportunityCategoryUserTemp,
		Eligibility: clean.CategoryEligibilityOptIn,
		Diagnostics: []clean.StructuredIssue{{
			Code:    clean.PreviewReasonInspectionLimit,
			Message: `walk C:\Users\corey\AppData\Local\Temp\tree: inspection limit exceeded`,
			Path:    `C:\Users\corey\AppData\Local\Temp\tree`,
		}},
	})
	if incomplete.State != clean.CategoryPreviewIncomplete {
		t.Fatalf("incomplete state = %q", incomplete.State)
	}
	if incomplete.ReasonCode != clean.PreviewReasonInspectionLimit || incomplete.Bytes != 0 {
		t.Fatalf("incomplete projection = %#v", incomplete)
	}
	assertObservationPathFree(t, incomplete, `C:\Users\corey`, `Temp\\tree`)
}

func TestProjectCategoryPreviewEmptyIsByCountNotBytes(t *testing.T) {
	// Zero candidates with incidental zero bytes remains empty — not complete.
	empty := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.OpportunityCategoryCrashDumps,
		Eligibility: clean.CategoryEligibilityOptIn,
	})
	if empty.State != clean.CategoryPreviewEmpty {
		t.Fatalf("zero candidates = %q, want empty", empty.State)
	}
}

func TestClassifyEagerPreviewNoWorkDistinctStates(t *testing.T) {
	complete := clean.CategoryPreviewObservation{State: clean.CategoryPreviewComplete, CandidateCount: 1, Bytes: 1}
	empty := clean.CategoryPreviewObservation{State: clean.CategoryPreviewEmpty}
	skipped := clean.CategoryPreviewObservation{State: clean.CategoryPreviewSkipped, ReasonCode: clean.PreviewReasonProtected}
	failed := clean.CategoryPreviewObservation{State: clean.CategoryPreviewFailed}
	waiting := clean.CategoryPreviewObservation{State: clean.CategoryPreviewWaiting}

	if got := clean.ClassifyEagerPreviewNoWork([]clean.CategoryPreviewObservation{complete, empty}, 0); got != clean.EagerPreviewNoWorkNeedSelection {
		t.Fatalf("need selection = %q", got)
	}
	if got := clean.ClassifyEagerPreviewNoWork([]clean.CategoryPreviewObservation{complete}, 1); got != clean.EagerPreviewNoWorkNone {
		t.Fatalf("selected = %q", got)
	}
	if got := clean.ClassifyEagerPreviewNoWork([]clean.CategoryPreviewObservation{empty, empty}, 0); got != clean.EagerPreviewNoWorkAllEmpty {
		t.Fatalf("all empty = %q", got)
	}
	if got := clean.ClassifyEagerPreviewNoWork([]clean.CategoryPreviewObservation{empty, skipped, failed}, 0); got != clean.EagerPreviewNoWorkDiagnostics {
		t.Fatalf("diagnostics = %q", got)
	}
	if got := clean.ClassifyEagerPreviewNoWork([]clean.CategoryPreviewObservation{complete, waiting}, 0); got != clean.EagerPreviewNoWorkNone {
		t.Fatalf("non-terminal = %q", got)
	}
}

func TestCheckEagerPreviewAvailabilityProtectionLoad(t *testing.T) {
	if clean.CheckEagerPreviewAvailability(clean.Options{}) != nil {
		t.Fatal("healthy options must be available")
	}
	unavailable := clean.CheckEagerPreviewAvailability(clean.Options{
		ProtectionLoadError: &clean.StructuredIssue{
			Code:    clean.PreviewReasonProtectionConfigFailed,
			Message: `open C:\Users\corey\AppData\Roaming\Foal\protection.txt: access denied`,
			Path:    `C:\Users\corey\AppData\Roaming\Foal\protection.txt`,
		},
	})
	if unavailable == nil {
		t.Fatal("expected unavailable")
	}
	if unavailable.Code != clean.PreviewReasonProtectionConfigFailed {
		t.Fatalf("code = %q", unavailable.Code)
	}
	if strings.Contains(unavailable.Message, `C:\`) || strings.Contains(unavailable.Message, "protection.txt") {
		t.Fatalf("message leaked path: %#v", unavailable)
	}
}

func TestRunEagerPreviewRefusesGlobalProtectionFailure(t *testing.T) {
	var events int
	err := clean.RunEagerPreview(context.Background(), clean.Options{
		ProtectionLoadError: &clean.StructuredIssue{
			Code:    clean.PreviewReasonProtectionConfigFailed,
			Message: `read C:\secret\protection.txt failed`,
			Path:    `C:\secret\protection.txt`,
		},
	}, func(clean.CategoryPreviewObservation) {
		events++
	})
	if err == nil || !errors.Is(err, clean.ErrEagerPreviewUnavailable) {
		t.Fatalf("err = %v, want ErrEagerPreviewUnavailable", err)
	}
	if events != 0 {
		t.Fatalf("emitted %d category events, want 0", events)
	}
}

func TestRunEagerPreviewStreamsSequentialPathFreeObservations(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-owned.tmp")
	if err := os.WriteFile(candidate, []byte("preview"), 0600); err != nil {
		t.Fatal(err)
	}
	secretPath := `C:\Users\private\secret-cache`
	var historyCalls int
	var reviewProbeCalls int
	adapter := &recordingRecycleBinAdapter{}

	var events []clean.CategoryPreviewObservation
	var scanningSeen []string
	err := clean.RunEagerPreview(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
		Validator: pathsafe.NewValidator(nil),
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{}
		},
		DevCachePathResolver: func(string) []string { return nil },
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return nil
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			reviewProbeCalls++
			return []clean.ReviewSuggestion{{
				Tool:      "npm",
				Label:     "npm cache",
				Command:   "npm cache clean --force",
				CachePath: secretPath,
			}}
		},
		RecycleBinAdapter: adapter,
		HistoryRecorder:   &eagerHistoryRecorder{onRecord: func() { historyCalls++ }},
		DetailedListDir:   root,
	}, func(obs clean.CategoryPreviewObservation) {
		events = append(events, obs)
		if obs.State == clean.CategoryPreviewScanning {
			scanningSeen = append(scanningSeen, obs.Identifier)
		}
		assertObservationPathFree(t, obs, secretPath, candidate, root)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("cleanup adapter paths = %#v, want none", adapter.paths)
	}
	if historyCalls != 0 {
		t.Fatalf("history writes = %d, want 0", historyCalls)
	}
	if reviewProbeCalls != 0 {
		t.Fatalf("review suggestion probes = %d, want 0", reviewProbeCalls)
	}

	queue := clean.EagerPreviewQueue()
	if len(scanningSeen) != len(queue) {
		t.Fatalf("scanning events = %d, want %d", len(scanningSeen), len(queue))
	}
	if len(events) != len(queue)*2 {
		t.Fatalf("events = %d, want 2 per queue entry (%d)", len(events), len(queue)*2)
	}
	for i, summary := range queue {
		scan := events[i*2]
		term := events[i*2+1]
		if scan.Identifier != summary.Identifier || scan.State != clean.CategoryPreviewScanning {
			t.Fatalf("scan event[%d] = %#v", i, scan)
		}
		if term.Identifier != summary.Identifier || !clean.IsTerminalPreviewState(term.State) {
			t.Fatalf("terminal event[%d] = %#v", i, term)
		}
		if i > 0 {
			prevTerm := events[i*2-1]
			if prevTerm.State == clean.CategoryPreviewScanning {
				t.Fatal("previous category still scanning when next started")
			}
		}
	}

	defaultObs := events[1]
	if defaultObs.Identifier != clean.DefaultCategoryFoalOwnedTempSandboxes {
		t.Fatalf("first terminal = %#v", defaultObs)
	}
	if defaultObs.State != clean.CategoryPreviewComplete || defaultObs.CandidateCount != 1 || defaultObs.Bytes != int64(len("preview")) {
		t.Fatalf("default observation = %#v", defaultObs)
	}
}

func TestRunEagerPreviewHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var scanned []string
	err := clean.RunEagerPreview(ctx, clean.Options{
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{}
		},
		DevCachePathResolver: func(string) []string { return nil },
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return nil
		},
	}, func(obs clean.CategoryPreviewObservation) {
		if obs.State == clean.CategoryPreviewScanning {
			scanned = append(scanned, obs.Identifier)
			if len(scanned) == 1 {
				cancel()
			}
		}
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if len(scanned) > 2 {
		t.Fatalf("scanned after cancel = %#v, want at most early stop", scanned)
	}
}

func TestRunEagerPreviewProtectionAndInspectionLimitIntegration(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected-cache")
	safe := filepath.Join(root, "safe-cache")
	for _, dir := range []string{protected, safe} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte("xx"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	validator := pathsafe.NewValidator([]string{protected})

	resolution, err := clean.ResolveCategory(context.Background(), clean.Options{
		Validator: validator,
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{protected, safe}
			}
			return nil
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return nil
		},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{}
		},
	}, clean.DevCacheCategoryNPM)
	if err != nil {
		t.Fatal(err)
	}
	obs := clean.ProjectCategoryPreview(resolution)
	if obs.State != clean.CategoryPreviewPartial {
		t.Fatalf("npm with mixed protection = %#v", obs)
	}
	if obs.CandidateCount != 1 || obs.Bytes <= 0 {
		t.Fatalf("safe sibling missing: %#v", obs)
	}
	if obs.ExcludedSiblingCount < 1 || obs.ReasonCode != clean.PreviewReasonProtected {
		t.Fatalf("protection projection = %#v", obs)
	}
	assertObservationPathFree(t, obs, protected, safe, root)
}

func TestDryRunAndExecuteDoNotInvokeEagerPreview(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "default.tmp")
	if err := os.WriteFile(candidate, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	result := clean.DryRun(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{}
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return nil
		},
	})
	if result.Mode != "dry_run" || len(result.Candidates) != 1 {
		t.Fatalf("dry-run changed: %#v", result)
	}

	adapter := &recordingRecycleBinAdapter{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		RecycleBinAdapter: adapter,
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidate},
		}},
	})
	if execResult.Totals.DeletedCount != 1 || len(adapter.paths) != 1 {
		t.Fatalf("execute changed: totals=%#v paths=%v", execResult.Totals, adapter.paths)
	}
}

func assertEagerGroupOrder(t *testing.T, queue []clean.CleanupCategorySummary) {
	t.Helper()
	wantOrder := []clean.ReportCategory{
		clean.ReportCategoryUserEssentials,
		clean.ReportCategorySystem,
		clean.ReportCategoryBrowsers,
		clean.ReportCategoryDeveloperTools,
		clean.ReportCategoryApplications,
	}
	rank := map[clean.ReportCategory]int{}
	for i, group := range wantOrder {
		rank[group] = i
	}
	prev := -1
	for _, summary := range queue {
		r, ok := rank[summary.ReportCategory]
		if !ok {
			t.Fatalf("unexpected report category %q on %q", summary.ReportCategory, summary.Identifier)
		}
		if r < prev {
			t.Fatalf("report groups out of order around %q (%s)", summary.Identifier, summary.ReportCategory)
		}
		prev = r
	}
}

func assertObservationPathFree(t *testing.T, obs clean.CategoryPreviewObservation, forbidden ...string) {
	t.Helper()
	encoded, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, path := range forbidden {
		if path != "" && strings.Contains(text, path) {
			t.Fatalf("observation leaked %q: %s", path, text)
		}
	}
	if strings.ContainsAny(obs.Identifier, `/\`) || strings.ContainsAny(obs.Label, `/\`) {
		t.Fatalf("path-like identifier/label: %#v", obs)
	}
	if strings.Contains(obs.ReasonCode, `\`) || strings.Contains(obs.ReasonCode, `/`) {
		t.Fatalf("path-like reason: %#v", obs)
	}
	if strings.Contains(obs.SafetyNote, `C:\`) || strings.Contains(obs.SafetyNote, `\\?\`) {
		t.Fatalf("path-like safety note: %#v", obs)
	}
}

type eagerHistoryRecorder struct {
	onRecord func()
}

func (r *eagerHistoryRecorder) Record(context.Context, history.SessionRecord, []history.ItemRecord) error {
	if r.onRecord != nil {
		r.onRecord()
	}
	return nil
}
