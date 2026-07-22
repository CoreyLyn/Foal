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
// measurement states (#347). Status and History keep the generic Command viewer.
//
// This slice measures directory children serially after entry and projects
// shared-core states (scanning/complete/partial/incomplete/skipped). Two-worker
// scheduling, path-bound live ranking, cancel/cache/resume, and presentation
// polish arrive in later tickets. Analyze remains read-only: no mutation,
// elevation, process action, or History write.

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
	// browseCursor indexes browseChildren (serial ranking; path-bound live rank is #348).
	browseCursor int
	// gen increments on each browse request so stale loads are ignored.
	gen int
	// measuring is true while a browse stream is active (children may already be visible).
	measuring bool
}

func newAnalyzeDriveModel(width, height int) analyzeDriveModel {
	return analyzeDriveModel{
		phase:   analyzePhaseDrive,
		loading: true,
		width:   width,
		height:  height,
	}
}

func (m *analyzeDriveModel) setSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *analyzeDriveModel) start() tea.Cmd {
	m.phase = analyzePhaseDrive
	m.loading = true
	m.notice = ""
	m.nav = analyzeDriveNavNone
	m.browseRoot = ""
	m.browseChildren = nil
	m.browseCursor = 0
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
	if !msg.result.OK {
		m.notice = fmt.Sprintf("Cannot browse: %s", msg.result.Reason.Message)
		// Failed entry from drive entry: stay on drive list (browseRoot was only
		// a pending target). Failed nested navigation: keep prior browse rows.
		if m.phase != analyzePhaseBrowse {
			m.browseRoot = ""
			m.browseChildren = nil
			m.browseCursor = 0
			m.phase = analyzePhaseDrive
			return nil
		}
		// Already browsing: do not replace successful children with a failed load.
		return nil
	}
	m.phase = analyzePhaseBrowse
	m.browseRoot = msg.result.Root
	// Authoritative final inventory from the shared core (may refine streaming rows).
	m.browseChildren = append([]analyze.BrowseChild(nil), msg.result.Children...)
	if m.browseCursor >= len(m.browseChildren) {
		m.browseCursor = 0
	}
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
	// (enterDrive/enterBrowseChild already set browseRoot.)
	m.browseChildren = nil
	m.browseCursor = 0
	return waitAnalyzeBrowseStreamCmd(msg.gen, msg.stream)
}

