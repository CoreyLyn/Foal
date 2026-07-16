package history_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		Aggregate: history.AggregateOutcomes{
			CandidateCount:           1,
			OpportunityCount:         2,
			CandidateBytes:           5,
			OpportunityObservedBytes: 4096,
		},
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
	if records[0].Session.Aggregate.OpportunityCount != 2 || records[0].Session.Aggregate.OpportunityObservedBytes != 4096 {
		t.Fatalf("aggregate = %#v, want opportunity totals", records[0].Session.Aggregate)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "session-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"opportunity_count":2`) ||
		!strings.Contains(string(raw), `"opportunity_observed_bytes":4096`) {
		t.Fatalf("encoded history = %s, want additive opportunity fields", raw)
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

func TestQueryReadsAggregateRecordsWithAndWithoutOpportunityFields(t *testing.T) {
	dir := t.TempDir()
	older := `{"type":"session","session":{"id":"older","command_parameters":{"command":"clean","args":["clean","--dry-run"]},"started_at":"2026-06-03T09:00:00Z","ended_at":"2026-06-03T09:00:01Z","mode":"dry_run","aggregate_outcomes":{"candidate_count":1,"deleted_count":0,"skipped_count":0,"error_count":0,"candidate_bytes":5,"affected_bytes":0}}}` + "\n"
	newer := `{"type":"session","session":{"id":"newer","command_parameters":{"command":"clean","args":["clean","--dry-run"]},"started_at":"2026-06-03T10:00:00Z","ended_at":"2026-06-03T10:00:01Z","mode":"dry_run","aggregate_outcomes":{"candidate_count":1,"deleted_count":0,"skipped_count":0,"error_count":0,"opportunity_count":2,"candidate_bytes":5,"opportunity_observed_bytes":4096,"affected_bytes":0}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "older.jsonl"), []byte(older), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "newer.jsonl"), []byte(newer), 0600); err != nil {
		t.Fatal(err)
	}

	result := history.NewFileQuery(dir).Recent(context.Background())

	if result.Status != "ok" || len(result.Sessions) != 2 {
		t.Fatalf("result = %#v, want two readable sessions", result)
	}
	if result.Sessions[0].ID != "newer" ||
		result.Sessions[0].Aggregate.OpportunityCount != 2 ||
		result.Sessions[0].Aggregate.OpportunityObservedBytes != 4096 {
		t.Fatalf("newer aggregate = %#v, want opportunity totals", result.Sessions[0].Aggregate)
	}
	if result.Sessions[1].ID != "older" ||
		result.Sessions[1].Aggregate.OpportunityCount != 0 ||
		result.Sessions[1].Aggregate.OpportunityObservedBytes != 0 {
		t.Fatalf("older aggregate = %#v, want additive fields to decode as zero", result.Sessions[1].Aggregate)
	}
	// Older History without recycle_bin_moved_bytes / permanently_deleted_bytes
	// remains readable with zero action-split fields.
	if result.Sessions[1].Aggregate.RecycleBinMovedBytes != 0 ||
		result.Sessions[1].Aggregate.PermanentlyDeletedBytes != 0 {
		t.Fatalf("older action totals = %#v, want zero for missing fields", result.Sessions[1].Aggregate)
	}
}

func TestQueryReadsActionSplitAggregateFields(t *testing.T) {
	dir := t.TempDir()
	payload := `{"type":"session","session":{"id":"split","command_parameters":{"command":"clean","args":["clean","--execute"]},"started_at":"2026-06-03T10:00:00Z","ended_at":"2026-06-03T10:00:01Z","mode":"execute","aggregate_outcomes":{"candidate_count":0,"deleted_count":1,"skipped_count":0,"error_count":0,"candidate_bytes":0,"recycle_bin_moved_bytes":12,"permanently_deleted_bytes":0,"affected_bytes":12}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "split.jsonl"), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}

	result := history.NewFileQuery(dir).Recent(context.Background())
	if result.Status != "ok" || len(result.Sessions) != 1 {
		t.Fatalf("result = %#v", result)
	}
	agg := result.Sessions[0].Aggregate
	if agg.RecycleBinMovedBytes != 12 || agg.PermanentlyDeletedBytes != 0 || agg.AffectedBytes != 12 {
		t.Fatalf("aggregate = %#v, want action-aware split", agg)
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

func TestFileRecorderSerializesOptionalTUIProvenanceWithoutCLIArgs(t *testing.T) {
	dir := t.TempDir()
	recorder := history.NewFileRecorder(dir)
	session := history.SessionRecord{
		ID: "tui-exact",
		Command: history.CommandParameters{
			Command:            "clean",
			Surface:            "tui",
			SelectionMode:      "exact",
			SelectedCategories: []string{"foal_owned_temp_sandboxes", "crash_dumps"},
		},
		Mode:      "execute",
		StartedAt: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 7, 15, 10, 0, 1, 0, time.UTC),
		Aggregate: history.AggregateOutcomes{DeletedCount: 1, AffectedBytes: 8},
	}
	if err := recorder.Record(context.Background(), session, []history.ItemRecord{{
		Path:   filepath.Join(dir, "deleted.tmp"),
		Rule:   "crash_dumps",
		Action: "move_to_recycle_bin",
		Result: "deleted",
	}}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "tui-exact.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, want := range []string{
		`"surface":"tui"`,
		`"selection_mode":"exact"`,
		`"selected_categories":["foal_owned_temp_sandboxes","crash_dumps"]`,
	} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("encoded history missing %s:\n%s", want, encoded)
		}
	}
	records := readHistoryRecords(t, filepath.Join(dir, "tui-exact.jsonl"))
	if len(records) != 2 || records[0].Session == nil {
		t.Fatalf("records = %#v", records)
	}
	// Session record must omit synthetic CLI args; item paths stay authoritative.
	if len(records[0].Session.Command.Args) != 0 {
		t.Fatalf("TUI provenance must not synthesize CLI args: %#v", records[0].Session.Command.Args)
	}
	if records[0].Session.Command.Surface != "tui" ||
		records[0].Session.Command.SelectionMode != "exact" ||
		len(records[0].Session.Command.SelectedCategories) != 2 {
		t.Fatalf("session command = %#v", records[0].Session.Command)
	}
	if records[1].Item == nil || records[1].Item.Path == "" || records[1].Item.Result != "deleted" {
		t.Fatalf("item-level history must remain authoritative: %#v", records[1].Item)
	}
	for _, id := range records[0].Session.Command.SelectedCategories {
		if strings.ContainsAny(id, `/\`) {
			t.Fatalf("selected category path-bearing: %q", id)
		}
	}
	// Session JSON line must not invent CLI execute flags.
	sessionLine := strings.SplitN(encoded, "\n", 2)[0]
	for _, forbidden := range []string{`"args"`, "--opt-in", "--execute"} {
		if strings.Contains(sessionLine, forbidden) {
			t.Fatalf("session provenance contains %q:\n%s", forbidden, sessionLine)
		}
	}
}

func TestQueryReadsOlderHistoryWithoutTUIProvenanceAndCLIKeepsArgs(t *testing.T) {
	dir := t.TempDir()
	// Older session without surface/selection_mode/selected_categories.
	older := `{"type":"session","session":{"id":"legacy","command_parameters":{"command":"clean","args":["clean","--execute"]},"started_at":"2026-06-03T09:00:00Z","ended_at":"2026-06-03T09:00:01Z","mode":"execute","aggregate_outcomes":{"candidate_count":0,"deleted_count":1,"skipped_count":0,"error_count":0,"candidate_bytes":0,"affected_bytes":4}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "legacy.jsonl"), []byte(older), 0600); err != nil {
		t.Fatal(err)
	}
	// CLI-shaped session with args and no TUI provenance.
	cli := history.SessionRecord{
		ID: "cli",
		Command: history.CommandParameters{
			Command: "clean",
			Args:    []string{"clean", "--execute", "--opt-in", "crash_dumps"},
		},
		Mode:      "execute",
		StartedAt: time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 7, 15, 11, 0, 1, 0, time.UTC),
		Aggregate: history.AggregateOutcomes{DeletedCount: 1, AffectedBytes: 4},
	}
	if err := history.NewFileRecorder(dir).Record(context.Background(), cli, nil); err != nil {
		t.Fatal(err)
	}

	result := history.NewFileQuery(dir).Recent(context.Background())
	if result.Status != "ok" || len(result.Sessions) != 2 {
		t.Fatalf("result = %#v", result)
	}
	var legacy, cliSession *history.SessionResult
	for i := range result.Sessions {
		switch result.Sessions[i].ID {
		case "legacy":
			legacy = &result.Sessions[i]
		case "cli":
			cliSession = &result.Sessions[i]
		}
	}
	if legacy == nil || cliSession == nil {
		t.Fatalf("sessions = %#v", result.Sessions)
	}
	if legacy.Command.Surface != "" || legacy.Command.SelectionMode != "" || len(legacy.Command.SelectedCategories) != 0 {
		t.Fatalf("legacy gained TUI fields: %#v", legacy.Command)
	}
	if len(legacy.Command.Args) != 2 || legacy.Command.Args[0] != "clean" {
		t.Fatalf("legacy args = %#v", legacy.Command.Args)
	}
	if cliSession.Command.Surface != "" || len(cliSession.Command.Args) != 4 {
		t.Fatalf("CLI session provenance/args broken: %#v", cliSession.Command)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "cli.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"surface"`) || strings.Contains(string(raw), `"selection_mode"`) {
		t.Fatalf("CLI session must omit empty TUI provenance fields:\n%s", raw)
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
