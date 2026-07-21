package cli

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/status"
	"github.com/CoreyLyn/Foal/internal/uninstall"
)

var statusCapture = status.Capture

type viewerLoadedMsg struct {
	command string
	body    string
}

// loadViewerCmd builds the read-only report for a command view off the UI
// loop. Every body comes from existing command paths and read models; the
// viewer never owns uninstall, deletion, or path-safety logic.
func loadViewerCmd(command string) tea.Cmd {
	return func() tea.Msg {
		return viewerLoadedMsg{command: command, body: renderViewerBody(command)}
	}
}

// loadAnalyzeViewerCmd loads the analyze view for a specific path.
func loadAnalyzeViewerCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return viewerLoadedMsg{command: "analyze", body: renderAnalyzeBody(path)}
	}
}

func renderViewerBody(command string) string {
	switch command {
	case "uninstall":
		return uninstall.RenderPreviewReport(uninstall.WithReviewSections(reviewUninstall()))
	case "status":
		return renderStatusReport(statusCapture())
	case "history":
		query, err := newHistoryQuery()
		if err != nil {
			return fmt.Sprintf("History is unavailable: %v\n", err)
		}
		return renderHistoryReport(query.Recent(context.Background()))
	case "analyze":
		return renderAnalyzeBody("")
	}
	return ""
}

func renderAnalyzeBody(path string) string {
	result, reason, ok := analyze.Run(context.Background(), path, analyze.Options{})
	if !ok {
		var b strings.Builder
		b.WriteString("Foal analyze\n")
		b.WriteString(fmt.Sprintf("Root: %s\n", path))
		b.WriteString("Status: invalid root (read-only view)\n")
		b.WriteString(fmt.Sprintf("Reason: %s\n", reason.Message))
		b.WriteString("\nThis view is read-only; no cleanup or deletion actions are available.\n")
		return b.String()
	}
	report := analyze.RenderHumanReport(result)
	// Add the read-only safety reminder after the shared report.
	return report + "\nThis view is read-only; no cleanup or deletion actions are available.\n"
}

type viewerSpec struct {
	title    string
	subtitle string
}

var viewerSpecs = map[string]viewerSpec{
	"uninstall": {
		title:    "Uninstall preview TUI",
		subtitle: "Preview-only review; nothing is executed or deleted",
	},
	"status": {
		title:    "Status TUI",
		subtitle: "Read-only system and Foal state snapshot",
	},
	"history": {
		title:    "History TUI",
		subtitle: "Read-only review of prior Foal operations",
	},
	"analyze": {
		title:    "Analyze TUI",
		subtitle: "Read-only directory insight; no cleanup or deletion",
	},
}

type viewerModel struct {
	command    string
	loading    bool
	notice     string
	vp         viewport.Model
	width      int
	height     int
	// Analyze-specific state: path editing
	editingPath bool
	analyzePath string
	cursorPos   int
}

func newViewerModel(command string, width, height int) viewerModel {
	model := viewerModel{
		command: command,
		loading: true,
		vp:      viewport.New(),
	}
	if command == "analyze" {
		model.analyzePath = "" // Default to current working directory
	}
	model.setSize(width, height)
	return model
}

func (m *viewerModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.vp.SetWidth(width)
	bodyHeight := height - m.chromeLineCount()
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.vp.SetHeight(bodyHeight)
}

func (m *viewerModel) chromeLineCount() int {
	return strings.Count(m.headerContent(), "\n") + strings.Count(viewerFooter, "\n")
}

func (m *viewerModel) applyLoaded(msg viewerLoadedMsg) {
	if msg.command != m.command {
		return
	}
	m.loading = false
	m.vp.SetContent(msg.body)
	m.vp.GotoTop()
	m.setSize(m.width, m.height)
}

// beginReload puts the view back into the loading state; the caller is
// responsible for issuing loadViewerCmd.
func (m *viewerModel) beginReload() {
	m.loading = true
	m.notice = "Reloading..."
	m.vp.GotoTop()
	m.setSize(m.width, m.height)
}

