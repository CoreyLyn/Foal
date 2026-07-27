package cli

import (
	"os"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// Whole-line styles decorate complete lines. Restricted token styling is a
// narrow mid-line exception for agreed token kinds (magnitude byte sizes,
// state markers, confirmation risk warning) and is applied after plain
// composition; plain frames remain the test oracle. Red remains reserved for
// irreversible risk, not for reclaimable-byte magnitude.
//
// Color language (256-color indexes; NO_COLOR drops hues, keeps bold/reverse):
//
//	border/rule  — gray 240 (recedes; never competes with content)
//	heading      — cyan 6
//	progress/ok  — cyan 14 (scanning + complete markers; not success-green)
//	attention    — yellow 11 (partial / failed markers; not danger)
//	skipped      — gray 8
//	magnitude    — amber 214 / orange 208 (size only)
//	risk         — pure red 1 (irreversible permanent only)
var (
	// Selection keeps reverse so focused Clean rows stay continuous. Accent cyan
	// is applied only when color is enabled so NO_COLOR stays reverse/bold only.
	tuiSelectedStyle       = lipgloss.NewStyle().Reverse(true).Bold(true)
	tuiSelectedAccentStyle = lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(lipgloss.Color("81"))
	// Borders and footer rules recede; headings stay the cyan focus cue.
	tuiBorderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	tuiRuleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	tuiHeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	tuiSummaryStyle = lipgloss.NewStyle().Bold(true)
	tuiFaintStyle   = lipgloss.NewStyle().Faint(true)
	tuiBannerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	// Phase / scan progress line (not a section heading prefix match alone).
	tuiProgressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	// No underline here: lipgloss renders underlined text rune by rune,
	// which would break contiguous-substring assertions and copy affordances.
	tuiLinkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

	// Magnitude hues: amber/yellow for attention, orange for strong. Never pure
	// red (1/9) for size — red stays reserved for irreversible risk.
	tuiMagnitudeAttentionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	tuiMagnitudeStrongStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
	tuiMagnitudeBoldStyle      = lipgloss.NewStyle().Bold(true)

	// Selected-row magnitude stacking: keep reverse on the byte token so the
	// focused row reads as continuous selection, while magnitude hue stays
	// visible on the size token (orange/amber, never pure red for size).
	tuiSelectedMagnitudeAttentionStyle = lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(lipgloss.Color("214"))
	tuiSelectedMagnitudeStrongStyle    = lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(lipgloss.Color("208"))
	tuiSelectedMagnitudeBoldStyle      = lipgloss.NewStyle().Reverse(true).Bold(true)

	// State-marker tokens (restricted mid-line): color carries reliability only.
	// Never pure red — failed/partial use yellow attention, not risk red.
	// Complete/cleaned uses cyan (14), not success-green: ADR 0023 defers a full
	// result success-green / fail-red palette; cyan matches Analyze measure language.
	tuiStateOKStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	tuiStateAttentionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	tuiStateSkippedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tuiStateProgressStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	tuiStateEmptyStyle     = lipgloss.NewStyle().Faint(true)

	// Selected-row state markers stack reverse with the same reliability hues.
	tuiSelectedStateOKStyle        = lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(lipgloss.Color("14"))
	tuiSelectedStateAttentionStyle = lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(lipgloss.Color("11"))
	tuiSelectedStateSkippedStyle   = lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(lipgloss.Color("8"))
	tuiSelectedStateProgressStyle  = lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(lipgloss.Color("14"))
	tuiSelectedStateEmptyStyle     = lipgloss.NewStyle().Reverse(true).Bold(true).Faint(true)

	// Risk channel: pure red + bold for irreversible permanent warning only.
	// Distinct from magnitude orange — large size is not danger.
	tuiRiskWarningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	tuiRiskBoldStyle    = lipgloss.NewStyle().Bold(true)
	// Soft risk notice on preview when permanent work is selected (not the
	// confirmation irreversible warning, which stays pure-red whole-line).
	tuiPermanentNoticeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
)

// confirmationPermanentIrreversibleWarning is the plain-frame risk copy shown
// on Clean confirmation when the exact selection discloses permanent deletion.
const confirmationPermanentIrreversibleWarning = "Permanent deletion is irreversible and cannot be recovered from the Recycle Bin."

// permanentSelectionNotice is the preview footer risk sentence when the exact
// selection includes permanent deletion (see eagerPermanentSelectionNotice).
const permanentSelectionNotice = "Selection includes permanent deletion."

// cleanByteTokenPattern matches cleanFormatBytes plain tokens, including the
// optional left padding used for preview column alignment.
var cleanByteTokenPattern = regexp.MustCompile(`(?:<1|\d+(?:\.\d+)?) (?:KB|MB|GB)`)

var tuiSectionHeadingPrefixes = []string{
	"Notices",
	"Protection rules",
	"Default candidates (",
	"Skipped items (",
	"Skipped by default (",
	"Review clues (",
	"Running application skips (",
	"Review suggestions (",
	"Inspection errors (",
	"Sessions (",
	"Skipped (",
	"Errors (",
	"Disk",
	// Clean category-first chrome titles / phase headings.
	"Foal Clean",
	"Confirm cleanup",
	"Cleanup result",
	"Clean unavailable",
	"Terminal too small",
	// Confirmation action-group section headers (detail sections, not totals).
	"Permanent deletion",
	"Recycle Bin",
	"Windows servicing",
}

// cleanPageTitlePrefixes are primary page titles that always get heading style
// even when they share wording with confirmation measured totals.
var cleanPageTitleExact = map[string]struct{}{
	"Foal Clean":         {},
	"Confirm cleanup":    {},
	"Cleanup result":     {},
	"Clean unavailable":  {},
	"Terminal too small": {},
	"Permanent deletion": {},
	"Recycle Bin":        {},
	"Windows servicing":  {},
}

// tuiLineKind is the semantic role a frame line plays, declared by the code
// that composes the line rather than inferred from the rendered text. The
// sniffing path below (prefix and regexp matching on already-rendered text) is
// how every surface historically got styled; it stays as the fallback for
// stylizeFrame callers (analyze, viewer, uninstall, menu), which have only
// composed text. Clean composes through tuiStyleLine and can declare its roles.
//
// Declaring the role removes a class of coupling: cleanStateMarkerAtStart has
// to exclude ASCII '-' because it would false-colour confirmation bullets
// ("  - Label"), and cleanPageTitleExact exists to stop heading prefixes
// over-matching measured totals. Neither constraint applies once the composer
// says what a line is.
type tuiLineKind int

const (
	// lineKindUnknown is the zero value: no declared role, fall back to
	// sniffing. Non-Clean surfaces stay here.
	lineKindUnknown tuiLineKind = iota
	// lineKindBlank is an empty or whitespace-only spacer line.
	lineKindBlank
	// lineKindPageTitle is a primary page title ("Foal Clean", "Cleanup result").
	lineKindPageTitle
	// lineKindSectionHeading is a section header inside a page (confirmation
	// action-group titles).
	lineKindSectionHeading
	// lineKindGroupHeading is a preview report-category grouping row. NOTE:
	// this currently renders unstyled — it matches no sniffing rule today.
	// Preserved as-is; whether it should read as a heading is a separate call.
	lineKindGroupHeading
	// lineKindProgressHeader is in-flight phase chrome (scanning / execution).
	lineKindProgressHeader
	// lineKindCategoryRow is a preview row: cursor, checkbox, marker, label,
	// and optionally a trusted measured byte token.
	lineKindCategoryRow
	// lineKindOutcomeRow is an execution or result row: marker plus label, with
	// a trusted affected byte token only on cleaned/partial outcomes.
	lineKindOutcomeRow
	// lineKindMeasuredTotal is a total carrying a trusted byte token eligible
	// for magnitude emphasis (selection totals, result aggregates).
	lineKindMeasuredTotal
	// lineKindProgressTotal is the mid-flight execution total. It carries a byte
	// token but is deliberately NOT magnitude-emphasised: magnitude is reserved
	// for settled measurements, not provisional progress.
	lineKindProgressTotal
	// lineKindConfirmSummary is a confirmation action-group total.
	lineKindConfirmSummary
	// lineKindConfirmDetail is a confirmation per-category row ("  - Label · ...").
	lineKindConfirmDetail
	// lineKindConfirmImpact is the indented impact note under a detail row.
	lineKindConfirmImpact
	// lineKindRisk is irreversible-permanent warning copy.
	lineKindRisk
	// lineKindPermanentNotice is the softer preview permanent-selection notice.
	lineKindPermanentNotice
	// lineKindProse is disclosure, notice, and explanation copy.
	lineKindProse
	// lineKindHint is a key-hint line.
	lineKindHint
	// lineKindRule is a horizontal '=' rule.
	lineKindRule
)

// tuiStyleLine is one plain-frame line plus optional trusted magnitude bytes
// for restricted token styling. Text stays the test oracle; when
// HasMagnitudeBytes is true, production classification uses
// cleanMagnitudeTierFromBytes and does not reverse-parse the display token.
// Kind, when set, replaces text sniffing for this line.
type tuiStyleLine struct {
	Text              string
	Kind              tuiLineKind
	MagnitudeBytes    int64
	HasMagnitudeBytes bool
}

// plainStyleLine returns a style line with no magnitude metadata and no
// declared role (sniffed styling).
func plainStyleLine(text string) tuiStyleLine {
	return tuiStyleLine{Text: text}
}

// styledLine returns a style line whose role is declared by its composer.
func styledLine(text string, kind tuiLineKind) tuiStyleLine {
	return tuiStyleLine{Text: text, Kind: kind}
}

// styledMagnitudeLine returns a style line with a declared role whose primary
// cleanFormatBytes token should be tier-classified from the trusted byte count.
func styledMagnitudeLine(text string, bytes int64, kind tuiLineKind) tuiStyleLine {
	return tuiStyleLine{Text: text, Kind: kind, MagnitudeBytes: bytes, HasMagnitudeBytes: true}
}

// magnitudeStyleLine returns a style line whose primary cleanFormatBytes token
// should be tier-classified from the trusted byte count.
func magnitudeStyleLine(text string, bytes int64) tuiStyleLine {
	return tuiStyleLine{Text: text, MagnitudeBytes: bytes, HasMagnitudeBytes: true}
}

// styleLinesFromPlain splits a plain multi-line frame (as strings.Split on
// "\n") into style lines without magnitude metadata. Used for paths that only
// have composed text (too-small / unavailable chrome).
func styleLinesFromPlain(content string) []tuiStyleLine {
	lines := strings.Split(content, "\n")
	out := make([]tuiStyleLine, len(lines))
	for i, line := range lines {
		out[i] = plainStyleLine(line)
	}
	return out
}

// joinStyleLineText joins style line plain text. Trailing empty Text elements
// preserve a final newline the same way content() historically did.
func joinStyleLineText(lines []tuiStyleLine) string {
	if len(lines) == 0 {
		return ""
	}
	parts := make([]string, len(lines))
	for i, line := range lines {
		parts[i] = line.Text
	}
	return strings.Join(parts, "\n")
}

// stylizeFrame decorates a rendered plain-text frame line by line. The plain
// frame stays the source of truth for tests and the nil-input entry path.
// Without per-line byte metadata, magnitude tiers fall back to formatted-token
// classification (see stylizeMagnitudeEligibleLine).
func stylizeFrame(content string) string {
	return stylizeStyleLines(styleLinesFromPlain(content))
}

// stylizeStyleLines decorates annotated frame lines. When a line carries
// trusted magnitude bytes, tier classification prefers cleanMagnitudeTierFromBytes.
func stylizeStyleLines(lines []tuiStyleLine) string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = stylizeStyleLine(line)
	}
	return strings.Join(out, "\n")
}

