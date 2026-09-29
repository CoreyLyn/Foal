package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// categoryResolverExactCandidates is the resolver kind for categories whose
// candidates are exact, product-owned paths (files or
// directories) found by a private discovery policy under resolved roots. Each
// root is gated on its product being idle before discovery and again after
// measurement, and every candidate is re-discovered immediately before
// mutation: a path is acted on only while the same policy still selects it.
const categoryResolverExactCandidates categoryResolverKind = "exact-candidates"

// ExactCandidateDiscoveryOptions injects environment, home, and clock seams for
// exact-candidate categories (unity-package-cache, espressif-tool-archives,
// vscode-outdated-extensions). Production leaves the zero value. Tests must use
// isolated roots and never read or mutate real tool data.
type ExactCandidateDiscoveryOptions struct {
	// LookupEnv overrides os.LookupEnv when non-nil.
	LookupEnv func(string) (string, bool)
	// UserHomeDir overrides os.UserHomeDir when non-nil.
	UserHomeDir func() (string, error)
	// Now overrides the clock for quiet-window policies. Zero means time.Now.
	Now time.Time
}

type exactCandidateDeps struct {
	lookupEnv   func(string) (string, bool)
	userHomeDir func() (string, error)
	now         func() time.Time
}

func productionExactCandidateDeps(opts ExactCandidateDiscoveryOptions) exactCandidateDeps {
	deps := exactCandidateDeps{lookupEnv: os.LookupEnv, userHomeDir: os.UserHomeDir, now: time.Now}
	if opts.LookupEnv != nil {
		deps.lookupEnv = opts.LookupEnv
	}
	if opts.UserHomeDir != nil {
		deps.userHomeDir = opts.UserHomeDir
	}
	if !opts.Now.IsZero() {
		now := opts.Now
		deps.now = func() time.Time { return now }
	}
	return deps
}

// envPath returns a trimmed, absolute, non-UNC environment path.
func (d exactCandidateDeps) envPath(name string) (string, bool) {
	value, ok := d.lookupEnv(name)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" || !filepath.IsAbs(value) || strings.HasPrefix(value, `\\`) {
		return "", false
	}
	return filepath.Clean(value), true
}

// userProfile returns the current user's profile directory (USERPROFILE, else
// the platform home directory).
func (d exactCandidateDeps) userProfile() (string, bool) {
	if profile, ok := d.envPath("USERPROFILE"); ok {
		return profile, true
	}
	home, err := d.userHomeDir()
	if err != nil || strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return "", false
	}
	return filepath.Clean(home), true
}

// exactCandidateRoot is one resolved root. application optionally scopes the
// idle gate to one product identity; empty uses the policy's applications.
type exactCandidateRoot struct {
	path        string
	application string
}

// exactCandidatePolicy is the private, catalog-owned policy of one
// exact-candidate category. discover must be deterministic, read-only, and
// fail closed: unknown layouts or unreadable metadata return no candidates.
type exactCandidatePolicy struct {
	resolveRoots func(exactCandidateDeps) []exactCandidateRoot
	discover     func(ctx context.Context, deps exactCandidateDeps, root string) []string
	// applications are the category-wide idle identities used when a root names
	// none. Empty means no process gate (shared-runtime policies).
	applications []string
	// requireIdleDetection makes a missing detector an unsafe state for
	// application-owned content rather than silently omitting its idle gate.
	requireIdleDetection bool
}

type exactCandidateResolver struct{}

func (exactCandidateResolver) resolve(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	resolveExactCandidateCategory(ctx, opts, category, core)
}

// exactCandidateCategoryEntry registers an exact-candidate category. Standard
// developer-tool categories join dev-caches; exact-only categories stay ungrouped.
func exactCandidateCategoryEntry(definition CleanupCategoryDefinition, policy exactCandidatePolicy) categoryCatalogEntry {
	definition = withSelectionGroup(definition, CategorySelectionGroupDevCaches)
	return categoryCatalogEntry{
		definition:          definition,
		resolverKind:        categoryResolverExactCandidates,
		resolver:            exactCandidateResolver{},
		runningApplications: append([]string(nil), policy.applications...),
		exactCandidates:     &policy,
	}
}

