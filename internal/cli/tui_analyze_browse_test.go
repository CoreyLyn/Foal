package cli

import (
	"context"
	"fmt"
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

func TestAnalyzeBrowseResponsiveRankedPresentation(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{
			Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true,
			Label: "System", FileSystem: "NTFS",
			TotalBytes: 1 << 30, FreeBytes: 1 << 29, HasCapacity: true,
		},
	})
	longName := strings.Repeat("LongChildName", 6)
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{
					Name: longName, Path: `C:\` + longName, Kind: analyze.BrowseKindDirectory,
					Bytes: 60, State: analyze.BrowseStateComplete, Navigable: true,
				},
				{
					Name: "partial-dir", Path: `C:\partial-dir`, Kind: analyze.BrowseKindDirectory,
					Bytes: 30, State: analyze.BrowseStatePartial, Navigable: true,
					SkipAggregates: []analyze.SkipAggregate{{Reason: analyze.SkipReasonPermissionDenied, Count: 2}},
				},
				{
					Name: "pagefile.sys", Path: `C:\pagefile.sys`, Kind: analyze.BrowseKindFile,
					Bytes: 10, State: analyze.BrowseStateComplete, Hidden: true, System: true,
				},
			},
		}
	})

	// Wide
	model := loadAnalyzeDrive(t)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = next.(rootModel)
	model = enterAnalyzeBrowse(t, model)
	wide := model.content()
	for _, want := range []string{
		"Location: C:\\",
		"Volume C:",
		"capacity",
		"free",
		"not a sum of child logical bytes",
		"Observed logical children",
		"ranked by observed logical bytes",
		"1.",
		"█",
		"directory",
		"complete",
		"hidden",
		"system",
		"Detail:",
		"bytes=",
		"partial",
	} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide content missing %q:\n%s", want, wide)
		}
	}
	for _, banned := range []string{"reclaimable", "allocated", "physically", "freed space"} {
		if strings.Contains(strings.ToLower(wide), banned) {
			t.Fatalf("wide content must not claim %q:\n%s", banned, wide)
		}
	}

	// Medium: kind dropped before state.
	next, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	model = next.(rootModel)
	medium := model.content()
	// Scan medium child lines for kind token as a column (between name and state).
	for _, line := range strings.Split(medium, "\n") {
		if !strings.Contains(line, "partial-dir") && !strings.Contains(line, "pagefile") && !strings.Contains(line, "LongChild") && !strings.Contains(line, "…") {
			continue
		}
		// Child rows start with cursor chrome.
		trim := strings.TrimLeft(line, " ")
		if !strings.HasPrefix(line, "> ") && !strings.HasPrefix(line, "  ") {
			continue
		}
		if strings.Contains(line, "directory") || strings.Contains(line, " · file · ") {
			t.Fatalf("medium layout should hide kind before state: %s", line)
		}
		if !strings.Contains(line, "complete") && !strings.Contains(line, "partial") {
			t.Fatalf("medium must keep state: %s", line)
		}
		_ = trim
	}

	// Narrow: cursor, name, size, state; no bar.
	next, _ = model.Update(tea.WindowSizeMsg{Width: 50, Height: 40})
	model = next.(rootModel)
	narrow := model.content()
	if strings.Contains(narrow, "█") {
		t.Fatalf("narrow must drop bar:\n%s", narrow)
	}
	if !strings.Contains(narrow, ">") {
		t.Fatalf("narrow keeps cursor:\n%s", narrow)
	}
	if !strings.Contains(narrow, "complete") && !strings.Contains(narrow, "partial") {
		t.Fatalf("narrow keeps state:\n%s", narrow)
	}
	// Long name truncated before size/state lost.
	var longLine string
	for _, line := range strings.Split(narrow, "\n") {
		if strings.Contains(line, "LongChild") || strings.Contains(line, "…") && strings.Contains(line, "complete") {
			longLine = line
			break
		}
	}
	if longLine == "" {
		// Name may truncate to ellipsis-only prefix; still require a complete-state row.
		for _, line := range strings.Split(narrow, "\n") {
			if strings.Contains(line, "complete") && (strings.HasPrefix(strings.TrimLeft(line, " "), ">") || strings.HasPrefix(line, "> ") || strings.HasPrefix(line, "  ")) {
				longLine = line
				if strings.Contains(line, "…") || strings.Contains(line, "Long") {
					break
				}
			}
		}
	}
	if longLine == "" || !strings.Contains(longLine, "complete") {
		t.Fatalf("long name row must retain state under narrow width:\n%s", narrow)
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
	// One-decimal percent; no approximate ~ markers required.
	if !strings.Contains(content, "25.0%") && !strings.Contains(content, "75.0%") {
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
	if !strings.Contains(row, "25.0%") || strings.Contains(row, "~") {
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
	// Percent is one-decimal (or <0.1%); no ~ / "observed" suffix.
	if !strings.Contains(inc, "40.0%") {
		t.Fatalf("incomplete row must still show percent: %s", inc)
	}
	if strings.Contains(inc, "~") {
		t.Fatalf("percent must not use ~: %s", inc)
	}
}

func TestAnalyzeBrowseSelectionBoundToPathThroughRerank(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	// Stream observations that invert rank: small first, then large overtakes.
	// Selection starts on "small"; after re-rank it must stay on small's path.
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		if onObservation != nil {
			onObservation(analyze.ChildObservation{
				Name: "small", Path: `C:\small`, Kind: analyze.BrowseKindDirectory,
				Bytes: 10, State: analyze.BrowseStateScanning, Navigable: true, Terminal: false,
			})
			onObservation(analyze.ChildObservation{
				Name: "large", Path: `C:\large`, Kind: analyze.BrowseKindDirectory,
				Bytes: 5, State: analyze.BrowseStateScanning, Navigable: true, Terminal: false,
			})
			// large grows past small — row order changes; selection path must not.
			onObservation(analyze.ChildObservation{
				Name: "large", Path: `C:\large`, Kind: analyze.BrowseKindDirectory,
				Bytes: 100, State: analyze.BrowseStateScanning, Navigable: true, Terminal: false,
			})
		}
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: analyze.RankBrowseChildren([]analyze.BrowseChild{
				{Name: "large", Path: `C:\large`, Kind: analyze.BrowseKindDirectory, Bytes: 100, State: analyze.BrowseStateComplete, Navigable: true},
				{Name: "small", Path: `C:\small`, Kind: analyze.BrowseKindDirectory, Bytes: 10, State: analyze.BrowseStateComplete, Navigable: true},
			}),
		}
	})

	model := loadAnalyzeDrive(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter must start browse")
	}
	started := cmd()
	next, cmd = model.Update(started)
	model = next.(rootModel)

	// Apply first observation (small).
	obs1 := cmd()
	next, cmd = model.Update(obs1)
	model = next.(rootModel)
	if model.analyze.browseSelectedPath != `C:\small` {
		t.Fatalf("selected after first obs = %q, want C:\\small", model.analyze.browseSelectedPath)
	}

	// second observation (large small bytes)
	obs2 := cmd()
	next, cmd = model.Update(obs2)
	model = next.(rootModel)
	// third observation (large overtakes)
	obs3 := cmd()
	next, cmd = model.Update(obs3)
	model = next.(rootModel)

	if model.analyze.browseSelectedPath != `C:\small` {
		t.Fatalf("selection must stay on path after re-rank: %q", model.analyze.browseSelectedPath)
	}
	// Ranked list: large first, small second — cursor path still small.
	if len(model.analyze.browseChildren) < 2 {
		t.Fatalf("children = %#v", model.analyze.browseChildren)
	}
	if model.analyze.browseChildren[0].Path != `C:\large` {
		t.Fatalf("rank0 = %q, want large", model.analyze.browseChildren[0].Path)
	}
	// View marks the selected path, not row 0.
	content := model.content()
	if !strings.Contains(content, "> small") && !strings.Contains(content, "> small ·") {
		// Row format is "> name · kind ..."
		lines := strings.Split(content, "\n")
		var smallLine, largeLine string
		for _, line := range lines {
			if strings.Contains(line, "small") && strings.Contains(line, "directory") {
				smallLine = line
			}
			if strings.Contains(line, "large") && strings.Contains(line, "directory") {
				largeLine = line
			}
		}
		if !strings.HasPrefix(strings.TrimLeft(smallLine, " "), ">") && !strings.HasPrefix(smallLine, "> ") {
			t.Fatalf("selected small row must be marked; small=%q large=%q\n%s", smallLine, largeLine, content)
		}
		if strings.HasPrefix(largeLine, "> ") {
			t.Fatalf("large must not steal selection marker: %q", largeLine)
		}
	}

	// Move selection to large, then re-rank with final load; Enter must open large.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if model.analyze.browseSelectedPath != `C:\large` {
		t.Fatalf("after j selected = %q", model.analyze.browseSelectedPath)
	}
	// Drain to terminal result.
	model = drainAnalyzeBrowse(t, model, cmd)
	if model.analyze.browseSelectedPath != `C:\large` {
		t.Fatalf("selection after final rank = %q, want large", model.analyze.browseSelectedPath)
	}

	// Enter opens the path selected before/through re-ranking (large).
	var entered []string
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		entered = append(entered, root)
		return analyze.BrowseResult{OK: true, Root: root, Children: nil}
	})
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter large must browse")
	}
	model = drainAnalyzeBrowse(t, model, cmd)
	if len(entered) != 1 || entered[0] != `C:\large` {
		t.Fatalf("enter targets = %#v, want C:\\large", entered)
	}
}

func TestAnalyzeBrowseFocusPublishedToBrowseOptions(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})
	var sawFocus bool
	var focusAtCall string
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		sawFocus = opts.Focus != nil
		if opts.Focus != nil {
			focusAtCall = opts.Focus.FocusedPath()
		}
		return analyze.BrowseResult{
			OK:   true,
			Root: `C:\`,
			Children: []analyze.BrowseChild{
				{Name: "a", Path: `C:\a`, Kind: analyze.BrowseKindDirectory, Bytes: 1, State: analyze.BrowseStateComplete, Navigable: true},
				{Name: "b", Path: `C:\b`, Kind: analyze.BrowseKindDirectory, Bytes: 2, State: analyze.BrowseStateComplete, Navigable: true},
			},
		}
	})
	model := loadAnalyzeDrive(t)
	model = enterAnalyzeBrowse(t, model)
	if !sawFocus {
		t.Fatal("browse options must carry Focus for next-slot promotion")
	}
	_ = focusAtCall
	// Moving focus publishes the selected path for promotion.
	if model.analyze.browseFocus == nil {
		t.Fatal("model must hold browseFocus")
	}
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := model.analyze.browseFocus.FocusedPath(); got != model.analyze.browseSelectedPath {
		t.Fatalf("focus path = %q, selected = %q", got, model.analyze.browseSelectedPath)
	}
}

func TestAnalyzeBrowseViewportFollowsSelectedPath(t *testing.T) {
	// Build a tall ranked list and select a path that would fall off-screen
	// after re-ranking; viewport offset must keep the selected path visible.
	m := newAnalyzeDriveModel(80, 20)
	m.phase = analyzePhaseBrowse
	m.browseRoot = `C:\`
	// 30 children ranked by bytes descending.
	children := make([]analyze.BrowseChild, 0, 30)
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("item-%02d", i)
		children = append(children, analyze.BrowseChild{
			Name: name, Path: `C:\` + name, Kind: analyze.BrowseKindFile,
			Bytes: int64(30 - i), State: analyze.BrowseStateComplete,
		})
	}
	m.browseChildren = children
	// Select a path near the bottom.
	m.browseSelectedPath = `C:\item-25`
	m.syncBrowseSelectionAfterRank()
	if m.browseOffset > analyze.IndexOfBrowsePath(m.browseChildren, m.browseSelectedPath) {
		t.Fatalf("offset %d past selected", m.browseOffset)
	}
	sel := analyze.IndexOfBrowsePath(m.browseChildren, m.browseSelectedPath)
	if sel < m.browseOffset || sel >= m.browseOffset+m.browseViewportRows() {
		t.Fatalf("selected idx %d not in viewport [%d,%d)", sel, m.browseOffset, m.browseOffset+m.browseViewportRows())
	}
	// Re-rank after inflating item-25 to the top; path selection stays, viewport follows.
	for i := range m.browseChildren {
		if m.browseChildren[i].Path == `C:\item-25` {
			m.browseChildren[i].Bytes = 10_000
		}
	}
	m.browseChildren = analyze.RankBrowseChildren(m.browseChildren)
	m.syncBrowseSelectionAfterRank()
	if m.browseSelectedPath != `C:\item-25` {
		t.Fatalf("path lost after re-rank: %q", m.browseSelectedPath)
	}
	sel = analyze.IndexOfBrowsePath(m.browseChildren, m.browseSelectedPath)
	if sel != 0 {
		t.Fatalf("item-25 should be rank 0 after inflate, got %d", sel)
	}
	if sel < m.browseOffset || sel >= m.browseOffset+m.browseViewportRows() {
		t.Fatalf("viewport lost selected after re-rank: idx=%d offset=%d vis=%d", sel, m.browseOffset, m.browseViewportRows())
	}
}
