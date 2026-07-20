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
	applicationCachePolicyVSCode         = "visual_studio_code"
	applicationCachePolicyCursor         = "cursor"
	applicationCachePolicyVSCodeInsiders = "visual_studio_code_insiders"
	applicationCachePolicyVSCodium       = "vscodium"
	applicationCachePolicyWindsurf       = "windsurf"
	applicationCachePolicyTrae           = "trae"
	applicationCachePolicyObsidian       = "obsidian"
	// CachedExtensionVSIXsRootName is the exact allowlisted relative root that
	// stores downloaded VSIX packages (not installed extensions).
	CachedExtensionVSIXsRootName = "CachedExtensionVSIXs"
)

// applicationCacheAllowlistedRelativeRoots is the fixed v1 regenerating-cache
// allowlist shared by registered Application cache editors. Each editor still
// has its own category, root base, and process identity.
var applicationCacheAllowlistedRelativeRoots = []string{
	"Cache",
	"CachedData",
	CachedExtensionVSIXsRootName,
	"Code Cache",
	"GPUCache",
	"DawnGraphiteCache",
	"DawnWebGPUCache",
}

// obsidianCacheAllowlistedRelativeRoots is the plain-Electron regenerating-cache
// allowlist for Obsidian. Single-segment roots only. It excludes CachedData and
// CachedExtensionVSIXs (no V8-code-cache or VSIX proof for a non-editor app) and
// every state/config/bundle directory: obsidian.json, the app .asar bundle, Local
// Storage, IndexedDB, Service Worker, and Preferences must never be candidates.
// DawnCache is included; Service Worker\CacheStorage is multi-segment and deferred.
var obsidianCacheAllowlistedRelativeRoots = []string{
	"Cache",
	"Code Cache",
	"GPUCache",
	"DawnCache",
	"DawnGraphiteCache",
	"DawnWebGPUCache",
}

// ApplicationCacheDiscoveryOptions configures idle Application cache discovery.
// Only the current user's standard Roaming AppData base is supported.
type ApplicationCacheDiscoveryOptions struct {
	// RoamingAppDataDir overrides %APPDATA% for tests. Empty uses the process
	// environment; blank/missing yields silent absence of all roots.
	RoamingAppDataDir string
	// stat stays inside the existing Application cache discovery seam so package
	// tests can keep root preflight deterministic without touching the real fs.
	stat func(string) (os.FileInfo, error)
}

// applicationCachePolicy is the private fixed-allowlist policy for one idle
// Application cache category. Paths and process identities never leave Clean.
type applicationCachePolicy struct {
	category           string
	application        string
	roamingAppDataPath []string
	relativeRoots      []string
}

