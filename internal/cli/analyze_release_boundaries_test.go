package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	// Explicit volume root is allowed for Analyze measurement but never gets Purge copy.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"analyze", `C:\`}, &stdout, &stderr)
	if code != exitOK {
		// Some environments may fail volume root for other reasons; skip only if
		// validation rejected the root entirely.
		if strings.Contains(stderr.String(), "dangerous_root") || strings.Contains(stderr.String(), "unsupported") {
			t.Skipf("volume root not accepted in this environment: %s", stderr.String())
		}
		t.Fatalf("Run returned %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "foal purge") {
		t.Fatalf("volume root human report must not include purge handoff:\n%s", stdout.String())
	}
}

func TestAnalyzeHumanReportOmitsPurgeHandoffOnWindowsManagedRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"analyze", `C:\Windows`}, &stdout, &stderr)
	if code != exitOK {
		// Permission or other scan issues are fine; still must not print handoff.
		out := stdout.String() + stderr.String()
		if strings.Contains(out, "foal purge") {
			t.Fatalf("Windows-managed root must not include purge handoff:\n%s", out)
		}
		return
	}
	if strings.Contains(stdout.String(), "foal purge") {
		t.Fatalf("Windows-managed root must not include purge handoff:\n%s", stdout.String())
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
