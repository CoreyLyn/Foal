package cli

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
)


// cleanFormatBytes formats byte counts for path-free Clean TUI chrome.
func cleanFormatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 KB"
	}
	if bytes < 1024 {
		return "<1 KB"
	}

	const (
		kilobyte = int64(1024)
		megabyte = 1024 * kilobyte
		gigabyte = 1024 * megabyte
	)

	value := float64(bytes) / float64(kilobyte)
	unit := "KB"
	if bytes >= gigabyte {
		value = float64(bytes) / float64(gigabyte)
		unit = "GB"
	} else if bytes >= megabyte {
		value = float64(bytes) / float64(megabyte)
		unit = "MB"
	}

	formatted := strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0")
	return formatted + " " + unit
}

// cleanExecutionStream carries progress then the final Result for one exact
// Clean execution started from the category-first confirmation path.
type cleanExecutionStream struct {
	progress <-chan clean.ExecutionProgress
	result   <-chan clean.Result
}

// nextEagerGeneration assigns unique Clean session generations so stale stream
// messages from a canceled or superseded session cannot mutate a later model
// that happens to reuse a small per-model counter.
var nextEagerGeneration atomic.Uint64

// Category-first eager preview model for the Clean TUI. This is the primary
// Clean surface after cutover: path-free terminal outcomes, exact per-session
// cleanup selection with live measured totals, focused diagnostics, unavailable
// and no-work classification, a separate confirmation page, one-shot handoff
// into shared exact Clean execution, observation-only path-free category
// progress, cooperative cancellation (ADR 0014), and an authoritative path-free
// result page. The first slice exposes no retry or rescan action.

var eagerPreviewSpinnerFrames = []string{"|", "/", "-", "\\"}

const eagerPreviewTickInterval = 120 * time.Millisecond

// eagerPreviewNav is the navigation intent returned by the category-first model.
type eagerPreviewNav int

const (
	eagerPreviewNavNone eagerPreviewNav = iota
	eagerPreviewNavMenu
	eagerPreviewNavQuit
	eagerPreviewNavInterrupt
)

// eagerCleanPhase is the authorization boundary for category-first Clean.
type eagerCleanPhase int

const (
	eagerPhasePreview eagerCleanPhase = iota
	eagerPhaseConfirmation
	eagerPhaseExecuting
	eagerPhaseResult
)

// cancellationRequestedMessage is shown after the first cooperative Ctrl+C
// during active execution. Completed work is never rolled back; permanent
// mutation that may already have started cannot be undone.
const cancellationRequestedMessage = "Cancellation requested. Completed operations will not be rolled back. Permanent deletion already in progress cannot be undone."

type eagerCategoryRow struct {
	Identifier     string
	Label          string
	ReportCategory clean.ReportCategory
	Eligibility    clean.CategoryEligibility
	// PlannedAction is catalog-owned; the TUI never chooses or overrides it.
	PlannedAction clean.DeletionAction
	State         clean.CategoryPreviewState
	// Selected is session-only cleanup authorization. Independent of cursor.
	// Initial selection is derived from shared eligibility + planned action
	// (defaults and delete_permanently start selected; non-default Recycle Bin
	// opt-ins start unselected). Non-selectable terminal outcomes clear and
	// disable the row for the rest of the session.
	Selected             bool
	CandidateCount       int
	Bytes                int64
	ExcludedSiblingCount int
	ReasonCode           string
	SafetyNote           string
}

type eagerCategoryObservationMsg struct {
	generation  uint64
	observation clean.CategoryPreviewObservation
}

type eagerPreviewFinishedMsg struct {
	generation uint64
	canceled   bool
}

type eagerPreviewUnavailableMsg struct {
	generation  uint64
	unavailable clean.EagerPreviewUnavailable
}

type eagerPreviewTickMsg struct {
	generation uint64
	frame      int
}

type eagerPreviewStream struct {
	events <-chan clean.CategoryPreviewObservation
	done   <-chan eagerPreviewFinishedMsg
}

// runEagerPreviewFn is the shared eager-preview seam. Production uses
// clean.RunEagerPreview; tests replace it with deterministic stubs.
var runEagerPreviewFn = clean.RunEagerPreview

// buildEagerPreviewOptions loads protection configuration for a read-only
// measurement pass. No History, detailed list, cleanup adapter, or Review
// suggestion probe is attached.
var buildEagerPreviewOptions = func() clean.Options {
	config := loadProtectionConfiguration()
	return clean.Options{
		Validator:                 config.Validator,
		ProtectionDiagnostics:     config.Diagnostics,
		ProtectionLoadError:       config.LoadError,
		DetectRunningApplications: clean.DetectSupportedApplications,
	}
}

// runExactCleanSelection is the confirmed TUI exact-execution seam. It compiles
// a path-free exact CategoryPlan, records structured TUI History provenance,
// passes equivalent per-run permanent authorization when the confirmed
// selection disclosed permanent work, and reuses shared Clean Execute (fresh
// resolution, protection, capacity, mixed actions). Tests replace it to assert
// handoff without deletion. It never synthesizes CLI arguments.
var runExactCleanSelection = func(ctx context.Context, selected []string, allowPermanent bool, reporter clean.ProgressReporter) clean.Result {
	plan, err := clean.CompileExactCategoryPlan(selected)
	if err != nil {
		return clean.Result{
			Status: "error",
			Mode:   "execute",
			Errors: []clean.StructuredIssue{{
				Code:        "invalid_category_plan",
				Message:     err.Error(),
				Recoverable: true,
			}},
		}
	}
	config := loadProtectionConfiguration()
	recorder, _ := newHistoryRecorder()
	return executeClean(ctx, clean.Options{
		Validator:                 config.Validator,
		ProtectionDiagnostics:     config.Diagnostics,
		ProtectionLoadError:       config.LoadError,
		HistoryRecorder:           recorder,
		CommandParameters:         exactTUICommandParameters(plan.Categories),
		Plan:                      &plan,
		AllowPermanentDeletion:    allowPermanent,
		DetectRunningApplications: clean.DetectSupportedApplications,
		ProgressReporter:          reporter,
	})
}

// exactTUICommandParameters builds path-free History provenance for a confirmed
// exact TUI plan. It does not synthesize CLI args (ADR 0016).
func exactTUICommandParameters(categories []string) history.CommandParameters {
	return history.CommandParameters{
		Command:            "clean",
		Surface:            "tui",
		SelectionMode:      string(clean.SelectionModeExact),
		SelectedCategories: append([]string(nil), categories...),
	}
}

func executeExactCleanSelectionCmd(ctx context.Context, selected []string, allowPermanent bool) tea.Cmd {
	selected = append([]string(nil), selected...)
	return func() tea.Msg {
		progress := make(chan clean.ExecutionProgress, 4)
		result := make(chan clean.Result, 1)
		stream := &cleanExecutionStream{progress: progress, result: result}
		go func() {
			defer close(progress)
			result <- runExactCleanSelection(ctx, selected, allowPermanent, func(event clean.ExecutionProgress) {
				progress <- event
			})
			close(result)
		}()
		return eagerExactExecutionStartedMsg{stream: stream}
	}
}

func waitEagerExactExecutionCmd(stream *cleanExecutionStream) tea.Cmd {
	return func() tea.Msg {
		if stream == nil {
			return eagerExactExecutedMsg{result: clean.Result{Status: "error", Mode: "execute"}}
		}
		if progress, ok := <-stream.progress; ok {
			return eagerExactExecutionProgressMsg{progress: progress, stream: stream}
		}
		return eagerExactExecutedMsg{result: <-stream.result}
	}
}

