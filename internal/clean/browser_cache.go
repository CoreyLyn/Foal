package clean

import (
	"bufio"
	"bytes"
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
	browserCacheRuleID = OpportunityCategoryBrowserCache
)

type browserProfileCatalogKind int

const (
	browserProfileCatalogLocalState browserProfileCatalogKind = iota
	browserProfileCatalogProfilesINI
)

type browserCacheConfig struct {
	application         string
	localAppDataPath    []string
	roamingAppDataPath  []string
	catalogKind         browserProfileCatalogKind
	cacheDirectoryKinds []string
}

// chromiumBrowserCacheDirectoryKinds is the exact allowlist of regenerable
// profile-relative cache roots for Chrome and Edge. Nested Service Worker
// CacheStorage is the Cache Storage API backend only; ScriptCache and Database
// siblings stay excluded (docs/research/chromium-service-worker-cachestorage.md).
var chromiumBrowserCacheDirectoryKinds = []string{
	"Cache",
	"Code Cache",
	"GPUCache",
	filepath.Join("Service Worker", "CacheStorage"),
}

// browserCacheConfigs registers every supported browser under the single
// browser_cache category. Chromium entries use Local State under Local AppData
// User Data. Firefox uses profiles.ini under Roaming and regenerable cache2
// under the matching Local profile tree.
var browserCacheConfigs = []browserCacheConfig{
	{
		application:         ApplicationGoogleChrome,
		localAppDataPath:    []string{"Google", "Chrome", "User Data"},
		catalogKind:         browserProfileCatalogLocalState,
		cacheDirectoryKinds: chromiumBrowserCacheDirectoryKinds,
	},
	{
		application:         ApplicationMicrosoftEdge,
		localAppDataPath:    []string{"Microsoft", "Edge", "User Data"},
		catalogKind:         browserProfileCatalogLocalState,
		cacheDirectoryKinds: chromiumBrowserCacheDirectoryKinds,
	},
	{
		application:         ApplicationMozillaFirefox,
		localAppDataPath:    []string{"Mozilla", "Firefox"},
		roamingAppDataPath:  []string{"Mozilla", "Firefox"},
		catalogKind:         browserProfileCatalogProfilesINI,
		cacheDirectoryKinds: []string{"cache2"},
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
	if config.catalogKind == browserProfileCatalogProfilesINI {
		return discoverFirefoxBrowserCache(ctx, config, opts, validator)
	}
	return discoverChromiumBrowserCache(ctx, config, opts, validator)
}

func discoverChromiumBrowserCache(ctx context.Context, config browserCacheConfig, opts BrowserCacheDiscoveryOptions, validator pathsafe.Validator) browserCacheDiscoveryResult {
	if ctx == nil {
		ctx = context.Background()
	}
	localAppDataDir := browserCacheLocalAppDataDir(opts)
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

	return inspectBrowserProfiles(ctx, config, userDataRoot, userDataRoot, profiles, validator, func(profile browserProfileCatalogEntry) string {
		dir := profile.id
		if profile.relativePath != "" {
			dir = profile.relativePath
		}
		return filepath.Join(userDataRoot, filepath.FromSlash(dir))
	})
}

func discoverFirefoxBrowserCache(ctx context.Context, config browserCacheConfig, opts BrowserCacheDiscoveryOptions, validator pathsafe.Validator) browserCacheDiscoveryResult {
	if ctx == nil {
		ctx = context.Background()
	}
	roamingAppDataDir := browserCacheRoamingAppDataDir(opts)
	localAppDataDir := browserCacheLocalAppDataDir(opts)
	if roamingAppDataDir == "" || localAppDataDir == "" {
		return browserCacheDiscoveryResult{}
	}
	catalogRoot := browserRoamingRoot(roamingAppDataDir, config)
	localRoot := browserUserDataRoot(localAppDataDir, config)

	for _, root := range []string{catalogRoot, localRoot} {
		suppressed, protectedRulePaths := browserDiscoverySuppressed(root, validator)
		if suppressed {
			return browserCacheDiscoveryResult{suppressed: true, suppressedProtectionPaths: protectedRulePaths}
		}
	}

	if _, err := os.Lstat(catalogRoot); errors.Is(err, fs.ErrNotExist) {
		return browserCacheDiscoveryResult{}
	} else if err != nil {
		diagnostic := browserCacheDiagnostic(catalogRoot, classifyError(err), err.Error())
		return browserCacheDiscoveryResult{diagnostic: &diagnostic}
	}

	profiles, err := readFirefoxProfilesINI(catalogRoot)
	if err != nil {
		diagnostic := browserCacheDiagnostic(catalogRoot, "browser_profile_catalog_unknown", err.Error())
		return browserCacheDiscoveryResult{diagnostic: &diagnostic}
	}
	if len(profiles) == 0 {
		return browserCacheDiscoveryResult{}
	}

	// Opportunity path is the Local Firefox root where regenerable caches live.
	return inspectBrowserProfiles(ctx, config, localRoot, localRoot, profiles, validator, func(profile browserProfileCatalogEntry) string {
		dir := profile.id
		if profile.relativePath != "" {
			dir = profile.relativePath
		}
		return filepath.Join(localRoot, filepath.FromSlash(dir))
	})
}

// inspectBrowserProfiles measures allowlisted cache directories under each
// resolved profile path. One incomplete or protected path discards the whole
// browser summary (complete-or-discard).
func inspectBrowserProfiles(
	ctx context.Context,
	config browserCacheConfig,
	opportunityPath string,
	protectionRoot string,
	profiles []browserProfileCatalogEntry,
	validator pathsafe.Validator,
	profilePath func(browserProfileCatalogEntry) string,
) browserCacheDiscoveryResult {
	detail := BrowserCacheOpportunityDetail{
		Browser:      config.application,
		ProfileCount: len(profiles),
		Profiles:     make([]BrowserCacheProfileDetail, 0, len(profiles)),
	}
	var total int64
	for _, profile := range profiles {
		resolvedProfilePath := profilePath(profile)
		if validator.IsUserProtected(resolvedProfilePath) {
			return browserCacheDiscoveryResult{suppressed: true, suppressedProtectionPaths: browserProtectedRulePaths(protectionRoot, validator)}
		}
		profileDetail := BrowserCacheProfileDetail{
			ID:     profile.id,
			Name:   profile.name,
			Path:   resolvedProfilePath,
			Caches: make([]BrowserCacheDirectory, 0, len(config.cacheDirectoryKinds)),
		}
		for _, kind := range config.cacheDirectoryKinds {
			cachePath := filepath.Join(resolvedProfilePath, kind)
			if validator.IsUserProtected(cachePath) {
				return browserCacheDiscoveryResult{suppressed: true, suppressedProtectionPaths: browserProtectedRulePaths(protectionRoot, validator)}
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
		Path:         opportunityPath,
		Bytes:        total,
		Status:       OpportunityStatus,
		Reason:       OpportunityReason,
		BrowserCache: &detail,
	}}
}

type browserProfileCatalogEntry struct {
	// id is the stable profile identity reported in JSON (Chromium Local State
	// key, or Firefox profiles.ini relative Path).
	id   string
	name string
	// relativePath is the directory under the browser root used to resolve the
	// profile on disk. Empty means id itself is the directory name (Chromium).
	relativePath string
}

// chromeProfileCatalogEntry is retained for older chrome-named call sites.
type chromeProfileCatalogEntry = browserProfileCatalogEntry

func readChromeProfileCatalog(userDataRoot string) ([]browserProfileCatalogEntry, error) {
	return readBrowserProfileCatalog(userDataRoot, browserCacheConfigs[0])
}

func readBrowserProfileCatalog(userDataRoot string, config browserCacheConfig) ([]browserProfileCatalogEntry, error) {
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
	profiles := make([]browserProfileCatalogEntry, 0, len(localState.Profile.InfoCache))
	for id, profile := range localState.Profile.InfoCache {
		if isExcludedBrowserProfileID(id) {
			continue
		}
		profiles = append(profiles, browserProfileCatalogEntry{id: id, name: profile.Name})
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].id < profiles[j].id
	})
	return profiles, nil
}

