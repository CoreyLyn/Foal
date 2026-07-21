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

// CategoryThunderUpdateDownload is the canonical exact-selection-only opt-in
// category for Thunder update downloads under the fixed ProgramData XLLiveUD
// download root.
const CategoryThunderUpdateDownload = "thunder-update-download"

// ApplicationThunder is the one logical process/service identity Foal reports
// for Thunder application and service activity. Clean never stops these;
// active or unknown state skips the entire thunder-update-download category.
const ApplicationThunder = "thunder"

// thunderUpdateDownloadCacheRoot is the only production discovery root. Foal
// never uses environment, registry, alternate-drive, or parent-directory discovery.
const thunderUpdateDownloadCacheRoot = `C:\ProgramData\Thunder Network\XLLiveUD\Download`

// thunderUpdateDownloadOptInImpactNotice is the path-free impact vocabulary shown
// when a candidate is present. It discloses that this cache is machine-wide
// and affects all users, uses Recycle Bin only, and that Thunder processes
// must be idle.
const thunderUpdateDownloadOptInImpactNotice = "Opt-in Thunder update download cleanup moves update packages to the Recycle Bin. Packages may be re-downloaded on demand, but this affects all users of the machine. Thunder processes and services must be idle; active or unknown state skips the entire category. This is not permanent deletion and is not secure erasure."

// ThunderUpdateDownloadDiscoveryOptions injects the fixed-root override, clock,
// and platform detection seams for the thunder-update-download category.
// Production leaves the zero value so the fixed ProgramData root and the
// platform process/service detectors are used. Tests must use isolated roots
// and never read the real Thunder download cache tree.
type ThunderUpdateDownloadDiscoveryOptions struct {
	// Root overrides the fixed production discovery root. Test-only:
	// production leaves it empty so only C:\ProgramData\Thunder Network\XLLiveUD\Download is used.
	Root string
	// Now overrides the current time for stability window calculations.
	// Test-only; production leaves it zero so time.Now() is used.
	Now time.Time
	// DetectActivity reports Thunder process/service activity. nil selects the
	// production platform detector (processes + services). Non-Windows production
	// fails closed to Unknown so the whole category is skipped.
	DetectActivity func(context.Context) ThunderUpdateDownloadActivityState
}

// ThunderUpdateDownloadActivityStatus reports whether relevant Thunder process/service
// activity is idle, running, or of unknown/undetermined state. Unknown never means idle.
type ThunderUpdateDownloadActivityStatus string

const (
	ThunderUpdateDownloadActivityIdle    ThunderUpdateDownloadActivityStatus = "idle"
	ThunderUpdateDownloadActivityRunning ThunderUpdateDownloadActivityStatus = "running"
	ThunderUpdateDownloadActivityUnknown ThunderUpdateDownloadActivityStatus = "unknown"
)

// ThunderUpdateDownloadActivityState is one conservative process/service observation.
// Message is optional path-free diagnostic text.
type ThunderUpdateDownloadActivityState struct {
	Status  ThunderUpdateDownloadActivityStatus
	Message string
}

type thunderUpdateDownloadCacheDeps struct {
	root            string
	now             time.Time
	lstat           func(string) (os.FileInfo, error)
	readDir         func(string) ([]fs.DirEntry, error)
	walkDir         func(string, fs.WalkDirFunc) error
	detectActivity  func(context.Context) ThunderUpdateDownloadActivityState
	joinPath        func(...string) string
	isPathUnderRoot func(string, string) bool
}

func productionThunderUpdateDownloadCacheDeps(opts ThunderUpdateDownloadDiscoveryOptions) thunderUpdateDownloadCacheDeps {
	deps := thunderUpdateDownloadCacheDeps{
		root:            thunderUpdateDownloadCacheRoot,
		lstat:           os.Lstat,
		readDir:         os.ReadDir,
		walkDir:         filepath.WalkDir,
		joinPath:        filepath.Join,
		isPathUnderRoot: isPathUnderRoot,
	}
	if trimmed := strings.TrimSpace(opts.Root); trimmed != "" {
		deps.root = filepath.Clean(trimmed)
	}
	if !opts.Now.IsZero() {
		deps.now = opts.Now
	} else {
		deps.now = time.Now()
	}
	if opts.DetectActivity != nil {
		deps.detectActivity = opts.DetectActivity
	} else {
		deps.detectActivity = productionDetectThunderUpdateDownloadActivity
	}
	return deps
}

// categoryResolverThunderUpdateDownload is the dedicated private resolver kind for
// the thunder-update-download category. It is intentionally not a developer-cache or
// application-cache entry: the `all`, `dev-caches`, `app-caches`, and `cli-agents`
// tokens never select it, and TUI Select All excludes it.
const categoryResolverThunderUpdateDownload categoryResolverKind = "thunder-update-download"

type thunderUpdateDownloadCacheResolver struct{}

func (thunderUpdateDownloadCacheResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveThunderUpdateDownloadCategory(ctx, opts, category, core)
}

func thunderUpdateDownloadCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverThunderUpdateDownload,
		resolver:     thunderUpdateDownloadCacheResolver{},
	}
}

func init() {
	registerCategoryIdentityValidator(CategoryThunderUpdateDownload, validateThunderUpdateDownloadCacheIdentity)
}