type eagerExactExecutionStartedMsg struct{ stream *cleanExecutionStream }
type eagerExactExecutionProgressMsg struct {
	progress clean.ExecutionProgress
	stream   *cleanExecutionStream
}
type eagerExactExecutedMsg struct{ result clean.Result }

// Minimum terminal geometry for a safe actionable/confirmable layout. Below
// this, Clean replaces stage content with Terminal too small guidance rather
// than truncating authorization or confirmation chrome.
const (
	eagerMinTerminalWidth  = 40
	eagerMinTerminalHeight = 12
)

type eagerCleanModel struct {
	generation   uint64
	cancel       context.CancelFunc
	rows         []eagerCategoryRow
	cursor       int
	activeIndex  int // index currently scanning; -1 when idle/finished
	completed    int // terminal outcomes observed
	spinnerFrame int
	startedAt    time.Time
	finished     bool
	canceled     bool
	unavailable  *clean.EagerPreviewUnavailable
	width        int
	height       int
	now          func() time.Time
	nav          eagerPreviewNav

	// previewStream is the active eager observation stream for the current
	// generation. Root continues wait cmds from this handle so observations
	// need not re-embed the stream. Cleared on finish, cancel, or unavailable.
	previewStream *eagerPreviewStream

	// viewportOffset is the first visible line of the scrollable body. Titles,
	// progress, totals, diagnostics, cancellation status, and key hints stay
	// fixed outside this window. There is no separate scroll mode.
	viewportOffset int
	// execFollowActive: when true, execution keeps the active category in view.
	// Manual Up/Down pauses following for temporary inspection; following
	// resumes only when a newly active category lies completely outside.
	execFollowActive  bool
	execTrackedActive int // last known active execution outcome index; -1 none

	phase                 eagerCleanPhase
	frozenCategories      []string
	// frozenAllowPermanent is the per-run permanent authorization disclosed by
	// the strengthened confirmation for the frozen exact selection.
	frozenAllowPermanent  bool
	executionStarted      bool
	executionResult       clean.Result
	executionProgress     clean.ExecutionProgress
	executionOutcomes     []clean.CategoryExecutionOutcome
	cancellationRequested bool
	cancelExecution       context.CancelFunc
}

func newEagerCleanModel(width, height int) eagerCleanModel {
	return newEagerCleanModelFromSummaries(clean.EagerPreviewQueue(), width, height)
}

// newEagerCleanModelFromSummaries builds the category-first model from an
// injected path-free summary queue. Production uses the canonical eager queue;
// tests inject mixed planned actions without activating production rules.
func newEagerCleanModelFromSummaries(queue []clean.CleanupCategorySummary, width, height int) eagerCleanModel {
	return eagerCleanModel{
		rows:              eagerRowsFromSummaries(queue),
		activeIndex:       -1,
		width:             width,
		height:            height,
		now:               time.Now,
		execTrackedActive: -1,
	}
}

func eagerRowsFromSummaries(queue []clean.CleanupCategorySummary) []eagerCategoryRow {
	rows := make([]eagerCategoryRow, 0, len(queue))
	for _, summary := range queue {
		rows = append(rows, eagerCategoryRow{
			Identifier:     summary.Identifier,
			Label:          summary.Label,
			ReportCategory: summary.ReportCategory,
			Eligibility:    summary.Eligibility,
			PlannedAction:  summary.PlannedAction,
			State:          clean.CategoryPreviewWaiting,
			// ADR 0018 / deletion policy: derive initial selection from shared
			// eligibility + planned action (no hard-coded permanent list).
			Selected: clean.InitiallySelectedCategory(summary),
		})
	}
	return rows
}

// setSize applies a terminal resize. It preserves the focused preview row or
// current viewport offset, and never changes category identity or authorization.
func (m *eagerCleanModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.reflowViewportAfterResize()
}

func (m *eagerCleanModel) reflowViewportAfterResize() {
	if m.terminalTooSmall() {
		return
	}
	switch m.phase {
	case eagerPhasePreview:
		if m.unavailable == nil {
			m.ensurePreviewCursorVisible()
		}
	case eagerPhaseExecuting:
		if m.execFollowActive {
			m.ensureExecutionOutcomeVisible(m.executionActiveIndex())
		} else {
			m.clampViewportOffset()
		}
	default:
		m.clampViewportOffset()
	}
}

func (m *eagerCleanModel) start() tea.Cmd {
	m.cancelPending()
	if m.cancelExecution != nil {
		m.cancelExecution()
		m.cancelExecution = nil
	}
	m.generation = nextEagerGeneration.Add(1)
	m.finished = false
	m.canceled = false
	m.completed = 0
	m.activeIndex = -1
	m.spinnerFrame = 0
	m.startedAt = m.now()
	m.nav = eagerPreviewNavNone
	m.unavailable = nil
	m.previewStream = nil
	m.phase = eagerPhasePreview
	m.frozenCategories = nil
	m.frozenAllowPermanent = false
	m.executionStarted = false
	m.executionResult = clean.Result{}
	m.executionProgress = clean.ExecutionProgress{}
	m.executionOutcomes = nil
	m.cancellationRequested = false
	m.viewportOffset = 0
	m.execFollowActive = false
	m.execTrackedActive = -1
	m.cursor = 0
	// Rebuild catalog-derived rows for a fresh session so queue growth/order
	// stays aligned with the shared catalog without a second TUI registry.
	m.rows = eagerRowsFromSummaries(clean.EagerPreviewQueue())

	opts := buildEagerPreviewOptions()
	if unavailable := clean.CheckEagerPreviewAvailability(opts); unavailable != nil {
		// Global pre-scan failure: no queue scan, selection, totals, history.
		m.unavailable = unavailable
		m.finished = true
		m.rows = nil
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	return tea.Batch(
		startEagerPreviewCmd(ctx, m.generation, opts),
		tickEagerPreviewCmd(m.generation, 0),
	)
}

func (m *eagerCleanModel) cancelPending() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.previewStream = nil
}

// applyStarted records the live observation stream for the current generation
// and returns the first wait command. Stale generations are ignored.
func (m *eagerCleanModel) applyStarted(msg eagerPreviewStartedMsg) tea.Cmd {
	if msg.generation != m.generation || m.canceled || m.unavailable != nil {
		return nil
	}
	m.previewStream = msg.stream
	return waitEagerPreviewCmd(msg.generation, msg.stream)
}

// continuePreviewWait returns the next stream wait when a generation still owns
// an active stream. Root calls this after each observation so path-free
// projections keep arriving without re-embedding the stream in every message.
func (m *eagerCleanModel) continuePreviewWait(generation uint64) tea.Cmd {
	if generation != m.generation || m.previewStream == nil || m.canceled || m.unavailable != nil || m.finished {
		return nil
	}
	return waitEagerPreviewCmd(generation, m.previewStream)
}

