package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func TestAnalyzeBrowseShowsCompactArtifactLabelAndGuardedPurgeHandoff(t *testing.T) {
	projectRoot := t.TempDir()
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		return analyze.BrowseResult{
			OK:   true,
			Root: projectRoot,
			Children: []analyze.BrowseChild{
				{
					Name:           "node_modules",
					Path:           filepath.Join(projectRoot, "node_modules"),
					Kind:           analyze.BrowseKindDirectory,
					Bytes:          100,
					State:          analyze.BrowseStateComplete,
					Classification: analyze.ClassificationProjectArtifactClue,
					Navigable:      true,
				},
				{
					Name:      "src",
					Path:      filepath.Join(projectRoot, "src"),
					Kind:      analyze.BrowseKindDirectory,
					Bytes:     10,
					State:     analyze.BrowseStateComplete,
					Navigable: true,
				},
			},
		}
	})

	model := loadAnalyzeDrive(t)
	cmd := model.analyze.beginBrowseLocation(projectRoot, "Browsing...", false)
	model = drainAnalyzeBrowse(t, model, cmd)
	content := model.content()
	if !strings.Contains(content, "artifact") {
		t.Fatalf("expected compact artifact label:\n%s", content)
	}
	if strings.Contains(content, "project_artifact_clue") {
		t.Fatalf("TUI should show compact label, not raw JSON token:\n%s", content)
	}
	if !strings.Contains(content, "foal purge") {
		t.Fatalf("purge-valid project root with clue must show handoff:\n%s", content)
	}
	if !strings.Contains(content, projectRoot) {
		t.Fatalf("handoff must include current location:\n%s", content)
	}
}

func TestAnalyzeBrowseOmitsPurgeHandoffOnVolumeRootEvenWithArtifactClue(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{
					Name:           "node_modules",
					Path:           `C:\node_modules`,
					Kind:           analyze.BrowseKindDirectory,
					Bytes:          100,
					State:          analyze.BrowseStateComplete,
					Classification: analyze.ClassificationProjectArtifactClue,
					Navigable:      true,
				},
			},
		}
	})
	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	content := model.content()
	if strings.Contains(content, "foal purge") {
		t.Fatalf("volume root must never show purge handoff:\n%s", content)
	}
	if !strings.Contains(content, "artifact") {
		t.Fatalf("direct child clue should still show compact artifact label:\n%s", content)
	}
}

func TestAnalyzeBrowseNegativeCapabilitiesDoNotMutateOrLaunch(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	var browseCalls int
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		browseCalls++
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{Name: "file.txt", Path: `C:\file.txt`, Kind: analyze.BrowseKindFile, Bytes: 4, State: analyze.BrowseStateComplete},
				{Name: "dir", Path: `C:\dir`, Kind: analyze.BrowseKindDirectory, Bytes: 8, State: analyze.BrowseStateComplete, Navigable: true},
			},
		}
	})
	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	beforeCalls := browseCalls
	beforeRoot := model.analyze.browseRoot
	beforeChildren := len(model.analyze.browseChildren)

	for _, key := range []tea.KeyPressMsg{
		{Code: 'd', Text: "d"},
		{Code: 'x', Text: "x"},
		{Code: 'p', Text: "p"},
		{Code: 'o', Text: "o"},
		{Code: ' ', Text: " "},
		{Code: tea.KeyDelete},
		{Code: tea.KeyBackspace},
	} {
		next, cmd := model.Update(key)
		model = next.(rootModel)
		if cmd != nil {
			msg := cmd()
			switch msg.(type) {
			case analyzeBrowseStartedMsg, analyzeBrowseLoadedMsg, analyzeBrowseObservationMsg, analyzeVolumesLoadedMsg:
				t.Fatalf("forbidden key must not start browse/volume work: %T", msg)
			}
		}
	}
	if model.analyze.browseRoot != beforeRoot {
		t.Fatalf("forbidden keys must not change browse root: %q -> %q", beforeRoot, model.analyze.browseRoot)
	}
	if len(model.analyze.browseChildren) != beforeChildren {
		t.Fatalf("forbidden keys must not change children")
	}
	if browseCalls != beforeCalls {
		t.Fatalf("forbidden keys must not re-browse; calls %d -> %d", beforeCalls, browseCalls)
	}
	model.analyze.browseSelectedPath = `C:\file.txt`
	model.analyze.syncBrowseSelectionAfterRank()
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd != nil {
		t.Fatalf("enter on file must not start navigation cmd")
	}
	if model.analyze.browseRoot != `C:\` {
		t.Fatalf("enter on file must stay on current root, got %q", model.analyze.browseRoot)
	}
	if model.analyze.notice == "" {
		t.Fatalf("enter on file should set a non-open notice")
	}
	content := strings.ToLower(model.content())
	for _, banned := range []string{"delete selected", "launch purge", "elevate", "uac", "history written"} {
		if strings.Contains(content, banned) {
			t.Fatalf("banned affordance language %q in content", banned)
		}
	}
}

func TestAnalyzeBrowseDoesNotWriteHistory(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		return analyze.BrowseResult{
			OK:   true,
			Root: root,
			Children: []analyze.BrowseChild{
				{Name: "a", Path: filepath.Join(root, "a"), Kind: analyze.BrowseKindFile, Bytes: 1, State: analyze.BrowseStateComplete},
			},
		}
	})
	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	_ = model.content()
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)
	if cmd != nil {
		model = drainAnalyzeBrowse(t, model, cmd)
	}
	entries, err := filepath.Glob(filepath.Join(historyDir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("Analyze must not write History sessions: %#v", entries)
	}
}