func stylizeStyleLine(line tuiStyleLine) string {
	trimmed := strings.TrimRight(line.Text, " ")
	if trimmed == "" {
		return line.Text
	}

	// A declared role wins; sniffing below is the fallback for callers that
	// only have composed text.
	if line.Kind != lineKindUnknown {
		return stylizeDeclaredLine(line, trimmed)
	}

	// Risk channel first: irreversible permanent warning (whole-line red/bold).
	// Distinct from magnitude; must not paint size tokens as danger.
	if isConfirmationRiskWarningLine(trimmed) {
		return styleRiskWarning(trimmed)
	}
	// Preview permanent-selection notice: same risk hue, softer than confirmation.
	if isPermanentSelectionNoticeLine(trimmed) {
		return stylePermanentNotice(trimmed)
	}

	// Restricted magnitude tokens on Clean preview/selected/confirmation
	// measured lines so the plain frame stays authoritative and whole-line
	// reverse can wrap segments. State markers on those rows are styled in the
	// same pass so selection reverse stays continuous.
	if isMagnitudeEligibleLine(trimmed) {
		return stylizeMagnitudeEligibleLine(trimmed, line.MagnitudeBytes, line.HasMagnitudeBytes)
	}

	// Execution / result outcome rows carry a leading state marker without a
	// magnitude field (waiting/skipped/failed) — color the marker only.
	if isCleanOutcomeMarkerLine(trimmed) {
		return stylizeCleanOutcomeMarkerLine(trimmed)
	}

	return stylizeNonMagnitudeLine(line.Text, trimmed)
}

