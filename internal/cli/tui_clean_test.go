package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

type recordingHistoryRecorder struct{}

func (*recordingHistoryRecorder) Record(context.Context, history.SessionRecord, []history.ItemRecord) error {
	return nil
}

func stubCleanPreviewDryRun(t *testing.T) {
	t.Helper()
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	originalExecute := executeClean
	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		if opts.HistoryRecorder != nil || opts.DetailedListDir != "" {
			t.Fatalf("clean TUI load options enable side effects: %#v", opts)
		}
		return clean.Result{
			Status: "preview",
			Mode:   "dry_run",
			DefaultRuleCatalog: []clean.RuleSummary{{
				ID:             "foal_owned_temp_sandboxes",
				Description:    "Foal-owned temporary sandbox entries",
				DefaultEnabled: true,
			}},
			Candidates: []clean.CandidatePreview{{
				Path:          `C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
				Bytes:         12,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			}},
			Skipped: []clean.SkippedItem{{
				Path:  `\\?\C:\Windows\System32`,
				Bytes: 4096,
				Rule:  "foal_owned_temp_sandboxes",
				Reason: clean.StructuredIssue{
					Code:        "protected_path",
					Message:     "protected Windows location",
					Recoverable: true,
					Path:        `\\?\C:\Windows\System32`,
					Rule:        "foal_owned_temp_sandboxes",
				},
			}},
			Errors: []clean.StructuredIssue{{
				Code:        "inspection_failed",
				Message:     "could not inspect root",
				Recoverable: true,
				Path:        `C:\Users\corey\AppData\Local\Temp\missing`,
				Rule:        "foal_owned_temp_sandboxes",
			}},
			Opportunities: []clean.UserTempOpportunity{
				{
					Category:         clean.OpportunityCategoryUserTemp,
					Path:             `C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
					Bytes:            4096,
					LatestModifiedAt: time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC),
					IdleDays:         9,
					Status:           clean.UserTempOpportunityStatus,
					Reason:           clean.UserTempOpportunityReason,
				},
				{
					Category: clean.OpportunityCategoryCrashDumps,
					Path:     `C:\Users\corey\AppData\Local\CrashDumps`,
					Bytes:    8192,
					Status:   clean.UserTempOpportunityStatus,
					Reason:   clean.UserTempOpportunityReason,
				},
				{
					Category: clean.OpportunityCategoryWindowsErrorReporting,
					Path:     `C:\Users\corey\AppData\Local\Microsoft\Windows\WER`,
					Bytes:    1024,
					Status:   clean.UserTempOpportunityStatus,
					Reason:   clean.UserTempOpportunityReason,
				},
				{
					Category: clean.OpportunityCategoryExplorerThumbnailCache,
					Path:     `C:\Users\corey\AppData\Local\Microsoft\Windows\Explorer`,
					Bytes:    2048,
					Status:   clean.UserTempOpportunityStatus,
					Reason:   clean.UserTempOpportunityReason,
				},
				{
					Category: clean.OpportunityCategoryINetCache,
					Path:     `C:\Users\corey\AppData\Local\Microsoft\Windows\INetCache`,
					Bytes:    4096,
					Status:   clean.UserTempOpportunityStatus,
					Reason:   clean.UserTempOpportunityReason,
				},
				{
					Category: clean.OpportunityCategoryD3DShaderCache,
					Path:     `C:\Users\corey\AppData\Local\D3DSCache`,
					Bytes:    2048,
					Status:   clean.UserTempOpportunityStatus,
					Reason:   clean.UserTempOpportunityReason,
				},
				{
					Category: clean.OpportunityCategoryNVIDIADXCache,
					Path:     `C:\Users\corey\AppData\Local\NVIDIA\DXCache`,
					Bytes:    4096,
					Status:   clean.UserTempOpportunityStatus,
					Reason:   clean.UserTempOpportunityReason,
				},
			},
			ReviewSuggestions: []clean.ReviewSuggestion{
				{
					Tool:      "pnpm",
					Label:     "pnpm cache",
					Command:   "pnpm store prune",
					CachePath: `C:\Users\corey\AppData\Local\pnpm\store\v10`,
				},
				{
					Tool:      "yarn",
					Label:     "yarn cache",
					Command:   "yarn cache clean",
					CachePath: `C:\Users\corey\AppData\Local\Yarn\Cache\v6`,
				},
				{
					Tool:      "bun",
					Label:     "bun cache",
					Command:   "bun pm cache rm",
					CachePath: `C:\Users\corey\.bun\install\cache`,
				},
				{
					Tool:      "corepack",
					Label:     "Corepack cache",
					Command:   "corepack cache clean",
					CachePath: `C:\Users\corey\AppData\Local\node\corepack\v1`,
				},
				{
					Tool:      "mise",
					Label:     "mise cache",
					Command:   "mise cache clear",
					CachePath: `C:\Users\corey\AppData\Local\mise`,
				},
			},
			Totals: clean.Totals{
				CandidateCount:           1,
				CandidateBytes:           12,
				SkippedCount:             1,
				OpportunityCount:         7,
				OpportunityObservedBytes: 25600,
			},
		}
	}
	executeClean = func(ctx context.Context, opts clean.Options) clean.Result {
		t.Fatal("clean TUI preview must not execute cleanup")
		return clean.Result{}
	}
	t.Cleanup(func() {
		dryRunClean = originalDryRun
		executeClean = originalExecute
	})
}

func TestCleanCategorySelectionRefreshesPreviewWithCanonicalIDs(t *testing.T) {
	originalDryRun := dryRunClean
	var received [][]string
	dryRunClean = func(_ context.Context, opts clean.Options) clean.Result {
		received = append(received, append([]string(nil), opts.OptIn...))
		result := clean.Result{}
		if len(opts.OptIn) == 1 {
			result.OptInCandidates = []clean.OptInCandidate{{Category: opts.OptIn[0], Path: `C:\private\candidate`, Bytes: 42}}
			result.Totals.OptInReclaimableBytes = 42
		}
		return result
	}
	t.Cleanup(func() { dryRunClean = originalDryRun })

	model := newCleanModel(120, 40)
	model.loadGeneration = 1
	model.applyLoaded(cleanPreviewLoadedMsg{generation: 1, model: clean.NewPreviewReadModelForSelection(clean.Result{}, nil)})
	if len(model.selectedCategoryIDs()) != 0 {
		t.Fatal("new Clean TUI session must start with empty opt-in selection")
	}

	cmd := model.handleKey(" ")
	if cmd == nil || !model.loading {
		t.Fatal("toggle must immediately stale the prior preview and start loading")
	}
	msg := cmd().(cleanPreviewLoadedMsg)
	if len(received) != 1 || len(received[0]) != 1 || received[0][0] != clean.OpportunityCategoryCrashDumps {
		t.Fatalf("DryRun OptIn = %#v; want canonical crash_dumps only", received)
	}
	model.applyLoaded(msg)
	if model.loading || model.model.OptInReclaimableBytes != 42 || !model.model.OptInCategories[0].Selected {
		t.Fatalf("selected preview was not applied: %#v", model.model)
	}
	if strings.Contains(strings.Join(model.selectedCategoryIDs(), "\n"), `C:\private`) {
		t.Fatal("selection state must never contain candidate paths")
	}

	cmd = model.handleKey(" ")
	msg = cmd().(cleanPreviewLoadedMsg)
	if len(received[1]) != 0 {
		t.Fatalf("deselect DryRun OptIn = %#v; want empty", received[1])
	}
	model.applyLoaded(msg)
}

