package cli

import (
	"strings"
	"unicode/utf8"
)

// Width-aware reflow for composed Clean frames.
//
// A frame line is composed without regard for terminal width, so a long line
// would hard-wrap in the terminal and occupy more display rows than the layout
// counted. Viewport capacity is computed in display rows, so an uncounted wrap
// pushes content off screen and desynchronises scrolling. Reflow happens before
// that accounting: the frame the layout measures is the frame the terminal
// draws.
//
// The choice between wrapping and truncating is a safety decision, not a
// cosmetic one, and follows the line's declared role (see tuiLineKind):
//
//   - Prose, disclosures, risk copy, hints, and totals WRAP. Truncating a
//     confirmation disclosure would silently drop half of what the user is
//     being asked to authorise.
//   - Structured rows (preview, execution, confirmation details) TRUNCATE their
//     label. These end in fields the user reads to decide — size, action,
//     state — so the label yields and the trailing fields survive.
//
// Widths are counted in runes, matching truncateRunes in tui_analyze_rank_view.go.
// This is exact for the current ASCII-plus-symbols copy. Localised or CJK labels
// would need display-width counting instead.

// wrapRunes breaks s into lines of at most width runes, splitting at spaces and
// never mid-word unless a single word is itself too long. Continuation lines
// repeat the original leading indent so wrapped list items and indented notes
// stay visually attached to their parent. A width of 0 or less, or text that
// already fits, yields s unchanged as one line.
func wrapRunes(s string, width int) []string {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}

	indent := s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	if utf8.RuneCountInString(indent) >= width {
		// A continuation indent this deep would leave no room for content.
		indent = ""
	}

	var out []string
	current := indent
	currentLen := utf8.RuneCountInString(current)
	filled := false
	breakLine := func() {
		out = append(out, current)
		current = indent
		currentLen = utf8.RuneCountInString(indent)
		filled = false
	}

	for _, word := range words {
		for {
			separator := 0
			if filled {
				separator = 1
			}
			if currentLen+separator+utf8.RuneCountInString(word) <= width {
				if filled {
					current += " "
					currentLen++
				}
				current += word
				currentLen += utf8.RuneCountInString(word)
				filled = true
				break
			}
			if filled {
				// Retry the word at the start of a fresh line.
				breakLine()
				continue
			}
			// The word alone overflows an empty line: split it. room is at
			// least 1 because indent was cleared above when too deep, so this
			// always consumes runes and terminates.
			room := width - currentLen
			if room < 1 {
				room = 1
			}
			runes := []rune(word)
			current += string(runes[:room])
			breakLine()
			word = string(runes[room:])
			if word == "" {
				break
			}
		}
	}
	if filled || len(out) == 0 {
		out = append(out, current)
	}
	return out
}

// truncateStructuredRow shortens a "·"-separated row to width by shrinking only
// the leading label, so trailing fields — measured size, planned action, state,
// failure guidance — stay intact. It reports false when the label cannot absorb
// the overflow on its own, either because the row has no separator or because
// the trailing fields alone already exceed width. Callers must then wrap rather
// than truncate: dropping a trailing field would discard information the user
// needs, which is exactly what a size or a failure hint is.
func truncateStructuredRow(s string, width int) (string, bool) {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return s, true
	}
	const separator = " · "
	index := strings.Index(s, separator)
	if index < 0 {
		return s, false
	}
	head, tail := s[:index], s[index:]
	room := width - utf8.RuneCountInString(tail)
	if room < 1 {
		return s, false
	}
	return truncateRunes(head, room) + tail, true
}

// reflowLineText expands one composed line into the display lines it occupies
// at width, choosing wrap or truncate by declared role. Lines that already fit
// are returned unchanged.
func reflowLineText(text string, kind tuiLineKind, width int) []string {
	if width <= 0 || utf8.RuneCountInString(text) <= width {
		return []string{text}
	}
	switch kind {
	case lineKindBlank:
		return []string{text}
	case lineKindRule:
		// Rules are generated at terminal width; regenerate rather than wrap so
		// an over-long rule never becomes two.
		return []string{strings.Repeat("=", width)}
	case lineKindCategoryRow, lineKindOutcomeRow, lineKindConfirmDetail:
		if truncated, ok := truncateStructuredRow(text, width); ok {
			return []string{truncated}
		}
		// The label alone cannot absorb the overflow. Wrap so trailing fields
		// survive — a failed servicing row, for instance, ends in recovery
		// guidance that must not be cut off.
		return wrapRunes(text, width)
	case lineKindPageTitle, lineKindSectionHeading, lineKindGroupHeading:
		return []string{truncateRunes(text, width)}
	default:
		// Prose, hints, risk copy, notices, and totals must stay complete.
		return wrapRunes(text, width)
	}
}

// reflowStyleLines expands composed style lines into the display lines the
// terminal will draw. Trusted magnitude bytes follow the segment that actually
// carries the byte token, so a wrapped total keeps exact tier classification
// instead of falling back to reverse-parsing the rendered token.
func reflowStyleLines(lines []tuiStyleLine, width int) []tuiStyleLine {
	if width <= 0 {
		return lines
	}
	out := make([]tuiStyleLine, 0, len(lines))
	for _, line := range lines {
		segments := reflowLineText(line.Text, line.Kind, width)
		if len(segments) == 1 && segments[0] == line.Text {
			out = append(out, line)
			continue
		}
		for _, text := range segments {
			segment := line
			segment.Text = text
			if line.HasMagnitudeBytes && !cleanByteTokenPattern.MatchString(text) {
				segment.MagnitudeBytes = 0
				segment.HasMagnitudeBytes = false
			}
			out = append(out, segment)
		}
	}
	return out
}

// reflowBodyEntries is reflowStyleLines for body entries, preserving each
// segment's row and outcome indices so cursor-follow and execution-follow keep
// resolving to the first display line of the entry they belong to.
func reflowBodyEntries(entries []eagerBodyLine, width int) []eagerBodyLine {
	if width <= 0 {
		return entries
	}
	out := make([]eagerBodyLine, 0, len(entries))
	for _, entry := range entries {
		segments := reflowLineText(entry.text, entry.kind, width)
		if len(segments) == 1 && segments[0] == entry.text {
			out = append(out, entry)
			continue
		}
		for _, text := range segments {
			segment := entry
			segment.text = text
			if entry.hasMagnitudeBytes && !cleanByteTokenPattern.MatchString(text) {
				segment.magnitudeBytes = 0
				segment.hasMagnitudeBytes = false
			}
			out = append(out, segment)
		}
	}
	return out
}
