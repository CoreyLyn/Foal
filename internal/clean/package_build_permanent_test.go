package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// packageBuildPermanentCategories is the #221 permanent-deletion activation set.
// Resolvers, gates, and impact notices stay on existing seams; only planned
// action + authorization/selection contracts change here.
func packageBuildPermanentCategories() []string {
	return []string{
		clean.DevCacheCategoryNPM,
		clean.DevCacheCategoryPNPM,
		clean.DevCacheCategoryYarn,
		clean.DevCacheCategoryGo,
		clean.DevCacheCategoryGoModCache,
		clean.DevCacheCategoryPip,
		clean.DevCacheCategoryCargo,
		clean.DevCacheCategoryNuGet,
		clean.DevCacheCategoryNuGetGlobalPackages,
		clean.DevCacheCategoryCorepack,
		clean.DevCacheCategoryUV,
		clean.DevCacheCategoryBun,
	}
}

func TestPackageBuildCachesDeclarePermanentPlannedAction(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	for _, id := range packageBuildPermanentCategories() {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("%q missing from catalog", id)
		}
		if summary.PlannedAction != clean.DeletionActionDeletePermanently {
			t.Fatalf("%q planned_action = %q, want delete_permanently", id, summary.PlannedAction)
		}
		if summary.Eligibility != clean.CategoryEligibilityOptIn {
			t.Fatalf("%q eligibility = %q, want opt-in", id, summary.Eligibility)
		}
		if !clean.InitiallySelectedCategory(summary) {
			t.Fatalf("%q must start selected when measurable", id)
		}
	}
}

func TestPackageBuildCachesPreserveRunningApplicationPolicies(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	want := map[string]clean.RunningApplicationPolicy{
		clean.DevCacheCategoryNPM:                 clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryPNPM:                clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryYarn:                clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryGo:                  clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryGoModCache:          clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryPip:                 clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryCargo:               clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryNuGet:               clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryNuGetGlobalPackages: clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryCorepack:            clean.RunningApplicationPolicySharedRuntime,
		clean.DevCacheCategoryUV:                  clean.RunningApplicationPolicyDistinctiveProcessIdle,
		clean.DevCacheCategoryBun:                 clean.RunningApplicationPolicyDistinctiveProcessIdle,
	}
	for id, policy := range want {
		summary, ok := catalog.Summary(id)
		if !ok {
			t.Fatalf("%q missing", id)
		}
		if summary.RunningApplicationPolicy != policy {
			t.Fatalf("%q policy = %q, want %q", id, summary.RunningApplicationPolicy, policy)
		}
	}
}

func TestPackageBuildDryRunReportsPermanentWithoutAuthorization(t *testing.T) {
	for _, category := range packageBuildPermanentCategories() {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			cachePath := filepath.Join(root, category)
			if err := os.Mkdir(cachePath, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("cache"), 0600); err != nil {
				t.Fatal(err)
			}

			result := clean.DryRun(context.Background(), clean.Options{
				AllowPermanentDeletion: false,
				OptIn:                  []string{category},
				DevCachePathResolver: func(cat string) []string {
					if cat == category {
						return []string{cachePath}
					}
					return nil
				},
				DiscoverOpportunities:     noOpportunities,
				DiscoverReviewSuggestions: noReviewSuggestions,
			})

			if len(result.OptInCandidates) != 1 {
				t.Fatalf("opt-in candidates = %#v", result.OptInCandidates)
			}
			candidate := result.OptInCandidates[0]
			if candidate.Category != category {
				t.Fatalf("category = %q", candidate.Category)
			}
			if candidate.PlannedAction != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("planned_action = %q, want delete_permanently", candidate.PlannedAction)
			}
			if len(result.Deleted) != 0 {
				t.Fatalf("dry-run must not delete: %#v", result.Deleted)
			}
			if _, err := os.Lstat(cachePath); err != nil {
				t.Fatalf("dry-run must leave cache intact: %v", err)
			}

			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			body := string(encoded)
			if !strings.Contains(body, `"planned_action":"delete_permanently"`) {
				t.Fatalf("JSON missing permanent planned_action: %s", body)
			}
			if strings.Contains(strings.ToLower(body), "secure erase") || strings.Contains(strings.ToLower(body), "shred") {
				t.Fatalf("must not claim secure erase: %s", body)
			}
		})
	}
}

func TestPackageBuildExecuteWithoutAllowPermanentSkips(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb12")
	cachePath := filepath.Join(root, "npm-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("npm!"), 0600); err != nil {
		t.Fatal(err)
	}

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{cachePath}
			}
			return nil
		},
		RecycleBinAdapter:         recycle,
		PermanentRemover:          permanent,
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
	})

	if len(recycle.paths) != 1 || recycle.paths[0] != recyclePath {
		t.Fatalf("recycle paths = %v, want default only", recycle.paths)
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover called without auth: %v", permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.DeletionActionMoveToRecycleBin) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Reason.Code != "permanent_deletion_not_authorized" {
		t.Fatalf("skip code = %q", skipped.Reason.Code)
	}
	if skipped.PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned action changed: %q", skipped.PlannedAction)
	}
	if skipped.Rule != clean.DevCacheCategoryNPM {
		t.Fatalf("skip rule = %q", skipped.Rule)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 4 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if _, err := os.Lstat(cachePath); err != nil {
		t.Fatalf("unauthorized permanent path must remain: %v", err)
	}
}