// stylizeDeclaredLine styles a line whose composer declared its role. Each arm
// routes to the same style helper the sniffing path would have chosen, so
// declaring a role changes routing only — never the rendered bytes. The
// equivalence is pinned by TestDeclaredKindMatchesSniffedStyling.
func stylizeDeclaredLine(line tuiStyleLine, trimmed string) string {
	colorOn := tuiMagnitudeColorEnabled()
	switch line.Kind {
	case lineKindBlank:
		return line.Text

	case lineKindPageTitle, lineKindSectionHeading:
		return tuiHeadingStyle.Render(trimmed)

	case lineKindProgressHeader:
		if colorOn {
			return tuiProgressStyle.Render(trimmed)
		}
		return tuiSummaryStyle.Render(trimmed)

	case lineKindRule:
		if colorOn {
			return tuiRuleStyle.Render(trimmed)
		}
		return tuiFaintStyle.Render(trimmed)

	case lineKindHint:
		return tuiFaintStyle.Render(trimmed)

	case lineKindRisk:
		return styleRiskWarning(trimmed)

	case lineKindPermanentNotice:
		return stylePermanentNotice(trimmed)

	// Magnitude-eligible surfaces. A line with no byte token degrades to the
	// marker-only pass inside stylizeMagnitudeEligibleLine, so these arms are
	// safe for rows that happen to carry no size (empty, skipped, waiting).
	case lineKindCategoryRow, lineKindOutcomeRow, lineKindMeasuredTotal,
		lineKindConfirmSummary, lineKindConfirmDetail:
		return stylizeMagnitudeEligibleLine(trimmed, line.MagnitudeBytes, line.HasMagnitudeBytes)

	// Unstyled surfaces. lineKindProgressTotal is deliberately here: the
	// mid-flight execution total carries a byte token that must stay plain.
	case lineKindGroupHeading, lineKindConfirmImpact, lineKindProgressTotal, lineKindProse:
		return line.Text

	default:
		return line.Text
	}
}

