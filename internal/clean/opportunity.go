package clean

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	OpportunityCategoryUserTemp               = "user_temp"
	OpportunityCategoryCrashDumps             = "crash_dumps"
	OpportunityCategoryWindowsErrorReporting  = "windows_error_reporting"
	OpportunityCategoryExplorerThumbnailCache = "explorer_thumbnail_cache"
	OpportunityCategoryINetCache              = "inet_cache"
	OpportunityCategoryD3DShaderCache         = "d3d_shader_cache"
	OpportunityCategoryNVIDIADXCache          = "nvidia_dx_cache"
	OpportunityCategoryAMDGPUShaderCaches     = "amd_gpu_shader_caches"
	OpportunityCategoryIntelGPUShaderCache    = "intel_gpu_shader_cache"
	OpportunityCategoryBrowserCache           = "browser_cache"
	OpportunityCategoryVSCodeCache            = "vscode_cache"
	OpportunityCategoryCursorCache            = "cursor_cache"
	OpportunityCategoryVSCodeInsidersCache    = "vscode_insiders_cache"
	OpportunityCategoryVSCodiumCache          = "vscodium_cache"
	OpportunityCategoryWindsurfCache          = "windsurf_cache"
	OpportunityCategoryTraeCache              = "trae_cache"
	OpportunityStatus                         = "skipped_by_default"
	OpportunityReason                         = "requires_explicit_opt_in"
	UserTempOpportunityStatus                 = OpportunityStatus
	UserTempOpportunityReason                 = OpportunityReason
	userTempDescendantLimit                   = 100_000
)

var (
	errOpportunityInspectionLimit = errors.New("opportunity inspection descendant limit exceeded")
	errOpportunityReparsePoint    = errors.New("opportunity inspection encountered a reparse point")
)

type UserTempDiscoveryOptions struct {
	TempDir string
	Now     time.Time
}

type OpportunityDiscoveryOptions struct {
	TempDir         string
	LocalAppDataDir string
	// LocalAppDataLowDir overrides the current-user LocalLow base (Known Folder
	// FOLDERID_LocalAppDataLow) for tests. Empty uses production resolution.
	// When LocalAppDataDir is a fixture override and this field is empty,
	// LocalLow roots are not scanned so host state cannot leak into fixtures.
	LocalAppDataLowDir string
	Now                time.Time
	// Categories restricts discovery to the listed categories. When empty,
	// discovery scans every implemented category. Used to scan only opted-in
	// categories at execute (ADR-0008: non-opted-in categories stay omitted)
	// and only non-opted-in categories for the dry-run review projection.
	Categories []string
}

// gpuShaderCacheOptInImpactNotice is shared by permanent GPU/shader cache
// categories (AMD/Intel and symmetric to D3D/NVIDIA class impact).
const gpuShaderCacheOptInImpactNotice = "Games and the desktop may recompile shaders after cleanup; temporary longer loads or stutter are expected."

// browserCacheOptInImpactNotice discloses regenerable-cache cleanup impact for
// browser_cache, including Cache Storage (Service Worker CacheStorage) offline
// PWA re-download. It does not claim logout, unregistration of service workers,
// or removal of cookies/history/IndexedDB.
const browserCacheOptInImpactNotice = "Browser cache cleanup removes regenerable HTTP, code, GPU, and Cache Storage (Service Worker CacheStorage) data. Cookies, passwords, history, and IndexedDB are not removed. Progressive Web Apps and offline sites may need a network connection on next visit to rebuild their cache; first loads after cleanup may be slower."

type BrowserCacheDiscoveryOptions struct {
	// LocalAppDataDir overrides %LOCALAPPDATA% for tests. Empty uses the process
	// environment. Chromium User Data and Firefox Local cache trees resolve here.
	LocalAppDataDir string
	// RoamingAppDataDir overrides %APPDATA% for tests. Empty uses the process
	// environment. Firefox profiles.ini catalog resolves under this root.
	// Chromium browsers ignore this field.
	RoamingAppDataDir string
}

type OpportunityDiscoveryResult struct {
	Opportunities []Opportunity                     `json:"opportunities"`
	Incomplete    []IncompleteOpportunityInspection `json:"incomplete"`
	ElapsedMS     int64                             `json:"elapsed_ms"`
}

type UserTempDiscoveryResult = OpportunityDiscoveryResult

type Opportunity struct {
	Category         string                         `json:"category"`
	Path             string                         `json:"path"`
	Bytes            int64                          `json:"bytes"`
	LatestModifiedAt time.Time                      `json:"-"`
	IdleDays         int                            `json:"-"`
	Status           string                         `json:"status"`
	Reason           string                         `json:"reason"`
	BrowserCache     *BrowserCacheOpportunityDetail `json:"browser_cache,omitempty"`
}

