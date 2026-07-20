package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/CoreyLyn/Foal/internal/analyze"
	"github.com/CoreyLyn/Foal/internal/buildinfo"
	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/history"
	"github.com/CoreyLyn/Foal/internal/purge"
	"github.com/CoreyLyn/Foal/internal/status"
	"github.com/CoreyLyn/Foal/internal/uninstall"
)

const (
	exitOK    = 0
	exitUsage = 2
	// Conventional exit status for a process ended by an interrupt signal.
	exitInterrupted = 130
)

type commandSpec struct {
	name        string
	description string
}

var commands = []commandSpec{
	{name: "version", description: "Report Foal build and runtime metadata."},
	{name: "analyze", description: "Inspect disk usage and cleanup opportunities without changing files."},
	{name: "clean", description: "Preview or execute conservative cleanup; permanent categories need per-run --allow-permanent."},
	{name: "purge", description: "Preview or permanently delete rebuildable project artifacts under explicit root(s)."},
	{name: "status", description: "Report a read-only system and Foal state snapshot."},
	{name: "history", description: "Show previous Foal operation records."},
	{name: "uninstall", description: "Preview installed applications; --execute runs official uninstallers for selected apps."},
}

var (
	dryRunClean                 = clean.DryRun
	executeClean                = clean.Execute
	dryRunPurge                 = purge.DryRun
	executePurge                = purge.Execute
	loadProtectionConfiguration = clean.LoadProtectionConfiguration
	newHistoryRecorder          = func() (history.Recorder, error) {
		recorder, err := history.NewDefaultFileRecorder()
		if err != nil {
			return nil, err
		}
		return recorder, nil
	}
	newHistoryQuery = func() (history.FileQuery, error) {
		return history.NewDefaultFileQuery()
	}
	newHistoryDir    = history.DefaultDir
	reviewUninstall  = uninstall.Review
	executeUninstall = uninstall.Execute
	currentBuildInfo = buildinfo.Current
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
			return runInteractiveShell(invocation.Input, stdout)
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

	if command == "version" {
		if len(positional) > 1 {
			return writeError(stderr, opts.json, command, args, jsonError{
				Code:        "invalid_version_invocation",
				Message:     "version does not accept arguments",
				Recoverable: true,
				Command:     command,
				Args:        args,
			})
		}
		result := currentBuildInfo()
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		_, _ = fmt.Fprintf(stdout, "Foal %s\nCommit: %s\nGo: %s\nPlatform: %s/%s\n",
			result.Version, result.Commit, result.GoVersion, result.OS, result.Arch)
		return exitOK
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
		result, reason, ok := analyze.Run(context.Background(), root, analyze.Options{})
		if !ok {
			code := reason.Code
			if code == "" {
				code = "invalid_root"
			}
			return writeError(stderr, opts.json, command, args, jsonError{
				Code:        code,
				Message:     reason.Message,
				Recoverable: true,
				Command:     command,
				Args:        args,
			})
		}
		if opts.json {
			return writeJSON(stdout, envelope{Command: command, Result: result})
		}

		_, _ = fmt.Fprint(stdout, analyze.RenderHumanReport(result))
		return exitOK
	}

	if command == "uninstall" {
		invocation, err := validateUninstallArgs(positional[1:])
		if err != nil {
			return writeError(stderr, opts.json, command, args, jsonError{
				Code:        "invalid_uninstall_invocation",
				Message:     err.Error(),
				Recoverable: true,
				Command:     command,
				Args:        args,
			})
		}
		if !invocation.execute {
			// Default remains preview-only: no uninstaller is run, no process
			// is stopped, no leftover is deleted. The preview contract is
			// unchanged from pre-execute behavior.
			result := uninstall.WithReviewSections(reviewUninstall())
			if opts.json {
				return writeJSON(stdout, envelope{Command: command, Result: result})
			}
			_, _ = fmt.Fprint(stdout, uninstall.RenderPreviewReport(result))
			return exitOK
		}

		recorder, _ := newHistoryRecorder()
		protectionConfig := loadProtectionConfiguration()
		executeOptions := uninstall.ExecuteOptions{
			Selection:          append([]string(nil), invocation.selection...),
			AllowStopProcesses: invocation.allowStopProcesses,
			AllowPermanent:     invocation.allowPermanent,
			Validator:          protectionConfig.Validator,
			HistoryRecorder:    recorder,
			CommandParameters: history.CommandParameters{
				Command: "uninstall",
				Args:    append([]string(nil), args...),
			},
		}
		if protectionConfig.LoadError != nil {
			executeOptions.ProtectionLoadError = &uninstall.ProtectionLoadIssue{
				Code:        protectionConfig.LoadError.Code,
				Message:     protectionConfig.LoadError.Message,
				Recoverable: protectionConfig.LoadError.Recoverable,
			}
		}
		result := executeUninstall(context.Background(), executeOptions)
		if opts.json {
			code := writeJSON(stdout, envelope{Command: command, Result: result})
			if code != exitOK {
				return code
			}
			if result.Status == uninstall.StatusExecuteError {
				return exitUsage
			}
			return exitOK
		}
		_, _ = fmt.Fprint(stdout, uninstall.RenderExecuteReport(result))
		if result.Status == uninstall.StatusExecuteError {
			return exitUsage
		}
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

	if command == "purge" {
		invocation, err := validatePurgeArgs(positional[1:])
		if err != nil {
			return writeError(stderr, opts.json, command, args, jsonError{
				Code:        "invalid_purge_invocation",
				Message:     err.Error(),
				Recoverable: true,
				Command:     command,
				Args:        args,
			})
		}
		recorder, _ := newHistoryRecorder()
		protectionConfig := loadProtectionConfiguration()
		purgeOptions := purge.Options{
			Roots:                  append([]string(nil), invocation.roots...),
			AllowPermanentDeletion: invocation.allowPermanent,
			Validator:              protectionConfig.Validator,
			HistoryRecorder:        recorder,
			CommandParameters: history.CommandParameters{
				Command: "purge",
				Args:    append([]string(nil), args...),
			},
		}
		if protectionConfig.LoadError != nil {
			purgeOptions.ProtectionLoadError = &purge.Issue{
				Code:        protectionConfig.LoadError.Code,
				Message:     protectionConfig.LoadError.Message,
				Recoverable: protectionConfig.LoadError.Recoverable,
			}
			if purgeOptions.ProtectionLoadError.Code == "" {
				purgeOptions.ProtectionLoadError.Code = purge.IssueProtectionFileLoadFailed
			}
		}
		var result purge.Result
		if invocation.execute {
			result = executePurge(context.Background(), purgeOptions)
		} else {
			result = dryRunPurge(context.Background(), purgeOptions)
		}
		if opts.json {
			code := writeJSON(stdout, envelope{Command: command, Result: result})
			if code != exitOK {
				return code
			}
			if result.Status == purge.StatusError {
				return exitUsage
			}
			return exitOK
		}
		if invocation.execute {
			_, _ = fmt.Fprint(stdout, purge.RenderExecuteReport(result))
		} else {
			_, _ = fmt.Fprint(stdout, purge.RenderPreviewReport(result))
		}
		if result.Status == purge.StatusError {
			return exitUsage
		}
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

		// Validate opt-in names
		optInEnabled, invalidNames, validNames := clean.NormalizedOptInSet(invocation.optIn)
		if len(invalidNames) > 0 {
			var validList string
			for i, name := range validNames {
				if i > 0 {
					validList += ", "
				}
				validList += fmt.Sprintf("%q", name)
			}
			return writeError(stderr, opts.json, command, args, jsonError{
				Code:        "invalid_opt_in_name",
				Message:     fmt.Sprintf("invalid opt-in name(s): %q; valid names are %s", invalidNames, validList),
				Recoverable: true,
				Command:     command,
				Args:        args,
			})
		}
		// Build opt-in slice for clean.Options
		var optInSlice []string
		for _, category := range clean.CanonicalCleanupCategoryCatalog().Summaries() {
			if optInEnabled[category.Identifier] {
				optInSlice = append(optInSlice, category.Identifier)
			}
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
			OptIn:                  optInSlice,
			AllowPermanentDeletion: invocation.allowPermanent,
		}
		protectionConfig := loadProtectionConfiguration()
		cleanOptions.Validator = protectionConfig.Validator
		cleanOptions.ProtectionDiagnostics = protectionConfig.Diagnostics
		cleanOptions.ProtectionLoadError = protectionConfig.LoadError

		var result clean.Result
		if invocation.execute {
			// For execute, only set up running-application detection if we have opted-in categories.
			// Default execute behavior is preserved (no detection).
			if len(optInSlice) > 0 {
				cleanOptions.DetectRunningApplications = clean.DetectSupportedApplications
			}
			result = executeClean(context.Background(), cleanOptions)
		} else {
			// For dry-run, always enable running-application detection for browser_cache review and dev cache gating.
			// Dry-run reports true planned actions without requiring or consuming permanent authorization.
			cleanOptions.DetectRunningApplications = clean.DetectSupportedApplications
			result = dryRunClean(context.Background(), cleanOptions)
		}
		if opts.json {
			code := writeJSON(stdout, envelope{Command: command, Result: result})
			if code != exitOK {
				return code
			}
			if result.Status == "error" {
				return exitUsage
			}
			return exitOK
		}

		if result.Status == "error" {
			_, _ = fmt.Fprint(stdout, clean.RenderFailureReport(result))
			return exitUsage
		}
		if invocation.execute {
			_, _ = fmt.Fprint(stdout, renderCleanExecuteHumanSummary(result))
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

// runInteractiveShell runs the Bubble Tea main menu. The framework owns
// console raw mode, the alternate screen, and interrupt handling, restoring
// the terminal on every exit path.
func runInteractiveShell(input io.Reader, stdout io.Writer) int {
	model := newRootModel()
	if input == nil {
		// Diagnostic/test path: render the entry frame once without
		// starting a program, mirroring the pre-framework contract.
		_, _ = io.WriteString(stdout, model.content())
		return exitOK
	}
	_, err := runMenuProgram(model, input, stdout)
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return exitInterrupted
		}
		return exitUsage
	}
	_, _ = fmt.Fprint(stdout, "\nFoal main menu closed.\n")
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
		case "--version":
			positional = append(positional, "version")
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
	dryRun         bool
	execute        bool
	allowPermanent bool
	optIn          []string
}

type purgeInvocation struct {
	roots          []string
	execute        bool
	allowPermanent bool
}

type uninstallInvocation struct {
	execute            bool
	allowStopProcesses bool
	allowPermanent     bool
	selection          []string
}

func validateUninstallArgs(args []string) (uninstallInvocation, error) {
	var invocation uninstallInvocation
	execute := false
	allowStopProcesses := false
	allowPermanent := false
	dryRun := false
	var selection []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--execute":
			// --execute is the mandatory mutation authorization. Without it
			// behavior stays preview-only: no uninstaller is run, no process
			// is stopped, no leftover is deleted.
			execute = true
		case "--allow-stop-processes":
			// Separate per-run authorization for process stopping. Default is
			// off; --execute alone never stops or kills a process. When an
			// app is running and this flag is absent, Execute skips the app
			// with a stable reason instead of killing it.
			allowStopProcesses = true
		case "--allow-permanent":
			// Per-run authorization for portable directory removal (permanent
			// deletion of the install tree). Separate from --execute: both are
			// required for portable removal. Without it, portable-class apps
			// are skipped and nothing is permanently deleted. Mirrors
			// Purge/Clean --allow-permanent semantics. Dry-run accepts the
			// flag without authorizing mutation.
			allowPermanent = true
		case "--select":
			if i+1 >= len(args) {
				return invocation, fmt.Errorf("--select requires an application display name (use foal uninstall --json to list applications)")
			}
			i++
			selection = append(selection, args[i])
		default:
			if strings.HasPrefix(arg, "-") {
				return invocation, fmt.Errorf("unknown uninstall option: %s", arg)
			}
			// Allow a bare positional name as a shorthand for --select <name>
			// so `foal uninstall --execute "Example App"` works. This never
			// authorizes more than the explicit --select form.
			selection = append(selection, arg)
		}
	}
	if dryRun && execute {
		return invocation, fmt.Errorf("uninstall accepts either --dry-run or --execute, not both")
	}
	if allowStopProcesses && !execute && !dryRun {
		// Flag alone with no mode defaults to preview (non-mutating); allow
		// without error so users can compose flags without surprise.
	}
	if allowStopProcesses && !execute {
		// --allow-stop-processes without --execute does not authorize
		// mutation; preview stays preview. Accept the flag without error so
		// the user can compose it with --dry-run for planning.
	}
	return uninstallInvocation{
		execute:            execute,
		allowStopProcesses: allowStopProcesses,
		allowPermanent:     allowPermanent,
		selection:          selection,
	}, nil
}

