package clean

import (
	"context"
	"path/filepath"
	"strings"
)

// CategoryWindowsUpdateDownloadCache is the canonical exact-selection-only,
// machine-wide opt-in category for stale direct children of the Windows Update
// download staging cache %SystemRoot%\SoftwareDistribution\Download. It is gated
// on the Windows Update service stack being observably idle (ADR 0030 / ADR
// 0033).
const CategoryWindowsUpdateDownloadCache = "windows-update-download-cache"

// ApplicationWindowsUpdate is the one logical service identity Foal reports for
// Windows Update service-stack activity. Clean never stops or reconfigures these
// services; active or unknown state skips the entire category.
const ApplicationWindowsUpdate = "windows-update"

// windowsUpdateDownloadCacheStabilityWindowDays is the minimum latest-observed-
// modification age (inclusive: exactly this many days qualifies) a direct child
// must reach before it can become a candidate. The window is long because a
// download payload's consumption state (pending vs applied) is not externally
// readable, so a long externally-observable quiet period stands in for it
// (thunder-update-download precedent). Unknown or future timestamps fail closed.
const windowsUpdateDownloadCacheStabilityWindowDays = 30

// windowsUpdateDownloadCacheOptInImpactNotice is the path-free machine-wide
// impact vocabulary shown when a candidate is present. It discloses machine-wide
// scope, that Windows re-downloads anything still needed, that the Windows Update
// services must be idle, that only the Recycle Bin is used, and that a
// non-elevated run reclaims only the user-deletable subset.
const windowsUpdateDownloadCacheOptInImpactNotice = "Opt-in Windows Update download cache cleanup moves stale update payloads from the shared %SystemRoot%\\SoftwareDistribution\\Download staging directory to the Recycle Bin. This affects all users of the machine, and Windows re-downloads anything it still needs. The Windows Update services must be idle; active or unknown state skips the entire category. Foal never elevates or stops services, so payloads owned by system services are skipped and only part of the directory is reclaimed. This is not permanent deletion and is not secure erasure."

// windowsUpdateServiceNames are the exact Windows service short names whose
// non-Stopped state marks the update stack active. Query failure is unknown.
// Foal only observes these read-only and never mutates their state.
var windowsUpdateServiceNames = []string{"wuauserv", "bits", "dosvc", "UsoSvc"}

// WindowsUpdateServicesStatus reports whether the Windows Update service stack is
// idle, running, or of unknown/undetermined state. Unknown never means idle.
type WindowsUpdateServicesStatus string

const (
	WindowsUpdateServicesIdle    WindowsUpdateServicesStatus = "idle"
	WindowsUpdateServicesRunning WindowsUpdateServicesStatus = "running"
	WindowsUpdateServicesUnknown WindowsUpdateServicesStatus = "unknown"
)

// WindowsUpdateServicesState is one conservative read-only service observation.
// Message is optional path-free diagnostic text.
type WindowsUpdateServicesState struct {
	Status  WindowsUpdateServicesStatus
	Message string
}

// resolveWindowsUpdateDownloadCacheRootFromSystemRoot resolves the exact
// %SystemRoot%\SoftwareDistribution\Download directory from a SystemRoot value.
// It fails closed (ok=false) for blank, relative, UNC, or otherwise unusable
// values so those are silent absence.
func resolveWindowsUpdateDownloadCacheRootFromSystemRoot(systemRoot string) (string, bool) {
	trimmed := strings.TrimSpace(systemRoot)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, `\\`) {
		return "", false
	}
	if !filepath.IsAbs(trimmed) {
		return "", false
	}
	return filepath.Join(filepath.Clean(trimmed), "SoftwareDistribution", "Download"), true
}

func resolveWindowsUpdateDownloadCacheRoot(dc discoveryContext) (string, bool) {
	systemRoot, _ := dc.env("SystemRoot")
	return resolveWindowsUpdateDownloadCacheRootFromSystemRoot(systemRoot)
}

func detectWindowsUpdateServicesAsFixedRoot(ctx context.Context) fixedRootActivityState {
	s := productionDetectWindowsUpdateServices(ctx)
	return fixedRootActivityState{Status: fixedRootActivityStatus(s.Status), Message: s.Message}
}

// Package-level registration runs before any init(), so catalog validation in
// category_catalog.init can see the windows-update-download-cache fixed-root policy.
var registerWindowsUpdateDownloadCacheFixedRootPolicy = func() struct{} {
	registerFixedRootPolicy(fixedRootPolicy{
		id:                     CategoryWindowsUpdateDownloadCache,
		resolveRoot:            resolveWindowsUpdateDownloadCacheRoot,
		acceptChild:            nil, // any ordinary direct child
		stabilityDays:          windowsUpdateDownloadCacheStabilityWindowDays,
		requireInspection:      true,
		detectActivity:         detectWindowsUpdateServicesAsFixedRoot,
		activityApplication:    ApplicationWindowsUpdate,
		activitySkipCode:       "windows_update_services_active",
		activityRunningMessage: "Windows Update services are active; windows update download cache was skipped",
		activityUnknownMessage: "Windows Update service state could not be determined; windows update download cache was skipped",
		rootUnreadableCode:     "windows_update_download_cache_root_unreadable",
		rootUnreadableMessage:  "Windows Update download cache root could not be inspected; windows update download cache was skipped",
		revalidationCode:       "windows_update_download_cache_revalidation_failed",
		impactNotice:           windowsUpdateDownloadCacheOptInImpactNotice,
		requireExactOnly:       true,
		requireRecycleBin:      true,
	})
	return struct{}{}
}()

func init() {
	registerCategoryIdentityValidator(CategoryWindowsUpdateDownloadCache, validateFixedRootIdentity)
}

// windowsUpdateDownloadCacheCategoryEntry registers the category on the shared
// fixed-root resolver. Kept as a named helper so the catalog table stays readable.
func windowsUpdateDownloadCacheCategoryEntry(definition CleanupCategoryDefinition) categoryCatalogEntry {
	return fixedRootCategoryEntry(definition)
}