type UserTempOpportunity = Opportunity

type BrowserCacheOpportunityDetail struct {
	Browser      string                      `json:"browser"`
	ProfileCount int                         `json:"profile_count"`
	Profiles     []BrowserCacheProfileDetail `json:"profiles"`
}

type BrowserCacheProfileDetail struct {
	ID     string                  `json:"id"`
	Name   string                  `json:"name,omitempty"`
	Path   string                  `json:"path"`
	Caches []BrowserCacheDirectory `json:"caches"`
}

type BrowserCacheDirectory struct {
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

type IncompleteOpportunityInspection struct {
	Category string          `json:"category"`
	Path     string          `json:"path"`
	Reason   StructuredIssue `json:"reason"`
}

func DiscoverUserTempOpportunities(ctx context.Context, opts UserTempDiscoveryOptions) UserTempDiscoveryResult {
	return discoverUserTempOpportunities(ctx, opts, opportunityDiscoveryDependencies{
		readDir:  os.ReadDir,
		joinPath: joinOpportunityPath,
		stat:     os.Lstat,
		walkDir:  filepath.WalkDir,
	})
}

func DiscoverOpportunities(ctx context.Context, opts OpportunityDiscoveryOptions) OpportunityDiscoveryResult {
	return discoverOpportunities(ctx, opts, opportunityDiscoveryDependencies{
		readDir:  os.ReadDir,
		joinPath: joinOpportunityPath,
		stat:     os.Lstat,
		walkDir:  filepath.WalkDir,
	})
}

func discoverOpportunities(ctx context.Context, opts OpportunityDiscoveryOptions, deps opportunityDiscoveryDependencies) OpportunityDiscoveryResult {
	startedAt := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	result := OpportunityDiscoveryResult{
		Opportunities: []Opportunity{},
		Incomplete:    []IncompleteOpportunityInspection{},
	}
	if opportunityCategoryEnabled(OpportunityCategoryUserTemp, opts.Categories) {
		userTemp := discoverUserTempOpportunities(ctx, UserTempDiscoveryOptions{TempDir: opts.TempDir, Now: opts.Now}, deps)
		result.Opportunities = append(result.Opportunities, userTemp.Opportunities...)
		result.Incomplete = append(result.Incomplete, userTemp.Incomplete...)
	}
	localAppDataDir := opts.LocalAppDataDir
	localAppDataLowDir := opts.LocalAppDataLowDir
	fixtureLocalAppData := localAppDataDir != ""
	if localAppDataDir == "" {
		localAppDataDir = os.Getenv("LOCALAPPDATA")
	}
	if localAppDataLowDir == "" {
		// Production: resolve Known Folder / fallback. Fixture LocalAppData
		// without an explicit LocalLow override isolates tests from host caches.
		if !fixtureLocalAppData {
			localAppDataLowDir = resolveLocalAppDataLowDir()
		}
	}
	for _, entry := range canonicalCategoryEntries {
		if len(entry.existenceRoots) == 0 || !opportunityCategoryEnabled(entry.definition.Identifier, opts.Categories) {
			continue
		}
		for _, root := range entry.existenceRoots {
			base := ""
			switch root.base {
			case existenceRootLocalAppData:
				base = localAppDataDir
			case existenceRootLocalAppDataLow:
				base = localAppDataLowDir
			}
			if base == "" || len(root.segments) == 0 {
				continue
			}
			pathParts := append([]string{base}, root.segments...)
			path := filepath.Join(pathParts...)
			if root.matchDirectFileName != nil {
				appendExistenceObservedMatchingFiles(ctx, &result, entry.definition.Identifier, path, root.matchDirectFileName, deps)
				continue
			}
			appendExistenceObservedOpportunity(ctx, &result, entry.definition.Identifier, path, deps)
		}
	}
	result.ElapsedMS = time.Since(startedAt).Milliseconds()
	return result
}

// opportunityCategoryEnabled reports whether a category should be scanned.
// An empty filter means scan every category; otherwise only listed categories
// are scanned.
func opportunityCategoryEnabled(category string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, c := range filter {
		if c == category {
			return true
		}
	}
	return false
}

type opportunityDiscoveryDependencies struct {
	readDir  func(string) ([]os.DirEntry, error)
	joinPath func(string, string) string
	stat     func(string) (os.FileInfo, error)
	walkDir  func(string, fs.WalkDirFunc) error
}

func discoverUserTempOpportunities(ctx context.Context, opts UserTempDiscoveryOptions, deps opportunityDiscoveryDependencies) UserTempDiscoveryResult {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	root := opts.TempDir
	if root == "" {
		root = os.TempDir()
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	if deps.joinPath == nil {
		deps.joinPath = joinOpportunityPath
	}
	result := UserTempDiscoveryResult{
		Opportunities: []UserTempOpportunity{},
		Incomplete:    []IncompleteOpportunityInspection{},
	}

	select {
	case <-ctx.Done():
		result.Incomplete = append(result.Incomplete, incompleteInspection(OpportunityCategoryUserTemp, root, "context_canceled", ctx.Err().Error()))
		result.ElapsedMS = time.Since(startedAt).Milliseconds()
		return result
	default:
	}

	entries, err := deps.readDir(root)
	if err != nil {
		result.Incomplete = append(result.Incomplete, incompleteInspection(OpportunityCategoryUserTemp, root, classifyError(err), err.Error()))
		result.ElapsedMS = time.Since(startedAt).Milliseconds()
		return result
	}
	for _, entry := range entries {
		path := deps.joinPath(root, entry.Name())
		if !isDirectChildPath(root, path) {
			result.Incomplete = append(result.Incomplete, incompleteInspection(OpportunityCategoryUserTemp, path, "unsafe_path", "resolved entry path is not a direct child of the user temp directory"))
			continue
		}
		if isFoalOwnedTempEntry(entry.Name()) {
			continue
		}
		inspection, err := inspectOpportunity(ctx, path, userTempDescendantLimit, deps.walkDir)
		if err != nil {
			result.Incomplete = append(result.Incomplete, incompleteInspection(OpportunityCategoryUserTemp, path, classifyOpportunityInspectionError(err), err.Error()))
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}
			continue
		}
		idleDays := int(now.Sub(inspection.latestModifiedAt) / (24 * time.Hour))
		if idleDays < 7 {
			continue
		}
		result.Opportunities = append(result.Opportunities, UserTempOpportunity{
			Category:         OpportunityCategoryUserTemp,
			Path:             path,
			Bytes:            inspection.bytes,
			LatestModifiedAt: inspection.latestModifiedAt,
			IdleDays:         idleDays,
			Status:           UserTempOpportunityStatus,
			Reason:           UserTempOpportunityReason,
		})
	}
	result.ElapsedMS = time.Since(startedAt).Milliseconds()
	return result
}

func appendExistenceObservedOpportunity(ctx context.Context, result *OpportunityDiscoveryResult, category, path string, deps opportunityDiscoveryDependencies) {
	if deps.stat == nil {
		deps.stat = os.Lstat
	}
	_, err := deps.stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		result.Incomplete = append(result.Incomplete, incompleteInspection(category, path, classifyError(err), err.Error()))
		return
	}
	inspection, err := inspectOpportunity(ctx, path, userTempDescendantLimit, deps.walkDir)
	if err != nil {
		result.Incomplete = append(result.Incomplete, incompleteInspection(category, path, classifyOpportunityInspectionError(err), err.Error()))
		return
	}
	result.Opportunities = append(result.Opportunities, UserTempOpportunity{
		Category: category,
		Path:     path,
		Bytes:    inspection.bytes,
		Status:   UserTempOpportunityStatus,
		Reason:   UserTempOpportunityReason,
	})
}