func TestCleanCategorySelectAllClearAllAndStaleGeneration(t *testing.T) {
	originalDryRun := dryRunClean
	dryRunClean = func(_ context.Context, opts clean.Options) clean.Result { return clean.Result{} }
	t.Cleanup(func() { dryRunClean = originalDryRun })

	model := newCleanModel(120, 40)
	model.loadGeneration = 1
	model.applyLoaded(cleanPreviewLoadedMsg{generation: 1, model: clean.NewPreviewReadModelForSelection(clean.Result{}, nil)})
	selectAll := model.handleKey("a")
	allMsg := selectAll().(cleanPreviewLoadedMsg)
	if got, want := len(model.selected), len(model.model.OptInCategories); got != want || got == 0 {
		t.Fatalf("select all selected %d of %d categories", got, want)
	}
	clearWhileLoading := model.handleKey("x")
	if clearWhileLoading == nil || len(model.selected) != 0 || model.loadGeneration != allMsg.generation+1 {
		t.Fatal("selection change during loading must supersede the in-flight preview")
	}
	clearMsg := clearWhileLoading().(cleanPreviewLoadedMsg)

	model.applyLoaded(cleanPreviewLoadedMsg{generation: allMsg.generation, model: clean.PreviewReadModel{OptInReclaimableBytes: 999}})
	if !model.loading || model.model.OptInReclaimableBytes == 999 {
		t.Fatal("late superseded generation must be discarded")
	}
	model.applyLoaded(clearMsg)
}

func TestCleanCanceledAndFailedSelectionPreviewRemainNotReady(t *testing.T) {
	originalDryRun := dryRunClean
	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		if opts.DetailedListDir != "" || opts.HistoryRecorder != nil {
			t.Fatal("TUI selection preview must not write detailed lists or history")
		}
		<-ctx.Done()
		return clean.Result{Status: "error"}
	}
	t.Cleanup(func() { dryRunClean = originalDryRun })

	model := newCleanModel(120, 40)
	model.loadGeneration = 1
	model.applyLoaded(cleanPreviewLoadedMsg{generation: 1, model: clean.NewPreviewReadModelForSelection(clean.Result{}, nil)})
	cmd := model.handleKey("a")
	model.cancelPendingLoad()
	msg := cmd().(cleanPreviewLoadedMsg)
	model.applyLoaded(msg)
	if model.previewReady || model.loading || !strings.Contains(model.content(), "not ready") {
		t.Fatalf("canceled preview became ready:\n%s", model.content())
	}

	model.loadGeneration++
	model.applyLoaded(cleanPreviewLoadedMsg{generation: model.loadGeneration, failed: true, model: clean.PreviewReadModel{OptInReclaimableBytes: 999}})
	if model.previewReady || !strings.Contains(model.content(), "failed") || strings.Contains(model.content(), "999 bytes") {
		t.Fatalf("failed preview became ready:\n%s", model.content())
	}
}

func TestCleanCategoryKeyboardActionsRenderPreviewOnlySelection(t *testing.T) {
	model := newCleanModel(180, 60)
	model.loadGeneration = 1
	model.applyLoaded(cleanPreviewLoadedMsg{generation: 1, model: clean.NewPreviewReadModelForSelection(clean.Result{}, nil)})
	content := model.content()
	if !strings.Contains(content, "[ ] Crash dumps (review-only") || !strings.Contains(content, "tab category | space toggle | a select all | x clear all") {
		t.Fatalf("empty selection actions or review-only wording missing:\n%s", content)
	}
	model.handleKey("tab")
	if !strings.Contains(model.content(), "Category focus: Windows Error Reporting") {
		t.Fatalf("tab did not move category focus:\n%s", model.content())
	}
	model.model = clean.NewPreviewReadModelForSelection(clean.Result{
		OptInCandidates: []clean.OptInCandidate{{Category: clean.OpportunityCategoryWindowsErrorReporting, Bytes: 21}},
		Totals:          clean.Totals{OptInReclaimableBytes: 21},
	}, []string{clean.OpportunityCategoryWindowsErrorReporting})
	model.refreshViewportContent()
	if !strings.Contains(model.content(), "[x] Windows Error Reporting (selected preview, 1 candidate(s), 21 bytes opt-in reclaimable)") ||
		strings.Contains(model.content(), "confirmation") {
		t.Fatalf("selected preview wording crossed the confirmation boundary:\n%s", model.content())
	}
}

func openCleanPreview(t *testing.T) rootModel {
	t.Helper()
	model := newRootModel()
	// Wide window so long candidate paths are not clipped by the viewport.
	next, _ := model.Update(tea.WindowSizeMsg{Width: 240, Height: 80})
	model = next.(rootModel)
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("opening the clean view must return a load command")
	}
	if !strings.Contains(model.content(), "Loading clean preview (dry-run)...") {
		t.Fatalf("clean view should show a loading state before data arrives:\n%s", model.content())
	}
	loaded, ok := cmd().(cleanPreviewLoadedMsg)
	if !ok {
		t.Fatalf("load command produced %T, want cleanPreviewLoadedMsg", cmd())
	}
	next, _ = model.Update(loaded)
	return next.(rootModel)
}

func TestCleanSelectionRendersReadOnlyPreview(t *testing.T) {
	stubCleanPreviewDryRun(t)

	model := openCleanPreview(t)

	content := model.content()
	for _, want := range []string{
		"Foal Clean",
		"Preview only - no files changed.",
		"Potential space: 12 bytes",
		"Dry-run complete",
		"No files changed.",
		"Default candidates (1)",
		"[candidate] foal-default.tmp (12 bytes)",
		"Skipped items (1)",
		"[boundary] System32",
		"protected_path",
		"Inspection errors (1)",
		"inspection_failed",
		"SoftwareDistribution",
		"Delivery Optimization",
		"excluded from Opportunity discovery",
		"will not request elevation automatically",
		"Protection rules",
		"Observed opportunity bytes: 25600 bytes (not counted as Potential space)",
		"[opportunity] old-tool-cache",
		"category: user_temp",
		"latest modified: 2026-06-01T12:00:00Z",
		"idle days: 9",
		"[opportunity] CrashDumps",
		"category: crash_dumps",
		"[opportunity] WER",
		"category: windows_error_reporting",
		"[opportunity] Explorer",
		"category: explorer_thumbnail_cache",
		"[opportunity] INetCache",
		"category: inet_cache",
		"[opportunity] D3DSCache",
		"category: d3d_shader_cache",
		"[opportunity] DXCache",
		"category: nvidia_dx_cache",
		"status: skipped_by_default",
		"reason: requires_explicit_opt_in",
		"Review suggestions (5)",
		"pnpm cache",
		"yarn cache",
		"bun cache",
		"Corepack cache",
		"mise cache",
		"Press c to copy candidate paths to the clipboard.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{
		"Clean preview TUI",
		"Read-only review over foal clean --dry-run",
		"Foal main menu",
		"Cleanup complete",
		"Execution complete",
		"Deleted:",
		"Execute",
		"execute cleanup",
		"Potential space: 4108 bytes",
		"Detailed candidate list:",
		"Run as Administrator",
		"0001-01-01",
		"CrashDumps (8192 bytes, category: crash_dumps, latest modified:",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("content contains forbidden execution or potential-space wording %q:\n%s", forbidden, content)
		}
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'e', Text: "e"})
	for _, want := range []string{
		`C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
		`\\?\C:\Windows\System32`,
		`C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
		`C:\Users\corey\AppData\Local\CrashDumps`,
		`C:\Users\corey\AppData\Local\Microsoft\Windows\WER`,
		`C:\Users\corey\AppData\Local\Microsoft\Windows\Explorer`,
		`C:\Users\corey\AppData\Local\Microsoft\Windows\INetCache`,
		`C:\Users\corey\AppData\Local\D3DSCache`,
		`C:\Users\corey\AppData\Local\NVIDIA\DXCache`,
		"Command: pnpm store prune",
		`Cache: C:\Users\corey\AppData\Local\pnpm\store\v10`,
		"Command: yarn cache clean",
		`Cache: C:\Users\corey\AppData\Local\Yarn\Cache\v6`,
		"Command: bun pm cache rm",
		`Cache: C:\Users\corey\.bun\install\cache`,
		"Command: corepack cache clean",
		`Cache: C:\Users\corey\AppData\Local\node\corepack\v1`,
		"Command: mise cache clear",
		`Cache: C:\Users\corey\AppData\Local\mise`,
	} {
		if !strings.Contains(model.content(), want) {
			t.Fatalf("expanded content missing %q:\n%s", want, model.content())
		}
	}
}

