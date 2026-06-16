package clean

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	OpportunityCategoryBrowserCache           = "browser_cache"
	OpportunityStatus                         = "skipped_by_default"
	OpportunityReason                         = "requires_explicit_opt_in"
	UserTempOpportunityStatus                 = OpportunityStatus
	UserTempOpportunityReason                 = OpportunityReason
	userTempDescendantLimit                   = 100_000
)

var (
	errOpportunityInspectionLimit          = errors.New("opportunity inspection descendant limit exceeded")
	errOpportunityReparsePoint             = errors.New("opportunity inspection encountered a reparse point")
	existenceObservedOpportunityCategories = []existenceObservedOpportunityCategory{
		{category: OpportunityCategoryCrashDumps, localAppDataPath: []string{"CrashDumps"}},
		{category: OpportunityCategoryWindowsErrorReporting, localAppDataPath: []string{"Microsoft", "Windows", "WER"}},
		{category: OpportunityCategoryExplorerThumbnailCache, localAppDataPath: []string{"Microsoft", "Windows", "Explorer"}},
		{category: OpportunityCategoryINetCache, localAppDataPath: []string{"Microsoft", "Windows", "INetCache"}},
		{category: OpportunityCategoryD3DShaderCache, localAppDataPath: []string{"D3DSCache"}},
		{category: OpportunityCategoryNVIDIADXCache, localAppDataPath: []string{"NVIDIA", "DXCache"}},
	}
)

type existenceObservedOpportunityCategory struct {
	category         string
	localAppDataPath []string
}

type UserTempDiscoveryOptions struct {
	TempDir string
	Now     time.Time
}

type OpportunityDiscoveryOptions struct {
	TempDir         string
	LocalAppDataDir string
	Now             time.Time
}

type BrowserCacheDiscoveryOptions struct {
	LocalAppDataDir string
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
	result := discoverUserTempOpportunities(ctx, UserTempDiscoveryOptions{
		TempDir: opts.TempDir,
		Now:     opts.Now,
	}, deps)
	if ctx == nil {
		ctx = context.Background()
	}
	localAppDataDir := opts.LocalAppDataDir
	if localAppDataDir == "" {
		localAppDataDir = os.Getenv("LOCALAPPDATA")
	}
	if localAppDataDir != "" {
		for _, definition := range existenceObservedOpportunityCategories {
			pathParts := append([]string{localAppDataDir}, definition.localAppDataPath...)
			appendExistenceObservedOpportunity(ctx, &result, definition.category, filepath.Join(pathParts...), deps)
		}
	}
	result.ElapsedMS = time.Since(startedAt).Milliseconds()
	return result
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
