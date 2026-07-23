package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func TestAnalyzeCLIDoesNotWriteHistory(t *testing.T) {
	historyDir := t.TempDir()
	t.Setenv("FOAL_HISTORY_DIR", historyDir)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"analyze", "--json", root}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("Run returned %d; stderr=%q", code, stderr.String())
	}
	entries, err := filepath.Glob(filepath.Join(historyDir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("analyze must not write History: %#v", entries)
	}
}

func TestAnalyzeHumanReportOmitsPurgeHandoffOnVolumeRoot(t *testing.T) {
	// Do not invoke cli.Run against real volume roots: default descendant limits
	// make full-disk walks hang CI. Handoff is pure policy over Result.Root.
	report := analyze.RenderHumanReport(analyze.Result{
		Status: analyze.StatusOK,
		Root:   `C:\`,
		TopChildren: []analyze.ChildResult{{
			Name:           "node_modules",
			Kind:           "directory",
			Classification: analyze.ClassificationProjectArtifactClue,
			Bytes:          1,
		}},
	})
	if strings.Contains(report, "foal purge") {
		t.Fatalf("volume root human report must not include purge handoff:\n%s", report)
	}
}

func TestAnalyzeHumanReportOmitsPurgeHandoffOnWindowsManagedRoot(t *testing.T) {
	// Synthetic report only — never walk C:\Windows in tests (CI timeout).
	report := analyze.RenderHumanReport(analyze.Result{
		Status: analyze.StatusOK,
		Root:   `C:\Windows`,
		TopChildren: []analyze.ChildResult{{
			Name:           "node_modules",
			Kind:           "directory",
			Classification: analyze.ClassificationProjectArtifactClue,
			Bytes:          1,
		}},
	})
	if strings.Contains(report, "foal purge") {
		t.Fatalf("Windows-managed root must not include purge handoff:\n%s", report)
	}
}

func TestAnalyzeCommandDescriptionIsReadOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("help exit %d stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "analyze") {
		t.Fatalf("help missing analyze: %s", out)
	}
	if strings.Contains(out, "cleanup opportunities") {
		t.Fatalf("analyze must not be described as cleanup opportunities: %s", out)
	}
	if !strings.Contains(out, "foal analyze") {
		t.Fatalf("help missing analyze examples: %s", out)
	}
}
