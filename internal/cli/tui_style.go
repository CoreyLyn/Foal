package cli

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Whole-line styles only: assertions and copy affordances rely on contiguous
// plain-text fragments, so styling must never inject escapes inside a line.
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
)

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
	switch {
	case trimmed == "":
		return line
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
