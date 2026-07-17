package cli

import (
	"os"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// Whole-line styles decorate complete lines. Restricted token styling is a
// narrow mid-line exception for agreed token kinds (magnitude byte sizes,
// confirmation risk warning) and is applied after plain composition; plain
// frames remain the test oracle. Red remains reserved for irreversible risk,
// not for reclaimable-byte magnitude.
var (
	tuiSelectedStyle = lipgloss.NewStyle().Reverse(true).Bold(true)
	tuiBorderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	tuiHeadingStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	tuiSummaryStyle  = lipgloss.NewStyle().Bold(true)
	tuiFaintStyle    = lipgloss.NewStyle().Faint(true)
	tuiBannerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
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

	// Risk channel: pure red + bold for irreversible permanent warning only.
	// Distinct from magnitude orange — large size is not danger.
	tuiRiskWarningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	tuiRiskBoldStyle    = lipgloss.NewStyle().Bold(true)
)

// confirmationPermanentIrreversibleWarning is the plain-frame risk copy shown
// on Clean confirmation when the exact selection discloses permanent deletion.
const confirmationPermanentIrreversibleWarning = "Permanent deletion is irreversible and cannot be recovered from the Recycle Bin."

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
}

// tuiStyleLine is one plain-frame line plus optional trusted magnitude bytes
// for restricted token styling. Text stays the test oracle; when
// HasMagnitudeBytes is true, production classification uses
// cleanMagnitudeTierFromBytes and does not reverse-parse the display token.
type tuiStyleLine struct {
	Text              string
	MagnitudeBytes    int64
	HasMagnitudeBytes bool
}

// plainStyleLine returns a style line with no magnitude metadata.
func plainStyleLine(text string) tuiStyleLine {
	return tuiStyleLine{Text: text}
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

	// Risk channel first: irreversible permanent warning (whole-line red/bold).
	// Distinct from magnitude; must not paint size tokens as danger.
	if isConfirmationRiskWarningLine(trimmed) {
		return styleRiskWarning(trimmed)
	}

	// Restricted magnitude tokens on Clean preview/selected/confirmation
	// measured lines so the plain frame stays authoritative and whole-line
	// reverse can wrap segments.
	if isMagnitudeEligibleLine(trimmed) {
		return stylizeMagnitudeEligibleLine(trimmed, line.MagnitudeBytes, line.HasMagnitudeBytes)
	}

	return stylizeNonMagnitudeLine(line.Text, trimmed)
}

// stylizeLine decorates one plain line without trusted byte metadata.
func stylizeLine(line string) string {
	return stylizeStyleLine(plainStyleLine(line))
}

// stylizeNonMagnitudeLine applies whole-line styles (cursor reverse, headings,
// borders, banners) for lines that are not magnitude or risk surfaces.
// original is returned unchanged when no style matches so trailing spaces stay.
func stylizeNonMagnitudeLine(original, trimmed string) string {
	switch {
	case strings.HasPrefix(trimmed, "> "):
		return tuiSelectedStyle.Render(trimmed)
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
	case strings.HasPrefix(trimmed, "Hints:"),
		trimmed == "No cleanup actions are available in this TUI view.",
		trimmed == "This view is read-only; no actions are executed.",
		trimmed == "This is a read-only navigation shell over existing Foal command paths.":
		return tuiFaintStyle.Render(trimmed)
	case strings.HasPrefix(trimmed, "Potential space:"):
		return tuiSummaryStyle.Render(trimmed)
	}
	for _, prefix := range tuiSectionHeadingPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return tuiHeadingStyle.Render(trimmed)
		}
	}
	return original
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

// stylizeMagnitudeEligibleLine applies restricted magnitude token styling after
// plain composition, then optional cursor reverse on the full row including the
// magnitude token so selection and size cues stack readably.
//
// When trusted is true, tier classification uses cleanMagnitudeTierFromBytes
// (production Clean path). When trusted is false, formatted-token reverse parse
// is an explicit fallback for plain-only stylizeFrame callers (tests / non-
// annotated frames); it is not the primary production path.
func stylizeMagnitudeEligibleLine(line string, bytes int64, trusted bool) string {
	// Preview cursor rows look like "  > [x] …"; Selected totals are never reverse.
	selected := strings.Contains(line, "> [")

	loc := cleanByteTokenPattern.FindStringIndex(line)
	if loc == nil {
		if selected {
			return tuiSelectedStyle.Render(line)
		}
		return line
	}
	left, token, right := line[:loc[0]], line[loc[0]:loc[1]], line[loc[1]:]
	tier := classifyMagnitudeTier(token, bytes, trusted)
	styledToken := styleMagnitudeToken(token, tier, selected)
	if selected {
		return tuiSelectedStyle.Render(left) + styledToken + tuiSelectedStyle.Render(right)
	}
	return left + styledToken + right
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
			return tuiSelectedStyle.Render(token)
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
