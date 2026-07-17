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

// Research fixture layouts from docs/research/amd-intel-gpu-shader-caches.md (#234/#238).
// No live GPU required: paths are synthetic under temp Local / LocalLow bases.

func writeAMDAllowlistedRoots(t *testing.T, localAppData string) map[string]string {
	t.Helper()
	roots := map[string]string{
		"DxCache":  filepath.Join(localAppData, "AMD", "DxCache"),
		"DxcCache": filepath.Join(localAppData, "AMD", "DxcCache"),
		"Dx9Cache": filepath.Join(localAppData, "AMD", "Dx9Cache"),
		"OglCache": filepath.Join(localAppData, "AMD", "OglCache"),
		"VkCache":  filepath.Join(localAppData, "AMD", "VkCache"),
	}
	payloads := map[string]string{
		"DxCache":  "example.0.parc",
		"DxcCache": "example.0.parc",
		"Dx9Cache": "example.bin",
		"OglCache": "example.parc",
		"VkCache":  "example.parc",
	}
	for name, root := range roots {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, payloads[name]), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Excluded siblings under parent AMD (must never become candidates).
	for _, rel := range [][]string{
		{"AMD", "RadeonSoftware", "UserSettings.json"},
		{"AMD", "cn", "note.log"},
	} {
		path := filepath.Join(append([]string{localAppData}, rel...)...)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("excluded"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return roots
}

func writeIntelShaderCacheRoot(t *testing.T, localLow string) string {
	t.Helper()
	root := filepath.Join(localLow, "Intel", "ShaderCache")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Flat content-addressed blobs (research host shape).
	for _, name := range []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name[:8]), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Excluded Local Intel siblings live under Local, not LocalLow — written by callers.
	return root
}

func TestAMDGPUShaderCaches_CatalogPermanentAndInitiallySelected(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.OpportunityCategoryAMDGPUShaderCaches)
	if !ok {
		t.Fatal("amd_gpu_shader_caches missing from catalog")
	}
	if summary.ReportCategory != clean.ReportCategorySystem {
		t.Fatalf("report = %q", summary.ReportCategory)
	}
	if summary.Eligibility != clean.CategoryEligibilityOptIn {
		t.Fatalf("eligibility = %q", summary.Eligibility)
	}
	if summary.RunningApplicationPolicy != clean.RunningApplicationPolicyNotApplicable {
		t.Fatalf("running policy = %q", summary.RunningApplicationPolicy)
	}
	if summary.PlannedAction != clean.DeletionActionDeletePermanently {
		t.Fatalf("planned_action = %q, want delete_permanently", summary.PlannedAction)
	}
	if !clean.InitiallySelectedCategory(summary) {
		t.Fatal("permanent amd_gpu_shader_caches must start selected when measurable")
	}

	for _, token := range []string{clean.OpportunityCategoryAMDGPUShaderCaches, "all"} {
		enabled, invalid, _ := clean.NormalizedOptInSet([]string{token})
		if len(invalid) != 0 {
			t.Fatalf("%s invalid = %#v", token, invalid)
		}
		if !enabled[clean.OpportunityCategoryAMDGPUShaderCaches] {
			t.Fatalf("%s did not enable amd_gpu_shader_caches", token)
		}
	}
	// Separate vendor categories: opting into AMD alone must not enable Intel.
	enabled, _, _ := clean.NormalizedOptInSet([]string{clean.OpportunityCategoryAMDGPUShaderCaches})
	if enabled[clean.OpportunityCategoryIntelGPUShaderCache] {
		t.Fatal("amd opt-in must not enable intel_gpu_shader_cache")
	}
	// Not a developer-tools group token.
	enabled, _, _ = clean.NormalizedOptInSet([]string{"dev-caches"})
	if enabled[clean.OpportunityCategoryAMDGPUShaderCaches] {
		t.Fatal("dev-caches must not enable amd_gpu_shader_caches")
	}
}