// readFirefoxProfilesINI enumerates ordinary relative profiles from Mozilla's
// profiles.ini. Absolute or path-escaping Path values are ignored (fail-closed
// catalog evidence only). A missing or unreadable file is an error so callers
// can emit browser_profile_catalog_unknown rather than guess profile folders.
// When the catalog exists and every Profile* entry is absolute/out-of-scope,
// return an error so callers emit a path-free recoverable diagnostic instead of
// silent absence.
func readFirefoxProfilesINI(catalogRoot string) ([]browserProfileCatalogEntry, error) {
	data, err := os.ReadFile(filepath.Join(catalogRoot, "profiles.ini"))
	if err != nil {
		return nil, err
	}
	profiles, err := parseFirefoxProfilesINI(data)
	if err != nil {
		return nil, fmt.Errorf("%s profiles.ini catalog is invalid: %w", applicationDisplayName(ApplicationMozillaFirefox), err)
	}
	return profiles, nil
}

func parseFirefoxProfilesINI(data []byte) ([]browserProfileCatalogEntry, error) {
	// Empty file is invalid catalog evidence — not a silent empty profile list.
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("empty profiles.ini")
	}

	type section struct {
		name string
		kv   map[string]string
	}
	var sections []section
	var current *section
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			sections = append(sections, section{name: name, kv: map[string]string{}})
			current = &sections[len(sections)-1]
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		current.kv[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var profiles []browserProfileCatalogEntry
	profileSectionsWithPath := 0
	outOfScopeProfiles := 0
	for _, sec := range sections {
		if !isFirefoxProfileSection(sec.name) {
			continue
		}
		pathValue := strings.TrimSpace(sec.kv["Path"])
		if pathValue == "" {
			continue
		}
		profileSectionsWithPath++
		isRelative := strings.TrimSpace(sec.kv["IsRelative"])
		// Only relative catalog paths under the standard root are in scope.
		if isRelative == "0" || filepath.IsAbs(pathValue) {
			outOfScopeProfiles++
			continue
		}
		rel := filepath.ToSlash(filepath.Clean(pathValue))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			outOfScopeProfiles++
			continue
		}
		// Keep the catalog Path as id so multi-profile layouts stay unique and
		// Local mirror joins remain exact (Profiles/<folder>).
		profiles = append(profiles, browserProfileCatalogEntry{
			id:           rel,
			name:         strings.TrimSpace(sec.kv["Name"]),
			relativePath: rel,
		})
	}
	// Catalog present but every listed profile is absolute/out-of-scope: do not
	// treat as silent absence (still never guess absolute paths as candidates).
	if len(profiles) == 0 && profileSectionsWithPath > 0 && outOfScopeProfiles == profileSectionsWithPath {
		return nil, errors.New("profiles.ini lists only absolute or out-of-scope profile paths")
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].id < profiles[j].id
	})
	return profiles, nil
}

