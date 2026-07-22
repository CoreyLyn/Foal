package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func stubAnalyzeBrowse(t *testing.T, fn func(ctx context.Context, root string, opts analyze.BrowseOptions) analyze.BrowseResult) {
	t.Helper()
	original := browseAnalyzeLocation
	browseAnalyzeLocation = fn
	t.Cleanup(func() { browseAnalyzeLocation = original })
}

func enterAnalyzeBrowse(t *testing.T, model rootModel) rootModel {
	t.Helper()
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("enter available drive must start browse load")
	}
	msg := cmd()
	loaded, ok := msg.(analyzeBrowseLoadedMsg)
	if !ok {
		t.Fatalf("browse cmd produced %T, want analyzeBrowseLoadedMsg", msg)
	}
	next, _ = model.Update(loaded)
	return next.(rootModel)
}

func TestAnalyzeEnterDriveBrowsesDirectChildrenOnly(t *testing.T) {
	var roots []string
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true, HasCapacity: true, TotalBytes: 100, FreeBytes: 40},
		{Root: `E:\`, Letter: "E:", Kind: analyze.VolumeKindRemovable, Available: true, Label: "USB"},
	})
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions) analyze.BrowseResult {
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
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions) analyze.BrowseResult {
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
	loaded, ok := cmd().(analyzeBrowseLoadedMsg)
	if !ok {
		t.Fatalf("got %T", cmd())
	}
	next, _ = model.Update(loaded)
	model = next.(rootModel)
	if !strings.Contains(model.content(), `Location: C:\Users`) {
		t.Fatalf("want Users browse:\n%s", model.content())
	}

	// Esc → parent volume root
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("esc from nested must reload parent")
	}
	loaded, ok = cmd().(analyzeBrowseLoadedMsg)
	if !ok {
		t.Fatalf("got %T", cmd())
	}
	next, _ = model.Update(loaded)
	model = next.(rootModel)
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
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions) analyze.BrowseResult {
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
	loaded := cmd().(analyzeBrowseLoadedMsg)
	next, _ = model.Update(loaded)
	model = next.(rootModel)
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
	stubAnalyzeBrowse(t, func(ctx context.Context, root string, opts analyze.BrowseOptions) analyze.BrowseResult {
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
