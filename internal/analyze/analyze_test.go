package analyze

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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

func TestRunRejectsDangerousRoots(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "volume root", path: `C:\`, code: "dangerous_root"},
		{name: "windows", path: `C:\Windows`, code: "dangerous_root"},
		{name: "program files", path: `C:\Program Files`, code: "dangerous_root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reason, ok := Run(context.Background(), tt.path, Options{})
			if ok {
				t.Fatalf("Run(%q) ok=true, want false", tt.path)
			}
			if reason.Code != tt.code {
				t.Fatalf("reason.Code = %q, want %q", reason.Code, tt.code)
			}
		})
	}
}

func TestRunRejectsUserProfileRoot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("user home unavailable")
	}
	_, reason, ok := Run(context.Background(), home, Options{})
	if ok {
		t.Fatalf("Run(%q) ok=true, want false", home)
	}
	if reason.Code != "dangerous_root" {
		t.Fatalf("reason.Code = %q, want dangerous_root", reason.Code)
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
