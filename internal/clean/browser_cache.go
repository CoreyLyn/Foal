package clean

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

const (
	browserCacheRuleID = "browser_cache"
)

var browserCacheDirectoryKinds = []string{"Cache", "Code Cache", "GPUCache"}

type browserCacheConfig struct {
	application      string
	localAppDataPath []string
}

var browserCacheConfigs = []browserCacheConfig{
	{
		application:      ApplicationGoogleChrome,
		localAppDataPath: []string{"Google", "Chrome", "User Data"},
	},
	{
		application:      ApplicationMicrosoftEdge,
		localAppDataPath: []string{"Microsoft", "Edge", "User Data"},
	},
}

type browserCacheDiscoveryResult struct {
	opportunity               *Opportunity
	diagnostic                *StructuredIssue
	incomplete                *IncompleteOpportunityInspection
	suppressed                bool
	suppressedProtectionPaths []string
}

type browserLocalState struct {
	Profile struct {
		InfoCache map[string]browserProfileInfo `json:"info_cache"`
	} `json:"profile"`
}

type browserProfileInfo struct {
	Name string `json:"name"`
}

func discoverChromeBrowserCache(ctx context.Context, opts BrowserCacheDiscoveryOptions, validator pathsafe.Validator) browserCacheDiscoveryResult {
	return discoverBrowserCache(ctx, browserCacheConfigs[0], opts, validator)
}

func discoverBrowserCache(ctx context.Context, config browserCacheConfig, opts BrowserCacheDiscoveryOptions, validator pathsafe.Validator) browserCacheDiscoveryResult {
	if ctx == nil {
		ctx = context.Background()
	}
	localAppDataDir := opts.LocalAppDataDir
	if localAppDataDir == "" {
		localAppDataDir = os.Getenv("LOCALAPPDATA")
	}
	if localAppDataDir == "" {
		return browserCacheDiscoveryResult{}
	}
	userDataRoot := browserUserDataRoot(localAppDataDir, config)
	suppressed, protectedRulePaths := browserDiscoverySuppressed(userDataRoot, validator)
	if suppressed {
		return browserCacheDiscoveryResult{suppressed: true, suppressedProtectionPaths: protectedRulePaths}
	}
	if _, err := os.Lstat(userDataRoot); errors.Is(err, fs.ErrNotExist) {
		return browserCacheDiscoveryResult{}
	} else if err != nil {
		diagnostic := browserCacheDiagnostic(userDataRoot, classifyError(err), err.Error())
		return browserCacheDiscoveryResult{diagnostic: &diagnostic}
	}

	profiles, err := readBrowserProfileCatalog(userDataRoot, config)
	if err != nil {
		diagnostic := browserCacheDiagnostic(userDataRoot, "browser_profile_catalog_unknown", err.Error())
		return browserCacheDiscoveryResult{diagnostic: &diagnostic}
	}
	if len(profiles) == 0 {
		return browserCacheDiscoveryResult{}
	}

	detail := BrowserCacheOpportunityDetail{
		Browser:      config.application,
		ProfileCount: len(profiles),
		Profiles:     make([]BrowserCacheProfileDetail, 0, len(profiles)),
	}
	var total int64
	for _, profile := range profiles {
		profilePath := filepath.Join(userDataRoot, profile.id)
		if validator.IsUserProtected(profilePath) {
			return browserCacheDiscoveryResult{suppressed: true, suppressedProtectionPaths: browserProtectedRulePaths(userDataRoot, validator)}
		}
		profileDetail := BrowserCacheProfileDetail{
			ID:     profile.id,
			Name:   profile.name,
			Path:   profilePath,
			Caches: make([]BrowserCacheDirectory, 0, len(browserCacheDirectoryKinds)),
		}
		for _, kind := range browserCacheDirectoryKinds {
			cachePath := filepath.Join(profilePath, kind)
			if validator.IsUserProtected(cachePath) {
				return browserCacheDiscoveryResult{suppressed: true, suppressedProtectionPaths: browserProtectedRulePaths(userDataRoot, validator)}
			}
			if _, err := os.Lstat(cachePath); errors.Is(err, fs.ErrNotExist) {
				profileDetail.Caches = append(profileDetail.Caches, BrowserCacheDirectory{Kind: kind, Path: cachePath})
				continue
			} else if err != nil {
				incomplete := incompleteInspection(OpportunityCategoryBrowserCache, cachePath, classifyError(err), err.Error())
				return browserCacheDiscoveryResult{incomplete: &incomplete}
			}
			inspection, err := inspectOpportunity(ctx, cachePath, userTempDescendantLimit, filepath.WalkDir)
			if err != nil {
				incomplete := incompleteInspection(OpportunityCategoryBrowserCache, cachePath, classifyOpportunityInspectionError(err), err.Error())
				return browserCacheDiscoveryResult{incomplete: &incomplete}
			}
			total += inspection.bytes
			profileDetail.Caches = append(profileDetail.Caches, BrowserCacheDirectory{
				Kind:  kind,
				Path:  cachePath,
				Bytes: inspection.bytes,
			})
		}
		detail.Profiles = append(detail.Profiles, profileDetail)
	}
	if total == 0 {
		return browserCacheDiscoveryResult{}
	}
	return browserCacheDiscoveryResult{opportunity: &Opportunity{
		Category:     OpportunityCategoryBrowserCache,
		Path:         userDataRoot,
		Bytes:        total,
		Status:       OpportunityStatus,
		Reason:       OpportunityReason,
		BrowserCache: &detail,
	}}
}