func startEagerPreviewCmd(ctx context.Context, generation uint64, opts clean.Options) tea.Cmd {
	return func() tea.Msg {
		if unavailable := clean.CheckEagerPreviewAvailability(opts); unavailable != nil {
			return eagerPreviewUnavailableMsg{generation: generation, unavailable: *unavailable}
		}
		events := make(chan clean.CategoryPreviewObservation, 1)
		done := make(chan eagerPreviewFinishedMsg, 1)
		go func() {
			defer close(events)
			err := runEagerPreviewFn(ctx, opts, func(obs clean.CategoryPreviewObservation) {
				select {
				case events <- obs:
				case <-ctx.Done():
				}
			})
			canceled := err != nil && ctx.Err() != nil
			done <- eagerPreviewFinishedMsg{generation: generation, canceled: canceled}
			close(done)
		}()
		return eagerPreviewStartedMsg{generation: generation, stream: &eagerPreviewStream{events: events, done: done}}
	}
}

type eagerPreviewStartedMsg struct {
	generation uint64
	stream     *eagerPreviewStream
}

func waitEagerPreviewCmd(generation uint64, stream *eagerPreviewStream) tea.Cmd {
	return func() tea.Msg {
		if stream == nil {
			return eagerPreviewFinishedMsg{generation: generation, canceled: true}
		}
		obs, ok := <-stream.events
		if ok {
			return eagerCategoryObservationMsg{generation: generation, observation: obs}
		}
		finished := <-stream.done
		return finished
	}
}

func tickEagerPreviewCmd(generation uint64, frame int) tea.Cmd {
	return tea.Tick(eagerPreviewTickInterval, func(time.Time) tea.Msg {
		return eagerPreviewTickMsg{generation: generation, frame: frame + 1}
	})
}

func (m *eagerCleanModel) applyObservation(msg eagerCategoryObservationMsg) {
	if msg.generation != m.generation || m.canceled || m.unavailable != nil {
		return
	}
	index := m.rowIndex(msg.observation.Identifier)
	if index < 0 {
		return
	}
	row := &m.rows[index]
	row.State = msg.observation.State
	row.CandidateCount = msg.observation.CandidateCount
	row.Bytes = msg.observation.Bytes
	row.ExcludedSiblingCount = msg.observation.ExcludedSiblingCount
	row.ReasonCode = msg.observation.ReasonCode
	row.SafetyNote = msg.observation.SafetyNote
	// Empty, skipped, incomplete, and failed cannot remain authorized.
	if !clean.SelectablePreviewOutcome(row.State) {
		row.Selected = false
	}
	switch msg.observation.State {
	case clean.CategoryPreviewScanning:
		m.activeIndex = index
	default:
		if m.activeIndex == index {
			m.activeIndex = -1
		}
		if clean.IsTerminalPreviewState(msg.observation.State) {
			m.completed++
		}
	}
}

func (m *eagerCleanModel) applyFinished(msg eagerPreviewFinishedMsg) {
	if msg.generation != m.generation {
		return
	}
	m.finished = true
	m.canceled = m.canceled || msg.canceled
	m.activeIndex = -1
	m.cancel = nil
	m.previewStream = nil
}

func (m *eagerCleanModel) applyUnavailable(msg eagerPreviewUnavailableMsg) {
	if msg.generation != m.generation {
		return
	}
	m.unavailable = &msg.unavailable
	m.finished = true
	m.activeIndex = -1
	m.rows = nil
	m.completed = 0
	m.cancelPending()
}

func (m *eagerCleanModel) applyTick(msg eagerPreviewTickMsg) tea.Cmd {
	if msg.generation != m.generation || m.unavailable != nil {
		return nil
	}
	// Preview ticks stop after cancel or scan finish; execution keeps the
	// header spinner alive until the authoritative Result arrives.
	switch m.phase {
	case eagerPhaseExecuting:
		// Active execution owns the frame until Result.
	case eagerPhasePreview:
		if m.canceled || m.finished {
			return nil
		}
	default:
		return nil
	}
	m.spinnerFrame = msg.frame
	return tickEagerPreviewCmd(m.generation, msg.frame)
}

func (m *eagerCleanModel) rowIndex(identifier string) int {
	for i, row := range m.rows {
		if row.Identifier == identifier {
			return i
		}
	}
	return -1
}

// allCategoriesTerminal reports whether every scannable category has a terminal
// path-free outcome. Confirmation cannot be enabled while this is false.
func (m eagerCleanModel) allCategoriesTerminal() bool {
	if m.unavailable != nil || len(m.rows) == 0 {
		return false
	}
	for _, row := range m.rows {
		if !clean.IsTerminalPreviewState(row.State) {
			return false
		}
	}
	return true
}

// confirmationEnabled requires every scannable category terminal and a
// non-empty exact selection before the first Enter may open confirmation.
func (m eagerCleanModel) confirmationEnabled() bool {
	return m.unavailable == nil && !m.canceled && m.phase == eagerPhasePreview &&
		m.allCategoriesTerminal() && m.selectedCount() > 0 && !m.executionStarted
}

// noWorkState classifies finished zero-selection presentation.
func (m eagerCleanModel) noWorkState() clean.EagerPreviewNoWorkState {
	if m.unavailable != nil || m.canceled {
		return clean.EagerPreviewNoWorkNone
	}
	observations := make([]clean.CategoryPreviewObservation, len(m.rows))
	for i, row := range m.rows {
		observations[i] = clean.CategoryPreviewObservation{
			Identifier:     row.Identifier,
			State:          row.State,
			CandidateCount: row.CandidateCount,
			Bytes:          row.Bytes,
		}
	}
	return clean.ClassifyEagerPreviewNoWork(observations, m.selectedCount())
}

// selectedCount returns how many categories are currently authorized.
func (m eagerCleanModel) selectedCount() int {
	n := 0
	for _, row := range m.rows {
		if row.Selected {
			n++
		}
	}
	return n
}

// selectedCategoryIDs returns canonical identifiers in stable display/scan
// order. Contains only selected default or opt-in identifiers — no aliases,
// group tokens, permission notices, review evidence, or paths.
func (m eagerCleanModel) selectedCategoryIDs() []string {
	ids := make([]string, 0, len(m.rows))
	for _, row := range m.rows {
		if row.Selected {
			ids = append(ids, row.Identifier)
		}
	}
	return ids
}

// selectionTotals returns selected category count, safely measured bytes for
// complete/partial selected rows, and selected waiting/scanning pending count.
// Unfinished, empty, skipped, incomplete, and failed work contributes no bytes.
func (m eagerCleanModel) selectionTotals() (categories int, measuredBytes int64, pending int) {
	for _, row := range m.rows {
		if !row.Selected {
			continue
		}
		categories++
		switch row.State {
		case clean.CategoryPreviewWaiting, clean.CategoryPreviewScanning:
			pending++
		case clean.CategoryPreviewComplete, clean.CategoryPreviewPartial:
			measuredBytes += row.Bytes
		}
	}
	return categories, measuredBytes, pending
}

// rowSelectable reports whether Space may toggle the row. Waiting, scanning,
// complete, and partial are selectable; disabled terminal outcomes are not.
func (m eagerCleanModel) rowSelectable(row eagerCategoryRow) bool {
	return clean.SelectablePreviewOutcome(row.State)
}

// toggleFocusedSelection toggles the focused row when selectable. Never
// restarts scanning and never mutates disabled diagnostics.
func (m *eagerCleanModel) toggleFocusedSelection() {
	if m.unavailable != nil || len(m.rows) == 0 {
		return
	}
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	if !m.rowSelectable(m.rows[m.cursor]) {
		return
	}
	m.rows[m.cursor].Selected = !m.rows[m.cursor].Selected
}