// stylizeLine decorates one plain line without trusted byte metadata.
func stylizeLine(line string) string {
	return stylizeStyleLine(plainStyleLine(line))
}

// stylizeNonMagnitudeLine applies whole-line styles (cursor reverse, headings,
// borders, banners) for lines that are not magnitude or risk surfaces.
// original is returned unchanged when no style matches so trailing spaces stay.
func stylizeNonMagnitudeLine(original, trimmed string) string {
	colorOn := tuiMagnitudeColorEnabled()
	switch {
	case strings.HasPrefix(trimmed, "> "):
		return styleSelectedLine(trimmed, colorOn)
	}
	// Banner rows mix wordmark art with side text; style the parts
	// separately, splitting only at known boundaries so asserted fragments
	// stay contiguous.
	if index := strings.Index(trimmed, bannerURL); index >= 0 {
		return tuiBannerStyle.Render(trimmed[:index]) + tuiLinkStyle.Render(bannerURL)
	}
	if index := strings.Index(trimmed, bannerTagline); index >= 0 {
		return tuiBannerStyle.Render(trimmed[:index]) + trimmed[index:]
	}
	if isBannerArtLine(trimmed) {
		return tuiBannerStyle.Render(trimmed)
	}
	switch {
	case strings.HasPrefix(trimmed, "+--") || strings.HasPrefix(trimmed, "| "):
		return tuiBorderStyle.Render(trimmed)
	case isFooterRuleLine(trimmed):
		if colorOn {
			return tuiRuleStyle.Render(trimmed)
		}
		return tuiFaintStyle.Render(trimmed)
	case strings.HasPrefix(trimmed, "Hints:"),
		trimmed == "No cleanup actions are available in this TUI view.",
		trimmed == "This view is read-only; no actions are executed.",
		trimmed == "This is a read-only navigation shell over existing Foal command paths.":
		return tuiFaintStyle.Render(trimmed)
	case strings.HasPrefix(trimmed, "Potential space:"):
		return tuiSummaryStyle.Render(trimmed)
	case isCleanProgressHeaderLine(trimmed):
		if colorOn {
			return tuiProgressStyle.Render(trimmed)
		}
		return tuiSummaryStyle.Render(trimmed)
	}
	// Exact Clean page / section titles first (avoid prefix over-match on
	// measured totals that somehow skip the magnitude path).
	if _, ok := cleanPageTitleExact[trimmed]; ok {
		return tuiHeadingStyle.Render(trimmed)
	}
	for _, prefix := range tuiSectionHeadingPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return tuiHeadingStyle.Render(trimmed)
		}
	}
	return original
}