func (m *viewerModel) handleKey(key string) tea.Cmd {
	if m.command == "analyze" {
		if m.editingPath {
			return m.handlePathEditKey(key)
		}
		switch key {
		case "e":
			// Enter path edit mode
			m.editingPath = true
			m.cursorPos = len(m.analyzePath)
			m.notice = "Editing path: type to edit, enter to rescan, esc to cancel"
			return nil
		case "r":
			// Reload with current path
			m.beginReload()
			return loadAnalyzeViewerCmd(m.analyzePath)
		}
	}

	switch key {
	case "j", "down":
		m.vp.ScrollDown(1)
	case "k", "up":
		m.vp.ScrollUp(1)
	case "r":
		// Reload for non-analyze commands
		m.beginReload()
		return loadViewerCmd(m.command)
	default:
		if m.command == "analyze" {
			m.notice = "Unknown key. Use j/k, r, e (edit path), esc/b, or q."
		} else {
			m.notice = "Unknown key. Use j/k, r, esc/b, or q."
		}
	}
	m.setSize(m.width, m.height)
	return nil
}

// handlePathEditKey handles keyboard input when in path edit mode for Analyze.
func (m *viewerModel) handlePathEditKey(key string) tea.Cmd {
	switch key {
	case "enter":
		// Exit edit mode and rescan with the new path
		m.editingPath = false
		m.loading = true
		m.notice = "Rescanning..."
		m.vp.GotoTop()
		return loadAnalyzeViewerCmd(m.analyzePath)
	case "esc", "escape":
		// Exit edit mode without changing path
		m.editingPath = false
		m.notice = ""
	case "backspace":
		if m.cursorPos > 0 {
			m.analyzePath = m.analyzePath[:m.cursorPos-1] + m.analyzePath[m.cursorPos:]
			m.cursorPos--
		}
	case "left":
		if m.cursorPos > 0 {
			m.cursorPos--
		}
	case "right":
		if m.cursorPos < len(m.analyzePath) {
			m.cursorPos++
		}
	case "home":
		m.cursorPos = 0
	case "end":
		m.cursorPos = len(m.analyzePath)
	default:
		// Handle printable characters (single rune, no modifiers)
		if len(key) == 1 {
			// Single printable character
			m.analyzePath = m.analyzePath[:m.cursorPos] + key + m.analyzePath[m.cursorPos:]
			m.cursorPos++
		} else if key == "space" {
			// Special case for space
			m.analyzePath = m.analyzePath[:m.cursorPos] + " " + m.analyzePath[m.cursorPos:]
			m.cursorPos++
		}
	}
	return nil
}

const viewerFooter = "\nHints: j/k scroll | r refresh | esc/b back | q quit\n" +
	"This view is read-only; no actions are executed.\n"

const analyzeViewerFooter = "\nHints: j/k scroll | r refresh | e edit path | esc/b back | q quit\n" +
	"This view is read-only; no cleanup or deletion actions are available.\n"

func (m viewerModel) content() string {
	footer := viewerFooter
	if m.command == "analyze" {
		footer = analyzeViewerFooter
	}
	if m.loading {
		return m.headerContent() + "\nLoading " + m.command + " view...\n" + footer
	}
	return m.headerContent() + m.vp.View() + "\n" + footer
}

