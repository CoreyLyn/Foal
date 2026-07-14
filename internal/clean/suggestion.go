package clean

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const toolQueryTimeout = 2 * time.Second

type reviewSuggestionProbe struct {
	label        string
	queryArgs    []string
	cleanCommand string
	parsePaths   func([]byte) []string
	// resolvePaths resolves paths without spawning the tool (Corepack-style).
	resolvePaths func(reviewSuggestionDependencies) []string
	// fallbackResolvePaths is used when a query fails, times out, or yields no
	// usable existing path. Bun uses this so Review discovery is not coupled to
	// the caller's current working directory (bun pm cache requires package.json).
	fallbackResolvePaths func(reviewSuggestionDependencies) []string
}

type reviewSuggestionTool struct {
	probes []reviewSuggestionProbe
}

var reviewSuggestionAllowlist = map[string]reviewSuggestionTool{
	"npm": {
		probes: []reviewSuggestionProbe{{
			label:        "npm cache",
			queryArgs:    []string{"config", "get", "cache"},
			cleanCommand: "npm cache clean --force",
		}},
	},
	"pnpm": {
		probes: []reviewSuggestionProbe{{
			label:        "pnpm cache",
			queryArgs:    []string{"store", "path"},
			cleanCommand: "pnpm store prune",
		}},
	},
	"yarn": {
		probes: []reviewSuggestionProbe{{
			label:        "yarn cache",
			queryArgs:    []string{"cache", "dir"},
			cleanCommand: "yarn cache clean",
		}},
	},
	"bun": {
		probes: []reviewSuggestionProbe{{
			label:                "bun cache",
			queryArgs:            []string{"pm", "cache"},
			cleanCommand:         "bun pm cache rm",
			fallbackResolvePaths: resolveBunReviewCachePaths,
		}},
	},
	"pip": {
		probes: []reviewSuggestionProbe{{
			label:        "pip cache",
			queryArgs:    []string{"cache", "dir"},
			cleanCommand: "pip cache purge",
		}},
	},
	"uv": {
		probes: []reviewSuggestionProbe{{
			label:        "uv cache",
			queryArgs:    []string{"cache", "dir"},
			cleanCommand: "uv cache prune",
		}},
	},
	"conda": {
		probes: []reviewSuggestionProbe{{
			label:        "conda cache",
			queryArgs:    []string{"info", "--json"},
			cleanCommand: "conda clean --all",
			parsePaths:   parseCondaPackageDirectories,
		}},
	},
	"go": {
		probes: []reviewSuggestionProbe{
			{
				label:        "Go build cache",
				queryArgs:    []string{"env", "GOCACHE"},
				cleanCommand: "go clean -cache",
			},
			{
				label:        "Go module cache",
				queryArgs:    []string{"env", "GOMODCACHE"},
				cleanCommand: "go clean -modcache",
			},
		},
	},
	"dotnet": {
		probes: []reviewSuggestionProbe{{
			label:        ".NET NuGet caches",
			queryArgs:    []string{"nuget", "locals", "all", "--list"},
			cleanCommand: "dotnet nuget locals all --clear",
			parsePaths:   parseDotnetNugetLocalPaths,
		}},
	},
	"mise": {
		probes: []reviewSuggestionProbe{{
			label:        "mise cache",
			queryArgs:    []string{"cache", "path"},
			cleanCommand: "mise cache clear",
		}},
	},
	"corepack": {
		probes: []reviewSuggestionProbe{{
			label:        "Corepack cache",
			cleanCommand: "corepack cache clean",
			resolvePaths: resolveCorepackCachePaths,
		}},
	},
}

type reviewSuggestionDependencies struct {
	lookPath    func(string) (string, error)
	runQuery    func(context.Context, string, ...string) ([]byte, error)
	pathExists  func(string) bool
	lookupEnv   func(string) (string, bool)
	userHomeDir func() (string, error)
	joinPath    func(...string) string
	goos        string
}

func DiscoverReviewSuggestions(ctx context.Context) []ReviewSuggestion {
	return discoverReviewSuggestions(ctx, []string{"npm", "pnpm", "yarn", "bun", "pip", "uv", "conda", "go", "dotnet", "corepack", "mise"}, reviewSuggestionDependencies{
		lookPath: exec.LookPath,
		runQuery: runToolQuery,
		pathExists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
		lookupEnv:   os.LookupEnv,
		userHomeDir: os.UserHomeDir,
		joinPath:    filepath.Join,
		goos:        runtime.GOOS,
	})
}

