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

func TestPurgeRejectsDryRunAndExecuteTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--dry-run", "--execute", t.TempDir()}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "either --dry-run or --execute") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPurgeExecuteWithoutAllowPermanentSkipsWithStructuredReason(t *testing.T) {
	disableHistoryRecording(t)
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "a"): "xy",
	})
	nm := filepath.Join(root, "app", "node_modules")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--execute", "--json", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	var env struct {
		Command string       `json:"command"`
		Result  purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.Command != "purge" || env.Result.Mode != purge.ModeExecute {
		t.Fatalf("envelope = %#v", env)
	}
	if len(env.Result.Deleted) != 0 || env.Result.Totals.PermanentlyDeletedBytes != 0 {
		t.Fatalf("must delete nothing without auth: %#v", env.Result)
	}
	found := false
	for _, s := range env.Result.Skipped {
		if s.Reason == purge.IssuePermanentDeletionNotAuthorized {
			found = true
			if s.PlannedAction != purge.PlannedActionDeletePermanently {
				t.Fatalf("planned action changed: %#v", s)
			}
		}
	}
	if !found {
		t.Fatalf("skipped = %#v, want permanent_deletion_not_authorized", env.Result.Skipped)
	}
	if _, err := os.Lstat(nm); err != nil {
		t.Fatalf("artifact must remain: %v", err)
	}
}

func TestPurgeExecuteWithAllowPermanentDeletesAndRecordsHistory(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	// Rebind history factory so default recorder picks up the test dir.
	originalRecorder := newHistoryRecorder
	newHistoryRecorder = func() (history.Recorder, error) {
		return history.NewFileRecorder(historyDir), nil
	}
	t.Cleanup(func() { newHistoryRecorder = originalRecorder })

	root := t.TempDir()
	writeTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "a"): "xyz",
		filepath.Join("web", "dist", "b"):         "ww",
	})
	nm := filepath.Join(root, "app", "node_modules")
	dist := filepath.Join(root, "web", "dist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--execute", "--allow-permanent", "--json", root}, &stdout, &stderr)
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
	result := env.Result
	if result.Status != purge.StatusOK || result.Mode != purge.ModeExecute {
		t.Fatalf("status/mode = %q/%q", result.Status, result.Mode)
	}
	if len(result.Deleted) != 2 {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	for _, d := range result.Deleted {
		if d.Action != purge.PlannedActionDeletePermanently {
			t.Fatalf("action = %#v", d)
		}
	}
	if result.Totals.PermanentlyDeletedBytes != 5 { // "xyz"+"ww"
		t.Fatalf("permanently_deleted_bytes = %d", result.Totals.PermanentlyDeletedBytes)
	}
	if len(result.Notices) == 0 || !strings.Contains(strings.Join(result.Notices, "\n"), "reinstalling") {
		t.Fatalf("notices = %#v", result.Notices)
	}
	if _, err := os.Lstat(nm); !os.IsNotExist(err) {
		t.Fatalf("node_modules still present: %v", err)
	}
	if _, err := os.Lstat(dist); !os.IsNotExist(err) {
		t.Fatalf("dist still present: %v", err)
	}

	// History provenance distinguishes purge from Clean.
	query := history.NewFileQuery(historyDir)
	hist := query.Recent(context.Background())
	if len(hist.Sessions) != 1 {
		t.Fatalf("history sessions = %#v", hist.Sessions)
	}
	sess := hist.Sessions[0]
	if sess.Command.Command != "purge" {
		t.Fatalf("command = %#v, want purge (not clean)", sess.Command)
	}
	if !strings.HasPrefix(sess.ID, "purge-") {
		t.Fatalf("session id = %q", sess.ID)
	}
	if sess.Mode != purge.ModeExecute || sess.Aggregate.DeletedCount != 2 {
		t.Fatalf("session = %#v", sess)
	}
	if sess.Aggregate.PermanentlyDeletedBytes != 5 {
		t.Fatalf("aggregate permanently_deleted_bytes = %d", sess.Aggregate.PermanentlyDeletedBytes)
	}
}

