package clean_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

// Research fixtures from docs/research/explorer-thumbnail-and-inet-cache-allowlists.md (#235/#239).

func writeExplorerThumbnailAllowlistedFiles(t *testing.T, localAppData string) (parent string, allowlisted []string) {
	t.Helper()
	parent = filepath.Join(localAppData, "Microsoft", "Windows", "Explorer")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"thumbcache_256.db",
		"thumbcache_idx.db",
		"iconcache_32.db",
		"ICONCACHE_96.DB", // case-insensitive match
	} {
		path := filepath.Join(parent, name)
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		allowlisted = append(allowlisted, path)
	}
	// Non-allowlisted siblings under Explorer (must never become candidates).
	for _, name := range []string{
		"ExplorerStartupLog.etl",
		"ExplorerStartupLog_RunOnce.etl",
		"RecommendationsFilterList.json",
		"thumbcache.db", // no underscore segment
		"iconcache.db",
		"other.txt",
	} {
		if err := os.WriteFile(filepath.Join(parent, name), []byte("excluded"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Nested decoy under Explorer must not expand scope.
	nested := filepath.Join(parent, "nested", "thumbcache_999.db")
	if err := os.MkdirAll(filepath.Dir(nested), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("nested"), 0600); err != nil {
		t.Fatal(err)
	}
	// Legacy IconCache.db outside Explorer is out of category.
	legacy := filepath.Join(localAppData, "IconCache.db")
	if err := os.WriteFile(legacy, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	return parent, allowlisted
}

func writeINetCacheAllowlistedRoots(t *testing.T, localAppData string) (ie, lowIE string) {
	t.Helper()
	ie = filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "IE")
	lowIE = filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "Low", "IE")
	for _, root := range []string{ie, lowIE} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(ie, "container.dat"), []byte("ie-cache"), 0600); err != nil {
		t.Fatal(err)
	}
	hashDir := filepath.Join(ie, "ABCD1234")
	if err := os.MkdirAll(hashDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hashDir, "file[1].dat"), []byte("blob"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lowIE, "low.bin"), []byte("low"), 0600); err != nil {
		t.Fatal(err)
	}
	// Non-allowlisted siblings under INetCache (must never become candidates).
	inetRoot := filepath.Join(localAppData, "Microsoft", "Windows", "INetCache")
	for _, rel := range [][]string{
		{"Low", "SuggestedSites.dat"},
		{"Virtualized", "note.txt"},
		{"Content.MSO", "doc.xlsx"},
		{"thumbnails", "preview.png"},
	} {
		path := filepath.Join(append([]string{inetRoot}, rel...)...)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("excluded"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Sibling WebCache tree is out of category entirely.
	webCache := filepath.Join(localAppData, "Microsoft", "Windows", "WebCache", "WebCacheV01.dat")
	if err := os.MkdirAll(filepath.Dir(webCache), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webCache, []byte("webcache"), 0600); err != nil {
		t.Fatal(err)
	}
	return ie, lowIE
}

func TestExplorerThumbnailCache_CatalogPermanentAndInitiallySelected(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.OpportunityCategoryExplorerThumbnailCache)
	if !ok {
		t.Fatal("explorer_thumbnail_cache missing from catalog")
	}
	if summary.Eligibility != clean.CategoryEligibilityOptIn {
		t.Fatalf("eligibility = %q", summary.Eligibility)
	}
	if summary.PlannedAction != clean.PlannedActionDeletePermanently {
		t.Fatalf("planned_action = %q, want delete_permanently", summary.PlannedAction)
	}
	if !clean.InitiallySelectedCategory(summary) {
		t.Fatal("permanent explorer_thumbnail_cache must start selected")
	}
	if summary.RunningApplicationPolicy != clean.RunningApplicationPolicyNotApplicable {
		t.Fatalf("running policy = %q", summary.RunningApplicationPolicy)
	}
	enabled, invalid, _ := clean.NormalizedOptInSet([]string{clean.OpportunityCategoryExplorerThumbnailCache})
	if len(invalid) != 0 || !enabled[clean.OpportunityCategoryExplorerThumbnailCache] {
		t.Fatalf("opt-in = %#v %#v", enabled, invalid)
	}
}

