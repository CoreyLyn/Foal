package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// CategoryWindowsTemp is the canonical exact-selection-only opt-in category for
// stale direct children of the machine-shared system temp directory
// %SystemRoot%\Temp. It is the machine-wide analogue of user_temp (ADR 0030 /
// ADR 0032).
const CategoryWindowsTemp = "windows-temp"

// windowsTempStabilityWindowDays is the minimum latest-observed-modification age
// (inclusive: exactly this many days qualifies) a direct child must reach before
// it can become a candidate. Unknown or future timestamps fail closed.
const windowsTempStabilityWindowDays = 14

// windowsTempOptInImpactNotice is the path-free machine-wide impact vocabulary
// shown when a candidate is present. It discloses that this cache is machine-wide
// and affects all users, uses the Recycle Bin only, is never permanent, and that
// a non-elevated run reclaims only the user-deletable subset.
const windowsTempOptInImpactNotice = "Opt-in Windows system temp cleanup moves stale entries from the shared system temp directory to the Recycle Bin. This affects all users of the machine. Foal never elevates, so entries owned by other users or by system services are skipped and only part of the directory is reclaimed. This is not permanent deletion and is not secure erasure."

// WindowsTempDiscoveryOptions injects the root override, SystemRoot override, and
// clock seams for the windows-temp category. Production leaves the zero value so
// the resolved %SystemRoot%\Temp root and time.Now() are used. Tests must use
// isolated roots and never read or mutate the real C:\Windows\Temp tree.
type WindowsTempDiscoveryOptions struct {
	// Root overrides the resolved %SystemRoot%\Temp discovery root. Test-only:
	// production leaves it empty so the SystemRoot environment resolution is used.
	Root string
	// SystemRoot overrides the SystemRoot environment value used for root
	// resolution. Test-only; production leaves it empty so os.Getenv("SystemRoot")
	// is read. Ignored when Root is set.
	SystemRoot string
	// Now overrides the current time for stability window calculations. Test-only;
	// production leaves it zero so time.Now() is used.
	Now time.Time
}

type windowsTempDeps struct {
	// root is the resolved %SystemRoot%\Temp directory. Empty means the root is
	// not resolvable (silent absence).
	root     string
	now      time.Time
	lstat    func(string) (os.FileInfo, error)
	readDir  func(string) ([]fs.DirEntry, error)
	walkDir  func(string, fs.WalkDirFunc) error
	joinPath func(...string) string
}

// resolveWindowsTempRootFromSystemRoot resolves the exact %SystemRoot%\Temp
// directory from a SystemRoot value. It fails closed (ok=false) for blank,
// relative, UNC, or otherwise unusable values so those are silent absence.
func resolveWindowsTempRootFromSystemRoot(systemRoot string) (string, bool) {
	trimmed := strings.TrimSpace(systemRoot)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, `\\`) {
		return "", false
	}
	if !filepath.IsAbs(trimmed) {
		return "", false
	}
	return filepath.Join(filepath.Clean(trimmed), "Temp"), true
}

func productionWindowsTempDeps(opts WindowsTempDiscoveryOptions) windowsTempDeps {
	deps := windowsTempDeps{
		lstat:    os.Lstat,
		readDir:  os.ReadDir,
		walkDir:  filepath.WalkDir,
		joinPath: filepath.Join,
	}
	if trimmed := strings.TrimSpace(opts.Root); trimmed != "" {
		deps.root = filepath.Clean(trimmed)
	} else {
		systemRoot := opts.SystemRoot
		if strings.TrimSpace(systemRoot) == "" {
			systemRoot = os.Getenv("SystemRoot")
		}
		if root, ok := resolveWindowsTempRootFromSystemRoot(systemRoot); ok {
			deps.root = root
		}
	}
	if !opts.Now.IsZero() {
		deps.now = opts.Now
	} else {
		deps.now = time.Now()
	}
	return deps
}

// categoryResolverWindowsTemp is the dedicated private resolver kind for the
// windows-temp category. It is intentionally not a developer-cache or
// application-cache entry: the `all`, `dev-caches`, `app-caches`, and
// `cli-agents` tokens never select it, and TUI Select All excludes it.
const categoryResolverWindowsTemp categoryResolverKind = "windows-temp"

type windowsTempResolver struct{}

func (windowsTempResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveWindowsTempCategory(ctx, opts, category, core)
}

func windowsTempCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverWindowsTemp,
		resolver:     windowsTempResolver{},
	}
}

func init() {
	registerCategoryIdentityValidator(CategoryWindowsTemp, validateWindowsTempIdentity)
}

