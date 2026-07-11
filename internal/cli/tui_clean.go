package cli

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
)

type cleanPreviewFilter string

const (
	cleanPreviewFilterAll               cleanPreviewFilter = "all"
	cleanPreviewFilterActionablePreview cleanPreviewFilter = "actionable preview"
	cleanPreviewFilterReviewOnly        cleanPreviewFilter = "review-only"
	cleanPreviewFilterDiagnostics       cleanPreviewFilter = "diagnostics"
	cleanPreviewSectionEntryLimit                          = 10
)

func nextCleanPreviewFilter(filter cleanPreviewFilter) cleanPreviewFilter {
	switch filter {
	case cleanPreviewFilterAll:
		return cleanPreviewFilterActionablePreview
	case cleanPreviewFilterActionablePreview:
		return cleanPreviewFilterReviewOnly
	case cleanPreviewFilterReviewOnly:
		return cleanPreviewFilterDiagnostics
	default:
		return cleanPreviewFilterAll
	}
}

func cleanFormatBytes(bytes int64) string {
	return fmt.Sprintf("%d bytes", bytes)
}

type cleanPreviewLoadedMsg struct {
	generation uint64
	model      clean.PreviewReadModel
	canceled   bool
	failed     bool
}

type cleanExecutionState uint8

const (
	cleanExecutionPreview cleanExecutionState = iota
	cleanExecutionConfirmation
	cleanExecutionRunning
	cleanExecutionResult
)

type cleanExecutedMsg struct{ result clean.Result }
type cleanExecutionProgressMsg struct {
	progress clean.ExecutionProgress
	stream   *cleanExecutionStream
}
type cleanExecutionStartedMsg struct{ stream *cleanExecutionStream }
type cleanExecutionStream struct {
	progress <-chan clean.ExecutionProgress
	result   <-chan clean.Result
}

func executeCleanSelectionCmd(ctx context.Context, selected []string) tea.Cmd {
	selected = append([]string(nil), selected...)
	return func() tea.Msg {
		progress := make(chan clean.ExecutionProgress, 4)
		result := make(chan clean.Result, 1)
		stream := &cleanExecutionStream{progress: progress, result: result}
		go func() {
			defer close(progress)
			result <- runCleanSelection(ctx, selected, func(event clean.ExecutionProgress) { progress <- event })
			close(result)
		}()
		return cleanExecutionStartedMsg{stream: stream}
	}
}

func waitCleanExecutionCmd(stream *cleanExecutionStream) tea.Cmd {
	return func() tea.Msg {
		if progress, ok := <-stream.progress; ok {
			return cleanExecutionProgressMsg{progress: progress, stream: stream}
		}
		return cleanExecutedMsg{result: <-stream.result}
	}
}

var runCleanSelection = func(ctx context.Context, selected []string, reporter clean.ProgressReporter) clean.Result {
	config := loadProtectionConfiguration()
	recorder, _ := newHistoryRecorder()
	args := []string{"clean", "--execute"}
	for _, id := range selected {
		args = append(args, "--opt-in", id)
	}
	return executeClean(ctx, clean.Options{
		Validator:                 config.Validator,
		ProtectionDiagnostics:     config.Diagnostics,
		ProtectionLoadError:       config.LoadError,
		HistoryRecorder:           recorder,
		CommandParameters:         history.CommandParameters{Command: "clean", Args: args},
		DetectRunningApplications: clean.DetectSupportedApplications,
		OptIn:                     selected,
		ProgressReporter:          reporter,
	})
}