// styleSelectedLine applies focused-row reverse. Soft cyan accent only when
// color is enabled; NO_COLOR keeps reverse+bold without a hue.
func styleSelectedLine(text string, colorOn bool) string {
	if colorOn {
		return tuiSelectedAccentStyle.Render(text)
	}
	return tuiSelectedStyle.Render(text)
}

// isFooterRuleLine reports Clean preview footer rules made of '=' only.
func isFooterRuleLine(line string) bool {
	if line == "" {
		return false
	}
	for _, r := range line {
		if r != '=' {
			return false
		}
	}
	return true
}

// isCleanProgressHeaderLine reports phase chrome that should read as in-flight
// measurement rather than a static section heading.
func isCleanProgressHeaderLine(line string) bool {
	return strings.HasPrefix(line, "Scanning ") ||
		strings.HasPrefix(line, "Scan complete") ||
		strings.HasPrefix(line, "Canceled ·") ||
		// Execution header: " sp phase · elapsed" (spinner + label).
		(len(line) > 2 && (line[0] == '|' || line[0] == '/' || line[0] == '-' || line[0] == '\\') && line[1] == ' ')
}

// isMagnitudeEligibleLine reports lines where trusted byte tokens may receive
// magnitude emphasis: category-first preview rows (checkbox chrome), the
// Selected total line, confirmation measured group/category lines, and result
// successful affected-style totals/outcomes. Execution-progress chrome stays
// outside this surface; non-success outcomes invent no success-byte field so
// they never match cleaned/partial patterns.
func isMagnitudeEligibleLine(line string) bool {
	if strings.HasPrefix(line, "Selected:") {
		return true
	}
	// Result footer totals: one trusted aggregate token per line. Wording is
	// never "freed"/"reclaimed" disk space (see resultFooterLines).
	if strings.HasPrefix(line, "Recycle Bin moved:") ||
		strings.HasPrefix(line, "Permanently deleted:") ||
		strings.HasPrefix(line, "Affected (processed):") {
		return true
	}
	// Result successful outcome rows (cleaned / partial) carry affected bytes.
	if strings.Contains(line, " · cleaned · ") || strings.Contains(line, " · partial · ") {
		return true
	}
	// Preview rows: "  > [x] ✓ …" / "    [ ] …" — require checkbox chrome.
	if strings.Contains(line, " [x] ") || strings.Contains(line, " [ ] ") {
		return true
	}
	return isConfirmationMeasuredLine(line)
}