// applicationCachePolicies is the private policy table. Each editor requires
// an explicit policy with its own category, application identity, and exact
// relative-root allowlist — never a user-data tree scan.
var applicationCachePolicies = map[string]applicationCachePolicy{
	applicationCachePolicyVSCode: {
		category:           OpportunityCategoryVSCodeCache,
		application:        ApplicationVisualStudioCode,
		roamingAppDataPath: []string{"Code"},
		relativeRoots:      append([]string(nil), applicationCacheAllowlistedRelativeRoots...),
	},
	applicationCachePolicyCursor: {
		category:           OpportunityCategoryCursorCache,
		application:        ApplicationCursor,
		roamingAppDataPath: []string{"Cursor"},
		relativeRoots:      append([]string(nil), applicationCacheAllowlistedRelativeRoots...),
	},
	// VS Code Insiders: side-by-side with Stable; isolated %APPDATA%\Code - Insiders
	// (Microsoft FAQ: Insiders installs side by side with isolated settings).
	applicationCachePolicyVSCodeInsiders: {
		category:           OpportunityCategoryVSCodeInsidersCache,
		application:        ApplicationVisualStudioCodeInsiders,
		roamingAppDataPath: []string{"Code - Insiders"},
		relativeRoots:      append([]string(nil), applicationCacheAllowlistedRelativeRoots...),
	},
	// VSCodium: open-source VS Code fork; BleachBit official cleaner confirms
	// %AppData%\VSCodium and VSCodium.exe on Windows.
	applicationCachePolicyVSCodium: {
		category:           OpportunityCategoryVSCodiumCache,
		application:        ApplicationVSCodium,
		roamingAppDataPath: []string{"VSCodium"},
		relativeRoots:      append([]string(nil), applicationCacheAllowlistedRelativeRoots...),
	},
	// Windsurf: VS Code-based AI editor; BleachBit official cleaner confirms
	// %AppData%\Windsurf and Windsurf.exe on Windows.
	applicationCachePolicyWindsurf: {
		category:           OpportunityCategoryWindsurfCache,
		application:        ApplicationWindsurf,
		roamingAppDataPath: []string{"Windsurf"},
		relativeRoots:      append([]string(nil), applicationCacheAllowlistedRelativeRoots...),
	},
	// Trae: VS Code fork; %APPDATA%\Trae holds the standard VS Code-family
	// regenerating-cache layout (including CachedExtensionVSIXs). Trae.exe is
	// the Windows launcher. Independent of the other editors.
	applicationCachePolicyTrae: {
		category:           OpportunityCategoryTraeCache,
		application:        ApplicationTrae,
		roamingAppDataPath: []string{"Trae"},
		relativeRoots:      append([]string(nil), applicationCacheAllowlistedRelativeRoots...),
	},
	// Obsidian: non-editor Electron note-taking app; %APPDATA%\obsidian holds a
	// plain-Electron regenerating-cache layout. Obsidian.exe is the Windows
	// launcher. Carries its own plain-Electron allowlist (no CachedData/
	// CachedExtensionVSIXs; single-segment roots only) so obsidian.json, the
	// .asar bundle, Local Storage, IndexedDB, Service Worker, and Preferences
	// are never candidates. Independent idle gate; never cross-authorizes an
	// editor or Trae.
	applicationCachePolicyObsidian: {
		category:           OpportunityCategoryObsidianCache,
		application:        ApplicationObsidian,
		roamingAppDataPath: []string{"obsidian"},
		relativeRoots:      append([]string(nil), obsidianCacheAllowlistedRelativeRoots...),
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
		stat:    applicationCacheStat(opts),
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

	preflight := preflightApplicationCacheRoot(opts, policy, validator, deps.stat)
	if preflight.absent {
		return applicationCacheDiscoveryResult{}
	}
	if len(preflight.suppressedProtectionPaths) > 0 {
		return applicationCacheDiscoveryResult{
			suppressedProtectionPaths: preflight.suppressedProtectionPaths,
		}
	}
	if preflight.err != nil {
		incomplete := incompleteInspection(policy.category, preflight.userDataRoot, classifyError(preflight.err), preflight.err.Error())
		return applicationCacheDiscoveryResult{incompletes: []IncompleteOpportunityInspection{incomplete}}
	}
	userDataRoot := preflight.userDataRoot

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

type applicationCacheRootPreflight struct {
	userDataRoot              string
	absent                    bool
	suppressedProtectionPaths []string
	err                       error
}

func preflightApplicationCacheRoot(
	opts ApplicationCacheDiscoveryOptions,
	policy applicationCachePolicy,
	validator pathsafe.Validator,
	stat func(string) (os.FileInfo, error),
) applicationCacheRootPreflight {
	roaming := applicationCacheRoamingAppDataDir(opts)
	if roaming == "" {
		return applicationCacheRootPreflight{absent: true}
	}
	userDataRoot := applicationCacheUserDataRoot(roaming, policy)
	// Only the user-data base itself suppresses every root. A rule on one
	// allowlisted child is handled per root so siblings stay independent.
	if validator.IsUserProtected(userDataRoot) {
		return applicationCacheRootPreflight{
			userDataRoot:              userDataRoot,
			suppressedProtectionPaths: applicationCacheProtectedRulePaths(userDataRoot, validator),
		}
	}
	if stat == nil {
		stat = os.Lstat
	}
	_, err := stat(userDataRoot)
	if errors.Is(err, fs.ErrNotExist) {
		return applicationCacheRootPreflight{userDataRoot: userDataRoot, absent: true}
	}
	return applicationCacheRootPreflight{userDataRoot: userDataRoot, err: err}
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
		return strings.TrimSpace(opts.RoamingAppDataDir)
	}
	return strings.TrimSpace(os.Getenv("APPDATA"))
}

func applicationCacheStat(opts ApplicationCacheDiscoveryOptions) func(string) (os.FileInfo, error) {
	if opts.stat != nil {
		return opts.stat
	}
	return os.Lstat
}

// isCachedExtensionVSIXsPath reports whether path is exactly an allowlisted
// CachedExtensionVSIXs root (for the VSIX re-download preview notice).
func isCachedExtensionVSIXsPath(path string) bool {
	return strings.EqualFold(filepath.Base(path), CachedExtensionVSIXsRootName)
}
