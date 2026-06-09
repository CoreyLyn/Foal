package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	ExecutableName      string
	Args                []string
	InteractiveTerminal bool
	Input               io.Reader
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
		if invocation.InteractiveTerminal && !opts.json {
			_, _ = fmt.Fprint(stdout, runMainMenu(invocation.Input))
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
		description: "Preview conservative cleanup candidates; TUI browsing arrives in a later slice.",
		selection:   "Clean TUI path\nClean preview browsing is not built in this slice. Run `foal clean --dry-run` for the existing non-destructive preview.\nNo files were changed.",
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
	if input == nil {
		return mainMenuEntryText()
	}

	selected := 0
	var builder strings.Builder
	builder.WriteString(renderMainMenu(selected, ""))
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		key := strings.ToLower(strings.TrimSpace(scanner.Text()))
		switch key {
		case "q", "quit", "esc":
			builder.WriteString("\nFoal main menu closed.\n")
			return builder.String()
		case "j", "down":
			selected = (selected + 1) % len(mainMenuItems)
			builder.WriteString("\n")
			builder.WriteString(renderMainMenu(selected, ""))
		case "k", "up":
			selected = (selected + len(mainMenuItems) - 1) % len(mainMenuItems)
			builder.WriteString("\n")
			builder.WriteString(renderMainMenu(selected, ""))
		case "", "enter":
			builder.WriteString("\n")
			builder.WriteString(mainMenuItems[selected].selection)
			builder.WriteString("\n")
			builder.WriteString(renderMainMenu(selected, ""))
		default:
			builder.WriteString("\n")
			builder.WriteString(renderMainMenu(selected, "Unknown key. Use j/k, up/down, enter, or q."))
		}
	}
	if err := scanner.Err(); err != nil {
		builder.WriteString("\n")
		builder.WriteString(renderMainMenu(selected, "Input ended with an error; no files were changed."))
	}
	return builder.String()
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