func (m *analyzeDriveModel) applyBrowseObservation(msg analyzeBrowseObservationMsg) tea.Cmd {
	if msg.gen != m.gen {
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

// upsertBrowseObservation merges a path-scoped observation into browseChildren.
// Ranking remains serial order for this slice (#348 owns path-bound live ranking).
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
	for i := range m.browseChildren {
		if m.browseChildren[i].Path == obs.Path {
			m.browseChildren[i] = child
			return
		}
	}
	m.browseChildren = append(m.browseChildren, child)
}

func loadAnalyzeBrowseCmd(gen int, root string) tea.Cmd {
	return func() tea.Msg {
		obsCh := make(chan analyze.ChildObservation, 64)
		resultCh := make(chan analyze.BrowseResult, 1)
		go func() {
			result := browseAnalyzeLocation(context.Background(), root, analyze.BrowseOptions{}, func(o analyze.ChildObservation) {
				// Buffer absorbs UI-safe bursts; block rather than drop terminals.
				obsCh <- o
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
			// Cancel-in-progress: bump gen so result is ignored, then navigate back.
			m.gen++
			m.loading = false
			m.measuring = false
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
		if len(m.browseChildren) > 0 {
			m.browseCursor = (m.browseCursor + 1) % len(m.browseChildren)
			m.notice = ""
		}
	case "k", "up":
		if len(m.browseChildren) > 0 {
			m.browseCursor = (m.browseCursor + len(m.browseChildren) - 1) % len(m.browseChildren)
			m.notice = ""
		}
	case "enter":
		return m.enterBrowseChild()
	default:
		m.notice = "Unknown key. Use j/k, enter, esc/b, or q."
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
	m.gen++
	m.loading = true
	m.notice = fmt.Sprintf("Browsing %s...", vol.Letter)
	m.browseRoot = vol.Root
	m.browseChildren = nil
	m.browseCursor = 0
	return analyzeDriveNavNone, loadAnalyzeBrowseCmd(m.gen, vol.Root)
}

func (m *analyzeDriveModel) enterBrowseChild() (analyzeDriveNav, tea.Cmd) {
	if len(m.browseChildren) == 0 {
		m.notice = "This location has no children."
		return analyzeDriveNavNone, nil
	}
	if m.browseCursor < 0 || m.browseCursor >= len(m.browseChildren) {
		return analyzeDriveNavNone, nil
	}
	child := m.browseChildren[m.browseCursor]
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
		m.gen++
		m.loading = true
		m.notice = fmt.Sprintf("Browsing %s...", child.Name)
		return analyzeDriveNavNone, loadAnalyzeBrowseCmd(m.gen, child.Path)
	default:
		m.notice = fmt.Sprintf("%s cannot be entered.", child.Name)
		return analyzeDriveNavNone, nil
	}
}

func (m *analyzeDriveModel) leaveBrowse() (analyzeDriveNav, tea.Cmd) {
	if m.browseRoot == "" || isAnalyzeVolumeRoot(m.browseRoot) {
		// Volume root → drive entry.
		m.phase = analyzePhaseDrive
		m.browseRoot = ""
		m.browseChildren = nil
		m.browseCursor = 0
		m.loading = false
		m.measuring = false
		m.notice = ""
		return analyzeDriveNavNone, nil
	}
	parent := parentBrowsePath(m.browseRoot)
	if parent == "" || isAnalyzeVolumeRoot(parent) && parent == m.browseRoot {
		m.phase = analyzePhaseDrive
		m.browseRoot = ""
		m.browseChildren = nil
		m.browseCursor = 0
		m.loading = false
		m.measuring = false
		m.notice = ""
		return analyzeDriveNavNone, nil
	}
	m.gen++
	m.loading = true
	m.measuring = false
	m.browseChildren = nil
	m.notice = "Returning..."
	return analyzeDriveNavNone, loadAnalyzeBrowseCmd(m.gen, parent)
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
	b.WriteString("| Browse · read-only · direct children on demand    |\n")
	b.WriteString("+--------------------------------------------------+\n\n")
	b.WriteString(fmt.Sprintf("Location: %s\n\n", m.browseRoot))

	if m.loading && len(m.browseChildren) == 0 {
		b.WriteString("Measuring direct children...\n")
		if m.notice != "" {
			b.WriteString(m.notice)
			b.WriteString("\n")
		}
		b.WriteString(analyzeBrowseFooter)
		return b.String()
	}

	locationComplete := !m.measuring && analyze.LocationMeasurementComplete(m.browseChildren)
	observedTotal := analyze.ObservedLocationBytes(m.browseChildren)

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
		b.WriteString("Direct children:\n\n")
		for i, child := range m.browseChildren {
			prefix := "  "
			if i == m.browseCursor {
				prefix = "> "
			}
			b.WriteString(prefix)
			b.WriteString(renderAnalyzeBrowseRow(child, observedTotal, locationComplete))
			b.WriteString("\n")
		}
		// Focused detail: aggregate counts/reasons, never unbounded paths.
		if m.browseCursor >= 0 && m.browseCursor < len(m.browseChildren) {
			b.WriteString("\nDetail: ")
			b.WriteString(analyze.FormatFocusedDetail(m.browseChildren[m.browseCursor]))
			b.WriteString("\n")
		}
	}

	if m.notice != "" {
		b.WriteString("\n")
		b.WriteString(m.notice)
		b.WriteString("\n")
	}
	b.WriteString(analyzeBrowseFooter)
	return b.String()
}

const analyzeDriveFooter = "\nHints: j/k move | enter open | r refresh | esc/b back | q quit\n" +
	"This view is read-only; no cleanup or deletion actions are available.\n" +
	"Drive entry reads volume metadata only until you enter a drive.\n"

const analyzeBrowseFooter = "\nHints: j/k move | enter open directory | esc/b parent | q quit\n" +
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

func renderAnalyzeBrowseRow(child analyze.BrowseChild, observedTotal int64, locationComplete bool) string {
	parts := []string{child.Name, child.Kind}
	if child.Hidden {
		parts = append(parts, "hidden")
	}
	if child.System {
		parts = append(parts, "system")
	}
	if child.Classification != "" {
		parts = append(parts, child.Classification)
	}
	// Always surface the shared-core state token so Partial/Scanning are explicit.
	if child.State != "" {
		parts = append(parts, child.State)
	}
	if child.State == analyze.BrowseStateSkipped && child.SkipReason != "" {
		parts = append(parts, child.SkipReason)
	}
	// Size: Partial/Incomplete as lower-bound ">=bytes"; never invent completeness.
	if child.State != analyze.BrowseStateSkipped {
		parts = append(parts, analyze.FormatSizeToken(child.Bytes, child.State, cleanFormatBytes))
	}
	// Percentage: approximate for scanning/partial/incomplete; exact only when
	// location total is complete and the child is complete. Never ">=N%".
	if share := analyze.FormatSharePercent(child.Bytes, observedTotal, child.State, locationComplete); share != "" {
		parts = append(parts, share)
	}
	if child.Kind == analyze.BrowseKindDirectory && child.Navigable {
		parts = append(parts, "enter")
	}
	return strings.Join(parts, " · ")
}
