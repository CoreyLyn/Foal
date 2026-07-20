package cli

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/status"
)

func openViewer(t *testing.T, downs int) (rootModel, tea.Cmd) {
	t.Helper()
	model := newRootModel()
	next, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	model = next.(rootModel)
	for i := 0; i < downs; i++ {
		model = updateRootKeys(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(rootModel), cmd
}

func loadViewer(t *testing.T, downs int) rootModel {
	t.Helper()
	model, cmd := openViewer(t, downs)
	if cmd == nil {
		t.Fatal("opening a command view must return a load command")
	}
	loaded, ok := cmd().(viewerLoadedMsg)
	if !ok {
		t.Fatalf("load command produced %T, want viewerLoadedMsg", cmd())
	}
	next, _ := model.Update(loaded)
	return next.(rootModel)
}

func TestStatusViewerRendersSnapshot(t *testing.T) {
	original := statusCapture
	statusCapture = func() status.Snapshot {
		return status.Snapshot{
			Status: "ok",
			Disk: status.DiskSnapshot{
				Path:           `C:\`,
				TotalBytes:     1000,
				FreeBytes:      400,
				AvailableBytes: 400,
			},
			OS:        status.OSSnapshot{GOOS: "windows", GOARCH: "amd64"},
			Foal:      status.FoalSnapshot{Name: "Foal", Command: "foal", Executable: "foal.exe", Version: "dev"},
			ElapsedMS: 3,
			Skipped:   []status.StatusIssue{},
			Errors:    []status.StatusIssue{},
		}
	}
	t.Cleanup(func() { statusCapture = original })

	model := loadViewer(t, 3)

	content := model.content()
	for _, want := range []string{
		"Status TUI",
		"Read-only system and Foal state snapshot",
		"OS: windows/amd64",
		`Path: C:\`,
		"Total: 1000 bytes",
		"Skipped (0)",
		"Errors (0)",
		"Elapsed: 3 ms",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func TestViewerBackAndRefreshKeys(t *testing.T) {
	original := statusCapture
	statusCapture = func() status.Snapshot {
		return status.Snapshot{Status: "ok"}
	}
	t.Cleanup(func() { statusCapture = original })

	model := loadViewer(t, 3)

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = next.(rootModel)
	if cmd == nil {
		t.Fatal("r must return a reload command")
	}
	if !strings.Contains(model.content(), "Loading status view...") {
		t.Fatalf("refresh should re-enter the loading state:\n%s", model.content())
	}
	if loaded, ok := cmd().(viewerLoadedMsg); !ok || loaded.command != "status" {
		t.Fatalf("reload command produced %#v, want status viewerLoadedMsg", cmd())
	}

	model = updateRootKeys(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if !strings.Contains(model.content(), "Foal main menu") {
		t.Fatalf("b should return to the main menu:\n%s", model.content())
	}
}

func TestRenderHistoryReportListsSessions(t *testing.T) {
	report := renderHistoryReport(history.QueryResult{
		Status: "ok",
		Sessions: []history.SessionResult{{
			ID:        "clean-dry-run-20260610",
			Mode:      "dry_run",
			StartedAt: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 6, 10, 9, 0, 1, 0, time.UTC),
			Aggregate: history.AggregateOutcomes{CandidateCount: 3, SkippedCount: 1},
		}},
		Errors: []history.QueryIssue{{
			Code:    "history_record_malformed",
			Message: "invalid line",
			Path:    `C:\history\broken.jsonl`,
		}},
	})

	for _, want := range []string{
		"Status: ok",
		"Sessions (1)",
		"clean-dry-run-20260610",
		"mode: dry_run | candidates: 3 | deleted: 0 | skipped: 1 | errors: 0",
		"started: 2026-06-10 09:00:00 UTC",
		"Errors (1)",
		`history_record_malformed: invalid line (C:\history\broken.jsonl)`,
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRenderHistoryReportHandlesEmptyHistory(t *testing.T) {
	report := renderHistoryReport(history.QueryResult{Status: "ok"})

	for _, want := range []string{"Sessions (0)", "No recorded sessions.", "Errors (0)", "None reported."} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}
