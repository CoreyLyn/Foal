package purge_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/core/delete"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
	"github.com/CoreyLyn/Foal/internal/purge"
)

func TestDryRunOmitsProtectedArtifactsWithoutLeakingPath(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("keep", "node_modules", "a"):    "aa",
		filepath.Join("secret", "node_modules", "b"):  "bbb",
		filepath.Join("secret", "dist", "out.js"):     "cccc",
		filepath.Join("other", "target", "debug", "x"): "d",
	})
	protected := filepath.Join(root, "secret")
	validator := pathsafe.NewValidator([]string{protected})

	result := purge.DryRun(context.Background(), purge.Options{
		Root:      root,
		Validator: validator,
	})
	if result.Status != purge.StatusPreview {
		t.Fatalf("status = %q message=%q", result.Status, result.Message)
	}

	for _, c := range result.Candidates {
		if strings.Contains(strings.ToLower(c.Path), "secret") {
			t.Fatalf("protected path leaked into candidates: %#v", c)
		}
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %#v, want keep/node_modules + other/target", result.Candidates)
	}

	protectedSkips := 0
	for _, s := range result.Skipped {
		if s.Reason != purge.IssueProtectedPath {
			continue
		}
		protectedSkips++
		if s.Path != "" {
			t.Fatalf("protected skip must not include path (leak risk): %#v", s)
		}
		if s.Kind == "" {
			t.Fatalf("protected skip should retain kind: %#v", s)
		}
		if strings.Contains(strings.ToLower(s.Detail), "secret") {
			t.Fatalf("detail leaked protected location: %#v", s)
		}
	}
	if protectedSkips != 2 {
		t.Fatalf("protected skips = %d in %#v, want 2 (node_modules+dist)", protectedSkips, result.Skipped)
	}

	// Totals exclude protected artifacts.
	var sum int64
	for _, c := range result.Candidates {
		sum += c.Bytes
	}
	if result.Totals.CandidateCount != 2 || result.Totals.Bytes != sum {
		t.Fatalf("totals = %#v", result.Totals)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	// Protected subtree path must not appear as a candidate path in JSON.
	if strings.Contains(string(raw), filepath.Join("secret", "node_modules")) ||
		strings.Contains(string(raw), filepath.Join("secret", "dist")) {
		t.Fatalf("JSON leaked protected candidate paths: %s", raw)
	}
}

func TestExecuteRespectsProtectionFilter(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("open", "node_modules", "a"):   "xyz",
		filepath.Join("locked", "node_modules", "b"): "wwww",
	})
	openNM := filepath.Join(root, "open", "node_modules")
	lockedNM := filepath.Join(root, "locked", "node_modules")
	validator := pathsafe.NewValidator([]string{filepath.Join(root, "locked")})
	remover := &recordingPermanentRemover{}

	result := purge.Execute(context.Background(), purge.Options{
		Root:                   root,
		Validator:              validator,
		AllowPermanentDeletion: true,
		PermanentRemover:       remover,
	})
	if result.Status != purge.StatusOK {
		t.Fatalf("status = %q message=%q", result.Status, result.Message)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Path != openNM {
		t.Fatalf("deleted = %#v, want only open node_modules", result.Deleted)
	}
	for _, p := range remover.paths {
		if strings.Contains(strings.ToLower(p), "locked") {
			t.Fatalf("remover touched protected path: %v", remover.paths)
		}
	}
	if _, err := os.Lstat(openNM); !os.IsNotExist(err) {
		t.Fatalf("open node_modules should be deleted: %v", err)
	}
	if _, err := os.Lstat(lockedNM); err != nil {
		t.Fatalf("locked node_modules must remain: %v", err)
	}
}

