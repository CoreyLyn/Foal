package clean

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIsVisualStudioInstanceDirName(t *testing.T) {
	valid := []string{
		"14.0", "15.0", "16.0", "17.0", "18.0",
		"17.0_a4d9e95d", "18.0_ABCDEF01", "14.0_1",
	}
	for _, name := range valid {
		if !isVisualStudioInstanceDirName(name) {
			t.Fatalf("%q should be a valid instance dir", name)
		}
	}
	invalid := []string{
		"", "Roslyn", "Settings", "Packages", "BackupFiles",
		"13.0", "13.0_deadbeef", "14", "14.0.1", "14.0-backup",
		"My17.0", "17.0_", "17.0_not-hex!", "017.0", "17.01",
		"17.0_g", // non-hex
	}
	for _, name := range invalid {
		if isVisualStudioInstanceDirName(name) {
			t.Fatalf("%q must fail closed", name)
		}
	}
}

func TestDiscoverVisualStudioCacheChildren_OrderAndExclusions(t *testing.T) {
	root := t.TempDir()
	// Shared Roslyn.
	if err := os.MkdirAll(filepath.Join(root, "Roslyn"), 0700); err != nil {
		t.Fatal(err)
	}
	// Instances out of order on disk.
	for _, inst := range []string{"18.0_bb", "16.0", "17.0_aa"} {
		if err := os.MkdirAll(filepath.Join(root, inst, "ComponentModelCache"), 0700); err != nil {
			t.Fatal(err)
		}
		// Non-allowlisted under instance.
		if err := os.MkdirAll(filepath.Join(root, inst, "Settings"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// Decoys at parent.
	for _, decoy := range []string{"Settings", "MEFCacheBackup", "13.0", "WebView2Cache"} {
		if err := os.MkdirAll(filepath.Join(root, decoy), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// Instance-level Roslyn ignored.
	if err := os.MkdirAll(filepath.Join(root, "18.0_bb", "Roslyn"), 0700); err != nil {
		t.Fatal(err)
	}

	got := discoverVisualStudioCacheChildren(context.Background(), root)
	want := []string{
		filepath.Join(root, "Roslyn"),
		filepath.Join(root, "16.0", "ComponentModelCache"),
		filepath.Join(root, "17.0_aa", "ComponentModelCache"),
		filepath.Join(root, "18.0_bb", "ComponentModelCache"),
	}
	if len(got) != len(want) {
		t.Fatalf("children = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveVisualStudioCacheRootScopes_SilentAbsence(t *testing.T) {
	deps := devCachePathDependencies{
		lookupEnv: func(string) (string, bool) { return "", false },
		joinPath:  filepath.Join,
	}
	if scopes := resolveVisualStudioCacheRootScopes(deps); len(scopes) != 0 {
		t.Fatalf("missing env scopes = %#v", scopes)
	}
	deps.lookupEnv = func(string) (string, bool) { return "   ", true }
	if scopes := resolveVisualStudioCacheRootScopes(deps); len(scopes) != 0 {
		t.Fatalf("blank env scopes = %#v", scopes)
	}

	local := t.TempDir()
	deps.lookupEnv = func(key string) (string, bool) {
		if key == "LOCALAPPDATA" {
			return local, true
		}
		return "", false
	}
	// No Microsoft\VisualStudio under local.
	if scopes := resolveVisualStudioCacheRootScopes(deps); len(scopes) != 0 {
		t.Fatalf("missing parent scopes = %#v", scopes)
	}

	parent := filepath.Join(local, "Microsoft", "VisualStudio")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	scopes := resolveVisualStudioCacheRootScopes(deps)
	if len(scopes) != 1 || scopes[0].Path != parent || scopes[0].Application != ApplicationVisualStudio {
		t.Fatalf("scopes = %#v, want parent with visual_studio", scopes)
	}
}

func TestVisualStudioCaches_CatalogEntryStructured(t *testing.T) {
	entry, ok := canonicalCategoryEntry(DevCacheCategoryVisualStudioCaches)
	if !ok {
		t.Fatal("visual-studio-caches missing from catalog")
	}
	if entry.resolverKind != categoryResolverDeveloperCache {
		t.Fatalf("resolver kind = %q", entry.resolverKind)
	}
	if entry.resolveRootScopes == nil || entry.discoverChildren == nil {
		t.Fatal("expected product-scoped structured discovery")
	}
	if entry.previewSafetyNote == nil {
		t.Fatal("expected preview safety note")
	}
	if len(entry.runningApplications) != 1 || entry.runningApplications[0] != ApplicationVisualStudio {
		t.Fatalf("running applications = %#v", entry.runningApplications)
	}
	if len(entry.reviewSuggestionTools) != 0 {
		t.Fatalf("review tools = %#v, want none", entry.reviewSuggestionTools)
	}
}
