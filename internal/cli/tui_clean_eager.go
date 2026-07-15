package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// Category-first eager preview model for the Clean TUI. This ticket delivers
// orchestration, path-free rendering, navigation, and cancellation. Primary
// root cutover remains isolated until the later cutover ticket.

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
	CandidateCount int
	Bytes          int64
}

type eagerCategoryObservationMsg struct {
	generation  uint64
	observation clean.CategoryPreviewObservation
}

type eagerPreviewFinishedMsg struct {
	generation uint64
	canceled   bool
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
	for i := range m.rows {
		m.rows[i].State = clean.CategoryPreviewWaiting
		m.rows[i].CandidateCount = 0
		m.rows[i].Bytes = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	return tea.Batch(
		startEagerPreviewCmd(ctx, m.generation, buildEagerPreviewOptions()),
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
	if msg.generation != m.generation || m.canceled {
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
	switch msg.observation.State {
	case clean.CategoryPreviewScanning:
		m.activeIndex = index
	default:
		if m.activeIndex == index {
			m.activeIndex = -1
		}
		if isTerminalPreviewState(msg.observation.State) {
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

func (m *eagerCleanModel) applyTick(msg eagerPreviewTickMsg) tea.Cmd {
	if msg.generation != m.generation || m.canceled || m.finished {
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

func isTerminalPreviewState(state clean.CategoryPreviewState) bool {
	switch state {
	case clean.CategoryPreviewComplete, clean.CategoryPreviewEmpty,
		clean.CategoryPreviewPartial, clean.CategoryPreviewSkipped,
		clean.CategoryPreviewIncomplete, clean.CategoryPreviewFailed:
		return true
	default:
		return false
	}
}

// handleKey processes category-first keys. Browsing never restarts the queue.
// Escape/b request menu; q quits; Ctrl+C is interrupt.
func (m *eagerCleanModel) handleKey(key string) (eagerPreviewNav, tea.Cmd) {
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
	}
	return eagerPreviewNavNone, nil
}

func (m eagerCleanModel) content() string {
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
		builder.WriteString(fmt.Sprintf("  %s %s %s\n", cursor, m.rowMarker(row), m.rowLabel(row)))
	}

	builder.WriteString("\nHints: up/down browse · b/Esc back · q quit\n")
	return builder.String()
}

func (m eagerCleanModel) headerLine() string {
	total := len(m.rows)
	elapsed := m.elapsedLabel()
	if m.canceled {
		return fmt.Sprintf("Canceled · %s", elapsed)
	}
	if m.finished {
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
	default:
		// partial, incomplete, failed — conservative diagnostic marker until #188.
		return "!"
	}
}

func (m eagerCleanModel) rowLabel(row eagerCategoryRow) string {
	switch row.State {
	case clean.CategoryPreviewComplete:
		return fmt.Sprintf("%s · %d item(s) · %s", row.Label, row.CandidateCount, cleanFormatBytes(row.Bytes))
	case clean.CategoryPreviewEmpty:
		return fmt.Sprintf("%s · empty", row.Label)
	case clean.CategoryPreviewWaiting:
		return fmt.Sprintf("%s · waiting", row.Label)
	case clean.CategoryPreviewScanning:
		return fmt.Sprintf("%s · scanning", row.Label)
	default:
		// Conservatively disabled path-free wording; full taxonomy is #188.
		return fmt.Sprintf("%s · unavailable", row.Label)
	}
}
