package clean

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePuppeteerCachePaths(t *testing.T) {
	deps := devCachePathDependencies{
		lookupEnv: func(key string) (string, bool) {
			switch key {
			case "PUPPETEER_CACHE_DIR":
				return `D:\custom\puppeteer`, true
			case "USERPROFILE":
				return `C:\Users\test`, true
			default:
				return "", false
			}
		},
		userHomeDir: func() (string, error) { return `C:\Users\test`, nil },
		joinPath:    func(parts ...string) string { return strings.Join(parts, `\`) },
		goos:        "windows",
	}
	paths := resolvePuppeteerCachePaths(deps)
	if len(paths) != 1 || paths[0] != `D:\custom\puppeteer` {
		t.Fatalf("override paths = %#v", paths)
	}

	deps.lookupEnv = func(key string) (string, bool) {
		switch key {
		case "PUPPETEER_CACHE_DIR":
			return "  \t  ", true
		case "USERPROFILE":
			return `C:\Users\test`, true
		default:
			return "", false
		}
	}
	paths = resolvePuppeteerCachePaths(deps)
	wantDefault := `C:\Users\test\.cache\puppeteer`
	if len(paths) != 1 || paths[0] != wantDefault {
		t.Fatalf("blank override paths = %#v, want %q", paths, wantDefault)
	}

	deps.lookupEnv = func(key string) (string, bool) {
		if key == "USERPROFILE" {
			return `C:\Users\test`, true
		}
		return "", false
	}
	paths = resolvePuppeteerCachePaths(deps)
	if len(paths) != 1 || paths[0] != wantDefault {
		t.Fatalf("default paths = %#v, want %q", paths, wantDefault)
	}

	deps.lookupEnv = func(string) (string, bool) { return "", false }
	deps.userHomeDir = func() (string, error) { return "", os.ErrNotExist }
	if paths = resolvePuppeteerCachePaths(deps); len(paths) != 0 {
		t.Fatalf("missing home paths = %#v, want nil", paths)
	}
}

func TestIsPuppeteerWindowsPlatformVersionDir(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"win64-131.0.6778.204", true},
		{"win32-1.0.0", true},
		{"linux-1.0.0", false},
		{"mac-1.0.0", false},
		{"mac_arm-1.0.0", false},
		{"linux_arm-1.0.0", false},
		{"win64", false},
		{"win64-", false},
		{"-1.0.0", false},
		{"win64-1.0.0-extra", false},
		{"WIN64-1.0.0", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isPuppeteerWindowsPlatformVersionDir(tt.name); got != tt.want {
			t.Errorf("isPuppeteerWindowsPlatformVersionDir(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestDiscoverPuppeteerBrowserChildrenDeterministicOrder(t *testing.T) {
	root := t.TempDir()
	// Create out of order; discovery must sort product then version.
	for _, rel := range []string{
		filepath.Join("firefox", "win64-2.0.0"),
		filepath.Join("chrome", "win64-2.0.0"),
		filepath.Join("chrome", "win64-1.0.0"),
		filepath.Join("chrome-headless-shell", "win32-1.0.0"),
	} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0700); err != nil {
			t.Fatal(err)
		}
	}
	children := discoverPuppeteerBrowserChildren(context.Background(), root)
	want := []string{
		filepath.Join(root, "chrome", "win64-1.0.0"),
		filepath.Join(root, "chrome", "win64-2.0.0"),
		filepath.Join(root, "chrome-headless-shell", "win32-1.0.0"),
		filepath.Join(root, "firefox", "win64-2.0.0"),
	}
	if len(children) != len(want) {
		t.Fatalf("children = %#v, want %#v", children, want)
	}
	for i := range want {
		if children[i] != want[i] {
			t.Fatalf("children[%d] = %q, want %q (all=%#v)", i, children[i], want[i], children)
		}
	}
	// Root and product parents must not appear.
	for _, child := range children {
		if child == root {
			t.Fatal("root returned as child")
		}
		base := filepath.Base(child)
		if base == "chrome" || base == "firefox" || base == "chrome-headless-shell" {
			t.Fatalf("product parent returned: %q", child)
		}
	}
}

func TestCategoryHasStructuredDevCacheDiscoveryForPuppeteer(t *testing.T) {
	if !categoryHasStructuredDevCacheDiscovery(DevCacheCategoryPuppeteerBrowsers) {
		t.Fatal("puppeteer-browsers must register structured child discovery")
	}
	// Existing whole-root categories remain unstructured.
	if categoryHasStructuredDevCacheDiscovery(DevCacheCategoryNPM) {
		t.Fatal("npm-cache must stay whole-root")
	}
}