// loadCleanPreviewCmd runs the existing dry-run command path off the UI loop
// and delivers the shared read model; the TUI never owns cleanup logic.
// Browsing stays free of side effects: no history session is recorded and no
// detailed-list file is written, unlike the `foal clean --dry-run` command.
func loadCleanPreviewCmd(ctx context.Context, generation uint64, selections ...[]string) tea.Cmd {
	return func() tea.Msg {
		var selected []string
		if len(selections) > 0 {
			selected = append([]string(nil), selections[0]...)
		}
		config := loadProtectionConfiguration()
		result := dryRunClean(ctx, clean.Options{
			Validator:                 config.Validator,
			ProtectionDiagnostics:     config.Diagnostics,
			ProtectionLoadError:       config.LoadError,
			DetectRunningApplications: clean.DetectSupportedApplications,
			OptIn:                     selected,
		})
		return cleanPreviewLoadedMsg{
			generation: generation,
			model:      clean.NewPreviewReadModelForSelection(result, selected),
			canceled:   ctx.Err() != nil,
			failed:     result.Status == "error",
		}
	}
}

type cleanModel struct {
	loading           bool
	loadGeneration    uint64
	cancelLoad        context.CancelFunc
	model             clean.PreviewReadModel
	filter            cleanPreviewFilter
	expanded          bool
	notice            string
	vp                viewport.Model
	width             int
	height            int
	selected          map[string]bool
	selectionIndex    int
	previewReady      bool
	executionState    cleanExecutionState
	executionResult   clean.Result
	executionProgress clean.ExecutionProgress
	cancelExecution   context.CancelFunc
}