func TestCleanPreviewTUIUsesCompactHeaderAndBottomSummary(t *testing.T) {
	stubCleanPreviewDryRun(t)

	model := openCleanPreview(t)
	content := model.content()
	header := model.clean.headerContent()

	for _, want := range []string{
		"Foal Clean",
		"Preview only - no files changed.",
		"Filter: all",
	} {
		if !strings.Contains(header, want) {
			t.Fatalf("header missing %q:\n%s", want, header)
		}
	}
	for _, forbidden := range []string{
		"Foal main menu",
		"Potential space:",
		"Review-only opportunities:",
		"Candidates:",
		"Dry-run complete",
		"Cleanup complete",
	} {
		if strings.Contains(header, forbidden) {
			t.Fatalf("header contains non-compact summary or menu wording %q:\n%s", forbidden, header)
		}
	}

	summaryIndex := strings.LastIndex(content, "Summary")
	if summaryIndex == -1 {
		t.Fatalf("content missing Summary category:\n%s", content)
	}
	for _, want := range []string{
		"Dry-run complete",
		"No files changed.",
		"Potential space: 12 bytes",
		"Observed opportunity bytes: 25600 bytes (not counted as Potential space)",
		"Default candidates: 1 | Skipped: 1 | Diagnostics: 1",
	} {
		index := strings.LastIndex(content, want)
		if index <= summaryIndex {
			t.Fatalf("%q should render in bottom Summary:\n%s", want, content)
		}
	}
}

func TestCleanPreviewTUIRendersStatusMarkersAndCompactLabels(t *testing.T) {
	model := clean.PreviewReadModel{
		Candidates: []clean.PreviewCandidate{{
			Path:          `C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
			Bytes:         12,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		}},
		Skipped: []clean.PreviewSkippedItem{{
			Path: `\\?\C:\Windows\System32`,
			Reason: clean.StructuredIssue{
				Code:        "protected_path",
				Message:     "protected Windows location",
				Recoverable: true,
			},
		}},
		Opportunities: []clean.Opportunity{{
			Category:         clean.OpportunityCategoryUserTemp,
			Path:             `C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
			Bytes:            4096,
			LatestModifiedAt: time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC),
			IdleDays:         9,
			Status:           clean.OpportunityStatus,
			Reason:           clean.OpportunityReason,
		}},
		ReviewSuggestions: []clean.PreviewReviewSuggestion{{
			Label:     "pnpm cache",
			Command:   "pnpm store prune",
			CachePath: `C:\Users\corey\AppData\Local\pnpm\store\v10`,
		}},
		Errors: []clean.StructuredIssue{{
			Code:        "inspection_failed",
			Path:        `C:\Users\corey\AppData\Local\Temp\missing`,
			Recoverable: true,
		}},
		PotentialSpaceBytes:      12,
		CandidateCount:           1,
		SkippedCount:             1,
		OpportunityCount:         1,
		OpportunityObservedBytes: 4096,
	}

	compact := renderCleanPreviewSections(model, cleanPreviewFilterAll, false)
	for _, want := range []string{
		"[candidate] foal-default.tmp (12 bytes)",
		"[boundary] System32",
		"[opportunity] old-tool-cache (4096 bytes, category: user_temp",
		"[review] pnpm cache (Review suggestion)",
		"[diagnostic] missing (rule: , error: inspection_failed, recoverable: true)",
		"[loaded] Protection rules not reported.",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("compact output missing %q:\n%s", want, compact)
		}
	}
	for _, forbidden := range []string{
		`C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
		`C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
		"safe to delete",
		"authorized",
		"Cleanup complete",
		"Deleted:",
	} {
		if strings.Contains(compact, forbidden) {
			t.Fatalf("compact output contains forbidden detail or wording %q:\n%s", forbidden, compact)
		}
	}

	expanded := renderCleanPreviewSections(model, cleanPreviewFilterAll, true)
	for _, want := range []string{
		`C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
		"rule: foal_owned_temp_sandboxes",
		"planned action: Recycle Bin",
		`C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
		"status: skipped_by_default",
		"reason: requires_explicit_opt_in",
		"Command: pnpm store prune",
		`Cache: C:\Users\corey\AppData\Local\pnpm\store\v10`,
	} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded output missing %q:\n%s", want, expanded)
		}
	}

	emptyLoaded := renderCleanPreviewSections(clean.PreviewReadModel{}, cleanPreviewFilterAll, false)
	for _, want := range []string{
		"[loaded] No default candidates found.",
		"[loaded] No skipped cleanup paths reported.",
		"[loaded] Protection rules not reported.",
		"[loaded] No recoverable inspection errors reported.",
	} {
		if !strings.Contains(emptyLoaded, want) {
			t.Fatalf("empty loaded output missing %q:\n%s", want, emptyLoaded)
		}
	}
}

func TestCleanPreviewRendersUserDefinedProtectionRuleFromReadModel(t *testing.T) {
	path := `C:\Users\corey\Work\Valuable`
	output := renderCleanPreviewSections(clean.PreviewReadModel{
		ProtectionRules: []clean.PreviewProtectionRule{{
			Path:        path,
			UserDefined: true,
		}},
	}, cleanPreviewFilterAll, true)

	if !strings.Contains(output, path) || !strings.Contains(output, "user-defined Protection rule") {
		t.Fatalf("output missing active user protection rule:\n%s", output)
	}
}

func TestCleanPreviewDoesNotRenderSuppressedReviewOnlyPaths(t *testing.T) {
	protectedRoot := `C:\Users\corey\AppData\Local\Protected`
	protectedOpportunity := protectedRoot + `\private-opportunity`
	protectedSuggestion := protectedRoot + `\private-suggestion`
	protectedIncomplete := protectedRoot + `\private-incomplete`
	visibleOpportunity := `C:\Users\corey\AppData\Local\ProtectedSibling\visible-opportunity`
	visibleSuggestion := `C:\Users\corey\AppData\Local\ProtectedSibling\visible-suggestion`
	visibleIncomplete := `C:\Users\corey\AppData\Local\ProtectedSibling\visible-incomplete`

	result := clean.DryRun(context.Background(), clean.Options{
		Validator: pathsafe.NewValidator([]string{protectedRoot}),
		DiscoverUserTempOpportunities: func(context.Context) clean.UserTempDiscoveryResult {
			return clean.UserTempDiscoveryResult{
				Opportunities: []clean.UserTempOpportunity{
					{Path: protectedOpportunity, Bytes: 10, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
					{Path: visibleOpportunity, Bytes: 20, Status: clean.UserTempOpportunityStatus, Reason: clean.UserTempOpportunityReason},
				},
				Incomplete: []clean.IncompleteOpportunityInspection{
					{
						Path: protectedIncomplete,
						Reason: clean.StructuredIssue{
							Code:        "inspection_failed",
							Message:     "protected inspection failed",
							Recoverable: true,
							Path:        protectedIncomplete,
						},
					},
					{
						Path: visibleIncomplete,
						Reason: clean.StructuredIssue{
							Code:        "inspection_failed",
							Message:     "visible inspection failed",
							Recoverable: true,
							Path:        visibleIncomplete,
						},
					},
				},
			}
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return []clean.ReviewSuggestion{
				{Label: "private cache", Command: "private clean", CachePath: protectedSuggestion},
				{Label: "visible cache", Command: "visible clean", CachePath: visibleSuggestion},
			}
		},
		Rules: []clean.Rule{{ID: "disabled_test_rule", DefaultEnabled: false}},
	})
	output := renderCleanPreviewSections(clean.NewPreviewReadModel(result), cleanPreviewFilterAll, true)

	for _, suppressed := range []string{protectedOpportunity, protectedSuggestion, protectedIncomplete} {
		if strings.Contains(output, suppressed) {
			t.Fatalf("TUI leaked suppressed review-only path %q:\n%s", suppressed, output)
		}
	}
	for _, visible := range []string{visibleOpportunity, visibleSuggestion, visibleIncomplete} {
		if !strings.Contains(output, visible) {
			t.Fatalf("TUI missing unprotected review-only path %q:\n%s", visible, output)
		}
	}
}

func TestCleanPreviewRendersProtectionDiagnosticsFromReadModel(t *testing.T) {
	source := `C:\Users\corey\AppData\Roaming\Foal\protection.txt`
	output := renderCleanPreviewSections(clean.PreviewReadModel{
		ProtectionDiagnostics: []clean.ProtectionDiagnostic{{
			Code:        "short_name_path",
			Message:     "8.3 short-name paths cannot be used as Protection rules",
			Recoverable: true,
			Source:      source,
			Line:        3,
		}},
	}, cleanPreviewFilterAll, true)

	for _, want := range []string{"Protection diagnostics", "short_name_path", source, "line 3"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestCleanPreviewBackReturnsToMenu(t *testing.T) {
	stubCleanPreviewDryRun(t)

	model := openCleanPreview(t)
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})

	if !strings.Contains(model.content(), "Foal main menu") {
		t.Fatalf("b should return to the main menu:\n%s", model.content())
	}
}

func TestCleanPreviewBackCancelsInFlightLoad(t *testing.T) {
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	started := make(chan struct{})
	canceled := make(chan struct{})
	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		close(started)
		<-ctx.Done()
		close(canceled)
		return clean.Result{}
	}
	t.Cleanup(func() { dryRunClean = originalDryRun })

	model := newRootModel()
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("opening the clean view must start a load")
	}
	done := make(chan struct{})
	go func() {
		_ = cmd()
		close(done)
	}()
	<-started

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("leaving the clean preview did not cancel the in-flight load")
	}
	<-done
	if !strings.Contains(model.content(), "Foal main menu") {
		t.Fatalf("b should return to the main menu:\n%s", model.content())
	}
}