func isFirefoxProfileSection(name string) bool {
	if !strings.HasPrefix(name, "Profile") {
		return false
	}
	suffix := name[len("Profile"):]
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

func browserRoamingRoot(roamingAppDataDir string, config browserCacheConfig) string {
	pathParts := append([]string{roamingAppDataDir}, config.roamingAppDataPath...)
	return filepath.Join(pathParts...)
}

// browserDiscoveryRoots returns the path roots used for Protection pre-checks
// before a browser gate runs discovery.
func browserDiscoveryRoots(config browserCacheConfig, opts BrowserCacheDiscoveryOptions) []string {
	var roots []string
	if local := browserCacheLocalAppDataDir(opts); local != "" && len(config.localAppDataPath) > 0 {
		roots = append(roots, browserUserDataRoot(local, config))
	}
	if config.catalogKind == browserProfileCatalogProfilesINI {
		if roaming := browserCacheRoamingAppDataDir(opts); roaming != "" && len(config.roamingAppDataPath) > 0 {
			roots = append(roots, browserRoamingRoot(roaming, config))
		}
	}
	return roots
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

// isBrowserApplication reports whether application is a supported browser_cache
// logical application (Chrome, Edge, or Firefox).
func isBrowserApplication(application string) bool {
	for _, config := range browserCacheConfigs {
		if config.application == application {
			return true
		}
	}
	return false
}
