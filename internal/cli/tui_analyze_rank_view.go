package cli

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

// tui_analyze_rank_view.go is pure presentation for the Analyze ranked disk-usage
// browse list (#350). Plain frames remain the test oracle; color/symbol styling is
// applied after composition and never invents reclaimable or physical wording.
//
// Layout priority (drop first under pressure): bar length → kind → rank → percent →
// name truncation. Cursor, safety state, and logical size always survive.
// Volume capacity/free stays on a separate metadata line from summed logical bytes.

// analyzeRankLayout is the responsive column set for one terminal width.
type analyzeRankLayout int

const (
	// analyzeRankLayoutWide: cursor, rank, bar, %, name, kind, state, size.
	analyzeRankLayoutWide analyzeRankLayout = iota
	// analyzeRankLayoutMedium: shorter bar; kind hidden; state kept.
	analyzeRankLayoutMedium
	// analyzeRankLayoutNarrow: cursor, name, state, size only.
	analyzeRankLayoutNarrow
)

// Width breakpoints for Analyze ranked rows (columns of the content area).
const (
	analyzeRankWideMin   = 100
	analyzeRankMediumMin = 72
)

// Bar widths by layout (filled cells of a fixed-width meter).
const (
	analyzeRankBarWide   = 12
	analyzeRankBarMedium = 6
)

