package purge_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/purge"
)

type recordingPermanentRemover struct {
	paths []string
	err   error
	// cancelAfter, when >0, cancels the context after that many Remove calls.
	cancelAfter int
	cancel      context.CancelFunc
	calls       atomic.Int32
}

func (r *recordingPermanentRemover) Remove(ctx context.Context, path string) error {
	r.paths = append(r.paths, path)
	if r.err != nil {
		r.calls.Add(1)
		return r.err
	}
	// Finish ordinary removal before cancel so mid-batch tests can assert a real
	// completed permanent delete followed by cooperative cancel of siblings.
	err := delete.FilesystemPermanentRemover{}.Remove(context.Background(), path)
	r.calls.Add(1)
	if r.cancel != nil && r.cancelAfter > 0 && int(r.calls.Load()) >= r.cancelAfter {
		r.cancel()
	}
	return err
}

type recordingHistoryRecorder struct {
	sessions []history.SessionRecord
	items    []history.ItemRecord
}

func (r *recordingHistoryRecorder) Record(_ context.Context, session history.SessionRecord, items []history.ItemRecord) error {
	r.sessions = append(r.sessions, session)
	r.items = append(r.items, items...)
	return nil
}

func writeArtifactTree(t *testing.T, root string, files map[string]string) {
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

func TestExecuteWithoutAllowPermanentSkipsAndDeletesNothing(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "pkg", "index.js"): "aaa",
		filepath.Join("lib", "target", "out"):                   "bbbb",
	})
	nm := filepath.Join(root, "app", "node_modules")
	target := filepath.Join(root, "lib", "target")

	remover := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	result := purge.Execute(context.Background(), purge.Options{
		Root:                   root,
		AllowPermanentDeletion: false,
		PermanentRemover:       remover,
		HistoryRecorder:        recorder,
		CommandParameters: history.CommandParameters{
			Command: "purge",
			Args:    []string{"purge", "--execute", root},
		},
	})

	if result.Status != purge.StatusOK || result.Mode != purge.ModeExecute {
		t.Fatalf("status/mode = %q/%q", result.Status, result.Mode)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if len(remover.paths) != 0 {
		t.Fatalf("remover called without auth: %v", remover.paths)
	}
	if len(result.Deleted) != 0 || result.Totals.PermanentlyDeletedBytes != 0 {
		t.Fatalf("deleted = %#v totals=%#v", result.Deleted, result.Totals)
	}
	authSkips := 0
	for _, s := range result.Skipped {
		if s.Reason == purge.IssuePermanentDeletionNotAuthorized {
			authSkips++
			if s.PlannedAction != purge.PlannedActionDeletePermanently {
				t.Fatalf("planned action changed on skip: %#v", s)
			}
		}
	}
	if authSkips != 2 {
		t.Fatalf("auth skips = %d in %#v, want 2", authSkips, result.Skipped)
	}
	if _, err := os.Lstat(nm); err != nil {
		t.Fatalf("node_modules must remain: %v", err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("target must remain: %v", err)
	}
	if len(recorder.sessions) != 1 || recorder.sessions[0].Command.Command != "purge" {
		t.Fatalf("history session = %#v", recorder.sessions)
	}
	if !strings.HasPrefix(recorder.sessions[0].ID, "purge-") {
		t.Fatalf("session id must distinguish purge: %q", recorder.sessions[0].ID)
	}
	for _, item := range recorder.items {
		if item.Result == "skipped" && item.SkippedReason != nil &&
			item.SkippedReason.Code == purge.IssuePermanentDeletionNotAuthorized {
			return
		}
	}
	t.Fatalf("history missing auth skip items: %#v", recorder.items)
}

func TestExecuteWithAllowPermanentDeletesFreshlyDiscoveredArtifacts(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "pkg", "index.js"): "aaa",
		filepath.Join("web", "dist", "out.js"):                  "ccccc",
		filepath.Join("src", "main.go"):                         "code",
	})
	nm := filepath.Join(root, "app", "node_modules")
	dist := filepath.Join(root, "web", "dist")

	// Dry-run first (paths must not be trusted alone by execute).
	preview := purge.DryRun(context.Background(), purge.Options{Root: root})
	if preview.Status != purge.StatusPreview || len(preview.Candidates) != 2 {
		t.Fatalf("preview = %#v", preview)
	}

	// Mutate tree after dry-run: remove dist, add target — execute must rediscover.
	if err := os.RemoveAll(dist); err != nil {
		t.Fatal(err)
	}
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("lib", "target", "x"): "yy",
	})
	target := filepath.Join(root, "lib", "target")

	remover := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	result := purge.Execute(context.Background(), purge.Options{
		Root:                   root,
		AllowPermanentDeletion: true,
		PermanentRemover:       remover,
		HistoryRecorder:        recorder,
		CommandParameters: history.CommandParameters{
			Command: "purge",
			Args:    []string{"purge", "--execute", "--allow-permanent", root},
		},
	})

	if result.Status != purge.StatusOK || result.Mode != purge.ModeExecute {
		t.Fatalf("status/mode = %q/%q message=%q", result.Status, result.Mode, result.Message)
	}
	if len(result.Deleted) != 2 {
		t.Fatalf("deleted = %#v, want node_modules+target from fresh discovery", result.Deleted)
	}
	deletedPaths := map[string]bool{}
	for _, d := range result.Deleted {
		deletedPaths[d.Path] = true
		if d.Action != purge.PlannedActionDeletePermanently {
			t.Fatalf("action = %q", d.Action)
		}
	}
	if !deletedPaths[nm] || !deletedPaths[target] {
		t.Fatalf("deleted paths = %#v, want fresh nm+target (not stale dist)", deletedPaths)
	}
	if _, err := os.Lstat(nm); !os.IsNotExist(err) {
		t.Fatalf("node_modules still exists: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists: %v", err)
	}
	if result.Totals.DeletedCount != 2 || result.Totals.PermanentlyDeletedBytes != 5 { // "aaa"+ "yy"
		t.Fatalf("totals = %#v", result.Totals)
	}
	if result.Totals.AffectedBytes != result.Totals.PermanentlyDeletedBytes {
		t.Fatalf("affected must equal permanently deleted: %#v", result.Totals)
	}
	if len(result.Notices) == 0 || !strings.Contains(strings.Join(result.Notices, "\n"), "reinstalling") {
		t.Fatalf("notices missing rebuild cost: %#v", result.Notices)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"action":"delete_permanently"`) {
		t.Fatalf("JSON missing permanent action: %s", body)
	}
	if strings.Contains(strings.ToLower(body), "secure erase") || strings.Contains(strings.ToLower(body), "shred") {
		t.Fatalf("must not claim secure erase: %s", body)
	}

	if len(recorder.sessions) != 1 {
		t.Fatalf("sessions = %#v", recorder.sessions)
	}
	sess := recorder.sessions[0]
	if sess.Command.Command != "purge" || sess.Mode != purge.ModeExecute {
		t.Fatalf("session = %#v", sess)
	}
	if !strings.HasPrefix(sess.ID, "purge-execute-") {
		t.Fatalf("session id = %q", sess.ID)
	}
	if sess.Aggregate.PermanentlyDeletedBytes != 5 || sess.Aggregate.DeletedCount != 2 {
		t.Fatalf("aggregate = %#v", sess.Aggregate)
	}
	deletedItems := 0
	for _, item := range recorder.items {
		if item.Result == "deleted" {
			deletedItems++
			if item.Action != purge.PlannedActionDeletePermanently {
				t.Fatalf("history item action = %#v", item)
			}
		}
	}
	if deletedItems != 2 {
		t.Fatalf("history deleted items = %d in %#v", deletedItems, recorder.items)
	}
}

func TestExecutePartialFailureIsHonestAndContinuesSiblings(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("a", "node_modules", "x"): "aa",
		filepath.Join("b", "dist", "y"):         "bbb",
	})
	nm := filepath.Join(root, "a", "node_modules")
	dist := filepath.Join(root, "b", "dist")

	calls := 0
	remover := permanentRemoverFunc(func(ctx context.Context, path string) error {
		calls++
		if path == nm {
			return errors.New("disk fault")
		}
		return delete.FilesystemPermanentRemover{}.Remove(ctx, path)
	})

	result := purge.Execute(context.Background(), purge.Options{
		Root:                   root,
		AllowPermanentDeletion: true,
		PermanentRemover:       remover,
	})

	if len(result.Failed) != 1 || result.Failed[0].Path != nm {
		t.Fatalf("failed = %#v", result.Failed)
	}
	if result.Failed[0].Reason.Code != purge.IssuePermanentDeleteFailed {
		t.Fatalf("fail code = %#v", result.Failed[0].Reason)
	}
	if !strings.Contains(result.Failed[0].Reason.Message, "may already be permanently deleted") {
		t.Fatalf("message = %q", result.Failed[0].Reason.Message)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Path != dist {
		t.Fatalf("deleted = %#v, sibling dist should succeed", result.Deleted)
	}
	if result.Totals.PermanentlyDeletedBytes != 3 || result.Totals.FailedCount != 1 {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if _, err := os.Lstat(dist); !os.IsNotExist(err) {
		t.Fatalf("dist should be gone: %v", err)
	}
	if calls != 2 {
		t.Fatalf("remover calls = %d, want both candidates attempted", calls)
	}
}

func TestExecuteCancelBeforeMutationDeletesNothing(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "x"): "data",
	})
	nm := filepath.Join(root, "app", "node_modules")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	remover := &recordingPermanentRemover{}
	result := purge.Execute(ctx, purge.Options{
		Root:                   root,
		AllowPermanentDeletion: true,
		PermanentRemover:       remover,
	})

	// Discovery sees canceled context first.
	if result.Status != purge.StatusCanceled {
		t.Fatalf("status = %q, want canceled", result.Status)
	}
	if len(result.Deleted) != 0 || len(remover.paths) != 0 {
		t.Fatalf("must not delete on pre-mutation cancel: deleted=%#v paths=%v", result.Deleted, remover.paths)
	}
	if _, err := os.Lstat(nm); err != nil {
		t.Fatalf("artifact must remain: %v", err)
	}
}

func TestExecuteCancelMidBatchRecordsPartialWithoutRollback(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("a", "node_modules", "x"): "aa",
		filepath.Join("b", "dist", "y"):         "bbb",
	})

	ctx, cancel := context.WithCancel(context.Background())
	remover := &recordingPermanentRemover{cancelAfter: 1, cancel: cancel}
	recorder := &recordingHistoryRecorder{}

	result := purge.Execute(ctx, purge.Options{
		Root:                   root,
		AllowPermanentDeletion: true,
		PermanentRemover:       remover,
		HistoryRecorder:        recorder,
		CommandParameters:      history.CommandParameters{Command: "purge"},
	})

	if len(result.Deleted) != 1 {
		t.Fatalf("deleted = %#v, want first candidate completed", result.Deleted)
	}
	canceledSkips := 0
	for _, s := range result.Skipped {
		if s.Reason == purge.IssueContextCanceled {
			canceledSkips++
		}
	}
	if canceledSkips != 1 {
		t.Fatalf("canceled skips = %d in %#v", canceledSkips, result.Skipped)
	}
	if result.Totals.PermanentlyDeletedBytes != result.Deleted[0].Bytes {
		t.Fatalf("totals = %#v", result.Totals)
	}
	if strings.Contains(strings.ToLower(result.Message), "rolled back") &&
		!strings.Contains(strings.ToLower(result.Message), "not rolled back") {
		t.Fatalf("must not promise rollback: %q", result.Message)
	}
	if len(recorder.sessions) != 1 || recorder.sessions[0].Aggregate.DeletedCount != 1 {
		t.Fatalf("history must record partial success: %#v", recorder.sessions)
	}
}

func TestExecuteRequiresExplicitRoot(t *testing.T) {
	result := purge.Execute(context.Background(), purge.Options{
		AllowPermanentDeletion: true,
	})
	if result.Status != purge.StatusError || result.Mode != purge.ModeExecute {
		t.Fatalf("status/mode = %q/%q", result.Status, result.Mode)
	}
	if !strings.Contains(result.Message, "explicit root") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestDryRunStillNonMutatingWithAllowPermanentFlag(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "x"): "zz",
	})
	nm := filepath.Join(root, "app", "node_modules")
	remover := &recordingPermanentRemover{}
	result := purge.DryRun(context.Background(), purge.Options{
		Root:                   root,
		AllowPermanentDeletion: true,
		PermanentRemover:       remover,
	})
	if result.Status != purge.StatusPreview || result.Mode != purge.ModeDryRun {
		t.Fatalf("status/mode = %q/%q", result.Status, result.Mode)
	}
	if len(remover.paths) != 0 || len(result.Deleted) != 0 {
		t.Fatal("dry-run must not mutate even with allow-permanent")
	}
	if _, err := os.Lstat(nm); err != nil {
		t.Fatalf("path must remain: %v", err)
	}
	if result.Candidates[0].PlannedAction != purge.PlannedActionDeletePermanently {
		t.Fatalf("planned_action = %q", result.Candidates[0].PlannedAction)
	}
}

func TestRenderExecuteReportMentionsRebuildCost(t *testing.T) {
	report := purge.RenderExecuteReport(purge.Result{
		Status:  purge.StatusOK,
		Mode:    purge.ModeExecute,
		Root:    `D:\work`,
		Totals:  purge.Totals{DeletedCount: 1, PermanentlyDeletedBytes: 10, AffectedBytes: 10},
		Notices: []string{purge.HighImpactNotice},
	})
	for _, want := range []string{"Foal purge", "Permanently deleted", "reinstalling", "not a secure-erasure"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

type permanentRemoverFunc func(context.Context, string) error

func (f permanentRemoverFunc) Remove(ctx context.Context, path string) error {
	return f(ctx, path)
}
