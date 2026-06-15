package analyze

import (
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

	result := Run(root)

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

	result := Run(root)

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

	result := Run(root)

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
