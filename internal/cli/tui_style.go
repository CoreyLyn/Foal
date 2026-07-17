package cli

import (
	"os"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// Whole-line styles decorate complete lines. Restricted token styling is a
// narrow mid-line exception for agreed token kinds (magnitude byte sizes) and
// is applied after plain composition; plain frames remain the test oracle.
// Planned-action markers (perm/bin) stay plain text — never pure-red whole-row
// risk tint on the preview list; red remains reserved for irreversible risk.
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
)

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

// stylizeFrame decorates a rendered plain-text frame line by line. The plain
// frame stays the source of truth for tests and the nil-input entry path.
func stylizeFrame(content string) string {
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		lines[index] = stylizeLine(line)
	}
	return strings.Join(lines, "\n")
}

func stylizeLine(line string) string {
	trimmed := strings.TrimRight(line, " ")
	if trimmed == "" {
		return line
	}

	// Restricted magnitude tokens on Clean preview/selected lines first so the
	// plain frame stays authoritative and whole-line reverse can wrap segments.
	if isMagnitudeEligibleLine(trimmed) {
		return stylizeMagnitudeEligibleLine(trimmed)
	}

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
	return line
}

// isMagnitudeEligibleLine reports lines where trusted byte tokens may receive
// magnitude emphasis: category-first preview rows (checkbox chrome) and the
// Selected total line. Execution progress, result, and confirmation lines are
// excluded from this ticket's magnitude surface.
func isMagnitudeEligibleLine(line string) bool {
	if strings.HasPrefix(line, "Selected:") {
		return true
	}
	// Preview rows: "  > [x] ✓ …" / "    [ ] …" — require checkbox chrome.
	return strings.Contains(line, " [x] ") || strings.Contains(line, " [ ] ")
}

// stylizeMagnitudeEligibleLine applies restricted magnitude token styling after
// plain composition, then optional cursor reverse on non-token segments.
func stylizeMagnitudeEligibleLine(line string) string {
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
	tier := cleanMagnitudeTierFromFormattedToken(token)
	styledToken := styleMagnitudeToken(token, tier)
	if selected {
		return tuiSelectedStyle.Render(left) + styledToken + tuiSelectedStyle.Render(right)
	}
	return left + styledToken + right
}

// styleMagnitudeToken applies restricted magnitude emphasis to one plain byte
// token. Zero/none tiers stay plain; NO_COLOR drops hues and keeps bold for
// attention/strong.
func styleMagnitudeToken(token string, tier cleanMagnitudeTier) string {
	return styleMagnitudeTokenWithColor(token, tier, tuiMagnitudeColorEnabled())
}

// styleMagnitudeTokenWithColor is the testable core for magnitude token styles.
func styleMagnitudeTokenWithColor(token string, tier cleanMagnitudeTier, colorEnabled bool) string {
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