// isConfirmationMeasuredLine reports confirmation body lines that carry
// trusted measured byte totals (group headers and per-category rows). Does not
// match preview checkboxes, execution rows, result aggregates, or risk copy.
func isConfirmationMeasuredLine(line string) bool {
	if strings.HasPrefix(line, "Permanent deletion ·") || strings.HasPrefix(line, "Recycle Bin ·") {
		return true
	}
	// Category rows: "  - Label · N item(s) · <bytes> · <action>"
	return strings.HasPrefix(line, "  - ") && strings.Contains(line, " item(s) · ")
}

// isConfirmationRiskWarningLine reports the irreversible permanent warning
// shown only when confirmation discloses permanent deletion.
func isConfirmationRiskWarningLine(line string) bool {
	return line == confirmationPermanentIrreversibleWarning
}

// isPermanentSelectionNoticeLine reports the preview footer risk sentence when
// permanent work is in the exact selection.
func isPermanentSelectionNoticeLine(line string) bool {
	return line == permanentSelectionNotice
}

// styleRiskWarning applies risk-channel emphasis (red/bold) to confirmation
// irreversible permanent copy. NO_COLOR keeps bold without red hue.
func styleRiskWarning(text string) string {
	return styleRiskWarningWithColor(text, tuiMagnitudeColorEnabled())
}

// styleRiskWarningWithColor is the testable core for risk warning styles.
func styleRiskWarningWithColor(text string, colorEnabled bool) string {
	if !colorEnabled {
		return tuiRiskBoldStyle.Render(text)
	}
	return tuiRiskWarningStyle.Render(text)
}

// stylePermanentNotice emphasizes the preview permanent-selection notice.
// Same risk hue as confirmation warning; NO_COLOR keeps bold only.
func stylePermanentNotice(text string) string {
	if !tuiMagnitudeColorEnabled() {
		return tuiRiskBoldStyle.Render(text)
	}
	return tuiPermanentNoticeStyle.Render(text)
}

// stylizeMagnitudeEligibleLine applies restricted magnitude token styling after
// plain composition, then optional cursor reverse on the full row including the
// magnitude token so selection and size cues stack readably. State markers on
// the same row are colored in the left/right segments so reliability stays
// scannable without whole-row tint.
//
// When trusted is true, tier classification uses cleanMagnitudeTierFromBytes
// (production Clean path). When trusted is false, formatted-token reverse parse
// is an explicit fallback for plain-only stylizeFrame callers (tests / non-
// annotated frames); it is not the primary production path.
func stylizeMagnitudeEligibleLine(line string, bytes int64, trusted bool) string {
	// Preview cursor rows look like "  > [x] …"; Selected totals are never reverse.
	selected := strings.Contains(line, "> [")
	colorOn := tuiMagnitudeColorEnabled()

	loc := cleanByteTokenPattern.FindStringIndex(line)
	if loc == nil {
		return styleSegmentWithOptionalMarker(line, selected, colorOn)
	}
	left, token, right := line[:loc[0]], line[loc[0]:loc[1]], line[loc[1]:]
	tier := classifyMagnitudeTier(token, bytes, trusted)
	styledToken := styleMagnitudeToken(token, tier, selected)
	return styleSegmentWithOptionalMarker(left, selected, colorOn) +
		styledToken +
		styleSegmentWithOptionalMarker(right, selected, colorOn)
}

// styleSegmentWithOptionalMarker colors at most one Clean state marker inside
// segment. When selected, non-marker spans keep continuous reverse (+ accent).
func styleSegmentWithOptionalMarker(segment string, selected bool, colorOn bool) string {
	if segment == "" {
		return ""
	}
	loc := findFirstCleanStateMarker(segment)
	if loc == nil {
		if selected {
			return styleSelectedLine(segment, colorOn)
		}
		return segment
	}
	before, marker, after := segment[:loc[0]], segment[loc[0]:loc[1]], segment[loc[1]:]
	styledMarker := styleCleanStateMarker(marker, selected, colorOn)
	if selected {
		return styleSelectedLine(before, colorOn) + styledMarker + styleSelectedLine(after, colorOn)
	}
	return before + styledMarker + after
}