func TestPurgeExecuteWiresAllowPermanentFlag(t *testing.T) {
	disableHistoryRecording(t)
	original := executePurge
	var captured purge.Options
	executePurge = func(_ context.Context, opts purge.Options) purge.Result {
		captured = opts
		return purge.Result{
			Status:  purge.StatusOK,
			Mode:    purge.ModeExecute,
			Root:    opts.Root,
			Roots:   append([]string(nil), opts.Roots...),
			Deleted: []purge.DeletedItem{},
			Failed:  []purge.FailedItem{},
			Skipped: []purge.Skipped{},
			Totals:  purge.Totals{},
		}
	}
	t.Cleanup(func() { executePurge = original })

	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--execute", "--allow-permanent", "--json", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if !captured.AllowPermanentDeletion {
		t.Fatal("AllowPermanentDeletion not set from --allow-permanent")
	}
	if len(captured.Roots) != 1 || captured.Roots[0] != root {
		t.Fatalf("roots = %#v, want [%q]", captured.Roots, root)
	}
	if captured.CommandParameters.Command != "purge" {
		t.Fatalf("options = %#v", captured)
	}
}

func TestPurgeExecuteHumanOutputMentionsRebuildCost(t *testing.T) {
	disableHistoryRecording(t)
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "a"): "x",
	})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--execute", "--allow-permanent", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Foal purge", "Permanently deleted", "reinstalling", "irreversible"} {
		if !strings.Contains(out, want) {
			t.Fatalf("human output missing %q:\n%s", want, out)
		}
	}
}

func TestPurgeDefaultRemainsDryRunNonMutating(t *testing.T) {
	disableHistoryRecording(t)
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "a"): "keep",
	})
	nm := filepath.Join(root, "app", "node_modules")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--json", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	var env struct {
		Result purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Result.Mode != purge.ModeDryRun || env.Result.Status != purge.StatusPreview {
		t.Fatalf("result = %#v", env.Result)
	}
	if len(env.Result.Deleted) != 0 {
		t.Fatalf("default path must not delete: %#v", env.Result.Deleted)
	}
	if _, err := os.Lstat(nm); err != nil {
		t.Fatalf("must remain: %v", err)
	}
}

func TestPurgeMultiRootDryRunJSONContract(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTree(t, rootA, map[string]string{
		filepath.Join("app", "node_modules", "a"): "aaa",
	})
	writeTree(t, rootB, map[string]string{
		filepath.Join("lib", "dist", "b"): "bbbb",
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--json", rootA, rootB}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	var env struct {
		Command string       `json:"command"`
		Result  purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.Command != "purge" || env.Result.Status != purge.StatusPreview {
		t.Fatalf("envelope = %#v", env)
	}
	if len(env.Result.Roots) != 2 {
		t.Fatalf("roots = %#v", env.Result.Roots)
	}
	if env.Result.Root != "" {
		t.Fatalf("single root field should be empty for multi-root: %q", env.Result.Root)
	}
	if len(env.Result.Candidates) != 2 {
		t.Fatalf("candidates = %#v", env.Result.Candidates)
	}
}

func TestPurgeRejectsDangerousRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--json", `C:\Windows`}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	var env struct {
		Command string       `json:"command"`
		Result  purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.Result.Status != purge.StatusError {
		t.Fatalf("result = %#v", env.Result)
	}
	if len(env.Result.Candidates) != 0 {
		t.Fatalf("candidates on dangerous root: %#v", env.Result.Candidates)
	}
	msg := strings.ToLower(env.Result.Message)
	if !strings.Contains(msg, "dangerous_root") && !strings.Contains(msg, "system") {
		t.Fatalf("message = %q", env.Result.Message)
	}
}

func TestPurgeProtectionOmitsProtectedArtifact(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		filepath.Join("open", "node_modules", "a"):   "aa",
		filepath.Join("locked", "node_modules", "b"): "bbb",
	})
	protectionFile := filepath.Join(t.TempDir(), "protection.txt")
	protected := filepath.Join(root, "locked")
	if err := os.WriteFile(protectionFile, []byte(protected+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOAL_PROTECTION_FILE", protectionFile)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"purge", "--json", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	var env struct {
		Result purge.Result `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want only open node_modules", env.Result.Candidates)
	}
	if strings.Contains(strings.ToLower(env.Result.Candidates[0].Path), "locked") {
		t.Fatalf("protected candidate leaked: %#v", env.Result.Candidates[0])
	}
	found := false
	for _, s := range env.Result.Skipped {
		if s.Reason == purge.IssueProtectedPath {
			found = true
			if s.Path != "" {
				t.Fatalf("protected skip path leak: %#v", s)
			}
		}
	}
	if !found {
		t.Fatalf("skipped = %#v, want protected_path", env.Result.Skipped)
	}
}

func TestHelpDocumentsPurgeExecuteAndAllowPermanent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	out := stdout.String()
	for _, want := range []string{
		"foal purge",
		"--execute",
		"--allow-permanent",
		"reinstall/rebuild",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
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
