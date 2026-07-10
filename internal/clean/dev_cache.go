package clean

import (
	"os"
	"path/filepath"
	"runtime"
)

// ResolveDevCachePath resolves a developer tool cache path using only
// environment variables and default paths (no external tool execution).
// Returns empty string if the path cannot be determined.
func ResolveDevCachePath(category string) string {
	return resolveDevCachePath(category, devCachePathDependencies{
		lookupEnv:   os.LookupEnv,
		userHomeDir: os.UserHomeDir,
		joinPath:    filepath.Join,
		goos:        runtime.GOOS,
	})
}

type devCachePathDependencies struct {
	lookupEnv   func(string) (string, bool)
	userHomeDir func() (string, error)
	joinPath    func(...string) string
	goos        string
}

func resolveDevCachePath(category string, deps devCachePathDependencies) string {
	switch category {
	case DevCacheCategoryNPM:
		return resolveNPMCachePath(deps)
	case DevCacheCategoryGo:
		return resolveGoCachePath(deps)
	case DevCacheCategoryPip:
		return resolvePipCachePath(deps)
	case DevCacheCategoryCargo:
		return resolveCargoCachePath(deps)
	case DevCacheCategoryNuGet:
		return resolveNuGetCachePath(deps)
	case DevCacheCategoryCorepack:
		return resolveCorepackOptInCachePath(deps)
	default:
		return ""
	}
}

func resolveNPMCachePath(deps devCachePathDependencies) string {
	// npm: NPM_CONFIG_CACHE -> %LOCALAPPDATA%\npm-cache
	if path, ok := deps.lookupEnv("NPM_CONFIG_CACHE"); ok && path != "" {
		return path
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return deps.joinPath(localAppData, "npm-cache")
	}
	return ""
}

func resolveGoCachePath(deps devCachePathDependencies) string {
	// go: GOCACHE -> %LOCALAPPDATA%\go-build
	if path, ok := deps.lookupEnv("GOCACHE"); ok && path != "" {
		return path
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return deps.joinPath(localAppData, "go-build")
	}
	return ""
}

func resolvePipCachePath(deps devCachePathDependencies) string {
	// pip: PIP_CACHE_DIR -> %LOCALAPPDATA%\pip\Cache
	if path, ok := deps.lookupEnv("PIP_CACHE_DIR"); ok && path != "" {
		return path
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return deps.joinPath(localAppData, "pip", "Cache")
	}
	return ""
}

func resolveCargoCachePath(deps devCachePathDependencies) string {
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
		return deps.joinPath(cargoHome, "registry", "cache")
	}
	return ""
}

func resolveNuGetCachePath(deps devCachePathDependencies) string {
	// nuget: NUGET_HTTP_CACHE_PATH -> %LOCALAPPDATA%\NuGet\v3-cache
	if path, ok := deps.lookupEnv("NUGET_HTTP_CACHE_PATH"); ok && path != "" {
		return path
	}
	if localAppData, ok := deps.lookupEnv("LOCALAPPDATA"); ok && localAppData != "" {
		return deps.joinPath(localAppData, "NuGet", "v3-cache")
	}
	return ""
}

func resolveCorepackOptInCachePath(deps devCachePathDependencies) string {
	// corepack: COREPACK_HOME -> %LOCALAPPDATA%\node\corepack\v1
	if corepackHome, ok := deps.lookupEnv("COREPACK_HOME"); ok && corepackHome != "" {
		return deps.joinPath(corepackHome, "v1")
	}
	base, ok := deps.lookupEnv("XDG_CACHE_HOME")
	if !ok {
		base, ok = deps.lookupEnv("LOCALAPPDATA")
	}
	if !ok {
		home, err := deps.userHomeDir()
		if err != nil || home == "" {
			return ""
		}
		if deps.goos == "windows" {
			base = deps.joinPath(home, "AppData", "Local")
		} else {
			base = deps.joinPath(home, ".cache")
		}
	}
	return deps.joinPath(base, "node", "corepack", "v1")
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

// devCacheCategoryMatchesSuggestion checks if a dev cache category matches
// a ReviewSuggestion. Returns true if the suggestion corresponds to the
// category (e.g., npm-cache matches an npm cache suggestion).
func devCacheCategoryMatchesSuggestion(category string, suggestion ReviewSuggestion) bool {
	switch category {
	case DevCacheCategoryNPM:
		return suggestion.Tool == "npm"
	case DevCacheCategoryGo:
		return suggestion.Tool == "go" && suggestion.Label == "Go build cache"
	case DevCacheCategoryPip:
		return suggestion.Tool == "pip"
	case DevCacheCategoryCargo:
		return suggestion.Tool == "cargo"
	case DevCacheCategoryNuGet:
		return suggestion.Tool == "dotnet"
	case DevCacheCategoryCorepack:
		return suggestion.Tool == "corepack"
	default:
		return false
	}
}
