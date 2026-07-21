package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/uninstall"
)

// tui_uninstall.go holds the Uninstall TUI model. It is an ADAPTER only: it
// collects multi-select app names, one confirmation, and authorization choices
// (execute, process-stop, permanent), then calls shared uninstall.Execute with
// the SAME ExecuteOptions shape and semantics as the CLI. It MUST NOT own
// uninstaller launch, path safety, Protection loading, elevation, leftover
// deletion, or portable removal logic - all of that lives in the shared Execute
// seam (#291-#294) which the TUI calls. Safety guarantees from #291-#294 stay
// intact; the TUI just calls Execute correctly.
//
// Pattern mirrors the Clean TUI (tui_clean_eager.go): preview selection is
// non-mutating, confirmation freezes the exact selection and authorizations,
// and the confirmed run hands off to the shared execute path exactly once with
// TUI History provenance (Surface=tui, SelectedCategories).

// uninstallPhase is the authorization boundary for the Uninstall TUI.
type uninstallPhase int

const (
	uninstallPhasePreview uninstallPhase = iota
	uninstallPhaseConfirmation
	uninstallPhaseExecuting
	uninstallPhaseResult
)

// uninstallPreviewNav is the navigation intent returned by the Uninstall model.
type uninstallPreviewNav int

const (
	uninstallNavNone uninstallPreviewNav = iota
	uninstallNavMenu
	uninstallNavQuit
	uninstallNavInterrupt
)

// Minimum terminal geometry for the Uninstall TUI. Below this, the model
// renders guidance instead of truncating confirmation disclosures.
const (
	uninstallMinTerminalWidth  = 40
	uninstallMinTerminalHeight = 12
)

type uninstallAppRow struct {
	app      uninstall.Application
	selected bool
}

// uninstallModel is the Uninstall TUI model. It is an adapter over shared
// uninstall.Execute: it never launches uninstallers, validates paths, loads
// Protection, requests elevation, or deletes anything itself.
type uninstallModel struct {
	loading bool
	notice  string
	review  uninstall.Result
	rows    []uninstallAppRow
	cursor  int
	width   int
	height  int
	now     func() time.Time

	// viewportOffset is the first visible line of the scrollable body. Titles
	// and key hints stay fixed outside this window. There is no separate scroll
	// mode: preview Up/Down move focus and the viewport follows; confirmation
	// and result Up/Down only adjust the offset.
	viewportOffset int

	// Authorization opt-ins captured in preview (default off). Frozen at
	// confirmation so the confirmed run cannot silently authorize more than
	// the user disclosed. These mirror CLI --allow-stop-processes and
	// --allow-permanent: --execute is implied by reaching the confirmed run.
	allowStopProcesses bool
	allowPermanent     bool

	phase                uninstallPhase
	frozenSelection      []string
	frozenAllowStop      bool
	frozenAllowPermanent bool

	executionStarted   bool
	executionStartedAt time.Time
	executionResult    uninstall.ExecuteResult
	cancelExecution    context.CancelFunc

	// cancellationRequested is set on the first cooperative Ctrl+C during
	// active execution. Completed work is never rolled back.
	cancellationRequested bool

	nav uninstallPreviewNav
}

// Messages flowing through the root model.

type uninstallPreviewLoadedMsg struct {
	review uninstall.Result
	err    error
}

type uninstallExecutedMsg struct {
	result uninstall.ExecuteResult
}

// loadUninstallPreviewCmd runs the read-only Review() off the UI loop. Preview
// is non-mutating: no history, no execute, no path safety. The review's
// PossibleLeftovers (app-owned, high confidence) become the per-app Confirmed
// leftover path set ceiling disclosed at confirmation.
var loadUninstallPreviewCmd = func() tea.Msg {
	review := uninstall.WithReviewSections(reviewUninstall())
	return uninstallPreviewLoadedMsg{review: review}
}

