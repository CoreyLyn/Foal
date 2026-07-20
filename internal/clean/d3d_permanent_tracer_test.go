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

// d3d_shader_cache is the #219 production permanent-deletion tracer. These tests
// exercise the live catalog action end-to-end (no CategoryPlannedActions injection).

func TestD3DDryRunReportsPermanentPlannedActionWithoutAuthorization(t *testing.T) {
	root := t.TempDir()
	d3dRoot := filepath.Join(root, "D3DSCache")
	if err := os.Mkdir(d3dRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d3dRoot, "shader.bin"), []byte("d3d-cache"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		// Dry-run must not require permanent authorization.
		AllowPermanentDeletion:    false,
		OptIn:                     []string{clean.OpportunityCategoryD3DShaderCache},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryD3DShaderCache,
					Path:     d3dRoot,
					Bytes:    9,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("opt-in candidates = %#v", result.OptInCandidates)
	}
	candidate := result.OptInCandidates[0]
	if candidate.Category != clean.OpportunityCategoryD3DShaderCache {
		t.Fatalf("category = %q", candidate.Category)
	}
	if candidate.PlannedAction != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("planned_action = %q, want delete_permanently", candidate.PlannedAction)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("dry-run must not delete: %#v", result.Deleted)
	}
	if _, err := os.Lstat(d3dRoot); err != nil {
		t.Fatalf("dry-run must leave d3d root intact: %v", err)
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
}

func TestD3DExecuteWithoutAllowPermanentSkipsAndContinuesRecycleBin(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb12")
	d3dRoot := filepath.Join(root, "D3DSCache")
	if err := os.Mkdir(d3dRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d3dRoot, "shader.bin"), []byte("shader!"), 0600); err != nil {
		t.Fatal(err)
	}

	recycle := &recordingRecycleBinAdapter{}
	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryD3DShaderCache},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		HistoryRecorder:        recorder,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryD3DShaderCache,
					Path:     d3dRoot,
					Bytes:    7,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
	})

	if len(recycle.paths) != 1 || recycle.paths[0] != recyclePath {
		t.Fatalf("recycle paths = %v, want default only", recycle.paths)
	}
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover called without auth: %v", permanent.paths)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Reason.Code != "permanent_deletion_not_authorized" {
		t.Fatalf("skip code = %q", skipped.Reason.Code)
	}
	if skipped.PlannedAction != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("planned action changed: %q", skipped.PlannedAction)
	}
	if skipped.Rule != clean.OpportunityCategoryD3DShaderCache {
		t.Fatalf("skip rule = %q", skipped.Rule)
	}
	if result.Totals.PermanentlyDeletedBytes != 0 || result.Totals.RecycleBinMovedBytes != 4 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if _, err := os.Lstat(d3dRoot); err != nil {
		t.Fatalf("unauthorized permanent path must remain: %v", err)
	}
	foundAuthSkip := false
	for _, item := range recorder.items {
		if item.Result == "skipped" && item.SkippedReason != nil && item.SkippedReason.Code == "permanent_deletion_not_authorized" {
			foundAuthSkip = true
			if item.PlannedAction != string(clean.PlannedActionDeletePermanently) {
				t.Fatalf("history planned action = %q", item.PlannedAction)
			}
			if item.Rule != clean.OpportunityCategoryD3DShaderCache {
				t.Fatalf("history rule = %q", item.Rule)
			}
		}
	}
	if !foundAuthSkip {
		t.Fatalf("history missing auth skip: %#v", recorder.items)
	}
}