// isCleanOutcomeMarkerLine reports execution/result rows that lead with a state
// marker and have no checkbox chrome (preview) or confirmation dash rows.
func isCleanOutcomeMarkerLine(line string) bool {
	if strings.Contains(line, " [x] ") || strings.Contains(line, " [ ] ") {
		return false
	}
	if strings.HasPrefix(line, "  - ") || strings.HasPrefix(line, "Selected:") {
		return false
	}
	if !strings.HasPrefix(line, "  ") {
		return false
	}
	return findFirstCleanStateMarker(line) != nil
}

// stylizeCleanOutcomeMarkerLine colors only the leading reliability marker on
// execution/result rows. Successful cleaned/partial rows with bytes are handled
// by the magnitude path instead.
func stylizeCleanOutcomeMarkerLine(line string) string {
	return styleSegmentWithOptionalMarker(line, false, tuiMagnitudeColorEnabled())
}

// findFirstCleanStateMarker returns [start,end) of the first Clean state glyph
// in a well-known position: after checkbox chrome, or as the execution marker
// following a two-space indent. Nil when no restricted marker applies.
func findFirstCleanStateMarker(s string) []int {
	for _, box := range []string{"[x] ", "[ ] "} {
		if i := strings.Index(s, box); i >= 0 {
			restStart := i + len(box)
			if loc := cleanStateMarkerAtStart(s[restStart:]); loc != nil {
				return []int{restStart + loc[0], restStart + loc[1]}
			}
		}
	}
	// Execution / result: "  <marker> <label…>" — no checkbox.
	if strings.HasPrefix(s, "  ") && !strings.Contains(s, "[") {
		if loc := cleanStateMarkerAtStart(s[2:]); loc != nil {
			return []int{2 + loc[0], 2 + loc[1]}
		}
	}
	return nil
}

// cleanStateMarkerAtStart reports a marker token at the start of s. Markers are
// single glyphs from the Clean preview/execution sets. ASCII '-' is intentionally
// omitted: it collides with confirmation list bullets ("  - Label") and would
// false-color those rows; spinner frames still color on | / \.
func cleanStateMarkerAtStart(s string) []int {
	if s == "" {
		return nil
	}
	for _, m := range []string{"✓", "⊘", "…", "–", "!", "?", "|", "/", "\\"} {
		if !strings.HasPrefix(s, m) {
			continue
		}
		end := len(m)
		if end < len(s) && s[end] != ' ' {
			continue
		}
		return []int{0, end}
	}
	return nil
}

// styleCleanStateMarker applies reliability hue to one plain marker token.
// selected stacks reverse; NO_COLOR keeps bold/reverse without hues. Pure red
// is never used — failed/partial stay yellow attention.
func styleCleanStateMarker(marker string, selected bool, colorOn bool) string {
	kind := classifyCleanStateMarker(marker)
	if selected {
		if !colorOn {
			return tuiSelectedStyle.Render(marker)
		}
		switch kind {
		case cleanStateOK:
			return tuiSelectedStateOKStyle.Render(marker)
		case cleanStateAttention:
			return tuiSelectedStateAttentionStyle.Render(marker)
		case cleanStateSkipped:
			return tuiSelectedStateSkippedStyle.Render(marker)
		case cleanStateProgress:
			return tuiSelectedStateProgressStyle.Render(marker)
		case cleanStateEmpty:
			return tuiSelectedStateEmptyStyle.Render(marker)
		default:
			return styleSelectedLine(marker, true)
		}
	}
	if !colorOn {
		// Markers stay plain under NO_COLOR so symbol meaning is unchanged;
		// bold is reserved for risk/magnitude fallbacks.
		return marker
	}
	switch kind {
	case cleanStateOK:
		return tuiStateOKStyle.Render(marker)
	case cleanStateAttention:
		return tuiStateAttentionStyle.Render(marker)
	case cleanStateSkipped:
		return tuiStateSkippedStyle.Render(marker)
	case cleanStateProgress:
		return tuiStateProgressStyle.Render(marker)
	case cleanStateEmpty:
		return tuiStateEmptyStyle.Render(marker)
	default:
		return marker
	}
}

