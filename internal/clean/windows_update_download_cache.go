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

// CategoryWindowsUpdateDownloadCache is the canonical exact-selection-only,
// machine-wide opt-in category for stale direct children of the Windows Update
// download staging cache %SystemRoot%\SoftwareDistribution\Download. It is gated
// on the Windows Update service stack being observably idle (ADR 0030 / ADR
// 0033).
const CategoryWindowsUpdateDownloadCache = "windows-update-download-cache"

// ApplicationWindowsUpdate is the one logical service identity Foal reports for
// Windows Update service-stack activity. Clean never stops or reconfigures these
// services; active or unknown state skips the entire category.
const ApplicationWindowsUpdate = "windows-update"

// windowsUpdateDownloadCacheStabilityWindowDays is the minimum latest-observed-
// modification age (inclusive: exactly this many days qualifies) a direct child
// must reach before it can become a candidate. The window is long because a
// download payload's consumption state (pending vs applied) is not externally
// readable, so a long externally-observable quiet period stands in for it
// (thunder-update-download precedent). Unknown or future timestamps fail closed.
const windowsUpdateDownloadCacheStabilityWindowDays = 30

// windowsUpdateDownloadCacheOptInImpactNotice is the path-free machine-wide
// impact vocabulary shown when a candidate is present. It discloses machine-wide
// scope, that Windows re-downloads anything still needed, that the Windows Update
// services must be idle, that only the Recycle Bin is used, and that a
// non-elevated run reclaims only the user-deletable subset.
const windowsUpdateDownloadCacheOptInImpactNotice = "Opt-in Windows Update download cache cleanup moves stale update payloads from the shared %SystemRoot%\\SoftwareDistribution\\Download staging directory to the Recycle Bin. This affects all users of the machine, and Windows re-downloads anything it still needs. The Windows Update services must be idle; active or unknown state skips the entire category. Foal never elevates or stops services, so payloads owned by system services are skipped and only part of the directory is reclaimed. This is not permanent deletion and is not secure erasure."

// windowsUpdateServiceNames are the exact Windows service short names whose
// non-Stopped state marks the update stack active. Query failure is unknown.
// Foal only observes these read-only and never mutates their state.
var windowsUpdateServiceNames = []string{"wuauserv", "bits", "dosvc", "UsoSvc"}

// WindowsUpdateDownloadCacheDiscoveryOptions injects the root override,
// SystemRoot override, clock, and service-detection seams. Production leaves the
// zero value so the resolved %SystemRoot%\SoftwareDistribution\Download root,
// time.Now(), and the platform SCM service detector are used. Tests must use
// isolated roots and never read or mutate the real Windows tree or SCM.
type WindowsUpdateDownloadCacheDiscoveryOptions struct {
	// Root overrides the resolved %SystemRoot%\SoftwareDistribution\Download
	// discovery root. Test-only: production leaves it empty so the SystemRoot
	// environment resolution is used.
	Root string
	// SystemRoot overrides the SystemRoot environment value used for root
	// resolution. Test-only; production leaves it empty so os.Getenv("SystemRoot")
	// is read. Ignored when Root is set.
	SystemRoot string
	// Now overrides the current time for stability window calculations. Test-only;
	// production leaves it zero so time.Now() is used.
	Now time.Time
	// DetectServices reports Windows Update service-stack state. nil selects the
	// production platform detector (read-only SCM query). Non-Windows production
	// fails closed to Unknown so the whole category is skipped.
	DetectServices func(context.Context) WindowsUpdateServicesState
}

// WindowsUpdateServicesStatus reports whether the Windows Update service stack is
// idle, running, or of unknown/undetermined state. Unknown never means idle.
type WindowsUpdateServicesStatus string

const (
	WindowsUpdateServicesIdle    WindowsUpdateServicesStatus = "idle"
	WindowsUpdateServicesRunning WindowsUpdateServicesStatus = "running"
	WindowsUpdateServicesUnknown WindowsUpdateServicesStatus = "unknown"
)

// WindowsUpdateServicesState is one conservative read-only service observation.
// Message is optional path-free diagnostic text.
type WindowsUpdateServicesState struct {
	Status  WindowsUpdateServicesStatus
	Message string
}

type windowsUpdateDownloadCacheDeps struct {
	// root is the resolved %SystemRoot%\SoftwareDistribution\Download directory.
	// Empty means the root is not resolvable (silent absence).
	root           string
	now            time.Time
	lstat          func(string) (os.FileInfo, error)
	readDir        func(string) ([]fs.DirEntry, error)
	walkDir        func(string, fs.WalkDirFunc) error
	detectServices func(context.Context) WindowsUpdateServicesState
	joinPath       func(...string) string
}

