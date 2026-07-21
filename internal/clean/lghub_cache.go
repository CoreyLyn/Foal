package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// CategoryLGHUBCache is the canonical exact-selection-only opt-in
// category for LG HUB content-addressed download blobs under the fixed
// ProgramData LGHUB cache root.
const CategoryLGHUBCache = "lghub-cache"

// ApplicationLGHUB is the one logical process/service identity Foal reports
// for LG HUB application and service activity. Clean never stops these;
// active or unknown state skips the entire lghub-cache category.
const ApplicationLGHUB = "lghub"

// lghubCacheRoot is the only production discovery root. Foal never uses
// environment, registry, alternate-drive, or parent-directory discovery.
const lghubCacheRoot = `C:\ProgramData\LGHUB\cache`

// lghubCacheOptInImpactNotice is the path-free impact vocabulary shown
// when a candidate is present. It discloses that this cache is machine-wide
// and affects all users, uses Recycle Bin only, and that LG HUB processes
// must be idle.
const lghubCacheOptInImpactNotice = "Opt-in LG HUB cache cleanup moves content-addressed download blobs to the Recycle Bin. Blobs can normally be re-downloaded on demand, but this affects all users of the machine. LG HUB processes and services must be idle; active or unknown state skips the entire category. This is not permanent deletion and is not secure erasure."

// LGHUBCacheDiscoveryOptions injects the fixed-root override and platform
// detection seams for hermetic tests. Production leaves the zero value so
// the fixed ProgramData root and the platform process/service detectors
// are used. Tests must never inspect a real LGHUB cache tree.
type LGHUBCacheDiscoveryOptions struct {
	// Root overrides the fixed production discovery root. Test-only:
	// production leaves it empty so only C:\ProgramData\LGHUB\cache is used.
	Root string
	// DetectActivity reports LG HUB process/service activity. nil selects the
	// production platform detector (processes + services). Non-Windows production
	// fails closed to Unknown so the whole category is skipped.
	DetectActivity func(context.Context) LGHUBActivityState
}

// LGHUBActivityStatus reports whether relevant LG HUB process/service activity
// is idle, running, or of unknown/undetermined state. Unknown never means idle.
type LGHUBActivityStatus string

const (
	LGHUBActivityIdle    LGHUBActivityStatus = "idle"
	LGHUBActivityRunning LGHUBActivityStatus = "running"
	LGHUBActivityUnknown LGHUBActivityStatus = "unknown"
)

// LGHUBActivityState is one conservative process/service observation. Message
// is optional path-free diagnostic text.
type LGHUBActivityState struct {
	Status  LGHUBActivityStatus
	Message string
}

type lghubCacheDeps struct {
	root            string
	lstat           func(string) (os.FileInfo, error)
	readDir         func(string) ([]fs.DirEntry, error)
	detectActivity  func(context.Context) LGHUBActivityState
	joinPath        func(...string) string
	isPathUnderRoot func(string, string) bool
}

func productionLGHUBCacheDeps(opts LGHUBCacheDiscoveryOptions) lghubCacheDeps {
	deps := lghubCacheDeps{
		root:            lghubCacheRoot,
		lstat:           os.Lstat,
		readDir:         os.ReadDir,
		joinPath:        filepath.Join,
		isPathUnderRoot: isPathUnderRoot,
	}
	if trimmed := strings.TrimSpace(opts.Root); trimmed != "" {
		deps.root = filepath.Clean(trimmed)
	}
	if opts.DetectActivity != nil {
		deps.detectActivity = opts.DetectActivity
	} else {
		deps.detectActivity = productionDetectLGHUBActivity
	}
	return deps
}

// categoryResolverLGHUBCache is the dedicated private resolver kind for
// the LG HUB cache category. It is intentionally not a developer-cache or
// application-cache entry: the `all`, `dev-caches`, `app-caches`, and
// `cli-agents` tokens never select it, and TUI Select All excludes it.
const categoryResolverLGHUBCache categoryResolverKind = "lghub-cache"

type lghubCacheResolver struct{}

func (lghubCacheResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveLGHUBCacheCategory(ctx, opts, category, core)
}

func lghubCacheCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverLGHUBCache,
		resolver:     lghubCacheResolver{},
	}
}

func init() {
	registerCategoryIdentityValidator(CategoryLGHUBCache, validateLGHUBCacheIdentity)
}