// runUninstallTUIExecute is the confirmed TUI execute seam. It builds the SAME
// ExecuteOptions shape as the CLI (authorization equivalence) and reuses shared
// uninstall.Execute (fresh discovery, Protection, path safety, deletion,
// history). Tests replace it to assert handoff without mutation. It never
// synthesizes CLI arguments; TUI provenance is recorded via Surface=tui and
// SelectedCategories, mirroring the Clean TUI exact-execution provenance
// (ADR 0016). Process-stop and permanent default off unless the confirmed
// selection disclosed and authorized them.
var runUninstallTUIExecute = func(ctx context.Context, selection []string, allowStop, allowPermanent bool) uninstall.ExecuteResult {
	config := loadProtectionConfiguration()
	recorder, _ := newHistoryRecorder()
	opts := uninstall.ExecuteOptions{
		Selection:          append([]string(nil), selection...),
		AllowStopProcesses: allowStop,
		AllowPermanent:     allowPermanent,
		Validator:          config.Validator,
		HistoryRecorder:    recorder,
		CommandParameters: history.CommandParameters{
			Command:            "uninstall",
			Surface:            "tui",
			SelectionMode:      "exact",
			SelectedCategories: append([]string(nil), selection...),
		},
	}
	if config.LoadError != nil {
		opts.ProtectionLoadError = &uninstall.ProtectionLoadIssue{
			Code:        config.LoadError.Code,
			Message:     config.LoadError.Message,
			Recoverable: config.LoadError.Recoverable,
		}
	}
	return executeUninstall(ctx, opts)
}

func newUninstallModel(width, height int) uninstallModel {
	return uninstallModel{
		loading: true,
		width:   width,
		height:  height,
		now:     time.Now,
	}
}

func (m *uninstallModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.reflowViewportAfterResize()
}

// reflowViewportAfterResize preserves the focused preview row or current
// viewport offset after a terminal resize. It never changes selection or auth.
func (m *uninstallModel) reflowViewportAfterResize() {
	if m.terminalTooSmall() {
		return
	}
	if m.phase == uninstallPhasePreview && !m.loading {
		m.ensurePreviewCursorVisible()
		return
	}
	m.clampViewportOffset()
}

// start kicks off the read-only preview load. No mutation, no history, no
// execute. The preview is the only phase before user confirmation.
func (m *uninstallModel) start() tea.Cmd {
	m.loading = true
	m.phase = uninstallPhasePreview
	m.viewportOffset = 0
	return loadUninstallPreviewCmd
}

func (m *uninstallModel) applyPreviewLoaded(msg uninstallPreviewLoadedMsg) {
	m.loading = false
	if msg.err != nil {
		m.notice = "Uninstall preview failed: " + msg.err.Error()
		return
	}
	m.review = msg.review
	m.rows = uninstallRowsFromApplications(msg.review.Applications)
	m.cursor = 0
	m.viewportOffset = 0
	// Focus the first selectable row so space works immediately.
	for i, row := range m.rows {
		if uninstallRowSelectable(row) {
			m.cursor = i
			break
		}
	}
	m.ensurePreviewCursorVisible()
}

func uninstallRowsFromApplications(apps []uninstall.Application) []uninstallAppRow {
	rows := make([]uninstallAppRow, 0, len(apps))
	for _, app := range apps {
		rows = append(rows, uninstallAppRow{app: app})
	}
	return rows
}

// uninstallRowSelectable reports whether the app may be selected for uninstall.
// Hard exclusions and not-executable apps are listed (so the user sees Foal
// recognized them) but not selectable. The classification lives in shared
// uninstall.Review/classifyApplicationPlan; the TUI never re-derives it.
func uninstallRowSelectable(row uninstallAppRow) bool {
	switch row.app.PlannedClass {
	case uninstall.PlannedClassOfficialUninstaller,
		uninstall.PlannedClassPortableDirectoryRemoval:
		return true
	}
	return false
}

func (m *uninstallModel) toggleFocusedSelection() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	if !uninstallRowSelectable(m.rows[m.cursor]) {
		return
	}
	m.rows[m.cursor].selected = !m.rows[m.cursor].selected
}

func (m uninstallModel) selectedAppNames() []string {
	names := make([]string, 0, len(m.rows))
	for _, row := range m.rows {
		if row.selected {
			names = append(names, row.app.Name)
		}
	}
	return names
}

func (m uninstallModel) selectedCount() int {
	n := 0
	for _, row := range m.rows {
		if row.selected {
			n++
		}
	}
	return n
}

func (m uninstallModel) confirmationEnabled() bool {
	return m.selectedCount() > 0
}

