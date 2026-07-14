package clean

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

const (
	applicationCachePolicyVSCode = "visual_studio_code"
	// CachedExtensionVSIXsRootName is the exact allowlisted relative root that
	// stores downloaded VSIX packages (not installed extensions).
	CachedExtensionVSIXsRootName = "CachedExtensionVSIXs"
)

// ApplicationCacheDiscoveryOptions configures idle Application cache discovery.
// Only the current user's standard Roaming AppData base is supported.
type ApplicationCacheDiscoveryOptions struct {
	// RoamingAppDataDir overrides %APPDATA% for tests. Empty uses the process
	// environment; blank/missing yields silent absence of all roots.
	RoamingAppDataDir string
}

// applicationCachePolicy is the private fixed-allowlist policy for one idle
// Application cache category. Paths and process identities never leave Clean.
type applicationCachePolicy struct {
	category           string
	application        string
	roamingAppDataPath []string
	relativeRoots      []string
}

// applicationCachePolicies is the private policy table. Adding a second editor
// requires an explicit policy with its own category, application identity, and
// exact relative-root allowlist — never a user-data tree scan.
var applicationCachePolicies = map[string]applicationCachePolicy{
	applicationCachePolicyVSCode: {
		category:           OpportunityCategoryVSCodeCache,
		application:        ApplicationVisualStudioCode,
		roamingAppDataPath: []string{"Code"},
		relativeRoots: []string{
			"Cache",
			"CachedData",
			CachedExtensionVSIXsRootName,
			"Code Cache",
			"GPUCache",
			"DawnGraphiteCache",
			"DawnWebGPUCache",
		},
	},
}

// applicationCacheDiscoveryResult is the reusable per-application discovery
// outcome: independent per-root opportunities, incompletes, and protection
// suppressions. Callers apply pre/post idle gating around discovery.
type applicationCacheDiscoveryResult struct {
	opportunities             []Opportunity
	incompletes               []IncompleteOpportunityInspection
	suppressedProtectionPaths []string
	// canceled is true when context canceled mid-inspection. Measured sibling
	// roots are discarded so post-check cannot reauthorize a partial scan.
	canceled bool
}

// DiscoverApplicationCaches is the injectable seam for idle Application cache
// discovery. Production uses discoverApplicationCaches; tests may override
// Options.DiscoverApplicationCaches for deterministic app-data/fs cases.
type DiscoverApplicationCachesFunc func(
	ctx context.Context,
	policyID string,
	opts ApplicationCacheDiscoveryOptions,
	validator pathsafe.Validator,
) applicationCacheDiscoveryResult

func discoverApplicationCaches(
	ctx context.Context,
	policyID string,
	opts ApplicationCacheDiscoveryOptions,
	validator pathsafe.Validator,
) applicationCacheDiscoveryResult {
	return discoverApplicationCachesWithDeps(ctx, policyID, opts, validator, applicationCacheDiscoveryDependencies{
		stat:    os.Lstat,
		walkDir: filepath.WalkDir,
	})
}

type applicationCacheDiscoveryDependencies struct {
	stat    func(string) (os.FileInfo, error)
	walkDir func(string, fs.WalkDirFunc) error
}

