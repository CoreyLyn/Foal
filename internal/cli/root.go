package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/CoreyLyn/Foal/internal/analyze"
	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/status"
	"github.com/CoreyLyn/Foal/internal/uninstall"
)

const (
	exitOK    = 0
	exitUsage = 2
)

type commandSpec struct {
	name        string
	description string
}

var commands = []commandSpec{
	{name: "analyze", description: "Inspect disk usage and cleanup opportunities without changing files."},
	{name: "clean", description: "Preview or execute conservative cleanup candidates through the Recycle Bin."},
	{name: "status", description: "Report a read-only system and Foal state snapshot."},
	{name: "history", description: "Show previous Foal operation records."},
	{name: "uninstall", description: "Preview application uninstall evidence without executing uninstallers."},
}

var (
	dryRunClean        = clean.DryRun
	executeClean       = clean.Execute
	newHistoryRecorder = func() (history.Recorder, error) {
		recorder, err := history.NewDefaultFileRecorder()
		if err != nil {
			return nil, err
		}
		return recorder, nil
	}
	newHistoryQuery = func() (history.FileQuery, error) {
		return history.NewDefaultFileQuery()
	}
	newHistoryDir   = history.DefaultDir
	reviewUninstall = uninstall.Review
)

type Invocation struct {
	ExecutableName            string
	Args                      []string
	InteractiveTerminal       bool
	OutputInteractiveTerminal bool
	Input                     io.Reader
}

// Run executes the Foal command line with output streams supplied by the caller.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunInvocation(Invocation{ExecutableName: "foal", Args: args}, stdout, stderr)
}

func RunInvocation(invocation Invocation, stdout, stderr io.Writer) int {
	args := append([]string(nil), invocation.Args...)
	opts, positional, err := parseOptions(args)
	if err != nil {
		return writeError(stderr, opts.json, "root", args, jsonError{
			Code:        "invalid_option",
			Message:     err.Error(),
			Recoverable: true,
			Command:     "root",
			Args:        args,
		})
	}

	if len(positional) == 0 {
		if canLaunchInteractiveEntry(invocation, opts) {
			restoreInput := enableRawInput(invocation.Input)
			defer restoreInput()
			restoreOutput := enableVirtualTerminalOutput(stdout)
			defer restoreOutput()
			runMainMenuScreenTo(invocation.Input, stdout)
			return exitOK
		}
		if opts.json {
			return writeJSON(stdout, envelope{
				Command: "root",
				Result: commandResult{
					Status:  "ok",
					Message: helpText(),
				},
			})
		}
		_, _ = fmt.Fprint(stdout, helpText())
		return exitOK
	}

	if positional[0] == "help" {
		if opts.json {
			return writeJSON(stdout, envelope{
				Command: "root",
				Result: commandResult{
					Status:  "ok",
					Message: helpText(),
				},
			})
		}
		_, _ = fmt.Fprint(stdout, helpText())
		return exitOK
	}

	command := positional[0]
	if command == "wole" {
		return writeError(stderr, opts.json, command, args, jsonError{
			Code:        "unknown_command",
			Message:     "unknown command: wole",
			Recoverable: true,
			Command:     command,
			Args:        args,
		})
	}

	if !isKnownCommand(command) {
		return writeError(stderr, opts.json, command, args, jsonError{
			Code:        "unknown_command",
			Message:     "unknown command: " + command,
			Recoverable: true,
			Command:     command,
			Args:        args,
		})
	}

	if command == "status" {
		result := status.Capture()
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		_, _ = fmt.Fprintf(stdout, "Foal status\nOS: %s/%s\nDisk: %s\n", result.OS.GOOS, result.OS.GOARCH, result.Disk.Path)
		return exitOK
	}

	if command == "analyze" {
		root := "."
		if len(positional) > 1 {
			root = positional[1]
		}
		result := analyze.Run(root)
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		_, _ = fmt.Fprintf(stdout, "Foal analyze\nRoot: %s\nFiles: %d\nDirectories: %d\nSkipped: %d\n",
			result.Root, result.Totals.FileCount, result.Totals.DirectoryCount, len(result.Skipped))
		return exitOK
	}

	if command == "uninstall" {
		result := uninstall.WithReviewSections(reviewUninstall())
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		_, _ = fmt.Fprint(stdout, uninstall.RenderPreviewReport(result))
		return exitOK
	}

	if command == "history" {
		query, err := newHistoryQuery()
		if err != nil {
			return writeError(stderr, opts.json, command, args, jsonError{
				Code:        "history_read_failed",
				Message:     err.Error(),
				Recoverable: true,
				Command:     command,
				Args:        args,
			})
		}
		result := query.Recent(context.Background())
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		_, _ = fmt.Fprintf(stdout, "Foal history\nSessions: %d\nStatus: %s\n", len(result.Sessions), result.Status)
		return exitOK
	}

	if command == "clean" {
		invocation, err := validateCleanArgs(positional[1:])
		if err != nil {
			return writeError(stderr, opts.json, command, args, jsonError{
				Code:        "invalid_clean_invocation",
				Message:     err.Error(),
				Recoverable: true,
				Command:     command,
				Args:        args,
			})
		}

		recorder, _ := newHistoryRecorder()
		detailedListDir := ""
		if invocation.dryRun && !opts.json {
			detailedListDir, _ = newHistoryDir()
		}
		cleanOptions := clean.Options{
			HistoryRecorder: recorder,
			DetailedListDir: detailedListDir,
			CommandParameters: history.CommandParameters{
				Command: "clean",
				Args:    append([]string(nil), args...),
			},
		}

		var result clean.Result
		if invocation.execute {
			result = executeClean(context.Background(), cleanOptions)
		} else {
			result = dryRunClean(context.Background(), cleanOptions)
		}
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		if invocation.execute {
			_, _ = fmt.Fprintf(stdout, "Foal clean\nExecution complete. Deleted: %d, skipped: %d, action: Recycle Bin.\n",
				result.Totals.DeletedCount, result.Totals.SkippedCount)
			return exitOK
		}
		_, _ = fmt.Fprint(stdout, clean.RenderPreviewReport(clean.NewPreviewReadModel(result)))
		return exitOK
	}

	result := commandResult{
		Status:  "preview",
		Message: command + " is routed but not implemented yet.",
	}
	if opts.json {
		return writeJSON(stdout, envelope{Command: command, Result: result})
	}

	_, _ = fmt.Fprintf(stdout, "Foal %s\n%s\n", command, result.Message)
	return exitOK
}

