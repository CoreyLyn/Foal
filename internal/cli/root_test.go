package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
)

func TestHelpUsesFoalNamingOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--help"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"Foal", "foal", "foal.exe"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"Wole", "wole", "Mole for Windows"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("help output contains forbidden legacy text %q:\n%s", forbidden, output)
		}
	}
}

func TestKnownCommandRoutesAsJSON(t *testing.T) {
	disableHistoryRecording(t)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	var got envelope
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if got.Command != "clean" {
		t.Fatalf("command = %q, want clean", got.Command)
	}
	result, ok := got.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result has type %T, want object", got.Result)
	}
	if result["status"] != "preview" {
		t.Fatalf("result.status = %v, want preview", result["status"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestStatusJSONReportsReadOnlySystemSnapshot(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"status", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var got envelope
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if got.Command != "status" {
		t.Fatalf("command = %q, want status", got.Command)
	}
	result, ok := got.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result has type %T, want object", got.Result)
	}
	for _, key := range []string{"disk", "os", "foal", "elapsed_ms", "skipped", "errors"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("result missing %q: %#v", key, result)
		}
	}
	if result["status"] != "ok" {
		t.Fatalf("result.status = %v, want ok", result["status"])
	}
	foal, ok := result["foal"].(map[string]interface{})
	if !ok {
		t.Fatalf("result.foal has type %T, want object", result["foal"])
	}
	if foal["name"] != "Foal" || foal["command"] != "foal" || foal["executable"] != "foal.exe" {
		t.Fatalf("foal state = %#v, want Foal/foal/foal.exe naming", foal)
	}
	encoded := stdout.String()
	for _, forbidden := range []string{"Wole", "wole"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("status JSON contains forbidden legacy text %q:\n%s", forbidden, encoded)
		}
	}
	osInfo, ok := result["os"].(map[string]interface{})
	if !ok {
		t.Fatalf("result.os has type %T, want object", result["os"])
	}
	if osInfo["goos"] == "" || osInfo["goarch"] == "" {
		t.Fatalf("os state = %#v, want goos and goarch", osInfo)
	}
	disk, ok := result["disk"].(map[string]interface{})
	if !ok {
		t.Fatalf("result.disk has type %T, want object", result["disk"])
	}
	if disk["path"] == "" {
		t.Fatalf("disk state = %#v, want path", disk)
	}
	if _, ok := result["elapsed_ms"].(float64); !ok {
		t.Fatalf("elapsed_ms has type %T, want number", result["elapsed_ms"])
	}
	if _, ok := result["skipped"].([]interface{}); !ok {
		t.Fatalf("skipped has type %T, want array", result["skipped"])
	}
	if _, ok := result["errors"].([]interface{}); !ok {
		t.Fatalf("errors has type %T, want array", result["errors"])
	}
}

func TestAnalyzeJSONReportsDirectoryInsight(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "beta.txt"), []byte("beta-data"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"analyze", "--json", root}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "ok" {
		t.Fatalf("result.status = %v, want ok", result["status"])
	}
	for _, key := range []string{"root", "totals", "top_children", "skipped", "elapsed_ms"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("result missing %q: %#v", key, result)
		}
	}
	totals := result["totals"].(map[string]interface{})
	if totals["file_count"] != float64(2) {
		t.Fatalf("totals.file_count = %v, want 2", totals["file_count"])
	}
	if totals["directory_count"] != float64(2) {
		t.Fatalf("totals.directory_count = %v, want 2", totals["directory_count"])
	}
	if totals["bytes"] != float64(14) {
		t.Fatalf("totals.bytes = %v, want 14", totals["bytes"])
	}
	topChildren := result["top_children"].([]interface{})
	if len(topChildren) == 0 {
		t.Fatal("top_children is empty")
	}
}

func TestAnalyzeJSONReportsReparsePointsAsSkipped(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "inside.txt"), []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"analyze", "--json", root}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	skipped := result["skipped"].([]interface{})
	if len(skipped) == 0 {
		t.Fatalf("skipped is empty; result=%#v", result)
	}
	first := skipped[0].(map[string]interface{})
	if first["reason"] != "reparse_point" {
		t.Fatalf("skipped[0].reason = %v, want reparse_point", first["reason"])
	}
}

func TestAnalyzeJSONReportsMissingRootAsSkippedWithStableExit(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"analyze", "--json", missing}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	skipped := result["skipped"].([]interface{})
	if len(skipped) != 1 {
		t.Fatalf("len(skipped) = %d, want 1; result=%#v", len(skipped), result)
	}
	first := skipped[0].(map[string]interface{})
	if first["reason"] != "not_found" {
		t.Fatalf("skipped[0].reason = %v, want not_found", first["reason"])
	}
}