func TestIntelGPUShaderCache_CatalogPermanentAndInitiallySelected(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.OpportunityCategoryIntelGPUShaderCache)
	if !ok {
		t.Fatal("intel_gpu_shader_cache missing from catalog")
	}
	if summary.PlannedAction != clean.DeletionActionDeletePermanently {
		t.Fatalf("planned_action = %q", summary.PlannedAction)
	}
	if !clean.InitiallySelectedCategory(summary) {
		t.Fatal("permanent intel_gpu_shader_cache must start selected when measurable")
	}
	if summary.ReportCategory != clean.ReportCategorySystem {
		t.Fatalf("report = %q", summary.ReportCategory)
	}
	if summary.RunningApplicationPolicy != clean.RunningApplicationPolicyNotApplicable {
		t.Fatalf("running policy = %q", summary.RunningApplicationPolicy)
	}

	enabled, invalid, _ := clean.NormalizedOptInSet([]string{clean.OpportunityCategoryIntelGPUShaderCache})
	if len(invalid) != 0 || !enabled[clean.OpportunityCategoryIntelGPUShaderCache] {
		t.Fatalf("opt-in = %#v %#v", enabled, invalid)
	}
	if enabled[clean.OpportunityCategoryAMDGPUShaderCaches] {
		t.Fatal("intel opt-in must not enable amd_gpu_shader_caches")
	}
}

func TestAMDGPUShaderCaches_DiscoverAllowlistedRootsOnly(t *testing.T) {
	localAppData := t.TempDir()
	localLow := t.TempDir()
	allowlisted := writeAMDAllowlistedRoots(t, localAppData)
	localLowDx := filepath.Join(localLow, "AMD", "DxCache")
	if err := os.MkdirAll(localLowDx, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localLowDx, "example.parc"), []byte("low"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:            t.TempDir(),
		LocalAppDataDir:    localAppData,
		LocalAppDataLowDir: localLow,
		Categories:         []string{clean.OpportunityCategoryAMDGPUShaderCaches},
	})

	if len(result.Incomplete) != 0 {
		t.Fatalf("incomplete = %#v", result.Incomplete)
	}
	if len(result.Opportunities) != 6 {
		t.Fatalf("opportunities = %#v, want 5 Local AMD children + LocalLow DxCache", result.Opportunities)
	}
	seen := map[string]bool{}
	for _, opp := range result.Opportunities {
		if opp.Category != clean.OpportunityCategoryAMDGPUShaderCaches {
			t.Fatalf("category = %q", opp.Category)
		}
		if opp.Bytes <= 0 {
			t.Fatalf("bytes = %d for %q", opp.Bytes, opp.Path)
		}
		if !opp.LatestModifiedAt.IsZero() || opp.IdleDays != 0 {
			t.Fatalf("must not emit age fields: %#v", opp)
		}
		seen[opp.Path] = true
		// Parent AMD must never be a candidate.
		if opp.Path == filepath.Join(localAppData, "AMD") {
			t.Fatal("parent AMD must not be a candidate")
		}
		if strings.Contains(opp.Path, "RadeonSoftware") || strings.Contains(opp.Path, filepath.Join("AMD", "cn")) {
			t.Fatalf("excluded sibling became candidate: %q", opp.Path)
		}
	}
	for _, root := range allowlisted {
		if !seen[root] {
			t.Fatalf("missing allowlisted root %q in %#v", root, result.Opportunities)
		}
	}
	if !seen[localLowDx] {
		t.Fatalf("missing LocalLow AMD\\DxCache in %#v", result.Opportunities)
	}
}

func TestAMDGPUShaderCaches_MissingRootsSilentAbsence(t *testing.T) {
	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:            t.TempDir(),
		LocalAppDataDir:    t.TempDir(),
		LocalAppDataLowDir: t.TempDir(),
		Categories:         []string{clean.OpportunityCategoryAMDGPUShaderCaches},
	})
	if len(result.Opportunities) != 0 || len(result.Incomplete) != 0 {
		t.Fatalf("result = %#v, want silent absence", result)
	}
}

