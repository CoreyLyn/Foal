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
	case strings.HasPrefix(trimmed, "+--") || strings.HasPrefix(trimmed, "| "):
		return tuiBorderStyle.Render(trimmed)
	case strings.HasPrefix(trimmed, "Hints:"),
		trimmed == "No cleanup actions are available in this TUI view.",
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
