package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

// tui_analyze_drive.go is the Analyze TUI: drive entry (#345) plus on-demand
// browse-and-measure of every direct child (#346 / ADR-0034) with honest child
// measurement states (#347), two-worker path-bound live ranking (#348), and
// responsive ranked disk-usage presentation (#350).
// Status and History keep the generic Command viewer.
//
// Directory children measure with at most two concurrent workers; focus promotes
// the selected queued path to the next free slot without preempting active work.
// Rows re-rank by latest observed logical bytes while selection stays bound to
// canonical child path. Leaving a location cooperatively cancels unfinished
// child measurements; a session-only in-memory cache reuses durable terminals
// on return and resumes missing work. Refresh discards the current location
// cache and rescans. Analyze remains read-only: no mutation, elevation,
// process action, or History write.

// analyzeDriveNav is the navigation intent returned by the Analyze browser model.
type analyzeDriveNav int

const (
	analyzeDriveNavNone analyzeDriveNav = iota
	analyzeDriveNavMenu
	analyzeDriveNavQuit
	analyzeDriveNavInterrupt
)

// analyzeViewPhase is the Analyze TUI phase.
type analyzeViewPhase int

const (
	analyzePhaseDrive analyzeViewPhase = iota
	analyzePhaseBrowse
)

// listAnalyzeLocalVolumes is the injectable volume-enumeration seam. Production
// uses the platform probe; tests inject deterministic fakes. Enumeration must
// never scan directory contents.
var listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
	return analyze.ListLocalVolumes(nil)
}

// browseAnalyzeLocation is the injectable direct-child browse seam. Production
// streams path-scoped observations from shared Analyze core; tests inject fakes
// and may ignore onObservation. Terminal states and aggregates come from the core.
// opts.Focus carries the live path preference for next-slot promotion.
var browseAnalyzeLocation = func(ctx context.Context, root string, opts analyze.BrowseOptions, onObservation analyze.ObservationHandler) analyze.BrowseResult {
	return analyze.StreamBrowseLocation(ctx, root, opts, onObservation)
}

type analyzeVolumesLoadedMsg struct {
	volumes []analyze.LocalVolume
}

type analyzeBrowseLoadedMsg struct {
	// gen discards stale async results after navigation away.
	gen    int
	result analyze.BrowseResult
}

// analyzeBrowseObservationMsg is one path-scoped child update during measurement.
// Timing is not part of correctness; gen discards stale streams after navigation.
type analyzeBrowseObservationMsg struct {
	gen int
	obs analyze.ChildObservation
	// stream is non-nil so the model can continue draining until the final result.
	stream *analyzeBrowseStream
}

type analyzeBrowseStartedMsg struct {
	gen    int
	stream *analyzeBrowseStream
}

// analyzeBrowseStream is the async observation + result pipe for one browse load.
type analyzeBrowseStream struct {
	observations <-chan analyze.ChildObservation
	result       <-chan analyze.BrowseResult
}

type analyzeDriveModel struct {
	phase   analyzeViewPhase
	loading bool
	notice  string
	volumes []analyze.LocalVolume
	cursor  int
	width   int
	height  int
	nav     analyzeDriveNav

	// Browse state (only meaningful in analyzePhaseBrowse).
	browseRoot     string
	browseChildren []analyze.BrowseChild
	// browseSelectedPath is the canonical child path for selection (not row index).
	// Enter and the cursor marker follow this path through live re-ranking.
	browseSelectedPath string
	// browseOffset is the first visible row index in the vertical viewport.
	// The viewport follows browseSelectedPath as ranks change.
	browseOffset int
	// browseFocus is shared with the in-flight browse stream so j/k promote
	// queued directory work without preempting active measurements.
	browseFocus *analyze.AtomicBrowseFocus
	// gen increments on each browse request so stale loads are ignored.
	gen int
	// measuring is true while a browse stream is active (children may already be visible).
	measuring bool
	// sessionCache holds durable terminal child summaries for this Analyze TUI
	// session only. Never persisted, never written to History, never used by cleanup.
	sessionCache *analyze.BrowseSessionCache
	// browseCancel cancels the in-flight StreamBrowseLocation for the current gen.
	// Nil when no browse stream is active.
	browseCancel context.CancelFunc
}