func (m viewerModel) headerContent() string {
	spec := viewerSpecs[m.command]
	var builder strings.Builder
	builder.WriteString("+--------------------------------------------------+\n")
	builder.WriteString(fmt.Sprintf("| %-48s |\n", spec.title))
	builder.WriteString(fmt.Sprintf("| %-48s |\n", spec.subtitle))
	builder.WriteString("+--------------------------------------------------+\n\n")

	if m.command == "analyze" {
		if m.editingPath {
			// Show path being edited with cursor position
			displayPath := m.analyzePath
			if displayPath == "" {
				displayPath = "(default: current directory)"
			}
			// Insert cursor indicator
			if m.cursorPos <= len(displayPath) {
				displayPath = displayPath[:m.cursorPos] + "|" + displayPath[m.cursorPos:]
			}
			builder.WriteString(fmt.Sprintf("Path: %s\n", displayPath))
		} else {
			displayPath := m.analyzePath
			if displayPath == "" {
				displayPath = "(default: current directory)"
			}
			builder.WriteString(fmt.Sprintf("Path: %s\n", displayPath))
		}
	}

	builder.WriteString(fmt.Sprintf("Scroll: %d%%\n", int(m.vp.ScrollPercent()*100)))
	if m.notice != "" {
		builder.WriteString(m.notice)
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderStatusReport(snapshot status.Snapshot) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Status: %s\n", snapshot.Status))
	builder.WriteString(fmt.Sprintf("OS: %s/%s\n", snapshot.OS.GOOS, snapshot.OS.GOARCH))
	builder.WriteString(fmt.Sprintf("Foal: %s (command: %s, executable: %s, version: %s)\n",
		snapshot.Foal.Name, snapshot.Foal.Command, snapshot.Foal.Executable, snapshot.Foal.Version))
	builder.WriteString(fmt.Sprintf("\nDisk\n  Path: %s\n", snapshot.Disk.Path))
	if snapshot.Disk.TotalBytes > 0 {
		builder.WriteString(fmt.Sprintf("  Total: %d bytes\n", snapshot.Disk.TotalBytes))
		builder.WriteString(fmt.Sprintf("  Free: %d bytes\n", snapshot.Disk.FreeBytes))
		builder.WriteString(fmt.Sprintf("  Available: %d bytes\n", snapshot.Disk.AvailableBytes))
	}
	writeStatusIssues(&builder, "Skipped", snapshot.Skipped)
	writeStatusIssues(&builder, "Errors", snapshot.Errors)
	builder.WriteString(fmt.Sprintf("\nElapsed: %d ms\n", snapshot.ElapsedMS))
	return builder.String()
}

func writeStatusIssues(builder *strings.Builder, label string, issues []status.StatusIssue) {
	builder.WriteString(fmt.Sprintf("\n%s (%d)\n", label, len(issues)))
	if len(issues) == 0 {
		builder.WriteString("  None reported.\n")
		return
	}
	for _, issue := range issues {
		builder.WriteString(fmt.Sprintf("  %s: %s (recoverable: %t)\n", issue.Code, issue.Message, issue.Recoverable))
	}
}

func renderHistoryReport(result history.QueryResult) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Status: %s\n", result.Status))
	builder.WriteString(fmt.Sprintf("\nSessions (%d)\n", len(result.Sessions)))
	if len(result.Sessions) == 0 {
		builder.WriteString("  No recorded sessions.\n")
	}
	for _, session := range result.Sessions {
		builder.WriteString(fmt.Sprintf("  %s\n", session.ID))
		builder.WriteString(fmt.Sprintf("    mode: %s | candidates: %d | deleted: %d | skipped: %d | errors: %d\n",
			session.Mode,
			session.Aggregate.CandidateCount,
			session.Aggregate.DeletedCount,
			session.Aggregate.SkippedCount,
			session.Aggregate.ErrorCount))
		builder.WriteString(fmt.Sprintf("    started: %s | ended: %s\n",
			session.StartedAt.Format("2006-01-02 15:04:05 MST"),
			session.EndedAt.Format("2006-01-02 15:04:05 MST")))
	}
	builder.WriteString(fmt.Sprintf("\nErrors (%d)\n", len(result.Errors)))
	if len(result.Errors) == 0 {
		builder.WriteString("  None reported.\n")
	}
	for _, issue := range result.Errors {
		builder.WriteString(fmt.Sprintf("  %s: %s", issue.Code, issue.Message))
		if issue.Path != "" {
			builder.WriteString(fmt.Sprintf(" (%s)", issue.Path))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}