func discoverApplicationCachesWithDeps(
	ctx context.Context,
	policyID string,
	opts ApplicationCacheDiscoveryOptions,
	validator pathsafe.Validator,
	deps applicationCacheDiscoveryDependencies,
) applicationCacheDiscoveryResult {
	if ctx == nil {
		ctx = context.Background()
	}
	policy, ok := applicationCachePolicies[policyID]
	if !ok {
		return applicationCacheDiscoveryResult{}
	}
	if deps.stat == nil {
		deps.stat = os.Lstat
	}
	if deps.walkDir == nil {
		deps.walkDir = filepath.WalkDir
	}

	roaming := opts.RoamingAppDataDir
	if roaming == "" {
		roaming = os.Getenv("APPDATA")
	}
	if strings.TrimSpace(roaming) == "" {
		return applicationCacheDiscoveryResult{}
	}

	userDataRoot := applicationCacheUserDataRoot(roaming, policy)
	// Only the user-data base itself suppresses every root. A rule on one
	// allowlisted child is handled per root so siblings stay independent.
	if validator.IsUserProtected(userDataRoot) {
		return applicationCacheDiscoveryResult{
			suppressedProtectionPaths: applicationCacheProtectedRulePaths(userDataRoot, validator),
		}
	}
	if _, err := deps.stat(userDataRoot); errors.Is(err, fs.ErrNotExist) {
		return applicationCacheDiscoveryResult{}
	} else if err != nil {
		incomplete := incompleteInspection(policy.category, userDataRoot, classifyError(err), err.Error())
		return applicationCacheDiscoveryResult{incompletes: []IncompleteOpportunityInspection{incomplete}}
	}

	result := applicationCacheDiscoveryResult{
		opportunities: []Opportunity{},
		incompletes:   []IncompleteOpportunityInspection{},
	}
	for _, relativeRoot := range policy.relativeRoots {
		select {
		case <-ctx.Done():
			result.canceled = true
			result.opportunities = nil
			result.incompletes = append(result.incompletes, incompleteInspection(
				policy.category, userDataRoot, "context_canceled", ctx.Err().Error(),
			))
			return result
		default:
		}

		rootPath := filepath.Join(userDataRoot, relativeRoot)
		if validator.IsUserProtected(rootPath) {
			result.suppressedProtectionPaths = append(
				result.suppressedProtectionPaths,
				applicationCacheProtectedRulePaths(rootPath, validator)...,
			)
			continue
		}
		if _, err := deps.stat(rootPath); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			result.incompletes = append(result.incompletes, incompleteInspection(
				policy.category, rootPath, classifyError(err), err.Error(),
			))
			continue
		}

		inspection, err := inspectOpportunity(ctx, rootPath, userTempDescendantLimit, deps.walkDir)
		if err != nil {
			result.incompletes = append(result.incompletes, incompleteInspection(
				policy.category, rootPath, classifyOpportunityInspectionError(err), err.Error(),
			))
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// Discard measured siblings: post-check must not reauthorize.
				result.canceled = true
				result.opportunities = nil
				return result
			}
			continue
		}
		result.opportunities = append(result.opportunities, Opportunity{
			Category: policy.category,
			Path:     rootPath,
			Bytes:    inspection.bytes,
			Status:   OpportunityStatus,
			Reason:   OpportunityReason,
		})
	}
	return result
}

func applicationCacheUserDataRoot(roamingAppDataDir string, policy applicationCachePolicy) string {
	parts := append([]string{roamingAppDataDir}, policy.roamingAppDataPath...)
	return filepath.Join(parts...)
}

func applicationCacheProtectedRulePaths(target string, validator pathsafe.Validator) []string {
	root := filepath.Clean(target)
	var paths []string
	for _, path := range validator.UserProtectionPaths() {
		cleanPath := filepath.Clean(path)
		// Rules that are the target or an ancestor that covers it, or rules
		// registered exactly for a descendant under the target when reporting
		// which rule paths participated in suppression.
		if sameOrDescendantCaseInsensitive(root, cleanPath) || sameOrDescendantCaseInsensitive(cleanPath, root) {
			paths = append(paths, path)
		}
	}
	return paths
}

func applicationCachePolicyForCategory(category string) (string, applicationCachePolicy, bool) {
	entry, ok := applicationCacheEntry(category)
	if !ok {
		return "", applicationCachePolicy{}, false
	}
	policy, ok := applicationCachePolicies[entry.applicationCachePolicyID]
	if !ok {
		return "", applicationCachePolicy{}, false
	}
	return entry.applicationCachePolicyID, policy, true
}

func applicationCacheRoamingAppDataDir(opts ApplicationCacheDiscoveryOptions) string {
	if opts.RoamingAppDataDir != "" {
		return opts.RoamingAppDataDir
	}
	return os.Getenv("APPDATA")
}

// isCachedExtensionVSIXsPath reports whether path is exactly an allowlisted
// CachedExtensionVSIXs root (for the VSIX re-download preview notice).
func isCachedExtensionVSIXsPath(path string) bool {
	return strings.EqualFold(filepath.Base(path), CachedExtensionVSIXsRootName)
}
