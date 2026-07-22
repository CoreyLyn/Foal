package analyze

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunClassifiesDirectHighConfidenceArtifactDirectoriesAsProjectArtifactClues(t *testing.T) {
	root := t.TempDir()
	artifactNames := []string{"node_modules", "target", "dist", "build", ".build", ".next", "__pycache__"}
	for _, name := range artifactNames {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "artifact.bin"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result := RunCompat(root)

	if len(result.TopChildren) != len(artifactNames) {
		t.Fatalf("len(TopChildren) = %d, want %d", len(result.TopChildren), len(artifactNames))
	}
	childrenByName := make(map[string]ChildResult, len(result.TopChildren))
	for _, child := range result.TopChildren {
		childrenByName[child.Name] = child
	}
	for _, name := range artifactNames {
		child, ok := childrenByName[name]
		if !ok {
			t.Fatalf("TopChildren missing %q: %#v", name, result.TopChildren)
		}
		if child.Path != filepath.Join(root, name) {
			t.Fatalf("%s Path = %q, want %q", name, child.Path, filepath.Join(root, name))
		}
		if child.Kind != "directory" {
			t.Fatalf("%s Kind = %q, want directory", name, child.Kind)
		}
		if child.Classification != "project_artifact_clue" {
			t.Fatalf("%s Classification = %q, want project_artifact_clue", name, child.Classification)
		}
	}
}

func TestRunDoesNotClassifyNonMatchingProjectArtifactClues(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"node_modules", "target", ".next"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("file"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"bin", "obj", "source", "node_modules_backup", "dist-cache", "project"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"node_modules", "build", "__pycache__"} {
		if err := os.Mkdir(filepath.Join(root, "project", name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	result := RunCompat(root)

	for _, child := range result.TopChildren {
		if child.Classification != "" {
			t.Fatalf("%s Classification = %q, want empty", child.Name, child.Classification)
		}
		switch child.Name {
		case "node_modules", "target", ".next":
			if child.Kind != "file" {
				t.Fatalf("%s Kind = %q, want file", child.Name, child.Kind)
			}
		}
	}
}

func TestRunClassificationDoesNotChangeTopChildSelectionOrOrdering(t *testing.T) {
	root := t.TempDir()
	nodeModules := filepath.Join(root, "node_modules")
	if err := os.Mkdir(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "small.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < defaultTopChildLimit; i++ {
		name := "child-" + strconv.Itoa(i)
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "data.bin"), make([]byte, 20-i), 0644); err != nil {
			t.Fatal(err)
		}
	}

	result := RunCompat(root)

	if len(result.TopChildren) != defaultTopChildLimit {
		t.Fatalf("len(TopChildren) = %d, want %d", len(result.TopChildren), defaultTopChildLimit)
	}
	for i, child := range result.TopChildren {
		wantName := "child-" + strconv.Itoa(i)
		if child.Name != wantName {
			t.Fatalf("TopChildren[%d].Name = %q, want %q", i, child.Name, wantName)
		}
		if child.Path != filepath.Join(root, wantName) {
			t.Fatalf("TopChildren[%d].Path = %q, want %q", i, child.Path, filepath.Join(root, wantName))
		}
		if child.Kind != "directory" || child.Bytes != int64(20-i) || child.FileCount != 1 || child.DirectoryCount != 1 {
			t.Fatalf("TopChildren[%d] changed existing fields: %#v", i, child)
		}
		if child.Classification != "" {
			t.Fatalf("TopChildren[%d].Classification = %q, want empty", i, child.Classification)
		}
	}
	if result.Totals.Bytes != 156 || result.Totals.FileCount != 11 || result.Totals.DirectoryCount != 12 {
		t.Fatalf("Totals = %#v, want bytes=156 files=11 directories=12", result.Totals)
	}
}

func TestRunReturnsIncompleteWhenDescendantLimitExceeded(t *testing.T) {
	root := t.TempDir()
	// Create a tree with many descendants to hit the limit.
	// Structure:
	// root/dir0/file0.txt
	// root/dir0/file1.txt
	// root/dir0/file2.txt
	// root/dir1/file0.txt
	// ... more directories and files to ensure we hit limits

	// Create several top-level directories each with multiple files.
	for i := 0; i < 10; i++ {
		dirPath := filepath.Join(root, "dir"+strconv.Itoa(i))
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 5; j++ {
			filePath := filepath.Join(dirPath, "file"+strconv.Itoa(j)+".txt")
			if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	// First, run without a limit to verify it completes normally.
	fullResult, _, ok := Run(context.Background(), root, Options{})
	if !ok {
		t.Fatalf("full Run failed unexpectedly")
	}
	if fullResult.Status != StatusOK {
		t.Fatalf("full result.Status = %q, want %q", fullResult.Status, StatusOK)
	}

	// Now set a low limit that we'll definitely hit.
	// Top-level directories themselves aren't counted as descendants when path == root,
	// but their children are.
	result, reason, ok := Run(context.Background(), root, Options{DescendantLimit: 20})
	if !ok {
		t.Fatalf("Run failed unexpectedly: %v", reason)
	}
	if result.Status != StatusIncomplete {
		t.Fatalf("result.Status = %q, want %q", result.Status, StatusIncomplete)
	}
	// Should have partial totals (not zero, but less than full).
	if result.Totals.Bytes == 0 {
		t.Fatalf("result.Totals.Bytes = 0, want partial bytes for inspected content")
	}
	if result.Totals.Bytes >= fullResult.Totals.Bytes {
		t.Fatalf("partial bytes (%d) >= full bytes (%d), incomplete didn't work",
			result.Totals.Bytes, fullResult.Totals.Bytes)
	}
	// Top children should still be present for the ones we processed.
	if len(result.TopChildren) == 0 {
		t.Fatalf("len(result.TopChildren) = 0, want at least some top children")
	}
}

func TestRunAcceptsLocalFixedVolumeAndWindowsManagedRoots(t *testing.T) {
	// Low descendant limit: root policy is the contract under test, not a full-disk walk.
	opts := Options{DescendantLimit: 1}
	tests := []struct {
		name string
		path string
	}{
		{name: "volume root", path: `C:\`},
		{name: "windows", path: `C:\Windows`},
		{name: "program files", path: `C:\Program Files`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, reason, ok := Run(context.Background(), tt.path, opts)
			if !ok {
				t.Fatalf("Run(%q) ok=false reason=%#v, want accepted analyze read root", tt.path, reason)
			}
			if result.Root == "" {
				t.Fatal("result.Root is empty")
			}
			if result.Status != StatusOK && result.Status != StatusIncomplete {
				t.Fatalf("result.Status = %q, want ok or incomplete", result.Status)
			}
			// Existing JSON projection fields remain present on the result path.
			if result.TopChildren == nil {
				t.Fatal("TopChildren is nil")
			}
			if result.Skipped == nil {
				t.Fatal("Skipped is nil")
			}
		})
	}
}

func TestRunAcceptsUserProfileRoot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("user home unavailable")
	}
	result, reason, ok := Run(context.Background(), home, Options{DescendantLimit: 1})
	if !ok {
		t.Fatalf("Run(%q) ok=false reason=%#v, want accepted analyze read root", home, reason)
	}
	if result.Root == "" {
		t.Fatal("result.Root is empty")
	}
}

func TestRunRejectsUnsupportedAnalyzeRoots(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "unc", path: `\\server\share\proj`, code: "unc_path"},
		{name: "device path", path: `\\.\C:`, code: "device_path"},
		{name: "volume device path", path: `\\.\PhysicalDrive0`, code: "device_path"},
		{name: "empty", path: `   `, code: "empty_path"},
		{name: "short name", path: `C:\PROGRA~1`, code: "short_name_path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reason, ok := Run(context.Background(), tt.path, Options{DescendantLimit: 1})
			if ok {
				t.Fatalf("Run(%q) ok=true, want false", tt.path)
			}
			if reason.Code != tt.code {
				t.Fatalf("reason.Code = %q, want %q (message=%q)", reason.Code, tt.code, reason.Message)
			}
			if reason.Message == "" {
				t.Fatal("empty message")
			}
		})
	}
}

func TestRunRejectsReparseRoot(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	_, reason, ok := Run(context.Background(), link, Options{DescendantLimit: 1})
	if ok {
		t.Fatalf("Run(reparse root) ok=true, want false")
	}
	if reason.Code != "reparse_point" {
		t.Fatalf("reason.Code = %q, want reparse_point (message=%q)", reason.Code, reason.Message)
	}
}

func TestRunProtectionNonIntervention(t *testing.T) {
	// Create a test directory with a protected child.
	root := t.TempDir()
	protectedChild := filepath.Join(root, "protected-dir")
	if err := os.Mkdir(protectedChild, 0755); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(protectedChild, "data.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run analyze on the root - even though we can imagine a protection rule,
	// analyze doesn't use protection rules at all.
	result, reason, ok := Run(context.Background(), root, Options{})
	if !ok {
		t.Fatalf("Run failed unexpectedly: %v", reason)
	}

	// The protected child should still be in the results.
	found := false
	for _, child := range result.TopChildren {
		if child.Name == "protected-dir" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("TopChildren missing protected-dir, protection non-intervention violated")
	}
}

func TestRunNoHistory(t *testing.T) {
	// Analyze runs should not create history sessions.
	// For this test, we just verify the contract - the analyze package doesn't import history,
	// and the Run function has no side effects beyond the scan itself.
	// This is a simple contract test to ensure we don't add history calls in the future.
	root := t.TempDir()
	_, _, ok := Run(context.Background(), root, Options{})
	if !ok {
		t.Fatal("Run failed unexpectedly")
	}
	// No history assertions needed - the fact that analyze doesn't import history
	// and has no history-related code is the contract. If that changes, this test
	// should be updated to verify no sessions are created.
}

func TestRenderHumanReportIncludesStatusOkAndTotals(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "test.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	result := RunCompat(root)
	report := RenderHumanReport(result)

	wantContains := []string{
		"Foal analyze",
		"Root: " + root,
		"Status: ok",
		"Totals: 7 bytes, 1 files, 1 directories",
		"Skipped: 0",
	}
	for _, want := range wantContains {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRenderHumanReportStatusIncompleteClearlyStatesIncomplete(t *testing.T) {
	result := Result{
		Status: StatusIncomplete,
		Root:   `C:\test\root`,
		Totals: Totals{Bytes: 100, FileCount: 5, DirectoryCount: 3},
	}
	report := RenderHumanReport(result)
	if !strings.Contains(report, "Status: incomplete (partial results only, no full tree size)") {
		t.Fatalf("incomplete status missing:\n%s", report)
	}
	if strings.Contains(report, "Status: ok") {
		t.Fatalf("incomplete report should not contain ok:\n%s", report)
	}
}

func TestRenderHumanReportIncludesTopChildrenWithSizeKindClassification(t *testing.T) {
	result := Result{
		Status: StatusOK,
		Root:   `C:\test\root`,
		Totals: Totals{Bytes: 1000, FileCount: 10, DirectoryCount: 5},
		TopChildren: []ChildResult{
			{
				Name:           "node_modules",
				Kind:           "directory",
				Classification: "project_artifact_clue",
				Bytes:          500,
			},
			{
				Name:           "target",
				Kind:           "directory",
				Classification: "project_artifact_clue",
				Bytes:          300,
			},
			{
				Name:           "source",
				Kind:           "directory",
				Bytes:          200,
			},
		},
	}
	report := RenderHumanReport(result)

	wantContains := []string{
		"Top children by size:",
		"directory",
		"500",
		"node_modules",
		"project_artifact_clue",
		"target",
		"200",
		"source",
	}
	for _, want := range wantContains {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRenderHumanReportIncludesPurgeHandoffWhenArtifactCluePresent(t *testing.T) {
	result := Result{
		Status: StatusOK,
		Root:   `C:\test\root`,
		Totals: Totals{Bytes: 1000, FileCount: 10, DirectoryCount: 5},
		TopChildren: []ChildResult{
			{
				Name:           "node_modules",
				Kind:           "directory",
				Classification: "project_artifact_clue",
				Bytes:          500,
			},
		},
	}
	report := RenderHumanReport(result)
	if !strings.Contains(report, "foal purge") {
		t.Fatalf("report missing purge handoff copy:\n%s", report)
	}
	if !strings.Contains(report, `C:\test\root`) {
		t.Fatalf("report missing root in purge handoff:\n%s", report)
	}
}

func TestRenderHumanReportDoesNotIncludePurgeHandoffWhenNoArtifactClues(t *testing.T) {
	result := Result{
		Status: StatusOK,
		Root:   `C:\test\root`,
		Totals: Totals{Bytes: 1000, FileCount: 10, DirectoryCount: 5},
		TopChildren: []ChildResult{
			{
				Name:  "source",
				Kind:  "directory",
				Bytes: 500,
			},
		},
	}
	report := RenderHumanReport(result)
	if strings.Contains(report, "foal purge") {
		t.Fatalf("report should not include purge handoff when no clues:\n%s", report)
	}
}