func TestCleanPreviewReloadCancelsPreviousAndRemainsCancellable(t *testing.T) {
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	contexts := make(chan context.Context, 2)
	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		contexts <- ctx
		<-ctx.Done()
		return clean.Result{}
	}
	t.Cleanup(func() { dryRunClean = originalDryRun })

	model := newRootModel()
	next, firstCmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	firstDone := make(chan struct{})
	go func() {
		_ = firstCmd()
		close(firstDone)
	}()
	firstCtx := <-contexts

	next, secondCmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)
	select {
	case <-firstCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("reload did not cancel the previous clean preview load")
	}
	<-firstDone

	secondDone := make(chan struct{})
	go func() {
		_ = secondCmd()
		close(secondDone)
	}()
	secondCtx := <-contexts
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	select {
	case <-secondCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("reloaded clean preview was not cancellable")
	}
	<-secondDone
}

func TestCleanPreviewBrowsingRecordsNoHistoryAndWritesNoFiles(t *testing.T) {
	stubCleanPreviewDryRun(t)
	originalRecorder := newHistoryRecorder
	originalDir := newHistoryDir
	newHistoryRecorder = func() (history.Recorder, error) {
		t.Fatal("browsing the clean preview TUI must not open a history recorder")
		return nil, nil
	}
	newHistoryDir = func() (string, error) {
		t.Fatal("browsing the clean preview TUI must not resolve the history directory")
		return "", nil
	}
	t.Cleanup(func() {
		newHistoryRecorder = originalRecorder
		newHistoryDir = originalDir
	})

	model := openCleanPreview(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("reload must return a load command")
	}
	msg := cmd()
	loaded, ok := msg.(cleanPreviewLoadedMsg)
	if !ok {
		t.Fatalf("reload command produced %T, want cleanPreviewLoadedMsg", msg)
	}
	next, _ = model.Update(loaded)
	_ = next.(rootModel)
}

func TestCleanPreviewCopyKeySendsCandidatePathsToClipboard(t *testing.T) {
	stubCleanPreviewDryRun(t)
	originalCopy := copyTextToClipboard
	copied := ""
	copyTextToClipboard = func(text string) error {
		copied = text
		return nil
	}
	t.Cleanup(func() { copyTextToClipboard = originalCopy })

	model := openCleanPreview(t)
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})

	if want := `C:\Users\corey\AppData\Local\Temp\foal-default.tmp` + "\n"; copied != want {
		t.Fatalf("clipboard payload = %q, want %q", copied, want)
	}
	if strings.Contains(copied, "old-tool-cache") {
		t.Fatalf("clipboard payload includes review-only opportunity path: %q", copied)
	}
	if !strings.Contains(model.content(), "Copied 1 candidate path(s) to the clipboard.") {
		t.Fatalf("content missing copy confirmation:\n%s", model.content())
	}
}

