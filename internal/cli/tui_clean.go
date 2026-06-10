package cli

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
)

type cleanPreviewFilter string

const (
	cleanPreviewFilterAll        cleanPreviewFilter = "all"
	cleanPreviewFilterCandidates cleanPreviewFilter = "default candidates"
	cleanPreviewFilterSkipped    cleanPreviewFilter = "skipped"
	cleanPreviewFilterReview     cleanPreviewFilter = "review"
	cleanPreviewFilterErrors     cleanPreviewFilter = "errors"
)

func nextCleanPreviewFilter(filter cleanPreviewFilter) cleanPreviewFilter {
	switch filter {
	case cleanPreviewFilterAll:
		return cleanPreviewFilterCandidates
	case cleanPreviewFilterCandidates:
		return cleanPreviewFilterSkipped
	case cleanPreviewFilterSkipped:
		return cleanPreviewFilterReview
	case cleanPreviewFilterReview:
		return cleanPreviewFilterErrors
	default:
		return cleanPreviewFilterAll
	}
}

func cleanPreviewFilterAllows(active, section cleanPreviewFilter) bool {
	return active == cleanPreviewFilterAll || active == section
}

func cleanFormatBytes(bytes int64) string {
	return fmt.Sprintf("%d bytes", bytes)
}

type cleanPreviewLoadedMsg struct {
	model clean.PreviewReadModel
}

// loadCleanPreviewCmd runs the existing dry-run command path off the UI loop
// and delivers the shared read model; the TUI never owns cleanup logic.
// Browsing stays free of side effects: no history session is recorded and no
// detailed-list file is written, unlike the `foal clean --dry-run` command.
func loadCleanPreviewCmd() tea.Msg {
	result := dryRunClean(context.Background(), clean.Options{})
	return cleanPreviewLoadedMsg{model: clean.NewPreviewReadModel(result)}
}

type cleanModel struct {
	loading  bool
	model    clean.PreviewReadModel
	filter   cleanPreviewFilter
	expanded bool
	notice   string
	vp       viewport.Model
	width    int
	height   int
}

func newCleanModel(width, height int) cleanModel {
	model := cleanModel{
		loading: true,
		filter:  cleanPreviewFilterAll,
		notice:  "Press c to copy candidate paths to the clipboard.",
		vp:      viewport.New(),
	}
	model.setSize(width, height)
	return model
}

func (m *cleanModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.vp.SetWidth(width)
	bodyHeight := height - m.chromeLineCount()
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.vp.SetHeight(bodyHeight)
}

// chromeLineCount measures the current header and footer so the viewport can
// fill the remaining rows; the header height varies with notices and the
// detailed-list path.
func (m *cleanModel) chromeLineCount() int {
	return strings.Count(m.headerContent(), "\n") + strings.Count(cleanPreviewFooter, "\n")
}

func (m *cleanModel) applyLoaded(msg cleanPreviewLoadedMsg) {
	m.loading = false
	m.model = msg.model
	m.refreshViewportContent()
	m.setSize(m.width, m.height)
}

// beginReload puts the view back into the loading state; the caller is
// responsible for issuing loadCleanPreviewCmd.
func (m *cleanModel) beginReload() {
	m.loading = true
	m.notice = "Reloading clean preview (dry-run)..."
	m.vp.GotoTop()
	m.setSize(m.width, m.height)
}

func (m *cleanModel) refreshViewportContent() {
	m.vp.SetContent(renderCleanPreviewSections(m.model, m.filter, m.expanded))
}

func (m *cleanModel) handleKey(key string) {
	switch key {
	case "j", "down":
		m.vp.ScrollDown(1)
	case "k", "up":
		m.vp.ScrollUp(1)
	case "f":
		m.filter = nextCleanPreviewFilter(m.filter)
		m.refreshViewportContent()
		m.vp.GotoTop()
	case "e":
		m.expanded = !m.expanded
		m.refreshViewportContent()
	case "c":
		m.notice = m.copyCandidatePathsNotice()
	default:
		m.notice = "Unknown key. Use j/k, f, e, c, r, b, or q."
	}
	m.setSize(m.width, m.height)
}