func init() {
	for _, entry := range canonicalCategoryEntries {
		if entry.resolverKind == categoryResolverExactCandidates {
			switch entry.definition.PlannedAction {
			case PlannedActionDeletePermanently:
				registerPermanentIdentityValidator(entry.definition.Identifier, validateExactCandidateIdentity)
			case PlannedActionMoveToRecycleBin:
				registerCategoryIdentityValidator(entry.definition.Identifier, validateExactCandidateRecycleIdentity)
			}
		}
	}
}

func exactCandidatePolicyFor(category string) (exactCandidatePolicy, bool) {
	entry, ok := canonicalCategoryEntry(category)
	if !ok || entry.resolverKind != categoryResolverExactCandidates || entry.exactCandidates == nil {
		return exactCandidatePolicy{}, false
	}
	return *entry.exactCandidates, true
}

func (root exactCandidateRoot) gateApplications(policy exactCandidatePolicy) []string {
	if root.application != "" {
		return []string{root.application}
	}
	return policy.applications
}

// resolveExactCandidateRoots normalizes and deduplicates policy roots.
func resolveExactCandidateRoots(policy exactCandidatePolicy, deps exactCandidateDeps) []exactCandidateRoot {
	seen := map[string]bool{}
	var roots []exactCandidateRoot
	for _, root := range policy.resolveRoots(deps) {
		if !filepath.IsAbs(root.path) || strings.HasPrefix(root.path, `\\`) {
			continue
		}
		root.path = filepath.Clean(root.path)
		identity := pathsafe.NormalizePathForIdentity(root.path)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		roots = append(roots, root)
	}
	return roots
}

// isRealDirectoryInfo reports a directory that is not a symlink, junction,
// mount point, or other reparse point.
func isRealDirectoryInfo(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

// isRealDirectory reports whether path is an existing real directory.
func isRealDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && isRealDirectoryInfo(info)
}

// isOrdinaryFileInfo reports a regular, non-reparse file.
func isOrdinaryFileInfo(info os.FileInfo) bool {
	return info.Mode().IsRegular()
}

// resolveExactCandidateCategory is the shared Dry-run / Execute / TUI
// resolution seam. Per root: missing is silent; unreadable is a diagnostic;
// protected suppresses the root; the product must be idle before discovery and
// again after measurement or the whole root is skipped.
func resolveExactCandidateCategory(ctx context.Context, opts Options, category string, core *categoryCoreResult) {
	policy, ok := exactCandidatePolicyFor(category)
	if !ok || core == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deps := productionExactCandidateDeps(opts.ExactCandidateDiscoveryOptions)
	planned := plannedActionForOpts(opts, category)

	for _, root := range resolveExactCandidateRoots(policy, deps) {
		if err := ctx.Err(); err != nil {
			core.Diagnostics = append(core.Diagnostics, issue("context_canceled", err.Error(), true, root.path, category))
			return
		}
		info, err := os.Lstat(root.path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			core.Diagnostics = append(core.Diagnostics, issue(classifyError(err), err.Error(), true, root.path, category))
			continue
		}
		if !isRealDirectoryInfo(info) {
			continue
		}
		if opts.Validator.IsUserProtected(root.path) {
			core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, root.path)
			continue
		}

		apps := root.gateApplications(policy)
		if policy.requireIdleDetection && len(apps) > 0 && opts.DetectRunningApplications == nil {
			core.Skipped = append(core.Skipped, SkippedItem{
				Path: root.path, Rule: category, PlannedAction: planned,
				Reason: issue(runningApplicationDetectionIssueCode, "application idle state is unavailable", true, root.path, category),
			})
			continue
		}
		gate := opts.DetectRunningApplications != nil && len(apps) > 0
		if gate && !exactCandidateAppsIdle(ctx, opts, apps, root.path, category, planned, core) {
			continue
		}

		type measured struct {
			path  string
			bytes int64
		}
		var found []measured
		for _, path := range policy.discover(ctx, deps, root.path) {
			if err := ctx.Err(); err != nil {
				core.Diagnostics = append(core.Diagnostics, issue("context_canceled", err.Error(), true, path, category))
				return
			}
			if !isStrictDescendantPath(root.path, path) {
				continue
			}
			if opts.Validator.IsUserProtected(path) {
				core.SuppressedProtectionPaths = append(core.SuppressedProtectionPaths, path)
				continue
			}
			bytes, ok := measureExactCandidate(ctx, category, path, core)
			if !ok || bytes <= 0 {
				continue
			}
			found = append(found, measured{path: path, bytes: bytes})
		}
		if len(found) == 0 {
			continue
		}
		if gate && !exactCandidateAppsIdle(ctx, opts, apps, root.path, category, planned, core) {
			continue
		}
		for _, candidate := range found {
			core.OptInCandidates = append(core.OptInCandidates, OptInCandidate{
				Path:          candidate.path,
				Bytes:         candidate.bytes,
				Category:      category,
				PlannedAction: planned,
			})
		}
	}
}