func TestPackageBuildExecuteWithAllowPermanentDispatchesPermanentRemover(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb")
	cachePath := filepath.Join(root, "go-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("gocache"), 0600); err != nil {
		t.Fatal(err)
	}

	collab := &orderedCollaborators{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.DevCacheCategoryGo},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryGo {
				return []string{cachePath}
			}
			return nil
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{{
				Application: clean.ApplicationGo,
				State:       clean.RunningApplicationStateIdle,
			}}
		},
		RecycleBinAdapter:         collab,
		PermanentRemover:          collab,
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
	})

	if len(collab.calls) != 2 {
		t.Fatalf("calls = %#v, want recycle then permanent", collab.calls)
	}
	if collab.calls[0].kind != "recycle" || collab.calls[0].path != recyclePath {
		t.Fatalf("first call = %#v", collab.calls[0])
	}
	if collab.calls[1].kind != "permanent" || collab.calls[1].path != cachePath {
		t.Fatalf("second call = %#v, want permanent go-cache", collab.calls[1])
	}

	byRule := map[string]clean.DeletedItem{}
	for _, item := range result.Deleted {
		byRule[item.Rule] = item
	}
	if byRule[clean.DefaultCategoryFoalOwnedTempSandboxes].Action != string(clean.DeletionActionMoveToRecycleBin) {
		t.Fatalf("recycle deleted = %#v", byRule[clean.DefaultCategoryFoalOwnedTempSandboxes])
	}
	goDeleted, ok := byRule[clean.DevCacheCategoryGo]
	if !ok || goDeleted.Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("go deleted = %#v", byRule[clean.DevCacheCategoryGo])
	}
	if result.Totals.RecycleBinMovedBytes != 2 {
		t.Fatalf("recycle_bin_moved_bytes = %d", result.Totals.RecycleBinMovedBytes)
	}
	if result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("permanently_deleted_bytes = 0, want go content")
	}
	if result.Totals.AffectedBytes != result.Totals.RecycleBinMovedBytes+result.Totals.PermanentlyDeletedBytes {
		t.Fatalf("affected_bytes = %d, want sum %#v", result.Totals.AffectedBytes, result.Totals)
	}
	if _, err := os.Lstat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("go cache still exists after permanent delete: %v", err)
	}

	foundPermanentHistory := false
	for _, item := range recorder.items {
		if item.Rule == clean.DevCacheCategoryGo && item.Result == "deleted" {
			foundPermanentHistory = true
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
		}
	}
	if !foundPermanentHistory {
		t.Fatalf("history missing go permanent success: %#v", recorder.items)
	}
}

func TestPackageBuildNineCategoriesExecutePermanentlyWhenAuthorized(t *testing.T) {
	for _, category := range packageBuildPermanentCategories() {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			cachePath := filepath.Join(root, category)
			if err := os.Mkdir(cachePath, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}

			permanent := &recordingPermanentRemover{}
			recycle := &recordingRecycleBinAdapter{}
			result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
				AllowPermanentDeletion: true,
				OptIn:                  []string{category},
				DevCachePathResolver: func(cat string) []string {
					if cat == category {
						return []string{cachePath}
					}
					return nil
				},
				// Distinctive-process categories need an idle snapshot to pass gates.
				DetectRunningApplications: idleSnapshotForPackageBuildCategory(category),
				RecycleBinAdapter:         recycle,
				PermanentRemover:          permanent,
				DiscoverOpportunities:     noOpportunities,
				DiscoverReviewSuggestions: noReviewSuggestions,
				Rules: []clean.Rule{{
					ID:             "disabled",
					DefaultEnabled: false,
				}},
			})

			if len(recycle.paths) != 0 {
				t.Fatalf("recycle adapter called for permanent category: %v", recycle.paths)
			}
			if len(permanent.paths) != 1 || permanent.paths[0] != cachePath {
				t.Fatalf("permanent paths = %v, want [%q]", permanent.paths, cachePath)
			}
			if len(result.Deleted) != 1 {
				t.Fatalf("deleted = %#v", result.Deleted)
			}
			if result.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("action = %q", result.Deleted[0].Action)
			}
			if result.Deleted[0].Rule != category {
				t.Fatalf("rule = %q", result.Deleted[0].Rule)
			}
			if result.Totals.PermanentlyDeletedBytes == 0 || result.Totals.RecycleBinMovedBytes != 0 {
				t.Fatalf("totals = %#v", result.Totals)
			}
			if _, err := os.Lstat(cachePath); !os.IsNotExist(err) {
				t.Fatalf("cache still exists: %v", err)
			}
		})
	}
}

