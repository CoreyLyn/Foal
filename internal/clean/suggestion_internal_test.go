package clean

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDiscoverReviewSuggestionsQueriesInstalledNPMCache(t *testing.T) {
	var gotExecutable string
	var gotArgs []string
	result := discoverReviewSuggestions(context.Background(), []string{"npm"}, reviewSuggestionDependencies{
		lookPath: func(tool string) (string, error) {
			if tool != "npm" {
				t.Fatalf("PATH lookup tool = %q, want npm", tool)
			}
			return `C:\Program Files\nodejs\npm.cmd`, nil
		},
		runQuery: func(_ context.Context, executable string, args ...string) ([]byte, error) {
			gotExecutable = executable
			gotArgs = append([]string(nil), args...)
			return []byte("C:\\Users\\corey\\AppData\\Local\\npm-cache\r\n"), nil
		},
		pathExists: func(path string) bool {
			return path == `C:\Users\corey\AppData\Local\npm-cache`
		},
	})

	if gotExecutable != `C:\Program Files\nodejs\npm.cmd` {
		t.Fatalf("query executable = %q, want resolved npm executable", gotExecutable)
	}
	if len(gotArgs) != 3 || gotArgs[0] != "config" || gotArgs[1] != "get" || gotArgs[2] != "cache" {
		t.Fatalf("query args = %#v, want only npm config get cache", gotArgs)
	}
	if len(result) != 1 {
		t.Fatalf("suggestions = %#v, want one npm suggestion", result)
	}
	suggestion := result[0]
	if suggestion.Tool != "npm" ||
		suggestion.Label != "npm cache" ||
		suggestion.Command != "npm cache clean --force" ||
		suggestion.CachePath != `C:\Users\corey\AppData\Local\npm-cache` {
		t.Fatalf("suggestion = %#v, want fixed npm review suggestion", suggestion)
	}
}

func TestDiscoverReviewSuggestionsRequiresPATHAndExistingCache(t *testing.T) {
	t.Run("npm not on PATH", func(t *testing.T) {
		result := discoverReviewSuggestions(context.Background(), []string{"npm"}, reviewSuggestionDependencies{
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

	t.Run("npm cache absent", func(t *testing.T) {
		result := discoverReviewSuggestions(context.Background(), []string{"npm"}, reviewSuggestionDependencies{
			lookPath: func(string) (string, error) {
				return `C:\Program Files\nodejs\npm.cmd`, nil
			},
			runQuery: func(context.Context, string, ...string) ([]byte, error) {
				return []byte(`C:\missing\npm-cache`), nil
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

func TestDiscoverReviewSuggestionsRejectsToolsOutsideNPMAllowlist(t *testing.T) {
	result := discoverReviewSuggestions(context.Background(), []string{"pnpm"}, reviewSuggestionDependencies{
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