// validateWindowsTempIdentity is the action-neutral, category-owned immediate
// pre-mutation validator. Immediately before the Recycle Bin move it repeats the
// per-candidate proof against a fresh read: the root re-resolves, the path is a
// direct child of that root, is a non-reparse ordinary path, and is still at
// least the stability window old. It must not mutate, never expand candidates,
// and any changed, ambiguous, unreadable, or too-recent state returns ok=false so
// the candidate is skipped fail-closed; the move_to_recycle_bin action is
// preserved and never becomes permanent deletion.
func validateWindowsTempIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	reject := func(message string) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: "windows_temp_revalidation_failed", Message: message}, false
	}
	if candidate.Category != CategoryWindowsTemp {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "category identity does not match windows-temp"}, false
	}
	path := strings.TrimSpace(candidate.Path)
	if path == "" {
		return reject("Windows system temp candidate path is empty")
	}

	deps := productionWindowsTempDeps(candidate.windowsTempDiscovery)
	root := deps.root
	if strings.TrimSpace(root) == "" {
		return reject("Windows system temp root is no longer resolvable")
	}

	// Must be a direct child of the resolved root.
	if !isDirectChildPath(root, path) {
		return reject("Windows system temp candidate is not a direct child of the system temp root")
	}

	// Must not be a reparse point.
	info, err := deps.lstat(path)
	if err != nil {
		return reject("Windows system temp candidate is no longer readable")
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
		return reject("Windows system temp candidate is no longer an ordinary non-reparse path")
	}

	// Must still be past the stability window on a fresh inspection.
	inspection, inspectErr := inspectOpportunity(context.Background(), path, userTempDescendantLimit, deps.walkDir)
	if inspectErr != nil {
		return reject("Windows system temp candidate could not be re-inspected")
	}
	if int(deps.now.Sub(inspection.latestModifiedAt)/(24*time.Hour)) < windowsTempStabilityWindowDays {
		return reject("Windows system temp candidate is no longer past the stability window")
	}

	return pathsafe.Reason{}, true
}

// resolveWindowsTempCategory is the shared DryRun / ResolveCategory resolution
// seam. It must not mutate. Every gate fails closed: any failure or uncertainty
// produces no candidate. Access-denied enumeration of the root is a whole-
// category recoverable skip; per-item stat failures are per-item skips.
//
// Order: resolve %SystemRoot%\Temp root → discover exact direct-child candidates
// (non-reparse, past the stability window) → measure candidates.
func resolveWindowsTempCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || category != CategoryWindowsTemp {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionWindowsTempDeps(opts.WindowsTempDiscoveryOptions)

	root := deps.root
	if strings.TrimSpace(root) == "" {
		// SystemRoot is unset/blank/relative/UNC: silent absence.
		return
	}
	// Root must be a real, non-reparse directory. Missing is silent absence.
	info, err := deps.lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue("windows_temp_root_unreadable",
			"Windows system temp root could not be inspected; windows-temp was skipped", true, "", category))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.IsDir() {
		core.Diagnostics = append(core.Diagnostics, issue("windows_temp_root_unreadable",
			"Windows system temp root is not an ordinary directory; windows-temp was skipped", true, "", category))
		return
	}
	// Protected root suppresses path-backed discovery before totals.
	if opts.Validator.IsUserProtected(root) {
		core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root)
		return
	}

	entries, err := deps.readDir(root)
	if err != nil {
		// Access-denied (or otherwise unreadable) enumeration skips the whole
		// category; it never elevates and never partially guesses.
		core.Diagnostics = append(core.Diagnostics, issue("windows_temp_root_unreadable",
			"Windows system temp root could not be listed; windows-temp was skipped", true, "", category))
		return
	}

	// Deterministic order for contract stability.
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.SliceStable(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })

	for _, name := range names {
		select {
		case <-ctx.Done():
			core.Diagnostics = append(core.Diagnostics, issue(PreviewReasonContextCanceled, ctx.Err().Error(), true, "", category))
			return
		default:
		}

		path := deps.joinPath(root, name)

		// Direct children only; reject path tricks.
		if !isDirectChildPath(root, path) {
			continue
		}

		// Per-item stat failure (access denied / locked) is a per-item skip, never
		// a category failure.
		info, statErr := deps.lstat(path)
		if statErr != nil {
			continue
		}
		// Reparse points are never candidates and never traversed.
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			continue
		}

		if opts.Validator.IsUserProtected(path) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, path)
			continue
		}

		// Deep latest-write inspection (user_temp semantics): a directory child is
		// one candidate covering its subtree. Incomplete inspection (including a
		// reparse point anywhere in the subtree) disqualifies the child fail-closed.
		inspection, inspectErr := inspectOpportunity(ctx, path, userTempDescendantLimit, deps.walkDir)
		if inspectErr != nil {
			continue
		}

		// Require at least the stability window (inclusive). Future timestamps make
		// the difference negative, so they fail the window and are excluded.
		if int(deps.now.Sub(inspection.latestModifiedAt)/(24*time.Hour)) < windowsTempStabilityWindowDays {
			continue
		}

		core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
			Path:          path,
			Bytes:         inspection.bytes,
			Category:      category,
			PlannedAction: plannedActionForOpts(opts, category),
		})
	}
}
