package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// unwrapForTest collapses soft-wrapped display rows back into logical text so a
// test can assert on copy that no longer fits a single terminal row.
func unwrapForTest(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestWrapRunesBreaksAtSpacesAndKeepsContent(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"fits unchanged", "short line", 40, []string{"short line"}},
		{"zero width is unconstrained", "some long text here", 0, []string{"some long text here"}},
		{"breaks at a space", "aaa bbb ccc", 7, []string{"aaa bbb", "ccc"}},
		{"exact fit does not break", "aaa bbb", 7, []string{"aaa bbb"}},
		{
			"continuation repeats indent",
			"  - Chrome cache is quite large",
			12,
			[]string{"  - Chrome", "  cache is", "  quite", "  large"},
		},
		{
			"oversized word is split",
			"supercalifragilistic x",
			10,
			[]string{"supercalif", "ragilistic", "x"},
		},
		{"empty stays one empty line", "", 10, []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapRunes(tc.text, tc.width)
			if len(got) != len(tc.want) {
				t.Fatalf("lines = %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("line %d = %q, want %q (all: %#v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestWrapRunesNeverExceedsWidthAndLosesNothing(t *testing.T) {
	texts := []string{
		confirmationServicingAuthorizationLine,
		confirmationServicingUACLine,
		confirmationServicingNoRestartLine,
		confirmationServicingBytesLine,
		confirmationActionTypeCaveat,
		confirmationPermanentIrreversibleWarning,
		cancellationRequestedMessage,
		"      Impact: administrator consent (UAC) required; cannot be canceled once started.",
	}
	for _, width := range []int{20, 40, 60, 80} {
		for _, text := range texts {
			lines := wrapRunes(text, width)
			for i, line := range lines {
				if utf8.RuneCountInString(line) > width {
					t.Fatalf("width %d: line %d overflows (%d runes): %q", width, i, utf8.RuneCountInString(line), line)
				}
			}
			if got, want := unwrapForTest(strings.Join(lines, " ")), unwrapForTest(text); got != want {
				t.Fatalf("width %d: wrapping lost content\n got  %q\n want %q", width, got, want)
			}
		}
	}
}

func TestTruncateStructuredRowKeepsTrailingFields(t *testing.T) {
	row := "  > [x] ✓ A very long category label indeed · 12 item(s) · 1.2 GB"
	got, ok := truncateStructuredRow(row, 50)
	if !ok {
		t.Fatalf("expected the label to absorb the overflow, got ok=false")
	}
	if utf8.RuneCountInString(got) > 50 {
		t.Fatalf("row still overflows: %q", got)
	}
	if !strings.HasSuffix(got, " · 12 item(s) · 1.2 GB") {
		t.Fatalf("trailing fields lost: %q", got)
	}
	if !strings.HasPrefix(got, "  > [x] ✓ ") {
		t.Fatalf("row chrome lost: %q", got)
	}
}

// TestTruncateStructuredRowRefusesToDropTrailingFields pins the safety rule:
// truncation may only shrink the label. When the trailing fields alone overflow,
// the caller must wrap instead — a failed servicing row ends in recovery
// guidance, and cutting it off would hide what the user needs to do next.
func TestTruncateStructuredRowRefusesToDropTrailingFields(t *testing.T) {
	row := "  ! Store · failed · component cleanup failed · This may be a transient lock; try again after a restart."
	if _, ok := truncateStructuredRow(row, 40); ok {
		t.Fatal("expected ok=false when trailing fields alone exceed the width")
	}
	if _, ok := truncateStructuredRow("no separator here at all", 10); ok {
		t.Fatal("expected ok=false for a row with no separator")
	}

	// Through reflow, that row must wrap and keep every word.
	lines := reflowLineText(row, lineKindOutcomeRow, 40)
	if len(lines) < 2 {
		t.Fatalf("expected the row to wrap, got %#v", lines)
	}
	if got, want := unwrapForTest(strings.Join(lines, " ")), unwrapForTest(row); got != want {
		t.Fatalf("wrapping lost content\n got  %q\n want %q", got, want)
	}
}

func TestReflowLineTextDispatchesByRole(t *testing.T) {
	long := "Label that is definitely much too long to fit · 12 item(s) · 1.2 GB"
	if got := reflowLineText(long, lineKindCategoryRow, 40); len(got) != 1 {
		t.Fatalf("structured rows truncate to one line, got %#v", got)
	}
	if got := reflowLineText(confirmationServicingUACLine, lineKindProse, 40); len(got) < 2 {
		t.Fatalf("prose must wrap, got %#v", got)
	}
	if got := reflowLineText(strings.Repeat("=", 90), lineKindRule, 40); len(got) != 1 || got[0] != strings.Repeat("=", 40) {
		t.Fatalf("rules regenerate at width, got %#v", got)
	}
	if got := reflowLineText("Foal Clean is a long title here", lineKindPageTitle, 12); len(got) != 1 ||
		utf8.RuneCountInString(got[0]) > 12 {
		t.Fatalf("titles truncate, got %#v", got)
	}
}

// TestReflowStyleLinesMovesMagnitudeToTheTokenSegment pins that a wrapped total
// keeps exact tier classification: trusted bytes follow whichever display row
// actually carries the byte token, instead of being stranded on a row without
// one (which would silently fall back to reverse-parsing the rendered token).
func TestReflowStyleLinesMovesMagnitudeToTheTokenSegment(t *testing.T) {
	const bytes = int64(1288490188)
	line := styledMagnitudeLine(
		"Selected: 3 categories including several long names · 1.2 GB", bytes, lineKindMeasuredTotal)
	got := reflowStyleLines([]tuiStyleLine{line}, 30)
	if len(got) < 2 {
		t.Fatalf("expected the total to wrap, got %#v", got)
	}
	carriers := 0
	for _, segment := range got {
		if segment.HasMagnitudeBytes {
			carriers++
			if !strings.Contains(segment.Text, "1.2 GB") {
				t.Fatalf("magnitude stranded on a segment with no byte token: %q", segment.Text)
			}
			if segment.MagnitudeBytes != bytes {
				t.Fatalf("trusted bytes = %d, want %d", segment.MagnitudeBytes, bytes)
			}
		}
	}
	if carriers != 1 {
		t.Fatalf("exactly one segment should carry magnitude, got %d: %#v", carriers, got)
	}
}

// TestReflowKeepsFrameWithinTerminalBounds is the accounting fix: viewport
// capacity is counted in the rows the terminal actually draws, so no composed
// line may overflow the width and the frame may not exceed the height. Before
// reflow, capacity was counted in composed lines and a wrapped disclosure
// pushed the frame off screen.
func TestReflowKeepsFrameWithinTerminalBounds(t *testing.T) {
	sizes := []struct{ width, height int }{
		{100, 40}, {80, 24}, {72, 30}, {60, 24}, {50, 40},
	}
	for _, size := range sizes {
		for _, tc := range declaredKindFrameCases() {
			model := tc.model
			model.setSize(size.width, size.height)
			lines := model.contentStyleLines()

			for i, line := range lines {
				if utf8.RuneCountInString(line.Text) > size.width {
					t.Fatalf("%s at %dx%d: line %d overflows width (%d runes): %q",
						tc.name, size.width, size.height, i, utf8.RuneCountInString(line.Text), line.Text)
				}
			}
			// contentStyleLines appends one trailing empty element to preserve
			// the historical final newline, so the drawn rows are len-1.
			if drawn := len(lines) - 1; drawn > size.height {
				t.Fatalf("%s at %dx%d: frame draws %d rows, exceeding the terminal height",
					tc.name, size.width, size.height, drawn)
			}
		}
	}
}

// TestReflowKeepsCursorVisibleAfterNarrowingResize covers reflowViewportAfterResize
// under the new accounting: narrowing the terminal adds wrapped rows, which moves
// every row's display line. The focused row must still be inside the viewport.
func TestReflowKeepsCursorVisibleAfterNarrowingResize(t *testing.T) {
	build := func() eagerCleanModel {
		m := newEagerCleanModelFromSummaries(injectedMixedActionSummaries(), 110, 20)
		m.finished = true
		for i := range m.rows {
			m.rows[i].State = clean.CategoryPreviewComplete
			m.rows[i].CandidateCount = 40 + i
			m.rows[i].Bytes = int64(i+1) * 512 * 1024 * 1024
			m.rows[i].Selected = true
			m.rows[i].SafetyNote = "Regenerated automatically the next time the tool runs."
		}
		return m
	}

	for _, width := range []int{110, 80, 64, 52} {
		model := build()
		model.cursor = len(model.rows) - 1
		model.ensurePreviewCursorVisible()
		model.setSize(width, 20)

		if model.terminalTooSmall() {
			continue
		}
		line := model.previewBodyLineForRow(model.cursor)
		if line < 0 {
			t.Fatalf("width %d: focused row has no display line", width)
		}
		capacity := model.bodyCapacity()
		if capacity <= 0 {
			t.Fatalf("width %d: no body capacity", width)
		}
		if line < model.viewportOffset || line >= model.viewportOffset+capacity {
			t.Fatalf("width %d: focused row at line %d is outside viewport [%d,%d)",
				width, line, model.viewportOffset, model.viewportOffset+capacity)
		}
	}
}

// TestReflowPreservesDisclosureCopyAtNarrowWidth pins the reason prose wraps
// instead of truncating: a confirmation disclosure is what the user authorises,
// so every word must survive a narrow terminal.
func TestReflowPreservesDisclosureCopyAtNarrowWidth(t *testing.T) {
	model := newEagerCleanModelFromSummaries(injectedMixedActionSummaries(), 64, 40)
	model.finished = true
	for i := range model.rows {
		model.rows[i].State = clean.CategoryPreviewComplete
		model.rows[i].CandidateCount = 3
		model.rows[i].Bytes = 4096
		model.rows[i].Selected = true
	}
	model.phase = eagerPhaseConfirmation

	content := unwrapForTest(model.content())
	for _, required := range []string{
		confirmationActionTypeCaveat,
		confirmationNextStepLine,
		confirmationPermanentIrreversibleWarning,
	} {
		if !strings.Contains(content, unwrapForTest(required)) {
			t.Fatalf("disclosure copy lost at width 64: %q\n%s", required, model.content())
		}
	}
}