type options struct {
	json bool
}

func parseOptions(args []string) (options, []string, error) {
	var opts options
	var positional []string
	commandSeen := false

	for _, arg := range args {
		if commandSeen {
			if arg == "--json" {
				opts.json = true
				continue
			}
			positional = append(positional, arg)
			continue
		}

		switch arg {
		case "--json":
			opts.json = true
		case "-h", "--help":
			positional = append(positional, "help")
			commandSeen = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, positional, fmt.Errorf("unknown option: %s", arg)
			}
			positional = append(positional, arg)
			commandSeen = true
		}
	}

	return opts, positional, nil
}

func canLaunchInteractiveEntry(invocation Invocation, opts options) bool {
	// Future TUI entry points should use this same guard: no-argument
	// interactivity is safe only when both sides are terminals and the caller
	// did not request JSON. This keeps scripts, pipes, and automation on the
	// deterministic command/help path instead of waiting for keyboard input.
	return invocation.InteractiveTerminal && invocation.OutputInteractiveTerminal && !opts.json
}

func isKnownCommand(name string) bool {
	for _, command := range commands {
		if command.name == name {
			return true
		}
	}
	return false
}

type cleanInvocation struct {
	dryRun  bool
	execute bool
}

func validateCleanArgs(args []string) (cleanInvocation, error) {
	var invocation cleanInvocation
	dryRun := false
	execute := false
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--execute":
			execute = true
		default:
			return invocation, fmt.Errorf("unknown clean option: %s", arg)
		}
	}
	invocation = cleanInvocation{dryRun: dryRun, execute: execute}
	if dryRun && execute {
		return invocation, fmt.Errorf("clean accepts either --dry-run or --execute, not both")
	}
	if !dryRun && !execute {
		return invocation, fmt.Errorf("clean requires explicit --dry-run preview or --execute confirmation")
	}
	return invocation, nil
}

func helpText() string {
	var builder strings.Builder
	builder.WriteString("Foal - safe, preview-first cleanup for Windows\n\n")
	builder.WriteString("Usage:\n")
	builder.WriteString("  foal [--json] <command>\n")
	builder.WriteString("  foal --help\n\n")
	builder.WriteString("Commands:\n")
	for _, command := range commands {
		builder.WriteString(fmt.Sprintf("  %-10s %s\n", command.name, command.description))
	}
	builder.WriteString("\nExamples:\n")
	builder.WriteString("  foal status --json\n")
	builder.WriteString("  foal clean --dry-run\n")
	builder.WriteString("  foal clean --execute\n")
	builder.WriteString("  foal.exe analyze\n")
	return builder.String()
}

