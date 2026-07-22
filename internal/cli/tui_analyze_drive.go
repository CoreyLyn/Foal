package cli

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
)

// tui_analyze_drive.go is the Analyze drive-entry TUI (issue #345 / ADR-0034).
// It lists local fixed and removable volumes with inexpensive metadata only.
// Entering a drive (browse-and-measure) is intentionally out of scope for this
// slice (#346). Status and History keep the generic Command viewer.

// analyzeDriveNav is the navigation intent returned by the drive-entry model.
type analyzeDriveNav int

const (
	analyzeDriveNavNone analyzeDriveNav = iota
	analyzeDriveNavMenu
	analyzeDriveNavQuit
	analyzeDriveNavInterrupt
)

// listAnalyzeLocalVolumes is the injectable volume-enumeration seam. Production
// uses the platform probe; tests inject deterministic fakes. Enumeration must
// never scan directory contents.
var listAnalyzeLocalVolumes = func() []analyze.LocalVolume {
	return analyze.ListLocalVolumes(nil)
}

type analyzeVolumesLoadedMsg struct {
	volumes []analyze.LocalVolume
}

// analyzeDriveModel is the Analyze TUI initial state: local drive entry only.
type analyzeDriveModel struct {
	loading bool
	notice  string
	volumes []analyze.LocalVolume
	cursor  int
	width   int
	height  int
	nav     analyzeDriveNav
}

func newAnalyzeDriveModel(width, height int) analyzeDriveModel {
	return analyzeDriveModel{
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
	m.loading = true
	m.notice = ""
	m.nav = analyzeDriveNavNone
	return loadAnalyzeVolumesCmd
}

// loadAnalyzeVolumesCmd enumerates local volumes off the UI loop.
func loadAnalyzeVolumesCmd() tea.Msg {
	return analyzeVolumesLoadedMsg{volumes: listAnalyzeLocalVolumes()}
}

func (m *analyzeDriveModel) applyLoaded(msg analyzeVolumesLoadedMsg) {
	m.loading = false
	m.volumes = append([]analyze.LocalVolume(nil), msg.volumes...)
	m.cursor = analyze.FocusLocalVolumeIndex(m.volumes)
	if m.notice == "Refreshing drives..." {
		m.notice = ""
	}
}

func (m *analyzeDriveModel) handleKey(key string) (analyzeDriveNav, tea.Cmd) {
	m.nav = analyzeDriveNavNone
	if m.loading {
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
		m.handleEnter()
	default:
		m.notice = "Unknown key. Use j/k, r, enter, esc/b, or q."
	}
	return analyzeDriveNavNone, nil
}

func (m *analyzeDriveModel) handleEnter() {
	if len(m.volumes) == 0 {
		m.notice = "No local drives are available."
		return
	}
	if m.cursor < 0 || m.cursor >= len(m.volumes) {
		return
	}
	vol := m.volumes[m.cursor]
	if !vol.Available {
		m.notice = fmt.Sprintf("%s is unavailable and cannot be entered.", vol.Letter)
		return
	}
	// Browse-and-measure is #346. This slice keeps drive entry read-only with
	// zero recursive directory scans; Enter does not start a scan.
	m.notice = fmt.Sprintf(
		"%s selected. Directory browse is not available in this slice; drive entry stays read-only.",
		vol.Letter,
	)
}

func (m analyzeDriveModel) content() string {
	var b strings.Builder
	b.WriteString("+--------------------------------------------------+\n")
	b.WriteString("| Analyze TUI                                      |\n")
	b.WriteString("| Local drive entry · read-only · no dir scan       |\n")
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

const analyzeDriveFooter = "\nHints: j/k move | enter select | r refresh | esc/b back | q quit\n" +
	"This view is read-only; no cleanup or deletion actions are available.\n" +
	"Drive entry reads volume metadata only; it does not scan directory contents.\n"

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
