package history_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/history"
)

func TestFileRecorderWritesSessionAndItemRecords(t *testing.T) {
	dir := t.TempDir()
	recorder := history.NewFileRecorder(dir)
	bytes := int64(5)

	session := history.SessionRecord{
		ID:        "session-1",
		Command:   history.CommandParameters{Command: "clean", Args: []string{"clean", "--dry-run"}},
		Mode:      "dry_run",
		StartedAt: time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 6, 3, 10, 0, 1, 0, time.UTC),
		Aggregate: history.AggregateOutcomes{CandidateCount: 1, CandidateBytes: 5},
	}
	items := []history.ItemRecord{{
		SessionID:     "session-1",
		Path:          filepath.Join(dir, "cache.tmp"),
		Rule:          "test_default_rule",
		PlannedAction: "move_to_recycle_bin",
		Bytes:         &bytes,
		Result:        "candidate",
	}}

	if err := recorder.Record(context.Background(), session, items); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	records := readHistoryRecords(t, filepath.Join(dir, "session-1.jsonl"))
	if len(records) != 2 {
		t.Fatalf("records = %#v, want session and item", records)
	}
	if records[0].Type != "session" || records[0].Session == nil {
		t.Fatalf("first record = %#v, want session record", records[0])
	}
	if records[0].Session.Command.Command != "clean" || records[0].Session.Mode != "dry_run" {
		t.Fatalf("session = %#v, want clean dry_run", records[0].Session)
	}
	if records[1].Type != "item" || records[1].Item == nil {
		t.Fatalf("second record = %#v, want item record", records[1])
	}
	if records[1].Item.Path != items[0].Path || records[1].Item.Rule != "test_default_rule" || records[1].Item.Result != "candidate" {
		t.Fatalf("item = %#v, want path/rule/candidate result", records[1].Item)
	}
}

func TestQueryReturnsEmptyHistoryForMissingDirectory(t *testing.T) {
	result := history.NewFileQuery(filepath.Join(t.TempDir(), "missing")).Recent(context.Background())

	if result.Status != "ok" {
		t.Fatalf("status = %q, want ok", result.Status)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want empty", result.Sessions)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %#v, want empty", result.Errors)
	}
}

func TestQueryReturnsRecentSessionsWithItemOutcomes(t *testing.T) {
	dir := t.TempDir()
	recorder := history.NewFileRecorder(dir)
	bytes := int64(42)

	older := history.SessionRecord{
		ID:        "older",
		Command:   history.CommandParameters{Command: "clean", Args: []string{"clean", "--dry-run"}},
		Mode:      "dry_run",
		StartedAt: time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 6, 3, 9, 0, 1, 0, time.UTC),
		Aggregate: history.AggregateOutcomes{CandidateCount: 1, CandidateBytes: bytes},
	}
	newer := history.SessionRecord{
		ID:        "newer",
		Command:   history.CommandParameters{Command: "clean", Args: []string{"clean", "--execute"}},
		Mode:      "execute",
		StartedAt: time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 6, 3, 10, 0, 1, 0, time.UTC),
		Aggregate: history.AggregateOutcomes{DeletedCount: 1, AffectedBytes: bytes},
	}
	if err := recorder.Record(context.Background(), older, []history.ItemRecord{{
		Path:          filepath.Join(dir, "candidate.tmp"),
		Rule:          "foal_owned_temp_sandboxes",
		PlannedAction: "move_to_recycle_bin",
		Bytes:         &bytes,
		Result:        "candidate",
	}}); err != nil {
		t.Fatalf("record older history: %v", err)
	}
	if err := recorder.Record(context.Background(), newer, []history.ItemRecord{{
		Path:   filepath.Join(dir, "deleted.tmp"),
		Rule:   "foal_owned_temp_sandboxes",
		Action: "move_to_recycle_bin",
		Bytes:  &bytes,
		Result: "deleted",
	}}); err != nil {
		t.Fatalf("record newer history: %v", err)
	}

	result := history.NewFileQuery(dir).Recent(context.Background())

	if result.Status != "ok" {
		t.Fatalf("status = %q, want ok; result=%#v", result.Status, result)
	}
	if len(result.Sessions) != 2 {
		t.Fatalf("sessions = %#v, want two sessions", result.Sessions)
	}
	if result.Sessions[0].ID != "newer" || result.Sessions[0].Mode != "execute" {
		t.Fatalf("first session = %#v, want newer execute session", result.Sessions[0])
	}
	if len(result.Sessions[0].Items) != 1 {
		t.Fatalf("newer items = %#v, want one item", result.Sessions[0].Items)
	}
	item := result.Sessions[0].Items[0]
	if item.Action != "move_to_recycle_bin" || item.Result != "deleted" || item.Bytes == nil || *item.Bytes != bytes {
		t.Fatalf("item = %#v, want deleted recycle-bin item with bytes", item)
	}
	if result.Sessions[1].Mode != "dry_run" || result.Sessions[1].Items[0].PlannedAction != "move_to_recycle_bin" {
		t.Fatalf("older session = %#v, want dry-run candidate", result.Sessions[1])
	}
}

func TestQueryReportsMalformedHistoryAsStructuredError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("{not-json\n"), 0600); err != nil {
		t.Fatal(err)
	}

	result := history.NewFileQuery(dir).Recent(context.Background())

	if result.Status != "partial" {
		t.Fatalf("status = %q, want partial", result.Status)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one structured error", result.Errors)
	}
	if result.Errors[0].Code != "history_decode_failed" || !result.Errors[0].Recoverable {
		t.Fatalf("error = %#v, want recoverable history_decode_failed", result.Errors[0])
	}
}

func readHistoryRecords(t *testing.T, path string) []history.Record {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open history file: %v", err)
	}
	defer file.Close()

	var records []history.Record
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record history.Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode record %q: %v", scanner.Text(), err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan history file: %v", err)
	}
	return records
}