// selectionIncludesPortable reports whether the selection includes any
// portable_directory_removal app. Portable removal requires per-run
// --allow-permanent authorization separate from --execute; the confirmation
// discloses this so the user knows permanent authorization is required.
func (m uninstallModel) selectionIncludesPortable() bool {
	for _, row := range m.rows {
		if row.selected && row.app.PlannedClass == uninstall.PlannedClassPortableDirectoryRemoval {
			return true
		}
	}
	return false
}

// selectionIncludesAdmin reports whether the selection includes any app that
// likely requires administrator rights (HKLM / machine-wide). Disclosed as a
// grouping before confirmation so UAC is expected, not surprising mid-batch
// (ADR 0028).
func (m uninstallModel) selectionIncludesAdmin() bool {
	for _, row := range m.rows {
		if row.selected && row.app.RequiresAdmin {
			return true
		}
	}
	return false
}

// selectedAdminAppNames returns the names of selected admin-required apps for
// the confirmation's admin-need grouping disclosure.
func (m uninstallModel) selectedAdminAppNames() []string {
	var names []string
	for _, row := range m.rows {
		if row.selected && row.app.RequiresAdmin {
			names = append(names, row.app.Name)
		}
	}
	return names
}

// confirmedLeftoverCount returns the total count of high-confidence app-owned
// Possible leftovers across the selected apps. This is the frozen ceiling
// disclosed at confirmation (the Confirmed leftover path set); the shared
// Execute seam revalidates and may delete only a subset after uninstaller
// success. Paths remain available via the preview detail / JSON surfaces.
func (m uninstallModel) confirmedLeftoverCount() int {
	selected := map[string]bool{}
	for _, row := range m.rows {
		if row.selected {
			selected[strings.ToLower(strings.TrimSpace(row.app.Name))] = true
		}
	}
	n := 0
	for _, leftover := range m.review.PossibleLeftovers {
		if leftover.Ownership != "app_owned" || leftover.Confidence != "high" {
			continue
		}
		if selected[strings.ToLower(strings.TrimSpace(leftover.App))] {
			n++
		}
	}
	return n
}

// handleKey dispatches keyboard input by phase. Preview and confirmation never
// mutate; only beginExecution calls shared uninstall.Execute. Returns nav
// intent and an optional command.
func (m *uninstallModel) handleKey(key string) (uninstallPreviewNav, tea.Cmd) {
	switch m.phase {
	case uninstallPhaseExecuting:
		// Mirrors Clean TUI: only Ctrl+C requests cooperative cancel. Stay
		// attached until the final Result and normal History complete.
		switch key {
		case "ctrl+c":
			m.cancellationRequested = true
			if m.cancelExecution != nil {
				m.cancelExecution()
			}
		}
		return uninstallNavNone, nil
	case uninstallPhaseResult:
		switch key {
		case "ctrl+c":
			m.nav = uninstallNavInterrupt
			return m.nav, nil
		case "q":
			m.nav = uninstallNavQuit
			return m.nav, nil
		case "enter", "esc", "b", "escape":
			m.nav = uninstallNavMenu
			return m.nav, nil
		case "up", "k":
			// Non-selectable content: Up/Down only adjust viewport offset.
			m.scrollViewport(-1)
			return uninstallNavNone, nil
		case "down", "j":
			m.scrollViewport(1)
			return uninstallNavNone, nil
		}
		return uninstallNavNone, nil
	case uninstallPhaseConfirmation:
		switch key {
		case "ctrl+c":
			m.nav = uninstallNavInterrupt
			return m.nav, nil
		case "q":
			m.nav = uninstallNavQuit
			return m.nav, nil
		case "esc", "b", "escape":
			// Preserve in-memory selection and authorizations; return to preview.
			m.phase = uninstallPhasePreview
			m.viewportOffset = 0
			m.ensurePreviewCursorVisible()
			return uninstallNavNone, nil
		case "enter":
			return m.beginExecution()
		case "up", "k":
			// Non-selectable content: Up/Down only adjust viewport offset.
			m.scrollViewport(-1)
			return uninstallNavNone, nil
		case "down", "j":
			m.scrollViewport(1)
			return uninstallNavNone, nil
		}
		return uninstallNavNone, nil
	}

	// Preview phase.
	if m.loading {
		switch key {
		case "ctrl+c":
			m.nav = uninstallNavInterrupt
			return m.nav, nil
		case "q":
			m.nav = uninstallNavQuit
			return m.nav, nil
		}
		return uninstallNavNone, nil
	}

	switch key {
	case "ctrl+c":
		m.nav = uninstallNavInterrupt
		return m.nav, nil
	case "q":
		m.nav = uninstallNavQuit
		return m.nav, nil
	case "esc", "b", "escape":
		m.nav = uninstallNavMenu
		return m.nav, nil
	case "enter":
		// First enter opens confirmation (non-mutating). A second enter in
		// the confirmation phase starts the confirmed run.
		if m.confirmationEnabled() {
			m.phase = uninstallPhaseConfirmation
			m.viewportOffset = 0
		}
		return uninstallNavNone, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		m.ensurePreviewCursorVisible()
	case "down", "j":
		if m.cursor+1 < len(m.rows) {
			m.cursor++
		}
		m.ensurePreviewCursorVisible()
	case " ", "space":
		// Toggle only; never mutates.
		m.toggleFocusedSelection()
	case "s":
		// Process-stop opt-in (mirrors CLI --allow-stop-processes). Default
		// off; --execute alone never stops or kills a process.
		m.allowStopProcesses = !m.allowStopProcesses
	case "p":
		// Permanent opt-in (mirrors CLI --allow-permanent). Required for
		// portable directory removal; default off.
		m.allowPermanent = !m.allowPermanent
	}
	return uninstallNavNone, nil
}