// Foal Analyze presentation colors (256-color). Red (1/9) is never used here.
var (
	// Selection: Foal pink accent (not reverse-only so NO_COLOR still uses "> ").
	tuiAnalyzeSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	// Measurement / scanning: blue/cyan.
	tuiAnalyzeMeasureStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	// Partial / Incomplete: yellow (attention, not danger).
	tuiAnalyzePartialStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	// Skipped / Unavailable: gray.
	tuiAnalyzeSkippedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// analyzeRankLayoutForWidth chooses wide/medium/narrow from terminal width.
func analyzeRankLayoutForWidth(width int) analyzeRankLayout {
	if width >= analyzeRankWideMin {
		return analyzeRankLayoutWide
	}
	if width >= analyzeRankMediumMin {
		return analyzeRankLayoutMedium
	}
	return analyzeRankLayoutNarrow
}

// analyzeStateSymbol is a NO_COLOR-safe glyph for measurement reliability.
// Color is never the only carrier of meaning.
func analyzeStateSymbol(state string) string {
	switch state {
	case analyze.BrowseStateScanning:
		return "…"
	case analyze.BrowseStateComplete:
		return "✓"
	case analyze.BrowseStatePartial:
		return "◐"
	case analyze.BrowseStateIncomplete:
		return "◑"
	case analyze.BrowseStateSkipped:
		return "⊘"
	default:
		return "?"
	}
}

// analyzeStateLabel is the plain reliability token shown in rows.
func analyzeStateLabel(state string) string {
	if state == "" {
		return "unknown"
	}
	return state
}

// FormatAnalyzeRankBar builds a fixed-width share bar for childBytes/observedTotal.
// Empty total yields an empty bar of the requested width. Approximate shares use
// the same bar geometry; wording of exact vs approximate lives on the percent token.
func FormatAnalyzeRankBar(childBytes, observedTotal int64, width int) string {
	if width <= 0 {
		return ""
	}
	filled := 0
	if observedTotal > 0 && childBytes > 0 {
		filled = int((childBytes * int64(width)) / observedTotal)
		if filled < 1 && childBytes > 0 {
			filled = 1
		}
		if filled > width {
			filled = width
		}
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// analyzeNameFlags returns presentation-only attribute tags (no cleanup language).
func analyzeNameFlags(child analyze.BrowseChild) string {
	var tags []string
	if child.Hidden {
		tags = append(tags, "hidden")
	}
	if child.System {
		tags = append(tags, "system")
	}
	if len(tags) == 0 {
		return ""
	}
	return "[" + strings.Join(tags, ",") + "]"
}

// truncateRunes truncates s to at most max runes, appending "…" when shortened.
// max < 1 returns empty; max == 1 returns "…" when s is non-empty.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

// AnalyzeRankRowInput is the pure input for one ranked browse row.
type AnalyzeRankRowInput struct {
	Child            analyze.BrowseChild
	Rank             int // 1-based display rank
	ObservedTotal    int64
	LocationComplete bool
	Selected         bool
	Width            int
}

// FormatAnalyzeRankRow builds one plain ranked row for the given width.
// Never claims reclaimable, allocated, physically occupied, or freed space.
func FormatAnalyzeRankRow(in AnalyzeRankRowInput) string {
	layout := analyzeRankLayoutForWidth(in.Width)
	child := in.Child
	cursor := "  "
	if in.Selected {
		cursor = "> "
	}

	state := analyzeStateLabel(child.State)
	sym := analyzeStateSymbol(child.State)
	stateTok := sym + state
	if child.State == analyze.BrowseStateSkipped && child.SkipReason != "" {
		// Stable skip reason stays on the reliability column (not cleanup language).
		stateTok = stateTok + ":" + child.SkipReason
	}

	// Size always present except pure skips with zero observed contribution.
	sizeTok := ""
	if child.State == analyze.BrowseStateSkipped {
		sizeTok = "—"
	} else {
		sizeTok = analyze.FormatSizeToken(child.Bytes, child.State, cleanFormatBytes)
	}

	// Percentage: approximate for scanning / non-complete location totals.
	pctTok := analyze.FormatSharePercent(child.Bytes, in.ObservedTotal, child.State, in.LocationComplete)

	flags := analyzeNameFlags(child)
	nameBase := child.Name
	if flags != "" {
		nameBase = child.Name + " " + flags
	}
	if label := analyze.CompactClassificationLabel(child.Classification); label != "" {
		// Compact presentation label for proven direct-child clue only.
		// JSON retains project_artifact_clue; TUI shows "artifact".
		nameBase = nameBase + " · " + label
	}

	// Fixed right-hand budget: size then state must never be dropped.
	// Estimate reserved columns by layout so the name flexes.
	const (
		cursorW = 2
		rankW   = 4 // "12. "
		sep     = 1
	)

	var barW int
	var showRank, showBar, showPct, showKind bool
	switch layout {
	case analyzeRankLayoutWide:
		barW = analyzeRankBarWide
		showRank, showBar, showPct, showKind = true, true, true, true
	case analyzeRankLayoutMedium:
		barW = analyzeRankBarMedium
		showRank, showBar, showPct, showKind = true, true, true, false
	default:
		barW = 0
		showRank, showBar, showPct, showKind = false, false, false, false
	}

	// When width is extremely tight, drop rank/bar/percent before size/state.
	// Re-evaluate if the nominal layout still fits fixed columns.
	fixed := cursorW + sep + utf8.RuneCountInString(sizeTok) + sep + utf8.RuneCountInString(stateTok)
	if showRank {
		fixed += rankW
	}
	if showBar {
		fixed += barW + sep
	}
	if showPct && pctTok != "" {
		fixed += utf8.RuneCountInString(pctTok) + sep
	}
	kindTok := ""
	if showKind {
		kindTok = child.Kind
		if kindTok == "" {
			kindTok = "?"
		}
		fixed += sep + utf8.RuneCountInString(kindTok)
	}

	// Minimum name room; if fixed columns overflow, peel optional columns.
	minName := 8
	for fixed+minName > in.Width && in.Width > 0 {
		switch {
		case showKind:
			fixed -= sep + utf8.RuneCountInString(kindTok)
			showKind = false
			kindTok = ""
		case showBar:
			fixed -= barW + sep
			showBar = false
			barW = 0
		case showPct && pctTok != "":
			fixed -= utf8.RuneCountInString(pctTok) + sep
			showPct = false
		case showRank:
			fixed -= rankW
			showRank = false
		default:
			// Only name can shrink further.
			minName = 1
			if fixed+minName > in.Width {
				// Last resort: still emit cursor+name+state+size; caller width may
				// be smaller than content (tests use explicit widths).
			}
			goto assemble
		}
	}

assemble:
	nameBudget := in.Width - fixed
	if nameBudget < 1 {
		nameBudget = 1
	}
	nameTok := truncateRunes(nameBase, nameBudget)

	var b strings.Builder
	b.WriteString(cursor)
	if showRank {
		b.WriteString(fmt.Sprintf("%2d. ", in.Rank))
	}
	if showBar {
		b.WriteString(FormatAnalyzeRankBar(child.Bytes, in.ObservedTotal, barW))
		b.WriteByte(' ')
	}
	if showPct && pctTok != "" {
		b.WriteString(pctTok)
		b.WriteByte(' ')
	}
	b.WriteString(nameTok)
	if showKind && kindTok != "" {
		b.WriteString(" · ")
		b.WriteString(kindTok)
	}
	b.WriteString(" · ")
	b.WriteString(stateTok)
	b.WriteString(" · ")
	b.WriteString(sizeTok)
	return b.String()
}

// FormatAnalyzeLocationTotalsLine projects observed logical child bytes separately
// from any volume capacity metadata. Never uses free/total volume figures here.
func FormatAnalyzeLocationTotalsLine(observedTotal int64, locationComplete bool, measuring bool) string {
	token := cleanFormatBytes(observedTotal)
	switch {
	case measuring:
		return fmt.Sprintf("Observed logical children: %s (measuring · approximate)", token)
	case !locationComplete:
		return fmt.Sprintf("Observed logical children: %s (incomplete location total · approximate shares)", token)
	default:
		return fmt.Sprintf("Observed logical children: %s (complete location total)", token)
	}
}

// FormatAnalyzeVolumeMetaLine formats capacity/free when known. Empty when the
// browse location is not a volume root or capacity is unavailable.
func FormatAnalyzeVolumeMetaLine(vol *analyze.LocalVolume) string {
	if vol == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("Volume %s", vol.Letter)}
	if vol.Label != "" {
		parts = append(parts, vol.Label)
	}
	if vol.HasCapacity {
		parts = append(parts,
			fmt.Sprintf("capacity %s", cleanFormatBytes(int64(vol.TotalBytes))),
			fmt.Sprintf("free %s", cleanFormatBytes(int64(vol.FreeBytes))),
		)
	} else if !vol.Available {
		parts = append(parts, "capacity unavailable")
	}
	parts = append(parts, "(volume metadata · not a sum of child logical bytes)")
	return strings.Join(parts, " · ")
}

// FormatAnalyzeFocusedDetailLine wraps core focused detail with TUI byte formatting.
func FormatAnalyzeFocusedDetailLine(child analyze.BrowseChild) string {
	return analyze.FormatFocusedDetailWith(child, cleanFormatBytes)
}

// FormatAnalyzePurgeHandoffLine returns copy-only Purge guidance for the current
// browse location when it independently passes Purge root validation and has a
// direct artifact clue. Empty otherwise. Never launches Purge.
func FormatAnalyzePurgeHandoffLine(root string, children []analyze.BrowseChild) string {
	hasClue := false
	for _, c := range children {
		if c.Classification == analyze.ClassificationProjectArtifactClue {
			hasClue = true
			break
		}
	}
	copyText := analyze.FormatPurgeHandoffCopy(root, hasClue)
	return strings.TrimRight(copyText, "\n")
}

// stylizeAnalyzeBrowseFrame applies Analyze-specific selection and state colors
// after plain composition. NO_COLOR keeps symbols/text only (no hues, no red).
func stylizeAnalyzeBrowseFrame(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, len(lines))
	colorOn := tuiAnalyzeColorEnabled()
	for i, line := range lines {
		out[i] = stylizeAnalyzeBrowseLine(line, colorOn)
	}
	return strings.Join(out, "\n")
}

func tuiAnalyzeColorEnabled() bool {
	return os.Getenv("NO_COLOR") == ""
}

func stylizeAnalyzeBrowseLine(line string, colorOn bool) string {
	trimmed := strings.TrimRight(line, " ")
	if trimmed == "" {
		return line
	}

	// Selected row: Foal pink (or bold under NO_COLOR). Never red.
	if strings.HasPrefix(trimmed, "> ") {
		if !colorOn {
			return tuiSelectedStyle.Render(trimmed)
		}
		return tuiAnalyzeSelectedStyle.Render(trimmed)
	}

	// Section chrome / headers reuse shared non-magnitude styles.
	if strings.HasPrefix(trimmed, "+--") || strings.HasPrefix(trimmed, "| ") {
		return tuiBorderStyle.Render(trimmed)
	}
	if strings.HasPrefix(trimmed, "Hints:") ||
		strings.HasPrefix(trimmed, "This view is read-only") ||
		strings.HasPrefix(trimmed, "Files and reparse") ||
		strings.HasPrefix(trimmed, "Next step:") ||
		strings.HasPrefix(trimmed, "Volume ") && strings.Contains(trimmed, "volume metadata") {
		return tuiFaintStyle.Render(trimmed)
	}
	if strings.HasPrefix(trimmed, "Observed logical children:") {
		if colorOn {
			return tuiAnalyzeMeasureStyle.Render(trimmed)
		}
		return tuiSummaryStyle.Render(trimmed)
	}
	if strings.HasPrefix(trimmed, "Measuring") {
		if colorOn {
			return tuiAnalyzeMeasureStyle.Render(trimmed)
		}
		return tuiSummaryStyle.Render(trimmed)
	}

	// Non-selected child rows: tint by reliability state when colored.
	if colorOn {
		switch {
		case strings.Contains(trimmed, "◐partial") || strings.Contains(trimmed, "◑incomplete"):
			return tuiAnalyzePartialStyle.Render(trimmed)
		case strings.Contains(trimmed, "⊘skipped"):
			return tuiAnalyzeSkippedStyle.Render(trimmed)
		case strings.Contains(trimmed, "…scanning"):
			return tuiAnalyzeMeasureStyle.Render(trimmed)
		case strings.Contains(trimmed, "✓complete"):
			return tuiAnalyzeMeasureStyle.Render(trimmed)
		}
	}

	// Fall back to shared stylize for banners/headings.
	return stylizeNonMagnitudeLine(line, trimmed)
}
