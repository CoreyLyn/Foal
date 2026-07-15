package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// Category-first eager preview model for the Clean TUI. This surface delivers
// path-free terminal outcomes, exact per-session cleanup selection with live
// measured totals, focused diagnostics, unavailable and no-work classification,
// and confirmation-gate readiness. Confirm/execute and root cutover remain
// later tickets.

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

type eagerCategoryRow struct {
	Identifier     string
	Label          string
	ReportCategory clean.ReportCategory
	Eligibility    clean.CategoryEligibility
	State          clean.CategoryPreviewState
	// Selected is session-only cleanup authorization. Independent of cursor.
	// Defaults start true; opt-ins start false. Non-selectable terminal
	// outcomes clear and disable the row for the rest of the session.
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
}

func newEagerCleanModel(width, height int) eagerCleanModel {
	queue := clean.EagerPreviewQueue()
	rows := make([]eagerCategoryRow, 0, len(queue))
	for _, summary := range queue {
		rows = append(rows, eagerCategoryRow{
			Identifier:     summary.Identifier,
			Label:          summary.Label,
			ReportCategory: summary.ReportCategory,
			Eligibility:    summary.Eligibility,
			State:          clean.CategoryPreviewWaiting,
			// ADR 0013: defaults start selected but remain removable; opt-ins do not.
			Selected: summary.Eligibility == clean.CategoryEligibilityDefault,
		})
	}
	return eagerCleanModel{
		rows:        rows,
		activeIndex: -1,
		width:       width,
		height:      height,
		now:         time.Now,
	}
}

func (m *eagerCleanModel) start() tea.Cmd {
	m.cancelPending()
	m.generation++
	m.finished = false
	m.canceled = false
	m.completed = 0
	m.activeIndex = -1
	m.spinnerFrame = 0
	m.startedAt = m.now()
	m.nav = eagerPreviewNavNone
	m.unavailable = nil
	for i := range m.rows {
		m.rows[i].State = clean.CategoryPreviewWaiting
		m.rows[i].CandidateCount = 0
		m.rows[i].Bytes = 0
		m.rows[i].ExcludedSiblingCount = 0
		m.rows[i].ReasonCode = ""
		m.rows[i].SafetyNote = ""
		// Fresh session defaults: defaults selected, opt-ins cleared.
		m.rows[i].Selected = m.rows[i].Eligibility == clean.CategoryEligibilityDefault
	}

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
	if msg.generation != m.generation || m.canceled || m.finished || m.unavailable != nil {
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
// non-empty exact selection. Does not open confirmation itself (ticket #190).
func (m eagerCleanModel) confirmationEnabled() bool {
	return m.unavailable == nil && !m.canceled && m.allCategoriesTerminal() && m.selectedCount() > 0
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

// handleKey processes category-first keys. Browsing and selection never restart
// the queue, write History, or call execution. Escape/b request menu; q quits;
// Ctrl+C is interrupt. Enter on unavailable returns to the menu. Confirmation
// Enter is owned by the next ticket; this ticket only tracks readiness.
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
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor+1 < len(m.rows) {
			m.cursor++
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

func (m eagerCleanModel) content() string {
	if m.unavailable != nil {
		return m.unavailableContent()
	}

	var builder strings.Builder
	builder.WriteString("Foal Clean\n")
	builder.WriteString(m.headerLine())
	builder.WriteString("\n")

	currentGroup := clean.ReportCategory("")
	for i, row := range m.rows {
		if row.ReportCategory != currentGroup {
			currentGroup = row.ReportCategory
			builder.WriteString(fmt.Sprintf("  %s\n", currentGroup))
		}
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		// Cursor (>) and checkbox ([x]/[ ]) stay independent of scan markers.
		builder.WriteString(fmt.Sprintf("  %s %s %s %s\n", cursor, m.checkbox(row), m.rowMarker(row), m.rowLabel(row)))
	}

	builder.WriteString("\n")
	builder.WriteString(m.selectionSummaryLine())
	builder.WriteString("\n")
	builder.WriteString(m.focusedDetailPanel())
	builder.WriteString("\n")
	builder.WriteString(m.footerHints())
	return builder.String()
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
		return base
	}
}
