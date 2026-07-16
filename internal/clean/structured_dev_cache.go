package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// DevCacheChildDiscoverer is the injectable seam for structured developer-cache
// child discovery under one resolved root. When non-nil, the function decides
// whether the category uses structured mode for that root:
//   - structured=false → whole-root candidate behavior (catalog default)
//   - structured=true  → only returned children may become Opt-in candidates;
//     empty children means no candidates (fail closed)
//
// Production categories register discoverChildren on the private catalog entry;
// tests inject this hook to exercise safety without real user caches or new
// public categories. Shared resolution always re-validates children.
type DevCacheChildDiscoverer func(ctx context.Context, category, root string) (children []string, structured bool)

// resolveDevCacheRootScopes returns product-aware root scopes for a category.
// Prefer Options.DevCacheRootScopeResolver when set; otherwise path resolvers
// produce scopes with empty Application (category-wide gating).
func resolveDevCacheRootScopes(opts Options, category string) []DevCacheRootScope {
	if opts.DevCacheRootScopeResolver != nil {
		return normalizeDevCacheRootScopes(opts.DevCacheRootScopeResolver(category))
	}
	entry, ok := canonicalCategoryEntry(category)
	if ok && entry.developerCache && entry.resolveRootScopes != nil {
		return normalizeDevCacheRootScopes(entry.resolveRootScopes(devCachePathDependencies{
			lookupEnv:   os.LookupEnv,
			userHomeDir: os.UserHomeDir,
			joinPath:    filepath.Join,
			goos:        runtime.GOOS,
		}))
	}
	resolveDevCache := opts.DevCachePathResolver
	if resolveDevCache == nil {
		resolveDevCache = ResolveDevCachePaths
	}
	paths := normalizeAndDeduplicatePaths(resolveDevCache(category))
	scopes := make([]DevCacheRootScope, 0, len(paths))
	for _, path := range paths {
		scopes = append(scopes, DevCacheRootScope{Path: path})
	}
	return scopes
}

// normalizeDevCacheRootScopes drops empty paths, trims application identities,
// and deduplicates by Windows path identity while preserving first-seen order
// and the first-seen application association for each path.
func normalizeDevCacheRootScopes(scopes []DevCacheRootScope) []DevCacheRootScope {
	seen := make(map[string]bool, len(scopes))
	result := make([]DevCacheRootScope, 0, len(scopes))
	for _, scope := range scopes {
		if pathsafe.IsEmptyOrWhitespacePath(scope.Path) {
			continue
		}
		path := filepath.Clean(scope.Path)
		identity := pathsafe.NormalizePathForIdentity(path)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		result = append(result, DevCacheRootScope{
			Path:        path,
			Application: strings.TrimSpace(scope.Application),
		})
	}
	return result
}

// resolveDevCacheChildCandidates returns structured children when the category
// has a catalog-owned policy or the injectable seam forces structured mode.
// When structured is false, callers treat the resolved root as a single candidate.
func resolveDevCacheChildCandidates(ctx context.Context, opts Options, category, root string) (children []string, structured bool) {
	if opts.DevCacheChildDiscoverer != nil {
		return opts.DevCacheChildDiscoverer(ctx, category, root)
	}
	entry, ok := canonicalCategoryEntry(category)
	if !ok || !entry.developerCache || entry.discoverChildren == nil {
		return nil, false
	}
	return entry.discoverChildren(ctx, root), true
}

// categoryHasStructuredDevCacheDiscovery reports whether the canonical catalog
// privately registers a child discovery policy for the category.
func categoryHasStructuredDevCacheDiscovery(category string) bool {
	entry, ok := canonicalCategoryEntry(category)
	return ok && entry.developerCache && entry.discoverChildren != nil
}

// isStrictDescendantPath reports whether path is a strict descendant of root
// under Windows path identity (case-insensitive, cleaned). The root itself is
// never a strict descendant.
func isStrictDescendantPath(root, path string) bool {
	rootID := pathsafe.NormalizePathForIdentity(root)
	pathID := pathsafe.NormalizePathForIdentity(path)
	if rootID == "" || pathID == "" || rootID == pathID {
		return false
	}
	return strings.HasPrefix(pathID, rootID+string(filepath.Separator))
}