// exactCandidateAppsIdle observes the product identities and records a
// whole-root skip when any is running or unknown.
func exactCandidateAppsIdle(ctx context.Context, opts Options, apps []string, root, category, planned string, core *categoryCoreResult) bool {
	idle, projected, reason := observeDevCacheApps(apps, root, category, opts.DetectRunningApplications(ctx))
	runningGateOutcome{runningStates: projected}.apply(&core.RunningStates, nil)
	if idle {
		return true
	}
	if reason != nil {
		core.Skipped = append(core.Skipped, SkippedItem{Path: root, Rule: category, PlannedAction: planned, Reason: *reason})
	}
	return false
}

// measureExactCandidate measures one candidate: an ordinary file by size, a real
// directory by complete inspection. Incomplete inspection is a diagnostic and
// yields no candidate.
func measureExactCandidate(ctx context.Context, category, path string, core *categoryCoreResult) (int64, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			core.Diagnostics = append(core.Diagnostics, issue(classifyError(err), err.Error(), true, path, category))
		}
		return 0, false
	}
	switch {
	case isOrdinaryFileInfo(info):
		return info.Size(), true
	case isRealDirectoryInfo(info):
		inspection, err := inspectOpportunity(ctx, path, userTempDescendantLimit, filepath.WalkDir)
		if err != nil {
			core.Diagnostics = append(core.Diagnostics, incompleteInspection(category, path, classifyOpportunityInspectionError(err), err.Error()).Reason)
			return 0, false
		}
		return inspection.bytes, true
	default:
		return 0, false
	}
}

// validateExactCandidateIdentity is the category-owned permanent pre-mutation
// check: the candidate must still be an ordinary file or real directory that
// the same policy re-discovers under a freshly resolved root. It never mutates
// or expands candidates.
func validateExactCandidateIdentity(candidate PermanentIdentityCandidate) (pathsafe.Reason, bool) {
	return exactCandidateStillSelected(candidate.Category, candidate.Path, candidate.exactDiscovery)
}

func validateExactCandidateRecycleIdentity(candidate CategoryIdentityCandidate) (pathsafe.Reason, bool) {
	return exactCandidateStillSelected(candidate.Category, candidate.Path, candidate.exactDiscovery)
}

func exactCandidateStillSelected(category, selectedPath string, discovery ExactCandidateDiscoveryOptions) (pathsafe.Reason, bool) {
	mismatch := pathsafe.Reason{Code: "identity_mismatch", Message: "candidate is no longer selected by its category policy"}
	policy, ok := exactCandidatePolicyFor(category)
	if !ok || strings.TrimSpace(selectedPath) == "" {
		return mismatch, false
	}
	info, err := os.Lstat(selectedPath)
	if err != nil || !(isOrdinaryFileInfo(info) || isRealDirectoryInfo(info)) {
		return mismatch, false
	}
	deps := productionExactCandidateDeps(discovery)
	for _, root := range resolveExactCandidateRoots(policy, deps) {
		if !isStrictDescendantPath(root.path, selectedPath) || !isRealDirectory(root.path) {
			continue
		}
		for _, discoveredPath := range policy.discover(context.Background(), deps, root.path) {
			if pathIdentityEqual(discoveredPath, selectedPath) {
				return pathsafe.Reason{}, true
			}
		}
	}
	return mismatch, false
}