func TestPackageBuildCapacityCheckExcludesPermanentBytes(t *testing.T) {
	root := t.TempDir()
	// Recycle candidate is 4 bytes; npm tree is large. Capacity of 10 must accept
	// recycle-only budget and must not sum permanent npm into the volume budget.
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb12")
	cachePath := filepath.Join(root, "npm-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "big.bin"), make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}

	collab := &orderedCollaborators{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.DevCacheCategoryNPM},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryNPM {
				return []string{cachePath}
			}
			return nil
		},
		RecycleBinAdapter:         collab,
		PermanentRemover:          collab,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
		RecycleBinCapacityProbe: func(string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{Volume: "v", MaxCapacity: 10, CurrentUsage: 0}, nil
		},
	})

	if len(collab.calls) != 2 {
		t.Fatalf("calls = %#v, want both actions (permanent excluded from capacity)", collab.calls)
	}
	if result.Totals.RecycleBinMovedBytes != 4 || result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}

func TestPackageBuildPermanentFailureNeverFallsBackToRecycleBin(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "pip-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("pip"), 0600); err != nil {
		t.Fatal(err)
	}

	recycle := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.DevCacheCategoryPip},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryPip {
				return []string{cachePath}
			}
			return nil
		},
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: false,
		}},
		RecycleBinAdapter: recycle,
		PermanentRemover: permanentRemoverFunc(func(context.Context, string) error {
			return os.ErrPermission
		}),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(recycle.paths) != 0 {
		t.Fatalf("permanent failure must never fall back to Recycle Bin: %v", recycle.paths)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("failed permanent must contribute zero action bytes: %#v", result.Totals)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("failed = %#v", result.Failed)
	}
	failed := result.Failed[0]
	if failed.Rule != clean.DevCacheCategoryPip {
		t.Fatalf("failed rule = %q", failed.Rule)
	}
	if failed.Action != string(clean.DeletionActionDeletePermanently) ||
		failed.PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("failed actions = %#v", failed)
	}
	if failed.Reason.Code != "permanent_delete_failed" {
		t.Fatalf("failed code = %q", failed.Reason.Code)
	}
	if _, err := os.Lstat(cachePath); err != nil {
		t.Fatalf("pip cache missing after failed permanent: %v", err)
	}
}

func TestNuGetGlobalPackagesImpactNoticePreserved(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "nuget-global")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "pkg"), []byte("nupkg"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryNuGetGlobalPackages},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryNuGetGlobalPackages {
				return []string{cachePath}
			}
			return nil
		},
		DetectRunningApplications: idleSnapshotForPackageBuildCategory(clean.DevCacheCategoryNuGetGlobalPackages),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v", result.OptInCandidates)
	}
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, notice := range model.Notices {
		msg := strings.ToLower(notice.Message)
		if notice.Kind == "opt_in_impact" &&
			strings.Contains(msg, "offline") &&
			strings.Contains(msg, "private-source") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notices = %#v, want nuget global packages offline/private-source impact", model.Notices)
	}
}

func TestGoModCacheImpactNoticePreservedWithPermanentAction(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-modcache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data.bin"), []byte("mod"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryGoModCache},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryGoModCache {
				return []string{cachePath}
			}
			return nil
		},
		DetectRunningApplications: idleSnapshotForPackageBuildCategory(clean.DevCacheCategoryGoModCache),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v", result.OptInCandidates)
	}
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "re-downloading modules") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notices = %#v, want go-modcache re-download impact notice", model.Notices)
	}
}

func TestUVCacheImpactNoticePreservedWithPermanentAction(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "uv-cache")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "data"), []byte("uv"), 0600); err != nil {
		t.Fatal(err)
	}
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryUV},
		DevCachePathResolver: func(category string) []string {
			if category == clean.DevCacheCategoryUV {
				return []string{cachePath}
			}
			return nil
		},
		DetectRunningApplications: idleSnapshotForPackageBuildCategory(clean.DevCacheCategoryUV),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v", result.OptInCandidates)
	}
	if result.OptInCandidates[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned_action = %q", result.OptInCandidates[0].PlannedAction)
	}
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "not zero-impact") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notices = %#v, want uv non-zero-impact notice", model.Notices)
	}
}

func idleSnapshotForPackageBuildCategory(category string) func(context.Context) []clean.RunningApplicationState {
	apps := map[string][]string{
		clean.DevCacheCategoryGo:                  {clean.ApplicationGo},
		clean.DevCacheCategoryGoModCache:          {clean.ApplicationGo},
		clean.DevCacheCategoryCargo:               {clean.ApplicationCargo},
		clean.DevCacheCategoryNuGet:               {clean.ApplicationDotNet, clean.ApplicationNuGet},
		clean.DevCacheCategoryNuGetGlobalPackages: {clean.ApplicationDotNet, clean.ApplicationNuGet},
		clean.DevCacheCategoryUV:                  {clean.ApplicationUV},
		clean.DevCacheCategoryBun:                 {clean.ApplicationBun},
	}
	return func(context.Context) []clean.RunningApplicationState {
		ids := apps[category]
		if len(ids) == 0 {
			return nil
		}
		out := make([]clean.RunningApplicationState, 0, len(ids))
		for _, id := range ids {
			out = append(out, clean.RunningApplicationState{
				Application: id,
				State:       clean.RunningApplicationStateIdle,
			})
		}
		return out
	}
}
