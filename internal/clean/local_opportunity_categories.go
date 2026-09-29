package clean

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	CategoryLarkProfileCache     = "lark-profile-cache"
	CategoryRDPClientOldTraces   = "rdp-client-old-traces"
	CategoryNotionPartitionCache = "notion-partition-cache"
	CategoryTraeSoloToolsStaging = "trae-solo-tools-staging"
)

const (
	larkProfileCacheImpactNotice     = "Lark profile cache cleanup may discard offline Service Worker content and require downloads or sign-in again. Confirm all work is synced before selecting it."
	rdpClientOldTracesImpactNotice   = "Old Remote Desktop ETL traces are diagnostic evidence. Selecting this category moves only traces unchanged for at least 48 hours to the Recycle Bin."
	notionPartitionCacheImpactNotice = "Notion partition cache cleanup may discard offline Service Worker content and require downloads or sign-in again. Confirm all work is synced before selecting it."
	traeSoloToolsStagingImpactNotice = "TRAE SOLO staged tool archives may be needed for offline repair or updates. Only archives unchanged for at least 30 days are eligible."
)

func exactOnlyCategoryDefinition(id, label string, report ReportCategory, running RunningApplicationPolicy) CleanupCategoryDefinition {
	definition := categoryDefinition(id, label, report, CategoryEligibilityOptIn, running, PlannedActionMoveToRecycleBin)
	definition.SelectionPolicy = CategorySelectionPolicyExactOnly
	return definition
}

func appDataExactRoot(deps exactCandidateDeps, segments ...string) []exactCandidateRoot {
	base, ok := deps.envPath("APPDATA")
	if !ok {
		return nil
	}
	return []exactCandidateRoot{{path: filepath.Join(append([]string{base}, segments...)...)}}
}

func localAppDataExactRoot(deps exactCandidateDeps, segments ...string) []exactCandidateRoot {
	base, ok := deps.envPath("LOCALAPPDATA")
	if !ok {
		return nil
	}
	return []exactCandidateRoot{{path: filepath.Join(append([]string{base}, segments...)...)}}
}

var larkUserDirectory = regexp.MustCompile(`(?i)^[0-9a-f]{32}$`)

var larkProfileCachePolicy = exactCandidatePolicy{
	resolveRoots: func(deps exactCandidateDeps) []exactCandidateRoot {
		return appDataExactRoot(deps, "LarkShell", "aha", "users")
	},
	discover: func(ctx context.Context, _ exactCandidateDeps, root string) []string {
		accounts, err := os.ReadDir(root)
		if err != nil {
			return nil
		}
		var candidates []string
		for _, account := range accounts {
			if ctx.Err() != nil {
				return nil
			}
			if !larkUserDirectory.MatchString(account.Name()) || !account.IsDir() {
				continue
			}
			profile := filepath.Join(root, account.Name(), "profile_explorer")
			if !isRealDirectory(profile) {
				continue
			}
			for _, name := range []string{"Cache", "Code Cache", "GPUCache", "DawnCache", "DawnGraphiteCache", "DawnWebGPUCache"} {
				path := filepath.Join(profile, name)
				if isRealDirectory(path) {
					candidates = append(candidates, path)
				}
			}
			worker := filepath.Join(profile, "Service Worker")
			cacheStorage := filepath.Join(worker, "CacheStorage")
			if isRealDirectory(worker) && isRealDirectory(cacheStorage) {
				candidates = append(candidates, cacheStorage)
			}
		}
		return candidates
	},
	applications:         []string{ApplicationLark},
	requireIdleDetection: true,
}

var notionPartitionCachePolicy = exactCandidatePolicy{
	resolveRoots: func(deps exactCandidateDeps) []exactCandidateRoot {
		return appDataExactRoot(deps, "Notion", "Partitions", "notion")
	},
	discover: func(_ context.Context, _ exactCandidateDeps, root string) []string {
		var candidates []string
		for _, name := range []string{"Cache", "Code Cache", "GPUCache", "DawnCache", "DawnGraphiteCache", "DawnWebGPUCache"} {
			path := filepath.Join(root, name)
			if isRealDirectory(path) {
				candidates = append(candidates, path)
			}
		}
		worker := filepath.Join(root, "Service Worker")
		cacheStorage := filepath.Join(worker, "CacheStorage")
		if isRealDirectory(worker) && isRealDirectory(cacheStorage) {
			candidates = append(candidates, cacheStorage)
		}
		return candidates
	},
	applications:         []string{ApplicationNotion},
	requireIdleDetection: true,
}

var rdpTraceName = regexp.MustCompile(`(?i)^RdClientAutoTrace-WppAutoTrace-[0-9]{8}-[0-9]{6}-[0-9]{3}\.etl$`)

var rdpClientOldTracesPolicy = exactCandidatePolicy{
	resolveRoots: func(deps exactCandidateDeps) []exactCandidateRoot {
		return localAppDataExactRoot(deps, "Temp", "DiagOutputDir", "RdClientAutoTrace")
	},
	discover: func(ctx context.Context, deps exactCandidateDeps, root string) []string {
		return discoverQuietOrdinaryFiles(ctx, deps, root, 48*time.Hour, rdpTraceName)
	},
}

var traeToolsArchiveName = regexp.MustCompile(`(?i)^tools-[0-9]+\.[0-9]+\.[0-9]+\.zip$`)

var traeSoloToolsStagingPolicy = exactCandidatePolicy{
	resolveRoots: func(deps exactCandidateDeps) []exactCandidateRoot {
		return appDataExactRoot(deps, "TRAE SOLO CN", "ModularData", "ai-agent", "vm", "tools_staging")
	},
	discover: func(ctx context.Context, deps exactCandidateDeps, root string) []string {
		return discoverQuietOrdinaryFiles(ctx, deps, root, 30*24*time.Hour, traeToolsArchiveName)
	},
	applications:         []string{ApplicationTraeSolo, ApplicationTrae},
	requireIdleDetection: true,
}

func discoverQuietOrdinaryFiles(ctx context.Context, deps exactCandidateDeps, root string, minAge time.Duration, namePattern *regexp.Regexp) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	now := deps.now()
	var paths []string
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil
		}
		if !namePattern.MatchString(entry.Name()) || entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !isOrdinaryFileInfo(info) || info.Size() <= 0 {
			continue
		}
		modified := info.ModTime()
		if modified.IsZero() || modified.After(now) || now.Sub(modified) < minAge || strings.HasSuffix(strings.ToLower(entry.Name()), ".tmp") {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}
