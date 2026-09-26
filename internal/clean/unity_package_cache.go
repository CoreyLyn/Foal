package clean

import (
	"context"
	"path/filepath"
)

// CategoryUnityPackageCache is the opt-in category for Unity Package Manager's
// global cache under the current user's %LOCALAPPDATA%\Unity\cache (Unity
// Manual "Global cache"): npm (registry data) and packages (unpacked packages)
// used by Unity 2022.3 and 2023.1, upm\db used by Unity 2023.2 and later, and
// upm\packages used by Unity 2023.2 only. The root, the upm parent, git-lfs
// caches, and unknown siblings are never candidates, and cache-root overrides
// such as UPM_CACHE_ROOT are not followed.
const CategoryUnityPackageCache = "unity-package-cache"

// ApplicationUnity is one logical identity for the Unity Editor, Unity Hub, and
// the Package Manager server process.
const ApplicationUnity = "unity"

const unityPackageCacheOptInImpactNotice = "Opt-in Unity package cache cleanup permanently deletes Unity Package Manager's global cache of downloaded registry data and unpacked packages. Unity downloads or unpacks packages again when a project needs them, so installs that relied on this cache need network access. Permanent deletion is ordinary filesystem removal (not secure erasure)."

// unityPackageCacheChildren are the exact documented cache children under the
// global cache root, in deterministic discovery order.
var unityPackageCacheChildren = [][]string{
	{"npm"},
	{"packages"},
	{"upm", "db"},
	{"upm", "packages"},
}

var unityPackageCachePolicy = exactCandidatePolicy{
	resolveRoots: func(deps exactCandidateDeps) []exactCandidateRoot {
		localAppData, ok := deps.envPath("LOCALAPPDATA")
		if !ok {
			return nil
		}
		return []exactCandidateRoot{{path: filepath.Join(localAppData, "Unity", "cache"), application: ApplicationUnity}}
	},
	discover:     discoverUnityPackageCacheChildren,
	applications: []string{ApplicationUnity},
}

func discoverUnityPackageCacheChildren(_ context.Context, _ exactCandidateDeps, root string) []string {
	var children []string
	for _, segments := range unityPackageCacheChildren {
		if len(segments) > 1 && !isRealDirectory(filepath.Join(root, segments[0])) {
			continue
		}
		child := filepath.Join(append([]string{root}, segments...)...)
		if isRealDirectory(child) {
			children = append(children, child)
		}
	}
	return children
}