// resolveWindowsUpdateDownloadCacheRootFromSystemRoot resolves the exact
// %SystemRoot%\SoftwareDistribution\Download directory from a SystemRoot value.
// It fails closed (ok=false) for blank, relative, UNC, or otherwise unusable
// values so those are silent absence.
func resolveWindowsUpdateDownloadCacheRootFromSystemRoot(systemRoot string) (string, bool) {
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
	return filepath.Join(filepath.Clean(trimmed), "SoftwareDistribution", "Download"), true
}

func productionWindowsUpdateDownloadCacheDeps(opts WindowsUpdateDownloadCacheDiscoveryOptions) windowsUpdateDownloadCacheDeps {
	deps := windowsUpdateDownloadCacheDeps{
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
		if root, ok := resolveWindowsUpdateDownloadCacheRootFromSystemRoot(systemRoot); ok {
			deps.root = root
		}
	}
	if !opts.Now.IsZero() {
		deps.now = opts.Now
	} else {
		deps.now = time.Now()
	}
	if opts.DetectServices != nil {
		deps.detectServices = opts.DetectServices
	} else {
		deps.detectServices = productionDetectWindowsUpdateServices
	}
	return deps
}

// categoryResolverWindowsUpdateDownloadCache is the dedicated private resolver
// kind for the windows-update-download-cache category. It is intentionally not a
// developer-cache or application-cache entry: the `all`, `dev-caches`,
// `app-caches`, and `cli-agents` tokens never select it, and TUI Select All
// excludes it.
const categoryResolverWindowsUpdateDownloadCache categoryResolverKind = "windows-update-download-cache"

type windowsUpdateDownloadCacheResolver struct{}

func (windowsUpdateDownloadCacheResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveWindowsUpdateDownloadCacheCategory(ctx, opts, category, core)
}

func windowsUpdateDownloadCacheCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return categoryCatalogEntry{
		definition:   definition,
		resolverKind: categoryResolverWindowsUpdateDownloadCache,
		resolver:     windowsUpdateDownloadCacheResolver{},
	}
}

func init() {
	registerCategoryIdentityValidator(CategoryWindowsUpdateDownloadCache, validateWindowsUpdateDownloadCacheIdentity)
}

// validateWindowsUpdateDownloadCacheIdentity is the action-neutral,
// category-owned immediate pre-mutation validator. Immediately before the
// Recycle Bin move it repeats the per-candidate proof against a fresh read: the
// root re-resolves, the path is a direct child of that root, is a non-reparse
// ordinary path, and is still at least the stability window old. It must not
// mutate, never expand candidates, and never repeats the category-wide service
// idle gate. Any changed, ambiguous, unreadable, or too-recent state returns
// ok=false so the candidate is skipped fail-closed; the move_to_recycle_bin
// action is preserved and never becomes permanent deletion.
func validateWindowsUpdateDownloadCacheIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	reject := func(message string) (pathsafe.Reason, bool) {
		return pathsafe.Reason{Code: "windows_update_download_cache_revalidation_failed", Message: message}, false
	}
	if candidate.Category != CategoryWindowsUpdateDownloadCache {
		return pathsafe.Reason{Code: "identity_mismatch", Message: "category identity does not match windows-update-download-cache"}, false
	}
	path := strings.TrimSpace(candidate.Path)
	if path == "" {
		return reject("Windows Update download cache candidate path is empty")
	}

	deps := productionWindowsUpdateDownloadCacheDeps(candidate.windowsUpdateDownloadCacheDiscovery)
	root := deps.root
	if strings.TrimSpace(root) == "" {
		return reject("Windows Update download cache root is no longer resolvable")
	}

	// Must be a direct child of the resolved root.
	if !isDirectChildPath(root, path) {
		return reject("Windows Update download cache candidate is not a direct child of the download root")
	}

	// Must not be a reparse point.
	info, err := deps.lstat(path)
	if err != nil {
		return reject("Windows Update download cache candidate is no longer readable")
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
		return reject("Windows Update download cache candidate is no longer an ordinary non-reparse path")
	}

	// Must still be past the stability window on a fresh inspection.
	inspection, inspectErr := inspectOpportunity(context.Background(), path, userTempDescendantLimit, deps.walkDir)
	if inspectErr != nil {
		return reject("Windows Update download cache candidate could not be re-inspected")
	}
	if int(deps.now.Sub(inspection.latestModifiedAt)/(24*time.Hour)) < windowsUpdateDownloadCacheStabilityWindowDays {
		return reject("Windows Update download cache candidate is no longer past the stability window")
	}

	return pathsafe.Reason{}, true
}

