package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/history"
)

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
			Opportunities: []clean.UserTempOpportunity{{
				Path:             `C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
				Bytes:            4096,
				LatestModifiedAt: time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC),
				IdleDays:         9,
				Status:           clean.UserTempOpportunityStatus,
				Reason:           clean.UserTempOpportunityReason,
			}},
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
				OpportunityCount:         1,
				OpportunityObservedBytes: 4096,
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

func openCleanPreview(t *testing.T) rootModel {
	t.Helper()
	model := newRootModel()
	// Wide window so long candidate paths are not clipped by the viewport.
	next, _ := model.Update(tea.WindowSizeMsg{Width: 240, Height: 60})
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
		"Clean preview TUI",
		"Read-only review over foal clean --dry-run",
		"Potential space: 12 bytes",
		"Default candidates (1)",
		`C:\Users\corey\AppData\Local\Temp\foal-default.tmp`,
		"Skipped items (1)",
		`\\?\C:\Windows\System32`,
		"protected_path",
		"Inspection errors (1)",
		"inspection_failed",
		"Protection rules",
		"Opportunities: 1, observed bytes: 4096 bytes",
		`C:\Users\corey\AppData\Local\Temp\old-tool-cache`,
		"latest modified: 2026-06-01T12:00:00Z",
		"idle days: 9",
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
		"Execution complete",
		"Deleted:",
		"Execute",
		"execute cleanup",
		"confirm",
		"Confirmation",
		"Potential space: 4108 bytes",
		"Detailed candidate list:",
		"Run as Administrator",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("content contains forbidden execution or potential-space wording %q:\n%s", forbidden, content)
		}
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'e', Text: "e"})
	for _, want := range []string{
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

func TestCleanPreviewRendersUserDefinedProtectionRuleFromReadModel(t *testing.T) {
	path := `C:\Users\corey\Work\Valuable`
	output := renderCleanPreviewSections(clean.PreviewReadModel{
		ProtectionRules: []clean.PreviewProtectionRule{{
			Path:        path,
			UserDefined: true,
		}},
	}, cleanPreviewFilterAll, false)

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
	}, cleanPreviewFilterAll, false)

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

	openCleanPreview(t)
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

	model.handleKey("f")
	if model.filter != cleanPreviewFilterCandidates {
		t.Fatalf("filter = %q, want %q", model.filter, cleanPreviewFilterCandidates)
	}
	if !model.vp.AtTop() {
		t.Fatal("changing the filter must reset scroll to the top")
	}
}

func TestCleanPreviewFilterShowsReviewSectionsWithoutChangingPotentialSpace(t *testing.T) {
	model := cleanModel{
		filter:   cleanPreviewFilterReview,
		expanded: true,
		vp:       viewport.New(viewport.WithWidth(80), viewport.WithHeight(40)),
		width:    80,
		height:   48,
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

	output := model.headerContent() + renderCleanPreviewSections(model.model, model.filter, model.expanded)

	for _, want := range []string{
		"Filter: review",
		"Potential space: 12 bytes",
		"Skipped by default (1)",
		"Browser cache family",
		"requires explicit future opt-in",
		"Review clues (1)",
		"Project artifact clue",
		"Review suggestions (1)",
		"Use Windows Settings to review large app storage.",
		"Running application skips (1)",
		"SyncClient.exe",
		"not counted as Potential space",
		"review only",
		"skipped, not executable here",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{
		"Potential space: 4108 bytes",
		"Default candidates (1)",
		"preview action metadata: Recycle Bin)\n  Browser cache family",
		"cleanup candidate",
		"Execution complete",
		"Deleted:",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output contains forbidden review/execution wording %q:\n%s", forbidden, output)
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

	output := renderCleanPreviewSections(readModel, cleanPreviewFilterReview, true)
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
	output := renderCleanPreviewSections(clean.PreviewReadModel{}, cleanPreviewFilterReview, true)

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

	output := renderCleanPreviewSections(model, cleanPreviewFilterReview, false)

	if !strings.Contains(output, "Skipped-by-default opportunities (11)") ||
		!strings.Contains(output, "1 omitted from this review view.") {
		t.Fatalf("output missing high-volume summary:\n%s", output)
	}
	if !strings.Contains(output, `opportunity-J`) {
		t.Fatalf("output missing tenth opportunity:\n%s", output)
	}
	if strings.Contains(output, `opportunity-K`) {
		t.Fatalf("output rendered opportunity beyond the cap:\n%s", output)
	}
}
