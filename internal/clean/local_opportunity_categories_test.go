package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestLocalOpportunityCategoriesSelectOnlyExactQuietArtifacts(t *testing.T) {
	base := t.TempDir()
	roaming := filepath.Join(base, "roaming")
	local := filepath.Join(base, "local")
	now := time.Now().Truncate(time.Second)
	old := now.Add(-40 * 24 * time.Hour)
	fresh := now.Add(-time.Hour)
	lark := filepath.Join(roaming, "LarkShell", "aha", "users", "0123456789abcdef0123456789abcdef", "profile_explorer")
	notion := filepath.Join(roaming, "Notion", "Partitions", "notion")
	rdp := filepath.Join(local, "Temp", "DiagOutputDir", "RdClientAutoTrace")
	trae := filepath.Join(roaming, "TRAE SOLO CN", "ModularData", "ai-agent", "vm", "tools_staging")
	want := []string{
		filepath.Join(lark, "Cache"),
		filepath.Join(lark, "Service Worker", "CacheStorage"),
		filepath.Join(notion, "Cache"),
		filepath.Join(notion, "Service Worker", "CacheStorage"),
		filepath.Join(rdp, "RdClientAutoTrace-WppAutoTrace-20260901-120000-123.etl"),
		filepath.Join(trae, "tools-1.0.13.zip"),
	}
	for _, path := range want {
		if filepath.Ext(path) == ".etl" || filepath.Ext(path) == ".zip" {
			writeExactFixtureFile(t, path, "safe", old)
		} else {
			writeExactFixtureFile(t, filepath.Join(path, "entry"), "safe", old)
		}
	}
	for _, path := range []string{
		filepath.Join(lark, "IndexedDB", "keep"),
		filepath.Join(lark, "Service Worker", "ScriptCache", "keep"),
		filepath.Join(roaming, "LarkShell", "aha", "users", "not-an-account", "profile_explorer", "Cache", "keep"),
		filepath.Join(notion, "IndexedDB", "keep"),
		filepath.Join(rdp, "MSRDCEventProcessor_0.etl"),
		filepath.Join(rdp, "RdClientAutoTrace-WppAutoTrace-20260928-120000-123.etl"),
		filepath.Join(trae, "tools-1.0.14.zip"),
		filepath.Join(trae, "tools-latest.zip"),
	} {
		stamp := old
		if filepath.Base(path) == "tools-1.0.14.zip" || filepath.Base(path) == "RdClientAutoTrace-WppAutoTrace-20260928-120000-123.etl" {
			stamp = fresh
		}
		writeExactFixtureFile(t, path, "keep", stamp)
	}
	options := clean.Options{
		OptIn: []string{clean.CategoryLarkProfileCache, clean.CategoryRDPClientOldTraces, clean.CategoryNotionPartitionCache, clean.CategoryTraeSoloToolsStaging},
		ExactCandidateDiscoveryOptions: clean.ExactCandidateDiscoveryOptions{
			LookupEnv: func(name string) (string, bool) {
				switch name {
				case "APPDATA":
					return roaming, true
				case "LOCALAPPDATA":
					return local, true
				}
				return "", false
			},
			Now: now,
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationLark, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationNotion, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationTraeSolo, State: clean.RunningApplicationStateIdle},
				{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateIdle},
			}
		},
		DiscoverOpportunities:         noOpportunities,
		DiscoverUserTempOpportunities: noUserTempOpportunities,
		DiscoverReviewSuggestions:     noReviewSuggestions,
		Rules:                         []clean.Rule{{ID: "test", DefaultEnabled: false}},
	}
	result := clean.DryRun(context.Background(), options)
	var got []string
	for _, item := range result.OptInCandidates {
		got = append(got, item.Path)
		if item.PlannedAction != string(clean.PlannedActionMoveToRecycleBin) {
			t.Fatalf("wrong action: %#v", item)
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v (diagnostics=%#v)", got, want, result.Errors)
	}

	// A running Lark instance suppresses only its own cache, including the
	// Service Worker candidate, without blocking independent categories.
	options.DetectRunningApplications = func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationLark, State: clean.RunningApplicationStateRunning},
			{Application: clean.ApplicationNotion, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationTraeSolo, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateIdle},
		}
	}
	result = clean.DryRun(context.Background(), options)
	for _, item := range result.OptInCandidates {
		if item.Category == clean.CategoryLarkProfileCache {
			t.Fatalf("running Lark leaked candidate: %#v", item)
		}
	}
	if len(result.OptInCandidates) != len(want)-2 {
		t.Fatalf("independent candidates = %#v", result.OptInCandidates)
	}

	// Quiet-file categories must be re-discovered at execution time: touching
	// a staged archive removes its eligibility without deleting it.
	if err := os.Chtimes(filepath.Join(trae, "tools-1.0.13.zip"), now, now); err != nil {
		t.Fatal(err)
	}
	options.DetectRunningApplications = func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationLark, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationNotion, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationTraeSolo, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationTrae, State: clean.RunningApplicationStateIdle},
		}
	}
	result = clean.DryRun(context.Background(), options)
	for _, item := range result.OptInCandidates {
		if item.Path == filepath.Join(trae, "tools-1.0.13.zip") {
			t.Fatal("freshly modified staging archive remained eligible")
		}
	}

	// A trace can become recent after fresh resolution and before the shared
	// Recycle Bin adapter runs. The category-owned hook must reject it.
	options.OptIn = []string{clean.CategoryRDPClientOldTraces}
	options.RecycleBinAdapter = &recordingRecycleBinAdapter{}
	options.ProgressReporter = func(progress clean.ExecutionProgress) {
		if progress.Phase == clean.ExecutionPhaseRecycleBinOperations {
			path := filepath.Join(rdp, "RdClientAutoTrace-WppAutoTrace-20260901-120000-123.etl")
			if err := os.Chtimes(path, now, now); err != nil {
				t.Errorf("touch trace: %v", err)
			}
		}
	}
	result = executeCleanWithSafeCapacity(context.Background(), options)
	if len(result.Deleted) != 0 {
		t.Fatalf("recent trace moved despite revalidation: %#v", result.Deleted)
	}
	foundMismatch := false
	for _, item := range result.Skipped {
		if item.Rule == clean.CategoryRDPClientOldTraces && item.Reason.Code == "identity_mismatch" {
			foundMismatch = true
		}
	}
	if !foundMismatch {
		t.Fatalf("missing identity_mismatch skip: %#v", result.Skipped)
	}

	options.OptIn = []string{clean.CategoryLarkProfileCache}
	options.DetectRunningApplications = nil
	options.ProgressReporter = nil
	result = clean.DryRun(context.Background(), options)
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("missing application detector leaked Lark candidates: %#v", result.OptInCandidates)
	}
}

func TestLocalOpportunityCategoriesCannotEnterAggregateSelection(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	all, invalid, _ := clean.NormalizedOptInSet([]string{"all"})
	if len(invalid) != 0 {
		t.Fatalf("invalid all token: %v", invalid)
	}
	for _, id := range []string{
		clean.CategoryLarkProfileCache,
		clean.CategoryRDPClientOldTraces,
		clean.CategoryNotionPartitionCache,
		clean.CategoryTraeSoloToolsStaging,
	} {
		summary, ok := catalog.Summary(id)
		if !ok || summary.SelectionPolicy != clean.CategorySelectionPolicyExactOnly ||
			summary.PlannedAction != clean.PlannedActionMoveToRecycleBin ||
			clean.InitiallySelectedCategory(summary) || all[id] {
			t.Fatalf("category %s has unsafe aggregate selection: %#v, all=%t", id, summary, all[id])
		}
	}
}