func TestAMDGPUShaderCaches_PartialAllowlistOnlyExistingChildren(t *testing.T) {
	localAppData := t.TempDir()
	// Only DxCache exists; other allowlisted children missing → one candidate.
	dx := filepath.Join(localAppData, "AMD", "DxCache")
	if err := os.MkdirAll(dx, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dx, "a.parc"), []byte("amd"), 0600); err != nil {
		t.Fatal(err)
	}
	// Non-allowlisted sibling must not produce a candidate or incomplete.
	if err := os.MkdirAll(filepath.Join(localAppData, "AMD", "RadeonSoftware"), 0700); err != nil {
		t.Fatal(err)
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:            t.TempDir(),
		LocalAppDataDir:    localAppData,
		LocalAppDataLowDir: t.TempDir(),
		Categories:         []string{clean.OpportunityCategoryAMDGPUShaderCaches},
	})
	if len(result.Opportunities) != 1 || result.Opportunities[0].Path != dx {
		t.Fatalf("opportunities = %#v, want only DxCache", result.Opportunities)
	}
	if len(result.Incomplete) != 0 {
		t.Fatalf("incomplete = %#v", result.Incomplete)
	}
}

func TestIntelGPUShaderCache_DiscoverLocalLowOnly(t *testing.T) {
	localAppData := t.TempDir()
	localLow := t.TempDir()
	intelRoot := writeIntelShaderCacheRoot(t, localLow)

	// Excluded Local\Intel tree (must never be discovered for this category).
	for _, rel := range [][]string{
		{"Intel", "IntelGraphicsSoftware", "UserSettings", "x.json"},
		{"Intel", "AGS", "data.ags"},
		{"Intel", "SUR", "QUEENCREEK", "note.bin"},
		{"Intel", "ShaderCache", "must-not-see.bin"}, // wrong base (Local, not LocalLow)
	} {
		path := filepath.Join(append([]string{localAppData}, rel...)...)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("excluded"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:            t.TempDir(),
		LocalAppDataDir:    localAppData,
		LocalAppDataLowDir: localLow,
		Categories:         []string{clean.OpportunityCategoryIntelGPUShaderCache},
	})
	if len(result.Incomplete) != 0 {
		t.Fatalf("incomplete = %#v", result.Incomplete)
	}
	if len(result.Opportunities) != 1 {
		t.Fatalf("opportunities = %#v, want single LocalLow ShaderCache root", result.Opportunities)
	}
	opp := result.Opportunities[0]
	if opp.Category != clean.OpportunityCategoryIntelGPUShaderCache || opp.Path != intelRoot {
		t.Fatalf("opportunity = %#v, want %q", opp, intelRoot)
	}
	if opp.Bytes != 16 { // two 8-byte payloads
		t.Fatalf("bytes = %d, want 16", opp.Bytes)
	}
	if strings.Contains(opp.Path, filepath.Join("AppData", "Local", "Intel")) &&
		!strings.Contains(opp.Path, "LocalLow") {
		t.Fatalf("must not discover Local Intel path: %q", opp.Path)
	}
}

func TestIntelGPUShaderCache_MissingRootSilentAbsence(t *testing.T) {
	result := clean.DiscoverOpportunities(context.Background(), clean.OpportunityDiscoveryOptions{
		TempDir:            t.TempDir(),
		LocalAppDataDir:    t.TempDir(),
		LocalAppDataLowDir: t.TempDir(),
		Categories:         []string{clean.OpportunityCategoryIntelGPUShaderCache},
	})
	if len(result.Opportunities) != 0 || len(result.Incomplete) != 0 {
		t.Fatalf("result = %#v, want silent absence", result)
	}
}