// selectAllSelectable authorizes every currently selectable waiting, scanning,
// complete, or partial category. Permission notices are not in the queue;
// disabled terminal rows stay excluded.
func (m *eagerCleanModel) selectAllSelectable() {
	for i := range m.rows {
		if m.rowSelectable(m.rows[i]) {
			m.rows[i].Selected = true
		}
	}
}

// clearSelection clears every selection, including default categories.
func (m *eagerCleanModel) clearSelection() {
	for i := range m.rows {
		m.rows[i].Selected = false
	}
}

// focusedRow returns the cursor row when browsing the category list.
func (m eagerCleanModel) focusedRow() (eagerCategoryRow, bool) {
	if m.unavailable != nil || len(m.rows) == 0 {
		return eagerCategoryRow{}, false
	}
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return eagerCategoryRow{}, false
	}
	return m.rows[m.cursor], true
}

// handleKey processes category-first keys. Preview browsing and selection never
// restart the queue, write History, or call execution. First Enter opens the
// confirmation page when enabled; second Enter freezes the exact selection and
// invokes shared Clean once. Escape/b from confirmation returns to preview
// without rescanning.
func (m *eagerCleanModel) handleKey(key string) (eagerPreviewNav, tea.Cmd) {
	if m.unavailable != nil {
		switch key {
		case "ctrl+c":
			m.nav = eagerPreviewNavInterrupt
			return m.nav, nil
		case "q":
			m.nav = eagerPreviewNavQuit
			return m.nav, nil
		case "enter", "esc", "b", "escape":
			m.nav = eagerPreviewNavMenu
			return m.nav, nil
		default:
			return eagerPreviewNavNone, nil
		}
	}

	switch m.phase {
	case eagerPhaseExecuting:
		// ADR 0014: only Ctrl+C requests cooperative cancel. Escape, b, and q
		// are ignored; repeated Ctrl+C does not force exit. Stay attached until
		// the final Result and normal History complete. Undersized terminals
		// keep the same no-abandon rule.
		switch key {
		case "ctrl+c":
			m.cancellationRequested = true
			if m.cancelExecution != nil {
				m.cancelExecution()
			}
		case "up", "k":
			// Temporary inspection: pause active following without changing auth.
			m.execFollowActive = false
			m.scrollViewport(-1)
		case "down", "j":
			m.execFollowActive = false
			m.scrollViewport(1)
		}
		return eagerPreviewNavNone, nil
	case eagerPhaseResult:
		switch key {
		case "ctrl+c":
			m.nav = eagerPreviewNavInterrupt
			return m.nav, nil
		case "q":
			m.nav = eagerPreviewNavQuit
			return m.nav, nil
		case "enter", "esc", "b", "escape":
			m.nav = eagerPreviewNavMenu
			return m.nav, nil
		case "up", "k":
			// Non-selectable content: Up/Down only adjust viewport offset.
			m.scrollViewport(-1)
			return eagerPreviewNavNone, nil
		case "down", "j":
			m.scrollViewport(1)
			return eagerPreviewNavNone, nil
		default:
			return eagerPreviewNavNone, nil
		}
	case eagerPhaseConfirmation:
		switch key {
		case "ctrl+c":
			m.cancelPending()
			m.canceled = true
			m.nav = eagerPreviewNavInterrupt
			return m.nav, nil
		case "q":
			m.cancelPending()
			m.canceled = true
			m.nav = eagerPreviewNavQuit
			return m.nav, nil
		case "esc", "b", "escape":
			// Preserve in-memory scan and selection; do not rescan.
			m.phase = eagerPhasePreview
			m.viewportOffset = 0
			m.ensurePreviewCursorVisible()
			return eagerPreviewNavNone, nil
		case "enter":
			return m.beginExactExecution()
		case "up", "k":
			// Non-selectable content: Up/Down only adjust viewport offset.
			m.scrollViewport(-1)
			return eagerPreviewNavNone, nil
		case "down", "j":
			m.scrollViewport(1)
			return eagerPreviewNavNone, nil
		default:
			return eagerPreviewNavNone, nil
		}
	}

	// Preview phase. Undersized terminals still honor return/quit keys.
	switch key {
	case "ctrl+c":
		m.cancelPending()
		m.canceled = true
		m.nav = eagerPreviewNavInterrupt
		return m.nav, nil
	case "q":
		m.cancelPending()
		m.canceled = true
		m.nav = eagerPreviewNavQuit
		return m.nav, nil
	case "esc", "b", "escape":
		m.cancelPending()
		m.canceled = true
		m.nav = eagerPreviewNavMenu
		return m.nav, nil
	case "enter":
		if m.confirmationEnabled() {
			m.phase = eagerPhaseConfirmation
			m.viewportOffset = 0
		}
		return eagerPreviewNavNone, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.ensurePreviewCursorVisible()
		}
	case "down", "j":
		if m.cursor+1 < len(m.rows) {
			m.cursor++
			m.ensurePreviewCursorVisible()
		}
	case " ", "space":
		// Toggle only; never returns a scan command.
		m.toggleFocusedSelection()
	case "a":
		m.selectAllSelectable()
	case "x":
		m.clearSelection()
	}
	return eagerPreviewNavNone, nil
}

// beginExactExecution freezes the confirmed selection and starts shared Clean
// exactly once. Repeated input cannot start a second execution.
func (m *eagerCleanModel) beginExactExecution() (eagerPreviewNav, tea.Cmd) {
	if m.executionStarted || m.phase == eagerPhaseExecuting {
		return eagerPreviewNavNone, nil
	}
	ids := m.selectedCategoryIDs()
	if len(ids) == 0 {
		return eagerPreviewNavNone, nil
	}
	allowPermanent := m.selectionIncludesPermanent()
	plan, err := clean.CompileExactCategoryPlan(ids)
	if err != nil {
		// Selection is catalog-derived; reject before any cleanup work.
		m.executionStarted = true
		m.frozenCategories = append([]string(nil), ids...)
		m.frozenAllowPermanent = allowPermanent
		m.executionResult = clean.Result{
			Status: "error",
			Mode:   "execute",
			Errors: []clean.StructuredIssue{{
				Code:        "invalid_category_plan",
				Message:     err.Error(),
				Recoverable: true,
			}},
		}
		m.executionOutcomes = clean.ProjectCategoryExecutionOutcomes(m.frozenCategories, m.executionResult)
		m.phase = eagerPhaseResult
		return eagerPreviewNavNone, nil
	}
	m.frozenCategories = append([]string(nil), plan.Categories...)
	m.frozenAllowPermanent = allowPermanent
	m.executionStarted = true
	m.cancellationRequested = false
	m.executionOutcomes = nil
	m.executionProgress = clean.ExecutionProgress{}
	m.phase = eagerPhaseExecuting
	// Seed in-progress rows from the frozen exact selection only.
	m.executionOutcomes = initialExecutionOutcomes(m.frozenCategories)
	m.viewportOffset = 0
	m.execFollowActive = true
	m.execTrackedActive = -1
	m.syncExecutionViewport()
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelExecution = cancel
	// Single command keeps the Bubble Tea handoff seam simple for tests and
	// production; the header spinner advances on progress observations and ticks.
	return eagerPreviewNavNone, executeExactCleanSelectionCmd(ctx, m.frozenCategories, m.frozenAllowPermanent)
}