// validateThunderUpdateDownloadCacheIdentity is the action-neutral, category-owned immediate
// pre-mutation validator. Immediately before Recycle Bin move, it repeats the
// per-candidate Thunder proof against a fresh read: direct child of the fixed
// root, non-reparse point, and Protection validation. It must not mutate,
// never expand candidates, and never repeat the category-wide process/service
// idle gate. Any changed, ambiguous, protected, unsupported, or invalid state
// returns ok=false so the candidate is skipped fail-closed; the move_to_recycle_bin
// action is preserved and never becomes permanent deletion.
func validateThunderUpdateDownloadCacheIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	reject := func(message string) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: "thunder_update_download_cache_revalidation_failed", Message: message}, false
	}
	if candidate.Category != CategoryThunderUpdateDownload {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "category identity does not match thunder-update-download"}, false
	}
	path := strings.TrimSpace(candidate.Path)
	if path == "" {
		return reject("Thunder update download cache candidate path is empty")
	}

	deps := productionThunderUpdateDownloadCacheDeps(candidate.thunderUpdateDownloadDiscovery)
	root := deps.root
	if strings.TrimSpace(root) == "" {
		return reject("Thunder update download cache root is no longer resolvable")
	}

	// Must be a direct child of the root
	if !isDirectChildPath(root, path) {
		return reject("Thunder update download cache candidate is not a direct child of the download root")
	}

	// Must not be a reparse point
	info, err := deps.lstat(path)
	if err != nil {
		return reject("Thunder update download cache candidate is no longer readable")
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
		return reject("Thunder update download cache candidate is no longer an ordinary non-reparse path")
	}

	return pathsafe.Reason{}, true
}

// resolveThunderUpdateDownloadCategory is the shared DryRun / ResolveCategory
// resolution seam. It must not mutate. Every gate fails closed: any failure or
// uncertainty produces no candidate, and active or unknown Thunder process/service
// state skips the entire category.
//
// Order: resolve fixed root → pre activity gate → discover exact candidates →
// post activity gate → measure candidates.
func resolveThunderUpdateDownloadCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || category != CategoryThunderUpdateDownload {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionThunderUpdateDownloadCacheDeps(opts.ThunderUpdateDownloadDiscoveryOptions)

	root := deps.root
	if strings.TrimSpace(root) == "" {
		return
	}
	// Root must be a real, non-reparse directory. Missing is silent absence.
	info, err := deps.lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue("thunder_update_download_cache_root_unreadable",
			"Thunder update download cache root could not be inspected; thunder update download cache was skipped", true, "", category))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.IsDir() {
		core.Diagnostics = append(core.Diagnostics, issue("thunder_update_download_cache_root_unreadable",
			"Thunder update download cache root is not an ordinary directory; thunder update download cache was skipped", true, "", category))
		return
	}
	// Protected root suppresses path-backed discovery before totals.
	if opts.Validator.IsUserProtected(root) {
		core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root)
		return
	}

	// Pre activity gate: active or unknown Thunder state skips the whole category.
	if !thunderUpdateDownloadActivityIdleGate(ctx, deps, root, category, core) {
		return
	}

	// Discover exact candidates: direct children only, no reparse points,
	// with 30-day stability window on latest observed modification across
	// child and all safely inspectable descendants.
	entries, err := deps.readDir(root)
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue("thunder_update_download_cache_root_unreadable",
			"Thunder update download cache root could not be listed; thunder update download cache was skipped", true, "", category))
		return
	}

	type measuredCandidate struct {
		path  string
		bytes int64
	}
	var measured []measuredCandidate

	// Process in deterministic order
	var names []string
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

		// Must be a direct child only (reject path tricks)
		if !isDirectChildPath(root, path) {
			continue
		}

		// Skip reparse points
		info, statErr := deps.lstat(path)
		if statErr != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			continue
		}

		// Skip if protected
		if opts.Validator.IsUserProtected(path) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, path)
			continue
		}

		// Inspect for latest observed modification time
		inspection, inspectErr := inspectOpportunity(ctx, path, userTempDescendantLimit, deps.walkDir)
		if inspectErr != nil {
			// Incomplete inspection disqualifies the child
			continue
		}

		// Require at least 30-day stability window
		idleDays := int(deps.now.Sub(inspection.latestModifiedAt) / (24 * time.Hour))
		if idleDays < 30 {
			continue
		}

		measured = append(measured, measuredCandidate{path: path, bytes: inspection.bytes})
	}

	if len(measured) == 0 {
		return
	}

	// Post activity re-check: any active or unknown Thunder state discards all
	// measured candidates for the whole category.
	if !thunderUpdateDownloadActivityIdleGate(ctx, deps, root, category, core) {
		return
	}

	for _, c := range measured {
		core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
			Path:          c.path,
			Bytes:         c.bytes,
			Category:      category,
			PlannedAction: plannedActionForOpts(opts, category),
		})
	}
}

// thunderUpdateDownloadActivityIdleGate detects Thunder activity once and returns true only when
// idle. Running or unknown state records a whole-category SkippedItem and
// projects the Thunder running state. Clean never stops processes or services.
func thunderUpdateDownloadActivityIdleGate(ctx context.Context, deps thunderUpdateDownloadCacheDeps, root, category string, core *categoryCoreResult) bool {
	detect := deps.detectActivity
	if detect == nil {
		detect = productionDetectThunderUpdateDownloadActivity
	}
	activity := detect(ctx)
	switch activity.Status {
	case ThunderUpdateDownloadActivityIdle:
		runningGateOutcome{runningStates: []RunningApplicationState{{
			Application: ApplicationThunder,
			State:       RunningApplicationStateIdle,
		}}}.apply(&core.RunningStates, nil)
		return true
	case ThunderUpdateDownloadActivityRunning:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationThunder,
			State:       RunningApplicationStateRunning,
			Message:     activity.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue("thunder_update_download_activity",
				"Thunder application or service is active; thunder update download cache was skipped",
				true, root, category),
		})
		return false
	default:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationThunder,
			State:       RunningApplicationStateUnknown,
			Message:     activity.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue("thunder_update_download_activity",
				"Thunder process or service state could not be determined; thunder update download cache was skipped",
				true, root, category),
		})
		return false
	}
}