func TestINetCache_CatalogPermanentAndInitiallySelected(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.OpportunityCategoryINetCache)
	if !ok {
		t.Fatal("inet_cache missing from catalog")
	}
	if summary.PlannedAction != clean.PlannedActionDeletePermanently {
		t.Fatalf("planned_action = %q, want delete_permanently", summary.PlannedAction)
	}
	if !clean.InitiallySelectedCategory(summary) {
		t.Fatal("permanent inet_cache must start selected")
	}
	enabled, invalid, _ := clean.NormalizedOptInSet([]string{clean.OpportunityCategoryINetCache})
	if len(invalid) != 0 || !enabled[clean.OpportunityCategoryINetCache] {
		t.Fatalf("opt-in = %#v %#v", enabled, invalid)
	}
}

func TestExplorerThumbnailCache_DiscoverAllowlistedDBFilesOnly(t *testing.T) {
	localAppData := t.TempDir()
	parent, allowlisted := writeExplorerThumbnailAllowlistedFiles(t, localAppData)

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: localAppData,
		Categories:      []string{clean.OpportunityCategoryExplorerThumbnailCache},
	})
	if len(result.Incomplete) != 0 {
		t.Fatalf("incomplete = %#v", result.Incomplete)
	}
	if len(result.Opportunities) != len(allowlisted) {
		t.Fatalf("opportunities = %#v, want %d allowlisted files", result.Opportunities, len(allowlisted))
	}
	seen := map[string]bool{}
	for _, opp := range result.Opportunities {
		if opp.Category != clean.OpportunityCategoryExplorerThumbnailCache {
			t.Fatalf("category = %q", opp.Category)
		}
		if opp.Path == parent {
			t.Fatal("whole Explorer root must never be a candidate")
		}
		if strings.Contains(opp.Path, "ExplorerStartupLog") ||
			strings.Contains(opp.Path, "RecommendationsFilterList") ||
			strings.Contains(opp.Path, "IconCache.db") ||
			strings.Contains(opp.Path, "nested") {
			t.Fatalf("non-allowlisted path became candidate: %q", opp.Path)
		}
		base := filepath.Base(opp.Path)
		lower := strings.ToLower(base)
		if !(strings.HasPrefix(lower, "thumbcache_") || strings.HasPrefix(lower, "iconcache_")) ||
			!strings.HasSuffix(lower, ".db") {
			t.Fatalf("unexpected basename candidate %q", base)
		}
		if opp.Bytes <= 0 {
			t.Fatalf("bytes = %d for %q", opp.Bytes, opp.Path)
		}
		if !opp.LatestModifiedAt.IsZero() || opp.IdleDays != 0 {
			t.Fatalf("must not emit age fields: %#v", opp)
		}
		seen[opp.Path] = true
	}
	for _, path := range allowlisted {
		if !seen[path] {
			t.Fatalf("missing allowlisted file %q in %#v", path, result.Opportunities)
		}
	}
}

func TestExplorerThumbnailCache_MissingAllowlistedFilesEmptyNoWholeRootFallback(t *testing.T) {
	localAppData := t.TempDir()
	// Parent exists with only non-allowlisted siblings.
	parent := filepath.Join(localAppData, "Microsoft", "Windows", "Explorer")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "ExplorerStartupLog.etl"), []byte("log"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "RecommendationsFilterList.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: localAppData,
		Categories:      []string{clean.OpportunityCategoryExplorerThumbnailCache},
	})
	if len(result.Opportunities) != 0 || len(result.Incomplete) != 0 {
		t.Fatalf("result = %#v, want empty category without whole-root fallback", result)
	}

	// Missing parent entirely is also silent empty.
	result = clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: t.TempDir(),
		Categories:      []string{clean.OpportunityCategoryExplorerThumbnailCache},
	})
	if len(result.Opportunities) != 0 || len(result.Incomplete) != 0 {
		t.Fatalf("missing parent result = %#v, want silent absence", result)
	}
}

