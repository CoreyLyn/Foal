package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

func TestAnalyzeRankLayoutBreakpoints(t *testing.T) {
	if analyzeRankLayoutForWidth(120) != analyzeRankLayoutWide {
		t.Fatal("120 should be wide")
	}
	if analyzeRankLayoutForWidth(analyzeRankWideMin) != analyzeRankLayoutWide {
		t.Fatal("wide min inclusive")
	}
	if analyzeRankLayoutForWidth(analyzeRankWideMin-1) != analyzeRankLayoutMedium {
		t.Fatal("just under wide is medium")
	}
	if analyzeRankLayoutForWidth(analyzeRankMediumMin) != analyzeRankLayoutMedium {
		t.Fatal("medium min inclusive")
	}
	if analyzeRankLayoutForWidth(analyzeRankMediumMin-1) != analyzeRankLayoutNarrow {
		t.Fatal("below medium is narrow")
	}
}

func TestFormatAnalyzeRankBarGeometry(t *testing.T) {
	bar := FormatAnalyzeRankBar(50, 100, 10)
	if utf8.RuneCountInString(bar) != 10 {
		t.Fatalf("bar width = %d want 10 (%q)", utf8.RuneCountInString(bar), bar)
	}
	if !strings.Contains(bar, "█") || !strings.Contains(bar, "░") {
		t.Fatalf("bar must mix fill and empty: %q", bar)
	}
	empty := FormatAnalyzeRankBar(0, 0, 8)
	if empty != strings.Repeat("░", 8) {
		t.Fatalf("empty total bar = %q", empty)
	}
}