// structuredDevCacheChildSafety is the shared fail-closed gate for one child
// path before measurement. Rejects the root itself, paths outside the root,
// regular files, and symlink/reparse candidates.
type structuredDevCacheChildSafety struct {
	// ok is true when the path may be measured as a structured candidate.
	ok bool
	// missing is true when Lstat reported not-exist (silent absence).
	missing bool
	// rejectReason is set for unreadable paths that should surface a diagnostic.
	rejectReason *StructuredIssue
}

func evaluateStructuredDevCacheChild(category, root, child string, lstat func(string) (os.FileInfo, error)) structuredDevCacheChildSafety {
	if pathsafe.IsEmptyOrWhitespacePath(child) {
		return structuredDevCacheChildSafety{}
	}
	if !isStrictDescendantPath(root, child) {
		return structuredDevCacheChildSafety{}
	}
	if lstat == nil {
		lstat = os.Lstat
	}
	info, err := lstat(child)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return structuredDevCacheChildSafety{missing: true}
		}
		reason := incompleteInspection(category, child, classifyError(err), err.Error()).Reason
		return structuredDevCacheChildSafety{rejectReason: &reason}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return structuredDevCacheChildSafety{}
	}
	if !info.IsDir() {
		return structuredDevCacheChildSafety{}
	}
	return structuredDevCacheChildSafety{ok: true}
}

// appendStructuredDevCacheCandidates validates, protects, and measures each
// structured child under root. Incomplete, unreadable, over-ceiling, or canceled
// children contribute zero bytes and do not become candidates; complete siblings
// may remain. Protection is applied per child and never authorizes siblings.
func appendStructuredDevCacheCandidates(
	ctx context.Context,
	opts Options,
	res *optInResolution,
	category, root string,
	rawChildren []string,
	deps structuredDevCacheMeasureDependencies,
) {
	if deps.lstat == nil {
		deps.lstat = os.Lstat
	}
	if deps.walkDir == nil {
		deps.walkDir = filepath.WalkDir
	}
	if deps.descendantLimit <= 0 {
		deps.descendantLimit = userTempDescendantLimit
	}

	children := normalizeAndDeduplicatePaths(rawChildren)
	for _, child := range children {
		select {
		case <-ctx.Done():
			res.diagnostics = append(res.diagnostics, issue("context_canceled", ctx.Err().Error(), true, child, category))
			return
		default:
		}

		safety := evaluateStructuredDevCacheChild(category, root, child, deps.lstat)
		if safety.missing || (!safety.ok && safety.rejectReason == nil) {
			continue
		}
		if safety.rejectReason != nil {
			res.diagnostics = append(res.diagnostics, *safety.rejectReason)
			continue
		}

		if opts.Validator.IsUserProtected(child) {
			res.suppressedProtectionPaths = append(
				res.suppressedProtectionPaths,
				structuredDevCacheProtectedRulePaths(child, opts.Validator)...,
			)
			continue
		}

		inspection, err := inspectOpportunity(ctx, child, deps.descendantLimit, deps.walkDir)
		if err != nil {
			res.diagnostics = append(res.diagnostics, incompleteInspection(
				category, child, classifyOpportunityInspectionError(err), err.Error(),
			).Reason)
			// Sibling independence: incomplete/canceled child contributes zero
			// bytes; already-measured complete siblings stay. Further children
			// still attempt measurement unless the context is done (loop select).
			continue
		}

		res.candidates = append(res.candidates, OptInCandidate{
			Path:          child,
			Bytes:         inspection.bytes,
			Category:      category,
			PlannedAction: plannedRecycleBinAction,
		})
	}
}

type structuredDevCacheMeasureDependencies struct {
	lstat           func(string) (os.FileInfo, error)
	walkDir         func(string, fs.WalkDirFunc) error
	descendantLimit int
}

func structuredDevCacheProtectedRulePaths(target string, validator pathsafe.Validator) []string {
	root := filepath.Clean(target)
	var paths []string
	for _, path := range validator.UserProtectionPaths() {
		cleanPath := filepath.Clean(path)
		if sameOrDescendantCaseInsensitive(root, cleanPath) || sameOrDescendantCaseInsensitive(cleanPath, root) {
			paths = append(paths, path)
		}
	}
	return paths
}
