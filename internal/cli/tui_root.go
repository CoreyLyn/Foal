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
	screenViewer
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
		selection:   "",
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
		selection:   "",
	},
	{
		title:       "History",
		command:     "history",
		description: "Browse prior Foal operation records through the existing JSON contract.",
		selection:   "",
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
	viewer   viewerModel
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
		m.viewer.setSize(msg.Width, msg.Height)
		return m, nil
	case cleanPreviewLoadedMsg:
		m.clean.applyLoaded(msg)
		return m, nil
	case viewerLoadedMsg:
		m.viewer.applyLoaded(msg)
		return m, nil
	case tea.KeyPressMsg:
		switch m.screen {
		case screenCleanPreview:
			return m.updateCleanPreviewKey(msg)
		case screenViewer:
			return m.updateViewerKey(msg)
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
		switch mainMenuItems[m.selected].command {
		case "clean":
			m.screen = screenCleanPreview
			m.clean = newCleanModel(m.width, m.height)
			return m, loadCleanPreviewCmd
		case "uninstall", "status", "history":
			command := mainMenuItems[m.selected].command
			m.screen = screenViewer
			m.viewer = newViewerModel(command, m.width, m.height)
			return m, loadViewerCmd(command)
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

func (m rootModel) updateViewerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		m.viewer.beginReload()
		return m, loadViewerCmd(m.viewer.command)
	}
	m.viewer.handleKey(msg.String())
	return m, nil
}

// content is the plain text frame. It is separate from View so the nil-input
// entry path and unit tests can assert on the rendered text directly.
func (m rootModel) content() string {
	switch m.screen {
	case screenCleanPreview:
		return m.clean.content()
	case screenViewer:
		return m.viewer.content()
	}
	return renderMainMenu(m.selected, m.notice)
}

func (m rootModel) View() tea.View {
	view := tea.NewView(stylizeFrame(m.content()))
	view.AltScreen = true
	return view
}

const (
	bannerURL     = "https://github.com/CoreyLyn/Foal"
	bannerTagline = "Safe, preview-first cleanup for Windows."
)

// foalBannerArt is a hand-set figlet-style "FOAL" wordmark; every row is 34
// columns and uses only the _|/\ charset so stylizeLine can recognize it.
var foalBannerArt = []string{
	` ______   ____             _      `,
	`|  ____| / __ \     /\    | |     `,
	`| |__   | |  | |   /  \   | |     `,
	`|  __|  | |  | |  / /\ \  | |     `,
	`| |     | |__| | / ____ \ | |____ `,
	`|_|      \____/ /_/    \_\|______|`,
}

func renderFoalBanner() string {
	sideText := map[int]string{
		2: bannerURL,
		3: bannerTagline,
	}
	var builder strings.Builder
	for index, row := range foalBannerArt {
		builder.WriteString(row)
		if text, ok := sideText[index]; ok {
			builder.WriteString("   ")
			builder.WriteString(text)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderMainMenu(selected int, notice string) string {
	var builder strings.Builder
	builder.WriteString(renderFoalBanner())
	builder.WriteString("\nFoal main menu\n")
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
