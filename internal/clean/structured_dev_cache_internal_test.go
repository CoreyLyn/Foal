package clean

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsStrictDescendantPath(t *testing.T) {
	root := `C:\Users\dev\cache`
	tests := []struct {
		path string
		want bool
	}{
		{root, false},
		{`C:\Users\dev\cache\`, false},
		{`C:\Users\dev\cache\child`, true},
		{`C:\Users\dev\cache\child\nested`, true},
		{`C:\Users\dev\cache-other`, false},
		{`C:\Users\dev`, false},
		{`C:\Users\dev\cache\..\escape`, false},
		{`D:\Users\dev\cache\child`, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isStrictDescendantPath(root, tt.path); got != tt.want {
			t.Errorf("isStrictDescendantPath(%q, %q) = %v, want %v", root, tt.path, got, tt.want)
		}
	}
	// Case-insensitive Windows identity.
	if !isStrictDescendantPath(`C:\Cache`, `c:\cache\Child`) {
		t.Fatal("expected case-insensitive strict descendant match")
	}
}

func TestEvaluateStructuredDevCacheChildRejectsUnsafe(t *testing.T) {
	root := t.TempDir()
	childDir := filepath.Join(root, "ok")
	if err := os.Mkdir(childDir, 0700); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "file.bin")
	if err := os.WriteFile(filePath, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	if safety := evaluateStructuredDevCacheChild("npm-cache", root, childDir, os.Lstat); !safety.ok {
		t.Fatalf("expected ok for directory child, got %#v", safety)
	}
	if safety := evaluateStructuredDevCacheChild("npm-cache", root, root, os.Lstat); safety.ok {
		t.Fatal("root must not be a structured candidate")
	}
	if safety := evaluateStructuredDevCacheChild("npm-cache", root, filePath, os.Lstat); safety.ok {
		t.Fatal("file must not be a structured candidate")
	}
	outside := filepath.Join(filepath.Dir(root), "outside")
	if safety := evaluateStructuredDevCacheChild("npm-cache", root, outside, os.Lstat); safety.ok {
		t.Fatal("outside path must not be a structured candidate")
	}
	missing := filepath.Join(root, "gone")
	if safety := evaluateStructuredDevCacheChild("npm-cache", root, missing, os.Lstat); safety.ok || !safety.missing {
		t.Fatalf("missing child safety = %#v, want missing", safety)
	}
}

func TestEvaluateStructuredDevCacheChildRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if safety := evaluateStructuredDevCacheChild("npm-cache", root, link, os.Lstat); safety.ok {
		t.Fatal("symlink/reparse must not be a structured candidate")
	}
}

func TestAppendStructuredDevCacheCandidatesInspectionLimit(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "big")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	// walkDir reports enough descendants to trip the ceiling.
	walkCalls := 0
	fakeWalk := func(path string, fn fs.WalkDirFunc) error {
		if err := fn(path, fakeDirEntry{name: filepath.Base(path), mode: os.ModeDir}, nil); err != nil {
			return err
		}
		for i := 0; i < 3; i++ {
			walkCalls++
			name := filepath.Join(path, "f")
			if err := fn(name, fakeDirEntry{name: "f", mode: 0}, nil); err != nil {
				return err
			}
		}
		return nil
	}

	var res optInResolution
	appendStructuredDevCacheCandidates(
		context.Background(),
		Options{},
		&res,
		DevCacheCategoryNPM,
		root,
		[]string{child},
		structuredDevCacheMeasureDependencies{
			lstat: os.Lstat,
			walkDir: fakeWalk,
			descendantLimit: 2,
		},
	)
	if len(res.candidates) != 0 {
		t.Fatalf("candidates = %#v, want none over inspection ceiling", res.candidates)
	}
	if len(res.diagnostics) == 0 {
		t.Fatal("expected inspection-limit diagnostic")
	}
	if res.diagnostics[0].Code != "inspection_limit_exceeded" {
		t.Fatalf("diagnostic code = %q, want inspection_limit_exceeded", res.diagnostics[0].Code)
	}
	if walkCalls == 0 {
		t.Fatal("expected walk to run")
	}
}

func TestAppendStructuredDevCacheCandidatesCanceledSiblingIndependence(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "d.bin"), []byte("ab"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var res optInResolution
	calls := 0
	fakeWalk := func(path string, fn fs.WalkDirFunc) error {
		calls++
		if calls == 1 {
			// First child measures successfully.
			return filepath.WalkDir(path, fn)
		}
		// Second child is canceled mid-measure.
		cancel()
		return context.Canceled
	}

	appendStructuredDevCacheCandidates(
		ctx,
		Options{},
		&res,
		DevCacheCategoryNPM,
		root,
		[]string{first, second},
		structuredDevCacheMeasureDependencies{
			lstat:   os.Lstat,
			walkDir: fakeWalk,
		},
	)

	if len(res.candidates) != 1 || res.candidates[0].Path != first {
		t.Fatalf("candidates = %#v, want first complete sibling retained", res.candidates)
	}
	if len(res.diagnostics) == 0 {
		t.Fatal("expected cancel diagnostic for second child")
	}
}

func TestCanonicalStructuredDevCachePolicyIsPlaywrightAndPuppeteer(t *testing.T) {
	// Structured child discovery: playwright-browsers, puppeteer-browsers, and
	// jetbrains-ide-caches. Whole-root categories stay nil.
	structured := map[string]bool{
		DevCacheCategoryPlaywright:         true,
		DevCacheCategoryPuppeteerBrowsers:  true,
		DevCacheCategoryJetBrainsIDECaches: true,
	}
	for _, id := range developerCacheCategoryIDs() {
		has := categoryHasStructuredDevCacheDiscovery(id)
		if structured[id] {
			if !has {
				t.Fatalf("%s must register structured child discovery", id)
			}
			continue
		}
		if has {
			t.Fatalf("unexpected structured policy on %q", id)
		}
	}
}

func TestValidateDeveloperCacheRegistryRejectsChildDiscoveryOnNonDevCache(t *testing.T) {
	entries := []categoryCatalogEntry{{
		definition: categoryDefinition(
			"user_temp", "User temp", ReportCategoryUserEssentials,
			CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable,
			DeletionActionMoveToRecycleBin,
		),
		opportunity: true,
		discoverChildren: func(context.Context, string) []string {
			return nil
		},
	}}
	err := validateDeveloperCacheRegistry(entries, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "child candidate discovery") {
		t.Fatalf("error = %v, want child candidate discovery rejection", err)
	}
}

func TestDeveloperCacheEntryWithChildrenBindsPolicy(t *testing.T) {
	discover := func(context.Context, string) []string {
		return []string{`C:\root\child`}
	}
	entry := developerCacheEntryWithChildren(
		categoryDefinition(
			"future-structured", "Future structured", ReportCategoryDeveloperTools,
			CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime,
			DeletionActionMoveToRecycleBin,
		),
		func(devCachePathDependencies) []string { return []string{`C:\root`} },
		discover,
		nil,
	)
	if !entry.developerCache || entry.discoverChildren == nil {
		t.Fatalf("entry missing structured policy: %#v", entry)
	}
	if got := entry.discoverChildren(context.Background(), `C:\root`); len(got) != 1 {
		t.Fatalf("discoverChildren = %#v", got)
	}
}

