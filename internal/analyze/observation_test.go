package analyze

import (
	"strings"
	"testing"
	"time"
)

func TestSizeIsLowerBoundOnlyPartialAndIncomplete(t *testing.T) {
	cases := map[string]bool{
		BrowseStateScanning:   false,
		BrowseStateComplete:   false,
		BrowseStatePartial:    true,
		BrowseStateIncomplete: true,
		BrowseStateSkipped:    false,
	}
	for state, want := range cases {
		if got := SizeIsLowerBound(state); got != want {
			t.Fatalf("SizeIsLowerBound(%q)=%v want %v", state, got, want)
		}
	}
}

func TestPercentIsApproximateRequiresLocationAndChildComplete(t *testing.T) {
	if !PercentIsApproximate(BrowseStateComplete, false) {
		t.Fatal("incomplete location must force approximate percent")
	}
	if !PercentIsApproximate(BrowseStatePartial, true) {
		t.Fatal("partial child must be approximate even when location claims complete")
	}
	if !PercentIsApproximate(BrowseStateScanning, true) {
		t.Fatal("scanning percent must be approximate")
	}
	if !PercentIsApproximate(BrowseStateIncomplete, true) {
		t.Fatal("incomplete percent must be approximate")
	}
	if PercentIsApproximate(BrowseStateComplete, true) {
		t.Fatal("complete child on complete location may use exact percent")
	}
}

func TestLocationMeasurementComplete(t *testing.T) {
	if !LocationMeasurementComplete(nil) {
		t.Fatal("empty location is complete")
	}
	ok := []BrowseChild{
		{Path: "a", State: BrowseStateComplete, Bytes: 1},
		{Path: "b", State: BrowseStateComplete, Bytes: 2},
	}
	if !LocationMeasurementComplete(ok) {
		t.Fatal("all complete should be complete")
	}
	partial := append([]BrowseChild{}, ok...)
	partial[1].State = BrowseStatePartial
	if LocationMeasurementComplete(partial) {
		t.Fatal("partial child makes location incomplete")
	}
}

func TestFormatSizeTokenLowerBoundPrefix(t *testing.T) {
	format := func(n int64) string { return "10 B" }
	if got := FormatSizeToken(10, BrowseStateComplete, format); got != "10 B" {
		t.Fatalf("complete = %q", got)
	}
	if got := FormatSizeToken(10, BrowseStatePartial, format); got != ">= 10 B" {
		t.Fatalf("partial = %q", got)
	}
	if got := FormatSizeToken(10, BrowseStateIncomplete, format); got != ">= 10 B" {
		t.Fatalf("incomplete = %q", got)
	}
	if got := FormatSizeToken(10, BrowseStateScanning, format); got != "10 B" {
		t.Fatalf("scanning must not use >= size: %q", got)
	}
}

func TestFormatSharePercentNeverUsesGreaterEqual(t *testing.T) {
	// Approximate: scanning/partial/incomplete must never carry ">=" on percent.
	for _, state := range []string{BrowseStateScanning, BrowseStatePartial, BrowseStateIncomplete} {
		got := FormatSharePercent(25, 100, state, false)
		if got == "" || strings.Contains(got, ">=") {
			t.Fatalf("state %s percent = %q (must be non-empty approx without >=)", state, got)
		}
		if !strings.Contains(got, "observed") && !strings.HasPrefix(got, "~") {
			t.Fatalf("state %s percent = %q want approximate labeling", state, got)
		}
	}
	exact := FormatSharePercent(25, 100, BrowseStateComplete, true)
	if exact != "25%" {
		t.Fatalf("exact = %q want 25%%", exact)
	}
	if strings.Contains(exact, ">=") || strings.Contains(exact, "~") {
		t.Fatalf("exact must not look approximate: %q", exact)
	}
}

func TestFormatFocusedDetailAggregatesWithoutPaths(t *testing.T) {
	child := BrowseChild{
		Name:           "secret",
		Path:           `C:\Windows\secret`,
		State:          BrowseStatePartial,
		Bytes:          100,
		FileCount:      3,
		DirectoryCount: 2,
		SkipAggregates: []SkipAggregate{
			{Reason: SkipReasonPermissionDenied, Count: 4},
			{Reason: SkipReasonReadError, Count: 1},
		},
	}
	detail := FormatFocusedDetail(child)
	if !strings.Contains(detail, "state=partial") {
		t.Fatalf("missing state: %s", detail)
	}
	if !strings.Contains(detail, "files=3") || !strings.Contains(detail, "dirs=2") {
		t.Fatalf("missing counts: %s", detail)
	}
	if !strings.Contains(detail, "permission_denied×4") || !strings.Contains(detail, "read_error×1") {
		t.Fatalf("missing aggregates: %s", detail)
	}
	// Must not leak descendant or focused path dumps as an error stream.
	if strings.Contains(detail, `C:\Windows`) {
		t.Fatalf("detail must not embed paths: %s", detail)
	}
}

func TestObservationThrottleTerminalAlwaysAllowed(t *testing.T) {
	th := newObservationThrottle(time.Hour)
	clock := time.Unix(0, 0)
	th.now = func() time.Time { return clock }
	if !th.allow(false) {
		t.Fatal("first non-terminal should pass")
	}
	if th.allow(false) {
		t.Fatal("second non-terminal within interval should be throttled")
	}
	if !th.allow(true) {
		t.Fatal("terminal must always pass")
	}
}

func TestIsTerminalBrowseState(t *testing.T) {
	if IsTerminalBrowseState(BrowseStateScanning) {
		t.Fatal("scanning is not terminal")
	}
	for _, s := range []string{BrowseStateComplete, BrowseStatePartial, BrowseStateIncomplete, BrowseStateSkipped} {
		if !IsTerminalBrowseState(s) {
			t.Fatalf("%s should be terminal", s)
		}
	}
}