// copyCandidatePathsNotice copies the default candidate paths to the system
// clipboard and reports the outcome as a notice line.
func (m *cleanModel) copyCandidatePathsNotice() string {
	if m.loading {
		return "Clean preview is still loading; nothing to copy yet."
	}
	if len(m.model.Candidates) == 0 {
		return "No candidate paths to copy."
	}
	paths := make([]string, 0, len(m.model.Candidates))
	for _, candidate := range m.model.Candidates {
		paths = append(paths, candidate.Path)
	}
	if err := copyTextToClipboard(strings.Join(paths, "\n") + "\n"); err != nil {
		return fmt.Sprintf("Clipboard copy failed: %v", err)
	}
	return fmt.Sprintf("Copied %d candidate path(s) to the clipboard.", len(paths))
}

const cleanPreviewFooter = "\nHints: j/k scroll | f filter | e expand | c copy | r refresh | b back | q quit\n" +
	"No cleanup actions are available in this TUI view.\n"

func (m cleanModel) content() string {
	if m.loading {
		return m.headerContent() + "\nLoading clean preview (dry-run)...\n" + cleanPreviewFooter
	}
	return m.headerContent() + m.vp.View() + "\n" + cleanPreviewFooter
}

func (m cleanModel) headerContent() string {
	var builder strings.Builder
	builder.WriteString("+--------------------------------------------------+\n")
	builder.WriteString("| Clean preview TUI                                |\n")
	builder.WriteString("| Read-only review over foal clean --dry-run       |\n")
	builder.WriteString("+--------------------------------------------------+\n\n")
	builder.WriteString(fmt.Sprintf("Potential space: %s\n", cleanFormatBytes(m.model.PotentialSpaceBytes)))
	builder.WriteString(fmt.Sprintf("Candidates: %d, skipped: %d, errors: %d\n", m.model.CandidateCount, m.model.SkippedCount, len(m.model.Errors)))
	builder.WriteString(fmt.Sprintf("Filter: %s | Scroll: %d%% | Expanded: %t\n", m.filter, int(m.vp.ScrollPercent()*100), m.expanded))
	if m.model.DetailedListPath != "" {
		builder.WriteString(fmt.Sprintf("Detailed candidate list: %s\n", m.model.DetailedListPath))
	}
	if m.notice != "" {
		builder.WriteString(m.notice)
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderCleanPreviewSections(model clean.PreviewReadModel, filter cleanPreviewFilter, expanded bool) string {
	var builder strings.Builder

	if len(model.Notices) > 0 && cleanPreviewFilterAllows(filter, cleanPreviewFilterAll) {
		builder.WriteString("\nNotices\n")
		for _, notice := range model.Notices {
			builder.WriteString(fmt.Sprintf("  %s\n", notice.Message))
		}
	}

	if cleanPreviewFilterAllows(filter, cleanPreviewFilterAll) {
		builder.WriteString("\nProtection rules\n")
		if len(model.ProtectionRules) == 0 {
			builder.WriteString("  No default-enabled protection rules were reported.\n")
		} else {
			for _, rule := range model.ProtectionRules {
				builder.WriteString(fmt.Sprintf("  %s: %s\n", rule.ID, rule.Description))
			}
		}
	}

	if cleanPreviewFilterAllows(filter, cleanPreviewFilterCandidates) {
		builder.WriteString(fmt.Sprintf("\nDefault candidates (%d)\n", len(model.Candidates)))
		if len(model.Candidates) == 0 {
			builder.WriteString("  No default candidates found.\n")
		} else {
			for _, candidate := range model.Candidates {
				builder.WriteString(fmt.Sprintf("  %s (%s, rule: %s, preview action metadata: Recycle Bin)\n",
					candidate.Path, cleanFormatBytes(candidate.Bytes), candidate.Rule))
				if expanded && candidate.PlannedAction != "" {
					builder.WriteString(fmt.Sprintf("    planned action metadata: %s\n", candidate.PlannedAction))
				}
			}
		}
	}

	if cleanPreviewFilterAllows(filter, cleanPreviewFilterSkipped) {
		builder.WriteString(fmt.Sprintf("\nSkipped items (%d)\n", len(model.Skipped)))
		if len(model.Skipped) == 0 {
			builder.WriteString("  No skipped cleanup paths reported.\n")
		} else {
			for _, skipped := range model.Skipped {
				builder.WriteString(fmt.Sprintf("  %s (rule: %s, reason: %s, not counted as Potential space)\n",
					skipped.Path, skipped.Rule, skipped.Reason.Code))
				if expanded && skipped.Reason.Message != "" {
					builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason.Message))
				}
			}
		}
	}

	if cleanPreviewFilterAllows(filter, cleanPreviewFilterReview) {
		writeCleanPreviewReviewSections(&builder, model, expanded)
	}

	if cleanPreviewFilterAllows(filter, cleanPreviewFilterErrors) {
		builder.WriteString(fmt.Sprintf("\nInspection errors (%d)\n", len(model.Errors)))
		if len(model.Errors) == 0 {
			builder.WriteString("  No recoverable inspection errors reported.\n")
		} else {
			for _, err := range model.Errors {
				builder.WriteString(fmt.Sprintf("  %s (rule: %s, error: %s, recoverable: %t)\n",
					err.Path, err.Rule, err.Code, err.Recoverable))
				if expanded && err.Message != "" {
					builder.WriteString(fmt.Sprintf("    %s\n", err.Message))
				}
			}
		}
	}

	return builder.String()
}

