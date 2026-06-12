package clean

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDiscoverReviewSuggestionsQueriesInstalledJavaScriptToolCaches(t *testing.T) {
	tests := []struct {
		tool         string
		executable   string
		queryArgs    []string
		cleanCommand string
		cachePath    string
	}{
		{
			tool:         "npm",
			executable:   `C:\Program Files\nodejs\npm.cmd`,
			queryArgs:    []string{"config", "get", "cache"},
			cleanCommand: "npm cache clean --force",
			cachePath:    `C:\Users\corey\AppData\Local\npm-cache`,
		},
		{
			tool:         "pnpm",
			executable:   `C:\Users\corey\AppData\Roaming\npm\pnpm.cmd`,
			queryArgs:    []string{"store", "path"},
			cleanCommand: "pnpm store prune",
			cachePath:    `C:\Users\corey\AppData\Local\pnpm\store\v10`,
		},
		{
			tool:         "yarn",
			executable:   `C:\Program Files\nodejs\yarn.cmd`,
			queryArgs:    []string{"cache", "dir"},
			cleanCommand: "yarn cache clean",
			cachePath:    `C:\Users\corey\AppData\Local\Yarn\Cache\v6`,
		},
		{
			tool:         "bun",
			executable:   `C:\Users\corey\.bun\bin\bun.exe`,
			queryArgs:    []string{"pm", "cache"},
			cleanCommand: "bun pm cache rm",
			cachePath:    `C:\Users\corey\.bun\install\cache`,
		},
	}

	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			var gotExecutable string
			var gotArgs []string
			result := discoverReviewSuggestions(context.Background(), []string{test.tool}, reviewSuggestionDependencies{
				lookPath: func(tool string) (string, error) {
					if tool != test.tool {
						t.Fatalf("PATH lookup tool = %q, want %s", tool, test.tool)
					}
					return test.executable, nil
				},
				runQuery: func(_ context.Context, executable string, args ...string) ([]byte, error) {
					gotExecutable = executable
					gotArgs = append([]string(nil), args...)
					return []byte(test.cachePath + "\r\n"), nil
				},
				pathExists: func(path string) bool {
					return path == test.cachePath
				},
			})

			if gotExecutable != test.executable {
				t.Fatalf("query executable = %q, want %q", gotExecutable, test.executable)
			}
			if !equalStrings(gotArgs, test.queryArgs) {
				t.Fatalf("query args = %#v, want only %#v", gotArgs, test.queryArgs)
			}
			if len(result) != 1 {
				t.Fatalf("suggestions = %#v, want one %s suggestion", result, test.tool)
			}
			suggestion := result[0]
			if suggestion.Tool != test.tool ||
				suggestion.Label != test.tool+" cache" ||
				suggestion.Command != test.cleanCommand ||
				suggestion.CachePath != test.cachePath {
				t.Fatalf("suggestion = %#v, want fixed %s review suggestion", suggestion, test.tool)
			}
		})
	}
}

func TestDiscoverReviewSuggestionsRequiresPATHAndExistingCache(t *testing.T) {
	for _, tool := range []string{"npm", "pnpm", "yarn", "bun"} {
		t.Run(tool+" not on PATH", func(t *testing.T) {
			result := discoverReviewSuggestions(context.Background(), []string{tool}, reviewSuggestionDependencies{
				lookPath: func(string) (string, error) {
					return "", errors.New("not found")
				},
				runQuery: func(context.Context, string, ...string) ([]byte, error) {
					t.Fatal("query ran without a successful PATH lookup")
					return nil, nil
				},
				pathExists: func(string) bool {
					t.Fatal("cache existence checked without a resolved path")
					return false
				},
			})
			if len(result) != 0 {
				t.Fatalf("suggestions = %#v, want none", result)
			}
		})

		t.Run(tool+" cache absent", func(t *testing.T) {
			result := discoverReviewSuggestions(context.Background(), []string{tool}, reviewSuggestionDependencies{
				lookPath: func(string) (string, error) {
					return tool + ".cmd", nil
				},
				runQuery: func(context.Context, string, ...string) ([]byte, error) {
					return []byte(`C:\missing\cache`), nil
				},
				pathExists: func(string) bool {
					return false
				},
			})
			if len(result) != 0 {
				t.Fatalf("suggestions = %#v, want none", result)
			}
		})
	}
}

func TestDiscoverReviewSuggestionsRejectsToolsOutsideJavaScriptAllowlist(t *testing.T) {
	result := discoverReviewSuggestions(context.Background(), []string{"python"}, reviewSuggestionDependencies{
		lookPath: func(string) (string, error) {
			t.Fatal("non-allowlisted tool reached PATH lookup")
			return "", nil
		},
		runQuery: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("non-allowlisted tool reached query runner")
			return nil, nil
		},
		pathExists: func(string) bool {
			t.Fatal("non-allowlisted tool reached cache existence check")
			return false
		},
	})
	if len(result) != 0 {
		t.Fatalf("suggestions = %#v, want none", result)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestDiscoverReviewSuggestionsDropsFailedAndTimedOutQueries(t *testing.T) {
	for _, test := range []struct {
		name     string
		runQuery func(context.Context, string, ...string) ([]byte, error)
	}{
		{
			name: "query failure",
			runQuery: func(context.Context, string, ...string) ([]byte, error) {
				return nil, errors.New("npm failed")
			},
		},
		{
			name: "query timeout",
			runQuery: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("query context has no deadline")
				}
				remaining := time.Until(deadline)
				if remaining <= 0 || remaining > toolQueryTimeout {
					t.Fatalf("query deadline remaining = %s, want within %s", remaining, toolQueryTimeout)
				}
				return nil, context.DeadlineExceeded
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := discoverReviewSuggestions(context.Background(), []string{"npm"}, reviewSuggestionDependencies{
				lookPath: func(string) (string, error) {
					return `C:\Program Files\nodejs\npm.cmd`, nil
				},
				runQuery: test.runQuery,
				pathExists: func(string) bool {
					t.Fatal("failed query reached cache existence check")
					return false
				},
			})
			if len(result) != 0 {
				t.Fatalf("suggestions = %#v, want none", result)
			}
		})
	}
}

func TestDiscoverReviewSuggestionsHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan []ReviewSuggestion, 1)
	go func() {
		done <- discoverReviewSuggestions(ctx, []string{"npm"}, reviewSuggestionDependencies{
			lookPath: func(string) (string, error) {
				return `C:\Program Files\nodejs\npm.cmd`, nil
			},
			runQuery: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			},
			pathExists: func(string) bool {
				t.Fatal("canceled query reached cache existence check")
				return false
			},
		})
	}()
	<-started
	cancel()

	select {
	case result := <-done:
		if len(result) != 0 {
			t.Fatalf("suggestions = %#v, want none", result)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled suggestion discovery blocked")
	}
}