// selectionIncludesPermanent reports whether the current exact selection
// discloses any delete_permanently planned action. Used for confirmation
// grouping and equivalent per-run permanent authorization.
func (m eagerCleanModel) selectionIncludesPermanent() bool {
	for _, row := range m.rows {
		if row.Selected && row.PlannedAction == clean.DeletionActionDeletePermanently {
			return true
		}
	}
	return false
}

// confirmationActionGroups splits the exact selection into Permanent deletion
// and Recycle Bin work. Empty groups are omitted. Action is catalog-owned.
func (m eagerCleanModel) confirmationActionGroups() (permanent, recycle []eagerCategoryRow) {
	for _, row := range m.rows {
		if !row.Selected {
			continue
		}
		switch row.PlannedAction {
		case clean.DeletionActionDeletePermanently:
			permanent = append(permanent, row)
		default:
			// Missing/unknown action is treated as Recycle Bin presentation only
			// for confirmation totals; shared Clean remains authoritative.
			recycle = append(recycle, row)
		}
	}
	return permanent, recycle
}

func confirmationGroupTotals(rows []eagerCategoryRow) (categories, candidates int, bytes int64) {
	for _, row := range rows {
		categories++
		candidates += row.CandidateCount
		bytes += row.Bytes
	}
	return categories, candidates, bytes
}

// initialExecutionOutcomes builds path-free in-progress rows for the frozen
// selection. Authorization cannot change after this snapshot.
func initialExecutionOutcomes(frozen []string) []clean.CategoryExecutionOutcome {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	out := make([]clean.CategoryExecutionOutcome, 0, len(frozen))
	for _, id := range frozen {
		label := id
		if summary, ok := catalog.Summary(id); ok {
			label = summary.Label
		}
		out = append(out, clean.CategoryExecutionOutcome{
			Identifier: id,
			Label:      label,
			State:      clean.InProgressExecutionState(""),
		})
	}
	return out
}

func (m *eagerCleanModel) applyExactExecutionStarted(msg eagerExactExecutionStartedMsg) tea.Cmd {
	if m.phase != eagerPhaseExecuting {
		return nil
	}
	return waitEagerExactExecutionCmd(msg.stream)
}

func (m *eagerCleanModel) applyExactExecutionProgress(msg eagerExactExecutionProgressMsg) tea.Cmd {
	if m.phase != eagerPhaseExecuting {
		return nil
	}
	m.executionProgress = msg.progress
	m.spinnerFrame++
	// Observation-only: project the shared phase onto every selected row that
	// is still in progress. Progress never authorizes work or changes Result.
	inProgress := clean.InProgressExecutionState(msg.progress.Phase)
	for i := range m.executionOutcomes {
		if !clean.IsTerminalExecutionState(m.executionOutcomes[i].State) {
			m.executionOutcomes[i].State = inProgress
		}
	}
	m.syncExecutionViewport()
	return waitEagerExactExecutionCmd(msg.stream)
}

func (m *eagerCleanModel) applyExactExecuted(msg eagerExactExecutedMsg) {
	if m.phase != eagerPhaseExecuting {
		return
	}
	m.executionResult = msg.result
	// Terminal outcomes come only from the authoritative final Result over the
	// frozen selection. Item-level Result/History remain the source of truth.
	m.executionOutcomes = clean.ProjectCategoryExecutionOutcomes(m.frozenCategories, msg.result)
	m.phase = eagerPhaseResult
	m.viewportOffset = 0
	m.execFollowActive = false
	m.execTrackedActive = -1
	if m.cancelExecution != nil {
		m.cancelExecution = nil
	}
}

func (m eagerCleanModel) content() string {
	if m.terminalTooSmall() {
		return m.tooSmallContent()
	}
	if m.unavailable != nil {
		return m.unavailableContent()
	}

	header := m.fixedHeaderLines()
	footer := m.fixedFooterLines()
	body := m.scrollableBodyLines()
	// height <= 0 means unconstrained (deterministic model tests without a
	// WindowSize). Otherwise body fills remaining rows under fixed chrome.
	capacity := len(body)
	if m.height > 0 {
		capacity = m.height - len(header) - len(footer)
		if capacity < 1 {
			return m.tooSmallContent()
		}
	}
	offset := m.clampedViewportOffset(len(body), capacity)
	end := offset + capacity
	if end > len(body) {
		end = len(body)
	}

	parts := make([]string, 0, len(header)+capacity+len(footer))
	parts = append(parts, header...)
	if len(body) > 0 {
		parts = append(parts, body[offset:end]...)
	}
	parts = append(parts, footer...)
	return strings.Join(parts, "\n") + "\n"
}

// terminalTooSmall reports whether the terminal cannot host the minimum safe
// fixed chrome plus one body line without truncating actionable layout.
func (m eagerCleanModel) terminalTooSmall() bool {
	if m.width > 0 && m.width < eagerMinTerminalWidth {
		return true
	}
	if m.height <= 0 {
		// Zero height is treated as unconstrained for pure model tests that
		// only assert textual contracts; production always receives WindowSize.
		return false
	}
	if m.height < eagerMinTerminalHeight {
		return true
	}
	// Dynamic chrome may still exceed the floor on multi-line diagnostics.
	headerN := len(m.fixedHeaderLines())
	footerN := len(m.fixedFooterLines())
	return m.height < headerN+footerN+1
}

func (m eagerCleanModel) tooSmallContent() string {
	return strings.Join([]string{
		"Terminal too small",
		"Resize the terminal larger to continue using Clean.",
		m.tooSmallHints(),
	}, "\n") + "\n"
}

func (m eagerCleanModel) tooSmallHints() string {
	if m.unavailable != nil {
		return "Hints: Enter/Esc/b menu · q quit"
	}
	switch m.phase {
	case eagerPhaseExecuting:
		if m.cancellationRequested {
			return "Waiting for final Result. Escape, b, and q are inactive."
		}
		return "Hints: Ctrl+C cancel · Escape/b/q inactive during execution"
	case eagerPhaseResult:
		return "Hints: Enter/Esc/b menu · q quit"
	case eagerPhaseConfirmation:
		return "Hints: b/Esc back · q quit"
	default:
		return "Hints: b/Esc back · q quit"
	}
}

// fixedHeaderLines are titles and phase/progress fixed above the body.
func (m eagerCleanModel) fixedHeaderLines() []string {
	if m.unavailable != nil {
		return []string{"Clean unavailable"}
	}
	switch m.phase {
	case eagerPhaseConfirmation:
		return []string{"Foal Clean", "Confirm cleanup", ""}
	case eagerPhaseExecuting:
		return []string{"Foal Clean", m.executionHeaderLine(), ""}
	case eagerPhaseResult:
		return []string{"Foal Clean", "Cleanup result", ""}
	default:
		return []string{"Foal Clean", m.headerLine(), ""}
	}
}