func newAnalyzeDriveModel(width, height int) analyzeDriveModel {
	return analyzeDriveModel{
		phase:        analyzePhaseDrive,
		loading:      true,
		width:        width,
		height:       height,
		sessionCache: analyze.NewBrowseSessionCache(),
	}
}

func (m *analyzeDriveModel) setSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *analyzeDriveModel) start() tea.Cmd {
	m.cancelBrowseWork()
	m.phase = analyzePhaseDrive
	m.loading = true
	m.notice = ""
	m.nav = analyzeDriveNavNone
	m.browseRoot = ""
	m.browseChildren = nil
	m.browseSelectedPath = ""
	m.browseOffset = 0
	m.browseFocus = nil
	m.measuring = false
	if m.sessionCache == nil {
		m.sessionCache = analyze.NewBrowseSessionCache()
	} else {
		m.sessionCache.Clear()
	}
	return loadAnalyzeVolumesCmd
}

// loadAnalyzeVolumesCmd enumerates local volumes off the UI loop.
func loadAnalyzeVolumesCmd() tea.Msg {
	return analyzeVolumesLoadedMsg{volumes: listAnalyzeLocalVolumes()}
}

func (m *analyzeDriveModel) applyLoaded(msg analyzeVolumesLoadedMsg) {
	if m.phase != analyzePhaseDrive {
		return
	}
	m.loading = false
	m.volumes = append([]analyze.LocalVolume(nil), msg.volumes...)
	m.cursor = analyze.FocusLocalVolumeIndex(m.volumes)
	if m.notice == "Refreshing drives..." {
		m.notice = ""
	}
}

func (m *analyzeDriveModel) applyBrowseLoaded(msg analyzeBrowseLoadedMsg) tea.Cmd {
	if msg.gen != m.gen {
		return nil
	}
	m.loading = false
	m.measuring = false
	m.browseCancel = nil
	if !msg.result.OK {
		m.notice = fmt.Sprintf("Cannot browse: %s", msg.result.Reason.Message)
		// Failed entry from drive entry: stay on drive list (browseRoot was only
		// a pending target). Failed nested navigation: keep prior browse rows.
		if m.phase != analyzePhaseBrowse {
			m.browseRoot = ""
			m.browseChildren = nil
			m.browseSelectedPath = ""
			m.browseOffset = 0
			m.browseFocus = nil
			m.phase = analyzePhaseDrive
			return nil
		}
		// Already browsing: do not replace successful children with a failed load.
		return nil
	}
	m.phase = analyzePhaseBrowse
	m.browseRoot = msg.result.Root
	// Authoritative final inventory from the shared core (already ranked).
	m.browseChildren = append([]analyze.BrowseChild(nil), msg.result.Children...)
	// Retain only durable terminals for this session (not nav-cancel Incomplete).
	m.ensureSessionCache().PutAll(msg.result.Root, msg.result.Children)
	m.syncBrowseSelectionAfterRank()
	m.notice = ""
	return nil
}

func (m *analyzeDriveModel) applyBrowseStarted(msg analyzeBrowseStartedMsg) tea.Cmd {
	if msg.gen != m.gen || msg.stream == nil {
		return nil
	}
	m.phase = analyzePhaseBrowse
	m.loading = false
	m.measuring = true
	// Keep path identity; clear prior inventory for the new location load.
	// (enterDrive/enterBrowseChild already set browseRoot and browseFocus.)
	m.browseChildren = nil
	m.browseSelectedPath = ""
	m.browseOffset = 0
	return waitAnalyzeBrowseStreamCmd(msg.gen, msg.stream)
}

