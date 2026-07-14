package clean

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDiscoverReviewSuggestionsQueriesInstalledToolCaches(t *testing.T) {
	tests := []struct {
		tool         string
		executable   string
		queryArgs    []string
		cleanCommand string
		cachePath    string
		queryOutput  string
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
		{
			tool:         "pip",
			executable:   `C:\Users\corey\AppData\Local\Programs\Python\Python313\Scripts\pip.exe`,
			queryArgs:    []string{"cache", "dir"},
			cleanCommand: "pip cache purge",
			cachePath:    `C:\Users\corey\AppData\Local\pip\Cache`,
		},
		{
			tool:         "uv",
			executable:   `C:\Users\corey\.local\bin\uv.exe`,
			queryArgs:    []string{"cache", "dir"},
			cleanCommand: "uv cache prune",
			cachePath:    `C:\Users\corey\AppData\Local\uv\cache`,
		},
		{
			tool:         "mise",
			executable:   `C:\Users\corey\AppData\Local\mise\bin\mise.exe`,
			queryArgs:    []string{"cache", "path"},
			cleanCommand: "mise cache clear",
			cachePath:    `C:\Users\corey\AppData\Local\mise`,
		},
		{
			tool:         "conda",
			executable:   `C:\Users\corey\miniconda3\Scripts\conda.exe`,
			queryArgs:    []string{"info", "--json"},
			cleanCommand: "conda clean --all",
			cachePath:    `D:\conda-cache\pkgs`,
			queryOutput:  `{"pkgs_dirs":["C:\\Users\\corey\\miniconda3\\pkgs","D:\\conda-cache\\pkgs"]}`,
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
					if test.queryOutput != "" {
						return []byte(test.queryOutput), nil
					}
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

func TestDiscoverReviewSuggestionsEmitsDistinctGoCacheSuggestions(t *testing.T) {
	executable := `C:\Program Files\Go\bin\go.exe`
	buildCache := `C:\Users\corey\AppData\Local\go-build`
	moduleCache := `C:\Users\corey\go\pkg\mod`
	queries := []string{}

	result := discoverReviewSuggestions(context.Background(), []string{"go"}, reviewSuggestionDependencies{
		lookPath: func(tool string) (string, error) {
			if tool != "go" {
				t.Fatalf("PATH lookup tool = %q, want go", tool)
			}
			return executable, nil
		},
		runQuery: func(_ context.Context, gotExecutable string, args ...string) ([]byte, error) {
			if gotExecutable != executable {
				t.Fatalf("query executable = %q, want %q", gotExecutable, executable)
			}
			queries = append(queries, strings.Join(args, " "))
			switch strings.Join(args, " ") {
			case "env GOCACHE":
				return []byte(buildCache + "\r\n"), nil
			case "env GOMODCACHE":
				return []byte(moduleCache + "\r\n"), nil
			default:
				t.Fatalf("unexpected query args: %#v", args)
				return nil, nil
			}
		},
		pathExists: func(path string) bool {
			return path == buildCache || path == moduleCache
		},
	})

	if !equalStrings(queries, []string{"env GOCACHE", "env GOMODCACHE"}) {
		t.Fatalf("queries = %#v, want exact Go cache queries", queries)
	}
	want := []ReviewSuggestion{
		{Tool: "go", Label: "Go build cache", Command: "go clean -cache", CachePath: buildCache},
		{Tool: "go", Label: "Go module cache", Command: "go clean -modcache", CachePath: moduleCache},
	}
	if len(result) != len(want) {
		t.Fatalf("suggestions = %#v, want %#v", result, want)
	}
	for index := range want {
		if result[index] != want[index] {
			t.Fatalf("suggestion %d = %#v, want %#v", index, result[index], want[index])
		}
	}
}

func TestDiscoverReviewSuggestionsResolvesCorepackHomeWithoutRunningQuery(t *testing.T) {
	cachePath := `D:\Corepack\v1`
	result := discoverReviewSuggestions(context.Background(), []string{"corepack"}, reviewSuggestionDependencies{
		lookPath: func(tool string) (string, error) {
			if tool != "corepack" {
				t.Fatalf("PATH lookup tool = %q, want corepack", tool)
			}
			return `C:\Program Files\nodejs\corepack.cmd`, nil
		},
		runQuery: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("Corepack must not run a cache path query")
			return nil, nil
		},
		lookupEnv: func(name string) (string, bool) {
			switch name {
			case "COREPACK_HOME":
				return `D:\Corepack`, true
			case "XDG_CACHE_HOME":
				return `D:\ignored-xdg`, true
			default:
				return "", false
			}
		},
		userHomeDir: func() (string, error) {
			t.Fatal("Corepack home fallback must not run when COREPACK_HOME is set")
			return "", nil
		},
		joinPath: func(parts ...string) string {
			return strings.Join(parts, `\`)
		},
		goos: "windows",
		pathExists: func(path string) bool {
			return path == cachePath
		},
	})

	want := ReviewSuggestion{
		Tool:      "corepack",
		Label:     "Corepack cache",
		Command:   "corepack cache clean",
		CachePath: cachePath,
	}
	if len(result) != 1 || result[0] != want {
		t.Fatalf("suggestions = %#v, want %#v", result, []ReviewSuggestion{want})
	}
}

func TestDiscoverReviewSuggestionsUsesOfficialCorepackFallbackChain(t *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		goos        string
		home        string
		want        string
	}{
		{
			name: "XDG cache precedes LOCALAPPDATA",
			environment: map[string]string{
				"XDG_CACHE_HOME": `D:\xdg-cache`,
				"LOCALAPPDATA":   `D:\local-cache`,
			},
			goos: "windows",
			home: `C:\Users\ignored`,
			want: `D:\xdg-cache\node\corepack\v1`,
		},
		{
			name: "LOCALAPPDATA fallback",
			environment: map[string]string{
				"LOCALAPPDATA": `D:\local-cache`,
			},
			goos: "windows",
			home: `C:\Users\ignored`,
			want: `D:\local-cache\node\corepack\v1`,
		},
		{
			name: "Windows user home fallback",
			goos: "windows",
			home: `C:\Users\corey`,
			want: `C:\Users\corey\AppData\Local\node\corepack\v1`,
		},
		{
			name: "non-Windows user home fallback",
			goos: "linux",
			home: `/home/corey`,
			want: `/home/corey/.cache/node/corepack/v1`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			separator := `\`
			if test.goos != "windows" {
				separator = "/"
			}
			result := discoverReviewSuggestions(context.Background(), []string{"corepack"}, reviewSuggestionDependencies{
				lookPath: func(string) (string, error) {
					return "corepack", nil
				},
				runQuery: func(context.Context, string, ...string) ([]byte, error) {
					t.Fatal("Corepack must not run a cache path query")
					return nil, nil
				},
				lookupEnv: func(name string) (string, bool) {
					value, found := test.environment[name]
					return value, found
				},
				userHomeDir: func() (string, error) {
					return test.home, nil
				},
				joinPath: func(parts ...string) string {
					return strings.Join(parts, separator)
				},
				goos: test.goos,
				pathExists: func(path string) bool {
					return path == test.want
				},
			})

			if len(result) != 1 || result[0].CachePath != test.want {
				t.Fatalf("suggestions = %#v, want Corepack cache at %q", result, test.want)
			}
		})
	}
}

func TestDiscoverReviewSuggestionsParsesLocalizedDotnetNugetCacheOutput(t *testing.T) {
	executable := `C:\Program Files\dotnet\dotnet.exe`
	globalPackages := `D:\NuGet\packages`
	checkedPaths := []string{}

	result := discoverReviewSuggestions(context.Background(), []string{"dotnet"}, reviewSuggestionDependencies{
		lookPath: func(tool string) (string, error) {
			if tool != "dotnet" {
				t.Fatalf("PATH lookup tool = %q, want dotnet", tool)
			}
			return executable, nil
		},
		runQuery: func(_ context.Context, gotExecutable string, args ...string) ([]byte, error) {
			if gotExecutable != executable {
				t.Fatalf("query executable = %q, want %q", gotExecutable, executable)
			}
			wantArgs := []string{"nuget", "locals", "all", "--list"}
			if !equalStrings(args, wantArgs) {
				t.Fatalf("query args = %#v, want only %#v", args, wantArgs)
			}
			return []byte("NuGet 本地资源:\r\n" +
				"http-cache: C:\\missing\\v3-cache\r\n" +
				"全局包: " + globalPackages + "\r\n" +
				"\r\n" +
				"diagnostic noise without a label\r\n"), nil
		},
		pathExists: func(path string) bool {
			checkedPaths = append(checkedPaths, path)
			return path == globalPackages
		},
	})

	if !equalStrings(checkedPaths, []string{`C:\missing\v3-cache`, globalPackages}) {
		t.Fatalf("checked paths = %#v, want labeled cache paths in output order", checkedPaths)
	}
	want := ReviewSuggestion{
		Tool:      "dotnet",
		Label:     ".NET NuGet caches",
		Command:   "dotnet nuget locals all --clear",
		CachePath: globalPackages,
	}
	if len(result) != 1 || result[0] != want {
		t.Fatalf("suggestions = %#v, want %#v", result, []ReviewSuggestion{want})
	}
}

func TestDiscoverReviewSuggestionsKeepsExistingGoCacheAfterOtherProbeFailsOrIsAbsent(t *testing.T) {
	moduleCache := `C:\Users\corey\go\pkg\mod`
	result := discoverReviewSuggestions(context.Background(), []string{"go"}, reviewSuggestionDependencies{
		lookPath: func(string) (string, error) {
			return `C:\Program Files\Go\bin\go.exe`, nil
		},
		runQuery: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			switch strings.Join(args, " ") {
			case "env GOCACHE":
				return nil, errors.New("GOCACHE query failed")
			case "env GOMODCACHE":
				return []byte(moduleCache), nil
			default:
				t.Fatalf("unexpected query args: %#v", args)
				return nil, nil
			}
		},
		pathExists: func(path string) bool {
			return path == moduleCache
		},
	})

	want := ReviewSuggestion{
		Tool:      "go",
		Label:     "Go module cache",
		Command:   "go clean -modcache",
		CachePath: moduleCache,
	}
	if len(result) != 1 || result[0] != want {
		t.Fatalf("suggestions = %#v, want only %#v", result, want)
	}
}

func TestDiscoverReviewSuggestionsRequiresPATHAndExistingCache(t *testing.T) {
	for _, tool := range []string{"npm", "pnpm", "yarn", "bun", "pip", "uv", "conda", "go", "dotnet", "corepack", "mise"} {
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
					if tool == "corepack" {
						t.Fatal("Corepack must not run a cache path query")
					}
					if tool == "conda" {
						return []byte(`{"pkgs_dirs":["C:\\missing\\conda-cache"]}`), nil
					}
					if tool == "dotnet" {
						return []byte("http-cache: C:\\missing\\nuget-http\r\nglobal-packages: C:\\missing\\nuget-packages"), nil
					}
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
				return nil, errors.New("mise failed")
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
			result := discoverReviewSuggestions(context.Background(), []string{"mise"}, reviewSuggestionDependencies{
				lookPath: func(string) (string, error) {
					return `C:\Users\corey\AppData\Local\mise\bin\mise.exe`, nil
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

func TestDiscoverReviewSuggestionsContinuesAfterOneToolFails(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "query failure", err: errors.New("pip failed")},
		{name: "query deadline", err: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			queries := []string{}
			result := discoverReviewSuggestions(context.Background(), []string{"pip", "uv"}, reviewSuggestionDependencies{
				lookPath: func(tool string) (string, error) {
					return tool + ".exe", nil
				},
				runQuery: func(_ context.Context, executable string, args ...string) ([]byte, error) {
					queries = append(queries, executable+" "+strings.Join(args, " "))
					if executable == "pip.exe" {
						return nil, test.err
					}
					return []byte("C:\\Users\\corey\\AppData\\Local\\uv\\cache\r\n"), nil
				},
				pathExists: func(path string) bool {
					return path == `C:\Users\corey\AppData\Local\uv\cache`
				},
			})

			wantQueries := []string{"pip.exe cache dir", "uv.exe cache dir"}
			if !equalStrings(queries, wantQueries) {
				t.Fatalf("queries = %#v, want %#v", queries, wantQueries)
			}
			if len(result) != 1 || result[0].Tool != "uv" {
				t.Fatalf("suggestions = %#v, want only uv after pip failure", result)
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

func TestDiscoverReviewSuggestionsBunPrefersQueryThenFallsBackToOfficialDefault(t *testing.T) {
	const (
		queryPath    = `D:\custom\bun-install-cache`
		defaultPath  = `C:\Users\corey\.bun\install\cache`
		bunExe       = `C:\Users\corey\.bun\bin\bun.exe`
	)

	t.Run("successful query prefers config-aware path", func(t *testing.T) {
		var queried bool
		result := discoverReviewSuggestions(context.Background(), []string{"bun"}, reviewSuggestionDependencies{
			lookPath: func(tool string) (string, error) {
				if tool != "bun" {
					t.Fatalf("lookPath tool = %q", tool)
				}
				return bunExe, nil
			},
			runQuery: func(_ context.Context, executable string, args ...string) ([]byte, error) {
				queried = true
				if executable != bunExe || !equalStrings(args, []string{"pm", "cache"}) {
					t.Fatalf("unexpected query: %s %#v", executable, args)
				}
				return []byte(queryPath + "\r\n"), nil
			},
			pathExists: func(path string) bool {
				return path == queryPath
			},
			lookupEnv: func(key string) (string, bool) {
				t.Fatal("fallback env lookup must not run when query path exists")
				return "", false
			},
		})
		if !queried {
			t.Fatal("expected bun pm cache query")
		}
		if len(result) != 1 || result[0].CachePath != queryPath || result[0].Command != "bun pm cache rm" {
			t.Fatalf("suggestions = %#v, want query path only", result)
		}
	})

	for _, test := range []struct {
		name       string
		queryErr   error
		queryOut   string
		existsOnly string
	}{
		{name: "no-project query failure", queryErr: errors.New("No package.json was found for directory"), existsOnly: defaultPath},
		{name: "query timeout", queryErr: context.DeadlineExceeded, existsOnly: defaultPath},
		{name: "query path missing falls back", queryOut: `D:\missing\bun-cache` + "\n", existsOnly: defaultPath},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := discoverReviewSuggestions(context.Background(), []string{"bun"}, reviewSuggestionDependencies{
				lookPath: func(string) (string, error) {
					return bunExe, nil
				},
				runQuery: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
					if test.queryErr != nil {
						return nil, test.queryErr
					}
					return []byte(test.queryOut), nil
				},
				pathExists: func(path string) bool {
					return path == test.existsOnly
				},
				lookupEnv: func(key string) (string, bool) {
					if key == "USERPROFILE" {
						return `C:\Users\corey`, true
					}
					return "", false
				},
				joinPath: func(parts ...string) string {
					return strings.Join(parts, `\`)
				},
			})
			if len(result) != 1 || result[0].CachePath != defaultPath {
				t.Fatalf("suggestions = %#v, want official default fallback %q", result, defaultPath)
			}
			if result[0].Tool != "bun" || result[0].Command != "bun pm cache rm" {
				t.Fatalf("suggestion = %#v, want bun review suggestion command", result[0])
			}
		})
	}

	t.Run("env BUN_INSTALL_CACHE_DIR wins on fallback", func(t *testing.T) {
		const envPath = `E:\bun\install-cache`
		result := discoverReviewSuggestions(context.Background(), []string{"bun"}, reviewSuggestionDependencies{
			lookPath: func(string) (string, error) {
				return bunExe, nil
			},
			runQuery: func(context.Context, string, ...string) ([]byte, error) {
				return nil, errors.New("No package.json was found for directory")
			},
			pathExists: func(path string) bool {
				return path == envPath
			},
			lookupEnv: func(key string) (string, bool) {
				if key == "BUN_INSTALL_CACHE_DIR" {
					return envPath, true
				}
				if key == "USERPROFILE" {
					return `C:\Users\corey`, true
				}
				return "", false
			},
		})
		if len(result) != 1 || result[0].CachePath != envPath {
			t.Fatalf("suggestions = %#v, want env fallback %q", result, envPath)
		}
	})

	t.Run("fallback root missing yields no suggestion", func(t *testing.T) {
		result := discoverReviewSuggestions(context.Background(), []string{"bun"}, reviewSuggestionDependencies{
			lookPath: func(string) (string, error) {
				return bunExe, nil
			},
			runQuery: func(context.Context, string, ...string) ([]byte, error) {
				return nil, errors.New("No package.json was found for directory")
			},
			pathExists: func(string) bool {
				return false
			},
			lookupEnv: func(key string) (string, bool) {
				if key == "USERPROFILE" {
					return `C:\Users\corey`, true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, `\`)
			},
		})
		if len(result) != 0 {
			t.Fatalf("suggestions = %#v, want none when fallback missing", result)
		}
	})
}