func validatePurgeArgs(args []string) (purgeInvocation, error) {
	var invocation purgeInvocation
	var roots []string
	dryRun := false
	execute := false
	allowPermanent := false
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			// Default is already dry-run; accept explicit flag for clarity.
			dryRun = true
		case "--execute":
			execute = true
		case "--allow-permanent":
			// Per-run permanent-deletion authorization (same vocabulary as Clean).
			// Dry-run accepts the flag without authorizing mutation; execute without
			// it skips candidates with permanent_deletion_not_authorized.
			allowPermanent = true
		default:
			if strings.HasPrefix(arg, "-") {
				return invocation, fmt.Errorf("unknown purge option: %s", arg)
			}
			roots = append(roots, arg)
		}
	}
	if dryRun && execute {
		return invocation, fmt.Errorf("purge accepts either --dry-run or --execute, not both")
	}
	if len(roots) == 0 {
		return invocation, fmt.Errorf("purge requires an explicit root path (example: foal purge .\\my-project); refusing to scan without a user-supplied root")
	}
	if allowPermanent && !execute && !dryRun {
		// Flag alone with root defaults to dry-run (non-mutating); allow without error.
	}
	return purgeInvocation{roots: roots, execute: execute, allowPermanent: allowPermanent}, nil
}

func validateCleanArgs(args []string) (cleanInvocation, error) {
	var invocation cleanInvocation
	dryRun := false
	execute := false
	allowPermanent := false
	var optIn []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--execute":
			execute = true
		case "--allow-permanent":
			// Per-run permanent-deletion authorization (ADR 0018). Dry-run accepts
			// the flag but does not require or consume it; execute without it skips
			// permanent candidates with permanent_deletion_not_authorized.
			allowPermanent = true
		case "--opt-in":
			if i+1 >= len(args) {
				return invocation, fmt.Errorf("--opt-in requires an argument (use \"all\" for all available, or a specific name like \"user_temp\")")
			}
			i++
			optIn = append(optIn, args[i])
		default:
			return invocation, fmt.Errorf("unknown clean option: %s", arg)
		}
	}
	invocation = cleanInvocation{dryRun: dryRun, execute: execute, allowPermanent: allowPermanent, optIn: optIn}
	if dryRun && execute {
		return invocation, fmt.Errorf("clean accepts either --dry-run or --execute, not both")
	}
	if !dryRun && !execute {
		if len(optIn) > 0 {
			return invocation, fmt.Errorf("--opt-in requires either --dry-run or --execute")
		}
		if allowPermanent {
			return invocation, fmt.Errorf("--allow-permanent requires --execute (or may accompany --dry-run without authorizing mutation)")
		}
		return invocation, fmt.Errorf("clean requires explicit --dry-run preview or --execute confirmation")
	}
	return invocation, nil
}

