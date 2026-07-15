package clean_test

import (
	"context"
	"encoding/json"
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
	// Queue size is catalog-derived: no fixed count assertion against a TUI constant.
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
		},
		{
			Identifier:               "opt_in_one",
			Label:                    "Opt-in one",
			ReportCategory:           clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityOptIn,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
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
	assertObservationPathFree(t, empty, `C:\`)
}

func TestProjectCategoryPreviewConservativeDisabledOutcomes(t *testing.T) {
	skipped := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DevCacheCategoryGo,
		Eligibility: clean.CategoryEligibilityOptIn,
		Skipped: []clean.SkippedItem{{
			Path: `C:\go-cache`,
			Rule: clean.DevCacheCategoryGo,
			Reason: clean.StructuredIssue{
				Code:    "application_running",
				Message: "Go is running",
				Path:    `C:\go-cache`,
			},
		}},
	})
	if skipped.State != clean.CategoryPreviewSkipped {
		t.Fatalf("skipped state = %q", skipped.State)
	}
	assertObservationPathFree(t, skipped, `C:\go-cache`)

	failed := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.OpportunityCategoryUserTemp,
		Eligibility: clean.CategoryEligibilityOptIn,
		Diagnostics: []clean.StructuredIssue{{
			Code:    "inspection_failed",
			Message: `open C:\Users\corey\AppData\Local\Temp: access denied`,
			Path:    `C:\Users\corey\AppData\Local\Temp`,
		}},
	})
	if failed.State != clean.CategoryPreviewFailed {
		t.Fatalf("failed state = %q", failed.State)
	}
	assertObservationPathFree(t, failed, `C:\Users\corey`)

	canceled := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:  clean.DevCacheCategoryNPM,
		Eligibility: clean.CategoryEligibilityOptIn,
		Diagnostics: []clean.StructuredIssue{{
			Code:    "context_canceled",
			Message: "context canceled",
			Path:    `C:\npm-cache`,
		}},
	})
	if canceled.State != clean.CategoryPreviewIncomplete {
		t.Fatalf("canceled state = %q", canceled.State)
	}
	assertObservationPathFree(t, canceled, `C:\npm-cache`)

	protected := clean.ProjectCategoryPreview(clean.CategoryResolution{
		Identifier:                clean.OpportunityCategoryCrashDumps,
		Eligibility:               clean.CategoryEligibilityOptIn,
		SuppressedProtectionPaths: []string{`C:\Users\corey\AppData\Local\CrashDumps`},
	})
	if protected.State != clean.CategoryPreviewSkipped {
		t.Fatalf("protected state = %q", protected.State)
	}
	assertObservationPathFree(t, protected, `CrashDumps`)
}

func TestRunEagerPreviewStreamsSequentialPathFreeObservations(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "foal-owned.tmp")
	if err := os.WriteFile(candidate, []byte("preview"), 0600); err != nil {
		t.Fatal(err)
	}
	// Sentinel private paths that must never appear in observations.
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
	// Exactly one category is scanning at emit time: scanning and terminal alternate.
	if len(events) != len(queue)*2 {
		t.Fatalf("events = %d, want 2 per queue entry (%d)", len(events), len(queue)*2)
	}
	for i, summary := range queue {
		scan := events[i*2]
		term := events[i*2+1]
		if scan.Identifier != summary.Identifier || scan.State != clean.CategoryPreviewScanning {
			t.Fatalf("scan event[%d] = %#v", i, scan)
		}
		if term.Identifier != summary.Identifier || term.State == clean.CategoryPreviewWaiting || term.State == clean.CategoryPreviewScanning {
			t.Fatalf("terminal event[%d] = %#v", i, term)
		}
		// Later categories are not scanned before earlier ones finish: the
		// scanning stream is strictly sequential by queue order.
		if i > 0 {
			prevTerm := events[i*2-1]
			if prevTerm.State == clean.CategoryPreviewScanning {
				t.Fatal("previous category still scanning when next started")
			}
		}
	}

	// Default category with candidate is complete; others empty under stubs.
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

func TestDryRunAndExecuteDoNotInvokeEagerPreview(t *testing.T) {
	// Guard against accidental wiring: DryRun still uses additive selected-only
	// resolution and does not require eager-preview side channels.
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
	// Structural: observation fields themselves must not look path-bearing.
	if strings.ContainsAny(obs.Identifier, `/\`) || strings.ContainsAny(obs.Label, `/\`) {
		t.Fatalf("path-like identifier/label: %#v", obs)
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