// resolveWindowsUpdateDownloadCacheCategory is the shared DryRun / ResolveCategory
// resolution seam. It must not mutate. Every gate fails closed: any failure or
// uncertainty produces no candidate, and active or unknown Windows Update service
// state skips the entire category. Access-denied enumeration of the root is a
// whole-category recoverable skip; per-item stat failures are per-item skips.
//
// Order: resolve %SystemRoot%\SoftwareDistribution\Download root → pre service
// gate → discover exact direct-child candidates (non-reparse, past the stability
// window) → post service gate → measure candidates.
func resolveWindowsUpdateDownloadCacheCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	if core == nil || category != CategoryWindowsUpdateDownloadCache {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionWindowsUpdateDownloadCacheDeps(opts.WindowsUpdateDownloadCacheDiscoveryOptions)

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
		core.Diagnostics = append(core.Diagnostics, issue("windows_update_download_cache_root_unreadable",
			"Windows Update download cache root could not be inspected; windows update download cache was skipped", true, "", category))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 || !info.IsDir() {
		core.Diagnostics = append(core.Diagnostics, issue("windows_update_download_cache_root_unreadable",
			"Windows Update download cache root is not an ordinary directory; windows update download cache was skipped", true, "", category))
		return
	}
	// Protected root suppresses path-backed discovery before totals.
	if opts.Validator.IsUserProtected(root) {
		core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root)
		return
	}

	// Pre service gate: active or unknown Windows Update service state skips the
	// whole category before any discovery.
	if !windowsUpdateServicesIdleGate(ctx, deps, root, category, core) {
		return
	}

	entries, err := deps.readDir(root)
	if err != nil {
		// Access-denied (or otherwise unreadable) enumeration skips the whole
		// category; it never elevates and never partially guesses.
		core.Diagnostics = append(core.Diagnostics, issue("windows_update_download_cache_root_unreadable",
			"Windows Update download cache root could not be listed; windows update download cache was skipped", true, "", category))
		return
	}

	// Deterministic order for contract stability.
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.SliceStable(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })

	type measuredCandidate struct {
		path  string
		bytes int64
	}
	var measured []measuredCandidate

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

		// Deep latest-write inspection: a directory child is one candidate covering
		// its subtree. Incomplete inspection (including a reparse point anywhere in
		// the subtree) disqualifies the child fail-closed.
		inspection, inspectErr := inspectOpportunity(ctx, path, userTempDescendantLimit, deps.walkDir)
		if inspectErr != nil {
			continue
		}

		// Require at least the stability window (inclusive). Future timestamps make
		// the difference negative, so they fail the window and are excluded.
		if int(deps.now.Sub(inspection.latestModifiedAt)/(24*time.Hour)) < windowsUpdateDownloadCacheStabilityWindowDays {
			continue
		}

		measured = append(measured, measuredCandidate{path: path, bytes: inspection.bytes})
	}

	if len(measured) == 0 {
		return
	}

	// Post service re-check: any active or unknown Windows Update service state
	// discards all measured candidates for the whole category.
	if !windowsUpdateServicesIdleGate(ctx, deps, root, category, core) {
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

// windowsUpdateServicesIdleGate detects Windows Update service state once and
// returns true only when idle. Running or unknown state records a whole-category
// SkippedItem with the stable, path-free reason windows_update_services_active
// and projects the running state. Clean never stops or reconfigures services.
func windowsUpdateServicesIdleGate(ctx context.Context, deps windowsUpdateDownloadCacheDeps, root, category string, core *categoryCoreResult) bool {
	detect := deps.detectServices
	if detect == nil {
		detect = productionDetectWindowsUpdateServices
	}
	services := detect(ctx)
	switch services.Status {
	case WindowsUpdateServicesIdle:
		runningGateOutcome{runningStates: []RunningApplicationState{{
			Application: ApplicationWindowsUpdate,
			State:       RunningApplicationStateIdle,
		}}}.apply(&core.RunningStates, nil)
		return true
	case WindowsUpdateServicesRunning:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationWindowsUpdate,
			State:       RunningApplicationStateRunning,
			Message:     services.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue("windows_update_services_active",
				"Windows Update services are active; windows update download cache was skipped",
				true, root, category),
		})
		return false
	default:
		core.RunningStates = mergeRunningApplicationStates(core.RunningStates, RunningApplicationState{
			Application: ApplicationWindowsUpdate,
			State:       RunningApplicationStateUnknown,
			Message:     services.Message,
		})
		core.Skipped = append(core.Skipped, SkippedItem{
			Path:          root,
			Bytes:         0,
			Rule:          category,
			PlannedAction: plannedActionForCategory(category),
			Reason: issue("windows_update_services_active",
				"Windows Update service state could not be determined; windows update download cache was skipped",
				true, root, category),
		})
		return false
	}
}