func TestDryRunRejectsDangerousRootsWithoutScanning(t *testing.T) {
	// Policy rejection is path-form based (no filesystem read of system trees).
	for _, root := range []string{`C:\`, `C:\Windows`, `C:\Program Files`, `C:\Windows\Temp`} {
		result := purge.DryRun(context.Background(), purge.Options{Root: root})
		if result.Status != purge.StatusError {
			t.Fatalf("root %q status = %q, want error", root, result.Status)
		}
		if len(result.Candidates) != 0 {
			t.Fatalf("root %q must not produce candidates: %#v", root, result.Candidates)
		}
		msg := strings.ToLower(result.Message)
		if !strings.Contains(msg, "dangerous_root") &&
			!strings.Contains(msg, "volume") &&
			!strings.Contains(msg, "system") {
			t.Fatalf("root %q message = %q, want dangerous-root style error", root, result.Message)
		}
	}
}

func TestDryRunRejectsDangerousRootAmongMultipleWithoutPartialScan(t *testing.T) {
	safe := t.TempDir()
	writeArtifactTree(t, safe, map[string]string{
		filepath.Join("app", "node_modules", "a"): "aa",
	})
	// One safe + one dangerous: whole run fails; safe tree must not be partially claimed.
	result := purge.DryRun(context.Background(), purge.Options{
		Roots: []string{safe, `C:\Windows`},
	})
	if result.Status != purge.StatusError {
		t.Fatalf("status = %q message=%q", result.Status, result.Message)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("partial candidates must not be claimed: %#v", result.Candidates)
	}
	if !strings.Contains(strings.ToLower(result.Message), "dangerous_root") &&
		!strings.Contains(strings.ToLower(result.Message), "system") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestDryRunMultiRootDiscoversEachRoot(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeArtifactTree(t, rootA, map[string]string{
		filepath.Join("app", "node_modules", "a"): "aaa",
	})
	writeArtifactTree(t, rootB, map[string]string{
		filepath.Join("lib", "dist", "b"): "bbbb",
	})

	result := purge.DryRun(context.Background(), purge.Options{
		Roots: []string{rootA, rootB},
	})
	if result.Status != purge.StatusPreview {
		t.Fatalf("status = %q message=%q", result.Status, result.Message)
	}
	if result.Root != "" {
		t.Fatalf("multi-root must leave single Root empty, got %q", result.Root)
	}
	if len(result.Roots) != 2 {
		t.Fatalf("roots = %#v", result.Roots)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	kinds := map[string]int64{}
	for _, c := range result.Candidates {
		kinds[c.Kind] = c.Bytes
		// Absolute path still under one of the roots.
		if !strings.HasPrefix(c.Path, rootA) && !strings.HasPrefix(c.Path, rootB) {
			t.Fatalf("candidate escaped roots: %#v", c)
		}
	}
	if kinds["node_modules"] != 3 || kinds["dist"] != 4 {
		t.Fatalf("kinds = %#v", kinds)
	}
	if result.Totals.CandidateCount != 2 || result.Totals.Bytes != 7 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}

func TestExecuteMultiRootDeletesAcrossRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeArtifactTree(t, rootA, map[string]string{
		filepath.Join("app", "node_modules", "a"): "xy",
	})
	writeArtifactTree(t, rootB, map[string]string{
		filepath.Join("web", "dist", "b"): "z",
	})
	nm := filepath.Join(rootA, "app", "node_modules")
	dist := filepath.Join(rootB, "web", "dist")

	result := purge.Execute(context.Background(), purge.Options{
		Roots:                  []string{rootA, rootB},
		AllowPermanentDeletion: true,
		PermanentRemover:       delete.FilesystemPermanentRemover{},
	})
	if result.Status != purge.StatusOK {
		t.Fatalf("status = %q message=%q", result.Status, result.Message)
	}
	if len(result.Deleted) != 2 {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if _, err := os.Lstat(nm); !os.IsNotExist(err) {
		t.Fatalf("node_modules remains: %v", err)
	}
	if _, err := os.Lstat(dist); !os.IsNotExist(err) {
		t.Fatalf("dist remains: %v", err)
	}
	if result.Totals.PermanentlyDeletedBytes != 3 {
		t.Fatalf("permanently_deleted_bytes = %d", result.Totals.PermanentlyDeletedBytes)
	}
}

func TestDryRunMultiRootReparseFailClosedPerRoot(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeArtifactTree(t, rootA, map[string]string{
		filepath.Join("app", "target", "x"): "yy",
	})
	// Symlink artifact under B must not become a candidate; A still discovers.
	real := filepath.Join(rootB, "real")
	if err := os.Mkdir(real, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rootB, "node_modules")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := purge.DryRun(context.Background(), purge.Options{
		Roots: []string{rootA, rootB},
	})
	if result.Status != purge.StatusPreview {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Kind != "target" {
		t.Fatalf("candidates = %#v, want only target from rootA", result.Candidates)
	}
	found := false
	for _, s := range result.Skipped {
		if s.Path == link && s.Reason == "reparse_point" {
			found = true
		}
	}
	if !found {
		t.Fatalf("skipped = %#v, want reparse_point for symlink under rootB", result.Skipped)
	}
}

func TestDryRunProtectionLoadFailureFailClosed(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root, map[string]string{
		filepath.Join("app", "node_modules", "a"): "a",
	})
	result := purge.DryRun(context.Background(), purge.Options{
		Root: root,
		ProtectionLoadError: &purge.Issue{
			Code:    purge.IssueProtectionFileLoadFailed,
			Message: "selected protection file could not be read",
		},
	})
	if result.Status != purge.StatusError {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("must not scan after protection load failure: %#v", result.Candidates)
	}
	if !strings.Contains(result.Message, purge.IssueProtectionFileLoadFailed) {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestDryRunNoImplicitMultiRootDefaults(t *testing.T) {
	// Empty Roots+Root still errors; never invents profile/drive roots.
	result := purge.DryRun(context.Background(), purge.Options{})
	if result.Status != purge.StatusError {
		t.Fatalf("status = %q", result.Status)
	}
	if !strings.Contains(result.Message, "explicit root") {
		t.Fatalf("message = %q", result.Message)
	}
}