func TestAMDGPUShaderCaches_DryRunReportsPermanentWithoutAuthorization(t *testing.T) {
	localAppData := t.TempDir()
	dx := filepath.Join(localAppData, "AMD", "DxCache")
	if err := os.MkdirAll(dx, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dx, "a.parc"), []byte("amd-dx"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryAMDGPUShaderCaches},
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:            t.TempDir(),
			LocalAppDataDir:    localAppData,
			LocalAppDataLowDir: t.TempDir(),
		},
		// Avoid host package-manager tool probes and default sandbox noise.
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v", result.OptInCandidates)
	}
	c := result.OptInCandidates[0]
	if c.Category != clean.OpportunityCategoryAMDGPUShaderCaches || c.Path != dx {
		t.Fatalf("candidate = %#v", c)
	}
	if c.PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned_action = %q", c.PlannedAction)
	}
	if len(result.Deleted) != 0 {
		t.Fatalf("dry-run deleted = %#v", result.Deleted)
	}
	if _, err := os.Lstat(dx); err != nil {
		t.Fatalf("dry-run must leave root: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if !strings.Contains(body, `"planned_action":"delete_permanently"`) {
		t.Fatalf("JSON missing permanent planned_action: %s", body)
	}
	if strings.Contains(strings.ToLower(body), "secure erase") {
		t.Fatalf("must not claim secure erase: %s", body)
	}
}

func TestIntelGPUShaderCache_DryRunReportsPermanentWithoutAuthorization(t *testing.T) {
	localLow := t.TempDir()
	root := writeIntelShaderCacheRoot(t, localLow)

	result := clean.DryRun(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryIntelGPUShaderCache},
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:            t.TempDir(),
			LocalAppDataDir:    t.TempDir(),
			LocalAppDataLowDir: localLow,
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != root {
		t.Fatalf("candidates = %#v", result.OptInCandidates)
	}
	if result.OptInCandidates[0].PlannedAction != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("planned_action = %q", result.OptInCandidates[0].PlannedAction)
	}
	if _, err := os.Lstat(root); err != nil {
		t.Fatalf("dry-run must leave root: %v", err)
	}
}

func TestAMDGPUShaderCaches_ExecuteRequiresAllowPermanent(t *testing.T) {
	localAppData := t.TempDir()
	dx := filepath.Join(localAppData, "AMD", "DxCache")
	if err := os.MkdirAll(dx, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dx, "a.parc"), []byte("amd"), 0600); err != nil {
		t.Fatal(err)
	}
	recyclePath := writeTestFile(t, t.TempDir(), "foal-owned.tmp", "rb12")

	permanent := &recordingPermanentRemover{}
	recycle := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: false,
		OptIn:                  []string{clean.OpportunityCategoryAMDGPUShaderCaches},
		RecycleBinAdapter:      recycle,
		PermanentRemover:       permanent,
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			LocalAppDataDir:    localAppData,
			LocalAppDataLowDir: t.TempDir(),
		},
		Rules: []clean.Rule{{
			ID:             clean.DefaultCategoryFoalOwnedTempSandboxes,
			DefaultEnabled: true,
			CandidatePaths: []string{recyclePath},
		}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover without auth: %v", permanent.paths)
	}
	if len(recycle.paths) != 1 || recycle.paths[0] != recyclePath {
		t.Fatalf("recycle paths = %v", recycle.paths)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason.Code != "permanent_deletion_not_authorized" {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	if result.Skipped[0].Rule != clean.OpportunityCategoryAMDGPUShaderCaches {
		t.Fatalf("skip rule = %q", result.Skipped[0].Rule)
	}
	if _, err := os.Lstat(dx); err != nil {
		t.Fatalf("unauthorized path must remain: %v", err)
	}
}

func TestAMDGPUShaderCaches_ExecuteWithAllowPermanentDeletesRoots(t *testing.T) {
	localAppData := t.TempDir()
	dx := filepath.Join(localAppData, "AMD", "DxCache")
	if err := os.MkdirAll(dx, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dx, "a.parc"), []byte("amd-dx"), 0600); err != nil {
		t.Fatal(err)
	}
	ogl := filepath.Join(localAppData, "AMD", "OglCache")
	if err := os.MkdirAll(ogl, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ogl, "b.parc"), []byte("ogl"), 0600); err != nil {
		t.Fatal(err)
	}
	// Excluded sibling must survive.
	settings := filepath.Join(localAppData, "AMD", "RadeonSoftware", "UserSettings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryAMDGPUShaderCaches},
		PermanentRemover:       permanent,
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			LocalAppDataDir:    localAppData,
			LocalAppDataLowDir: t.TempDir(),
		},
		Rules: []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(permanent.paths) != 2 {
		t.Fatalf("permanent paths = %v, want DxCache and OglCache", permanent.paths)
	}
	// recordingPermanentRemover does not remove on disk; check planned outcome via Result.
	if len(result.Deleted) != 2 {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	for _, item := range result.Deleted {
		if item.Rule != clean.OpportunityCategoryAMDGPUShaderCaches {
			t.Fatalf("deleted rule = %q", item.Rule)
		}
		if item.Action != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("action = %q", item.Action)
		}
	}
	if result.Totals.PermanentlyDeletedBytes == 0 {
		t.Fatalf("permanently_deleted_bytes = 0")
	}
	if _, err := os.Lstat(settings); err != nil {
		t.Fatalf("excluded sibling must remain: %v", err)
	}
}