func mainMenuEntryText() string {
	return renderMainMenu(0, "")
}

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

func runMainMenu(input io.Reader) string {
	var builder strings.Builder
	runMainMenuTo(input, &builder)
	return builder.String()
}

func runMainMenuTo(input io.Reader, output io.Writer) {
	runMainMenuWithRenderer(input, output, appendMenuRenderer{})
}

func runMainMenuScreenTo(input io.Reader, output io.Writer) {
	renderer := &screenMenuRenderer{}
	renderer.begin(output)
	defer renderer.end(output)
	runMainMenuWithRenderer(input, output, renderer)
}

func runMainMenuWithRenderer(input io.Reader, output io.Writer, renderer menuRenderer) {
	if input == nil {
		renderer.frame(output, mainMenuEntryText())
		return
	}

	selected := 0
	cleanView := cleanPreviewTUIState{}
	renderer.frame(output, renderMainMenu(selected, ""))
	reader := bufio.NewReader(input)
	for {
		key, ok := readMenuKey(reader)
		if !ok {
			return
		}
		if cleanView.open {
			switch key {
			case "q", "quit", "esc":
				renderer.message(output, "\nFoal main menu closed.\n")
				return
			case "b", "back":
				cleanView.open = false
				renderer.frame(output, renderMainMenu(selected, ""))
			case "j", "down":
				cleanView.scroll++
				renderer.frame(output, renderCleanPreviewTUI(cleanView))
			case "k", "up":
				if cleanView.scroll > 0 {
					cleanView.scroll--
				}
				renderer.frame(output, renderCleanPreviewTUI(cleanView))
			case "f", "filter":
				cleanView.filter = nextCleanPreviewFilter(cleanView.filter)
				cleanView.scroll = 0
				renderer.frame(output, renderCleanPreviewTUI(cleanView))
			case "e", "expand":
				cleanView.expanded = !cleanView.expanded
				renderer.frame(output, renderCleanPreviewTUI(cleanView))
			case "c", "copy":
				cleanView.notice = "Copy paths from the detailed list or visible rows for manual review."
				renderer.frame(output, renderCleanPreviewTUI(cleanView))
			default:
				cleanView.notice = "Unknown key. Use j/k, f, e, c, b, or q."
				renderer.frame(output, renderCleanPreviewTUI(cleanView))
			}
			continue
		}

		switch key {
		case "q", "quit", "esc":
			renderer.message(output, "\nFoal main menu closed.\n")
			return
		case "j", "down":
			selected = (selected + 1) % len(mainMenuItems)
			renderer.frame(output, renderMainMenu(selected, ""))
		case "k", "up":
			selected = (selected + len(mainMenuItems) - 1) % len(mainMenuItems)
			renderer.frame(output, renderMainMenu(selected, ""))
		case "", "enter":
			if mainMenuItems[selected].command == "clean" {
				cleanView = newCleanPreviewTUIState()
				renderer.frame(output, renderCleanPreviewTUI(cleanView))
			} else {
				renderer.frame(output, mainMenuItems[selected].selection+"\n"+renderMainMenu(selected, ""))
			}
		default:
			renderer.frame(output, renderMainMenu(selected, "Unknown key. Use j/k, up/down, enter, or q."))
		}
	}
}

type menuRenderer interface {
	frame(output io.Writer, text string)
	message(output io.Writer, text string)
}

type appendMenuRenderer struct{}

func (appendMenuRenderer) frame(output io.Writer, text string) {
	_, _ = io.WriteString(output, text)
}

func (appendMenuRenderer) message(output io.Writer, text string) {
	_, _ = io.WriteString(output, text)
}

type screenMenuRenderer struct {
	ended bool
}