func discoverReviewSuggestions(ctx context.Context, tools []string, deps reviewSuggestionDependencies) []ReviewSuggestion {
	if ctx == nil {
		ctx = context.Background()
	}
	deps = withReviewSuggestionDependencyDefaults(deps)
	suggestions := []ReviewSuggestion{}
	for _, toolName := range tools {
		if ctx.Err() != nil {
			break
		}
		tool, allowed := reviewSuggestionAllowlist[toolName]
		if !allowed {
			continue
		}
		executable, err := deps.lookPath(toolName)
		if err != nil {
			continue
		}
		for _, probe := range tool.probes {
			var paths []string
			if probe.resolvePaths != nil {
				paths = probe.resolvePaths(deps)
			} else {
				queryCtx, cancel := context.WithTimeout(ctx, toolQueryTimeout)
				output, err := deps.runQuery(queryCtx, executable, probe.queryArgs...)
				queryErr := queryCtx.Err()
				cancel()
				if err != nil || queryErr != nil {
					// Prefer a successful query; fall back only when configured
					// (Bun: CWD-sensitive pm cache must not hide the official root).
					if probe.fallbackResolvePaths == nil {
						continue
					}
					paths = probe.fallbackResolvePaths(deps)
				} else {
					paths = parseReviewSuggestionPaths(probe, output)
					// Query succeeded but no usable existing path: fall back.
					if probe.fallbackResolvePaths != nil && firstExistingCachePath(paths, deps.pathExists) == "" {
						paths = probe.fallbackResolvePaths(deps)
					}
				}
			}
			// Emit a separate suggestion for each existing path
			for _, path := range paths {
				path = strings.TrimSpace(path)
				if path != "" && deps.pathExists(path) {
					suggestions = append(suggestions, ReviewSuggestion{
						Tool:      toolName,
						Label:     probe.label,
						Command:   probe.cleanCommand,
						CachePath: path,
					})
				}
			}
		}
	}
	return suggestions
}

func withReviewSuggestionDependencyDefaults(deps reviewSuggestionDependencies) reviewSuggestionDependencies {
	if deps.lookupEnv == nil {
		deps.lookupEnv = os.LookupEnv
	}
	if deps.userHomeDir == nil {
		deps.userHomeDir = os.UserHomeDir
	}
	if deps.joinPath == nil {
		deps.joinPath = filepath.Join
	}
	if deps.goos == "" {
		deps.goos = runtime.GOOS
	}
	return deps
}

func resolveCorepackCachePaths(deps reviewSuggestionDependencies) []string {
	corepackHome, found := deps.lookupEnv("COREPACK_HOME")
	if !found {
		base, found := deps.lookupEnv("XDG_CACHE_HOME")
		if !found {
			base, found = deps.lookupEnv("LOCALAPPDATA")
		}
		if !found {
			home, err := deps.userHomeDir()
			if err != nil || home == "" {
				return nil
			}
			if deps.goos == "windows" {
				base = deps.joinPath(home, "AppData", "Local")
			} else {
				base = deps.joinPath(home, ".cache")
			}
		}
		corepackHome = deps.joinPath(base, "node", "corepack")
	}
	return []string{deps.joinPath(corepackHome, "v1")}
}

// resolveBunReviewCachePaths is the Dry-run Review fallback when bun pm cache
// fails (for example outside a Bun project), times out, or returns no usable
// existing path. It reuses the Execute env/default resolver so Review and
// Opt-in share the same official roots; custom probe paths remain preferred
// when the query succeeds.
func resolveBunReviewCachePaths(deps reviewSuggestionDependencies) []string {
	return resolveBunCachePaths(devCachePathDependencies{
		lookupEnv:   deps.lookupEnv,
		userHomeDir: deps.userHomeDir,
		joinPath:    deps.joinPath,
		goos:        deps.goos,
	})
}

func parseReviewSuggestionPaths(probe reviewSuggestionProbe, output []byte) []string {
	if probe.parsePaths != nil {
		return probe.parsePaths(output)
	}
	cachePath := strings.TrimSpace(string(output))
	if cachePath == "" {
		return nil
	}
	return []string{cachePath}
}

func parseCondaPackageDirectories(output []byte) []string {
	var info struct {
		PackageDirectories []string `json:"pkgs_dirs"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return nil
	}
	return info.PackageDirectories
}

func parseDotnetNugetLocalPaths(output []byte) []string {
	var paths []string
	for _, line := range strings.Split(string(output), "\n") {
		_, path, found := strings.Cut(strings.TrimSpace(line), ":")
		if found && strings.TrimSpace(path) != "" {
			paths = append(paths, strings.TrimSpace(path))
		}
	}
	return paths
}

func firstExistingCachePath(paths []string, pathExists func(string) bool) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" && pathExists(path) {
			return path
		}
	}
	return ""
}

func runToolQuery(ctx context.Context, executable string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, executable, args...).Output()
}