func writeCleanPreviewReviewSections(builder *strings.Builder, model clean.PreviewReadModel, expanded bool) {
	builder.WriteString(fmt.Sprintf("\nSkipped by default (%d)\n", len(model.SkippedByDefault)))
	if len(model.SkippedByDefault) == 0 {
		builder.WriteString("  No skipped-by-default review items reported.\n")
	} else {
		for _, skipped := range model.SkippedByDefault {
			builder.WriteString(fmt.Sprintf("  %s", skipped.Name))
			if skipped.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", skipped.Path))
			}
			builder.WriteString(" (not counted as Potential space)\n")
			if expanded && skipped.Reason != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason))
			}
		}
	}

	builder.WriteString(fmt.Sprintf("\nReview clues (%d)\n", len(model.ReviewClues)))
	if len(model.ReviewClues) == 0 {
		builder.WriteString("  No review clues reported.\n")
	} else {
		for _, clue := range model.ReviewClues {
			builder.WriteString(fmt.Sprintf("  %s", clue.Name))
			if clue.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", clue.Path))
			}
			builder.WriteString(" (review only)\n")
			if expanded && clue.Details != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", clue.Details))
			}
		}
	}

	builder.WriteString(fmt.Sprintf("\nRunning application skips (%d)\n", len(model.RunningApplicationSkips)))
	if len(model.RunningApplicationSkips) == 0 {
		builder.WriteString("  No running application skips reported.\n")
	} else {
		for _, skipped := range model.RunningApplicationSkips {
			builder.WriteString(fmt.Sprintf("  %s", skipped.Name))
			if skipped.Application != "" {
				builder.WriteString(fmt.Sprintf(" (%s)", skipped.Application))
			}
			if skipped.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", skipped.Path))
			}
			builder.WriteString(" (skipped, not executable here)\n")
			if expanded && skipped.Reason != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason))
			}
		}
	}

	builder.WriteString(fmt.Sprintf("\nReview suggestions (%d)\n", len(model.ReviewSuggestions)))
	if len(model.ReviewSuggestions) == 0 {
		builder.WriteString("  No review suggestions reported.\n")
	} else {
		for _, suggestion := range model.ReviewSuggestions {
			builder.WriteString(fmt.Sprintf("  %s\n", suggestion.Label))
			if expanded && suggestion.NextStep != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", suggestion.NextStep))
			}
		}
	}
}