// appendExistenceObservedMatchingFiles discovers independent file candidates
// under parent. The parent directory is never a candidate. Only direct children
// whose basenames match matchName and that are regular non-reparse files are
// measured. Missing parent or zero matches is silent empty — never a whole-
// parent fallback.
func appendExistenceObservedMatchingFiles(
	ctx context.Context,
	result *OpportunityDiscoveryResult,
	category, parent string,
	matchName func(string) bool,
	deps opportunityDiscoveryDependencies,
) {
	if matchName == nil {
		return
	}
	if deps.stat == nil {
		deps.stat = os.Lstat
	}
	if deps.readDir == nil {
		deps.readDir = os.ReadDir
	}
	if deps.joinPath == nil {
		deps.joinPath = joinOpportunityPath
	}

	info, err := deps.stat(parent)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		result.Incomplete = append(result.Incomplete, incompleteInspection(category, parent, classifyError(err), err.Error()))
		return
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		// Parent must be a real directory. Fail closed: never promote parent.
		return
	}

	select {
	case <-ctx.Done():
		result.Incomplete = append(result.Incomplete, incompleteInspection(category, parent, "context_canceled", ctx.Err().Error()))
		return
	default:
	}

	entries, err := deps.readDir(parent)
	if err != nil {
		result.Incomplete = append(result.Incomplete, incompleteInspection(category, parent, classifyError(err), err.Error()))
		return
	}
	// Deterministic order for contract stability.
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	for _, name := range names {
		if !matchName(name) {
			continue
		}
		path := deps.joinPath(parent, name)
		if !isDirectChildPath(parent, path) {
			continue
		}
		fi, err := deps.stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			result.Incomplete = append(result.Incomplete, incompleteInspection(category, path, classifyError(err), err.Error()))
			continue
		}
		// Only regular files; reject directories and reparse points with matching names.
		if fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			continue
		}
		appendExistenceObservedOpportunity(ctx, result, category, path, deps)
	}
}

