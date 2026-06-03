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
