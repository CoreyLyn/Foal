package clean

import (
	"os"
	"path/filepath"
	"runtime"
)

// ResolveDevCachePaths resolves developer tool cache paths using only
// environment variables and default paths (no external tool execution).
// Returns empty slice if no paths can be determined.
func ResolveDevCachePaths(category string) []string {
	return resolveDevCachePaths(category, devCachePathDependencies{
		lookupEnv:   os.LookupEnv,
		userHomeDir: os.UserHomeDir,
		joinPath:    filepath.Join,
		goos:        runtime.GOOS,
	})
}

// ResolveDevCachePath resolves a single developer tool cache path for backward
// compatibility. Use ResolveDevCachePaths instead.
func ResolveDevCachePath(category string) string {
	paths := ResolveDevCachePaths(category)
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

type devCachePathDependencies struct {
	lookupEnv   func(string) (string, bool)
	userHomeDir func() (string, error)
	joinPath    func(...string) string
	goos        string
}

func resolveDevCachePaths(category string, deps devCachePathDependencies) []string {
	switch category {
	case DevCacheCategoryNPM:
		return resolveNPMCachePaths(deps)
	case DevCacheCategoryGo:
		return resolveGoCachePaths(deps)
	case DevCacheCategoryPip:
		return resolvePipCachePaths(deps)
	case DevCacheCategoryCargo:
		return resolveCargoCachePaths(deps)
	case DevCacheCategoryNuGet:
		return resolveNuGetCachePaths(deps)
	case DevCacheCategoryNuGetGlobalPackages:
		return resolveNuGetGlobalPackagesPaths(deps)
	case DevCacheCategoryCorepack:
		return resolveCorepackOptInCachePaths(deps)
	default:
		return nil
	}
}

func resolveNPMCachePaths(deps devCachePathDependencies) []string {
	// npm: NPM_CONFIG_CACHE -> %LOCALAPPDATA%\npm-cache
	if path, ok := deps.lookupEnv("NPM_CONFIG_CACHE"); ok && path != "" {
		return []string{path}
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return []string{deps.joinPath(localAppData, "npm-cache")}
	}
	return nil
}

func resolveGoCachePaths(deps devCachePathDependencies) []string {
	// go: GOCACHE -> %LOCALAPPDATA%\go-build
	if path, ok := deps.lookupEnv("GOCACHE"); ok && path != "" {
		return []string{path}
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return []string{deps.joinPath(localAppData, "go-build")}
	}
	return nil
}

func resolvePipCachePaths(deps devCachePathDependencies) []string {
	// pip: PIP_CACHE_DIR -> %LOCALAPPDATA%\pip\Cache
	if path, ok := deps.lookupEnv("PIP_CACHE_DIR"); ok && path != "" {
		return []string{path}
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return []string{deps.joinPath(localAppData, "pip", "Cache")}
	}
	return nil
}

func resolveCargoCachePaths(deps devCachePathDependencies) []string {
	// cargo: CARGO_HOME -> %USERPROFILE%\.cargo\registry\cache
	var cargoHome string
	if path, ok := deps.lookupEnv("CARGO_HOME"); ok && path != "" {
		cargoHome = path
	} else if userProfile, ok := deps.lookupEnv("USERPROFILE"); ok && userProfile != "" {
		cargoHome = deps.joinPath(userProfile, ".cargo")
	} else if home, err := deps.userHomeDir(); err == nil && home != "" {
		if deps.goos == "windows" {
			cargoHome = deps.joinPath(home, ".cargo")
		} else {
			cargoHome = deps.joinPath(home, ".cargo")
		}
	}
	if cargoHome != "" {
		return []string{deps.joinPath(cargoHome, "registry", "cache")}
	}
	return nil
}

func resolveNuGetCachePaths(deps devCachePathDependencies) []string {
	// nuget: NUGET_HTTP_CACHE_PATH -> %LOCALAPPDATA%\NuGet\v3-cache
	if path, ok := deps.lookupEnv("NUGET_HTTP_CACHE_PATH"); ok && path != "" {
		return []string{path}
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return []string{deps.joinPath(localAppData, "NuGet", "v3-cache")}
	}
	return nil
}

func resolveNuGetGlobalPackagesPaths(deps devCachePathDependencies) []string {
	// nuget global packages: NUGET_PACKAGES -> %USERPROFILE%\.nuget\packages
	if path, ok := deps.lookupEnv("NUGET_PACKAGES"); ok && path != "" {
		return []string{path}
	}
	if userProfile, ok := deps.lookupEnv("USERPROFILE"); ok && userProfile != "" {
		return []string{deps.joinPath(userProfile, ".nuget", "packages")}
	}
	if home, err := deps.userHomeDir(); err == nil && home != "" {
		return []string{deps.joinPath(home, ".nuget", "packages")}
	}
	return nil
}

func resolveCorepackOptInCachePaths(deps devCachePathDependencies) []string {
	// corepack: COREPACK_HOME -> %LOCALAPPDATA%\node\corepack\v1
	if corepackHome, ok := deps.lookupEnv("COREPACK_HOME"); ok && corepackHome != "" {
		return []string{deps.joinPath(corepackHome, "v1")}
	}
	base, ok := deps.lookupEnv("XDG_CACHE_HOME")
	if !ok {
		base, ok = deps.lookupEnv("LOCALAPPDATA")
	}
	if !ok {
		home, err := deps.userHomeDir()
		if err == nil && home != "" {
			if deps.goos == "windows" {
				base = deps.joinPath(home, "AppData", "Local")
			} else {
				base = deps.joinPath(home, ".cache")
			}
		}
	}
	if base != "" {
		return []string{deps.joinPath(base, "node", "corepack", "v1")}
	}
	return nil
}

// devCacheCategories returns the list of all dev cache categories.
func devCacheCategories() []string {
	return developerCacheCategoryIDs()
}

// isDevCacheCategory checks if a category is a dev cache category.
func isDevCacheCategory(category string) bool {
	entry, ok := canonicalCategoryEntry(category)
	return ok && entry.developerCache
}
