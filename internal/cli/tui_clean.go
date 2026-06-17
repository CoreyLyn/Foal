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
	cleanPreviewFilterAll         cleanPreviewFilter = "all"
	cleanPreviewFilterCandidates  cleanPreviewFilter = "default candidates"
	cleanPreviewFilterSkipped     cleanPreviewFilter = "skipped"
	cleanPreviewFilterReview      cleanPreviewFilter = "review"
	cleanPreviewFilterErrors      cleanPreviewFilter = "errors"
	cleanPreviewSectionEntryLimit                    = 10
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
	generation uint64
	model      clean.PreviewReadModel
}

// loadCleanPreviewCmd runs the existing dry-run command path off the UI loop
// and delivers the shared read model; the TUI never owns cleanup logic.
// Browsing stays free of side effects: no history session is recorded and no
// detailed-list file is written, unlike the `foal clean --dry-run` command.
func loadCleanPreviewCmd(ctx context.Context, generation uint64) tea.Cmd {
	return func() tea.Msg {
		config := loadProtectionConfiguration()
		result := dryRunClean(ctx, clean.Options{
			Validator:                 config.Validator,
			ProtectionDiagnostics:     config.Diagnostics,
			ProtectionLoadError:       config.LoadError,
			DetectRunningApplications: clean.DetectSupportedBrowserApplications,
		})
		return cleanPreviewLoadedMsg{
			generation: generation,
			model:      clean.NewPreviewReadModel(result),
		}
	}
}

type cleanModel struct {
	loading        bool
	loadGeneration uint64
	cancelLoad     context.CancelFunc
	model          clean.PreviewReadModel
	filter         cleanPreviewFilter
	expanded       bool
	notice         string
	vp             viewport.Model
	width          int
	height         int
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
	if msg.generation != m.loadGeneration {
		return
	}
	m.loading = false
	m.cancelLoad = nil
	m.model = msg.model
	m.refreshViewportContent()
	m.setSize(m.width, m.height)
}

func (m *cleanModel) startLoad(reload bool) tea.Cmd {
	m.cancelPendingLoad()
	m.loadGeneration++
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelLoad = cancel
	m.loading = true
	if reload {
		m.notice = "Reloading clean preview (dry-run)..."
	}
	m.vp.GotoTop()
	m.setSize(m.width, m.height)
	return loadCleanPreviewCmd(ctx, m.loadGeneration)
}

func (m *cleanModel) cancelPendingLoad() {
	if m.cancelLoad == nil {
		return
	}
	m.cancelLoad()
	m.cancelLoad = nil
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
	builder.WriteString("Foal Clean\n")
	builder.WriteString("Preview only - no files changed.\n")
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
	for _, category := range clean.PreviewReportCategories(model, clean.PreviewReportCategoryOptions{
		EntryLimit:        cleanPreviewSectionEntryLimit,
		Expanded:          expanded,
		IncludeCandidates: cleanPreviewFilterAllows(filter, cleanPreviewFilterCandidates),
		IncludeSkipped:    cleanPreviewFilterAllows(filter, cleanPreviewFilterSkipped),
		IncludeReview:     cleanPreviewFilterAllows(filter, cleanPreviewFilterReview),
		IncludeErrors:     cleanPreviewFilterAllows(filter, cleanPreviewFilterErrors),
		IncludeSummary:    true,
		PreviewSummary:     true,
	}) {
		builder.WriteString("\n")
		builder.WriteString(category.Name)
		builder.WriteString("\n")
		for _, line := range category.Lines {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}