func TestUninstallJSONReportsPreviewOnlyReviewContract(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"uninstall", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "preview" {
		t.Fatalf("result.status = %v, want preview", result["status"])
	}
	for _, key := range []string{
		"applications",
		"evidence_sources",
		"possible_leftovers",
		"shared_state_concerns",
		"unknown_state",
		"skipped",
		"execution",
	} {
		if _, ok := result[key]; !ok {
			t.Fatalf("result missing %q: %#v", key, result)
		}
	}
	execution, ok := result["execution"].(map[string]interface{})
	if !ok {
		t.Fatalf("execution has type %T, want object", result["execution"])
	}
	if execution["allowed"] != false {
		t.Fatalf("execution.allowed = %v, want false", execution["allowed"])
	}
	actions, ok := execution["actions"].([]interface{})
	if !ok {
		t.Fatalf("execution.actions has type %T, want array", execution["actions"])
	}
	if len(actions) != 0 {
		t.Fatalf("execution.actions = %#v, want empty", actions)
	}
}

func TestCommandSpecificArgumentsAreRouted(t *testing.T) {
	disableHistoryRecording(t)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Foal clean") {
		t.Fatalf("stdout = %q, want routed clean output", stdout.String())
	}
}

func TestCleanRequiresDryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--json"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("Run returned %d, want %d", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var got envelope
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON error: %v\n%s", err, stderr.String())
	}
	if got.Error == nil || got.Error.Code != "invalid_clean_invocation" {
		t.Fatalf("error = %+v, want invalid_clean_invocation", got.Error)
	}
}

func TestCleanExecuteJSONRoutesConfirmedExecution(t *testing.T) {
	disableHistoryRecording(t)
	originalExecute := executeClean
	defer func() { executeClean = originalExecute }()

	called := false
	executeClean = func(ctx context.Context, opts clean.Options) clean.Result {
		called = true
		return clean.Result{
			Status:     "ok",
			Mode:       "execute",
			Candidates: []clean.CandidatePreview{},
			Deleted: []clean.DeletedItem{{
				Path:  `C:\Users\corey\AppData\Local\Temp\foal-owned.tmp`,
				Bytes: 5,
				Rule:  "foal_owned_temp_sandboxes",
			}},
			Skipped: []clean.SkippedItem{},
			Errors:  []clean.StructuredIssue{},
			Totals: clean.Totals{
				CandidateCount: 1,
				DeletedCount:   1,
				AffectedBytes:  5,
			},
		}
	}

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--execute", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !called {
		t.Fatal("executeClean was not called")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if result["status"] != "ok" || result["mode"] != "execute" {
		t.Fatalf("result status/mode = %v/%v, want ok/execute", result["status"], result["mode"])
	}
	deleted := result["deleted"].([]interface{})
	if len(deleted) != 1 {
		t.Fatalf("deleted = %#v, want one item", deleted)
	}
}

func TestCleanDryRunCreatesHistoryWithCommandParameters(t *testing.T) {
	original := newHistoryRecorder
	historyDir := t.TempDir()
	newHistoryRecorder = func() (history.Recorder, error) {
		recorder := history.NewFileRecorder(historyDir)
		return recorder, nil
	}
	t.Cleanup(func() { newHistoryRecorder = original })

	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	files, err := os.ReadDir(historyDir)
	if err != nil {
		t.Fatalf("read history dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("history files = %v, want one session file", files)
	}
	data, err := os.ReadFile(filepath.Join(historyDir, files[0].Name()))
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("history file is empty")
	}
	var record history.Record
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode session record: %v\n%s", err, lines[0])
	}
	if record.Type != "session" || record.Session == nil {
		t.Fatalf("record = %#v, want session", record)
	}
	if record.Session.Command.Command != "clean" || record.Session.Mode != "dry_run" {
		t.Fatalf("session = %#v, want clean dry_run", record.Session)
	}
	if got := record.Session.Command.Args; len(got) != 3 || got[0] != "clean" || got[1] != "--dry-run" || got[2] != "--json" {
		t.Fatalf("args = %#v, want original clean invocation args", got)
	}
}

func TestUnknownCommandJSONErrorShape(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--json", "missing"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("Run returned %d, want %d", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var got envelope
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON error: %v\n%s", err, stderr.String())
	}
	if got.Command != "missing" {
		t.Fatalf("command = %q, want missing", got.Command)
	}
	if got.Error == nil {
		t.Fatal("error is nil")
	}
	if got.Error.Code != "unknown_command" {
		t.Fatalf("error.code = %q, want unknown_command", got.Error.Code)
	}
	if got.Error.Message == "" {
		t.Fatal("error.message is empty")
	}
	if !got.Error.Recoverable {
		t.Fatal("error.recoverable = false, want true")
	}
	if got.Error.Command != "missing" {
		t.Fatalf("error.command = %q, want missing", got.Error.Command)
	}
}

func TestNoWoleCompatibilityCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--json", "wole"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("Run returned %d, want %d", code, exitUsage)
	}
	var got envelope
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON error: %v\n%s", err, stderr.String())
	}
	if got.Error == nil || got.Error.Code != "unknown_command" {
		t.Fatalf("error = %+v, want unknown_command", got.Error)
	}
}

func readResultObject(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()

	var got envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(data))
	}
	result, ok := got.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result has type %T, want object", got.Result)
	}
	return result
}

func disableHistoryRecording(t *testing.T) {
	t.Helper()
	original := newHistoryRecorder
	newHistoryRecorder = func() (history.Recorder, error) {
		return nil, nil
	}
	t.Cleanup(func() { newHistoryRecorder = original })
}