// beginExecution freezes the selection and authorizations and starts shared
// uninstall.Execute exactly once. Repeated input cannot start a second run.
// The frozen selection and authorizations cannot change after this point,
// matching Clean TUI's freeze-at-confirmation behavior.
func (m *uninstallModel) beginExecution() (uninstallPreviewNav, tea.Cmd) {
	if m.executionStarted || m.phase == uninstallPhaseExecuting {
		return uninstallNavNone, nil
	}
	selection := m.selectedAppNames()
	if len(selection) == 0 {
		return uninstallNavNone, nil
	}
	m.frozenSelection = append([]string(nil), selection...)
	m.frozenAllowStop = m.allowStopProcesses
	m.frozenAllowPermanent = m.allowPermanent
	m.executionStarted = true
	m.executionStartedAt = m.now()
	m.phase = uninstallPhaseExecuting
	m.viewportOffset = 0
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelExecution = cancel
	return uninstallNavNone, executeUninstallSelectionCmd(ctx, m.frozenSelection, m.frozenAllowStop, m.frozenAllowPermanent)
}

func executeUninstallSelectionCmd(ctx context.Context, selection []string, allowStop, allowPermanent bool) tea.Cmd {
	selection = append([]string(nil), selection...)
	return func() tea.Msg {
		result := runUninstallTUIExecute(ctx, selection, allowStop, allowPermanent)
		return uninstallExecutedMsg{result: result}
	}
}

func (m *uninstallModel) applyExecuted(msg uninstallExecutedMsg) {
	if m.phase != uninstallPhaseExecuting {
		return
	}
	// Result view reflects the shared ExecuteResult (per-app outcomes, elevation
	// outcome, leftover outcomes). The TUI never re-derives outcomes.
	m.executionResult = msg.result
	m.phase = uninstallPhaseResult
	m.viewportOffset = 0
	if m.cancelExecution != nil {
		m.cancelExecution = nil
	}
}

// --- Rendering (plain-text frame, test oracle) ---

func (m uninstallModel) terminalTooSmall() bool {
	if m.width > 0 && m.width < uninstallMinTerminalWidth {
		return true
	}
	if m.height <= 0 {
		// Zero height is unconstrained for pure model tests that only assert
		// textual contracts; production always receives WindowSize.
		return false
	}
	if m.height < uninstallMinTerminalHeight {
		return true
	}
	// Dynamic chrome may still exceed the floor when footer diagnostics grow.
	return m.height < len(m.fixedHeaderLines())+len(m.fixedFooterLines())+1
}

func (m uninstallModel) tooSmallContent() string {
	return "Terminal too small\nResize the terminal larger to continue using Uninstall.\n"
}

