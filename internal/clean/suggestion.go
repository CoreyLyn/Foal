package clean

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

const toolQueryTimeout = 2 * time.Second

type reviewSuggestionProbe struct {
	label        string
	queryArgs    []string
	cleanCommand string
	parsePaths   func([]byte) []string
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
			label:        "bun cache",
			queryArgs:    []string{"pm", "cache"},
			cleanCommand: "bun pm cache rm",
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
}

type reviewSuggestionDependencies struct {
	lookPath   func(string) (string, error)
	runQuery   func(context.Context, string, ...string) ([]byte, error)
	pathExists func(string) bool
}

func DiscoverReviewSuggestions(ctx context.Context) []ReviewSuggestion {
	return discoverReviewSuggestions(ctx, []string{"npm", "pnpm", "yarn", "bun", "pip", "uv", "conda", "go", "dotnet"}, reviewSuggestionDependencies{
		lookPath: exec.LookPath,
		runQuery: runToolQuery,
		pathExists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
	})
}

func discoverReviewSuggestions(ctx context.Context, tools []string, deps reviewSuggestionDependencies) []ReviewSuggestion {
	if ctx == nil {
		ctx = context.Background()
	}
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
			queryCtx, cancel := context.WithTimeout(ctx, toolQueryTimeout)
			output, err := deps.runQuery(queryCtx, executable, probe.queryArgs...)
			queryErr := queryCtx.Err()
			cancel()
			if err != nil || queryErr != nil {
				continue
			}
			cachePath := firstExistingCachePath(parseReviewSuggestionPaths(probe, output), deps.pathExists)
			if cachePath == "" {
				continue
			}
			suggestions = append(suggestions, ReviewSuggestion{
				Tool:      toolName,
				Label:     probe.label,
				Command:   probe.cleanCommand,
				CachePath: cachePath,
			})
		}
	}
	return suggestions
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
