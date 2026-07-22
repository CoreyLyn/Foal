package cli

import (
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func TestFormatAnalyzeRankRowCompactArtifactLabel(t *testing.T) {
	child := sampleRankChild("node_modules", 100, analyze.BrowseStateComplete, analyze.BrowseKindDirectory)
	child.Classification = analyze.ClassificationProjectArtifactClue
	row := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: child, Rank: 1, ObservedTotal: 100, LocationComplete: true, Selected: false, Width: 120,
	})
	if !strings.Contains(row, "artifact") {
		t.Fatalf("compact artifact label missing: %s", row)
	}
	if strings.Contains(row, "project_artifact_clue") {
		t.Fatalf("row must not show raw JSON classification token: %s", row)
	}
}

func TestFormatAnalyzePurgeHandoffLineGuarded(t *testing.T) {
	children := []analyze.BrowseChild{
		{Name: "node_modules", Classification: analyze.ClassificationProjectArtifactClue},
	}
	if got := FormatAnalyzePurgeHandoffLine(`C:\`, children); got != "" {
		t.Fatalf("volume root handoff = %q, want empty", got)
	}
	if got := FormatAnalyzePurgeHandoffLine(`C:\Windows`, children); got != "" {
		t.Fatalf("Windows root handoff = %q, want empty", got)
	}
	if got := FormatAnalyzePurgeHandoffLine(`C:\Users\example\project`, nil); got != "" {
		t.Fatalf("no-clue handoff = %q, want empty", got)
	}
}