func (m *analyzeDriveModel) applyBrowseObservation(msg analyzeBrowseObservationMsg) tea.Cmd {
	if msg.gen != m.gen {
		// Stale stream after navigation cancel: do not accept into a newer location.
		return nil
	}
	m.phase = analyzePhaseBrowse
	m.loading = false
	m.measuring = true
	m.upsertBrowseObservation(msg.obs)
	if msg.stream != nil {
		return waitAnalyzeBrowseStreamCmd(msg.gen, msg.stream)
	}
	return nil
}

// upsertBrowseObservation merges a path-scoped observation into browseChildren,
// re-ranks by latest observed logical bytes, and keeps selection on the same path.
func (m *analyzeDriveModel) upsertBrowseObservation(obs analyze.ChildObservation) {
	child := analyze.BrowseChild{
		Name:           obs.Name,
		Path:           obs.Path,
		Kind:           obs.Kind,
		Bytes:          obs.Bytes,
		FileCount:      obs.FileCount,
		DirectoryCount: obs.DirectoryCount,
		Classification: obs.Classification,
		State:          obs.State,
		SkipReason:     obs.SkipReason,
		SkipAggregates: append([]analyze.SkipAggregate(nil), obs.SkipAggregates...),
		Hidden:         obs.Hidden,
		System:         obs.System,
		Navigable:      obs.Navigable,
	}
	found := false
	for i := range m.browseChildren {
		if m.browseChildren[i].Path == obs.Path {
			m.browseChildren[i] = child
			found = true
			break
		}
	}
	if !found {
		m.browseChildren = append(m.browseChildren, child)
	}
	// Default selection to the first discovered child when none is set yet.
	if m.browseSelectedPath == "" && child.Path != "" {
		m.browseSelectedPath = child.Path
		m.publishBrowseFocus()
	}
	// Session cache only durable terminals so mid-scan leave keeps completed work.
	if analyze.IsDurableCachedChild(child) && m.browseRoot != "" {
		m.ensureSessionCache().Put(m.browseRoot, child)
	}
	m.browseChildren = analyze.RankBrowseChildren(m.browseChildren)
	m.syncBrowseSelectionAfterRank()
}

// selectedBrowseChild returns the child bound to browseSelectedPath, if any.
func (m *analyzeDriveModel) selectedBrowseChild() (analyze.BrowseChild, bool) {
	idx := analyze.IndexOfBrowsePath(m.browseChildren, m.browseSelectedPath)
	if idx < 0 {
		return analyze.BrowseChild{}, false
	}
	return m.browseChildren[idx], true
}

// syncBrowseSelectionAfterRank keeps path selection valid and viewport on-path
// after a rank change. If the selected path disappeared, fall back to the first row.
func (m *analyzeDriveModel) syncBrowseSelectionAfterRank() {
	if len(m.browseChildren) == 0 {
		m.browseSelectedPath = ""
		m.browseOffset = 0
		m.publishBrowseFocus()
		return
	}
	idx := analyze.IndexOfBrowsePath(m.browseChildren, m.browseSelectedPath)
	if idx < 0 {
		m.browseSelectedPath = m.browseChildren[0].Path
		idx = 0
		m.publishBrowseFocus()
	}
	m.ensureBrowseSelectionVisible(idx)
}

// browseViewportRows estimates how many child rows fit in the content area.
// A conservative fixed reserve keeps the selected path visible without needing
// full layout measurement in this slice.
func (m analyzeDriveModel) browseViewportRows() int {
	// Header (~6) + measuring banner (~2) + detail (~2) + footer (~4) + padding.
	const reserved = 16
	rows := m.height - reserved
	if rows < 3 {
		rows = 3
	}
	return rows
}

// ensureBrowseSelectionVisible scrolls browseOffset so selectedIdx is on-screen.
func (m *analyzeDriveModel) ensureBrowseSelectionVisible(selectedIdx int) {
	if selectedIdx < 0 {
		return
	}
	n := len(m.browseChildren)
	if n == 0 {
		m.browseOffset = 0
		return
	}
	vis := m.browseViewportRows()
	if selectedIdx < m.browseOffset {
		m.browseOffset = selectedIdx
	} else if selectedIdx >= m.browseOffset+vis {
		m.browseOffset = selectedIdx - vis + 1
	}
	maxOff := n - vis
	if maxOff < 0 {
		maxOff = 0
	}
	if m.browseOffset < 0 {
		m.browseOffset = 0
	}
	if m.browseOffset > maxOff {
		m.browseOffset = maxOff
	}
}

