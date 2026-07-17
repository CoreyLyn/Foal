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
	"github.com/CoreyLyn/Foal/internal/purge"
)

func TestPurgeCommandIsKnownAndListedInHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("help exit = %d stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "purge") {
		t.Fatalf("help missing purge command:\n%s", out)
	}
	if !strings.Contains(out, "foal purge") {
		t.Fatalf("help missing purge example:\n%s", out)
	}
}

func TestPurgeRequiresExplicitRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d; stdout=%q stderr=%q", code, exitUsage, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "explicit root") {
		t.Fatalf("stderr = %q, want explicit root error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestPurgeJSONRequiresExplicitRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--json"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	var env struct {
		Command string `json:"command"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("invalid error JSON: %v\n%s", err, stderr.String())
	}
	if env.Command != "purge" || env.Error == nil || env.Error.Code != "invalid_purge_invocation" {
		t.Fatalf("error envelope = %#v", env)
	}
	if !strings.Contains(env.Error.Message, "explicit root") {
		t.Fatalf("message = %q", env.Error.Message)
	}
}

func TestPurgeRejectsMissingRootPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-root")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--json", missing}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d stderr=%q stdout=%q", code, exitUsage, stderr.String(), stdout.String())
	}
	var env struct {
		Command string       `json:"command"`
		Result  purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.Command != "purge" || env.Result.Status != purge.StatusError {
		t.Fatalf("envelope = %#v", env)
	}
	if len(env.Result.Candidates) != 0 {
		t.Fatalf("candidates on error = %#v", env.Result.Candidates)
	}
}

func TestPurgeDryRunJSONContractWithFixtureTree(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "pkg", "index.js"): "aaa",
		filepath.Join("app", "src", "main.ts"):                  "src",
		filepath.Join("lib", "target", "release", "x"):          "bbbb",
		filepath.Join("lib", "node_modules_backup", "x"):        "nope",
		filepath.Join("legacy", "bin", "tool"):                  "bin",
		filepath.Join("legacy", "obj", "a"):                     "obj",
		filepath.Join("web", "dist", "out.js"):                  "ccccc",
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--json", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}

	var env struct {
		Command string       `json:"command"`
		Result  purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.Command != "purge" {
		t.Fatalf("command = %q", env.Command)
	}
	result := env.Result
	if result.Status != purge.StatusPreview || result.Mode != purge.ModeDryRun {
		t.Fatalf("status/mode = %q %q", result.Status, result.Mode)
	}
	if result.Root == "" {
		t.Fatal("root empty")
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("candidates = %#v, want node_modules/target/dist", result.Candidates)
	}
	byKind := map[string]purge.Candidate{}
	for _, c := range result.Candidates {
		byKind[c.Kind] = c
		if c.Path == "" || c.RelativePath == "" {
			t.Fatalf("candidate missing paths: %#v", c)
		}
		if c.Kind == "bin" || c.Kind == "obj" || strings.Contains(c.RelativePath, "node_modules_backup") {
			t.Fatalf("non-allowlisted candidate: %#v", c)
		}
	}
	if byKind["node_modules"].Bytes != 3 || byKind["target"].Bytes != 4 || byKind["dist"].Bytes != 5 {
		t.Fatalf("bytes by kind = %#v", byKind)
	}
	if result.Totals.CandidateCount != 3 || result.Totals.Bytes != 12 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}

func TestPurgeHumanOutputListsKindPathBytes(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "a"): "xy",
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Foal purge", "dry-run", "node_modules", "2", "No changes were made"} {
		if !strings.Contains(out, want) {
			t.Fatalf("human output missing %q:\n%s", want, out)
		}
	}
}

func TestPurgeAcceptsExplicitDryRunFlag(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--dry-run", "--json", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	var env struct {
		Result purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Result.Status != purge.StatusPreview || env.Result.Mode != purge.ModeDryRun {
		t.Fatalf("result = %#v", env.Result)
	}
}

func TestPurgeRejectsExecuteInThisSlice(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--execute", t.TempDir()}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "execute is not available") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPurgeRejectsMultiRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "a", "b"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "exactly one explicit root") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCleanDryRunStillHasNoProjectArtifactCandidates(t *testing.T) {
	// Regression: purge must not feed Clean Potential space / candidates.
	disableHistoryRecording(t)
	originalDryRun := dryRunClean
	defer func() { dryRunClean = originalDryRun }()

	dryRunClean = func(context.Context, clean.Options) clean.Result {
		return clean.Result{
			Status:     "preview",
			Mode:       "dry_run",
			Candidates: nil,
			Totals:     clean.Totals{},
		}
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"clean", "--dry-run", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	result := readResultObject(t, stdout.Bytes())
	if cands, ok := result["candidates"].([]interface{}); ok && len(cands) != 0 {
		t.Fatalf("clean candidates = %#v, want none for project artifacts", cands)
	}
	// Ensure no purge-shaped project artifact fields leaked into clean.
	for _, forbidden := range []string{"node_modules", "project_artifact", "purge"} {
		// "purge" may appear only if clean embeds help text; JSON result should not.
		raw, _ := json.Marshal(result)
		if strings.Contains(string(raw), "node_modules") || strings.Contains(string(raw), "project_artifact") {
			t.Fatalf("clean dry-run JSON mentions project artifacts: %s", raw)
		}
		_ = forbidden
	}
	if strings.Contains(stdout.String(), "node_modules") {
		t.Fatalf("clean dry-run JSON contains node_modules:\n%s", stdout.String())
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
