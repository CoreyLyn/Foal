package clean

// CategoryThunderUpdateDownload is the canonical exact-selection-only opt-in
// category for Thunder update downloads under the fixed ProgramData XLLiveUD
// download root.
const CategoryThunderUpdateDownload = "thunder-update-download"

// ApplicationThunder is the one logical process/service identity Foal reports
// for Thunder application and service activity. Clean never stops these;
// active or unknown state skips the entire thunder-update-download category.
const ApplicationThunder = "thunder"

// thunderUpdateDownloadCacheRoot is the only production discovery root. Foal
// never uses environment, registry, alternate-drive, or parent-directory discovery.
const thunderUpdateDownloadCacheRoot = `C:\ProgramData\Thunder Network\XLLiveUD\Download`

// thunderUpdateDownloadStabilityWindowDays is the minimum latest-observed-
// modification age for a direct child before it can become a candidate.
const thunderUpdateDownloadStabilityWindowDays = 30

// thunderUpdateDownloadOptInImpactNotice is the path-free impact vocabulary shown
// when a candidate is present. It discloses that this cache is machine-wide
// and affects all users, uses Recycle Bin only, and that Thunder processes
// must be idle.
const thunderUpdateDownloadOptInImpactNotice = "Opt-in Thunder update download cleanup moves update packages to the Recycle Bin. Packages may be re-downloaded on demand, but this affects all users of the machine. Thunder processes and services must be idle; active or unknown state skips the entire category. This is not permanent deletion and is not secure erasure."

func resolveThunderUpdateDownloadRoot(dc discoveryContext) (string, bool) {
	return thunderUpdateDownloadCacheRoot, true
}

// Package-level registration runs before any init(), so catalog validation in
// category_catalog.init can see the thunder-update-download fixed-root policy.
var registerThunderUpdateDownloadFixedRootPolicy = func() struct{} {
	registerFixedRootPolicy(fixedRootPolicy{
		id:                     CategoryThunderUpdateDownload,
		resolveRoot:            resolveThunderUpdateDownloadRoot,
		acceptChild:            nil, // any ordinary direct child
		stabilityDays:          thunderUpdateDownloadStabilityWindowDays,
		requireInspection:      true,
		detectActivity:         productionDetectThunderUpdateDownloadActivity,
		activityApplication:    ApplicationThunder,
		activitySkipCode:       "thunder_update_download_activity",
		activityRunningMessage: "Thunder application or service is active; thunder update download cache was skipped",
		activityUnknownMessage: "Thunder process or service state could not be determined; thunder update download cache was skipped",
		rootUnreadableCode:     "thunder_update_download_cache_root_unreadable",
		rootUnreadableMessage:  "Thunder update download cache root could not be inspected; thunder update download cache was skipped",
		revalidationCode:       "thunder_update_download_cache_revalidation_failed",
		impactNotice:           thunderUpdateDownloadOptInImpactNotice,
		requireExactOnly:       true,
		requireRecycleBin:      true,
	})
	return struct{}{}
}()

func init() {
	registerCategoryIdentityValidator(CategoryThunderUpdateDownload, validateFixedRootIdentity)
}