// isExplorerThumbnailCacheDBName reports whether basename matches the research
// allowlist thumbcache_*.db / iconcache_*.db (case-insensitive). The underscore
// after the prefix is required; bare thumbcache.db / iconcache.db are excluded.
func isExplorerThumbnailCacheDBName(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return false
	}
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".db") {
		return false
	}
	return strings.HasPrefix(lower, "thumbcache_") || strings.HasPrefix(lower, "iconcache_")
}

func joinOpportunityPath(root, name string) string {
	return filepath.Join(root, name)
}

func isDirectChildPath(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanRoot) || !filepath.IsAbs(cleanPath) {
		return false
	}
	if strings.HasPrefix(cleanRoot, `\\`) || strings.HasPrefix(cleanPath, `\\`) {
		return false
	}
	relative, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || relative == "." || relative == "" || filepath.IsAbs(relative) {
		return false
	}
	return filepath.Dir(relative) == "."
}

type opportunityInspection struct {
	bytes            int64
	latestModifiedAt time.Time
}

func inspectOpportunity(ctx context.Context, path string, descendantLimit int, walkDir func(string, fs.WalkDirFunc) error) (opportunityInspection, error) {
	var inspection opportunityInspection
	descendants := 0
	err := walkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if walkErr != nil {
			return walkErr
		}
		if current != path {
			descendants++
			if descendants > descendantLimit {
				return errOpportunityInspectionLimit
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errOpportunityReparsePoint
		}
		if info.ModTime().After(inspection.latestModifiedAt) {
			inspection.latestModifiedAt = info.ModTime()
		}
		if !info.IsDir() {
			inspection.bytes += info.Size()
		}
		return nil
	})
	return inspection, err
}

func classifyOpportunityInspectionError(err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "context_canceled"
	case errors.Is(err, errOpportunityInspectionLimit):
		return "inspection_limit_exceeded"
	case errors.Is(err, errOpportunityReparsePoint):
		return "reparse_point"
	default:
		return classifyError(err)
	}
}

func isFoalOwnedTempEntry(name string) bool {
	return strings.HasPrefix(name, "foal-") || strings.HasPrefix(name, "Foal-")
}

func incompleteInspection(category, path, code, message string) IncompleteOpportunityInspection {
	return IncompleteOpportunityInspection{
		Category: category,
		Path:     path,
		Reason:   issue(code, message, true, path, ""),
	}
}

func (opportunity Opportunity) MarshalJSON() ([]byte, error) {
	type opportunityJSON struct {
		Category         string     `json:"category"`
		Path             string     `json:"path"`
		Bytes            int64      `json:"bytes"`
		LatestModifiedAt *time.Time `json:"latest_modified_at,omitempty"`
		IdleDays         *int       `json:"idle_days,omitempty"`
		Status           string     `json:"status"`
		Reason           string     `json:"reason"`
	}
	encoded := opportunityJSON{
		Category: normalizedOpportunityCategory(opportunity.Category),
		Path:     opportunity.Path,
		Bytes:    opportunity.Bytes,
		Status:   opportunity.Status,
		Reason:   opportunity.Reason,
	}
	if opportunity.BrowserCache != nil {
		type browserOpportunityJSON struct {
			Category     string                         `json:"category"`
			Path         string                         `json:"path"`
			Bytes        int64                          `json:"bytes"`
			Status       string                         `json:"status"`
			Reason       string                         `json:"reason"`
			BrowserCache *BrowserCacheOpportunityDetail `json:"browser_cache,omitempty"`
		}
		return json.Marshal(browserOpportunityJSON{
			Category:     normalizedOpportunityCategory(opportunity.Category),
			Path:         opportunity.Path,
			Bytes:        opportunity.Bytes,
			Status:       opportunity.Status,
			Reason:       opportunity.Reason,
			BrowserCache: opportunity.BrowserCache,
		})
	}
	if normalizedOpportunityCategory(opportunity.Category) == OpportunityCategoryUserTemp {
		encoded.LatestModifiedAt = &opportunity.LatestModifiedAt
		encoded.IdleDays = &opportunity.IdleDays
	}
	return json.Marshal(encoded)
}

func normalizedOpportunityCategory(category string) string {
	if category == "" {
		return OpportunityCategoryUserTemp
	}
	return category
}