func (m *analyzeDriveModel) publishBrowseFocus() {
	if m.browseFocus != nil {
		m.browseFocus.Set(m.browseSelectedPath)
	}
}

// moveBrowseSelection steps the path-bound cursor by delta (+1 down / -1 up)
// through the current ranked list and promotes the new path for next-slot work.
func (m *analyzeDriveModel) moveBrowseSelection(delta int) {
	n := len(m.browseChildren)
	if n == 0 {
		return
	}
	idx := analyze.IndexOfBrowsePath(m.browseChildren, m.browseSelectedPath)
	if idx < 0 {
		idx = 0
	}
	idx = (idx + delta%n + n) % n
	m.browseSelectedPath = m.browseChildren[idx].Path
	m.publishBrowseFocus()
	m.ensureBrowseSelectionVisible(idx)
	m.notice = ""
}

func loadAnalyzeBrowseCmd(gen int, root string, focus *analyze.AtomicBrowseFocus, known []analyze.BrowseChild, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}
		obsCh := make(chan analyze.ChildObservation, 64)
		resultCh := make(chan analyze.BrowseResult, 1)
		go func() {
			opts := analyze.BrowseOptions{
				Focus:         focus,
				KnownChildren: known,
			}
			result := browseAnalyzeLocation(ctx, root, opts, func(o analyze.ChildObservation) {
				// Buffer absorbs UI-safe bursts; after cancel drop remaining obs
				// so stale work cannot block forever on a dead consumer.
				select {
				case obsCh <- o:
				case <-ctx.Done():
				}
			})
			close(obsCh)
			resultCh <- result
		}()
		return analyzeBrowseStartedMsg{
			gen: gen,
			stream: &analyzeBrowseStream{
				observations: obsCh,
				result:       resultCh,
			},
		}
	}
}

// waitAnalyzeBrowseStreamCmd prefers observations, then the final BrowseResult.
// Cadence is driven by core throttle + channel availability, not by this waiter.
func waitAnalyzeBrowseStreamCmd(gen int, stream *analyzeBrowseStream) tea.Cmd {
	return func() tea.Msg {
		if stream == nil {
			return analyzeBrowseLoadedMsg{gen: gen, result: analyze.BrowseResult{OK: false}}
		}
		if stream.observations != nil {
			if obs, ok := <-stream.observations; ok {
				return analyzeBrowseObservationMsg{gen: gen, obs: obs, stream: stream}
			}
		}
		result, ok := <-stream.result
		if !ok {
			return analyzeBrowseLoadedMsg{gen: gen, result: analyze.BrowseResult{OK: false}}
		}
		return analyzeBrowseLoadedMsg{gen: gen, result: result}
	}
}

func (m *analyzeDriveModel) handleKey(key string) (analyzeDriveNav, tea.Cmd) {
	m.nav = analyzeDriveNavNone
	// Block interaction only while the initial load has no rows yet. During
	// streaming measurement, children may already be visible for navigation.
	if m.loading && len(m.browseChildren) == 0 && m.phase != analyzePhaseBrowse {
		switch key {
		case "ctrl+c":
			return analyzeDriveNavInterrupt, nil
		case "q":
			return analyzeDriveNavQuit, nil
		case "esc", "escape", "b":
			return analyzeDriveNavMenu, nil
		default:
			return analyzeDriveNavNone, nil
		}
	}
	if m.loading && len(m.browseChildren) == 0 && m.phase == analyzePhaseBrowse {
		switch key {
		case "ctrl+c":
			return analyzeDriveNavInterrupt, nil
		case "q":
			return analyzeDriveNavQuit, nil
		case "esc", "escape", "b":
			// Cancel-in-progress: stop workers and discard stale stream, then navigate back.
			return m.leaveBrowse()
		default:
			return analyzeDriveNavNone, nil
		}
	}

	if m.phase == analyzePhaseBrowse {
		return m.handleBrowseKey(key)
	}
	return m.handleDriveKey(key)
}

