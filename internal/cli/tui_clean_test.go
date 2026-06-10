package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
)

func stubCleanPreviewDryRun(t *testing.T) {
	t.Helper()
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	originalExecute := executeClean
	dryRunClean = func(ctx context.Context, opts clean.Options) clean.Result {
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
			Totals:           clean.Totals{CandidateCount: 1, CandidateBytes: 12, SkippedCount: 1},
			DetailedListPath: `C:\Users\corey\AppData\Roaming\Foal\history\clean-dry-run-detail.txt`,
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
	next, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
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
		"Detailed candidate list:",
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
		"Run as Administrator",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("content contains forbidden execution or potential-space wording %q:\n%s", forbidden, content)
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