type chromeProfileCatalogEntry struct {
	id   string
	name string
}

func readChromeProfileCatalog(userDataRoot string) ([]chromeProfileCatalogEntry, error) {
	return readBrowserProfileCatalog(userDataRoot, browserCacheConfigs[0])
}

func readBrowserProfileCatalog(userDataRoot string, config browserCacheConfig) ([]chromeProfileCatalogEntry, error) {
	data, err := os.ReadFile(filepath.Join(userDataRoot, "Local State"))
	if err != nil {
		return nil, err
	}
	var localState browserLocalState
	if err := json.Unmarshal(data, &localState); err != nil {
		return nil, fmt.Errorf("%s Local State profile catalog is invalid: %w", applicationDisplayName(config.application), err)
	}
	if localState.Profile.InfoCache == nil {
		return nil, fmt.Errorf("%s Local State profile catalog is missing", applicationDisplayName(config.application))
	}
	profiles := make([]chromeProfileCatalogEntry, 0, len(localState.Profile.InfoCache))
	for id, profile := range localState.Profile.InfoCache {
		if isExcludedBrowserProfileID(id) {
			continue
		}
		profiles = append(profiles, chromeProfileCatalogEntry{id: id, name: profile.Name})
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].id < profiles[j].id
	})
	return profiles, nil
}

func isExcludedChromeProfileID(id string) bool {
	return isExcludedBrowserProfileID(id)
}

func isExcludedBrowserProfileID(id string) bool {
	return id == "Guest Profile" || id == "System Profile"
}

func chromeUserDataRoot(localAppDataDir string) string {
	return browserUserDataRoot(localAppDataDir, browserCacheConfigs[0])
}

func browserUserDataRoot(localAppDataDir string, config browserCacheConfig) string {
	pathParts := append([]string{localAppDataDir}, config.localAppDataPath...)
	return filepath.Join(pathParts...)
}

func browserCacheDiagnostic(path, code, message string) StructuredIssue {
	return issue(code, message, true, path, browserCacheRuleID)
}

func chromeDiscoverySuppressed(userDataRoot string, validator pathsafe.Validator) (bool, []string) {
	return browserDiscoverySuppressed(userDataRoot, validator)
}

func browserDiscoverySuppressed(userDataRoot string, validator pathsafe.Validator) (bool, []string) {
	protectedRulePaths := browserProtectedRulePaths(userDataRoot, validator)
	return len(protectedRulePaths) > 0 || validator.IsUserProtected(userDataRoot), protectedRulePaths
}

func chromeProtectedRulePaths(userDataRoot string, validator pathsafe.Validator) []string {
	return browserProtectedRulePaths(userDataRoot, validator)
}

func browserProtectedRulePaths(userDataRoot string, validator pathsafe.Validator) []string {
	root := filepath.Clean(userDataRoot)
	var paths []string
	for _, path := range validator.UserProtectionPaths() {
		cleanPath := filepath.Clean(path)
		if sameOrDescendantCaseInsensitive(cleanPath, root) {
			paths = append(paths, path)
		}
	}
	return paths
}

func sameOrDescendantCaseInsensitive(path, root string) bool {
	path = strings.ToLower(filepath.Clean(path))
	root = strings.ToLower(filepath.Clean(root))
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}