func (m *analyzeDriveModel) handleDriveKey(key string) (analyzeDriveNav, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return analyzeDriveNavInterrupt, nil
	case "q":
		return analyzeDriveNavQuit, nil
	case "esc", "escape", "b":
		return analyzeDriveNavMenu, nil
	case "j", "down":
		if len(m.volumes) > 0 {
			m.cursor = (m.cursor + 1) % len(m.volumes)
			m.notice = ""
		}
	case "k", "up":
		if len(m.volumes) > 0 {
			m.cursor = (m.cursor + len(m.volumes) - 1) % len(m.volumes)
			m.notice = ""
		}
	case "r":
		m.loading = true
		m.notice = "Refreshing drives..."
		return analyzeDriveNavNone, loadAnalyzeVolumesCmd
	case "enter":
		return m.enterDrive()
	default:
		m.notice = "Unknown key. Use j/k, r, enter, esc/b, or q."
	}
	return analyzeDriveNavNone, nil
}

func (m *analyzeDriveModel) handleBrowseKey(key string) (analyzeDriveNav, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return analyzeDriveNavInterrupt, nil
	case "q":
		return analyzeDriveNavQuit, nil
	case "esc", "escape", "b":
		return m.leaveBrowse()
	case "j", "down":
		m.moveBrowseSelection(1)
	case "k", "up":
		m.moveBrowseSelection(-1)
	case "r":
		// Refresh: cancel active work, discard this location's session cache,
		// re-enumerate, and rescan. Does not clear sibling locations.
		if m.browseRoot == "" {
			return analyzeDriveNavNone, nil
		}
		return analyzeDriveNavNone, m.beginBrowseLocation(m.browseRoot, "Refreshing...", true)
	case "enter":
		return m.enterBrowseChild()
	default:
		m.notice = "Unknown key. Use j/k, enter, r refresh, esc/b, or q."
	}
	return analyzeDriveNavNone, nil
}

func (m *analyzeDriveModel) enterDrive() (analyzeDriveNav, tea.Cmd) {
	if len(m.volumes) == 0 {
		m.notice = "No local drives are available."
		return analyzeDriveNavNone, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.volumes) {
		return analyzeDriveNavNone, nil
	}
	vol := m.volumes[m.cursor]
	if !vol.Available {
		m.notice = fmt.Sprintf("%s is unavailable and cannot be entered.", vol.Letter)
		return analyzeDriveNavNone, nil
	}
	// Enumerate and measure only after entry; no sibling drives are prefetched.
	return analyzeDriveNavNone, m.beginBrowseLocation(vol.Root, fmt.Sprintf("Browsing %s...", vol.Letter), false)
}

func (m *analyzeDriveModel) enterBrowseChild() (analyzeDriveNav, tea.Cmd) {
	if len(m.browseChildren) == 0 {
		m.notice = "This location has no children."
		return analyzeDriveNavNone, nil
	}
	// Enter always targets the path selected before the latest re-ranking.
	child, ok := m.selectedBrowseChild()
	if !ok {
		m.notice = "No child is selected."
		return analyzeDriveNavNone, nil
	}
	switch {
	case child.Kind == analyze.BrowseKindFile:
		m.notice = fmt.Sprintf("%s is a file and cannot be entered.", child.Name)
		return analyzeDriveNavNone, nil
	case child.Kind == analyze.BrowseKindReparse || !child.Navigable:
		if child.SkipReason == "reparse_point" || child.Kind == analyze.BrowseKindReparse {
			m.notice = fmt.Sprintf("%s is a reparse point and is not navigable.", child.Name)
		} else {
			m.notice = fmt.Sprintf("%s cannot be entered.", child.Name)
		}
		return analyzeDriveNavNone, nil
	case child.Kind == analyze.BrowseKindDirectory && child.Navigable:
		return analyzeDriveNavNone, m.beginBrowseLocation(child.Path, fmt.Sprintf("Browsing %s...", child.Name), false)
	default:
		m.notice = fmt.Sprintf("%s cannot be entered.", child.Name)
		return analyzeDriveNavNone, nil
	}
}

