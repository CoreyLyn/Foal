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
	"github.com/CoreyLyn/Foal/internal/servicing"
)

// nextEagerGeneration assigns unique Clean session generations so stale stream
// messages from a canceled or superseded session cannot mutate a later model
// that happens to reuse a small per-model counter.
var nextEagerGeneration atomic.Uint64

// cleanExecutionStream carries progress then the final Result for one exact
// Clean execution started from the category-first confirmation path.
type cleanExecutionStream struct {
	progress <-chan clean.ExecutionProgress
	result   <-chan clean.Result
}

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
	PlannedAction clean.PlannedAction
	// SelectionPolicy is catalog-owned. Exact-selection-only rows are excluded
	// from Select All (they may still be toggled individually when selectable).
	SelectionPolicy clean.CategorySelectionPolicy
	State           clean.CategoryPreviewState
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

	// Servicing marks an exact-selection-only Windows servicing category row.
	// Servicing rows are never file-scanned by the eager preview; they use the
	// servicing state machine below instead of the CategoryPreviewState in State,
	// and become selectable only after an explicit analysis reports work.
	Servicing bool
	// ServicingState is the servicing row lifecycle (analysis_required, analyzing,
	// ready, no_work, skipped, failed). Meaningful only when Servicing is true.
	ServicingState clean.ServicingRowState
	// ServicingReclaimablePackages is the parsed reclaimable-package count from a
	// ready or no_work analysis. It is the only servicing quantity disclosed at
	// confirmation; servicing never reports reclaimable bytes.
	ServicingReclaimablePackages int
	// ServicingReasonCode is the stable path-free reason for a skipped or failed
	// analysis. Empty for analysis_required, analyzing, ready, and no_work.
	ServicingReasonCode string
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
var runExactCleanSelection = func(ctx context.Context, selected []string, allowPermanent, allowServicing bool, reporter clean.ProgressReporter) clean.Result {
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
		Validator:              config.Validator,
		ProtectionDiagnostics:  config.Diagnostics,
		ProtectionLoadError:    config.LoadError,
		HistoryRecorder:        recorder,
		CommandParameters:      exactTUICommandParameters(plan.Categories),
		Plan:                   &plan,
		AllowPermanentDeletion: allowPermanent,
		// Per-run Windows servicing authorization equivalent to --allow-servicing,
		// granted by the strengthened confirmation only when the frozen exact
		// selection disclosed a servicing category. Independent of permanent
		// authorization; execution performs a fresh composite analysis.
		AllowServicing:            allowServicing,
		ServicingGateway:          servicing.NewGateway(),
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

func executeExactCleanSelectionCmd(ctx context.Context, selected []string, allowPermanent, allowServicing bool) tea.Cmd {
	selected = append([]string(nil), selected...)
	return func() tea.Msg {
		// Larger buffer: Slice C emits per-category boundaries in addition to
		// phase markers; a tiny buffer can stall the execute goroutine while
		// the TUI drains one event per tick/cmd.
		progress := make(chan clean.ExecutionProgress, 64)
		result := make(chan clean.Result, 1)
		stream := &cleanExecutionStream{progress: progress, result: result}
		go func() {
			defer close(progress)
			result <- runExactCleanSelection(ctx, selected, allowPermanent, allowServicing, func(event clean.ExecutionProgress) {
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

	phase            eagerCleanPhase
	frozenCategories []string
	// frozenAllowPermanent is the per-run permanent authorization disclosed by
	// the strengthened confirmation for the frozen exact selection.
	frozenAllowPermanent bool
	// frozenAllowServicing is the per-run Windows servicing authorization
	// (equivalent to --allow-servicing) granted by confirmation when the frozen
	// exact selection includes a servicing category.
	frozenAllowServicing bool
	executionStarted     bool
	// executionStartedAt is wall-clock start of the confirmed execute path.
	// Elapsed execution chrome uses this, never preview startedAt.
	executionStartedAt    time.Time
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
		row := eagerCategoryRow{
			Identifier:      summary.Identifier,
			Label:           summary.Label,
			ReportCategory:  summary.ReportCategory,
			Eligibility:     summary.Eligibility,
			PlannedAction:   summary.PlannedAction,
			SelectionPolicy: summary.SelectionPolicy,
			State:           clean.CategoryPreviewWaiting,
			// ADR 0018 / deletion policy: derive initial selection from shared
			// eligibility + planned action (no hard-coded permanent list).
			Selected: clean.InitiallySelectedCategory(summary),
		}
		// Windows servicing categories (ADR 0029) start unselected as
		// analysis_required and are never file-scanned. Entering Clean must not
		// analyze the component store or trigger UAC; a distinct analysis action
		// is required before the row can become selectable.
		if clean.IsServicingSummary(summary) {
			row.Servicing = true
			row.ServicingState = clean.ServicingRowAnalysisRequired
			row.Selected = false
			// Servicing rows carry no file-scan CategoryPreviewState; the servicing
			// state machine governs them. Leave State unset so scan-based selection,
			// confirmation-gating, and no-work classification never misread them.
			row.State = ""
		}
		rows = append(rows, row)
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
	m.frozenAllowServicing = false
	m.executionStarted = false
	m.executionStartedAt = time.Time{}
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
	// Servicing rows are never file-scanned; ignore any stray observation so the
	// servicing state machine remains the sole authority for these rows.
	if row.Servicing {
		return
	}
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

// runServicingAnalysisFn is the shared servicing-analysis seam for the Clean
// TUI's explicit `a` action. Production delegates to shared Clean, which drives
// the platform servicing gateway (isolated elevated helper + fixed DISM
// capability); tests replace it with deterministic outcomes so contract tests
// never launch UAC or DISM. The TUI owns no DISM invocation, elevation, helper
// protocol, cancellation enforcement, or output parsing.
var runServicingAnalysisFn = func(ctx context.Context, identifier string) clean.ServicingOperation {
	return clean.AnalyzeServicingCategory(ctx, clean.Options{
		ServicingGateway: servicing.NewGateway(),
	}, identifier)
}

type eagerServicingAnalyzedMsg struct {
	generation          uint64
	identifier          string
	state               clean.ServicingRowState
	reclaimablePackages int
	reason              string
}

// requestServicingAnalysisCmd runs one read-only component-store analysis for a
// servicing row through the shared seam. It is dispatched only by an explicit
// user analysis action; entering Clean never schedules it.
func requestServicingAnalysisCmd(generation uint64, identifier string) tea.Cmd {
	return func() tea.Msg {
		op := runServicingAnalysisFn(context.Background(), identifier)
		return eagerServicingAnalyzedMsg{
			generation:          generation,
			identifier:          identifier,
			state:               clean.ServicingRowStateFromAnalysis(op),
			reclaimablePackages: op.ReclaimablePackages,
			reason:              op.Reason,
		}
	}
}

// requestFocusedServicingAnalysis starts an explicit analysis when the focused
// row is a servicing row that is analyzable. It returns nil when the focus is
// not a servicing row (so the caller falls back to Select All) or an analysis is
// already in flight. The row moves to analyzing and drops any prior selection so
// a stale ready result can never remain authorized across a re-analysis.
func (m *eagerCleanModel) requestFocusedServicingAnalysis() tea.Cmd {
	if m.unavailable != nil || len(m.rows) == 0 {
		return nil
	}
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	row := &m.rows[m.cursor]
	if !row.Servicing || !clean.ServicingRowAnalyzable(row.ServicingState) {
		return nil
	}
	row.ServicingState = clean.ServicingRowAnalyzing
	row.Selected = false
	row.ServicingReclaimablePackages = 0
	row.ServicingReasonCode = ""
	return requestServicingAnalysisCmd(m.generation, row.Identifier)
}

// applyServicingAnalyzed records a completed servicing analysis on its row. A
// ready result makes the row selectable but never auto-selects it; every other
// outcome clears any selection. Stale results from a superseded generation are
// ignored.
func (m *eagerCleanModel) applyServicingAnalyzed(msg eagerServicingAnalyzedMsg) {
	if msg.generation != m.generation || m.canceled || m.unavailable != nil {
		return
	}
	index := m.rowIndex(msg.identifier)
	if index < 0 {
		return
	}
	row := &m.rows[index]
	if !row.Servicing {
		return
	}
	row.ServicingState = msg.state
	row.ServicingReclaimablePackages = msg.reclaimablePackages
	row.ServicingReasonCode = msg.reason
	if !clean.ServicingRowSelectable(row.ServicingState) {
		row.Selected = false
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
	if m.unavailable != nil {
		return false
	}
	return eagerAllCategoriesTerminal(m.rows)
}

// confirmationEnabled requires every scannable category terminal and a
// non-empty exact selection before the first Enter may open confirmation.
func (m eagerCleanModel) confirmationEnabled() bool {
	return eagerConfirmationEnabled(m.unavailable != nil, m.canceled, m.phase, m.rows, m.executionStarted)
}

// noWorkState classifies finished zero-selection presentation.
func (m eagerCleanModel) noWorkState() clean.EagerPreviewNoWorkState {
	return eagerNoWorkState(m.rows, m.selectedCount(), m.unavailable != nil, m.canceled)
}

// selectedCount returns how many categories are currently authorized.
func (m eagerCleanModel) selectedCount() int {
	return eagerSelectedCount(m.rows)
}

// selectedCategoryIDs returns canonical identifiers in stable display/scan
// order. Contains only selected default or opt-in identifiers — no aliases,
// group tokens, permission notices, review evidence, or paths.
func (m eagerCleanModel) selectedCategoryIDs() []string {
	return eagerSelectedCategoryIDs(m.rows)
}

// selectionTotals returns selected category count, safely measured bytes for
// complete/partial selected rows, and selected waiting/scanning pending count.
// Unfinished, empty, skipped, incomplete, and failed work contributes no bytes.
func (m eagerCleanModel) selectionTotals() (categories int, measuredBytes int64, pending int) {
	return eagerSelectionTotals(m.rows)
}

// rowSelectable reports whether Space may toggle the row. Waiting, scanning,
// complete, and partial are selectable; disabled terminal outcomes are not.
func (m eagerCleanModel) rowSelectable(row eagerCategoryRow) bool {
	return eagerRowSelectable(row)
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
// disabled terminal rows stay excluded. Exact-selection-only categories are
// excluded from Select All and may only be toggled individually.
func (m *eagerCleanModel) selectAllSelectable() {
	for i := range m.rows {
		if m.rows[i].SelectionPolicy == clean.CategorySelectionPolicyExactOnly {
			continue
		}
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
		// Context-sensitive: on a servicing row the explicit `a` action requests
		// component-store analysis (ADR 0029); elsewhere it selects all
		// selectable non-servicing categories. Servicing rows are always excluded
		// from Select All.
		if cmd := m.requestFocusedServicingAnalysis(); cmd != nil {
			return eagerPreviewNavNone, cmd
		}
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
	allowServicing := m.selectionIncludesServicing()
	plan, err := clean.CompileExactCategoryPlan(ids)
	if err != nil {
		// Selection is catalog-derived; reject before any cleanup work.
		m.executionStarted = true
		m.frozenCategories = append([]string(nil), ids...)
		m.frozenAllowPermanent = allowPermanent
		m.frozenAllowServicing = allowServicing
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
	m.frozenAllowServicing = allowServicing
	m.executionStarted = true
	m.executionStartedAt = m.now()
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
	// Preview/confirmation already stopped the tick chain (finished=true or
	// non-executing phase). Restart ticks with the execute handoff so the
	// header spinner stays alive through long Fresh scanning / deletion.
	return eagerPreviewNavNone, tea.Batch(
		executeExactCleanSelectionCmd(ctx, m.frozenCategories, m.frozenAllowPermanent, m.frozenAllowServicing),
		tickEagerPreviewCmd(m.generation, m.spinnerFrame),
	)
}

// selectionIncludesServicing reports whether the current exact selection
// includes any Windows servicing (invoke_windows_servicing) category. Used for
// confirmation disclosure and the equivalent per-run servicing authorization.
func (m eagerCleanModel) selectionIncludesServicing() bool {
	return eagerSelectionIncludesServicing(m.rows)
}

// selectionIncludesPermanent reports whether the current exact selection
// discloses any delete_permanently planned action. Used for confirmation
// grouping and equivalent per-run permanent authorization.
func (m eagerCleanModel) selectionIncludesPermanent() bool {
	return eagerSelectionIncludesPermanent(m.rows)
}

// confirmationActionGroups splits the exact selection into Permanent deletion,
// Recycle Bin, and Windows servicing work. Empty groups are omitted. Action is
// catalog-owned.
func (m eagerCleanModel) confirmationActionGroups() (permanent, recycle, servicing []eagerCategoryRow) {
	return eagerConfirmationActionGroups(m.rows)
}

// initialExecutionOutcomes builds path-free in-progress rows for the frozen
// selection. Authorization cannot change after this snapshot. Rows start
// waiting until shared progress names an ActiveCategory.
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
			State:      clean.CategoryExecutionWaiting,
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
	// Observation-only. Slice D: CompletedCategory projects a provisional
	// terminal once shared Clean finished that category's work for this run.
	// ActiveCategory becomes rechecking/ready/cleaning for open rows. Final
	// Result always overwrites provisional terminals completely. Progress
	// never authorizes work.
	if clean.HasCategoryCompletion(msg.progress) {
		completedID := msg.progress.CompletedCategory
		for i := range m.executionOutcomes {
			if m.executionOutcomes[i].Identifier != completedID {
				continue
			}
			// Never regress a provisional terminal back to waiting/in-progress.
			// A second completion for the same id is ignored (shared Clean
			// dedupes); final Result still replaces the whole projection.
			if clean.IsTerminalExecutionState(m.executionOutcomes[i].State) {
				break
			}
			m.executionOutcomes[i].State = msg.progress.CompletedState
			m.executionOutcomes[i].AffectedBytes = msg.progress.CompletedAffectedBytes
			m.executionOutcomes[i].RecycleBinMovedBytes = msg.progress.CompletedRecycleBinMovedBytes
			m.executionOutcomes[i].PermanentlyDeletedBytes = msg.progress.CompletedPermanentlyDeletedBytes
			m.executionOutcomes[i].DeletedCount = msg.progress.CompletedDeletedCount
			m.executionOutcomes[i].SkippedCount = msg.progress.CompletedSkippedCount
			break
		}
	}
	active := msg.progress.ActiveCategory
	phase := msg.progress.Phase
	for i := range m.executionOutcomes {
		if clean.IsTerminalExecutionState(m.executionOutcomes[i].State) {
			continue
		}
		if active == "" {
			// Phase openers without a category (initial Scanning / Complete)
			// must not promote every waiting row. Aggregate Recycle Bin safety
			// is intentionally category-unscoped and maps onto open rows.
			if phase == clean.ExecutionPhaseScanning || phase == "" || phase == clean.ExecutionPhaseComplete {
				continue
			}
			m.executionOutcomes[i].State = clean.InProgressExecutionState(phase)
			continue
		}
		projected := clean.ProjectInProgressCategoryState(
			phase, active, m.executionOutcomes[i].Identifier,
		)
		if projected == clean.CategoryExecutionWaiting {
			// Keep prior in-progress state after this row has been active so
			// it does not flash back to waiting when ActiveCategory moves on.
			if m.executionOutcomes[i].State == clean.CategoryExecutionWaiting ||
				m.executionOutcomes[i].State == "" {
				m.executionOutcomes[i].State = clean.CategoryExecutionWaiting
			}
			continue
		}
		m.executionOutcomes[i].State = projected
	}
	m.syncExecutionViewport()
	return waitEagerExactExecutionCmd(msg.stream)
}

func (m *eagerCleanModel) applyExactExecuted(msg eagerExactExecutedMsg) {
	if m.phase != eagerPhaseExecuting {
		return
	}
	m.executionResult = msg.result
	// Authoritative final Result completely replaces any mid-flight provisional
	// category terminals. Item-level Result/History remain the source of truth.
	m.executionOutcomes = clean.ProjectCategoryExecutionOutcomes(m.frozenCategories, msg.result)
	m.phase = eagerPhaseResult
	m.viewportOffset = 0
	m.execFollowActive = false
	m.execTrackedActive = -1
	if m.cancelExecution != nil {
		m.cancelExecution = nil
	}
}

// content returns the plain-text frame (test oracle). Magnitude styling uses
// contentStyleLines so production View can classify tiers from trusted int64
// bytes rather than reverse-parsing cleanFormatBytes tokens.
func (m eagerCleanModel) content() string {
	return joinStyleLineText(m.contentStyleLines())
}

// contentStyleLines builds the same frame as content() with optional trusted
// magnitude byte metadata on measured/affected lines. Lines are reflowed to the
// terminal width first, so capacity is counted in the rows the terminal will
// actually draw rather than in composed lines.
func (m eagerCleanModel) contentStyleLines() []tuiStyleLine {
	// Fixed floors first, then the dynamic check against reflowed chrome.
	// Header and footer are reflowed once and reused for both decisions.
	if m.width > 0 && m.width < eagerMinTerminalWidth {
		return styleLinesFromPlain(m.tooSmallContent())
	}
	if m.height > 0 && m.height < eagerMinTerminalHeight {
		return styleLinesFromPlain(m.tooSmallContent())
	}
	header := m.reflowedHeaderStyleLines()
	footer := m.reflowedFooterStyleLines()
	if m.height > 0 && m.height < len(header)+len(footer)+1 {
		return styleLinesFromPlain(m.tooSmallContent())
	}
	if m.unavailable != nil {
		return styleLinesFromPlain(m.unavailableContent())
	}

	body := m.reflowedBodyStyleLines()
	// height <= 0 means unconstrained (deterministic model tests without a
	// WindowSize). Otherwise body fills remaining rows under fixed chrome.
	capacity := len(body)
	if m.height > 0 {
		capacity = m.height - len(header) - len(footer)
		if capacity < 1 {
			return styleLinesFromPlain(m.tooSmallContent())
		}
	}
	offset := m.clampedViewportOffset(len(body), capacity)
	end := offset + capacity
	if end > len(body) {
		end = len(body)
	}

	parts := make([]tuiStyleLine, 0, len(header)+capacity+len(footer)+1)
	parts = append(parts, header...)
	if len(body) > 0 {
		parts = append(parts, body[offset:end]...)
	}
	parts = append(parts, footer...)
	// Trailing empty line preserves the historical final newline from content().
	parts = append(parts, styledLine("", lineKindBlank))
	return parts
}

// terminalTooSmall reports whether the terminal cannot host the minimum safe
// fixed chrome plus one body line without truncating actionable layout. Chrome
// is measured after reflow, so wrapped disclosure copy counts as the multiple
// rows it really occupies.
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
	headerN := len(m.reflowedHeaderStyleLines())
	footerN := len(m.reflowedFooterStyleLines())
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
	return styleLinesText(m.fixedHeaderStyleLines())
}

func (m eagerCleanModel) fixedHeaderStyleLines() []tuiStyleLine {
	if m.unavailable != nil {
		return []tuiStyleLine{styledLine("Clean unavailable", lineKindPageTitle)}
	}
	switch m.phase {
	case eagerPhaseConfirmation:
		return []tuiStyleLine{
			styledLine("Foal Clean", lineKindPageTitle),
			styledLine("Confirm cleanup", lineKindPageTitle),
			styledLine("", lineKindBlank),
		}
	case eagerPhaseExecuting:
		return []tuiStyleLine{
			styledLine("Foal Clean", lineKindPageTitle),
			styledLine(m.executionHeaderLine(), lineKindProgressHeader),
			styledLine("", lineKindBlank),
		}
	case eagerPhaseResult:
		return []tuiStyleLine{
			styledLine("Foal Clean", lineKindPageTitle),
			styledLine("Cleanup result", lineKindPageTitle),
			styledLine("", lineKindBlank),
		}
	default:
		return []tuiStyleLine{
			styledLine("Foal Clean", lineKindPageTitle),
			styledLine(m.headerLine(), lineKindProgressHeader),
			styledLine("", lineKindBlank),
		}
	}
}

// fixedFooterLines hold totals, diagnostics, cancellation status, and key hints.
func (m eagerCleanModel) fixedFooterLines() []string {
	return styleLinesText(m.fixedFooterStyleLines())
}

func (m eagerCleanModel) fixedFooterStyleLines() []tuiStyleLine {
	if m.unavailable != nil {
		code := "unavailable"
		message := "Clean cannot start."
		if m.unavailable.Code != "" {
			code = m.unavailable.Code
		}
		if m.unavailable.Message != "" {
			message = m.unavailable.Message
		}
		return []tuiStyleLine{
			styledLine(fmt.Sprintf("Code: %s", code), lineKindProse),
			styledLine(message, lineKindProse),
			styledLine("", lineKindBlank),
			styledLine("Hints: Enter/Esc/b menu · q quit", lineKindHint),
		}
	}
	switch m.phase {
	case eagerPhaseConfirmation:
		return m.confirmationFooterStyleLines()
	case eagerPhaseExecuting:
		lines := []tuiStyleLine{styledLine("", lineKindBlank)}
		if m.cancellationRequested {
			lines = append(lines, styledLine(cancellationRequestedMessage, lineKindProse))
		}
		// Mid-flight totals are provisional: the byte token stays unemphasised
		// so magnitude keeps meaning "settled measurement".
		lines = append(lines, styledLine(m.executionFooterLine(), lineKindProgressTotal))
		if m.cancellationRequested {
			lines = append(lines, styledLine("Waiting for final Result. Escape, b, and q are inactive.", lineKindProse))
		} else {
			lines = append(lines, styledLine("Hints: Ctrl+C cancel · Escape/b/q inactive during execution", lineKindHint))
		}
		return lines
	case eagerPhaseResult:
		return m.resultFooterStyleLines()
	default:
		n, measured, pending := m.selectionTotals()
		rule := styledLine(eagerFooterRuleLine(m.width), lineKindRule)
		// Preview info block: selection summary, permanent notice, focused
		// detail — framed by '=' rules. Hints stay outside below the box.
		lines := []tuiStyleLine{
			styledLine("", lineKindBlank),
			rule,
			// Selected totals: tier from trusted measured bytes.
			styledMagnitudeLine(eagerSelectionSummaryLine(n, measured, pending), measured, lineKindMeasuredTotal),
		}
		// Permanent-selection notice is risk chrome only; disappears when the
		// exact selection has no permanent category. Does not authorize cleanup.
		if notice := eagerPermanentSelectionNotice(m.selectionIncludesPermanent()); notice != "" {
			lines = append(lines, styledLine(notice, lineKindPermanentNotice))
		}
		// Focused diagnostics are prose throughout: their byte tokens are
		// contextual detail, not a measured total, and take no magnitude hue.
		for _, line := range strings.Split(m.focusedDetailPanel(), "\n") {
			lines = append(lines, styledLine(line, lineKindProse))
		}
		lines = append(lines, rule)
		lines = append(lines, eagerFooterHintLines(
			m.allCategoriesTerminal(), m.noWorkState(), m.confirmationEnabled(), m.focusedServicingAnalyzable(),
		)...)
		return lines
	}
}

// scrollableBodyLines are the central content. Group headings travel with their
// category rows; there is no independent heading freeze or scroll mode.
func (m eagerCleanModel) scrollableBodyLines() []string {
	return styleLinesText(m.scrollableBodyStyleLines())
}

func (m eagerCleanModel) scrollableBodyStyleLines() []tuiStyleLine {
	return bodyEntriesToStyleLines(m.scrollableBodyEntries())
}

// bodyEntriesToStyleLines projects body entries onto style lines, carrying the
// declared role and any trusted magnitude bytes across.
func bodyEntriesToStyleLines(entries []eagerBodyLine) []tuiStyleLine {
	out := make([]tuiStyleLine, len(entries))
	for i, line := range entries {
		if line.hasMagnitudeBytes {
			out[i] = styledMagnitudeLine(line.text, line.magnitudeBytes, line.kind)
		} else {
			out[i] = styledLine(line.text, line.kind)
		}
	}
	return out
}

// Reflowed accessors return the lines the terminal actually displays at the
// current width. Every viewport calculation must count these, not the composed
// lines: a composed line wider than the terminal occupies more than one display
// row, and counting it as one is what pushes content off screen.

func (m eagerCleanModel) reflowedHeaderStyleLines() []tuiStyleLine {
	return reflowStyleLines(m.fixedHeaderStyleLines(), m.width)
}

func (m eagerCleanModel) reflowedFooterStyleLines() []tuiStyleLine {
	return reflowStyleLines(m.fixedFooterStyleLines(), m.width)
}

func (m eagerCleanModel) reflowedBodyEntries() []eagerBodyLine {
	return reflowBodyEntries(m.scrollableBodyEntries(), m.width)
}

func (m eagerCleanModel) reflowedBodyStyleLines() []tuiStyleLine {
	return bodyEntriesToStyleLines(m.reflowedBodyEntries())
}

// styleLinesText projects annotated style lines back to plain strings.
func styleLinesText(lines []tuiStyleLine) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line.Text
	}
	return out
}

type eagerBodyLine struct {
	text              string
	kind              tuiLineKind // declared styling role; see tuiLineKind
	rowIndex          int         // preview row index; -1 when not a category row
	outcomeIndex      int         // execution/result outcome index; -1 when not an outcome row
	magnitudeBytes    int64
	hasMagnitudeBytes bool // trusted int64 for the line's cleanFormatBytes token
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
			servicingRow := clean.IsServicingCategory(outcome.Identifier)
			line := eagerBodyLine{
				text:         fmt.Sprintf("  %s %s", m.executionRowMarker(outcome.State), m.executionRowLabelFor(outcome, servicingRow)),
				kind:         lineKindOutcomeRow,
				rowIndex:     -1,
				outcomeIndex: i,
			}
			// Servicing outcomes carry no bytes; only file-deletion outcomes get a
			// trusted affected-byte magnitude token.
			if !servicingRow {
				switch outcome.State {
				case clean.CategoryExecutionCleaned, clean.CategoryExecutionPartial:
					line.hasMagnitudeBytes = true
					line.magnitudeBytes = outcome.AffectedBytes
				}
			}
			lines = append(lines, line)
		}
		return lines
	default:
		lines := make([]eagerBodyLine, 0, len(m.rows)+4)
		// Plain-frame byte column alignment for complete/partial rows only.
		// Catalog/display order is preserved; size never reorders categories.
		leftWidth, byteWidth := eagerPreviewByteColumnWidths(m.rows)
		currentGroup := clean.ReportCategory("")
		for i, row := range m.rows {
			if row.ReportCategory != currentGroup {
				currentGroup = row.ReportCategory
				lines = append(lines, eagerBodyLine{
					text:         fmt.Sprintf("  %s", currentGroup),
					kind:         lineKindGroupHeading,
					rowIndex:     -1,
					outcomeIndex: -1,
				})
			}
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			// Cursor (>) and checkbox ([x]/[ ]) stay independent of scan markers.
			label := eagerPreviewRowLabelAligned(row, leftWidth, byteWidth)
			if row.Servicing {
				label = eagerServicingRowLabel(row)
			}
			line := eagerBodyLine{
				text:         fmt.Sprintf("  %s %s %s %s", cursor, m.checkbox(row), m.rowMarker(row), label),
				kind:         lineKindCategoryRow,
				rowIndex:     i,
				outcomeIndex: -1,
			}
			// Complete/partial measured rows: tier from trusted row.Bytes.
			switch row.State {
			case clean.CategoryPreviewComplete, clean.CategoryPreviewPartial:
				line.hasMagnitudeBytes = true
				line.magnitudeBytes = row.Bytes
			}
			lines = append(lines, line)
		}
		return lines
	}
}

func (m eagerCleanModel) bodyCapacity() int {
	if m.terminalTooSmall() || m.height <= 0 {
		if m.height <= 0 {
			// Unconstrained: treat capacity as large enough for full body.
			return len(m.reflowedBodyEntries()) + 1
		}
		return 0
	}
	cap := m.height - len(m.reflowedHeaderStyleLines()) - len(m.reflowedFooterStyleLines())
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
	bodyLen := len(m.reflowedBodyEntries())
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

// previewBodyLineForRow returns the first display line the preview row occupies
// after reflow, which is the line cursor-follow must keep visible.
func (m eagerCleanModel) previewBodyLineForRow(rowIndex int) int {
	for i, line := range m.reflowedBodyEntries() {
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
	// Prefer the category named by shared progress so viewport follow tracks
	// the real active boundary rather than the first non-terminal waiting row.
	activeID := m.executionProgress.ActiveCategory
	if activeID != "" {
		for i, outcome := range m.executionOutcomes {
			if outcome.Identifier == activeID && !clean.IsTerminalExecutionState(outcome.State) {
				return i
			}
		}
	}
	for i, outcome := range m.executionOutcomes {
		if !clean.IsTerminalExecutionState(outcome.State) &&
			outcome.State != clean.CategoryExecutionWaiting {
			return i
		}
	}
	for i, outcome := range m.executionOutcomes {
		if !clean.IsTerminalExecutionState(outcome.State) {
			return i
		}
	}
	return -1
}

// executionOutcomeBodyLine returns the first display line the outcome occupies
// after reflow.
func (m eagerCleanModel) executionOutcomeBodyLine(outcomeIndex int) int {
	for i, line := range m.reflowedBodyEntries() {
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
	offset := m.clampedViewportOffset(len(m.reflowedBodyEntries()), cap)
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
	phase := m.executionProgress.Phase
	reassurance := ""
	if (phase == "" || phase == clean.ExecutionPhaseScanning) &&
		m.executionElapsed() >= eagerExecutionStillWorkingThreshold {
		reassurance = eagerExecutionStillScanningReassurance
	}
	phaseLabel := sharedExecutionPhaseLabel(phase)
	if active := strings.TrimSpace(m.executionProgress.ActiveCategory); active != "" {
		// Path-free catalog label only; never candidate paths.
		label := active
		if summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(active); ok && summary.Label != "" {
			label = summary.Label
		}
		phaseLabel = phaseLabel + " · " + label
	}
	return eagerExecutionHeaderLine(
		m.spinnerFrame,
		phaseLabel,
		m.executionElapsedLabel(),
		reassurance,
	)
}

// executionElapsed is duration since confirmed execute began (not preview).
func (m eagerCleanModel) executionElapsed() time.Duration {
	if m.executionStartedAt.IsZero() {
		return 0
	}
	now := m.now
	if now == nil {
		now = time.Now
	}
	d := now().Sub(m.executionStartedAt)
	if d < 0 {
		return 0
	}
	return d
}

func (m eagerCleanModel) executionElapsedLabel() string {
	return eagerExecutionElapsedLabel(m.executionElapsed())
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
	case clean.ExecutionPhaseServicingOperations:
		return "Windows servicing"
	case clean.ExecutionPhaseComplete:
		return "Completion"
	default:
		return "Fresh scanning"
	}
}

func (m eagerCleanModel) confirmationBodyEntries() []eagerBodyLine {
	permanent, recycle, servicing := m.confirmationActionGroups()
	return confirmationBodyEntriesFromGroups(permanent, recycle, servicing)
}

func (m eagerCleanModel) confirmationFooterLines() []string {
	return styleLinesText(m.confirmationFooterStyleLines())
}

func (m eagerCleanModel) confirmationFooterStyleLines() []tuiStyleLine {
	n, measured, _ := m.selectionTotals()
	includesPermanent := m.selectionIncludesPermanent()
	_, recycle, servicing := m.confirmationActionGroups()
	lines := []tuiStyleLine{
		styledLine("", lineKindBlank),
		styledMagnitudeLine(
			fmt.Sprintf("Selected: %d categories · %s", n, cleanFormatBytes(measured)), measured, lineKindMeasuredTotal),
	}
	// Risk / recoverability first so irreversible disclosure stays visible.
	if includesPermanent {
		lines = append(lines, styledLine(confirmationPermanentIrreversibleWarning, lineKindRisk))
	}
	if len(recycle) > 0 {
		lines = append(lines, styledLine(confirmationRecycleRecoverabilityNote, lineKindProse))
	}
	// Windows servicing disclosure (ADR 0029): administrator consent after
	// ordinary cleanup, /NoRestart, non-interruptible after mutation begins, and
	// the equivalent per-run --allow-servicing authorization this confirmation
	// grants. Only shown when the selection discloses a servicing category.
	if len(servicing) > 0 {
		for _, line := range confirmationServicingDisclosureLines() {
			lines = append(lines, styledLine(line, lineKindProse))
		}
	}
	// Fresh-rescan expectation + action-type safety, then execute hint.
	lines = append(lines,
		styledLine(confirmationActionTypeCaveat, lineKindProse),
		styledLine(confirmationNextStepLine, lineKindProse),
		styledLine("", lineKindBlank),
		styledLine(confirmationExecuteHintLine(includesPermanent), lineKindProse),
	)
	return lines
}

func (m eagerCleanModel) resultTotals() (recycle, permanent, affected int64) {
	return eagerResultTotals(m.executionResult, m.executionOutcomes)
}

func (m eagerCleanModel) resultFooterLines() []string {
	return styleLinesText(m.resultFooterStyleLines())
}

func (m eagerCleanModel) resultFooterStyleLines() []tuiStyleLine {
	recycle, permanent, affected := m.resultTotals()
	terminal := clean.CountTerminalExecutionOutcomes(m.executionOutcomes)
	total := len(m.executionOutcomes)
	lines := []tuiStyleLine{
		styledLine("", lineKindBlank),
		styledLine(fmt.Sprintf("Processed: %d/%d", terminal, total), lineKindProgressTotal),
		styledMagnitudeLine(fmt.Sprintf("Recycle Bin moved: %s", cleanFormatBytes(recycle)), recycle, lineKindMeasuredTotal),
		styledMagnitudeLine(fmt.Sprintf("Permanently deleted: %s", cleanFormatBytes(permanent)), permanent, lineKindMeasuredTotal),
		// Aggregate is processed content, never "freed" or "reclaimed" disk space.
		styledMagnitudeLine(fmt.Sprintf("Affected (processed): %s", cleanFormatBytes(affected)), affected, lineKindMeasuredTotal),
	}
	if clean.ResultHasPermanentPartialRisk(m.executionResult) {
		lines = append(lines, styledLine(clean.PermanentPartialRiskWarning, lineKindProse))
	}
	// Run-level failures carry no category Rule, so they appear on no outcome
	// row. Surface them here (code-mapped, path-free) or a rejected plan would
	// render as an unexplained all-zero result.
	for _, note := range eagerResultErrorNotes(m.executionResult) {
		lines = append(lines, styledLine(note, lineKindProse))
	}
	// Windows servicing disclosures (restart required, post-start cancellation)
	// are preserved from the authoritative Result without inventing bytes.
	for _, note := range eagerResultServicingNotes(m.executionResult) {
		lines = append(lines, styledLine(note, lineKindProse))
	}
	// Post-mutation free-space observation: an approximate external reading shown
	// below Affected, de-emphasized (plain, ≈ prefix) so it never reads as a
	// precise deletion total. A measured-zero or absent observation shows nothing.
	if observed, present := clean.ServicingObservedFreeBytes(m.executionResult.ServicingOperations); present {
		lines = append(lines,
			styledLine(fmt.Sprintf("Windows component store: observed free-space increase ≈ %s (approximate; not in Affected)", cleanFormatBytes(observed)), lineKindProse),
			styledLine(fmt.Sprintf("Mixed cleanup impact ≈ %s (approximate: Affected plus observed servicing free-space)", cleanFormatBytes(affected+observed)), lineKindProse),
		)
	}
	lines = append(lines, styledLine("", lineKindBlank), styledLine("Enter/Esc/b: menu · q: quit", lineKindProse))
	return lines
}

func (m eagerCleanModel) executionFooterLine() string {
	terminal := clean.CountTerminalExecutionOutcomes(m.executionOutcomes)
	total := len(m.executionOutcomes)
	// Mid-flight: Processed counts provisional terminals; Affected sums only
	// successful bytes already recorded on those provisional outcomes (honest
	// partial, never invented). Final Result view uses the full split footer.
	if m.phase != eagerPhaseResult {
		affected := clean.SumExecutionAffectedBytes(m.executionOutcomes)
		return fmt.Sprintf("Processed: %d/%d · Affected (processed): %s", terminal, total, cleanFormatBytes(affected))
	}
	recycle, permanent, affected := m.resultTotals()
	return fmt.Sprintf(
		"Processed: %d/%d · Recycle Bin moved: %s · Permanently deleted: %s · Affected (processed): %s",
		terminal, total, cleanFormatBytes(recycle), cleanFormatBytes(permanent), cleanFormatBytes(affected),
	)
}

func (m eagerCleanModel) executionRowMarker(state clean.CategoryExecutionState) string {
	return eagerExecutionRowMarker(state, m.spinnerFrame)
}

func (m eagerCleanModel) executionRowLabel(outcome clean.CategoryExecutionOutcome) string {
	return eagerExecutionRowLabel(outcome)
}

// executionRowLabelFor renders a servicing outcome without a byte token (a
// servicing action processes no file bytes) and every other outcome normally.
func (m eagerCleanModel) executionRowLabelFor(outcome clean.CategoryExecutionOutcome, servicingRow bool) string {
	if servicingRow {
		return eagerServicingExecutionRowLabel(outcome)
	}
	return eagerExecutionRowLabel(outcome)
}

func (m eagerCleanModel) checkbox(row eagerCategoryRow) string {
	return eagerCheckbox(row.Selected)
}

// selectionSummaryLine shows live selected totals. Unfinished selected work is
// pending, never guessed as zero bytes. After every selected category is
// terminal the line collapses to count and measured total.
func (m eagerCleanModel) selectionSummaryLine() string {
	n, measured, pending := m.selectionTotals()
	return eagerSelectionSummaryLine(n, measured, pending)
}

func (m eagerCleanModel) unavailableContent() string {
	code := ""
	message := ""
	if m.unavailable != nil {
		code = m.unavailable.Code
		message = m.unavailable.Message
	}
	return eagerUnavailableContent(code, message)
}

func (m eagerCleanModel) headerLine() string {
	return eagerPreviewHeaderLine(
		m.canceled,
		m.finished,
		m.allCategoriesTerminal(),
		m.activeIndex,
		m.completed,
		m.scannableRowCount(),
		m.elapsedLabel(),
	)
}

// scannableRowCount is the number of file-scanned preview rows. Windows
// servicing rows are never file-scanned (they use an explicit analysis action),
// so they are excluded from the scan progress denominator.
func (m eagerCleanModel) scannableRowCount() int {
	n := 0
	for _, row := range m.rows {
		if !row.Servicing {
			n++
		}
	}
	return n
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
	if row.Servicing {
		return eagerServicingRowMarker(row.ServicingState, m.spinnerFrame)
	}
	return eagerPreviewRowMarker(row.State, m.spinnerFrame)
}

func (m eagerCleanModel) rowLabel(row eagerCategoryRow) string {
	if row.Servicing {
		return eagerServicingRowLabel(row)
	}
	return eagerPreviewRowLabel(row)
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
	if row.Servicing {
		return eagerServicingFocusedDetailBody(row)
	}
	return eagerFocusedDetailBody(row)
}

// focusedServicingAnalyzable reports whether the focused row is a Windows
// servicing row in a state that accepts the explicit analysis action.
func (m eagerCleanModel) focusedServicingAnalyzable() bool {
	row, ok := m.focusedRow()
	return ok && row.Servicing && clean.ServicingRowAnalyzable(row.ServicingState)
}

func (m eagerCleanModel) footerHints() string {
	return eagerFooterHints(
		m.allCategoriesTerminal(), m.noWorkState(), m.confirmationEnabled(), m.focusedServicingAnalyzable(),
	)
}