func TestCleanPreviewCopyPayloadExcludesAllReviewOnlySurfaces(t *testing.T) {
	originalCopy := copyTextToClipboard
	copied := ""
	copyTextToClipboard = func(text string) error {
		copied = text
		return nil
	}
	t.Cleanup(func() { copyTextToClipboard = originalCopy })

	protectedRoot := `C:\Users\corey\AppData\Local\Protected`
	protectedOpportunity := protectedRoot + `\private-opportunity`
	protectedSuggestion := protectedRoot + `\private-suggestion`
	protectedIncomplete := protectedRoot + `\private-incomplete`
	candidatePath := filepath.Join(t.TempDir(), "foal-default.tmp")
	if err := os.WriteFile(candidatePath, []byte("default candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	reviewCluePath := `D:\Code\Personal\Foal\node_modules`
	browserUserDataPath := `C:\Users\corey\AppData\Local\Google\Chrome\User Data`
	browserProfilePath := browserUserDataPath + `\Default`
	browserCachePath := browserProfilePath + `\Code Cache`
	result := clean.DryRun(context.Background(), clean.Options{
		Validator: pathsafe.NewValidator([]string{protectedRoot}),
		DiscoverOpportunities: func(context.Context) clean.OpportunityDiscoveryResult {
			return clean.OpportunityDiscoveryResult{
				Opportunities: []clean.Opportunity{
					{
						Category: clean.OpportunityCategoryUserTemp,
						Path:     protectedOpportunity,
						Bytes:    10,
						Status:   clean.OpportunityStatus,
						Reason:   clean.OpportunityReason,
					},
					{
						Category: clean.OpportunityCategoryUserTemp,
						Path:     `C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
						Bytes:    20,
						Status:   clean.OpportunityStatus,
						Reason:   clean.OpportunityReason,
					},
					{
						Category: clean.OpportunityCategoryBrowserCache,
						Path:     browserUserDataPath,
						Bytes:    30,
						Status:   clean.OpportunityStatus,
						Reason:   clean.OpportunityReason,
						BrowserCache: &clean.BrowserCacheOpportunityDetail{
							Browser:      clean.ApplicationGoogleChrome,
							ProfileCount: 1,
							Profiles: []clean.BrowserCacheProfileDetail{{
								ID:   "Default",
								Path: browserProfilePath,
								Caches: []clean.BrowserCacheDirectory{{
									Kind:  "Code Cache",
									Path:  browserCachePath,
									Bytes: 30,
								}},
							}},
						},
					},
				},
				Incomplete: []clean.IncompleteOpportunityInspection{
					{
						Category: clean.OpportunityCategoryUserTemp,
						Path:     protectedIncomplete,
						Reason: clean.StructuredIssue{
							Code:        "inspection_failed",
							Message:     "protected inspection failed",
							Recoverable: true,
							Path:        protectedIncomplete,
						},
					},
					{
						Category: clean.OpportunityCategoryUserTemp,
						Path:     `C:\Users\corey\AppData\Local\Temp\partial-cache`,
						Reason: clean.StructuredIssue{
							Code:        "inspection_limit",
							Message:     "partial inspection",
							Recoverable: true,
							Path:        `C:\Users\corey\AppData\Local\Temp\partial-cache`,
						},
					},
				},
			}
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			return []clean.ReviewSuggestion{
				{Label: "private cache", Command: "private clean", CachePath: protectedSuggestion},
				{Label: "pnpm cache", Command: "pnpm store prune", CachePath: `C:\Users\corey\AppData\Local\pnpm\store\v10`},
			}
		},
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			DefaultEnabled: true,
			CandidatePaths: []string{candidatePath},
		}},
	})
	readModel := clean.NewPreviewReadModel(result)
	readModel.ReviewClues = append(readModel.ReviewClues, clean.PreviewReviewClue{
		Name:    "Project artifact clue",
		Path:    reviewCluePath,
		Details: "review manually before deleting",
	})
	model := cleanModel{
		filter:  cleanPreviewFilterReviewOnly,
		vp:      viewport.New(viewport.WithWidth(120), viewport.WithHeight(40)),
		width:   120,
		height:  48,
		loading: false,
		model:   readModel,
	}

	model.handleKey("c")

	if want := candidatePath + "\n"; copied != want {
		t.Fatalf("clipboard payload = %q, want only default candidate path %q", copied, want)
	}
	for _, forbidden := range []string{
		"old-tool-cache",
		"partial-cache",
		"pnpm",
		"node_modules",
		browserUserDataPath,
		browserProfilePath,
		browserCachePath,
		protectedOpportunity,
		protectedSuggestion,
		protectedIncomplete,
		reviewCluePath,
	} {
		if strings.Contains(copied, forbidden) {
			t.Fatalf("clipboard payload includes review-only or protected data %q: %q", forbidden, copied)
		}
	}
}

func TestCleanPreviewCopyKeyReportsClipboardFailure(t *testing.T) {
	stubCleanPreviewDryRun(t)
	originalCopy := copyTextToClipboard
	copyTextToClipboard = func(text string) error {
		return errors.New("clip unavailable")
	}
	t.Cleanup(func() { copyTextToClipboard = originalCopy })

	model := openCleanPreview(t)
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})

	if !strings.Contains(model.content(), "Clipboard copy failed: clip unavailable") {
		t.Fatalf("content missing clipboard failure notice:\n%s", model.content())
	}
}

func TestCleanPreviewRefreshKeyReloadsThroughDryRun(t *testing.T) {
	stubCleanPreviewDryRun(t)

	model := openCleanPreview(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)

	if cmd == nil {
		t.Fatal("r must return a reload command")
	}
	if !strings.Contains(model.content(), "Loading clean preview (dry-run)...") {
		t.Fatalf("refresh should re-enter the loading state:\n%s", model.content())
	}
	if _, ok := cmd().(cleanPreviewLoadedMsg); !ok {
		t.Fatal("reload command must produce cleanPreviewLoadedMsg")
	}
}

func TestCleanPreviewReloadLoadsProtectionConfigurationAfresh(t *testing.T) {
	originalLoader := loadProtectionConfiguration
	originalDryRun := dryRunClean
	loadCount := 0
	loadProtectionConfiguration = func() clean.ProtectionConfiguration {
		loadCount++
		path := `C:\Work\First`
		if loadCount == 2 {
			path = `C:\Work\Second`
		}
		return clean.ProtectionConfiguration{Validator: pathsafe.NewValidator([]string{path})}
	}
	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		return clean.Result{
			Status:          "preview",
			Mode:            "dry_run",
			ProtectionRules: []clean.ProtectionRule{{Path: opts.Validator.UserProtectionPaths()[0]}},
		}
	}
	t.Cleanup(func() {
		loadProtectionConfiguration = originalLoader
		dryRunClean = originalDryRun
	})

	first := loadCleanPreviewCmd(context.Background(), 1)().(cleanPreviewLoadedMsg)
	second := loadCleanPreviewCmd(context.Background(), 2)().(cleanPreviewLoadedMsg)

	if loadCount != 2 {
		t.Fatalf("load count = %d, want one fresh load per TUI command", loadCount)
	}
	if first.model.ProtectionRules[0].Path != `C:\Work\First` ||
		second.model.ProtectionRules[0].Path != `C:\Work\Second` {
		t.Fatalf("rules = %#v then %#v, want refreshed configuration", first.model.ProtectionRules, second.model.ProtectionRules)
	}
}

func TestCleanPreviewLoadAndReloadRenderBrowserRunningStateFromSharedReadModel(t *testing.T) {
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	loadCount := 0
	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
		if opts.DetectRunningApplications == nil {
			t.Fatal("clean TUI load must use shared browser running application detection")
		}
		loadCount++
		if loadCount == 1 {
			return clean.Result{
				Status: "preview",
				Mode:   "dry_run",
				RunningApplications: []clean.RunningApplicationState{{
					Application: clean.ApplicationGoogleChrome,
					State:       clean.RunningApplicationStateRunning,
				}},
			}
		}
		return clean.Result{
			Status: "preview",
			Mode:   "dry_run",
			RunningApplications: []clean.RunningApplicationState{{
				Application: clean.ApplicationMicrosoftEdge,
				State:       clean.RunningApplicationStateUnknown,
				Message:     "process snapshot failed",
			}},
			Errors: []clean.StructuredIssue{{
				Code:        "running_application_detection_unknown",
				Message:     "process snapshot failed",
				Recoverable: true,
				Rule:        "browser_review",
			}},
		}
	}
	t.Cleanup(func() { dryRunClean = originalDryRun })

	first := loadCleanPreviewCmd(context.Background(), 1)().(cleanPreviewLoadedMsg)
	firstOutput := renderCleanPreviewSections(first.model, cleanPreviewFilterReviewOnly, true)
	if !strings.Contains(firstOutput, "Running application skips (1)") ||
		!strings.Contains(firstOutput, "Google Chrome") ||
		!strings.Contains(firstOutput, "browser cache review was skipped") {
		t.Fatalf("first load missing running browser skip:\n%s", firstOutput)
	}

	second := loadCleanPreviewCmd(context.Background(), 2)().(cleanPreviewLoadedMsg)
	secondOutput := renderCleanPreviewSections(second.model, cleanPreviewFilterDiagnostics, true)
	if !strings.Contains(secondOutput, "running_application_detection_unknown") ||
		!strings.Contains(secondOutput, "process snapshot failed") {
		t.Fatalf("reload missing unknown browser diagnostic:\n%s", secondOutput)
	}
}

func TestCleanReadyPreviewRequiresSeparateConfirmationBeforeExecution(t *testing.T) {
	stubCleanPreviewDryRun(t)
	calls := 0
	original := executeClean
	executeClean = func(_ context.Context, opts clean.Options) clean.Result {
		calls++
		if got := strings.Join(opts.OptIn, ","); got != clean.OpportunityCategoryCrashDumps {
			t.Fatalf("execute OptIn = %q, want canonical category identifier", got)
		}
		if opts.DetailedListDir != "" {
			t.Fatal("TUI execute must not pass a detailed-list directory")
		}
		return clean.Result{Status: "ok", Mode: "execute", Totals: clean.Totals{DeletedCount: 2, OptInDeletedCount: 1, AffectedBytes: 30, OptInAffectedBytes: 20}}
	}
	t.Cleanup(func() { executeClean = original })

	model := openCleanPreview(t)
	model.clean.selected[clean.OpportunityCategoryCrashDumps] = true
	model.clean.model = clean.NewPreviewReadModelForSelection(clean.Result{
		Candidates:      []clean.CandidatePreview{{Bytes: 10}},
		OptInCandidates: []clean.OptInCandidate{{Category: clean.OpportunityCategoryCrashDumps, Bytes: 20}},
		Totals:          clean.Totals{CandidateCount: 1, CandidateBytes: 10, OptInCandidateCount: 1, OptInReclaimableBytes: 20},
	}, []string{clean.OpportunityCategoryCrashDumps})
	model.clean.previewReady = true

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd != nil || calls != 0 || !strings.Contains(model.content(), "Confirm Clean execution") || !strings.Contains(model.content(), "Crash dumps") || !strings.Contains(model.content(), "rescans") {
		t.Fatalf("enter must open confirmation without executing:\n%s", model.content())
	}
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil || calls != 0 || !strings.Contains(model.content(), "Executing Clean") {
		t.Fatalf("confirmation must enter visible executing state:\n%s", model.content())
	}
	// A second Enter while in flight cannot start another execution.
	next, second := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if second != nil || calls != 0 {
		t.Fatal("executing state accepted a second confirmation")
	}
	next, _ = model.Update(cmd())
	model = next.(rootModel)
	if calls != 1 || !strings.Contains(model.content(), "Clean execution result") || !strings.Contains(model.content(), "Deleted: 2") || !strings.Contains(model.content(), "Opt-in deleted: 1") || !strings.Contains(model.content(), "Affected bytes: 30 bytes") {
		t.Fatalf("shared result was not rendered directly:\n%s", model.content())
	}
}

func TestCleanConfirmationBackAndEscapeCancelWithoutExecution(t *testing.T) {
	stubCleanPreviewDryRun(t)
	model := openCleanPreview(t)
	model.clean.previewReady = true
	model, _ = updateRootKeyWithCmd(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if model.screen != screenCleanPreview || !model.clean.previewReady || strings.Contains(model.content(), "Confirm Clean execution") {
		t.Fatal("back must return to ready preview")
	}
	model, _ = updateRootKeyWithCmd(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.screen != screenCleanPreview || !model.clean.previewReady {
		t.Fatal("escape must cancel confirmation without quitting")
	}
}

func TestCleanExecutionHandoffLoadsFreshBoundariesAndPassesNoPreviewPaths(t *testing.T) {
	originalExecute := executeClean
	originalLoader := loadProtectionConfiguration
	originalRecorder := newHistoryRecorder
	validator := pathsafe.NewValidator([]string{`C:\Protected`})
	recorder := &recordingHistoryRecorder{}
	loadProtectionConfiguration = func() clean.ProtectionConfiguration {
		return clean.ProtectionConfiguration{Validator: validator, Diagnostics: []clean.ProtectionDiagnostic{{Line: 2, Code: "invalid_path"}}}
	}
	newHistoryRecorder = func() (history.Recorder, error) { return recorder, nil }
	executeClean = func(_ context.Context, opts clean.Options) clean.Result {
		if got := opts.Validator.UserProtectionPaths(); len(got) != 1 || got[0] != `C:\Protected` {
			t.Fatalf("fresh protection rules = %#v", got)
		}
		if opts.HistoryRecorder != recorder || opts.DetectRunningApplications == nil {
			t.Fatal("production history or running detector missing")
		}
		if opts.RecycleBinAdapter != nil || opts.RecycleBinCapacityProbe != nil || opts.DetailedListDir != "" {
			t.Fatalf("production defaults or no-detailed-list boundary violated: %#v", opts)
		}
		if len(opts.OptIn) != 1 || opts.OptIn[0] != clean.DevCacheCategoryGo {
			t.Fatalf("OptIn = %#v, want canonical identifier only", opts.OptIn)
		}
		return clean.Result{Status: "ok"}
	}
	t.Cleanup(func() {
		executeClean = originalExecute
		loadProtectionConfiguration = originalLoader
		newHistoryRecorder = originalRecorder
	})

	msg := executeCleanSelectionCmd([]string{clean.DevCacheCategoryGo})().(cleanExecutedMsg)
	if msg.result.Status != "ok" {
		t.Fatalf("result = %#v", msg.result)
	}
}

func TestCleanExecutionResultRendersAllSkippedMixedAndErrorOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		result clean.Result
		wants  []string
	}{
		{name: "all skipped", result: clean.Result{Status: "ok", Skipped: []clean.SkippedItem{{Path: `C:\cache`, Reason: clean.StructuredIssue{Code: "recycle_bin_capacity", Message: "capacity unavailable"}}}, Totals: clean.Totals{SkippedCount: 1}}, wants: []string{"Deleted: 0", "Skipped: 1", "recycle_bin_capacity", "capacity unavailable"}},
		{name: "mixed", result: clean.Result{Status: "ok", Skipped: []clean.SkippedItem{{Path: `C:\protected`, Reason: clean.StructuredIssue{Code: "protected_path", Message: "protected"}}}, Totals: clean.Totals{DeletedCount: 1, SkippedCount: 1, AffectedBytes: 8}}, wants: []string{"Deleted: 1", "Skipped: 1", "protected_path", "Affected bytes: 8 bytes"}},
		{name: "error", result: clean.Result{Status: "error", Errors: []clean.StructuredIssue{{Code: "permission_denied", Message: "access denied"}}}, wants: []string{"Status: error", "Errors: 1", "permission_denied", "access denied"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderCleanExecutionResult(tt.result)
			for _, want := range tt.wants {
				if !strings.Contains(got, want) {
					t.Fatalf("result missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func updateRootKeyWithCmd(t *testing.T, model rootModel, key tea.KeyPressMsg) (rootModel, tea.Cmd) {
	t.Helper()
	next, cmd := model.Update(key)
	return next.(rootModel), cmd
}

func TestCleanPreviewRendersChromeBrowserCacheOpportunityAsSummary(t *testing.T) {
	model := clean.PreviewReadModel{
		Opportunities: []clean.Opportunity{{
			Category: clean.OpportunityCategoryBrowserCache,
			Path:     `C:\Users\corey\AppData\Local\Google\Chrome\User Data`,
			Bytes:    12,
			Status:   clean.OpportunityStatus,
			Reason:   clean.OpportunityReason,
			BrowserCache: &clean.BrowserCacheOpportunityDetail{
				Browser:      clean.ApplicationGoogleChrome,
				ProfileCount: 2,
				Profiles: []clean.BrowserCacheProfileDetail{{
					ID:   "Default",
					Path: `C:\Users\corey\AppData\Local\Google\Chrome\User Data\Default`,
					Caches: []clean.BrowserCacheDirectory{{
						Kind:  "Code Cache",
						Path:  `C:\Users\corey\AppData\Local\Google\Chrome\User Data\Default\Code Cache`,
						Bytes: 4,
					}},
				}},
			},
		}},
		OpportunityCount:         1,
		OpportunityObservedBytes: 12,
	}

	output := renderCleanPreviewSections(model, cleanPreviewFilterReviewOnly, true)

	for _, want := range []string{
		"Google Chrome browser cache",
		"category: browser_cache",
		"profiles: 2",
		"12 bytes",
		"not counted as Potential space",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("TUI output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"Code Cache", `\Default`, `\User Data`} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("TUI output contains noisy Chrome detail %q:\n%s", forbidden, output)
		}
	}
}

func TestCleanPreviewRendersEdgeBrowserCacheOpportunityAsSummary(t *testing.T) {
	model := clean.PreviewReadModel{
		Opportunities: []clean.Opportunity{{
			Category: clean.OpportunityCategoryBrowserCache,
			Path:     `C:\Users\corey\AppData\Local\Microsoft\Edge\User Data`,
			Bytes:    12,
			Status:   clean.OpportunityStatus,
			Reason:   clean.OpportunityReason,
			BrowserCache: &clean.BrowserCacheOpportunityDetail{
				Browser:      clean.ApplicationMicrosoftEdge,
				ProfileCount: 2,
				Profiles: []clean.BrowserCacheProfileDetail{{
					ID:   "Default",
					Path: `C:\Users\corey\AppData\Local\Microsoft\Edge\User Data\Default`,
					Caches: []clean.BrowserCacheDirectory{{
						Kind:  "GPUCache",
						Path:  `C:\Users\corey\AppData\Local\Microsoft\Edge\User Data\Default\GPUCache`,
						Bytes: 4,
					}},
				}},
			},
		}},
		OpportunityCount:         1,
		OpportunityObservedBytes: 12,
	}

	output := renderCleanPreviewSections(model, cleanPreviewFilterReviewOnly, true)

	for _, want := range []string{
		"Microsoft Edge browser cache",
		"category: browser_cache",
		"profiles: 2",
		"12 bytes",
		"not counted as Potential space",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("TUI output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"GPUCache", `\Default`, `\User Data`} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("TUI output contains noisy Edge detail %q:\n%s", forbidden, output)
		}
	}
}

func TestCleanPreviewTUIRendersSharedFoalReportCategories(t *testing.T) {
	model := clean.PreviewReadModel{
		ProtectionRules: []clean.PreviewProtectionRule{{
			ID:          "foal_owned_temp_sandboxes",
			Description: "Foal-owned temporary sandbox entries",
		}},
		Candidates: []clean.PreviewCandidate{{
			Path:          `C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
			Bytes:         12,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		}},
		Opportunities: []clean.Opportunity{
			{
				Category: clean.OpportunityCategoryUserTemp,
				Path:     `C:\Users\corey\AppData\Local\Temp\old-cache`,
				Bytes:    20,
				Status:   clean.OpportunityStatus,
				Reason:   clean.OpportunityReason,
			},
			{
				Category: clean.OpportunityCategoryWindowsErrorReporting,
				Path:     `C:\Users\corey\AppData\Local\Microsoft\Windows\WER`,
				Bytes:    30,
				Status:   clean.OpportunityStatus,
				Reason:   clean.OpportunityReason,
			},
			{
				Category: clean.OpportunityCategoryBrowserCache,
				Path:     `C:\Users\corey\AppData\Local\Microsoft\Edge\User Data`,
				Bytes:    40,
				Status:   clean.OpportunityStatus,
				Reason:   clean.OpportunityReason,
				BrowserCache: &clean.BrowserCacheOpportunityDetail{
					Browser:      clean.ApplicationMicrosoftEdge,
					ProfileCount: 1,
				},
			},
		},
		ReviewSuggestions: []clean.PreviewReviewSuggestion{{
			Label:   "Go build cache",
			Command: "go clean -cache",
		}},
		ReviewClues: []clean.PreviewReviewClue{{
			Name:    "Rebuildable project artifacts",
			Details: "Use foal analyze <path> to inspect rebuildable project directories explicitly.",
		}},
		RunningApplicationSkips: []clean.PreviewRunningApplicationSkip{{
			Name:        "Google Chrome browser review",
			Application: "Google Chrome",
			Reason:      "Google Chrome is running; browser cache review was skipped.",
		}},
		Notices: []clean.PreviewNotice{{
			Kind:    "permission_boundary",
			Message: "Permission boundary: administrator-only caches are excluded.",
		}},
		PotentialSpaceBytes:      12,
		CandidateCount:           1,
		OpportunityCount:         3,
		OpportunityObservedBytes: 90,
		Summary:                  "Dry-run summary: No changes were made.",
	}

	output := renderCleanPreviewSections(model, cleanPreviewFilterAll, true)

	assertContainsInOrder(t, output, []string{
		"System",
		`C:\Users\corey\AppData\Local\Microsoft\Windows\WER`,
		"User essentials",
		"status: default candidate",
		`C:\Users\corey\AppData\Local\Temp\old-cache`,
		"Browsers",
		"Microsoft Edge browser cache",
		"status: running skip",
		"Developer tools",
		"status: Review suggestion",
		"Project artifacts",
		"status: Review clue",
		"Protection",
	})
	for _, forbidden := range []string{
		"Applications",
		"Cloud",
		"Virtualization",
		"Potential space: 102 bytes",
		"Execution complete",
		"Deleted:",
		"close browser",
		"Run as Administrator",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("TUI output contains forbidden category or semantic drift %q:\n%s", forbidden, output)
		}
	}
}

func TestCleanPreviewScrollClampsAtBounds(t *testing.T) {
	candidates := make([]clean.PreviewCandidate, 60)
	for index := range candidates {
		candidates[index] = clean.PreviewCandidate{
			Path:  `C:\Users\corey\AppData\Local\Temp\foal-sandbox.tmp`,
			Bytes: 1,
			Rule:  "foal_owned_temp_sandboxes",
		}
	}
	model := newCleanModel(80, 16)
	model.applyLoaded(cleanPreviewLoadedMsg{model: clean.PreviewReadModel{
		Candidates:          candidates,
		PotentialSpaceBytes: 60,
		CandidateCount:      60,
	}})

	if !model.vp.AtTop() {
		t.Fatal("clean preview should start scrolled to the top")
	}
	model.handleKey("k")
	if !model.vp.AtTop() {
		t.Fatal("k at the top must not scroll past the first row")
	}
	for i := 0; i < 200; i++ {
		model.handleKey("j")
	}
	if !model.vp.AtBottom() {
		t.Fatal("repeated j must stop at the bottom of the content")
	}
	model.handleKey("j")
	if !model.vp.AtBottom() {
		t.Fatal("j at the bottom must stay clamped")
	}
}

func TestCleanPreviewFilterKeyCyclesAndResetsScroll(t *testing.T) {
	model := newCleanModel(80, 16)
	model.applyLoaded(cleanPreviewLoadedMsg{model: clean.PreviewReadModel{}})

	for _, want := range []cleanPreviewFilter{
		cleanPreviewFilterActionablePreview,
		cleanPreviewFilterReviewOnly,
		cleanPreviewFilterDiagnostics,
		cleanPreviewFilterAll,
	} {
		model.handleKey("f")
		if model.filter != want {
			t.Fatalf("filter = %q, want %q", model.filter, want)
		}
		if !model.vp.AtTop() {
			t.Fatal("changing the filter must reset scroll to the top")
		}
	}
}

func TestCleanPreviewIntentFiltersRenderFocusedSectionsWithoutChangingTotals(t *testing.T) {
	readModel := clean.PreviewReadModel{
		Title: "Foal clean",
		Candidates: []clean.PreviewCandidate{{
			Path:          `C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
			Bytes:         12,
			Rule:          "foal_owned_temp_sandboxes",
			PlannedAction: "move_to_recycle_bin",
		}},
		Skipped: []clean.PreviewSkippedItem{{
			Path: `\\?\C:\Windows\System32`,
			Reason: clean.StructuredIssue{
				Code:        "protected_path",
				Recoverable: true,
			},
		}},
		SkippedByDefault: []clean.PreviewSkippedByDefaultItem{{
			Name:   "Browser cache family",
			Path:   `C:\Users\corey\AppData\Local\Browser\Cache`,
			Bytes:  4096,
			Reason: "requires explicit future opt-in",
		}},
		Opportunities: []clean.Opportunity{{
			Category: clean.OpportunityCategoryUserTemp,
			Path:     `C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
			Bytes:    4096,
			Status:   clean.OpportunityStatus,
			Reason:   clean.OpportunityReason,
		}},
		IncompleteOpportunityInspections: []clean.IncompleteOpportunityInspection{{
			Category: clean.OpportunityCategoryUserTemp,
			Path:     `C:\Users\corey\AppData\Local\Temp\partial-cache`,
			Reason: clean.StructuredIssue{
				Code:        "inspection_limit",
				Recoverable: true,
			},
		}},
		ReviewClues: []clean.PreviewReviewClue{{
			Name:    "Project artifact clue",
			Path:    `D:\Code\Personal\Foal\node_modules`,
			Details: "review manually before deleting",
		}},
		ReviewSuggestions: []clean.PreviewReviewSuggestion{{
			Label:    "Open Windows Storage settings",
			NextStep: "Use Windows Settings to review large app storage.",
		}},
		RunningApplicationSkips: []clean.PreviewRunningApplicationSkip{{
			Name:        "Sync client cache",
			Application: "SyncClient.exe",
			Path:        `C:\Users\corey\Sync\Cache`,
			Reason:      "application is running",
		}},
		ProtectionDiagnostics: []clean.ProtectionDiagnostic{{
			Code:        "invalid_protection_rule",
			Source:      `C:\Users\corey\AppData\Roaming\Foal\protection.txt`,
			Recoverable: true,
		}},
		Errors: []clean.StructuredIssue{{
			Code:        "inspection_failed",
			Path:        `C:\Users\corey\AppData\Local\Temp\missing`,
			Recoverable: true,
		}},
		PotentialSpaceBytes:      12,
		CandidateCount:           1,
		SkippedCount:             1,
		OpportunityCount:         2,
		OpportunityObservedBytes: 8192,
	}

	assertFilterOutput := func(filter cleanPreviewFilter, want []string, forbidden []string) {
		t.Helper()
		output := renderCleanPreviewSections(readModel, filter, true)
		for _, text := range []string{
			"Potential space: 12 bytes",
			"Observed opportunity bytes: 8192 bytes (not counted as Potential space)",
			"Default candidates: 1 | Skipped: 1 | Diagnostics: 1",
		} {
			if !strings.Contains(output, text) {
				t.Fatalf("%s output changed summary %q:\n%s", filter, text, output)
			}
		}
		for _, text := range want {
			if !strings.Contains(output, text) {
				t.Fatalf("%s output missing %q:\n%s", filter, text, output)
			}
		}
		for _, text := range forbidden {
			if strings.Contains(output, text) {
				t.Fatalf("%s output contains forbidden %q:\n%s", filter, text, output)
			}
		}
	}

	assertFilterOutput(cleanPreviewFilterAll, []string{
		"Default candidates (1)",
		"Skipped items (1)",
		"old-tool-cache",
		"Project artifact clue",
		"Review suggestions (1)",
		"SyncClient.exe",
		"inspection_limit",
		"invalid_protection_rule",
		"inspection_failed",
	}, nil)
	assertFilterOutput(cleanPreviewFilterActionablePreview, []string{
		"Default candidates (1)",
		"Skipped items (1)",
		"inspection_failed",
	}, []string{
		"old-tool-cache",
		"Browser cache family",
		"Project artifact clue",
		"Review suggestions (1)",
		"SyncClient.exe",
		"inspection_limit",
		"invalid_protection_rule",
	})
	assertFilterOutput(cleanPreviewFilterReviewOnly, []string{
		"old-tool-cache",
		"Browser cache family",
		"Project artifact clue",
		"Review suggestions (1)",
		"SyncClient.exe",
	}, []string{
		"Default candidates (1)",
		"Skipped items (1)",
		"inspection_failed",
		"inspection_limit",
		"invalid_protection_rule",
	})
	assertFilterOutput(cleanPreviewFilterDiagnostics, []string{
		"inspection_failed",
		"inspection_limit",
		"invalid_protection_rule",
	}, []string{
		"Default candidates (1)",
		"Skipped items (1)",
		"old-tool-cache",
		"Browser cache family",
		"Project artifact clue",
		"Review suggestions (1)",
		"SyncClient.exe",
	})
}

func TestCleanPreviewFilterKeepsCopyPayloadScopedToDefaultCandidates(t *testing.T) {
	originalCopy := copyTextToClipboard
	copied := ""
	copyTextToClipboard = func(text string) error {
		copied = text
		return nil
	}
	t.Cleanup(func() { copyTextToClipboard = originalCopy })

	model := cleanModel{
		filter:   cleanPreviewFilterReviewOnly,
		expanded: true,
		vp:       viewport.New(viewport.WithWidth(80), viewport.WithHeight(40)),
		width:    80,
		height:   48,
		loading:  false,
		model: clean.PreviewReadModel{
			Title: "Foal clean",
			Candidates: []clean.PreviewCandidate{{
				Path:          `C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
				Bytes:         12,
				Rule:          "foal_owned_temp_sandboxes",
				PlannedAction: "move_to_recycle_bin",
			}},
			SkippedByDefault: []clean.PreviewSkippedByDefaultItem{{
				Name:   "Browser cache family",
				Path:   `C:\Users\corey\AppData\Local\Browser\Cache`,
				Bytes:  4096,
				Reason: "requires explicit future opt-in",
			}},
			ReviewClues: []clean.PreviewReviewClue{{
				Name:    "Project artifact clue",
				Path:    `D:\Code\Personal\Foal\node_modules`,
				Details: "review manually before deleting",
			}},
			ReviewSuggestions: []clean.PreviewReviewSuggestion{{
				Label:    "Open Windows Storage settings",
				NextStep: "Use Windows Settings to review large app storage.",
			}},
			RunningApplicationSkips: []clean.PreviewRunningApplicationSkip{{
				Name:        "Sync client cache",
				Application: "SyncClient.exe",
				Path:        `C:\Users\corey\Sync\Cache`,
				Reason:      "application is running",
			}},
			PotentialSpaceBytes: 12,
			CandidateCount:      1,
		},
	}

	model.handleKey("c")

	if want := `C:\Users\corey\AppData\Local\Temp\foal-default.tmp` + "\n"; copied != want {
		t.Fatalf("clipboard payload = %q, want %q", copied, want)
	}
	for _, forbidden := range []string{"Browser cache family", "node_modules", "Open Windows Storage settings", "Sync client cache"} {
		if strings.Contains(copied, forbidden) {
			t.Fatalf("clipboard payload includes review-only text %q: %q", forbidden, copied)
		}
	}
}

func TestCleanPreviewRendersSharedProjectArtifactClueAsReadOnlyAnalysisGuidance(t *testing.T) {
	readModel := clean.NewPreviewReadModel(clean.Result{
		Status: "preview",
		Mode:   "dry_run",
	})

	output := renderCleanPreviewSections(readModel, cleanPreviewFilterReviewOnly, true)

	for _, want := range []string{
		"Review clues (1)",
		"Rebuildable project artifacts (review only)",
		"status: Review clue",
		"foal analyze <path>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{
		"cleanup candidate",
		"Execution complete",
		"Deleted:",
		"move_to_recycle_bin",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains cleanup semantics %q:\n%s", forbidden, output)
		}
	}
}

func TestCleanPreviewRendersReviewSuggestionSafetyNoteOnceAboveSuggestions(t *testing.T) {
	readModel := clean.PreviewReadModel{
		ReviewSuggestions: []clean.PreviewReviewSuggestion{
			{Label: "Review npm cache"},
			{Label: "Review Go build cache"},
		},
	}

	output := renderCleanPreviewSections(readModel, cleanPreviewFilterReviewOnly, true)
	note := "Clearing a tool cache while the tool is installing or building can disrupt that operation. Confirm the tool is idle first."

	if strings.Count(output, note) != 1 {
		t.Fatalf("safety note count = %d, want 1:\n%s", strings.Count(output, note), output)
	}
	headingIndex := strings.Index(output, "Review suggestions (2)\n")
	noteIndex := strings.Index(output, note)
	firstSuggestionIndex := strings.Index(output, "Review npm cache")
	if headingIndex == -1 || noteIndex <= headingIndex || firstSuggestionIndex <= noteIndex {
		t.Fatalf("safety note must render above the suggestions list:\n%s", output)
	}
}

func TestCleanPreviewOmitsReviewSuggestionSafetyNoteWithoutSuggestions(t *testing.T) {
	output := renderCleanPreviewSections(clean.PreviewReadModel{}, cleanPreviewFilterReviewOnly, true)

	if strings.Contains(output, clean.ReviewSuggestionSafetyNote) {
		t.Fatalf("safety note must not render without suggestions:\n%s", output)
	}
}

func TestCleanPreviewCapsHighVolumeOpportunityRendering(t *testing.T) {
	opportunities := make([]clean.UserTempOpportunity, 0, 11)
	for index := 0; index < 11; index++ {
		opportunities = append(opportunities, clean.UserTempOpportunity{
			Path:             `C:\Users\corey\AppData\Local\Temp\opportunity-` + string(rune('A'+index)),
			Bytes:            int64(index + 1),
			LatestModifiedAt: time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC),
			IdleDays:         9,
			Status:           clean.UserTempOpportunityStatus,
			Reason:           clean.UserTempOpportunityReason,
		})
	}
	model := clean.PreviewReadModel{
		Opportunities:            opportunities,
		OpportunityCount:         len(opportunities),
		OpportunityObservedBytes: 66,
	}

	output := renderCleanPreviewSections(model, cleanPreviewFilterReviewOnly, false)

	if !strings.Contains(output, "Skipped by default: 11 user-temp opportunity item(s)") ||
		!strings.Contains(output, "1 omitted.") {
		t.Fatalf("output missing high-volume summary:\n%s", output)
	}
	if !strings.Contains(output, `opportunity-J`) {
		t.Fatalf("output missing tenth opportunity:\n%s", output)
	}
	if strings.Contains(output, `opportunity-K`) {
		t.Fatalf("output rendered opportunity beyond the cap:\n%s", output)
	}
}
