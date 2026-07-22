package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func stubAnalyzeBrowse(t *testing.T, fn func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult) {
	t.Helper()
	original := browseAnalyzeLocation
	browseAnalyzeLocation = fn
	t.Cleanup(func() { browseAnalyzeLocation = original })
}

// drainAnalyzeBrowse runs the browse start/observation/result stream to terminal.
func drainAnalyzeBrowse(t *testing.T, model rootModel, cmd tea.Cmd) rootModel {
	t.Helper()
	for i := 0; i < 10000 && cmd != nil; i++ {
		msg := cmd()
		next, nextCmd := model.Update(msg)
		model = next.(rootModel)
		switch msg.(type) {
		case analyzeBrowseLoadedMsg:
			return model
		case analyzeBrowseStartedMsg, analyzeBrowseObservationMsg:
			cmd = nextCmd
		default:
			t.Fatalf("unexpected browse stream msg %T", msg)
		}
	}
	t.Fatal("browse stream did not finish")
	return model
}

func enterAnalyzeBrowse(t *testing.T, model rootModel) rootModel {
	t.Helper()
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter available drive must start browse load")
	}
	return drainAnalyzeBrowse(t, model, cmd)
}

func TestAnalyzeEnterDriveBrowsesDirectChildrenOnly(t *testing.T) {
	var roots []string
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true, HasCapacity: true, TotalBytes: 100, FreeBytes: 40},
		{Root: `E:\`, Letter: "E:", Kind: analyze.VolumeKindRemovable, Available: true, Label: "USB"},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		roots = append(roots, root)
		if root != `C:\` {
			t.Fatalf("must not prefetch sibling; got root %q", root)
		}
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{Name: "Windows", Path: `C:\Windows`, Kind: analyze.BrowseKindDirectory, Bytes: 1000, State: analyze.BrowseStateComplete, Navigable: true},
				{Name: "pagefile.sys", Path: `C:\pagefile.sys`, Kind: analyze.BrowseKindFile, Bytes: 50, State: analyze.BrowseStateComplete, Hidden: true, System: true},
				{Name: "DocsLink", Path: `C:\DocsLink`, Kind: analyze.BrowseKindReparse, State: analyze.BrowseStateSkipped, SkipReason: "reparse_point"},
			},
		}
	})

	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)

	if model.analyze.phase != analyzePhaseBrowse {
		t.Fatalf("phase = %v, want browse", model.analyze.phase)
	}
	if len(roots) != 1 || roots[0] != `C:\` {
		t.Fatalf("browse roots = %#v, want only C:\\", roots)
	}
	content := model.content()
	for _, want := range []string{
		"Location: C:\\",
		"Windows",
		"pagefile.sys",
		"hidden",
		"system",
		"DocsLink",
		"reparse_point",
		"read-only",
		"not navigable",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("browse missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "no cleanup or deletion") {
		t.Fatalf("must stay read-only:\n%s", content)
	}
}

func TestAnalyzeBrowseEnterDirectoryAndEscToParentThenDrive(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		switch filepath.Clean(root) {
		case `C:\`:
			return analyze.BrowseResult{
				OK:   true,
				Root: `C:\`,
				Children: []analyze.BrowseChild{
					{Name: "Users", Path: `C:\Users`, Kind: analyze.BrowseKindDirectory, Bytes: 10, State: analyze.BrowseStateComplete, Navigable: true},
					{Name: "readme.txt", Path: `C:\readme.txt`, Kind: analyze.BrowseKindFile, Bytes: 3, State: analyze.BrowseStateComplete},
				},
			}
		case `C:\Users`:
			return analyze.BrowseResult{
				OK:   true,
				Root: `C:\Users`,
				Children: []analyze.BrowseChild{
					{Name: "Public", Path: `C:\Users\Public`, Kind: analyze.BrowseKindDirectory, Bytes: 5, State: analyze.BrowseStateComplete, Navigable: true},
				},
			}
		default:
			t.Fatalf("unexpected browse root %q", root)
			return analyze.BrowseResult{}
		}
	})

	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	if !strings.Contains(model.content(), "Location: C:\\") {
		t.Fatalf("want volume root browse:\n%s", model.content())
	}

	// Enter Users (first child, navigable directory).
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter Users must load browse")
	}
	model = drainAnalyzeBrowse(t, model, cmd)
	if !strings.Contains(model.content(), `Location: C:\Users`) {
		t.Fatalf("want Users browse:\n%s", model.content())
	}

	// Esc → parent volume root
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("esc from nested must reload parent")
	}
	model = drainAnalyzeBrowse(t, model, cmd)
	if model.analyze.phase != analyzePhaseBrowse {
		t.Fatalf("phase = %v after esc to parent", model.analyze.phase)
	}
	if !strings.Contains(model.content(), "Location: C:\\") {
		t.Fatalf("esc should return to volume root:\n%s", model.content())
	}

	// Esc from volume root → drive entry
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if cmd != nil {
		t.Fatalf("esc from volume root must not load browse, got %T", cmd())
	}
	if model.analyze.phase != analyzePhaseDrive {
		t.Fatalf("phase = %v, want drive", model.analyze.phase)
	}
	if !strings.Contains(model.content(), "Local volumes") {
		t.Fatalf("should show drive entry:\n%s", model.content())
	}
}

func TestAnalyzeBrowseFileAndReparseNotNavigable(t *testing.T) {
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
				{Name: "big.dat", Path: `C:\big.dat`, Kind: analyze.BrowseKindFile, Bytes: 99, State: analyze.BrowseStateComplete},
				{Name: "junction", Path: `C:\junction`, Kind: analyze.BrowseKindReparse, State: analyze.BrowseStateSkipped, SkipReason: "reparse_point"},
				{Name: "Windows", Path: `C:\Windows`, Kind: analyze.BrowseKindDirectory, Bytes: 1, State: analyze.BrowseStateComplete, Navigable: true},
			},
		}
	})

	model := loadAnalyzeDrive(t)
	// Enter drive
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	model = drainAnalyzeBrowse(t, model, cmd)
	afterDrive := browseCalls

	// Enter file (cursor 0)
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd != nil {
		t.Fatal("enter on file must not start browse")
	}
	if browseCalls != afterDrive {
		t.Fatalf("file enter caused browse calls %d -> %d", afterDrive, browseCalls)
	}
	if !strings.Contains(model.content(), "file and cannot be entered") {
		t.Fatalf("file notice missing:\n%s", model.content())
	}

	// Move to reparse
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd != nil {
		t.Fatal("enter on reparse must not start browse")
	}
	if browseCalls != afterDrive {
		t.Fatalf("reparse enter caused browse calls")
	}
	if !strings.Contains(model.content(), "reparse point") {
		t.Fatalf("reparse notice missing:\n%s", model.content())
	}
}

func TestAnalyzeBrowseUsesSharedBrowseServiceNotTUITraversal(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `D:\`, Letter: "D:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		return analyze.BrowseResult{
			OK:   true,
			Root: root,
			Children: []analyze.BrowseChild{
				{Name: "only", Path: filepath.Join(root, "only"), Kind: analyze.BrowseKindFile, Bytes: 7, State: analyze.BrowseStateComplete},
			},
		}
	})

	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	if !strings.Contains(model.content(), "only") {
		t.Fatalf("missing child from shared service:\n%s", model.content())
	}
}

func TestAnalyzeBrowseEscFromDriveEntryStillReturnsMenu(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	model := loadAnalyzeDrive(t)
	next, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if model.screen != screenMenu {
		t.Fatalf("screen = %v", model.screen)
	}
}

func TestIsAnalyzeVolumeRoot(t *testing.T) {
	if !isAnalyzeVolumeRoot(`C:\`) {
		t.Fatal(`C:\ should be volume root`)
	}
	if isAnalyzeVolumeRoot(`C:\Users`) {
		t.Fatal(`C:\Users should not be volume root`)
	}
	got := parentBrowsePath(`C:\Users\Public`)
	if filepath.Clean(got) != filepath.Clean(`C:\Users`) {
		t.Fatalf("parent = %q", got)
	}
}

func TestAnalyzeBrowseRendersHonestStatesAndPercentWording(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{
					Name: "complete-dir", Path: `C:\complete-dir`, Kind: analyze.BrowseKindDirectory,
					Bytes: 50, FileCount: 2, DirectoryCount: 1,
					State: analyze.BrowseStateComplete, Navigable: true,
				},
				{
					Name: "partial-dir", Path: `C:\partial-dir`, Kind: analyze.BrowseKindDirectory,
					Bytes: 30, FileCount: 1, DirectoryCount: 1,
					State: analyze.BrowseStatePartial, Navigable: true,
					SkipAggregates: []analyze.SkipAggregate{
						{Reason: analyze.SkipReasonPermissionDenied, Count: 2},
					},
				},
				{
					Name: "incomplete-dir", Path: `C:\incomplete-dir`, Kind: analyze.BrowseKindDirectory,
					Bytes: 20, FileCount: 1, DirectoryCount: 1,
					State: analyze.BrowseStateIncomplete, Navigable: true,
					SkipAggregates: []analyze.SkipAggregate{
						{Reason: analyze.SkipReasonHardLimit, Count: 1},
					},
				},
				{
					Name: "link", Path: `C:\link`, Kind: analyze.BrowseKindReparse,
					State: analyze.BrowseStateSkipped, SkipReason: analyze.SkipReasonReparsePoint,
				},
			},
		}
	})

	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	content := model.content()

	// Partial/Incomplete sizes use >= ; percentages never use >=.
	if !strings.Contains(content, "partial") {
		t.Fatalf("missing partial state:\n%s", content)
	}
	if !strings.Contains(content, "incomplete") {
		t.Fatalf("missing incomplete state:\n%s", content)
	}
	if !strings.Contains(content, ">=") {
		t.Fatalf("partial/incomplete sizes must use >= lower bound:\n%s", content)
	}
	if strings.Contains(content, ">=%") || strings.Contains(content, ">= %") {
		t.Fatalf("percentages must never use >=:\n%s", content)
	}
	// Location not complete → approximate observed shares.
	if !strings.Contains(content, "observed") && !strings.Contains(content, "~") {
		t.Fatalf("non-complete location must label approximate shares:\n%s", content)
	}
	// Skipped reparse with stable reason.
	if !strings.Contains(content, "skipped") || !strings.Contains(content, "reparse_point") {
		t.Fatalf("skipped reparse missing:\n%s", content)
	}
	// Focused detail aggregates without descendant paths.
	if !strings.Contains(content, "Detail:") {
		t.Fatalf("focused detail missing:\n%s", content)
	}
	if !strings.Contains(content, "state=") {
		t.Fatalf("detail must expose state:\n%s", content)
	}
	// Cursor starts on first child (complete-dir); move to partial for aggregate reasons.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	content = model.content()
	if !strings.Contains(content, "permission_denied") {
		t.Fatalf("focused partial detail must show aggregate reason:\n%s", content)
	}
	// Must not dump synthetic descendant paths from aggregates.
	if strings.Contains(content, `C:\partial-dir\secret-denied-child`) {
		t.Fatalf("must not render unbounded descendant paths:\n%s", content)
	}
}

func TestAnalyzeBrowseAppliesStreamingObservationsBeforeFinalResult(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		if onObservation != nil {
			onObservation(analyze.ChildObservation{
				Name: "growing", Path: `C:\growing`, Kind: analyze.BrowseKindDirectory,
				Bytes: 10, State: analyze.BrowseStateScanning, Navigable: true, Terminal: false,
			})
			onObservation(analyze.ChildObservation{
				Name: "growing", Path: `C:\growing`, Kind: analyze.BrowseKindDirectory,
				Bytes: 40, State: analyze.BrowseStateScanning, Navigable: true, Terminal: false,
			})
		}
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{Name: "growing", Path: `C:\growing`, Kind: analyze.BrowseKindDirectory, Bytes: 40, State: analyze.BrowseStateComplete, Navigable: true},
			},
		}
	})

	model := loadAnalyzeDrive(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter must start browse stream")
	}
	// First message starts the stream; next should surface scanning before terminal load.
	started := cmd()
	if _, ok := started.(analyzeBrowseStartedMsg); !ok {
		t.Fatalf("got %T, want analyzeBrowseStartedMsg", started)
	}
	next, cmd = model.Update(started)
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("started must continue stream wait")
	}
	obsMsg := cmd()
	if _, ok := obsMsg.(analyzeBrowseObservationMsg); !ok {
		t.Fatalf("got %T, want observation", obsMsg)
	}
	next, cmd = model.Update(obsMsg)
	model = next.(rootModel)
	content := model.content()
	if !strings.Contains(content, "scanning") {
		t.Fatalf("streaming scan must show scanning state:\n%s", content)
	}
	if !strings.Contains(content, "growing") {
		t.Fatalf("streaming scan must show path identity:\n%s", content)
	}
	// Drain remaining stream to terminal complete.
	model = drainAnalyzeBrowse(t, model, cmd)
	if !strings.Contains(model.content(), "complete") {
		t.Fatalf("final state should be complete:\n%s", model.content())
	}
}

func TestAnalyzeBrowseExactPercentOnlyWhenLocationComplete(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{Name: "a", Path: `C:\a`, Kind: analyze.BrowseKindFile, Bytes: 25, FileCount: 1, State: analyze.BrowseStateComplete},
				{Name: "b", Path: `C:\b`, Kind: analyze.BrowseKindFile, Bytes: 75, FileCount: 1, State: analyze.BrowseStateComplete},
			},
		}
	})
	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	content := model.content()
	// Exact integer percent allowed; no approximate markers required.
	if !strings.Contains(content, "25%") && !strings.Contains(content, "75%") {
		// cleanFormatBytes path still shows percent via FormatSharePercent.
		t.Fatalf("complete location should show exact percentages:\n%s", content)
	}
	if strings.Contains(content, "~25%") || strings.Contains(content, "~75%") {
		t.Fatalf("exact complete location must not force approx on complete children:\n%s", content)
	}
	// Scanning-style approx wording must not appear for complete-only inventory.
	row := renderAnalyzeBrowseRow(analyze.BrowseChild{
		Name: "a", Kind: analyze.BrowseKindFile, Bytes: 25, State: analyze.BrowseStateComplete,
	}, 100, true)
	if strings.Contains(row, ">=") {
		t.Fatalf("complete size must not use >=: %s", row)
	}
	if !strings.Contains(row, "25%") || strings.Contains(row, "~") {
		t.Fatalf("exact percent row = %q", row)
	}
	// Incomplete child percent never ">=N%".
	inc := renderAnalyzeBrowseRow(analyze.BrowseChild{
		Name: "x", Kind: analyze.BrowseKindDirectory, Bytes: 40, State: analyze.BrowseStateIncomplete, Navigable: true,
	}, 100, false)
	if !strings.Contains(inc, ">=") {
		t.Fatalf("incomplete bytes need >=: %s", inc)
	}
	if strings.Contains(inc, ">=%") || strings.Contains(strings.ReplaceAll(inc, ">=", ""), "BUG") {
		t.Fatalf("bad percent marker: %s", inc)
	}
	// After removing size token, the percent segment must not start with >=.
	if strings.Contains(inc, ">=40%") || strings.Contains(inc, ">= 40%") {
		t.Fatalf("percent must not use >=: %s", inc)
	}
	if !strings.Contains(inc, "observed") && !strings.Contains(inc, "~") {
		t.Fatalf("incomplete percent must be approximate: %s", inc)
	}
}