func (m *analyzeDriveModel) leaveBrowse() (analyzeDriveNav, tea.Cmd) {
	// Persist durable terminals observed so far, then cancel unfinished work.
	if m.browseRoot != "" && len(m.browseChildren) > 0 {
		m.ensureSessionCache().PutAll(m.browseRoot, m.browseChildren)
	}
	m.cancelBrowseWork()
	m.gen++ // discard any late observations from the canceled generation

	if m.browseRoot == "" || isAnalyzeVolumeRoot(m.browseRoot) {
		// Volume root → drive entry.
		m.phase = analyzePhaseDrive
		m.browseRoot = ""
		m.browseChildren = nil
		m.browseSelectedPath = ""
		m.browseOffset = 0
		m.browseFocus = nil
		m.loading = false
		m.measuring = false
		m.notice = ""
		return analyzeDriveNavNone, nil
	}
	parent := parentBrowsePath(m.browseRoot)
	if parent == "" || (isAnalyzeVolumeRoot(parent) && parent == m.browseRoot) {
		m.phase = analyzePhaseDrive
		m.browseRoot = ""
		m.browseChildren = nil
		m.browseSelectedPath = ""
		m.browseOffset = 0
		m.browseFocus = nil
		m.loading = false
		m.measuring = false
		m.notice = ""
		return analyzeDriveNavNone, nil
	}
	return analyzeDriveNavNone, m.beginBrowseLocation(parent, "Returning...", false)
}

func (m *analyzeDriveModel) ensureSessionCache() *analyze.BrowseSessionCache {
	if m.sessionCache == nil {
		m.sessionCache = analyze.NewBrowseSessionCache()
	}
	return m.sessionCache
}

// cancelBrowseWork cooperatively cancels any in-flight location measurement.
// Stale observations are discarded via gen mismatch; nav-cancel Incomplete is
// not written as durable hard-limit cache (Put filters non-durable).
func (m *analyzeDriveModel) cancelBrowseWork() {
	if m.browseCancel != nil {
		m.browseCancel()
		m.browseCancel = nil
	}
	m.measuring = false
}

