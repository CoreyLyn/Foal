package history_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/history"
)

// TestFileRecorderRoundTripsServicingOperationsSeparateFromItems verifies the
// four required History servicing projections persist on the session (not as
// file items), round-trip through FileQuery, and stay path-free and byte-free.
func TestFileRecorderRoundTripsServicingOperationsSeparateFromItems(t *testing.T) {
	dir := t.TempDir()
	recorder := history.NewFileRecorder(dir)
	exit0 := 0
	exit3017 := 3017
	servicing := []history.ServicingRecord{
		{Category: "winsxs_component_store", PlannedAction: "invoke_windows_servicing", Capability: "execute_component_store_cleanup", ReclaimablePackages: 4, CleanupRecommended: true, Outcome: "completed", ExitCode: &exit0},
		{Category: "winsxs_component_store", PlannedAction: "invoke_windows_servicing", Capability: "execute_component_store_cleanup", Outcome: "skipped", Reason: "windows_servicing_not_authorized"},
		{Category: "winsxs_component_store", PlannedAction: "invoke_windows_servicing", Capability: "execute_component_store_cleanup", ReclaimablePackages: 4, CleanupRecommended: true, Outcome: "failed", Reason: "windows_servicing_cleanup_failed", ExitCode: &exit3017, RestartRequired: true},
		{Category: "winsxs_component_store", PlannedAction: "invoke_windows_servicing", Capability: "execute_component_store_cleanup", Outcome: "canceled", Reason: "context_canceled"},
	}
	session := history.SessionRecord{
		ID:                  "clean-execute-servicing",
		Command:             history.CommandParameters{Command: "clean", Args: []string{"clean", "--execute"}},
		Mode:                "execute",
		StartedAt:           time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC),
		EndedAt:             time.Date(2026, 7, 20, 10, 0, 5, 0, time.UTC),
		Aggregate:           history.AggregateOutcomes{ServicingOperationCount: len(servicing)},
		ServicingOperations: servicing,
	}

	if err := recorder.Record(context.Background(), session, nil); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	result := history.NewFileQuery(dir).Recent(context.Background())
	if result.Status != "ok" || len(result.Sessions) != 1 {
		t.Fatalf("query result = %#v", result)
	}
	got := result.Sessions[0]
	if len(got.Items) != 0 {
		t.Fatalf("servicing must not be stored as file items: %#v", got.Items)
	}
	if got.Aggregate.ServicingOperationCount != 4 {
		t.Fatalf("aggregate servicing count = %d, want 4", got.Aggregate.ServicingOperationCount)
	}
	if len(got.ServicingOperations) != 4 {
		t.Fatalf("servicing records = %d, want 4", len(got.ServicingOperations))
	}
	success := got.ServicingOperations[0]
	if success.Outcome != "completed" || success.ReclaimablePackages != 4 || !success.CleanupRecommended ||
		success.ExitCode == nil || *success.ExitCode != 0 || success.RestartRequired {
		t.Fatalf("success projection lost fields: %#v", success)
	}
	if got.ServicingOperations[1].Reason != "windows_servicing_not_authorized" || got.ServicingOperations[1].ExitCode != nil {
		t.Fatalf("authorization skip projection lost fields: %#v", got.ServicingOperations[1])
	}
	fail := got.ServicingOperations[2]
	if fail.Outcome != "failed" || fail.ExitCode == nil || *fail.ExitCode != 3017 || !fail.RestartRequired {
		t.Fatalf("failure projection lost fields: %#v", fail)
	}
	if got.ServicingOperations[3].Outcome != "canceled" || got.ServicingOperations[3].Reason != "context_canceled" {
		t.Fatalf("cancellation projection lost fields: %#v", got.ServicingOperations[3])
	}

	raw, err := os.ReadFile(filepath.Join(dir, "clean-execute-servicing.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	blob := string(raw)
	if !strings.Contains(blob, `"servicing_operations"`) || !strings.Contains(blob, `"servicing_operation_count":4`) {
		t.Fatalf("encoded history missing servicing contract: %s", blob)
	}
	// The category identifier is path-free; only real paths/tool output are banned.
	for _, forbidden := range []string{`C:\\`, `\\WinSxS`, `dism`, `"path"`, `"bytes"`, `stdout`, `stderr`} {
		if strings.Contains(blob, forbidden) {
			t.Fatalf("servicing history leaked %q: %s", forbidden, blob)
		}
	}
}

// TestQueryReadsHistoryWithoutServicingUnchanged confirms older deletion-only
// sessions decode with an empty servicing list and zero count.
func TestQueryReadsHistoryWithoutServicingUnchanged(t *testing.T) {
	dir := t.TempDir()
	older := `{"type":"session","session":{"id":"older","command_parameters":{"command":"clean","args":["clean","--dry-run"]},"started_at":"2026-06-03T09:00:00Z","ended_at":"2026-06-03T09:00:01Z","mode":"dry_run","aggregate_outcomes":{"candidate_count":1,"candidate_bytes":5,"affected_bytes":0}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "older.jsonl"), []byte(older), 0600); err != nil {
		t.Fatal(err)
	}
	result := history.NewFileQuery(dir).Recent(context.Background())
	if len(result.Sessions) != 1 {
		t.Fatalf("sessions = %#v", result.Sessions)
	}
	if result.Sessions[0].Aggregate.ServicingOperationCount != 0 || len(result.Sessions[0].ServicingOperations) != 0 {
		t.Fatalf("older session should have no servicing: %#v", result.Sessions[0])
	}
}
