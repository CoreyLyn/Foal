package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/CoreyLyn/Foal/internal/analyze"
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
	{name: "clean", description: "Preview conservative cleanup candidates; execution will remain Recycle Bin-only."},
	{name: "status", description: "Report a read-only system and Foal state snapshot."},
	{name: "history", description: "Show previous Foal operation records."},
	{name: "uninstall", description: "Preview application uninstall evidence without executing uninstallers."},
}

// Run executes the Foal command line with output streams supplied by the caller.
func Run(args []string, stdout, stderr io.Writer) int {
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

	if len(positional) == 0 || positional[0] == "help" {
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
		result := uninstall.Review()
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		_, _ = fmt.Fprint(stdout, "Foal uninstall\nPreview only. No uninstallers, process stops, or leftover deletion actions were executed.\n")
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
	builder.WriteString("  foal.exe analyze\n")
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