// beginBrowseLocation cancels prior work, opens a new generation, and starts
// streaming browse for root. When refresh is true the location session cache
// is discarded so every child is re-enumerated and remeasured.
func (m *analyzeDriveModel) beginBrowseLocation(root string, notice string, refresh bool) tea.Cmd {
	// Persist durable terminals for the location we are leaving (nav or enter).
	if m.browseRoot != "" && len(m.browseChildren) > 0 {
		m.ensureSessionCache().PutAll(m.browseRoot, m.browseChildren)
	}
	m.cancelBrowseWork()
	m.gen++
	if refresh {
		m.ensureSessionCache().ClearLocation(root)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.browseCancel = cancel
	m.loading = true
	m.measuring = false
	m.notice = notice
	m.browseRoot = root
	m.browseChildren = nil
	m.browseSelectedPath = ""
	m.browseOffset = 0
	m.browseFocus = analyze.NewAtomicBrowseFocus()
	known := m.ensureSessionCache().KnownFor(root)
	return loadAnalyzeBrowseCmd(m.gen, root, m.browseFocus, known, ctx)
}

// isAnalyzeVolumeRoot reports whether path is a drive-letter volume root (C:\).
func isAnalyzeVolumeRoot(path string) bool {
	cleaned := filepath.Clean(path)
	vol := filepath.VolumeName(cleaned)
	if vol == "" {
		return false
	}
	// filepath.Clean(`C:\`) stays `C:\` on Windows; also accept `C:`.
	root := vol + `\`
	return strings.EqualFold(cleaned, root) || strings.EqualFold(cleaned, vol)
}

func parentBrowsePath(path string) string {
	cleaned := filepath.Clean(path)
	if isAnalyzeVolumeRoot(cleaned) {
		return cleaned
	}
	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		return cleaned
	}
	return parent
}

func (m analyzeDriveModel) content() string {
	if m.phase == analyzePhaseBrowse || (m.loading && m.browseRoot != "") {
		return m.browseContent()
	}
	return m.driveContent()
}

func (m analyzeDriveModel) driveContent() string {
	var b strings.Builder
	b.WriteString("+--------------------------------------------------+\n")
	b.WriteString("| Analyze TUI                                      |\n")
	b.WriteString("| Local drive entry · read-only · on-demand browse  |\n")
	b.WriteString("+--------------------------------------------------+\n\n")

	if m.loading {
		b.WriteString("Loading local drives...\n")
		if m.notice != "" {
			b.WriteString(m.notice)
			b.WriteString("\n")
		}
		b.WriteString(analyzeDriveFooter)
		return b.String()
	}

	if len(m.volumes) == 0 {
		b.WriteString("No supported local fixed or removable drives were found.\n")
		b.WriteString("Network, optical, UNC, and device paths are excluded.\n")
	} else {
		b.WriteString("Local volumes (fixed and removable):\n\n")
		for i, vol := range m.volumes {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}
			b.WriteString(prefix)
			b.WriteString(renderAnalyzeDriveRow(vol))
			b.WriteString("\n")
		}
	}

	if m.notice != "" {
		b.WriteString("\n")
		b.WriteString(m.notice)
		b.WriteString("\n")
	}
	b.WriteString(analyzeDriveFooter)
	return b.String()
}

func (m analyzeDriveModel) browseContent() string {
	var b strings.Builder
	b.WriteString("+--------------------------------------------------+\n")
	b.WriteString("| Analyze TUI                                      |\n")
	b.WriteString("| Browse · ranked logical disk usage · read-only    |\n")
	b.WriteString("+--------------------------------------------------+\n\n")
	b.WriteString(fmt.Sprintf("Location: %s\n", m.browseRoot))

	if m.loading && len(m.browseChildren) == 0 {
		b.WriteString("\nMeasuring direct children...\n")
		if m.notice != "" {
			b.WriteString(m.notice)
			b.WriteString("\n")
		}
		b.WriteString(analyzeBrowseFooter)
		return b.String()
	}

	locationComplete := !m.measuring && analyze.LocationMeasurementComplete(m.browseChildren)
	observedTotal := analyze.ObservedLocationBytes(m.browseChildren)

	// Volume capacity/free is independent of summed logical child bytes.
	if vol := m.volumeForBrowseRoot(); vol != nil {
		if meta := FormatAnalyzeVolumeMetaLine(vol); meta != "" {
			b.WriteString(meta)
			b.WriteString("\n")
		}
	}
	b.WriteString(FormatAnalyzeLocationTotalsLine(observedTotal, locationComplete, m.measuring))
	b.WriteString("\n\n")

	if m.measuring {
		b.WriteString("Measuring (approximate observed shares while scanning)...\n\n")
	}

	if len(m.browseChildren) == 0 {
		if m.measuring {
			b.WriteString("Waiting for first child observations...\n")
		} else {
			b.WriteString("No direct children in this location.\n")
		}
	} else {
		b.WriteString("Direct children (ranked by observed logical bytes):\n\n")
		selectedIdx := analyze.IndexOfBrowsePath(m.browseChildren, m.browseSelectedPath)
		vis := m.browseViewportRows()
		start := m.browseOffset
		if start < 0 {
			start = 0
		}
		if start > len(m.browseChildren) {
			start = 0
		}
		end := start + vis
		if end > len(m.browseChildren) {
			end = len(m.browseChildren)
		}
		width := m.width
		if width <= 0 {
			width = 80
		}
		for i := start; i < end; i++ {
			child := m.browseChildren[i]
			selected := child.Path == m.browseSelectedPath || i == selectedIdx
			// Rank is absolute position in the full ranked list (not viewport-local).
			b.WriteString(FormatAnalyzeRankRow(AnalyzeRankRowInput{
				Child:            child,
				Rank:             i + 1,
				ObservedTotal:    observedTotal,
				LocationComplete: locationComplete,
				Selected:         selected,
				Width:            width,
			}))
			b.WriteString("\n")
		}
		// Focused detail: state, bytes, counts, skipped total, aggregate reasons.
		if child, ok := m.selectedBrowseChild(); ok {
			b.WriteString("\nDetail: ")
			b.WriteString(FormatAnalyzeFocusedDetailLine(child))
			b.WriteString("\n")
		}
	}

	// Guarded copy-only Purge handoff: only when the current browse location
	// passes Purge root validation and has a direct artifact clue. Never
	// launches Purge or transfers selection.
	if handoff := FormatAnalyzePurgeHandoffLine(m.browseRoot, m.browseChildren); handoff != "" {
		b.WriteString("\n")
		b.WriteString(handoff)
		b.WriteString("\n")
	}

	if m.notice != "" {
		b.WriteString("\n")
		b.WriteString(m.notice)
		b.WriteString("\n")
	}
	b.WriteString(analyzeBrowseFooter)
	return b.String()
}

// volumeForBrowseRoot returns volume metadata when the current browse root is a
// listed volume root; otherwise nil (nested paths do not inherit capacity lines).
func (m analyzeDriveModel) volumeForBrowseRoot() *analyze.LocalVolume {
	if m.browseRoot == "" || !isAnalyzeVolumeRoot(m.browseRoot) {
		return nil
	}
	for i := range m.volumes {
		if strings.EqualFold(filepath.Clean(m.volumes[i].Root), filepath.Clean(m.browseRoot)) {
			return &m.volumes[i]
		}
	}
	return nil
}

const analyzeDriveFooter = "\nHints: j/k move | enter open | r refresh | esc/b back | q quit\n" +
	"This view is read-only; no cleanup or deletion actions are available.\n" +
	"Drive entry reads volume metadata only until you enter a drive.\n"

const analyzeBrowseFooter = "\nHints: j/k move | enter open directory | r refresh | esc/b parent | q quit\n" +
	"This view is read-only; no cleanup or deletion actions are available.\n" +
	"Files and reparse points are listed but not navigable. Hidden/system stay visible.\n"

func renderAnalyzeDriveRow(vol analyze.LocalVolume) string {
	parts := []string{vol.Letter}
	if vol.Label != "" {
		parts = append(parts, vol.Label)
	}
	kind := string(vol.Kind)
	if kind == "" {
		kind = "local"
	}
	parts = append(parts, kind)
	if vol.FileSystem != "" {
		parts = append(parts, vol.FileSystem)
	}
	if vol.HasCapacity {
		parts = append(parts, fmt.Sprintf("total %s", cleanFormatBytes(int64(vol.TotalBytes))))
		parts = append(parts, fmt.Sprintf("free %s", cleanFormatBytes(int64(vol.FreeBytes))))
	} else if !vol.Available {
		parts = append(parts, "capacity unavailable")
	}
	if !vol.Available {
		parts = append(parts, "[unavailable]")
	}
	return strings.Join(parts, " · ")
}

// renderAnalyzeBrowseRow is the compatibility plain-row helper used by older tests.
// Production browse rows go through FormatAnalyzeRankRow (responsive ranked layout).
func renderAnalyzeBrowseRow(child analyze.BrowseChild, observedTotal int64, locationComplete bool) string {
	return FormatAnalyzeRankRow(AnalyzeRankRowInput{
		Child:            child,
		Rank:             1,
		ObservedTotal:    observedTotal,
		LocationComplete: locationComplete,
		Selected:         false,
		Width:            120,
	})
}