func TestD3DExecuteWithAllowPermanentDispatchesSharedPermanentRemover(t *testing.T) {
	root := t.TempDir()
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb")
	d3dRoot := filepath.Join(root, "D3DSCache")
	if err := os.Mkdir(d3dRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d3dRoot, "shader.bin"), []byte("shader"), 0600); err != nil {
		t.Fatal(err)
	}

	collab := &orderedCollaborators{}
	recorder := &recordingHistoryRecorder{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryD3DShaderCache},
		RecycleBinAdapter:      collab,
		PermanentRemover:       collab,
		HistoryRecorder:        recorder,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryD3DShaderCache,
					Path:     d3dRoot,
					Bytes:    6,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
	})

	if len(collab.calls) != 2 {
		t.Fatalf("calls = %#v, want recycle then permanent", collab.calls)
	}
	if collab.calls[0].kind != "recycle" || collab.calls[0].path != recyclePath {
		t.Fatalf("first call = %#v", collab.calls[0])
	}
	if collab.calls[1].kind != "permanent" || collab.calls[1].path != d3dRoot {
		t.Fatalf("second call = %#v, want permanent d3d root", collab.calls[1])
	}

	byRule := map[string]clean.DeletedItem{}
	for _, item := range result.Deleted {
		byRule[item.Rule] = item
	}
	if byRule[clean.DefaultCategoryFoalOwnedTempSandboxes].Action != string(clean.PlannedActionMoveToRecycleBin) {
		t.Fatalf("recycle deleted = %#v", byRule[clean.DefaultCategoryFoalOwnedTempSandboxes])
	}
	d3dDeleted, ok := byRule[clean.OpportunityCategoryD3DShaderCache]
	if !ok || d3dDeleted.Action != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("d3d deleted = %#v", byRule[clean.OpportunityCategoryD3DShaderCache])
	}
	if result.Totals.RecycleBinMovedBytes != 2 {
		t.Fatalf("recycle_bin_moved_bytes = %d", result.Totals.RecycleBinMovedBytes)
	}
	if result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("permanently_deleted_bytes = 0, want d3d content")
	}
	if result.Totals.AffectedBytes != result.Totals.RecycleBinMovedBytes+result.Totals.PermanentlyDeletedBytes {
		t.Fatalf("affected_bytes = %d, want sum of action totals %#v", result.Totals.AffectedBytes, result.Totals)
	}
	if _, err := os.Lstat(d3dRoot); !os.IsNotExist(err) {
		t.Fatalf("d3d root still exists after permanent delete: %v", err)
	}

	foundPermanentHistory := false
	for _, item := range recorder.items {
		if item.Rule == clean.OpportunityCategoryD3DShaderCache && item.Result == "deleted" {
			foundPermanentHistory = true
			if item.Action != string(clean.PlannedActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
		}
	}
	if !foundPermanentHistory {
		t.Fatalf("history missing d3d permanent success: %#v", recorder.items)
	}
	if recorder.sessions[0].Aggregate.PermanentlyDeletedBytes == 0 {
		t.Fatalf("history aggregate permanent bytes = %#v", recorder.sessions[0].Aggregate)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if !strings.Contains(body, `"action":"delete_permanently"`) {
		t.Fatalf("JSON missing actual permanent action: %s", body)
	}
	if !strings.Contains(body, `"permanently_deleted_bytes"`) {
		t.Fatalf("JSON missing permanently_deleted_bytes: %s", body)
	}
}

func TestD3DCapacityCheckExcludesPermanentBytes(t *testing.T) {
	root := t.TempDir()
	// Recycle candidate is 4 bytes; D3D tree is large. Capacity of 10 must accept
	// recycle-only budget and must not sum permanent D3D into the volume budget.
	recyclePath := writeTestFile(t, root, "foal-owned.tmp", "rb12")
	d3dRoot := filepath.Join(root, "D3DSCache")
	if err := os.Mkdir(d3dRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d3dRoot, "big.bin"), make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}

	collab := &orderedCollaborators{}
	result := clean.Execute(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryD3DShaderCache},
		RecycleBinAdapter:      collab,
		PermanentRemover:       collab,
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryD3DShaderCache,
					Path:     d3dRoot,
					Bytes:    100,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
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

func TestD3DPermanentFailureNeverFallsBackToRecycleBin(t *testing.T) {
	root := t.TempDir()
	d3dRoot := filepath.Join(root, "D3DSCache")
	if err := os.Mkdir(d3dRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d3dRoot, "shader.bin"), []byte("shader"), 0600); err != nil {
		t.Fatal(err)
	}

	recycle := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryD3DShaderCache},
		// Disable default sandbox discovery so only the D3D permanent candidate runs.
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: false,
		}},
		RecycleBinAdapter: recycle,
		PermanentRemover: permanentRemoverFunc(func(context.Context, string) error {
			return os.ErrPermission
		}),
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{{
					Category: clean.OpportunityCategoryD3DShaderCache,
					Path:     d3dRoot,
					Bytes:    6,
					Status:   clean.OpportunityStatus,
					Reason:   clean.OpportunityReason,
				}},
			}
		},
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
	if failed.Rule != clean.OpportunityCategoryD3DShaderCache {
		t.Fatalf("failed rule = %q", failed.Rule)
	}
	if failed.Action != string(clean.PlannedActionDeletePermanently) ||
		failed.PlannedAction != string(clean.PlannedActionDeletePermanently) {
		t.Fatalf("failed actions = %#v", failed)
	}
	if failed.Reason.Code != "permanent_delete_failed" {
		t.Fatalf("failed code = %q", failed.Reason.Code)
	}
	if !strings.Contains(failed.Reason.Message, "may already be permanently deleted") {
		t.Fatalf("missing partial-risk warning: %q", failed.Reason.Message)
	}
	// Remover never deleted; path must remain (no Recycle Bin substitute either).
	if _, err := os.Lstat(d3dRoot); err != nil {
		t.Fatalf("d3d root missing after failed permanent: %v", err)
	}
}