// fixedFooterLines hold totals, diagnostics, cancellation status, and key hints.
func (m eagerCleanModel) fixedFooterLines() []string {
	if m.unavailable != nil {
		code := "unavailable"
		message := "Clean cannot start."
		if m.unavailable.Code != "" {
			code = m.unavailable.Code
		}
		if m.unavailable.Message != "" {
			message = m.unavailable.Message
		}
		return []string{
			fmt.Sprintf("Code: %s", code),
			message,
			"",
			"Hints: Enter/Esc/b menu · q quit",
		}
	}
	switch m.phase {
	case eagerPhaseConfirmation:
		return m.confirmationFooterLines()
	case eagerPhaseExecuting:
		lines := []string{""}
		if m.cancellationRequested {
			lines = append(lines, cancellationRequestedMessage)
		}
		lines = append(lines, m.executionFooterLine())
		if m.cancellationRequested {
			lines = append(lines, "Waiting for final Result. Escape, b, and q are inactive.")
		} else {
			lines = append(lines, "Hints: Ctrl+C cancel · Escape/b/q inactive during execution")
		}
		return lines
	case eagerPhaseResult:
		return m.resultFooterLines()
	default:
		lines := []string{"", m.selectionSummaryLine()}
		lines = append(lines, strings.Split(m.focusedDetailPanel(), "\n")...)
		lines = append(lines, strings.Split(m.footerHints(), "\n")...)
		return lines
	}
}

// scrollableBodyLines are the central content. Group headings travel with their
// category rows; there is no independent heading freeze or scroll mode.
func (m eagerCleanModel) scrollableBodyLines() []string {
	lines := m.scrollableBodyEntries()
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line.text
	}
	return out
}

type eagerBodyLine struct {
	text         string
	rowIndex     int // preview row index; -1 when not a category row
	outcomeIndex int // execution/result outcome index; -1 when not an outcome row
}

func (m eagerCleanModel) scrollableBodyEntries() []eagerBodyLine {
	if m.unavailable != nil {
		return nil
	}
	switch m.phase {
	case eagerPhaseConfirmation:
		return m.confirmationBodyEntries()
	case eagerPhaseExecuting, eagerPhaseResult:
		lines := make([]eagerBodyLine, 0, len(m.executionOutcomes))
		for i, outcome := range m.executionOutcomes {
			lines = append(lines, eagerBodyLine{
				text:         fmt.Sprintf("  %s %s", m.executionRowMarker(outcome.State), m.executionRowLabel(outcome)),
				rowIndex:     -1,
				outcomeIndex: i,
			})
		}
		return lines
	default:
		lines := make([]eagerBodyLine, 0, len(m.rows)+4)
		currentGroup := clean.ReportCategory("")
		for i, row := range m.rows {
			if row.ReportCategory != currentGroup {
				currentGroup = row.ReportCategory
				lines = append(lines, eagerBodyLine{
					text:         fmt.Sprintf("  %s", currentGroup),
					rowIndex:     -1,
					outcomeIndex: -1,
				})
			}
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			// Cursor (>) and checkbox ([x]/[ ]) stay independent of scan markers.
			lines = append(lines, eagerBodyLine{
				text:         fmt.Sprintf("  %s %s %s %s", cursor, m.checkbox(row), m.rowMarker(row), m.rowLabel(row)),
				rowIndex:     i,
				outcomeIndex: -1,
			})
		}
		return lines
	}
}

func (m eagerCleanModel) bodyCapacity() int {
	if m.terminalTooSmall() || m.height <= 0 {
		if m.height <= 0 {
			// Unconstrained: treat capacity as large enough for full body.
			return len(m.scrollableBodyLines()) + 1
		}
		return 0
	}
	cap := m.height - len(m.fixedHeaderLines()) - len(m.fixedFooterLines())
	if cap < 0 {
		return 0
	}
	return cap
}