// cleanStateMarkerKind is the reliability channel for a Clean state glyph.
type cleanStateMarkerKind int

const (
	cleanStateOther cleanStateMarkerKind = iota
	cleanStateOK
	cleanStateAttention
	cleanStateSkipped
	cleanStateProgress
	cleanStateEmpty
)

func classifyCleanStateMarker(marker string) cleanStateMarkerKind {
	switch marker {
	case "✓":
		return cleanStateOK
	case "!", "?":
		// ? = analysis required / unknown; attention, not irreversible risk.
		return cleanStateAttention
	case "⊘":
		return cleanStateSkipped
	case "…", "|", "/", "\\":
		return cleanStateProgress
	case "–":
		return cleanStateEmpty
	default:
		return cleanStateOther
	}
}

// classifyMagnitudeTier prefers trusted int64 bytes; formatted-token parse is
// fallback-only when the style seam has plain text and no byte metadata.
func classifyMagnitudeTier(token string, bytes int64, trusted bool) cleanMagnitudeTier {
	if trusted {
		return cleanMagnitudeTierFromBytes(bytes)
	}
	return cleanMagnitudeTierFromFormattedToken(token)
}

// styleMagnitudeToken applies restricted magnitude emphasis to one plain byte
// token. Zero/none tiers stay plain (or reverse-only when selected); NO_COLOR
// drops hues and keeps bold for attention/strong.
func styleMagnitudeToken(token string, tier cleanMagnitudeTier, selected bool) string {
	return styleMagnitudeTokenWithColor(token, tier, tuiMagnitudeColorEnabled(), selected)
}

// styleMagnitudeTokenWithColor is the testable core for magnitude token styles.
// selected stacks reverse with magnitude hue so focused preview rows keep both
// the selection cue and the size cue without pure-red size coloring.
func styleMagnitudeTokenWithColor(token string, tier cleanMagnitudeTier, colorEnabled bool, selected bool) string {
	if selected {
		switch tier {
		case cleanMagnitudeAttention:
			if !colorEnabled {
				return tuiSelectedMagnitudeBoldStyle.Render(token)
			}
			return tuiSelectedMagnitudeAttentionStyle.Render(token)
		case cleanMagnitudeStrong:
			if !colorEnabled {
				return tuiSelectedMagnitudeBoldStyle.Render(token)
			}
			return tuiSelectedMagnitudeStrongStyle.Render(token)
		default:
			// None/Neutral: continuous reverse so the focused row has no gap.
			return styleSelectedLine(token, colorEnabled)
		}
	}
	switch tier {
	case cleanMagnitudeAttention:
		if !colorEnabled {
			return tuiMagnitudeBoldStyle.Render(token)
		}
		return tuiMagnitudeAttentionStyle.Render(token)
	case cleanMagnitudeStrong:
		if !colorEnabled {
			return tuiMagnitudeBoldStyle.Render(token)
		}
		return tuiMagnitudeStrongStyle.Render(token)
	default:
		// None and Neutral: no magnitude hue. Neutral may stay plain.
		return token
	}
}

// tuiMagnitudeColorEnabled reports whether magnitude hues may be emitted.
// Any non-empty NO_COLOR disables hues (no-color.org); bold may still apply.
func tuiMagnitudeColorEnabled() bool {
	return os.Getenv("NO_COLOR") == ""
}

// isBannerArtLine reports whether a line consists only of the FOAL wordmark
// charset, so plain box borders and report rows never match.
func isBannerArtLine(line string) bool {
	for _, r := range line {
		switch r {
		case '_', '|', '/', '\\', ' ':
		default:
			return false
		}
	}
	return true
}
