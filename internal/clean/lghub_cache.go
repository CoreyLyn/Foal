package clean

import (
	"context"
	"os"
)

// CategoryLGHUBCache is the canonical exact-selection-only opt-in
// category for LG HUB content-addressed download blobs under the fixed
// ProgramData LGHUB cache root.
const CategoryLGHUBCache = "lghub-cache"

// ApplicationLGHUB is the one logical process/service identity Foal reports
// for LG HUB application and service activity. Clean never stops these;
// active or unknown state skips the entire lghub-cache category.
const ApplicationLGHUB = "lghub"

// lghubCacheRoot is the only production discovery root. Foal never uses
// environment, registry, alternate-drive, or parent-directory discovery.
const lghubCacheRoot = `C:\ProgramData\LGHUB\cache`

// lghubCacheOptInImpactNotice is the path-free impact vocabulary shown
// when a candidate is present. It discloses that this cache is machine-wide
// and affects all users, uses Recycle Bin only, and that LG HUB processes
// must be idle.
const lghubCacheOptInImpactNotice = "Opt-in LG HUB cache cleanup moves content-addressed download blobs to the Recycle Bin. Blobs can normally be re-downloaded on demand, but this affects all users of the machine. LG HUB processes and services must be idle; active or unknown state skips the entire category. This is not permanent deletion and is not secure erasure."

// LGHUBActivityStatus reports whether relevant LG HUB process/service activity
// is idle, running, or of unknown/undetermined state. Unknown never means idle.
type LGHUBActivityStatus string

const (
	LGHUBActivityIdle    LGHUBActivityStatus = "idle"
	LGHUBActivityRunning LGHUBActivityStatus = "running"
	LGHUBActivityUnknown LGHUBActivityStatus = "unknown"
)

// LGHUBActivityState is one conservative process/service observation. Message
// is optional path-free diagnostic text.
type LGHUBActivityState struct {
	Status  LGHUBActivityStatus
	Message string
}

func resolveLGHUBCacheRoot(dc discoveryContext) (string, bool) {
	return lghubCacheRoot, true
}

func acceptLGHUBCacheChild(name string, info os.FileInfo) bool {
	if !is64LowerHexName(name) {
		return false
	}
	return info.Mode().IsRegular()
}

func detectLGHUBActivityAsFixedRoot(ctx context.Context) fixedRootActivityState {
	s := productionDetectLGHUBActivity(ctx)
	return fixedRootActivityState{Status: fixedRootActivityStatus(s.Status), Message: s.Message}
}

// Package-level registration runs before any init(), so catalog validation in
// category_catalog.init can see the lghub-cache fixed-root policy.
var registerLGHUBCacheFixedRootPolicy = func() struct{} {
	registerFixedRootPolicy(fixedRootPolicy{
		id:                     CategoryLGHUBCache,
		resolveRoot:            resolveLGHUBCacheRoot,
		acceptChild:            acceptLGHUBCacheChild,
		bytesFromFileSize:      true,
		requireByteMatch:       true,
		detectActivity:         detectLGHUBActivityAsFixedRoot,
		activityApplication:    ApplicationLGHUB,
		activitySkipCode:       "lghub_activity",
		activityRunningMessage: "LG HUB application or service is active; LG HUB cache was skipped",
		activityUnknownMessage: "LG HUB process or service state could not be determined; LG HUB cache was skipped",
		rootUnreadableCode:     "lghub_cache_root_unreadable",
		rootUnreadableMessage:  "LGHUB cache root could not be inspected; LG HUB cache was skipped",
		revalidationCode:       "lghub_cache_revalidation_failed",
		impactNotice:           lghubCacheOptInImpactNotice,
		requireExactOnly:       true,
		requireRecycleBin:      true,
	})
	return struct{}{}
}()

func init() {
	registerCategoryIdentityValidator(CategoryLGHUBCache, validateFixedRootIdentity)
}

// lghubCacheCategoryEntry registers the category on the shared fixed-root
// resolver. Kept as a named helper so the catalog table stays readable.
func lghubCacheCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return fixedRootCategoryEntry(definition)
}

// is64LowerHexName reports whether name is exactly 64 lowercase hex characters.
func is64LowerHexName(name string) bool {
	if len(name) != 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