func TestIntelGPUShaderCache_ExecuteWithAllowPermanent(t *testing.T) {
	localLow := t.TempDir()
	root := writeIntelShaderCacheRoot(t, localLow)

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.OpportunityCategoryIntelGPUShaderCache},
		PermanentRemover:       permanent,
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			LocalAppDataDir:    t.TempDir(),
			LocalAppDataLowDir: localLow,
		},
		Rules: []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(permanent.paths) != 1 || permanent.paths[0] != root {
		t.Fatalf("permanent paths = %v, want %q", permanent.paths, root)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("deleted = %#v", result.Deleted)
	}
	if result.Deleted[0].Rule != clean.OpportunityCategoryIntelGPUShaderCache {
		t.Fatalf("rule = %q", result.Deleted[0].Rule)
	}
}

func TestAMDGPUShaderCaches_ProtectionSuppressesCandidate(t *testing.T) {
	localAppData := t.TempDir()
	dx := filepath.Join(localAppData, "AMD", "DxCache")
	if err := os.MkdirAll(dx, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dx, "a.parc"), []byte("amd"), 0600); err != nil {
		t.Fatal(err)
	}
	ogl := filepath.Join(localAppData, "AMD", "OglCache")
	if err := os.MkdirAll(ogl, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ogl, "b.parc"), []byte("ogl"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:     []string{clean.OpportunityCategoryAMDGPUShaderCaches},
		Validator: pathsafe.NewValidator([]string{dx}),
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:            t.TempDir(),
			LocalAppDataDir:    localAppData,
			LocalAppDataLowDir: t.TempDir(),
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != ogl {
		t.Fatalf("candidates = %#v, want only unprotected OglCache", result.OptInCandidates)
	}
}

func TestIntelGPUShaderCache_ProtectionSuppressesCandidate(t *testing.T) {
	localLow := t.TempDir()
	root := writeIntelShaderCacheRoot(t, localLow)

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:     []string{clean.OpportunityCategoryIntelGPUShaderCache},
		Validator: pathsafe.NewValidator([]string{root}),
		OpportunityDiscoveryOptions: clean.OpportunityDiscoveryOptions{
			TempDir:            t.TempDir(),
			LocalAppDataDir:    t.TempDir(),
			LocalAppDataLowDir: localLow,
		},
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("candidates = %#v, want protection suppression", result.OptInCandidates)
	}
}

func TestSeparateVendorCategoriesNoMegaCategory(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	if _, ok := catalog.Summary("gpu_caches"); ok {
		t.Fatal("merged gpu_caches must not exist")
	}
	if _, ok := catalog.Summary("gpu_shader_caches"); ok {
		t.Fatal("merged gpu_shader_caches must not exist")
	}
	if _, ok := catalog.Summary(clean.OpportunityCategoryAMDGPUShaderCaches); !ok {
		t.Fatal("amd_gpu_shader_caches required")
	}
	if _, ok := catalog.Summary(clean.OpportunityCategoryIntelGPUShaderCache); !ok {
		t.Fatal("intel_gpu_shader_cache required")
	}
}