func (renderer *screenMenuRenderer) begin(output io.Writer) {
	_, _ = io.WriteString(output, "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
}

func (renderer *screenMenuRenderer) end(output io.Writer) {
	if renderer.ended {
		return
	}
	renderer.ended = true
	_, _ = io.WriteString(output, "\x1b[?25h\x1b[?1049l")
}

func (renderer *screenMenuRenderer) frame(output io.Writer, text string) {
	_, _ = io.WriteString(output, "\x1b[2J\x1b[H")
	_, _ = io.WriteString(output, text)
}

func (renderer *screenMenuRenderer) message(output io.Writer, text string) {
	renderer.end(output)
	_, _ = io.WriteString(output, text)
}

func readMenuKey(reader *bufio.Reader) (string, bool) {
	b, err := reader.ReadByte()
	if err != nil {
		return "", false
	}

	switch b {
	case '\r', '\n':
		return "enter", true
	case 0x1b:
		return readEscapeMenuKey(reader), true
	case 0x00, 0xe0:
		return readWindowsExtendedMenuKey(reader), true
	}

	if isMenuCommandByte(b) && reader.Buffered() == 0 {
		return strings.ToLower(string(b)), true
	}
	if isMenuCommandByte(b) && nextBufferedByteIsMenuCommand(reader) {
		return strings.ToLower(string(b)), true
	}

	if isMenuCommandByte(b) || isMenuWordByte(b) {
		var builder strings.Builder
		builder.WriteByte(b)
		for reader.Buffered() > 0 {
			next, err := reader.ReadByte()
			if err != nil {
				break
			}
			if next == '\r' || next == '\n' {
				break
			}
			builder.WriteByte(next)
		}
		return normalizeMenuKey(builder.String()), true
	}

	return strings.ToLower(strings.TrimSpace(string(b))), true
}

func readEscapeMenuKey(reader *bufio.Reader) string {
	if reader.Buffered() == 0 {
		return "esc"
	}
	next, err := reader.ReadByte()
	if err != nil {
		return "esc"
	}
	if next == '[' || next == 'O' {
		if reader.Buffered() == 0 {
			return "esc"
		}
		key, err := reader.ReadByte()
		if err != nil {
			return "esc"
		}
		switch key {
		case 'A':
			return "up"
		case 'B':
			return "down"
		}
	}
	return "esc"
}

func readWindowsExtendedMenuKey(reader *bufio.Reader) string {
	if reader.Buffered() == 0 {
		return ""
	}
	key, err := reader.ReadByte()
	if err != nil {
		return ""
	}
	switch key {
	case 'H':
		return "up"
	case 'P':
		return "down"
	}
	return ""
}

func normalizeMenuKey(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func isMenuCommandByte(b byte) bool {
	switch b {
	case 'b', 'B', 'c', 'C', 'e', 'E', 'f', 'F', 'j', 'J', 'k', 'K', 'q', 'Q':
		return true
	}
	return false
}

func nextBufferedByteIsMenuCommand(reader *bufio.Reader) bool {
	next, err := reader.Peek(1)
	if err != nil || len(next) == 0 {
		return false
	}
	return isMenuCommandByte(next[0]) || next[0] == 0x1b || next[0] == 0x00 || next[0] == 0xe0
}

func isMenuWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func enableRawInput(input io.Reader) func() {
	file, ok := input.(*os.File)
	if !ok {
		return func() {}
	}
	return enableRawInputFile(file)
}

func enableVirtualTerminalOutput(output io.Writer) func() {
	file, ok := output.(*os.File)
	if !ok {
		return func() {}
	}
	return enableVirtualTerminalOutputFile(file)
}

type cleanPreviewFilter string

const (
	cleanPreviewFilterAll        cleanPreviewFilter = "all"
	cleanPreviewFilterCandidates cleanPreviewFilter = "default candidates"
	cleanPreviewFilterSkipped    cleanPreviewFilter = "skipped"
	cleanPreviewFilterReview     cleanPreviewFilter = "review"
	cleanPreviewFilterErrors     cleanPreviewFilter = "errors"
)

type cleanPreviewTUIState struct {
	open     bool
	model    clean.PreviewReadModel
	filter   cleanPreviewFilter
	scroll   int
	expanded bool
	notice   string
}

func newCleanPreviewTUIState() cleanPreviewTUIState {
	recorder, _ := newHistoryRecorder()
	detailedListDir, _ := newHistoryDir()
	result := dryRunClean(context.Background(), clean.Options{
		HistoryRecorder: recorder,
		DetailedListDir: detailedListDir,
		CommandParameters: history.CommandParameters{
			Command: "clean",
			Args:    []string{"clean", "--dry-run"},
		},
	})
	return cleanPreviewTUIState{
		open:   true,
		model:  clean.NewPreviewReadModel(result),
		filter: cleanPreviewFilterAll,
		notice: "Copy paths from the detailed list or visible rows for manual review.",
	}
}

func nextCleanPreviewFilter(filter cleanPreviewFilter) cleanPreviewFilter {
	switch filter {
	case cleanPreviewFilterAll:
		return cleanPreviewFilterCandidates
	case cleanPreviewFilterCandidates:
		return cleanPreviewFilterSkipped
	case cleanPreviewFilterSkipped:
		return cleanPreviewFilterReview
	case cleanPreviewFilterReview:
		return cleanPreviewFilterErrors
	default:
		return cleanPreviewFilterAll
	}
}

func renderCleanPreviewTUI(state cleanPreviewTUIState) string {
	model := state.model
	var builder strings.Builder
	builder.WriteString("+--------------------------------------------------+\n")
	builder.WriteString("| Clean preview TUI                                |\n")
	builder.WriteString("| Read-only review over foal clean --dry-run       |\n")
	builder.WriteString("+--------------------------------------------------+\n\n")
	builder.WriteString(fmt.Sprintf("Potential space: %s\n", cleanFormatBytes(model.PotentialSpaceBytes)))
	builder.WriteString(fmt.Sprintf("Candidates: %d, skipped: %d, errors: %d\n", model.CandidateCount, model.SkippedCount, len(model.Errors)))
	builder.WriteString(fmt.Sprintf("Filter: %s | Scroll: %d | Expanded: %t\n", state.filter, state.scroll, state.expanded))
	if model.DetailedListPath != "" {
		builder.WriteString(fmt.Sprintf("Detailed candidate list: %s\n", model.DetailedListPath))
	}
	if state.notice != "" {
		builder.WriteString(state.notice)
		builder.WriteString("\n")
	}

	if len(model.Notices) > 0 && cleanPreviewFilterAllows(state.filter, cleanPreviewFilterAll) {
		builder.WriteString("\nNotices\n")
		for _, notice := range model.Notices {
			builder.WriteString(fmt.Sprintf("  %s\n", notice.Message))
		}
	}

	if cleanPreviewFilterAllows(state.filter, cleanPreviewFilterAll) {
		builder.WriteString("\nProtection rules\n")
		if len(model.ProtectionRules) == 0 {
			builder.WriteString("  No default-enabled protection rules were reported.\n")
		} else {
			for _, rule := range visibleCleanPreviewRows(state.scroll, model.ProtectionRules) {
				builder.WriteString(fmt.Sprintf("  %s: %s\n", rule.ID, rule.Description))
			}
		}
	}

	if cleanPreviewFilterAllows(state.filter, cleanPreviewFilterCandidates) {
		builder.WriteString(fmt.Sprintf("\nDefault candidates (%d)\n", len(model.Candidates)))
		if len(model.Candidates) == 0 {
			builder.WriteString("  No default candidates found.\n")
		} else {
			for _, candidate := range visibleCleanPreviewRows(state.scroll, model.Candidates) {
				builder.WriteString(fmt.Sprintf("  %s (%s, rule: %s, preview action metadata: Recycle Bin)\n",
					candidate.Path, cleanFormatBytes(candidate.Bytes), candidate.Rule))
				if state.expanded && candidate.PlannedAction != "" {
					builder.WriteString(fmt.Sprintf("    planned action metadata: %s\n", candidate.PlannedAction))
				}
			}
		}
	}

	if cleanPreviewFilterAllows(state.filter, cleanPreviewFilterSkipped) {
		builder.WriteString(fmt.Sprintf("\nSkipped items (%d)\n", len(model.Skipped)))
		if len(model.Skipped) == 0 {
			builder.WriteString("  No skipped cleanup paths reported.\n")
		} else {
			for _, skipped := range visibleCleanPreviewRows(state.scroll, model.Skipped) {
				builder.WriteString(fmt.Sprintf("  %s (rule: %s, reason: %s, not counted as Potential space)\n",
					skipped.Path, skipped.Rule, skipped.Reason.Code))
				if state.expanded && skipped.Reason.Message != "" {
					builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason.Message))
				}
			}
		}
	}

	if cleanPreviewFilterAllows(state.filter, cleanPreviewFilterReview) {
		writeCleanPreviewReviewSections(&builder, model, state)
	}

	if cleanPreviewFilterAllows(state.filter, cleanPreviewFilterErrors) {
		builder.WriteString(fmt.Sprintf("\nInspection errors (%d)\n", len(model.Errors)))
		if len(model.Errors) == 0 {
			builder.WriteString("  No recoverable inspection errors reported.\n")
		} else {
			for _, err := range visibleCleanPreviewRows(state.scroll, model.Errors) {
				builder.WriteString(fmt.Sprintf("  %s (rule: %s, error: %s, recoverable: %t)\n",
					err.Path, err.Rule, err.Code, err.Recoverable))
				if state.expanded && err.Message != "" {
					builder.WriteString(fmt.Sprintf("    %s\n", err.Message))
				}
			}
		}
	}

	builder.WriteString("\nHints: j/k scroll | f filter | e expand | c copy note | b back | q quit\n")
	builder.WriteString("No cleanup actions are available in this TUI view.\n")
	return builder.String()
}