func TestINetCache_DiscoverAllowlistedIERootsOnly(t *testing.T) {
	localAppData := t.TempDir()
	ie, lowIE := writeINetCacheAllowlistedRoots(t, localAppData)
	inetRoot := filepath.Join(localAppData, "Microsoft", "Windows", "INetCache")

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: localAppData,
		Categories:      []string{clean.OpportunityCategoryINetCache},
	})
	if len(result.Incomplete) != 0 {
		t.Fatalf("incomplete = %#v", result.Incomplete)
	}
	if len(result.Opportunities) != 2 {
		t.Fatalf("opportunities = %#v, want IE and Low\\IE only", result.Opportunities)
	}
	seen := map[string]bool{}
	for _, opp := range result.Opportunities {
		if opp.Category != clean.OpportunityCategoryINetCache {
			t.Fatalf("category = %q", opp.Category)
		}
		if opp.Path == inetRoot {
			t.Fatal("whole INetCache root must never be a candidate")
		}
		if strings.Contains(opp.Path, "SuggestedSites") ||
			strings.Contains(opp.Path, "Virtualized") ||
			strings.Contains(opp.Path, "Content.MSO") ||
			strings.Contains(opp.Path, "thumbnails") ||
			strings.Contains(opp.Path, "WebCache") ||
			strings.Contains(opp.Path, "Content.IE5") {
			t.Fatalf("non-allowlisted path became candidate: %q", opp.Path)
		}
		if opp.Bytes <= 0 {
			t.Fatalf("bytes = %d for %q", opp.Bytes, opp.Path)
		}
		seen[opp.Path] = true
	}
	if !seen[ie] || !seen[lowIE] {
		t.Fatalf("missing allowlisted roots in %#v", result.Opportunities)
	}
}

func TestINetCache_MissingAllowlistedChildrenEmptyNoWholeRootFallback(t *testing.T) {
	localAppData := t.TempDir()
	// Only non-allowlisted content under INetCache.
	inetRoot := filepath.Join(localAppData, "Microsoft", "Windows", "INetCache")
	if err := os.MkdirAll(filepath.Join(inetRoot, "Virtualized"), 0700); err != nil {
		t.Fatal(err)
	}
	suggested := filepath.Join(inetRoot, "Low", "SuggestedSites.dat")
	if err := os.MkdirAll(filepath.Dir(suggested), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suggested, []byte("sites"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inetRoot, "cache.dat"), []byte("root-file"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: localAppData,
		Categories:      []string{clean.OpportunityCategoryINetCache},
	})
	if len(result.Opportunities) != 0 || len(result.Incomplete) != 0 {
		t.Fatalf("result = %#v, want empty without whole-root fallback", result)
	}

	// Only Low parent without Low\IE must not promote Low or INetCache.
	if err := os.MkdirAll(filepath.Join(inetRoot, "Low"), 0700); err != nil {
		t.Fatal(err)
	}
	result = clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: localAppData,
		Categories:      []string{clean.OpportunityCategoryINetCache},
	})
	if len(result.Opportunities) != 0 {
		t.Fatalf("opportunities = %#v, want none without IE/Low\\IE", result.Opportunities)
	}
}

func TestINetCache_PartialAllowlistOnlyExistingChildren(t *testing.T) {
	localAppData := t.TempDir()
	ie := filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "IE")
	if err := os.MkdirAll(ie, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ie, "a.dat"), []byte("ie"), 0600); err != nil {
		t.Fatal(err)
	}
	// Non-allowlisted sibling only.
	suggested := filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "Low", "SuggestedSites.dat")
	if err := os.MkdirAll(filepath.Dir(suggested), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suggested, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: localAppData,
		Categories:      []string{clean.OpportunityCategoryINetCache},
	})
	if len(result.Opportunities) != 1 || result.Opportunities[0].Path != ie {
		t.Fatalf("opportunities = %#v, want only IE", result.Opportunities)
	}
}

