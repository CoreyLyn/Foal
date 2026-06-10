package cli

import (
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type tuiScreen int

const (
	screenMenu tuiScreen = iota
	screenCleanPreview
)

type mainMenuItem struct {
	title       string
	command     string
	description string
	selection   string
}

var mainMenuItems = []mainMenuItem{
	{
		title:       "Clean",
		command:     "clean",
		description: "Browse conservative clean preview data from the existing dry-run read model.",
		selection:   "",
	},
	{
		title:       "Uninstall",
		command:     "uninstall",
		description: "Review installed application evidence; preview-only, no uninstallers are executed.",
		selection:   "Uninstall TUI path\nUninstall remains preview-only; no uninstallers are executed, no processes are stopped, and no leftovers are deleted.\nNo files were changed.",
	},
	{
		title:       "Analyze",
		command:     "analyze",
		description: "Inspect disk usage through the existing read-only command path.",
		selection:   "Analyze TUI path\nAnalyze is available through `foal analyze --json <path>`; the read-only view is not built in this slice.\nNo files were changed.",
	},
	{
		title:       "Status",
		command:     "status",
		description: "Inspect a read-only system and Foal state snapshot.",
		selection:   "Status TUI path\nStatus is available through `foal status --json`; the read-only view is not built in this slice.\nNo files were changed.",
	},
	{
		title:       "History",
		command:     "history",
		description: "Browse prior Foal operation records through the existing JSON contract.",
		selection:   "History TUI path\nHistory is available through `foal history --json`; the read-only view is not built in this slice.\nNo files were changed.",
	},
	{
		title:       "Extensions",
		command:     "future",
		description: "Reserved for future read-only command views.",
		selection:   "Extensions\nFuture read-only views are not built in this slice.\nNo files were changed.",
	},
}

type rootModel struct {
	screen   tuiScreen
	selected int
	notice   string
	clean    cleanModel
	width    int
	height   int
}

func newRootModel() rootModel {
	// Sane defaults until the first WindowSizeMsg arrives.
	return rootModel{width: 80, height: 24}
}

func (m rootModel) Init() tea.Cmd {
	return nil
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clean.setSize(msg.Width, msg.Height)
		return m, nil
	case cleanPreviewLoadedMsg:
		m.clean.applyLoaded(msg)
		return m, nil
	case tea.KeyPressMsg:
		if m.screen == screenCleanPreview {
			return m.updateCleanPreviewKey(msg)
		}
		return m.updateMenuKey(msg)
	}
	return m, nil
}

func (m rootModel) updateMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Interrupt
	case "q", "esc":
		return m, tea.Quit
	case "j", "down":
		m.selected = (m.selected + 1) % len(mainMenuItems)
		m.notice = ""
	case "k", "up":
		m.selected = (m.selected + len(mainMenuItems) - 1) % len(mainMenuItems)
		m.notice = ""
	case "enter":
		if mainMenuItems[m.selected].command == "clean" {
			m.screen = screenCleanPreview
			m.clean = newCleanModel(m.width, m.height)
			return m, loadCleanPreviewCmd
		}
		m.notice = mainMenuItems[m.selected].selection
	default:
		m.notice = "Unknown key. Use j/k, up/down, enter, or q."
	}
	return m, nil
}

func (m rootModel) updateCleanPreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Interrupt
	case "q", "esc":
		return m, tea.Quit
	case "b":
		m.screen = screenMenu
		m.notice = ""
		return m, nil
	case "r":
		m.clean.beginReload()
		return m, loadCleanPreviewCmd
	}
	m.clean.handleKey(msg.String())
	return m, nil
}

// content is the plain text frame. It is separate from View so the nil-input
// entry path and unit tests can assert on the rendered text directly.
func (m rootModel) content() string {
	if m.screen == screenCleanPreview {
		return m.clean.content()
	}
	if m.notice != "" && strings.Contains(m.notice, "\n") {
		// Placeholder selections render above the menu, matching the
		// pre-framework frame layout.
		return m.notice + "\n" + renderMainMenu(m.selected, "")
	}
	return renderMainMenu(m.selected, m.notice)
}

func (m rootModel) View() tea.View {
	view := tea.NewView(stylizeFrame(m.content()))
	view.AltScreen = true
	return view
}

func renderMainMenu(selected int, notice string) string {
	var builder strings.Builder
	builder.WriteString("+--------------------------------------------------+\n")
	builder.WriteString("| FOAL                                             |\n")
	builder.WriteString("| Safe, preview-first cleanup for Windows          |\n")
	builder.WriteString("+--------------------------------------------------+\n\n")
	builder.WriteString("Foal main menu\n")
	builder.WriteString("Safe, preview-first cleanup for Windows\n")
	builder.WriteString("This is a read-only navigation shell over existing Foal command paths.\n\n")
	builder.WriteString("Commands:\n")
	for index, item := range mainMenuItems {
		prefix := " "
		if index == selected {
			prefix = ">"
		}
		builder.WriteString(fmt.Sprintf("%s %-10s %-10s %s\n", prefix, item.title, "("+item.command+")", item.description))
	}
	if notice != "" {
		builder.WriteString("\n")
		builder.WriteString(notice)
		builder.WriteString("\n")
	}
	builder.WriteString("\nHints: j/k or up/down: move | enter: open | q: quit\n")
	return builder.String()
}

// runMenuProgram is a seam so RunInvocation tests can stub the interactive
// program run.
var runMenuProgram = func(model tea.Model, input io.Reader, output io.Writer) (tea.Model, error) {
	return tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output)).Run()
}
