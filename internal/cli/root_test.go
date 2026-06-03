package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	var stdout, stderr bytes.Buffer

	code := Run([]string{"status", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
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
	if result["status"] != "preview" {
		t.Fatalf("result.status = %v, want preview", result["status"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestCommandSpecificArgumentsAreRouted(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"clean", "--dry-run"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("Run returned %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Foal clean") {
		t.Fatalf("stdout = %q, want routed clean output", stdout.String())
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
	if _, ok := result["skipped"].([]interface{}); !ok {
		t.Fatalf("skipped has type %T, want array", result["skipped"])
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