func cleanPreviewFilterAllows(active, section cleanPreviewFilter) bool {
	return active == cleanPreviewFilterAll || active == section
}

func visibleCleanPreviewRows[T any](scroll int, rows []T) []T {
	const limit = 10
	if scroll < 0 {
		scroll = 0
	}
	if scroll >= len(rows) {
		return []T{}
	}
	end := scroll + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[scroll:end]
}

func writeCleanPreviewReviewSections(builder *strings.Builder, model clean.PreviewReadModel, state cleanPreviewTUIState) {
	builder.WriteString(fmt.Sprintf("\nSkipped by default (%d)\n", len(model.SkippedByDefault)))
	if len(model.SkippedByDefault) == 0 {
		builder.WriteString("  No skipped-by-default review items reported.\n")
	} else {
		for _, skipped := range visibleCleanPreviewRows(state.scroll, model.SkippedByDefault) {
			builder.WriteString(fmt.Sprintf("  %s", skipped.Name))
			if skipped.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", skipped.Path))
			}
			builder.WriteString(" (not counted as Potential space)\n")
			if state.expanded && skipped.Reason != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason))
			}
		}
	}

	builder.WriteString(fmt.Sprintf("\nReview clues (%d)\n", len(model.ReviewClues)))
	if len(model.ReviewClues) == 0 {
		builder.WriteString("  No review clues reported.\n")
	} else {
		for _, clue := range visibleCleanPreviewRows(state.scroll, model.ReviewClues) {
			builder.WriteString(fmt.Sprintf("  %s", clue.Name))
			if clue.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", clue.Path))
			}
			builder.WriteString(" (review only)\n")
			if state.expanded && clue.Details != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", clue.Details))
			}
		}
	}

	builder.WriteString(fmt.Sprintf("\nRunning application skips (%d)\n", len(model.RunningApplicationSkips)))
	if len(model.RunningApplicationSkips) == 0 {
		builder.WriteString("  No running application skips reported.\n")
	} else {
		for _, skipped := range visibleCleanPreviewRows(state.scroll, model.RunningApplicationSkips) {
			builder.WriteString(fmt.Sprintf("  %s", skipped.Name))
			if skipped.Application != "" {
				builder.WriteString(fmt.Sprintf(" (%s)", skipped.Application))
			}
			if skipped.Path != "" {
				builder.WriteString(fmt.Sprintf(" - %s", skipped.Path))
			}
			builder.WriteString(" (skipped, not executable here)\n")
			if state.expanded && skipped.Reason != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", skipped.Reason))
			}
		}
	}

	builder.WriteString(fmt.Sprintf("\nReview suggestions (%d)\n", len(model.ReviewSuggestions)))
	if len(model.ReviewSuggestions) == 0 {
		builder.WriteString("  No review suggestions reported.\n")
	} else {
		for _, suggestion := range visibleCleanPreviewRows(state.scroll, model.ReviewSuggestions) {
			builder.WriteString(fmt.Sprintf("  %s\n", suggestion.Label))
			if state.expanded && suggestion.NextStep != "" {
				builder.WriteString(fmt.Sprintf("    %s\n", suggestion.NextStep))
			}
		}
	}
}

func cleanFormatBytes(bytes int64) string {
	return fmt.Sprintf("%d bytes", bytes)
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

func writeError(stderr io.Writer, asJSON bool, command string, args []string, err jsonError) int {
	err.Command = command
	err.Args = args
	if asJSON {
		if code := writeJSON(stderr, envelope{Command: command, Error: &err}); code != exitOK {
			return code
		}
		return exitUsage
	}

	_, _ = fmt.Fprintf(stderr, "Foal error [%s]: %s\n", err.Code, err.Message)
	return exitUsage
}

func writeJSON(w io.Writer, value interface{}) int {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return 1
	}
	return exitOK
}
