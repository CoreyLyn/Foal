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
	screenUninstallPreview
	screenViewer
	screenAnalyzeDrive
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
		description: "Measure cleanup categories, form an exact selection, and confirm Recycle Bin cleanup.",
		selection:   "",
	},
	{
		title:       "Uninstall",
		command:     "uninstall",
		description: "Multi-select installed apps, confirm the plan, and run shared Uninstall execute.",
		selection:   "",
	},
	{
		title:       "Analyze",
		command:     "analyze",
		description: "Read-only local drive browser; measure direct children on demand.",
		selection:   "",
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
	screen    tuiScreen
	selected  int
	notice    string
	clean     eagerCleanModel
	uninstall uninstallModel
	viewer    viewerModel
	analyze   analyzeDriveModel
	width     int
	height    int
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
		m.uninstall.setSize(msg.Width, msg.Height)
		m.viewer.setSize(msg.Width, msg.Height)
		m.analyze.setSize(msg.Width, msg.Height)
		return m, nil
	case eagerPreviewStartedMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		return m, m.clean.applyStarted(msg)
	case eagerCategoryObservationMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		m.clean.applyObservation(msg)
		return m, m.clean.continuePreviewWait(msg.generation)
	case eagerPreviewFinishedMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		m.clean.applyFinished(msg)
		return m, nil
	case eagerPreviewUnavailableMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		m.clean.applyUnavailable(msg)
		return m, nil
	case eagerPreviewTickMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		return m, m.clean.applyTick(msg)
	case eagerServicingAnalyzedMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		m.clean.applyServicingAnalyzed(msg)
		return m, nil
	case eagerExactExecutionStartedMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		return m, m.clean.applyExactExecutionStarted(msg)
	case eagerExactExecutionProgressMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		return m, m.clean.applyExactExecutionProgress(msg)
	case eagerExactExecutedMsg:
		if m.screen != screenCleanPreview {
			return m, nil
		}
		m.clean.applyExactExecuted(msg)
		return m, nil
	case uninstallPreviewLoadedMsg:
		if m.screen != screenUninstallPreview {
			return m, nil
		}
		m.uninstall.applyPreviewLoaded(msg)
		return m, nil
	case uninstallExecutedMsg:
		if m.screen != screenUninstallPreview {
			return m, nil
		}
		m.uninstall.applyExecuted(msg)
		return m, nil
	case viewerLoadedMsg:
		m.viewer.applyLoaded(msg)
		return m, nil
	case analyzeVolumesLoadedMsg:
		if m.screen != screenAnalyzeDrive {
			return m, nil
		}
		m.analyze.applyLoaded(msg)
		return m, nil
	case analyzeBrowseStartedMsg:
		if m.screen != screenAnalyzeDrive {
			return m, nil
		}
		return m, m.analyze.applyBrowseStarted(msg)
	case analyzeBrowseObservationMsg:
		if m.screen != screenAnalyzeDrive {
			return m, nil
		}
		return m, m.analyze.applyBrowseObservation(msg)
	case analyzeBrowseLoadedMsg:
		if m.screen != screenAnalyzeDrive {
			return m, nil
		}
		return m, m.analyze.applyBrowseLoaded(msg)
	case tea.KeyPressMsg:
		switch m.screen {
		case screenCleanPreview:
			return m.updateCleanPreviewKey(msg)
		case screenUninstallPreview:
			return m.updateUninstallPreviewKey(msg)
		case screenViewer:
			return m.updateViewerKey(msg)
		case screenAnalyzeDrive:
			return m.updateAnalyzeDriveKey(msg)
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
			// Primary Clean surface is category-first eager preview only.
			m.screen = screenCleanPreview
			m.clean = newEagerCleanModel(m.width, m.height)
			return m, m.clean.start()
		case "uninstall":
			// Uninstall TUI: multi-select + confirmation through shared
			// Execute. The TUI is an adapter only; it owns no uninstaller,
			// path safety, Protection, elevation, or deletion logic.
			m.screen = screenUninstallPreview
			m.uninstall = newUninstallModel(m.width, m.height)
			return m, m.uninstall.start()
		case "analyze":
			// Dedicated Analyze browser: drive entry + on-demand browse (#345/#346).
			m.screen = screenAnalyzeDrive
			m.analyze = newAnalyzeDriveModel(m.width, m.height)
			return m, m.analyze.start()
		case "status", "history":
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
	nav, cmd := m.clean.handleKey(msg.String())
	switch nav {
	case eagerPreviewNavMenu:
		m.screen = screenMenu
		m.notice = ""
		return m, nil
	case eagerPreviewNavQuit:
		return m, tea.Quit
	case eagerPreviewNavInterrupt:
		return m, tea.Interrupt
	}
	return m, cmd
}

func (m rootModel) updateUninstallPreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	nav, cmd := m.uninstall.handleKey(msg.String())
	switch nav {
	case uninstallNavMenu:
		m.screen = screenMenu
		m.notice = ""
		return m, nil
	case uninstallNavQuit:
		return m, tea.Quit
	case uninstallNavInterrupt:
		return m, tea.Interrupt
	}
	return m, cmd
}

func (m rootModel) updateViewerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c":
		return m, tea.Interrupt
	case "q":
		// Path edit treats q as a character; otherwise quit the TUI.
		if m.viewer.editingPath {
			cmd := m.viewer.handleKey(key)
			return m, cmd
		}
		return m, tea.Quit
	case "esc", "escape", "b":
		// Path edit: esc cancels the edit; b is a path character.
		// Otherwise return to the main menu (same as Clean/Uninstall).
		if m.viewer.editingPath {
			cmd := m.viewer.handleKey(key)
			return m, cmd
		}
		m.screen = screenMenu
		m.notice = ""
		return m, nil
	}
	// All other keys (including 'r' reload, scrolling, etc.) go to the viewer
	cmd := m.viewer.handleKey(key)
	return m, cmd
}

func (m rootModel) updateAnalyzeDriveKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	nav, cmd := m.analyze.handleKey(msg.String())
	switch nav {
	case analyzeDriveNavMenu:
		m.screen = screenMenu
		m.notice = ""
		return m, nil
	case analyzeDriveNavQuit:
		return m, tea.Quit
	case analyzeDriveNavInterrupt:
		return m, tea.Interrupt
	}
	return m, cmd
}

// content is the plain text frame. It is separate from View so the nil-input
// entry path and unit tests can assert on the rendered text directly.
func (m rootModel) content() string {
	switch m.screen {
	case screenCleanPreview:
		return m.clean.content()
	case screenUninstallPreview:
		return m.uninstall.content()
	case screenViewer:
		return m.viewer.content()
	case screenAnalyzeDrive:
		return m.analyze.content()
	}
	return renderMainMenu(m.selected, m.notice)
}

func (m rootModel) View() tea.View {
	// Clean carries trusted magnitude byte metadata on annotated lines so
	// tier classification prefers int64 over reverse-parsing display tokens.
	// Analyze browse uses pink/cyan/yellow/gray state styling (never red).
	// Other screens keep plain post-process styling.
	var framed string
	switch m.screen {
	case screenCleanPreview:
		framed = stylizeStyleLines(m.clean.contentStyleLines())
	case screenAnalyzeDrive:
		framed = stylizeAnalyzeBrowseFrame(m.content())
	default:
		framed = stylizeFrame(m.content())
	}
	view := tea.NewView(framed)
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
		// Fixed-width title so description first letters stay column-aligned;
		// command slug is internal only and is not shown in the menu.
		builder.WriteString(fmt.Sprintf("%s %-12s  %s\n", prefix, item.title, item.description))
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