func newCleanModel(width, height int) cleanModel {
	model := cleanModel{
		loading:  true,
		filter:   cleanPreviewFilterAll,
		notice:   "Press c to copy candidate paths to the clipboard.",
		vp:       viewport.New(),
		selected: make(map[string]bool),
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
	if msg.canceled {
		m.previewReady = false
		m.notice = "Clean preview refresh canceled; selection totals are not ready."
		m.setSize(m.width, m.height)
		return
	}
	if msg.failed {
		m.previewReady = false
		m.notice = "Clean preview refresh failed; selection totals are not ready."
		m.model = msg.model
		m.refreshViewportContent()
		m.setSize(m.width, m.height)
		return
	}
	m.previewReady = true
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
	m.previewReady = false
	if reload {
		m.notice = "Reloading clean preview (dry-run)..."
	}
	m.vp.GotoTop()
	m.setSize(m.width, m.height)
	return loadCleanPreviewCmd(ctx, m.loadGeneration, m.selectedCategoryIDs())
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

func (m *cleanModel) handleKey(key string) tea.Cmd {
	switch key {
	case "enter":
		if m.previewReady && !m.loading {
			m.executionState = cleanExecutionConfirmation
		}
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
	case "tab":
		m.moveSelection(1)
	case "shift+tab":
		m.moveSelection(-1)
	case " ":
		return m.toggleSelectedCategory()
	case "a":
		return m.setAllCategories(true)
	case "x":
		return m.setAllCategories(false)
	default:
		m.notice = "Unknown key. Use tab/space, a/x, j/k, f, e, c, r, b, or q."
	}
	m.setSize(m.width, m.height)
	return nil
}

func (m *cleanModel) selectedCategoryIDs() []string {
	ids := make([]string, 0, len(m.selected))
	for _, category := range m.model.OptInCategories {
		if m.selected[category.Identifier] {
			ids = append(ids, category.Identifier)
		}
	}
	return ids
}

func (m *cleanModel) moveSelection(delta int) {
	if len(m.model.OptInCategories) == 0 {
		return
	}
	m.selectionIndex = (m.selectionIndex + delta + len(m.model.OptInCategories)) % len(m.model.OptInCategories)
	m.refreshViewportContent()
}

func (m *cleanModel) toggleSelectedCategory() tea.Cmd {
	if len(m.model.OptInCategories) == 0 {
		m.notice = "Category catalog is not available yet."
		return nil
	}
	id := m.model.OptInCategories[m.selectionIndex].Identifier
	m.selected[id] = !m.selected[id]
	if !m.selected[id] {
		delete(m.selected, id)
	}
	m.notice = "Selection changed; refreshing shared clean preview..."
	return m.startLoad(false)
}

func (m *cleanModel) setAllCategories(selected bool) tea.Cmd {
	if len(m.model.OptInCategories) == 0 {
		m.notice = "Category catalog is not available yet."
		return nil
	}
	clear(m.selected)
	if selected {
		for _, category := range m.model.OptInCategories {
			m.selected[category.Identifier] = true
		}
	}
	m.notice = "Selection changed; refreshing shared clean preview..."
	return m.startLoad(false)
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

const cleanPreviewFooter = "\nHints: enter confirm | tab category | space toggle | a select all | x clear all | j/k scroll | f filter | e expand | c copy | r refresh | b back | q quit\n"

func (m cleanModel) content() string {
	switch m.executionState {
	case cleanExecutionConfirmation:
		return m.confirmationContent()
	case cleanExecutionRunning:
		return renderCleanExecutionProgress(m.executionProgress)
	case cleanExecutionResult:
		return renderCleanExecutionResult(m.executionResult)
	}
	if m.loading {
		return m.headerContent() + "\nLoading clean preview (dry-run)...\n" + cleanPreviewFooter
	}
	if !m.previewReady {
		return m.headerContent() + "\nClean preview totals are not ready. Change the selection or refresh to retry.\n" + cleanPreviewFooter
	}
	return m.headerContent() + m.vp.View() + "\n" + cleanPreviewFooter
}

func renderCleanExecutionProgress(progress clean.ExecutionProgress) string {
	label := map[clean.ExecutionPhase]string{
		clean.ExecutionPhaseScanning:             "Fresh candidate scanning",
		clean.ExecutionPhaseRecycleBinSafety:     "Aggregate Recycle Bin safety checks",
		clean.ExecutionPhaseRecycleBinOperations: "Recycle Bin operations",
		clean.ExecutionPhaseComplete:             "Completion",
	}[progress.Phase]
	if label == "" {
		label = "Starting shared Clean execution"
	}
	return "Foal Clean\nExecuting Clean through the shared Recycle Bin path...\nProgress: " + label + "\nCancellation does not roll back completed Recycle Bin operations.\n"
}

func (m cleanModel) confirmationContent() string {
	var builder strings.Builder
	optInCount := 0
	builder.WriteString("Foal Clean\nConfirm Clean execution\n")
	builder.WriteString("This confirmation authorizes selected categories, not previewed paths. Execution rescans and validates fresh candidates.\n")
	builder.WriteString("Selected categories:\n")
	for _, category := range m.model.OptInCategories {
		if m.selected[category.Identifier] {
			optInCount += category.CandidateCount
			builder.WriteString("  - " + category.Label + "\n")
		}
	}
	builder.WriteString(fmt.Sprintf("Latest preview: default %d candidate(s), %s; opt-in %d candidate(s), %s.\n",
		m.model.CandidateCount, cleanFormatBytes(m.model.PotentialSpaceBytes),
		optInCount, cleanFormatBytes(m.model.OptInReclaimableBytes)))
	builder.WriteString("Enter: execute | b/Esc: cancel and return to preview\n")
	return builder.String()
}

func renderCleanExecutionResult(result clean.Result) string {
	var builder strings.Builder
	builder.WriteString("Foal Clean\nClean execution result\n")
	builder.WriteString(fmt.Sprintf("Status: %s\nDeleted: %d\nSkipped: %d\nErrors: %d\nDefault deleted: %d\nOpt-in deleted: %d\nAffected bytes: %s\nOpt-in affected bytes: %s\n",
		result.Status, result.Totals.DeletedCount, result.Totals.SkippedCount, len(result.Errors),
		result.Totals.DeletedCount-result.Totals.OptInDeletedCount, result.Totals.OptInDeletedCount,
		cleanFormatBytes(result.Totals.AffectedBytes), cleanFormatBytes(result.Totals.OptInAffectedBytes)))
	for _, skipped := range result.Skipped {
		builder.WriteString(fmt.Sprintf("Skipped boundary: %s (%s: %s)\n", skipped.Path, skipped.Reason.Code, skipped.Reason.Message))
	}
	for _, issue := range result.Errors {
		builder.WriteString(fmt.Sprintf("Error: %s: %s\n", issue.Code, issue.Message))
	}
	builder.WriteString("The fresh execution set may differ from the preview.\n")
	builder.WriteString("b: return to preview | q/Esc: quit\n")
	return builder.String()
}

func (m cleanModel) headerContent() string {
	var builder strings.Builder
	builder.WriteString("Foal Clean\n")
	builder.WriteString("Preview only - no files changed.\n")
	builder.WriteString(fmt.Sprintf("Filter: %s | Scroll: %d%% | Expanded: %t\n", m.filter, int(m.vp.ScrollPercent()*100), m.expanded))
	if len(m.model.OptInCategories) > 0 {
		builder.WriteString(fmt.Sprintf("Category focus: %s\n", m.model.OptInCategories[m.selectionIndex].Label))
	}
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
	if len(model.OptInCategories) > 0 {
		builder.WriteString("\nOpt-in categories\n")
		currentGroup := clean.ReportCategory("")
		for _, category := range model.OptInCategories {
			if category.ReportCategory != currentGroup {
				currentGroup = category.ReportCategory
				builder.WriteString(fmt.Sprintf("  %s\n", currentGroup))
			}
			marker := "[ ]"
			state := fmt.Sprintf("review-only, observed %s", cleanFormatBytes(category.ObservedBytes))
			if category.Selected {
				marker = "[x]"
				state = fmt.Sprintf("selected preview, %d candidate(s), %s opt-in reclaimable", category.CandidateCount, cleanFormatBytes(category.ReclaimableBytes))
			}
			builder.WriteString(fmt.Sprintf("    %s %s (%s)\n", marker, category.Label, state))
		}
	}
	for _, category := range clean.PreviewReportCategories(model, clean.PreviewReportCategoryOptions{
		EntryLimit:                   cleanPreviewSectionEntryLimit,
		Expanded:                     expanded,
		Compact:                      true,
		IncludeCandidates:            cleanPreviewFilterIncludesCandidates(filter),
		IncludeSkipped:               cleanPreviewFilterIncludesSkipped(filter),
		IncludeReview:                cleanPreviewFilterIncludesReview(filter),
		IncludeErrors:                cleanPreviewFilterIncludesDiagnostics(filter),
		IncludeIncompleteInspections: cleanPreviewFilterIncludesInspectionDiagnostics(filter),
		IncludeProtectionDiagnostics: cleanPreviewFilterIncludesProtectionDiagnostics(filter),
		IncludeSummary:               true,
		PreviewSummary:               true,
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

func cleanPreviewFilterIncludesCandidates(filter cleanPreviewFilter) bool {
	return filter == cleanPreviewFilterAll || filter == cleanPreviewFilterActionablePreview
}

func cleanPreviewFilterIncludesSkipped(filter cleanPreviewFilter) bool {
	return filter == cleanPreviewFilterAll || filter == cleanPreviewFilterActionablePreview
}

func cleanPreviewFilterIncludesReview(filter cleanPreviewFilter) bool {
	return filter == cleanPreviewFilterAll || filter == cleanPreviewFilterReviewOnly
}

func cleanPreviewFilterIncludesDiagnostics(filter cleanPreviewFilter) bool {
	return filter == cleanPreviewFilterAll ||
		filter == cleanPreviewFilterActionablePreview ||
		filter == cleanPreviewFilterDiagnostics
}

func cleanPreviewFilterIncludesInspectionDiagnostics(filter cleanPreviewFilter) bool {
	return filter == cleanPreviewFilterAll || filter == cleanPreviewFilterDiagnostics
}

func cleanPreviewFilterIncludesProtectionDiagnostics(filter cleanPreviewFilter) bool {
	return filter == cleanPreviewFilterAll || filter == cleanPreviewFilterDiagnostics
}