func (m eagerCleanModel) clampedViewportOffset(bodyLen, capacity int) int {
	if capacity <= 0 || bodyLen <= 0 {
		return 0
	}
	maxOffset := bodyLen - capacity
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := m.viewportOffset
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func (m *eagerCleanModel) clampViewportOffset() {
	cap := m.bodyCapacity()
	bodyLen := len(m.scrollableBodyLines())
	m.viewportOffset = m.clampedViewportOffset(bodyLen, cap)
}

func (m *eagerCleanModel) scrollViewport(delta int) {
	if m.terminalTooSmall() {
		return
	}
	m.viewportOffset += delta
	m.clampViewportOffset()
}

func (m *eagerCleanModel) ensurePreviewCursorVisible() {
	if m.unavailable != nil || len(m.rows) == 0 {
		m.clampViewportOffset()
		return
	}
	line := m.previewBodyLineForRow(m.cursor)
	m.ensureBodyLineVisible(line)
}

func (m eagerCleanModel) previewBodyLineForRow(rowIndex int) int {
	for i, line := range m.scrollableBodyEntries() {
		if line.rowIndex == rowIndex {
			return i
		}
	}
	return -1
}

func (m *eagerCleanModel) ensureBodyLineVisible(lineIndex int) {
	if lineIndex < 0 {
		m.clampViewportOffset()
		return
	}
	cap := m.bodyCapacity()
	if cap <= 0 {
		return
	}
	if lineIndex < m.viewportOffset {
		m.viewportOffset = lineIndex
	} else if lineIndex >= m.viewportOffset+cap {
		m.viewportOffset = lineIndex - cap + 1
	}
	m.clampViewportOffset()
}

func (m eagerCleanModel) executionActiveIndex() int {
	for i, outcome := range m.executionOutcomes {
		if !clean.IsTerminalExecutionState(outcome.State) {
			return i
		}
	}
	return -1
}

func (m eagerCleanModel) executionOutcomeBodyLine(outcomeIndex int) int {
	for i, line := range m.scrollableBodyEntries() {
		if line.outcomeIndex == outcomeIndex {
			return i
		}
	}
	return -1
}

func (m *eagerCleanModel) ensureExecutionOutcomeVisible(outcomeIndex int) {
	if outcomeIndex < 0 {
		m.clampViewportOffset()
		return
	}
	m.ensureBodyLineVisible(m.executionOutcomeBodyLine(outcomeIndex))
}

func (m eagerCleanModel) executionOutcomeCompletelyOutside(outcomeIndex int) bool {
	line := m.executionOutcomeBodyLine(outcomeIndex)
	if line < 0 {
		return false
	}
	cap := m.bodyCapacity()
	if cap <= 0 {
		return true
	}
	offset := m.clampedViewportOffset(len(m.scrollableBodyLines()), cap)
	return line < offset || line >= offset+cap
}

// syncExecutionViewport follows the active category by default and brings a
// newly active category into view only when it is completely outside the
// current viewport after temporary inspection.
func (m *eagerCleanModel) syncExecutionViewport() {
	if m.phase != eagerPhaseExecuting {
		return
	}
	active := m.executionActiveIndex()
	if active < 0 {
		m.execTrackedActive = -1
		return
	}
	newlyActive := active != m.execTrackedActive
	m.execTrackedActive = active
	if m.execFollowActive {
		m.ensureExecutionOutcomeVisible(active)
		return
	}
	if newlyActive && m.executionOutcomeCompletelyOutside(active) {
		m.execFollowActive = true
		m.ensureExecutionOutcomeVisible(active)
	}
}

func (m eagerCleanModel) executionHeaderLine() string {
	spinner := eagerPreviewSpinnerFrames[m.spinnerFrame%len(eagerPreviewSpinnerFrames)]
	return fmt.Sprintf("%s %s", spinner, sharedExecutionPhaseLabel(m.executionProgress.Phase))
}

func sharedExecutionPhaseLabel(phase clean.ExecutionPhase) string {
	switch phase {
	case clean.ExecutionPhaseScanning:
		return "Fresh scanning"
	case clean.ExecutionPhaseRecycleBinSafety:
		return "Recycle Bin safety check"
	case clean.ExecutionPhaseRecycleBinOperations:
		return "Moving to Recycle Bin"
	case clean.ExecutionPhasePermanentOperations:
		return "Permanent deletion"
	case clean.ExecutionPhaseComplete:
		return "Completion"
	default:
		return "Fresh scanning"
	}
}

func (m eagerCleanModel) confirmationBodyEntries() []eagerBodyLine {
	permanent, recycle := m.confirmationActionGroups()
	lines := make([]eagerBodyLine, 0, len(permanent)+len(recycle)+8)
	appendGroup := func(title string, rows []eagerCategoryRow) {
		if len(rows) == 0 {
			return
		}
		cats, candidates, bytes := confirmationGroupTotals(rows)
		lines = append(lines, eagerBodyLine{
			text:         fmt.Sprintf("%s · %d categories · %d item(s) · %s", title, cats, candidates, cleanFormatBytes(bytes)),
			rowIndex:     -1,
			outcomeIndex: -1,
		})
		for _, row := range rows {
			action := clean.DeletionActionLabel(row.PlannedAction)
			lines = append(lines, eagerBodyLine{
				text: fmt.Sprintf("  - %s · %d item(s) · %s · %s",
					row.Label, row.CandidateCount, cleanFormatBytes(row.Bytes), action),
				rowIndex:     -1,
				outcomeIndex: -1,
			})
			if row.SafetyNote != "" {
				lines = append(lines, eagerBodyLine{
					text:         "      Impact: " + row.SafetyNote,
					rowIndex:     -1,
					outcomeIndex: -1,
				})
			}
		}
	}
	appendGroup("Permanent deletion", permanent)
	appendGroup("Recycle Bin", recycle)
	return lines
}

func (m eagerCleanModel) confirmationFooterLines() []string {
	n, measured, _ := m.selectionTotals()
	lines := []string{
		"",
		fmt.Sprintf("Selected: %d categories · %s", n, cleanFormatBytes(measured)),
		"Latest preview total only. Execution resolves fresh state; candidates or bytes may differ from preview.",
		"Fresh execution cannot introduce a deletion action type that was not disclosed above.",
	}
	if m.selectionIncludesPermanent() {
		lines = append(lines, "Permanent deletion is irreversible and cannot be recovered from the Recycle Bin.")
	}
	// Recycle Bin-only wording when that group is present; never claim free space.
	if _, recycle := m.confirmationActionGroups(); len(recycle) > 0 {
		lines = append(lines, "Recycle Bin items are moved, not permanently erased.")
	}
	lines = append(lines, "", "Enter: execute | b/Esc: back to preview")
	return lines
}

func (m eagerCleanModel) resultTotals() (recycle, permanent, affected int64) {
	recycle = m.executionResult.Totals.RecycleBinMovedBytes
	permanent = m.executionResult.Totals.PermanentlyDeletedBytes
	affected = m.executionResult.Totals.AffectedBytes
	if recycle == 0 && permanent == 0 && affected == 0 {
		recycle = clean.SumExecutionRecycleBinMovedBytes(m.executionOutcomes)
		permanent = clean.SumExecutionPermanentlyDeletedBytes(m.executionOutcomes)
		affected = clean.SumExecutionAffectedBytes(m.executionOutcomes)
	}
	if affected == 0 {
		affected = recycle + permanent
	}
	return recycle, permanent, affected
}

func (m eagerCleanModel) resultFooterLines() []string {
	recycle, permanent, affected := m.resultTotals()
	terminal := clean.CountTerminalExecutionOutcomes(m.executionOutcomes)
	total := len(m.executionOutcomes)
	lines := []string{
		"",
		fmt.Sprintf("Processed: %d/%d", terminal, total),
		fmt.Sprintf("Recycle Bin moved: %s", cleanFormatBytes(recycle)),
		fmt.Sprintf("Permanently deleted: %s", cleanFormatBytes(permanent)),
		// Aggregate is processed content, never "freed" or "reclaimed" disk space.
		fmt.Sprintf("Affected (processed): %s", cleanFormatBytes(affected)),
	}
	if clean.ResultHasPermanentPartialRisk(m.executionResult) {
		lines = append(lines, clean.PermanentPartialRiskWarning)
	}
	lines = append(lines, "", "Enter/Esc/b: menu · q: quit")
	return lines
}

func (m eagerCleanModel) executionFooterLine() string {
	terminal := clean.CountTerminalExecutionOutcomes(m.executionOutcomes)
	total := len(m.executionOutcomes)
	// In-progress progress cannot invent successful move bytes; only the final
	// Result (and its projection) contributes totals.
	if m.phase != eagerPhaseResult {
		return fmt.Sprintf("Processed: %d/%d · Affected (processed): 0 KB", terminal, total)
	}
	recycle, permanent, affected := m.resultTotals()
	return fmt.Sprintf(
		"Processed: %d/%d · Recycle Bin moved: %s · Permanently deleted: %s · Affected (processed): %s",
		terminal, total, cleanFormatBytes(recycle), cleanFormatBytes(permanent), cleanFormatBytes(affected),
	)
}

func (m eagerCleanModel) executionRowMarker(state clean.CategoryExecutionState) string {
	switch state {
	case clean.CategoryExecutionRechecking, clean.CategoryExecutionReady, clean.CategoryExecutionCleaning:
		return eagerPreviewSpinnerFrames[m.spinnerFrame%len(eagerPreviewSpinnerFrames)]
	case clean.CategoryExecutionCleaned:
		return "✓"
	case clean.CategoryExecutionEmpty:
		return "–"
	case clean.CategoryExecutionSkipped:
		return "⊘"
	case clean.CategoryExecutionPartial, clean.CategoryExecutionFailed, clean.CategoryExecutionCanceled:
		return "!"
	default:
		return "!"
	}
}

func (m eagerCleanModel) executionRowLabel(outcome clean.CategoryExecutionOutcome) string {
	switch outcome.State {
	case clean.CategoryExecutionRechecking:
		return outcome.Label + " · rechecking"
	case clean.CategoryExecutionReady:
		return outcome.Label + " · ready"
	case clean.CategoryExecutionCleaning:
		return outcome.Label + " · cleaning"
	case clean.CategoryExecutionEmpty:
		return outcome.Label + " · empty"
	case clean.CategoryExecutionCleaned:
		return fmt.Sprintf("%s · cleaned · %s", outcome.Label, cleanFormatBytes(outcome.AffectedBytes))
	case clean.CategoryExecutionPartial:
		return fmt.Sprintf("%s · partial · %s", outcome.Label, cleanFormatBytes(outcome.AffectedBytes))
	case clean.CategoryExecutionSkipped:
		return outcome.Label + " · skipped"
	case clean.CategoryExecutionFailed:
		return outcome.Label + " · failed"
	case clean.CategoryExecutionCanceled:
		return outcome.Label + " · canceled"
	default:
		return outcome.Label
	}
}

func (m eagerCleanModel) checkbox(row eagerCategoryRow) string {
	if row.Selected {
		return "[x]"
	}
	return "[ ]"
}

// selectionSummaryLine shows live selected totals. Unfinished selected work is
// pending, never guessed as zero bytes. After every selected category is
// terminal the line collapses to count and measured total.
func (m eagerCleanModel) selectionSummaryLine() string {
	n, measured, pending := m.selectionTotals()
	if pending > 0 {
		return fmt.Sprintf("Selected: %d categories · %s measured · %d pending", n, cleanFormatBytes(measured), pending)
	}
	return fmt.Sprintf("Selected: %d categories · %s", n, cleanFormatBytes(measured))
}

func (m eagerCleanModel) unavailableContent() string {
	code := "unavailable"
	message := "Clean cannot start."
	if m.unavailable != nil {
		if m.unavailable.Code != "" {
			code = m.unavailable.Code
		}
		if m.unavailable.Message != "" {
			message = m.unavailable.Message
		}
	}
	return strings.Join([]string{
		"Clean unavailable",
		fmt.Sprintf("Code: %s", code),
		message,
		"",
		"Hints: Enter/Esc/b menu · q quit",
		"",
	}, "\n")
}

func (m eagerCleanModel) headerLine() string {
	total := len(m.rows)
	elapsed := m.elapsedLabel()
	if m.canceled {
		return fmt.Sprintf("Canceled · %s", elapsed)
	}
	if m.finished && m.allCategoriesTerminal() {
		return fmt.Sprintf("Scan complete · %d/%d · %s", total, total, elapsed)
	}
	// Scanning n/N: n is the 1-based index of the active category, or completed+1.
	n := m.completed + 1
	if m.activeIndex >= 0 {
		n = m.activeIndex + 1
	}
	if n > total {
		n = total
	}
	if total == 0 {
		return fmt.Sprintf("Scanning 0/0 · %s", elapsed)
	}
	if !m.allCategoriesTerminal() {
		return fmt.Sprintf("Scanning %d/%d · Confirmation available after scan completes · %s", n, total, elapsed)
	}
	return fmt.Sprintf("Scanning %d/%d · %s", n, total, elapsed)
}

func (m eagerCleanModel) elapsedLabel() string {
	if m.startedAt.IsZero() {
		return "0s"
	}
	now := m.now
	if now == nil {
		now = time.Now
	}
	seconds := int(now().Sub(m.startedAt).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%ds", seconds)
}

func (m eagerCleanModel) rowMarker(row eagerCategoryRow) string {
	switch row.State {
	case clean.CategoryPreviewWaiting:
		return "…"
	case clean.CategoryPreviewScanning:
		return eagerPreviewSpinnerFrames[m.spinnerFrame%len(eagerPreviewSpinnerFrames)]
	case clean.CategoryPreviewComplete:
		return "✓"
	case clean.CategoryPreviewEmpty:
		return "–"
	case clean.CategoryPreviewSkipped:
		return "⊘"
	case clean.CategoryPreviewPartial, clean.CategoryPreviewIncomplete, clean.CategoryPreviewFailed:
		return "!"
	default:
		return "!"
	}
}

func (m eagerCleanModel) rowLabel(row eagerCategoryRow) string {
	switch row.State {
	case clean.CategoryPreviewComplete:
		return fmt.Sprintf("%s · %d item(s) · %s", row.Label, row.CandidateCount, cleanFormatBytes(row.Bytes))
	case clean.CategoryPreviewPartial:
		return fmt.Sprintf("%s · %d item(s) · %s · partial", row.Label, row.CandidateCount, cleanFormatBytes(row.Bytes))
	case clean.CategoryPreviewEmpty:
		return fmt.Sprintf("%s · empty", row.Label)
	case clean.CategoryPreviewSkipped:
		return fmt.Sprintf("%s · skipped", row.Label)
	case clean.CategoryPreviewIncomplete:
		return fmt.Sprintf("%s · incomplete", row.Label)
	case clean.CategoryPreviewFailed:
		return fmt.Sprintf("%s · failed", row.Label)
	case clean.CategoryPreviewWaiting:
		return fmt.Sprintf("%s · waiting", row.Label)
	case clean.CategoryPreviewScanning:
		return fmt.Sprintf("%s · scanning", row.Label)
	default:
		return row.Label
	}
}

// focusedDetailPanel is the bottom contextual diagnostic that follows the
// cursor. Disabled rows remain focusable; content is always path-free.
func (m eagerCleanModel) focusedDetailPanel() string {
	row, ok := m.focusedRow()
	if !ok {
		return "Focused: (none)"
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Focused: %s", row.Label))
	lines = append(lines, m.focusedDetailBody(row))
	if row.SafetyNote != "" {
		lines = append(lines, "Safety: "+row.SafetyNote)
	}
	return strings.Join(lines, "\n")
}

func (m eagerCleanModel) focusedDetailBody(row eagerCategoryRow) string {
	switch row.State {
	case clean.CategoryPreviewWaiting:
		return "Waiting to scan."
	case clean.CategoryPreviewScanning:
		return "Scanning…"
	case clean.CategoryPreviewComplete:
		return fmt.Sprintf("Complete · %d item(s) · %s", row.CandidateCount, cleanFormatBytes(row.Bytes))
	case clean.CategoryPreviewPartial:
		body := fmt.Sprintf("Partial · %d item(s) · %s", row.CandidateCount, cleanFormatBytes(row.Bytes))
		if row.ExcludedSiblingCount > 0 {
			body += fmt.Sprintf(" · %d excluded", row.ExcludedSiblingCount)
		}
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		}
		return body
	case clean.CategoryPreviewEmpty:
		return "Empty · no candidates found"
	case clean.CategoryPreviewSkipped:
		body := "Skipped"
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		}
		if row.ExcludedSiblingCount > 0 {
			body += fmt.Sprintf(" · %d excluded", row.ExcludedSiblingCount)
		}
		return body
	case clean.CategoryPreviewIncomplete:
		body := "Incomplete"
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		}
		return body
	case clean.CategoryPreviewFailed:
		body := "Failed"
		if row.ReasonCode != "" {
			body += " · " + pathFreeReasonExplanation(row.ReasonCode)
		} else {
			body += " · measurement failed"
		}
		return body
	default:
		return "Unknown state"
	}
}

