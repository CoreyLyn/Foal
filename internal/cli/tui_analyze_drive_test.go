package cli

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func stubAnalyzeVolumes(t *testing.T, volumes []analyze.LocalVolume) {
	t.Helper()
	original := listAnalyzeLocalVolumes
	listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
		return append([]analyze.LocalVolume(nil), volumes...)
	}
	t.Cleanup(func() { listAnalyzeLocalVolumes = original })
}

func openAnalyzeDrive(t *testing.T) (rootModel, tea.Cmd) {
	t.Helper()
	model := newRootModel()
	next, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = next.(rootModel)
	// Analyze is the third main-menu item (Clean, Uninstall, Analyze).
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(rootModel), cmd
}

func loadAnalyzeDrive(t *testing.T) rootModel {
	t.Helper()
	model, cmd := openAnalyzeDrive(t)
	if model.screen != screenAnalyzeDrive {
		t.Fatalf("screen = %v, want screenAnalyzeDrive", model.screen)
	}
	if cmd == nil {
		t.Fatal("opening Analyze must return a volume load command")
	}
	loaded, ok := cmd().(analyzeVolumesLoadedMsg)
	if !ok {
		t.Fatalf("load cmd produced %T, want analyzeVolumesLoadedMsg", cmd())
	}
	next, _ := model.Update(loaded)
	return next.(rootModel)
}