// renderCleanExecuteHumanSummary reports action-aware completion totals without
// claiming free space, secure erasure, or Recycle Bin fallback.
func renderCleanExecuteHumanSummary(result clean.Result) string {
	var builder strings.Builder
	builder.WriteString("Foal clean\n")
	builder.WriteString(fmt.Sprintf("Execution complete. Deleted: %d, skipped: %d.\n",
		result.Totals.DeletedCount, result.Totals.SkippedCount))
	builder.WriteString(fmt.Sprintf("Recycle Bin moved: %d bytes. Permanently deleted: %d bytes. Affected: %d bytes.\n",
		result.Totals.RecycleBinMovedBytes, result.Totals.PermanentlyDeletedBytes, result.Totals.AffectedBytes))
	if result.Totals.PermanentlyDeletedBytes > 0 {
		builder.WriteString("Permanent deletion is ordinary filesystem removal; it is irreversible and is not a secure-erasure wipe.\n")
	}
	return builder.String()
}

func helpText() string {
	var builder strings.Builder
	builder.WriteString("Foal - safe, preview-first cleanup for Windows\n\n")
	builder.WriteString("Usage:\n")
	builder.WriteString("  foal [--json] <command>\n")
	builder.WriteString("  foal --help\n")
	builder.WriteString("  foal --version [--json]\n\n")
	builder.WriteString("Commands:\n")
	for _, command := range commands {
		builder.WriteString(fmt.Sprintf("  %-10s %s\n", command.name, command.description))
	}
	builder.WriteString("\nClean options:\n")
	builder.WriteString("  --dry-run            Preview candidates and true planned actions (no authorization required).\n")
	builder.WriteString("  --execute            Confirm cleanup for freshly resolved candidates.\n")
	builder.WriteString("  --allow-permanent    Per-run authorization for permanent deletion (with --execute).\n")
	builder.WriteString("                       Without it, permanent categories are skipped; Recycle Bin work continues.\n")
	builder.WriteString("                       Permanent deletion is ordinary filesystem removal (not secure erasure)\n")
	builder.WriteString("                       and is never used as a Recycle Bin fallback.\n")
	builder.WriteString("  --opt-in <name>      Include an opt-in category (repeatable).\n")
	builder.WriteString("                       Group tokens: \"all\", \"dev-caches\", \"app-caches\", \"cli-agents\".\n")
	builder.WriteString("                       \"app-caches\" expands non-editor Application cache categories under\n")
	builder.WriteString("                       the Applications report category (e.g. obsidian_cache); \"dev-caches\"\n")
	builder.WriteString("                       expands developer-cache plus editor Application cache categories only.\n")
	builder.WriteString("                       \"cli-agents\" expands independently registered product-scoped\n")
	builder.WriteString("                       CLI-agent categories only; it is not a mega-category and does not\n")
	builder.WriteString("                       imply all CLI-agent data is cache or safe to delete.\n")
	builder.WriteString("\nPurge options:\n")
	builder.WriteString("  <root> [root...]     Required explicit project/workspace root(s) to scan (never implied).\n")
	builder.WriteString("                       Multiple roots must each be valid; volume roots and system paths are rejected.\n")
	builder.WriteString("                       Not a Clean category: Clean does not discover project artifacts.\n")
	builder.WriteString("  --dry-run            Preview only (default). Reports planned permanent deletion without mutation.\n")
	builder.WriteString("  --execute            Fresh rediscovery then permanent deletion of matching artifacts.\n")
	builder.WriteString("  --allow-permanent    Per-run authorization for permanent deletion (with --execute).\n")
	builder.WriteString("                       Without it, candidates are skipped; nothing is deleted.\n")
	builder.WriteString("                       Permanent deletion is ordinary filesystem removal (not secure erasure).\n")
	builder.WriteString("                       Removing project artifacts requires reinstall/rebuild afterward.\n")
	builder.WriteString("                       v1 allowlist: node_modules, target, dist, build, .build, .next, __pycache__.\n")
	builder.WriteString("\nUninstall options:\n")
	builder.WriteString("  (no flags)           Preview installed applications and possible residue (default; no mutation).\n")
	builder.WriteString("  --execute            Required for mutation. Runs official uninstallers for selected apps.\n")
	builder.WriteString("                       Prefers quiet uninstall, then falls back to interactive.\n")
	builder.WriteString("  --select <name>      Select one application by display name (repeatable).\n")
	builder.WriteString("                       A bare name is accepted as shorthand: foal uninstall --execute \"Example App\".\n")
	builder.WriteString("  --allow-stop-processes  Separate per-run authorization for process stopping. Default is off;\n")
	builder.WriteString("                       --execute alone never kills a process. Running apps without this flag are\n")
	builder.WriteString("                       skipped with a stable reason. Leftover deletion uses the Recycle Bin and runs only\n")
	builder.WriteString("                       after the uninstaller reports success; a failed or canceled uninstaller deletes nothing.\n")
	builder.WriteString("  --allow-permanent    Per-run authorization for portable directory removal (with --execute).\n")
	builder.WriteString("                       Apps without an uninstall command but with a trusted install location are\n")
	builder.WriteString("                       skipped without this flag; nothing is permanently deleted. Portable removal\n")
	builder.WriteString("                       is ordinary filesystem removal (not the Recycle Bin, not secure erasure) and is\n")
	builder.WriteString("                       never a silent fallback for a failed official uninstaller.\n")
	builder.WriteString("\nExamples:\n")
	builder.WriteString("  foal status --json\n")
	builder.WriteString("  foal clean --dry-run\n")
	builder.WriteString("  foal clean --execute\n")
	builder.WriteString("  foal clean --execute --opt-in d3d_shader_cache --allow-permanent\n")
	builder.WriteString("  foal clean --execute --opt-in playwright-browsers --allow-permanent\n")
	builder.WriteString("  foal purge .\\my-project\n")
	builder.WriteString("  foal purge --json .\\proj-a .\\proj-b\n")
	builder.WriteString("  foal purge --execute --allow-permanent .\\my-project\n")
	builder.WriteString("  foal uninstall --json\n")
	builder.WriteString("  foal uninstall --execute --select \"Example App\"\n")
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