// validateLGHUBCacheIdentity is the action-neutral, category-owned immediate
// pre-mutation validator. Immediately before Recycle Bin move, it repeats the
// per-candidate LG HUB proof against a fresh read: direct child of the fixed
// root, exactly 64 lowercase hex characters, ordinary non-reparse file, and
// Protection validation. It never mutates, never expands candidates, and never
// repeats the category-wide process/service idle gate. Any changed, ambiguous,
// protected, unsupported, or invalid state returns ok=false so the candidate
// is skipped fail-closed; the move_to_recycle_bin action is preserved and never
// becomes permanent deletion.
func validateLGHUBCacheIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	reject := func(message string) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: "lghub_cache_revalidation_failed", Message: message}, false
	}
	if candidate.Category != CategoryLGHUBCache {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "category identity does not match lghub-cache"}, false
	}
	path := strings.TrimSpace(candidate.Path)
	if path == "" {
		return reject("LG HUB cache candidate path is empty")
	}

	deps := productionLGHUBCacheDeps(candidate.lghubDiscovery)
	root := deps.root
	if strings.TrimSpace(root) == "" {
		return reject("LGHUB cache root is no longer resolvable")
	}

	// Must be a direct child of the root
	if !isDirectChildPath(root, path) {
		return reject("LG HUB cache candidate is not a direct child of the cache root")
	}

	// Must be an exact 64 lowercase hex filename
	name := filepath.Base(path)
	if !is64LowerHexName(name) {
		return reject("LG HUB cache candidate is not an ordinary file named 64 lowercase hex characters")
	}

	// Must still be an ordinary non-reparse file
	info, err := deps.lstat(path)
	if err != nil {
		return reject("LG HUB cache candidate is no longer readable")
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.Mode().IsRegular() {
		return reject("LG HUB cache candidate is no longer an ordinary non-reparse file")
	}
	if info.Size() != candidate.Bytes {
		return reject("LG HUB cache candidate size changed since resolution; it was not moved to the Recycle Bin")
	}

	return pathsafe.Reason{}, true
}

// resolveLGHUBCacheCategory is the shared DryRun / ResolveCategory
// resolution seam. It never mutates. Every gate fails closed: any failure or
// uncertainty produces no candidate, and active or unknown LG HUB process/service
// state skips the entire category.
//
// Order: resolve fixed root → pre activity gate → discover exact candidates →
// post activity gate → measure candidates.
func resolveLGHUBCacheCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || category != CategoryLGHUBCache {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionLGHUBCacheDeps(opts.LGHUBCacheDiscoveryOptions)

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
		core.Diagnostics = append(core.Diagnostics, issue("lghub_cache_root_unreadable",
			"LGHUB cache root could not be inspected; LG HUB cache was skipped", true, "", category))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.IsDir() {
		core.Diagnostics = append(core.Diagnostics, issue("lghub_cache_root_unreadable",
			"LGHUB cache root is not an ordinary directory; LG HUB cache was skipped", true, "", category))
		return
	}
	// Protected root suppresses path-backed discovery before totals.
	if opts.Validator.IsUserProtected(root) {
		core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root)
		return
	}

	// Pre activity gate: active or unknown LG HUB state skips the whole category.
	if !lghubActivityIdleGate(ctx, deps, root, category, core) {
		return
	}

	// Discover exact candidates: ordinary files with exactly 64 lowercase hex
	// names as direct children only.
	entries, err := deps.readDir(root)
	if err != nil {
		core.Diagnostics = append(core.Diagnostics, issue("lghub_cache_root_unreadable",
			"LGHUB cache root could not be listed; LG HUB cache was skipped", true, "", category))
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

		// Skip directories, reparse points, and non-conforming filenames
		if !is64LowerHexName(name) {
			continue
		}

		path := deps.joinPath(root, name)

		// Must be a direct child only (reject path tricks)
		if !isDirectChildPath(root, path) {
			continue
		}

		// Must be an ordinary non-reparse file
		fileInfo, statErr := deps.lstat(path)
		if statErr != nil {
			continue
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || fileInfo.Mode()&os.ModeIrregular != 0 || !fileInfo.Mode().IsRegular() {
			continue
		}

		// Skip if protected
		if opts.Validator.IsUserProtected(path) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, path)
			continue
		}

		measured = append(measured, measuredCandidate{path: path, bytes: fileInfo.Size()})
	}

	if len(measured) == 0 {
		return
	}

	// Post activity re-check: any active or unknown LG HUB state discards all
	// measured candidates for the whole category.
	if !lghubActivityIdleGate(ctx, deps, root, category, core) {
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

// lghubActivityIdleGate detects LG HUB activity once and returns true only when
// idle. Running or unknown state records a whole-category SkippedItem and
// projects the LG HUB running state. Clean never stops processes or services.
func lghubActivityIdleGate(ctx context.Context, deps lghubCacheDeps, root, category string, core *categoryCoreResult) bool {
	detect := deps.detectActivity
	if detect == nil {
		detect = productionDetectLGHUBActivity
	}
	activity := detect(ctx)
	switch activity.Status {
	case LGHUBActivityIdle:
		runningGateOutcome{runningStates: []RunningApplicationState{{
			Application: ApplicationLGHUB,
			State:       RunningApplicationStateIdle,
		}}}.apply(&core.RunningStates, nil)
		return true
	case LGHUBActivityRunning:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationLGHUB,
			State:       RunningApplicationStateRunning,
			Message:     activity.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue("lghub_activity",
				"LG HUB application or service is active; LG HUB cache was skipped",
				true, root, category),
		})
		return false
	default:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationLGHUB,
			State:       RunningApplicationStateUnknown,
			Message:     activity.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue("lghub_activity",
				"LG HUB process or service state could not be determined; LG HUB cache was skipped",
				true, root, category),
		})
		return false
	}
}

// is64LowerHexName reports whether name is exactly 64 lowercase hex characters.
func is64LowerHexName(name string) bool {
	if len(name) != 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