func TestExplorerThumbnailAndINetCache_OtherSystemCategoriesUnaffected(t *testing.T) {
	// Regression: crash_dumps / WER / D3D / NVIDIA stay whole configured roots.
	localAppData := t.TempDir()
	crash := filepath.Join(localAppData, "CrashDumps")
	wer := filepath.Join(localAppData, "Microsoft", "Windows", "WER")
	d3d := filepath.Join(localAppData, "D3DSCache")
	nvidia := filepath.Join(localAppData, "NVIDIA", "DXCache")
	for _, root := range []string{crash, wer, d3d, nvidia} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "x.bin"), []byte("ok"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Explorer / INetCache parents exist but only non-allowlisted content.
	if err := os.MkdirAll(filepath.Join(localAppData, "Microsoft", "Windows", "Explorer"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localAppData, "Microsoft", "Windows", "Explorer", "ExplorerStartupLog.etl"), []byte("log"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "Virtualized"), 0700); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:         t.TempDir(),
		LocalAppDataDir: localAppData,
		Categories: []string{
			clean.OpportunityCategoryCrashDumps,
			clean.OpportunityCategoryWindowsErrorReporting,
			clean.OpportunityCategoryExplorerThumbnailCache,
			clean.OpportunityCategoryINetCache,
			clean.OpportunityCategoryD3DShaderCache,
			clean.OpportunityCategoryNVIDIADXCache,
		},
	})
	wantRoots := map[string]string{
		clean.OpportunityCategoryCrashDumps:            crash,
		clean.OpportunityCategoryWindowsErrorReporting: wer,
		clean.OpportunityCategoryD3DShaderCache:        d3d,
		clean.OpportunityCategoryNVIDIADXCache:         nvidia,
	}
	if len(result.Opportunities) != len(wantRoots) {
		t.Fatalf("opportunities = %#v, want only four unchanged system roots", result.Opportunities)
	}
	for _, opp := range result.Opportunities {
		want, ok := wantRoots[opp.Category]
		if !ok || opp.Path != want {
			t.Fatalf("unexpected opportunity %#v", opp)
		}
		if opp.Category == clean.OpportunityCategoryExplorerThumbnailCache ||
			opp.Category == clean.OpportunityCategoryINetCache {
			t.Fatalf("empty allowlist categories must stay empty: %#v", opp)
		}
	}
}

func TestExplorerThumbnailCache_DryRunOptInPermanentAction(t *testing.T) {
	localAppData := t.TempDir()
	_, allowlisted := writeExplorerThumbnailAllowlistedFiles(t, localAppData)

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.OpportunityCategoryExplorerThumbnailCache},
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:         t.TempDir(),
			LocalAppDataDir: localAppData,
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	// Opted-in category must dual-project to OptInCandidates only (not review opportunities).
	for _, opp := range result.Opportunities {
		if opp.Category == clean.OpportunityCategoryExplorerThumbnailCache {
			t.Fatalf("opted-in explorer_thumbnail_cache must not appear as opportunity: %#v", opp)
		}
	}
	if len(result.OptInCandidates) != len(allowlisted) {
		t.Fatalf("candidates = %#v, want %d files", result.OptInCandidates, len(allowlisted))
	}
	for _, c := range result.OptInCandidates {
		if c.Category != clean.OpportunityCategoryExplorerThumbnailCache {
			t.Fatalf("category = %q", c.Category)
		}
		if c.PlannedAction != string(clean.PlannedActionDeletePermanently) {
			t.Fatalf("planned_action = %q", c.PlannedAction)
		}
		if _, err := os.Lstat(c.Path); err != nil {
			t.Fatalf("dry-run must leave %q: %v", c.Path, err)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if !strings.Contains(body, `"planned_action":"delete_permanently"`) {
		t.Fatalf("JSON missing permanent planned_action: %s", body)
	}
}

func TestINetCache_DryRunOptInPermanentAction(t *testing.T) {
	localAppData := t.TempDir()
	ie, lowIE := writeINetCacheAllowlistedRoots(t, localAppData)

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.OpportunityCategoryINetCache},
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:         t.TempDir(),
			LocalAppDataDir: localAppData,
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	for _, opp := range result.Opportunities {
		if opp.Category == clean.OpportunityCategoryINetCache {
			t.Fatalf("opted-in inet_cache must not appear as opportunity: %#v", opp)
		}
	}
	if len(result.OptInCandidates) != 2 {
		t.Fatalf("candidates = %#v", result.OptInCandidates)
	}
	seen := map[string]bool{}
	for _, c := range result.OptInCandidates {
		if c.PlannedAction != string(clean.PlannedActionDeletePermanently) {
			t.Fatalf("planned_action = %q", c.PlannedAction)
		}
		seen[c.Path] = true
	}
	if !seen[ie] || !seen[lowIE] {
		t.Fatalf("candidates = %#v, want IE and Low\\IE", result.OptInCandidates)
	}
}

