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

type reviewSuggestionTool struct {
	label        string
	queryArgs    []string
	cleanCommand string
	parsePaths   func([]byte) []string
}

var reviewSuggestionAllowlist = map[string]reviewSuggestionTool{
	"npm": {
		label:        "npm cache",
		queryArgs:    []string{"config", "get", "cache"},
		cleanCommand: "npm cache clean --force",
	},
	"pnpm": {
		label:        "pnpm cache",
		queryArgs:    []string{"store", "path"},
		cleanCommand: "pnpm store prune",
	},
	"yarn": {
		label:        "yarn cache",
		queryArgs:    []string{"cache", "dir"},
		cleanCommand: "yarn cache clean",
	},
	"bun": {
		label:        "bun cache",
		queryArgs:    []string{"pm", "cache"},
		cleanCommand: "bun pm cache rm",
	},
	"pip": {
		label:        "pip cache",
		queryArgs:    []string{"cache", "dir"},
		cleanCommand: "pip cache purge",
	},
	"uv": {
		label:        "uv cache",
		queryArgs:    []string{"cache", "dir"},
		cleanCommand: "uv cache prune",
	},
	"conda": {
		label:        "conda cache",
		queryArgs:    []string{"info", "--json"},
		cleanCommand: "conda clean",
		parsePaths:   parseCondaPackageDirectories,
	},
}

type reviewSuggestionDependencies struct {
	lookPath   func(string) (string, error)
	runQuery   func(context.Context, string, ...string) ([]byte, error)
	pathExists func(string) bool
}

func DiscoverReviewSuggestions(ctx context.Context) []ReviewSuggestion {
	return discoverReviewSuggestions(ctx, []string{"npm", "pnpm", "yarn", "bun", "pip", "uv", "conda"}, reviewSuggestionDependencies{
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
		queryCtx, cancel := context.WithTimeout(ctx, toolQueryTimeout)
		output, err := deps.runQuery(queryCtx, executable, tool.queryArgs...)
		queryErr := queryCtx.Err()
		cancel()
		if err != nil || queryErr != nil {
			continue
		}
		cachePath := firstExistingCachePath(parseReviewSuggestionPaths(tool, output), deps.pathExists)
		if cachePath == "" {
			continue
		}
		suggestions = append(suggestions, ReviewSuggestion{
			Tool:      toolName,
			Label:     tool.label,
			Command:   tool.cleanCommand,
			CachePath: cachePath,
		})
	}
	return suggestions
}

func parseReviewSuggestionPaths(tool reviewSuggestionTool, output []byte) []string {
	if tool.parsePaths != nil {
		return tool.parsePaths(output)
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
