package clean

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeAndDeduplicatePathsWindowsIdentity(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{
			name:  "empty input",
			paths: []string{},
			want:  []string{},
		},
		{
			name:  "empty and whitespace paths discarded",
			paths: []string{"", "   ", "\t", `C:\Valid`},
			want:  []string{`C:\Valid`},
		},
		{
			name:  "case variants deduplicated, first spelling preserved",
			paths: []string{`C:\Cache`, `c:\cache`, `C:\CACHE`},
			want:  []string{`C:\Cache`},
		},
		{
			name:  "long path prefix variants deduplicated",
			paths: []string{`C:\Cache`, `\\?\C:\Cache`, `C:\Cache`},
			want:  []string{`C:\Cache`},
		},
		{
			name:  "trailing separator variants deduplicated",
			paths: []string{`C:\Cache\`, `C:\Cache`, `C:\Cache\\`},
			want:  []string{`C:\Cache`},
		},
		{
			name:  "redundant separator variants deduplicated",
			paths: []string{`C:\\Users\\Corey\\Cache`, `C:\Users\Corey\Cache`},
			want:  []string{`C:\Users\Corey\Cache`},
		},
		{
			name:  "forward slash variants deduplicated",
			paths: []string{`C:/Users/Corey/Cache`, `C:\Users\Corey\Cache`},
			want:  []string{`C:\Users\Corey\Cache`},
		},
		{
			name:  "mixed case/separator/prefix variants deduplicated, first preserved",
			paths: []string{`C:\Users\Corey\Cache`, `\\?\C:/Users/corey/CACHE\`, `c:\users\corey\cache`},
			want:  []string{`C:\Users\Corey\Cache`},
		},
		{
			name:  "distinct paths kept separately",
			paths: []string{`C:\Cache1`, `C:\Cache2`, `c:\cache1`},
			want:  []string{`C:\Cache1`, `C:\Cache2`},
		},
		{
			name:  "sibling paths not treated as duplicates",
			paths: []string{`C:\Users\Corey\Cache`, `C:\Users\Other\Cache`},
			want:  []string{`C:\Users\Corey\Cache`, `C:\Users\Other\Cache`},
		},
		{
			name:  "parent and child paths not treated as duplicates",
			paths: []string{`C:\Users\Corey`, `C:\Users\Corey\Cache`},
			want:  []string{`C:\Users\Corey`, `C:\Users\Corey\Cache`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAndDeduplicatePaths(tt.paths)
			if len(got) != len(tt.want) {
				t.Fatalf("len(got) = %d, want %d\ngot: %q\nwant: %q", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDevCachePathResolution(t *testing.T) {
	t.Run("env-wins: npm uses NPM_CONFIG_CACHE only when set", func(t *testing.T) {
		// Env set: only env path is returned
		deps := devCachePathDependencies{
			lookupEnv: func(key string) (string, bool) {
				if key == "NPM_CONFIG_CACHE" {
					return "C:\\custom\\npm-cache", true
				}
				if key == "LOCALAPPDATA" {
					return "C:\\Users\\test\\AppData\\Local", true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, "\\")
			},
		}
		paths := resolveDevCachePaths(DevCacheCategoryNPM, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\custom\\npm-cache" {
			t.Errorf("expected env path, got %q", paths[0])
		}

		// Env not set: only default is returned
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "LOCALAPPDATA" {
				return "C:\\Users\\test\\AppData\\Local", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryNPM, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\AppData\\Local\\npm-cache" {
			t.Errorf("expected default path, got %q", paths[0])
		}
	})

	t.Run("env-wins: go uses GOCACHE only when set", func(t *testing.T) {
		// Env set: only env path is returned
		deps := devCachePathDependencies{
			lookupEnv: func(key string) (string, bool) {
				if key == "GOCACHE" {
					return "C:\\custom\\go-build", true
				}
				if key == "LOCALAPPDATA" {
					return "C:\\Users\\test\\AppData\\Local", true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, "\\")
			},
		}
		paths := resolveDevCachePaths(DevCacheCategoryGo, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\custom\\go-build" {
			t.Errorf("expected env path, got %q", paths[0])
		}

		// Env not set: only default is returned
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "LOCALAPPDATA" {
				return "C:\\Users\\test\\AppData\\Local", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryGo, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\AppData\\Local\\go-build" {
			t.Errorf("expected default path, got %q", paths[0])
		}
	})

	t.Run("env-wins: pip uses PIP_CACHE_DIR only when set", func(t *testing.T) {
		// Env set: only env path is returned
		deps := devCachePathDependencies{
			lookupEnv: func(key string) (string, bool) {
				if key == "PIP_CACHE_DIR" {
					return "C:\\custom\\pip-cache", true
				}
				if key == "LOCALAPPDATA" {
					return "C:\\Users\\test\\AppData\\Local", true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, "\\")
			},
		}
		paths := resolveDevCachePaths(DevCacheCategoryPip, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\custom\\pip-cache" {
			t.Errorf("expected env path, got %q", paths[0])
		}

		// Env not set: only default is returned
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "LOCALAPPDATA" {
				return "C:\\Users\\test\\AppData\\Local", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryPip, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\AppData\\Local\\pip\\Cache" {
			t.Errorf("expected default path, got %q", paths[0])
		}
	})

	t.Run("env-wins: nuget uses NUGET_HTTP_CACHE_PATH only when set", func(t *testing.T) {
		// Env set: only env path is returned
		deps := devCachePathDependencies{
			lookupEnv: func(key string) (string, bool) {
				if key == "NUGET_HTTP_CACHE_PATH" {
					return "C:\\custom\\nuget-cache", true
				}
				if key == "LOCALAPPDATA" {
					return "C:\\Users\\test\\AppData\\Local", true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, "\\")
			},
		}
		paths := resolveDevCachePaths(DevCacheCategoryNuGet, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\custom\\nuget-cache" {
			t.Errorf("expected env path, got %q", paths[0])
		}

		// Env not set: only default is returned
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "LOCALAPPDATA" {
				return "C:\\Users\\test\\AppData\\Local", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryNuGet, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\AppData\\Local\\NuGet\\v3-cache" {
			t.Errorf("expected default path, got %q", paths[0])
		}
	})

	t.Run("env-wins: corepack uses COREPACK_HOME only when set", func(t *testing.T) {
		// Env set: only env path is returned
		deps := devCachePathDependencies{
			lookupEnv: func(key string) (string, bool) {
				if key == "COREPACK_HOME" {
					return "C:\\custom\\corepack", true
				}
				if key == "LOCALAPPDATA" {
					return "C:\\Users\\test\\AppData\\Local", true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, "\\")
			},
		}
		paths := resolveDevCachePaths(DevCacheCategoryCorepack, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\custom\\corepack\\v1" {
			t.Errorf("expected env path, got %q", paths[0])
		}

		// Env not set: only default is returned
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "LOCALAPPDATA" {
				return "C:\\Users\\test\\AppData\\Local", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryCorepack, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\AppData\\Local\\node\\corepack\\v1" {
			t.Errorf("expected default path, got %q", paths[0])
		}
	})

	t.Run("env-wins: nuget global packages uses NUGET_PACKAGES only when set", func(t *testing.T) {
		// Env set: only env path is returned
		deps := devCachePathDependencies{
			lookupEnv: func(key string) (string, bool) {
				if key == "NUGET_PACKAGES" {
					return "C:\\custom\\nuget-global", true
				}
				if key == "USERPROFILE" {
					return "C:\\Users\\test", true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, "\\")
			},
		}
		paths := resolveDevCachePaths(DevCacheCategoryNuGetGlobalPackages, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\custom\\nuget-global" {
			t.Errorf("expected env path, got %q", paths[0])
		}

		// Env not set: default to USERPROFILE\.nuget\packages
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "USERPROFILE" {
				return "C:\\Users\\test", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryNuGetGlobalPackages, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\.nuget\\packages" {
			t.Errorf("expected default USERPROFILE path, got %q", paths[0])
		}

		// No USERPROFILE but userHomeDir works
		deps.lookupEnv = func(key string) (string, bool) {
			return "", false
		}
		deps.userHomeDir = func() (string, error) {
			return "C:\\Users\\testhome", nil
		}
		paths = resolveDevCachePaths(DevCacheCategoryNuGetGlobalPackages, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path from userHomeDir, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\testhome\\.nuget\\packages" {
			t.Errorf("expected userHomeDir path, got %q", paths[0])
		}

		// No env or userHomeDir: no paths
		deps.lookupEnv = func(key string) (string, bool) {
			return "", false
		}
		deps.userHomeDir = func() (string, error) {
			return "", os.ErrNotExist
		}
		paths = resolveDevCachePaths(DevCacheCategoryNuGetGlobalPackages, deps)
		if len(paths) != 0 {
			t.Fatalf("expected 0 paths when no resolution, got %d", len(paths))
		}
	})

	t.Run("env-wins: uv uses non-empty UV_CACHE_DIR only when set", func(t *testing.T) {
		deps := devCachePathDependencies{
			lookupEnv: func(key string) (string, bool) {
				if key == "UV_CACHE_DIR" {
					return "C:\\custom\\uv-cache", true
				}
				if key == "LOCALAPPDATA" {
					return "C:\\Users\\test\\AppData\\Local", true
				}
				return "", false
			},
			joinPath: func(parts ...string) string {
				return strings.Join(parts, "\\")
			},
		}
		paths := resolveDevCachePaths(DevCacheCategoryUV, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\custom\\uv-cache" {
			t.Errorf("expected env path, got %q", paths[0])
		}

		// Whitespace-only UV_CACHE_DIR falls through to default.
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "UV_CACHE_DIR" {
				return "   ", true
			}
			if key == "LOCALAPPDATA" {
				return "C:\\Users\\test\\AppData\\Local", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryUV, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 default path after blank UV_CACHE_DIR, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\AppData\\Local\\uv\\cache" {
			t.Errorf("expected default path, got %q", paths[0])
		}

		// Env not set: Windows standard user cache under uv\cache.
		deps.lookupEnv = func(key string) (string, bool) {
			if key == "LOCALAPPDATA" {
				return "C:\\Users\\test\\AppData\\Local", true
			}
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryUV, deps)
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0] != "C:\\Users\\test\\AppData\\Local\\uv\\cache" {
			t.Errorf("expected default path, got %q", paths[0])
		}

		// Missing LOCALAPPDATA: no candidate root.
		deps.lookupEnv = func(key string) (string, bool) {
			return "", false
		}
		paths = resolveDevCachePaths(DevCacheCategoryUV, deps)
		if len(paths) != 0 {
			t.Fatalf("expected 0 paths when no resolution, got %d", len(paths))
		}
	})
}