func pathFreeReasonExplanation(code string) string {
	switch code {
	case clean.PreviewReasonProtected:
		return "protected by Protection rules"
	case clean.PreviewReasonApplicationRunning, clean.PreviewReasonDevToolRunning:
		return "application is running"
	case clean.PreviewReasonRunningStateUnknown:
		return "application state unknown"
	case clean.PreviewReasonInspectionLimit:
		return "inspection limit exceeded"
	case clean.PreviewReasonContextCanceled:
		return "scan canceled"
	case clean.PreviewReasonInspectionFailed:
		return "measurement failed"
	case clean.PreviewReasonEmpty:
		return "no candidates found"
	case "reparse_point":
		return "reparse point excluded"
	default:
		// Stable code only — never forward raw OS text that may embed paths.
		if code == "" {
			return "see category state"
		}
		return code
	}
}

func (m eagerCleanModel) footerHints() string {
	const base = "Hints: up/down browse · space toggle · a select all · x clear · b/Esc back · q quit"
	if !m.allCategoriesTerminal() {
		return base
	}
	// Distinct zero-authorization messages; none call execution or write History.
	switch m.noWorkState() {
	case clean.EagerPreviewNoWorkNeedSelection:
		return "Select at least one category to continue.\n" + base
	case clean.EagerPreviewNoWorkAllEmpty:
		return "Nothing to clean.\n" + base
	case clean.EagerPreviewNoWorkDiagnostics:
		return "No selectable cleanup found. Some categories were skipped or could not be measured.\n" + base
	default:
		if m.confirmationEnabled() {
			return "Hints: enter confirm · up/down browse · space toggle · a select all · x clear · b/Esc back · q quit"
		}
		return base
	}
}