func TestExplorerThumbnailCache_ExecutePermanentlyDeletesAllowlistedFiles(t *testing.T) {
	localAppData := t.TempDir()
	parent, allowlisted := writeExplorerThumbnailAllowlistedFiles(t, localAppData)
	excluded := filepath.Join(parent, "ExplorerStartupLog.etl")

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn:                  []string{clean.OpportunityCategoryExplorerThumbnailCache},
		AllowPermanentDeletion: true,
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:         t.TempDir(),
			LocalAppDataDir: localAppData,
		},
		Rules: []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.Deleted) != len(allowlisted) {
		t.Fatalf("deleted = %#v, want %d allowlisted files", result.Deleted, len(allowlisted))
	}
	for _, item := range result.Deleted {
		if item.Action != string(clean.PlannedActionDeletePermanently) {
			t.Fatalf("action = %q", item.Action)
		}
		if item.Rule != clean.OpportunityCategoryExplorerThumbnailCache {
			t.Fatalf("rule = %q", item.Rule)
		}
	}
	for _, p := range allowlisted {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("allowlisted file %q should be permanently deleted: %v", p, err)
		}
	}
	if _, err := os.Lstat(excluded); err != nil {
		t.Fatalf("excluded sibling must remain: %v", err)
	}
	if result.Totals.PermanentlyDeletedBytes <= 0 || result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}

func TestINetCache_ExecutePermanentlyDeletesAllowlistedRoots(t *testing.T) {
	localAppData := t.TempDir()
	ie, lowIE := writeINetCacheAllowlistedRoots(t, localAppData)
	suggested := filepath.Join(localAppData, "Microsoft", "Windows", "INetCache", "Low", "SuggestedSites.dat")

	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn:                  []string{clean.OpportunityCategoryINetCache},
		AllowPermanentDeletion: true,
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:         t.TempDir(),
			LocalAppDataDir: localAppData,
		},
		Rules: []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.Deleted) != 2 {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	seen := map[string]bool{}
	for _, item := range result.Deleted {
		if item.Action != string(clean.PlannedActionDeletePermanently) {
			t.Fatalf("action = %q", item.Action)
		}
		seen[item.Path] = true
	}
	if !seen[ie] || !seen[lowIE] {
		t.Fatalf("deleted paths = %#v, want IE and Low\\IE", seen)
	}
	for _, p := range []string{ie, lowIE} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%q should be permanently deleted: %v", p, err)
		}
	}
	if _, err := os.Lstat(suggested); err != nil {
		t.Fatalf("SuggestedSites must remain: %v", err)
	}
	if result.Totals.PermanentlyDeletedBytes <= 0 || result.Totals.RecycleBinMovedBytes != 0 {
		t.Fatalf("totals = %#v", result.Totals)
	}
}

func TestExplorerThumbnailCache_ProtectionSuppressesCandidate(t *testing.T) {
	localAppData := t.TempDir()
	parent := filepath.Join(localAppData, "Microsoft", "Windows", "Explorer")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(parent, "thumbcache_256.db")
	open := filepath.Join(parent, "iconcache_32.db")
	for _, path := range []string{protected, open} {
		if err := os.WriteFile(path, []byte("db"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:     []string{clean.OpportunityCategoryExplorerThumbnailCache},
		Validator: pathsafe.NewValidator([]string{protected}),
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:         t.TempDir(),
			LocalAppDataDir: localAppData,
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != open {
		t.Fatalf("candidates = %#v, want only unprotected file", result.OptInCandidates)
	}
}

func TestINetCache_ProtectionSuppressesCandidate(t *testing.T) {
	localAppData := t.TempDir()
	ie, lowIE := writeINetCacheAllowlistedRoots(t, localAppData)

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:     []string{clean.OpportunityCategoryINetCache},
		Validator: pathsafe.NewValidator([]string{ie}),
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:         t.TempDir(),
			LocalAppDataDir: localAppData,
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != lowIE {
		t.Fatalf("candidates = %#v, want only unprotected Low\\IE", result.OptInCandidates)
	}
}