func TestRootModelAnalyzeOpensDriveEntry(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{
			Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true,
			Label: "System", FileSystem: "NTFS", TotalBytes: 1024 * 1024 * 1024, FreeBytes: 512 * 1024 * 1024, HasCapacity: true,
		},
		{
			Root: `E:\`, Letter: "E:", Kind: analyze.VolumeKindRemovable, Available: true,
			Label: "USB", FileSystem: "FAT32", TotalBytes: 1024 * 1024, FreeBytes: 256 * 1024, HasCapacity: true,
		},
	})

	model := loadAnalyzeDrive(t)
	content := model.content()
	for _, want := range []string{
		"Analyze TUI",
		"Local drive entry",
		"read-only",
		"> C:",
		"System",
		"NTFS",
		"fixed",
		"  E:",
		"USB",
		"FAT32",
		"removable",
		"until you enter a drive",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("drive entry missing %q:\n%s", want, content)
		}
	}
	// Must not open the generic command viewer / path-edit surface.
	for _, forbidden := range []string{"e edit path", "Path: (default: current directory)", "Loading analyze view"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("drive entry still shows generic viewer affordance %q:\n%s", forbidden, content)
		}
	}
}

func TestAnalyzeDriveEntryFocusesCWhenPresent(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `D:\`, Letter: "D:", Kind: analyze.VolumeKindFixed, Available: true},
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
		{Root: `E:\`, Letter: "E:", Kind: analyze.VolumeKindRemovable, Available: true},
	})

	model := loadAnalyzeDrive(t)
	if model.analyze.cursor != 1 {
		t.Fatalf("cursor = %d, want C: at index 1", model.analyze.cursor)
	}
	if !strings.Contains(model.content(), "> C:") {
		t.Fatalf("content should focus C:\n%s", model.content())
	}
}

func TestAnalyzeDriveEntryFocusesFirstAvailableWithoutC(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `D:\`, Letter: "D:", Kind: analyze.VolumeKindFixed, Available: false},
		{Root: `E:\`, Letter: "E:", Kind: analyze.VolumeKindRemovable, Available: true},
		{Root: `F:\`, Letter: "F:", Kind: analyze.VolumeKindFixed, Available: true},
	})

	model := loadAnalyzeDrive(t)
	if model.analyze.cursor != 1 {
		t.Fatalf("cursor = %d, want first available E: at 1", model.analyze.cursor)
	}
	if !strings.Contains(model.content(), "> E:") {
		t.Fatalf("content should focus E:\n%s", model.content())
	}
}

func TestAnalyzeDriveEntryListsUnavailableAndBlocksEnter(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true, HasCapacity: true, TotalBytes: 10, FreeBytes: 2},
		{Root: `F:\`, Letter: "F:", Kind: analyze.VolumeKindRemovable, Available: false, Label: "Empty"},
	})

	model := loadAnalyzeDrive(t)
	// Move focus to unavailable F:
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if model.analyze.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", model.analyze.cursor)
	}
	content := model.content()
	if !strings.Contains(content, "F:") || !strings.Contains(content, "[unavailable]") {
		t.Fatalf("unavailable drive missing from list:\n%s", content)
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(model.content(), "unavailable and cannot be entered") {
		t.Fatalf("enter on unavailable must show block notice:\n%s", model.content())
	}
}

func TestAnalyzeDriveEntryEnterOnAvailableStartsBrowseNotVolumeRescan(t *testing.T) {
	var listCalls int
	var browseRoots []string
	original := listAnalyzeLocalVolumes
	listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
		listCalls++
		return []analyze.LocalVolume{
			{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true, HasCapacity: true, TotalBytes: 100, FreeBytes: 40},
		}
	}
	t.Cleanup(func() { listAnalyzeLocalVolumes = original })

	origBrowse := browseAnalyzeLocation
	browseAnalyzeLocation = func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
		browseRoots = append(browseRoots, root)
		return analyze.BrowseResult{
			OK:   true,
			Root: root,
			Children: []analyze.BrowseChild{
				{Name: "file.txt", Path: root + `file.txt`, Kind: analyze.BrowseKindFile, Bytes: 4, State: analyze.BrowseStateComplete},
			},
		}
	}
	t.Cleanup(func() { browseAnalyzeLocation = origBrowse })

	model := loadAnalyzeDrive(t)
	if listCalls != 1 {
		t.Fatalf("list calls after open = %d, want 1", listCalls)
	}

	// Enter starts on-demand browse of the selected drive only; does not re-list volumes.
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if listCalls != 1 {
		t.Fatalf("enter must not re-list volumes; calls = %d", listCalls)
	}
	if cmd == nil {
		t.Fatal("enter available drive must return browse command")
	}
	model = drainAnalyzeBrowse(t, model, cmd)
	if len(browseRoots) != 1 || browseRoots[0] != `C:\` {
		t.Fatalf("browse roots = %#v, want only C:\\", browseRoots)
	}
	content := model.content()
	if !strings.Contains(content, "file.txt") {
		t.Fatalf("browse should list direct children:\n%s", content)
	}
	for _, forbidden := range []string{"Top children", "Totals:", "Loading analyze view", "Rescanning"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("enter must not open legacy directory insight scan: found %q\n%s", forbidden, content)
		}
	}
}

func TestAnalyzeDriveEntryRefreshReenumerates(t *testing.T) {
	calls := 0
	original := listAnalyzeLocalVolumes
	listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
		calls++
		if calls == 1 {
			return []analyze.LocalVolume{
				{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
			}
		}
		return []analyze.LocalVolume{
			{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
			{Root: `E:\`, Letter: "E:", Kind: analyze.VolumeKindRemovable, Available: true, Label: "NewUSB"},
		}
	}
	t.Cleanup(func() { listAnalyzeLocalVolumes = original })

	model := loadAnalyzeDrive(t)
	if calls != 1 {
		t.Fatalf("open calls = %d, want 1", calls)
	}
	if strings.Contains(model.content(), "E:") {
		t.Fatal("first load should not list E:")
	}

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("r must return a reload command")
	}
	if !strings.Contains(model.content(), "Refreshing drives") && !strings.Contains(model.content(), "Loading local drives") {
		t.Fatalf("refresh should show loading state:\n%s", model.content())
	}
	loaded, ok := cmd().(analyzeVolumesLoadedMsg)
	if !ok {
		t.Fatalf("reload produced %T", cmd())
	}
	next, _ = model.Update(loaded)
	model = next.(rootModel)
	if calls != 2 {
		t.Fatalf("after refresh calls = %d, want 2", calls)
	}
	if !strings.Contains(model.content(), "E:") || !strings.Contains(model.content(), "NewUSB") {
		t.Fatalf("refresh should include new drive:\n%s", model.content())
	}
}

func TestAnalyzeDriveEntryEscReturnsToMenuAndQQuits(t *testing.T) {
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true},
	})

	model := loadAnalyzeDrive(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(rootModel)
	if cmd != nil {
		if msg := cmd(); msg == (tea.QuitMsg{}) {
			t.Fatal("esc must return to menu, not quit")
		}
	}
	if model.screen != screenMenu {
		t.Fatalf("esc screen = %v, want screenMenu", model.screen)
	}
	if !strings.Contains(model.content(), "Foal main menu") {
		t.Fatalf("esc should show main menu:\n%s", model.content())
	}

	model = loadAnalyzeDrive(t)
	_, cmd = model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q must return a quit command")
	}
	if got := cmd(); got != (tea.QuitMsg{}) {
		t.Fatalf("q cmd = %#v, want QuitMsg", got)
	}
}

func TestAnalyzeDriveEntryOpeningPerformsZeroRecursiveScans(t *testing.T) {
	// Contract: load path only calls the volume list seam, never analyze.Run
	// directory measurement. We assert the load message is volumes-only and the
	// rendered body has no Top children / Totals scan report.
	stubAnalyzeVolumes(t, []analyze.LocalVolume{
		{Root: `C:\`, Letter: "C:", Kind: analyze.VolumeKindFixed, Available: true, Label: "OS", FileSystem: "NTFS", HasCapacity: true, TotalBytes: 2000, FreeBytes: 500},
	})

	model, cmd := openAnalyzeDrive(t)
	if model.screen != screenAnalyzeDrive {
		t.Fatalf("screen = %v", model.screen)
	}
	msg := cmd()
	loaded, ok := msg.(analyzeVolumesLoadedMsg)
	if !ok {
		t.Fatalf("msg type %T, want analyzeVolumesLoadedMsg (not viewerLoadedMsg)", msg)
	}
	if _, isViewer := msg.(viewerLoadedMsg); isViewer {
		t.Fatal("opening Analyze must not load generic viewer body")
	}
	if len(loaded.volumes) != 1 {
		t.Fatalf("volumes = %#v", loaded.volumes)
	}
	next, _ := model.Update(loaded)
	content := next.(rootModel).content()
	for _, forbidden := range []string{"Top children", "Totals:", "project_artifact_clue", "Elapsed:"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("drive entry must not run directory insight report: found %q\n%s", forbidden, content)
		}
	}
}

func TestStatusAndHistoryStillUseGenericViewer(t *testing.T) {
	// Opening Status (index 3) and History (index 4) must still use screenViewer.
	model := newRootModel()
	next, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = next.(rootModel)

	// Status
	for i := 0; i < 3; i++ {
		model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if model.screen != screenViewer || model.viewer.command != "status" {
		t.Fatalf("Status screen=%v command=%q, want screenViewer/status", model.screen, model.viewer.command)
	}
	if cmd == nil {
		t.Fatal("Status must load viewer body")
	}

	// Back to menu then History. Selected remains on Status (index 3); one down -> History.
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.screen != screenMenu {
		t.Fatalf("esc from status screen=%v", model.screen)
	}
	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	next, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(rootModel)
	if model.screen != screenViewer || model.viewer.command != "history" {
		t.Fatalf("History screen=%v command=%q, want screenViewer/history", model.screen, model.viewer.command)
	}
	if cmd == nil {
		t.Fatal("History must load viewer body")
	}
}

func TestRenderAnalyzeDriveRowFormatsMetadata(t *testing.T) {
	row := renderAnalyzeDriveRow(analyze.LocalVolume{
		Letter: "C:", Label: "System", Kind: analyze.VolumeKindFixed,
		FileSystem: "NTFS", HasCapacity: true, TotalBytes: 1024 * 1024 * 1024, FreeBytes: 512 * 1024 * 1024, Available: true,
	})
	for _, want := range []string{"C:", "System", "fixed", "NTFS", "total", "free"} {
		if !strings.Contains(row, want) {
			t.Fatalf("row missing %q: %s", want, row)
		}
	}

	unavailable := renderAnalyzeDriveRow(analyze.LocalVolume{
		Letter: "F:", Kind: analyze.VolumeKindRemovable, Available: false,
	})
	if !strings.Contains(unavailable, "[unavailable]") {
		t.Fatalf("unavailable row: %s", unavailable)
	}
}
