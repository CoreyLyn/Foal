package clean

import (
	"path/filepath"
	"strings"
)

// CategoryWindowsTemp is the canonical exact-selection-only opt-in category for
// stale direct children of the machine-shared system temp directory
// %SystemRoot%\Temp. It is the machine-wide analogue of user_temp (ADR 0030 /
// ADR 0032). Discovery and revalidation run through the shared fixed-root
// engine; this file owns only the product policy constants and root resolution.
const CategoryWindowsTemp = "windows-temp"

// windowsTempStabilityWindowDays is the minimum latest-observed-modification age
// (inclusive: exactly this many days qualifies) a direct child must reach before
// it can become a candidate. Unknown or future timestamps fail closed.
const windowsTempStabilityWindowDays = 14

// windowsTempOptInImpactNotice is the path-free machine-wide impact vocabulary
// shown when a candidate is present. It discloses that this cache is machine-wide
// and affects all users, uses the Recycle Bin only, is never permanent, and that
// a non-elevated run reclaims only the user-deletable subset.
const windowsTempOptInImpactNotice = "Opt-in Windows system temp cleanup moves stale entries from the shared system temp directory to the Recycle Bin. This affects all users of the machine. Foal never elevates, so entries owned by other users or by system services are skipped and only part of the directory is reclaimed. This is not permanent deletion and is not secure erasure."

// resolveWindowsTempRootFromSystemRoot resolves the exact %SystemRoot%\Temp
// directory from a SystemRoot value. It fails closed (ok=false) for blank,
// relative, UNC, or otherwise unusable values so those are silent absence.
func resolveWindowsTempRootFromSystemRoot(systemRoot string) (string, bool) {
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
	return filepath.Join(filepath.Clean(trimmed), "Temp"), true
}

func resolveWindowsTempRoot(dc discoveryContext) (string, bool) {
	systemRoot, _ := dc.env("SystemRoot")
	return resolveWindowsTempRootFromSystemRoot(systemRoot)
}

// Package-level registration runs before any init(), so catalog validation in
// category_catalog.init can see the windows-temp fixed-root policy.
var registerWindowsTempFixedRootPolicy = func() struct{} {
	registerFixedRootPolicy(fixedRootPolicy{
		id:                    CategoryWindowsTemp,
		resolveRoot:           resolveWindowsTempRoot,
		acceptChild:           nil, // any ordinary direct child
		stabilityDays:         windowsTempStabilityWindowDays,
		requireInspection:     true,
		rootUnreadableCode:    "windows_temp_root_unreadable",
		rootUnreadableMessage: "Windows system temp root could not be inspected; windows-temp was skipped",
		revalidationCode:      "windows_temp_revalidation_failed",
		impactNotice:          windowsTempOptInImpactNotice,
		requireExactOnly:      true,
		requireRecycleBin:     true,
	})
	return struct{}{}
}()

func init() {
	registerCategoryIdentityValidator(CategoryWindowsTemp, validateFixedRootIdentity)
}