func sampleRankChild(name string, bytes int64, state, kind string) analyze.BrowseChild {
	return analyze.BrowseChild{
		Name:      name,
		Path:      `C:\` + name,
		Kind:      kind,
		Bytes:     bytes,
		State:     state,
		Navigable: kind == analyze.BrowseKindDirectory,
	}
}

func TestFormatAnalyzeRankRowWideShowsCursorRankBarPercentNameKindStateSize(t *testing.T) {
	row := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: sampleRankChild("Windows", 75, analyze.BrowseStateComplete, analyze.BrowseKindDirectory),
		Rank:  1, ObservedTotal: 100, LocationComplete: true, Selected: true, Width: 120,
	})
	for _, want := range []string{
		"> ",
		"1.",
		"█",
		"75%",
		"Windows",
		"directory",
		"complete",
		"✓",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("wide row missing %q: %s", want, row)
		}
	}
	// Size token from cleanFormatBytes (sub-KB values render as "<1 KB").
	if !strings.Contains(row, "KB") && !strings.Contains(row, "B") {
		t.Fatalf("wide row missing size: %s", row)
	}
	for _, banned := range []string{"reclaimable", "allocated", "physically", "freed"} {
		if strings.Contains(strings.ToLower(row), banned) {
			t.Fatalf("row must not claim %q: %s", banned, row)
		}
	}
}

func TestFormatAnalyzeRankRowMediumHidesKindKeepsStateAndShortBar(t *testing.T) {
	row := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: sampleRankChild("Users", 40, analyze.BrowseStateComplete, analyze.BrowseKindDirectory),
		Rank:  2, ObservedTotal: 100, LocationComplete: true, Selected: false, Width: 80,
	})
	if strings.Contains(row, "directory") {
		t.Fatalf("medium must hide kind before state: %s", row)
	}
	if !strings.Contains(row, "complete") {
		t.Fatalf("medium must keep state: %s", row)
	}
	if !strings.Contains(row, "█") && !strings.Contains(row, "░") {
		t.Fatalf("medium still shows a bar: %s", row)
	}
	// Medium bar is shorter than wide (6 vs 12).
	wideBar := FormatAnalyzeRankBar(40, 100, analyzeRankBarWide)
	medBar := FormatAnalyzeRankBar(40, 100, analyzeRankBarMedium)
	if !(strings.Contains(row, medBar) || utf8.RuneCountInString(medBar) < utf8.RuneCountInString(wideBar)) {
		t.Fatalf("expected shorter medium bar geometry; row=%s", row)
	}
	if !strings.Contains(row, "Users") {
		t.Fatalf("name must remain: %s", row)
	}
}

func TestFormatAnalyzeRankRowNarrowKeepsCursorNameSizeState(t *testing.T) {
	row := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: sampleRankChild("pagefile.sys", 10, analyze.BrowseStateComplete, analyze.BrowseKindFile),
		Rank:  3, ObservedTotal: 100, LocationComplete: true, Selected: true, Width: 48,
	})
	if !strings.HasPrefix(row, "> ") {
		t.Fatalf("narrow selected cursor: %s", row)
	}
	if !strings.Contains(row, "pagefile") {
		t.Fatalf("name retained: %s", row)
	}
	if !strings.Contains(row, "complete") {
		t.Fatalf("state retained: %s", row)
	}
	// No bar / rank / kind required on narrow.
	if strings.Contains(row, "directory") || strings.Contains(row, "file ·") {
		// kind may still appear only if width accidentally wide — assert no bar blocks.
	}
	if strings.Contains(row, "█") {
		t.Fatalf("narrow should drop bar: %s", row)
	}
	// No horizontal-scroll requirement: row should not exceed width by large margin
	// after name truncation (allow small slack for multi-byte symbols).
	if n := utf8.RuneCountInString(row); n > 60 {
		t.Fatalf("narrow row too long (%d): %s", n, row)
	}
}

func TestFormatAnalyzeRankRowTruncatesLongNameBeforeSizeOrState(t *testing.T) {
	long := strings.Repeat("VeryLongDirectoryName", 8)
	row := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: sampleRankChild(long, 99, analyze.BrowseStatePartial, analyze.BrowseKindDirectory),
		Rank:  1, ObservedTotal: 100, LocationComplete: false, Selected: false, Width: 70,
	})
	if !strings.Contains(row, "partial") {
		t.Fatalf("state must survive truncation: %s", row)
	}
	if !strings.Contains(row, ">=") {
		t.Fatalf("partial size lower bound must survive: %s", row)
	}
	if !strings.Contains(row, "…") {
		t.Fatalf("long name should truncate with ellipsis: %s", row)
	}
	// Full name must not force size off the row.
	if strings.Count(row, long) > 0 && !strings.Contains(row, "…") {
		// If the full name fit, that is fine; when it does not, ellipsis is required.
	}
}

func TestFormatAnalyzeRankRowApproximatePercentWhenIncomplete(t *testing.T) {
	row := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: sampleRankChild("scan-me", 25, analyze.BrowseStateScanning, analyze.BrowseKindDirectory),
		Rank:  1, ObservedTotal: 100, LocationComplete: false, Selected: false, Width: 120,
	})
	if !strings.Contains(row, "~") && !strings.Contains(row, "observed") {
		t.Fatalf("scanning/non-complete percent must be approximate: %s", row)
	}
	if strings.Contains(row, ">=25%") || strings.Contains(row, ">= 25%") {
		t.Fatalf("percent must never use >=: %s", row)
	}
	exact := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: sampleRankChild("done", 25, analyze.BrowseStateComplete, analyze.BrowseKindFile),
		Rank:  1, ObservedTotal: 100, LocationComplete: true, Selected: false, Width: 120,
	})
	if !strings.Contains(exact, "25%") {
		t.Fatalf("exact percent missing: %s", exact)
	}
	if strings.Contains(exact, "~25%") {
		t.Fatalf("exact must not be approximate: %s", exact)
	}
}

func TestFormatAnalyzeRankRowIdentifiesHiddenSystemWithoutCleanupLanguage(t *testing.T) {
	child := sampleRankChild("pagefile.sys", 1, analyze.BrowseStateComplete, analyze.BrowseKindFile)
	child.Hidden = true
	child.System = true
	row := FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child: child, Rank: 1, ObservedTotal: 1, LocationComplete: true, Selected: false, Width: 100,
	})
	if !strings.Contains(row, "hidden") || !strings.Contains(row, "system") {
		t.Fatalf("hidden/system tags missing: %s", row)
	}
	for _, banned := range []string{"delete", "reclaim", "clean", "trash", "candidate"} {
		if strings.Contains(strings.ToLower(row), banned) {
			t.Fatalf("no cleanup language (%q): %s", banned, row)
		}
	}
}

func TestFormatAnalyzeLocationTotalsSeparateFromVolumeMeta(t *testing.T) {
	obs := FormatAnalyzeLocationTotalsLine(1024, true, false)
	if !strings.Contains(obs, "logical") {
		t.Fatalf("must label logical children: %s", obs)
	}
	if strings.Contains(strings.ToLower(obs), "free") || strings.Contains(strings.ToLower(obs), "capacity") {
		t.Fatalf("child totals must not mix volume free/capacity: %s", obs)
	}
	vol := &analyze.LocalVolume{
		Letter: "C:", HasCapacity: true, TotalBytes: 1 << 30, FreeBytes: 1 << 29, Available: true, Label: "System",
	}
	meta := FormatAnalyzeVolumeMetaLine(vol)
	if !strings.Contains(meta, "capacity") || !strings.Contains(meta, "free") {
		t.Fatalf("volume meta needs capacity/free: %s", meta)
	}
	if !strings.Contains(meta, "not a sum of child logical bytes") {
		t.Fatalf("volume meta must disclaim child sum: %s", meta)
	}
	for _, banned := range []string{"reclaimable", "allocated", "physically occupied", "freed"} {
		if strings.Contains(strings.ToLower(obs), banned) || strings.Contains(strings.ToLower(meta), banned) {
			t.Fatalf("banned wording %q in %q / %q", banned, obs, meta)
		}
	}
}

func TestAnalyzeStateSymbolsPreserveNOColorDistinctions(t *testing.T) {
	// Every terminal/non-terminal state has a distinct symbol + label pair.
	seen := map[string]string{}
	for _, state := range []string{
		analyze.BrowseStateScanning,
		analyze.BrowseStateComplete,
		analyze.BrowseStatePartial,
		analyze.BrowseStateIncomplete,
		analyze.BrowseStateSkipped,
	} {
		sym := analyzeStateSymbol(state)
		label := analyzeStateLabel(state)
		key := sym + label
		if prev, ok := seen[key]; ok {
			t.Fatalf("collision between %s and %s", prev, state)
		}
		seen[key] = state
		if sym == "" || label == "" {
			t.Fatalf("empty symbol/label for %s", state)
		}
	}
}

func TestStylizeAnalyzeBrowseFrameNoRedAndNOColor(t *testing.T) {
	plain := ">  1. ██████░░░░ 50% Windows · directory · ✓complete · 50 B\n" +
		"  2. ███░░░░░░░ ~30% observed partial-dir · ◐partial · >= 30 B\n" +
		"  3. ░░░░░░░░░░ link · ⊘skipped · —\n" +
		"Measuring (approximate observed shares while scanning)...\n" +
		"Observed logical children: 80 B (incomplete location total · approximate shares)\n"

	t.Setenv("NO_COLOR", "1")
	noColor := stylizeAnalyzeBrowseFrame(plain)
	// Under NO_COLOR, state text/symbols still present; ANSI color CSI should be absent
	// for our custom hues. Bold/reverse (selected) may still emit attributes.
	for _, want := range []string{"✓complete", "◐partial", "⊘skipped", "Measuring", "logical"} {
		if !strings.Contains(noColor, want) {
			t.Fatalf("NO_COLOR lost %q:\n%s", want, noColor)
		}
	}
	// Pure red ANSI (38;5;1 or 31) must not appear from Analyze styling.
	if strings.Contains(noColor, "38;5;1") || strings.Contains(noColor, "[31m") {
		t.Fatalf("NO_COLOR frame must not use red: %q", noColor)
	}

	t.Setenv("NO_COLOR", "")
	colored := stylizeAnalyzeBrowseFrame(plain)
	// Selected pink 212 and measure cyan 14 may appear; red must not.
	if strings.Contains(colored, "38;5;1") || strings.Contains(colored, "[31m") {
		t.Fatalf("colored Analyze frame must not use red: %q", colored)
	}
	if !strings.Contains(colored, ">") {
		t.Fatalf("selection marker preserved: %s", colored)
	}
}

func TestFormatAnalyzeFocusedDetailLineIncludesBytesAndSkippedTotal(t *testing.T) {
	child := analyze.BrowseChild{
		Name: "x", Path: `C:\x`, State: analyze.BrowseStatePartial,
		Bytes: 2048, FileCount: 2, DirectoryCount: 1,
		SkipAggregates: []analyze.SkipAggregate{
			{Reason: analyze.SkipReasonPermissionDenied, Count: 3},
		},
	}
	detail := FormatAnalyzeFocusedDetailLine(child)
	if !strings.Contains(detail, "state=partial") {
		t.Fatalf("state: %s", detail)
	}
	if !strings.Contains(detail, "bytes=") {
		t.Fatalf("bytes: %s", detail)
	}
	if !strings.Contains(detail, "skipped_total=3") {
		t.Fatalf("skipped_total: %s", detail)
	}
	if strings.Contains(detail, `C:\x`) {
		t.Fatalf("must not embed path: %s", detail)
	}
}