// content returns the plain-text frame with fixed header/footer and a
// height-clamped scrollable body so long app lists remain navigable.
func (m uninstallModel) content() string {
	if m.terminalTooSmall() {
		return m.tooSmallContent()
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

	var b strings.Builder
	for _, line := range header {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if len(body) > 0 {
		for _, line := range body[offset:end] {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	for _, line := range footer {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func (m uninstallModel) fixedHeaderLines() []string {
	lines := []string{
		"+--------------------------------------------------+",
		"| Uninstall TUI                                    |",
	}
	switch m.phase {
	case uninstallPhaseConfirmation:
		lines = append(lines, "| Confirm uninstall: review the plan below.        |")
	case uninstallPhaseExecuting:
		lines = append(lines, "| Uninstalling... shared execute path is running.  |")
	case uninstallPhaseResult:
		lines = append(lines, "| Uninstall result: shared execute outcome.        |")
	default:
		lines = append(lines, "| Preview-only: select apps, then confirm to run.  |")
	}
	lines = append(lines, "+--------------------------------------------------+", "")
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	return lines
}

func (m uninstallModel) fixedFooterLines() []string {
	if m.loading {
		return []string{"", m.footerHintsLine()}
	}
	switch m.phase {
	case uninstallPhaseConfirmation, uninstallPhaseExecuting:
		return []string{"", m.footerHintsLine()}
	case uninstallPhaseResult:
		// Result body already includes the return/quit guidance line.
		return nil
	}
	// Preview: selection and authorization stay fixed under the scrollable list.
	lines := []string{
		"",
		fmt.Sprintf("Selected: %d", m.selectedCount()),
		fmt.Sprintf("Process-stop authorized: %t (toggle: s)", m.allowStopProcesses),
		fmt.Sprintf("Permanent authorized: %t (toggle: p)", m.allowPermanent),
	}
	if m.selectionIncludesPortable() && !m.allowPermanent {
		lines = append(lines, "Note: selection includes portable removal which requires permanent authorization (toggle p).")
	}
	lines = append(lines, "", m.footerHintsLine())
	return lines
}

func (m uninstallModel) scrollableBodyLines() []string {
	if m.loading {
		return []string{"Loading uninstall preview..."}
	}
	switch m.phase {
	case uninstallPhaseConfirmation:
		return m.confirmationBodyLines()
	case uninstallPhaseExecuting:
		return m.executionBodyLines()
	case uninstallPhaseResult:
		return m.resultBodyLines()
	}
	return m.previewBodyLines()
}

func (m uninstallModel) previewBodyLines() []string {
	if len(m.rows) == 0 {
		return []string{"No installed applications discovered."}
	}
	lines := make([]string, 0, len(m.rows)+1)
	lines = append(lines, fmt.Sprintf("Applications (%d):", len(m.rows)))
	for i, row := range m.rows {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		checkbox := "[ ]"
		if row.selected {
			checkbox = "[x]"
		}
		selectable := uninstallRowSelectable(row)
		label := row.app.Name
		if !selectable {
			label += " (" + uninstallPlannedClassShort(row.app.PlannedClass) + ", not selectable)"
		} else {
			label += " (" + uninstallPlannedClassShort(row.app.PlannedClass) + ")"
		}
		if row.app.RequiresAdmin {
			label += " [admin]"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", marker, checkbox, label))
	}
	return lines
}

// confirmationBodyLines renders the confirmation disclosures required by the
// spec: per selected app planned class (official_uninstaller vs
// portable_directory_removal), confirmed leftover scope (count summary),
// process-stop opt-in state, permanent authorization state (and that portable
// removal requires it), and admin-need grouping. Paths stay out of the primary
// list (path-free UX matching Clean TUI); the leftover count is the summary.
func (m uninstallModel) confirmationBodyLines() []string {
	lines := []string{
		"Confirm uninstall",
		"==================",
		"This will run shared Uninstall Execute for the selected apps.",
		"Leftover deletion uses the Recycle Bin and runs only after the uninstaller reports success.",
		"A failed or canceled uninstaller deletes nothing.",
		"",
		"Selected applications:",
	}
	// Stable order for deterministic confirmation/test oracle.
	for _, row := range m.selectedRowsStable() {
		lines = append(lines, fmt.Sprintf("  - %s", row.app.Name))
		lines = append(lines, fmt.Sprintf("      plan: %s", uninstallPlannedClassLabel(row.app.PlannedClass)))
		if row.app.RequiresAdmin {
			lines = append(lines, "      requires admin: true (UAC may be requested)")
		}
	}
	lines = append(lines, "")

	leftoverCount := m.confirmedLeftoverCount()
	lines = append(lines, fmt.Sprintf("Confirmed leftover path set: %d path(s) (revalidated subset deleted to Recycle Bin after success)", leftoverCount))
	lines = append(lines, "")

	// Disclose the authorization state the user set in preview. Values are
	// frozen into frozenAllow* only when beginExecution starts the run; during
	// confirmation the live toggles are the source of truth (and cannot change
	// in this phase).
	lines = append(lines,
		fmt.Sprintf("Process-stop authorization: %t", m.allowStopProcesses),
		fmt.Sprintf("Permanent authorization: %t", m.allowPermanent),
	)
	if m.selectionIncludesPortable() {
		if m.allowPermanent {
			lines = append(lines, "Portable directory removal is authorized (permanent deletion of trusted install trees).")
		} else {
			lines = append(lines, "Portable directory removal is NOT authorized: portable targets will be skipped and nothing permanently deleted.")
		}
	}
	lines = append(lines, "")

	if adminApps := m.selectedAdminAppNames(); len(adminApps) > 0 {
		lines = append(lines, "Applications likely requiring administrator rights (UAC):")
		for _, name := range adminApps {
			lines = append(lines, fmt.Sprintf("  - %s", name))
		}
		lines = append(lines, "Without elevation these are skipped with a stable reason.", "")
	}

	lines = append(lines, "Enter: confirm and run shared Execute | esc/b: back to preview")
	return lines
}

func (m uninstallModel) executionBodyLines() []string {
	lines := []string{
		"Running shared Uninstall Execute...",
		fmt.Sprintf("Selected: %d app(s).", len(m.frozenSelection)),
	}
	if m.cancellationRequested {
		lines = append(lines, cancellationRequestedMessage)
	}
	lines = append(lines, "", "Completed work is not rolled back. Ctrl+C requests cooperative cancel.")
	return lines
}

// resultBodyLines reflects the shared ExecuteResult. The TUI never re-derives
// outcomes; it projects per-app outcomes, elevation outcome, and totals.
func (m uninstallModel) resultBodyLines() []string {
	result := m.executionResult
	var lines []string
	switch result.Status {
	case uninstall.StatusExecuteError:
		lines = append(lines, "Status: error")
		if result.Message != "" {
			lines = append(lines, result.Message)
		}
	case uninstall.StatusExecuteCanceled:
		lines = append(lines, "Status: canceled")
		if result.Message != "" {
			lines = append(lines, result.Message)
		}
	default:
		lines = append(lines, "Status: complete")
	}
	lines = append(lines, fmt.Sprintf("Selected: %d, uninstalled: %d, skipped: %d, failed: %d, canceled: %d.",
		result.Totals.SelectedCount,
		result.Totals.UninstalledCount,
		result.Totals.SkippedCount,
		result.Totals.FailedCount,
		result.Totals.CanceledCount,
	))
	if result.Elevation.Requested {
		elevation := "Elevation: "
		if result.Elevation.Granted {
			elevation += "granted"
		} else {
			elevation += "not granted (admin-required apps skipped)"
		}
		if result.Elevation.Reason != "" {
			elevation += " - " + result.Elevation.Reason
		}
		lines = append(lines, elevation)
	}
	for _, app := range result.Applications {
		lines = append(lines, "")
		lines = append(lines, "  - "+uninstallValueOrUnknown(app.Name))
		lines = append(lines, fmt.Sprintf("      plan: %s", uninstallPlannedClassLabel(app.PlannedClass)))
		lines = append(lines, fmt.Sprintf("      action: %s", app.Action))
		lines = append(lines, fmt.Sprintf("      result: %s", app.Result))
		if app.SkippedReason != "" {
			lines = append(lines, fmt.Sprintf("      skipped reason: %s", app.SkippedReason))
		}
		if app.Detail != "" {
			lines = append(lines, "      detail: "+app.Detail)
		}
		if len(app.LeftoverOutcomes) > 0 {
			lines = append(lines, fmt.Sprintf("      leftover paths: %d deleted via Recycle Bin, %d skipped",
				uninstallLeftoverDeletedCount(app.LeftoverOutcomes),
				uninstallLeftoverSkippedCount(app.LeftoverOutcomes)))
		}
	}
	lines = append(lines, "", "Enter/esc/b: back to menu | q: quit")
	return lines
}

func (m uninstallModel) footerHintsLine() string {
	switch m.phase {
	case uninstallPhaseConfirmation:
		return "Hints: j/k scroll | enter confirm | esc/b back | q quit"
	case uninstallPhaseExecuting:
		return "Hints: ctrl+c cooperative cancel"
	case uninstallPhaseResult:
		return ""
	}
	return "Hints: j/k move | space toggle | s stop-proc | p permanent | enter confirm | b back | q quit"
}

func (m uninstallModel) bodyCapacity() int {
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

func (m uninstallModel) clampedViewportOffset(bodyLen, capacity int) int {
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

func (m *uninstallModel) clampViewportOffset() {
	cap := m.bodyCapacity()
	bodyLen := len(m.scrollableBodyLines())
	m.viewportOffset = m.clampedViewportOffset(bodyLen, cap)
}

func (m *uninstallModel) scrollViewport(delta int) {
	if m.terminalTooSmall() {
		return
	}
	m.viewportOffset += delta
	m.clampViewportOffset()
}

// previewBodyLineForRow maps a preview row index to its scrollable body line.
// Line 0 is the "Applications (N):" heading; row i is at line i+1.
func (m uninstallModel) previewBodyLineForRow(rowIndex int) int {
	if rowIndex < 0 || rowIndex >= len(m.rows) {
		return -1
	}
	return rowIndex + 1
}

func (m *uninstallModel) ensurePreviewCursorVisible() {
	if m.loading || len(m.rows) == 0 {
		m.clampViewportOffset()
		return
	}
	m.ensureBodyLineVisible(m.previewBodyLineForRow(m.cursor))
}

func (m *uninstallModel) ensureBodyLineVisible(lineIndex int) {
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

// selectedRowsStable returns selected rows in stable display order (cursor
// order). The frozen selection is captured from this set at confirmation.
func (m uninstallModel) selectedRowsStable() []uninstallAppRow {
	var out []uninstallAppRow
	for _, row := range m.rows {
		if row.selected {
			out = append(out, row)
		}
	}
	return out
}

// uninstallPlannedClassShort is a compact path-free marker for the preview list.
func uninstallPlannedClassShort(class string) string {
	switch class {
	case uninstall.PlannedClassOfficialUninstaller:
		return "official"
	case uninstall.PlannedClassPortableDirectoryRemoval:
		return "portable"
	case uninstall.PlannedClassNotExecutable:
		return "not-executable"
	case uninstall.PlannedClassHardExclusion:
		return "hard-exclusion"
	}
	return class
}

// uninstallPlannedClassLabel maps a stable planned_class JSON value to the
// domain term used in CONTEXT.md and ADRs 0026-0028 for the human report. An
// empty class renders no field.
func uninstallPlannedClassLabel(class string) string {
	switch class {
	case uninstall.PlannedClassOfficialUninstaller:
		return "Official uninstaller invocation"
	case uninstall.PlannedClassPortableDirectoryRemoval:
		return "Portable directory removal"
	case uninstall.PlannedClassNotExecutable:
		return "Not executable"
	case uninstall.PlannedClassHardExclusion:
		return "Uninstall hard exclusion"
	default:
		return ""
	}
}

func uninstallLeftoverDeletedCount(outcomes []uninstall.LeftoverPathOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Result == uninstall.ResultLeftoverDeleted {
			n++
		}
	}
	return n
}

func uninstallLeftoverSkippedCount(outcomes []uninstall.LeftoverPathOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Result == uninstall.ResultLeftoverSkipped {
			n++
		}
	}
	return n
}

// uninstallValueOrUnknown returns the value or "unknown" when empty. Mirrors
// the helper in the uninstall report package without crossing package bounds.
func uninstallValueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
