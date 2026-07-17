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

// jetbrainsProductFixture enumerates every catalogued standard-layout product
// prefix and logical application gate (deterministic catalog order).
var jetbrainsProductFixture = []struct {
	dirName     string
	application string
	label       string
	// extraChildren are product-specific allowlisted children beyond caches/index
	// (Rider-only resharper-host).
	extraChildren []string
}{
	{dirName: "IntelliJIdea2024.1", application: clean.ApplicationIntelliJIDEA, label: "IDEA Ultimate"},
	{dirName: "IdeaIC2024.2", application: clean.ApplicationIntelliJIDEA, label: "IDEA Community"},
	{dirName: "PyCharm2024.1", application: clean.ApplicationPyCharm, label: "PyCharm Professional"},
	{dirName: "PyCharmCE2024.3", application: clean.ApplicationPyCharm, label: "PyCharm Community"},
	{dirName: "WebStorm2024.1", application: clean.ApplicationWebStorm, label: "WebStorm"},
	{dirName: "PhpStorm2024.2", application: clean.ApplicationPhpStorm, label: "PhpStorm"},
	{dirName: "RubyMine2024.1", application: clean.ApplicationRubyMine, label: "RubyMine"},
	{dirName: "CLion2024.3", application: clean.ApplicationCLion, label: "CLion"},
	{dirName: "DataGrip2024.1", application: clean.ApplicationDataGrip, label: "DataGrip"},
	{dirName: "DataSpell2024.2", application: clean.ApplicationDataSpell, label: "DataSpell"},
	{dirName: "GoLand2024.1", application: clean.ApplicationGoLand, label: "GoLand"},
	{dirName: "RustRover2024.3", application: clean.ApplicationRustRover, label: "RustRover"},
	{dirName: "Aqua2024.1", application: clean.ApplicationAqua, label: "Aqua"},
	{dirName: "MPS2024.1", application: clean.ApplicationMPS, label: "MPS"},
	{dirName: "Writerside2024.1", application: clean.ApplicationWriterside, label: "Writerside"},
	{dirName: "Rider2025.3", application: clean.ApplicationRider, label: "Rider", extraChildren: []string{"resharper-host"}},
}

// jetbrainsProcessFixture pairs every logical JetBrains application with its
// exact Windows launcher process names (private detection surface).
var jetbrainsProcessFixture = []struct {
	application string
	executables []string
}{
	{clean.ApplicationIntelliJIDEA, []string{"idea64.exe", "idea.exe"}},
	{clean.ApplicationPyCharm, []string{"pycharm64.exe", "pycharm.exe"}},
	{clean.ApplicationWebStorm, []string{"webstorm64.exe", "webstorm.exe"}},
	{clean.ApplicationPhpStorm, []string{"phpstorm64.exe", "phpstorm.exe"}},
	{clean.ApplicationRubyMine, []string{"rubymine64.exe", "rubymine.exe"}},
	{clean.ApplicationCLion, []string{"clion64.exe", "clion.exe"}},
	{clean.ApplicationDataGrip, []string{"datagrip64.exe", "datagrip.exe"}},
	{clean.ApplicationDataSpell, []string{"dataspell64.exe", "dataspell.exe"}},
	{clean.ApplicationGoLand, []string{"goland64.exe", "goland.exe"}},
	{clean.ApplicationRustRover, []string{"rustrover64.exe", "rustrover.exe"}},
	{clean.ApplicationAqua, []string{"aqua64.exe", "aqua.exe"}},
	{clean.ApplicationMPS, []string{"mps64.exe", "mps.exe"}},
	{clean.ApplicationWriterside, []string{"writerside64.exe", "writerside.exe"}},
	{clean.ApplicationRider, []string{"rider64.exe", "rider.exe"}},
}

func writeJetBrainsProductRoot(t *testing.T, jetbrainsParent, dirName string, children map[string]string) string {
	t.Helper()
	root := filepath.Join(jetbrainsParent, dirName)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for name, payload := range children {
		child := filepath.Join(root, name)
		if err := os.MkdirAll(child, 0700); err != nil {
			t.Fatal(err)
		}
		if payload != "" {
			if err := os.WriteFile(filepath.Join(child, "data.bin"), []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func jetbrainsLocalAppData(t *testing.T) (localAppData, jetbrainsParent string) {
	t.Helper()
	localAppData = t.TempDir()
	jetbrainsParent = filepath.Join(localAppData, "JetBrains")
	if err := os.MkdirAll(jetbrainsParent, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", localAppData)
	return localAppData, jetbrainsParent
}

func idleJetBrainsDetector() func(context.Context) []clean.RunningApplicationState {
	return func(context.Context) []clean.RunningApplicationState {
		states := make([]clean.RunningApplicationState, 0, len(jetbrainsProcessFixture))
		for _, fx := range jetbrainsProcessFixture {
			states = append(states, clean.RunningApplicationState{
				Application: fx.application,
				State:       clean.RunningApplicationStateIdle,
			})
		}
		return states
	}
}

func TestJetBrainsIDECaches_CatalogAndGroupTokens(t *testing.T) {
	summary, ok := clean.CanonicalCleanupCategoryCatalog().Summary(clean.DevCacheCategoryJetBrainsIDECaches)
	if !ok {
		t.Fatal("jetbrains-ide-caches missing from catalog")
	}
	if summary.Label != "JetBrains IDE caches" {
		t.Fatalf("label = %q", summary.Label)
	}
	if summary.ReportCategory != clean.ReportCategoryDeveloperTools {
		t.Fatalf("report category = %q", summary.ReportCategory)
	}
	if summary.Eligibility != clean.CategoryEligibilityOptIn {
		t.Fatalf("eligibility = %q", summary.Eligibility)
	}
	if summary.RunningApplicationPolicy != clean.RunningApplicationPolicyDistinctiveProcessIdle {
		t.Fatalf("running policy = %q", summary.RunningApplicationPolicy)
	}

	for _, token := range []string{
		clean.DevCacheCategoryJetBrainsIDECaches,
		"dev-caches",
		"all",
	} {
		enabled, invalid, _ := clean.NormalizedOptInSet([]string{token})
		if len(invalid) != 0 {
			t.Fatalf("%s invalid = %#v", token, invalid)
		}
		if !enabled[clean.DevCacheCategoryJetBrainsIDECaches] {
			t.Fatalf("%s did not enable jetbrains-ide-caches", token)
		}
	}

	enabled, _, _ := clean.NormalizedOptInSet([]string{clean.DevCacheCategoryJetBrainsIDECaches})
	if len(enabled) != 1 || !enabled[clean.DevCacheCategoryJetBrainsIDECaches] {
		t.Fatalf("solo opt-in = %#v", enabled)
	}
	if enabled[clean.DevCacheCategoryElectron] {
		t.Fatal("jetbrains opt-in must not enable electron-cache")
	}
}

func TestJetBrainsIDECaches_EditionPrefixesDiscoverCachesAndIndex(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)

	wantPaths := make([]string, 0, len(jetbrainsProductFixture)*3)
	for _, fx := range jetbrainsProductFixture {
		children := map[string]string{
			"caches": fx.label + "-c",
			"index":  fx.label + "-i",
			// Permanent exclusions must never become candidates.
			"LocalHistory": "history",
			"fileHistory":  "fh",
			"vcs-log":      "vcs",
			"jcef_cache":   "jcef",
			"plugins":      "plug",
			"log":          "log",
			"coverage":     "cov",
			"projects":     "proj",
			"tmp":          "tmp",
			"full-line":    "fl",
		}
		// Place resharper-host under every product so only Rider policy can select it.
		children["resharper-host"] = fx.label + "-rs"
		root := writeJetBrainsProductRoot(t, parent, fx.dirName, children)
		wantPaths = append(wantPaths,
			filepath.Join(root, "caches"),
			filepath.Join(root, "index"),
		)
		for _, extra := range fx.extraChildren {
			wantPaths = append(wantPaths, filepath.Join(root, extra))
		}
	}
	// Non-IDE / unknown roots under JetBrains parent.
	for _, decoy := range []string{
		"Toolbox", "Installations", "Transient", "Daemon", "Shared",
		"dotPeek", "ReSharper", "Fleet2024.1", "AndroidStudio2024.1",
		"IntelliJIdea2019.3", "IntelliJIdea2020", "IntelliJIdea2020.0",
		"MyIntelliJIdea2024.1", "IntelliJIdea2024.1-backup",
		"PyCharmEdu2024.1", "consentOptions", "WebIde2024.1",
	} {
		_ = writeJetBrainsProductRoot(t, parent, decoy, map[string]string{"caches": "nope"})
	}
	// Regular file decoy.
	if err := os.WriteFile(filepath.Join(parent, "IntelliJIdea2023.2"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules:                     []clean.Rule{{ID: clean.DefaultCategoryFoalOwnedTempSandboxes, DefaultEnabled: false}},
	})

	if len(result.OptInCandidates) != len(wantPaths) {
		t.Fatalf("candidates = %#v, want %d paths", result.OptInCandidates, len(wantPaths))
	}
	got := make(map[string]clean.OptInCandidate, len(result.OptInCandidates))
	for _, c := range result.OptInCandidates {
		if c.Category != clean.DevCacheCategoryJetBrainsIDECaches {
			t.Fatalf("category = %q", c.Category)
		}
		if c.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("planned action = %q, want delete_permanently", c.PlannedAction)
		}
		base := filepath.Base(c.Path)
		if base != "caches" && base != "index" && base != "resharper-host" {
			t.Fatalf("unexpected child candidate %q", c.Path)
		}
		// resharper-host only under Rider product roots.
		if base == "resharper-host" {
			product := filepath.Base(filepath.Dir(c.Path))
			if !strings.HasPrefix(strings.ToLower(product), "rider") {
				t.Fatalf("resharper-host leaked under non-Rider product: %q", c.Path)
			}
		}
		// Product-version roots and JetBrains parent must never be candidates.
		if strings.EqualFold(filepath.Base(filepath.Dir(c.Path)), "JetBrains") {
			t.Fatalf("product root leaked as candidate: %q", c.Path)
		}
		got[c.Path] = c
	}
	for _, want := range wantPaths {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing candidate %q among %#v", want, result.OptInCandidates)
		}
	}
	// Deterministic product-catalog then version then child order matches fixture.
	for i, want := range wantPaths {
		if result.OptInCandidates[i].Path != want {
			t.Fatalf("candidates[%d] = %q, want %q", i, result.OptInCandidates[i].Path, want)
		}
	}
	if result.Totals.CandidateBytes != 0 {
		t.Fatalf("default candidates must stay frozen, got %d", result.Totals.CandidateBytes)
	}
	if result.Totals.OptInReclaimableBytes == 0 {
		t.Fatal("expected non-zero opt-in reclaimable")
	}
}

func TestJetBrainsIDECaches_MissingRootSilent(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	// No JetBrains directory.
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("candidates = %#v, want silent absence", result.OptInCandidates)
	}

	t.Setenv("LOCALAPPDATA", "   ")
	result = clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) != 0 {
		t.Fatalf("blank LOCALAPPDATA candidates = %#v", result.OptInCandidates)
	}
}

func TestJetBrainsIDECaches_IndependentProductGates(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	ideaRoot := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{
		"caches": "idea",
		"index":  "idea-i",
	})
	pyRoot := writeJetBrainsProductRoot(t, parent, "PyCharm2024.1", map[string]string{
		"caches": "py",
		"index":  "py-i",
	})
	riderRoot := writeJetBrainsProductRoot(t, parent, "Rider2025.3", map[string]string{
		"caches":         "rider-c",
		"index":          "rider-i",
		"resharper-host": "rider-rs",
	})
	// Second Rider version shares the logical rider gate.
	_ = writeJetBrainsProductRoot(t, parent, "Rider2024.1", map[string]string{
		"caches": "rider-old",
	})
	ideaCaches := filepath.Join(ideaRoot, "caches")
	pyCaches := filepath.Join(pyRoot, "caches")
	riderCaches := filepath.Join(riderRoot, "caches")
	riderRS := filepath.Join(riderRoot, "resharper-host")

	t.Run("running IDEA keeps idle PyCharm and Rider", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateRunning},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationRider, State: clean.RunningApplicationStateIdle},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(ideaRoot)) {
				t.Fatalf("IDEA child leaked while running: %#v", result.OptInCandidates)
			}
		}
		foundPy, foundRider := false, false
		for _, c := range result.OptInCandidates {
			if c.Path == pyCaches || c.Path == filepath.Join(pyRoot, "index") {
				foundPy = true
			}
			if c.Path == riderCaches || c.Path == riderRS || c.Path == filepath.Join(riderRoot, "index") {
				foundRider = true
			}
		}
		if !foundPy {
			t.Fatalf("candidates = %#v, want PyCharm children", result.OptInCandidates)
		}
		if !foundRider {
			t.Fatalf("candidates = %#v, want Rider children", result.OptInCandidates)
		}
		// Scoped skip on product root; no unrelated tool states.
		foundSkip := false
		for _, s := range result.Skipped {
			if s.Path == ideaRoot && s.Rule == clean.DevCacheCategoryJetBrainsIDECaches {
				foundSkip = true
				if s.Bytes != 0 {
					t.Fatalf("skip bytes = %d, want 0", s.Bytes)
				}
			}
		}
		if !foundSkip {
			t.Fatalf("skipped = %#v, want IDEA root skip", result.Skipped)
		}
		apps := applicationIDs(result.RunningApplications)
		if len(apps) != 3 ||
			apps[0] != clean.ApplicationIntelliJIDEA ||
			apps[1] != clean.ApplicationPyCharm ||
			apps[2] != clean.ApplicationRider {
			t.Fatalf("running applications = %#v, want [intellij_idea pycharm rider]", result.RunningApplications)
		}
		for _, state := range result.RunningApplications {
			if state.Application == clean.ApplicationGo {
				t.Fatalf("unrelated go leaked: %#v", result.RunningApplications)
			}
		}
	})

	t.Run("running PyCharm keeps idle IDEA", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateRunning},
					{Application: clean.ApplicationRider, State: clean.RunningApplicationStateIdle},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		foundIdea := false
		for _, c := range result.OptInCandidates {
			if c.Path == ideaCaches || c.Path == filepath.Join(ideaRoot, "index") {
				foundIdea = true
			}
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(pyRoot)) {
				t.Fatalf("PyCharm child leaked while running: %#v", result.OptInCandidates)
			}
		}
		if !foundIdea {
			t.Fatalf("candidates = %#v, want IDEA children", result.OptInCandidates)
		}
	})

	t.Run("running Rider skips all Rider versions and keeps idle IDEA/PyCharm", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationRider, State: clean.RunningApplicationStateRunning},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.Contains(strings.ToLower(c.Path), "rider") {
				t.Fatalf("Rider child leaked while running: %#v", result.OptInCandidates)
			}
		}
		foundIdea, foundPy := false, false
		for _, c := range result.OptInCandidates {
			if c.Path == ideaCaches || c.Path == filepath.Join(ideaRoot, "index") {
				foundIdea = true
			}
			if c.Path == pyCaches || c.Path == filepath.Join(pyRoot, "index") {
				foundPy = true
			}
		}
		if !foundIdea || !foundPy {
			t.Fatalf("candidates = %#v, want IDEA/PyCharm children", result.OptInCandidates)
		}
	})

	t.Run("any IDEA edition running skips all IDEA versions", func(t *testing.T) {
		// Second IDEA edition root.
		idea2 := writeJetBrainsProductRoot(t, parent, "IdeaIC2023.3", map[string]string{"caches": "ic"})
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateRunning},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationRider, State: clean.RunningApplicationStateIdle},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.Contains(strings.ToLower(c.Path), "intellijidea") ||
				strings.Contains(strings.ToLower(c.Path), "ideaic") {
				t.Fatalf("IDEA version child survived: %#v", result.OptInCandidates)
			}
		}
		_ = idea2
	})

	t.Run("post-measurement unsafe discards only that product", func(t *testing.T) {
		call := 0
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				call++
				// Shared pre idle for all; post for Rider becomes running.
				if call == 1 {
					return []clean.RunningApplicationState{
						{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateIdle},
						{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
						{Application: clean.ApplicationRider, State: clean.RunningApplicationStateIdle},
					}
				}
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationRider, State: clean.RunningApplicationStateRunning},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.Contains(strings.ToLower(c.Path), "rider") {
				t.Fatalf("Rider child survived post-gate: %#v", result.OptInCandidates)
			}
		}
		foundIdea, foundPy := false, false
		for _, c := range result.OptInCandidates {
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(ideaRoot)) {
				foundIdea = true
			}
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(pyRoot)) {
				foundPy = true
			}
		}
		if !foundIdea || !foundPy {
			t.Fatalf("candidates = %#v, want IDEA/PyCharm retained after Rider post discard", result.OptInCandidates)
		}
	})

	t.Run("unknown product state discards only its scope", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationRider, State: clean.RunningApplicationStateUnknown, Message: "snapshot failed"},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.Contains(strings.ToLower(c.Path), "rider") {
				t.Fatalf("Rider survived unknown: %#v", result.OptInCandidates)
			}
		}
		if len(result.OptInCandidates) == 0 {
			t.Fatal("want IDEA/PyCharm candidates under unknown Rider")
		}
	})
}

func TestJetBrainsIDECaches_DefaultExecuteDoesNotResolve(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	_ = writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "x"})

	resolverCalls := 0
	detectionCalled := false
	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn: []string{},
		DevCacheRootScopeResolver: func(category string) []clean.DevCacheRootScope {
			if category == clean.DevCacheCategoryJetBrainsIDECaches {
				resolverCalls++
			}
			return nil
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			detectionCalled = true
			return nil
		},
		DiscoverReviewSuggestions: func(context.Context) []clean.ReviewSuggestion {
			t.Fatal("execute without opt-in must not run review suggestion probes")
			return nil
		},
		RecycleBinAdapter:     adapter,
		DiscoverOpportunities: noOpportunities,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if resolverCalls != 0 {
		t.Fatalf("jetbrains root resolver called %d times without opt-in", resolverCalls)
	}
	if detectionCalled {
		t.Fatal("detection must not run without opt-in")
	}
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter paths = %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d", result.Totals.OptInDeletedCount)
	}
}

func TestJetBrainsIDECaches_ExecuteFreshResolveAndHistory(t *testing.T) {
	local, parent := jetbrainsLocalAppData(t)
	previewRoot := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "old!"})

	// Dry-run sees preview layout.
	dry := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(dry.OptInCandidates) != 1 || dry.OptInCandidates[0].Path != filepath.Join(previewRoot, "caches") {
		t.Fatalf("dry-run candidates = %#v", dry.OptInCandidates)
	}

	// Change installed layout for execute: remove preview product, add new.
	if err := os.RemoveAll(previewRoot); err != nil {
		t.Fatal(err)
	}
	executeRoot := writeJetBrainsProductRoot(t, parent, "PyCharm2024.2", map[string]string{"index": "new!"})
	executeChild := filepath.Join(executeRoot, "index")

	permanent := &recordingPermanentRemover{}
	recorder := &recordingHistoryRecorder{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		PermanentRemover:          permanent,
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 1 || permanent.paths[0] != executeChild {
		t.Fatalf("permanent paths = %v, want only %q", permanent.paths, executeChild)
	}
	for _, p := range permanent.paths {
		if p == filepath.Join(previewRoot, "caches") {
			t.Fatal("execute trusted dry-run path")
		}
	}
	if execResult.Totals.OptInDeletedCount != 1 {
		t.Fatalf("opt-in deleted = %d", execResult.Totals.OptInDeletedCount)
	}
	if len(execResult.Deleted) != 1 || execResult.Deleted[0].Action != string(clean.DeletionActionDeletePermanently) {
		t.Fatalf("deleted = %#v, want delete_permanently", execResult.Deleted)
	}
	found := false
	for _, item := range recorder.items {
		if item.Path == executeChild {
			found = true
			if item.Rule != clean.DevCacheCategoryJetBrainsIDECaches {
				t.Fatalf("history rule = %q", item.Rule)
			}
			if item.Action != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("history action = %q", item.Action)
			}
		}
		if item.Path == filepath.Join(previewRoot, "caches") {
			t.Fatal("history recorded preview path")
		}
	}
	if !found {
		t.Fatalf("history items = %#v, want execute child", recorder.items)
	}
	_ = local
}

func TestJetBrainsIDECaches_Protection(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	ideaRoot := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{
		"caches": "a",
		"index":  "b",
	})
	pyRoot := writeJetBrainsProductRoot(t, parent, "PyCharm2024.1", map[string]string{
		"caches": "c",
	})
	riderRoot := writeJetBrainsProductRoot(t, parent, "Rider2025.3", map[string]string{
		"caches":         "r",
		"index":          "ri",
		"resharper-host": "rs",
	})
	ideaCaches := filepath.Join(ideaRoot, "caches")
	ideaIndex := filepath.Join(ideaRoot, "index")
	pyCaches := filepath.Join(pyRoot, "caches")
	riderCaches := filepath.Join(riderRoot, "caches")
	riderRS := filepath.Join(riderRoot, "resharper-host")

	t.Run("protect JetBrains parent suppresses all", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
			Validator:                 pathsafe.NewValidator([]string{parent}),
			DetectRunningApplications: idleJetBrainsDetector(),
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		if len(result.OptInCandidates) != 0 || result.Totals.OptInReclaimableBytes != 0 {
			t.Fatalf("protected parent leaked: %#v", result.OptInCandidates)
		}
	})

	t.Run("protect product root suppresses its children only", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
			Validator:                 pathsafe.NewValidator([]string{ideaRoot}),
			DetectRunningApplications: idleJetBrainsDetector(),
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if c.Path == ideaCaches || c.Path == ideaIndex {
				t.Fatalf("protected product child leaked: %#v", result.OptInCandidates)
			}
		}
		foundPy, foundRider := false, false
		for _, c := range result.OptInCandidates {
			if c.Path == pyCaches {
				foundPy = true
			}
			if c.Path == riderCaches || c.Path == riderRS {
				foundRider = true
			}
		}
		if !foundPy {
			t.Fatalf("sibling product suppressed incorrectly: %#v", result.OptInCandidates)
		}
		if !foundRider {
			t.Fatalf("Rider sibling suppressed incorrectly: %#v", result.OptInCandidates)
		}
	})

	t.Run("protect Rider root suppresses only Rider children", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
			Validator:                 pathsafe.NewValidator([]string{riderRoot}),
			DetectRunningApplications: idleJetBrainsDetector(),
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(riderRoot)) {
				t.Fatalf("protected Rider child leaked: %#v", result.OptInCandidates)
			}
		}
		foundIdea := false
		for _, c := range result.OptInCandidates {
			if c.Path == ideaCaches {
				foundIdea = true
			}
		}
		if !foundIdea {
			t.Fatalf("IDEA suppressed when only Rider protected: %#v", result.OptInCandidates)
		}
	})

	t.Run("protect one child keeps sibling", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
			Validator:                 pathsafe.NewValidator([]string{riderCaches}),
			DetectRunningApplications: idleJetBrainsDetector(),
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		foundRS, foundIndex := false, false
		for _, c := range result.OptInCandidates {
			if c.Path == riderCaches {
				t.Fatalf("protected Rider child leaked: %#v", result.OptInCandidates)
			}
			if c.Path == riderRS {
				foundRS = true
			}
			if c.Path == filepath.Join(riderRoot, "index") {
				foundIndex = true
			}
		}
		if !foundRS || !foundIndex {
			t.Fatalf("Rider sibling children suppressed: %#v", result.OptInCandidates)
		}
	})
}

func TestJetBrainsIDECaches_CapacityDoesNotBlockPermanent(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	root := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{
		"caches": "12345678",
	})
	child := filepath.Join(root, "caches")

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		RecycleBinCapacityProbe: func(path string) (clean.RecycleBinVolumeConfig, error) {
			return clean.RecycleBinVolumeConfig{
				Volume:       "C:",
				NukeOnDelete: false,
				MaxCapacity:  1,
				CurrentUsage: 0,
			}, nil
		},
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 1 || permanent.paths[0] != child {
		t.Fatalf("permanent paths = %v, want %q (capacity must not block permanent)", permanent.paths, child)
	}
	for _, skipped := range result.Skipped {
		if skipped.Reason.Code == "recycle_bin_capacity" {
			t.Fatalf("permanent candidate skipped for recycle capacity: %#v", skipped)
		}
	}
}

func TestJetBrainsIDECaches_Cancellation(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	_ = writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "data"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	permanent := &recordingPermanentRemover{}
	_ = executeCleanWithSafeCapacity(ctx, clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("canceled execute permanent paths = %v", permanent.paths)
	}
}

func TestJetBrainsIDECaches_ImpactNoticeAndFrozenDefaults(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	_ = writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "data"})

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	foundImpact := false
	for _, notice := range model.Notices {
		if notice.Kind == "opt_in_impact" && strings.Contains(notice.Message, "JetBrains") &&
			strings.Contains(notice.Message, "index") {
			foundImpact = true
		}
	}
	if !foundImpact {
		t.Fatalf("notices = %#v, want jetbrains index rebuild impact", model.Notices)
	}
	for _, c := range result.Candidates {
		if c.Rule == clean.DevCacheCategoryJetBrainsIDECaches || strings.Contains(c.Path, "JetBrains") {
			t.Fatalf("jetbrains path leaked into default candidates: %#v", c)
		}
	}
	if result.Totals.OptInReclaimableBytes == 0 {
		t.Fatal("expected non-zero opt-in reclaimable")
	}

	defaultResult := clean.DryRun(context.Background(), clean.Options{
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(defaultResult.OptInCandidates) != 0 {
		t.Fatalf("default dry-run opt-in candidates = %#v", defaultResult.OptInCandidates)
	}
}

func TestJetBrainsIDECaches_TUICategoryIdentifierOnly(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, cat := range model.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryJetBrainsIDECaches {
			found = true
			if cat.Selected {
				t.Fatal("jetbrains-ide-caches must start unselected")
			}
		}
	}
	if !found {
		t.Fatalf("OptInCategories missing jetbrains-ide-caches: %#v", model.OptInCategories)
	}

	selected := clean.NewPreviewReadModelForSelection(result, []string{clean.DevCacheCategoryJetBrainsIDECaches})
	for _, cat := range selected.OptInCategories {
		if cat.Identifier == clean.DevCacheCategoryJetBrainsIDECaches && !cat.Selected {
			t.Fatal("expected selected after identifier selection")
		}
	}
}

func TestJetBrainsIDECaches_EagerPreviewSafetyNote(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	_ = writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "data"})

	var terminal *clean.CategoryPreviewObservation
	err := clean.RunEagerPreview(context.Background(), clean.Options{
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	}, func(obs clean.CategoryPreviewObservation) {
		if obs.Identifier == clean.DevCacheCategoryJetBrainsIDECaches && obs.State != clean.CategoryPreviewScanning {
			cp := obs
			terminal = &cp
		}
	})
	if err != nil {
		t.Fatalf("RunEagerPreview: %v", err)
	}
	if terminal == nil {
		t.Fatal("missing jetbrains eager preview observation")
	}
	if terminal.State != clean.CategoryPreviewComplete && terminal.State != clean.CategoryPreviewPartial {
		t.Fatalf("state = %q, want complete/partial; obs=%#v", terminal.State, terminal)
	}
	if terminal.Bytes == 0 || terminal.CandidateCount == 0 {
		t.Fatalf("observation = %#v, want measured candidates", terminal)
	}
	if !strings.Contains(terminal.SafetyNote, "JetBrains") || !strings.Contains(terminal.SafetyNote, "index") {
		t.Fatalf("safety note = %q, want jetbrains index rebuild notice", terminal.SafetyNote)
	}
}

func TestJetBrainsIDECaches_PublicCatalogPathFree(t *testing.T) {
	summaries := clean.CanonicalCleanupCategoryCatalog().Summaries()
	raw, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{
		"IntelliJIdea", "IdeaIC", "PyCharmCE", "WebStorm", "PhpStorm", "RubyMine",
		"CLion", "DataGrip", "DataSpell", "GoLand", "RustRover", "Writerside",
		"idea64.exe", "pycharm64.exe", "webstorm64.exe", "phpstorm64.exe",
		"rubymine64.exe", "clion64.exe", "datagrip64.exe", "dataspell64.exe",
		"goland64.exe", "rustrover64.exe", "aqua64.exe", "mps64.exe", "writerside64.exe",
		"rider64.exe",
		"resolveRootScopes", "discoverChildren", "LocalHistory", "resharper-host",
		`JetBrains\`,
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("catalog exposes private detail %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, "jetbrains-ide-caches") {
		t.Fatalf("catalog missing public identifier: %s", encoded)
	}
}

// TestJetBrainsIDECaches_RiderMachineShapedLayout mirrors current-machine
// evidence for Rider 2025.3: allowlisted caches/index/resharper-host are
// selected while representative excluded siblings stay untouched.
func TestJetBrainsIDECaches_RiderMachineShapedLayout(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	root := writeJetBrainsProductRoot(t, parent, "Rider2025.3", map[string]string{
		"caches":         strings.Repeat("c", 64),
		"index":          strings.Repeat("i", 32),
		"resharper-host": strings.Repeat("r", 48),
		// Representative permanent exclusions / unknown siblings.
		"LocalHistory":   "history",
		"fileHistory":    "fh",
		"vcs-log":        "vcs",
		"jcef_cache":     "jcef",
		"plugins":        "plug",
		"log":            "log",
		"event-log-data": "eld",
		"coverage":       "cov",
		"projects":       "proj",
		"data-source":    "ds",
		"editor":         "ed",
		"full-line":      "fl",
		"tmp":            "tmp",
		"splash":         "sp",
		"unknown-state":  "nope",
	})
	// Non-IDE ReSharper root must never become a product root.
	_ = writeJetBrainsProductRoot(t, parent, "ReSharper", map[string]string{
		"caches":         "nope",
		"resharper-host": "nope",
	})

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	want := []string{
		filepath.Join(root, "caches"),
		filepath.Join(root, "index"),
		filepath.Join(root, "resharper-host"),
	}
	if len(result.OptInCandidates) != len(want) {
		t.Fatalf("candidates = %#v, want exactly %v", result.OptInCandidates, want)
	}
	for i, path := range want {
		if result.OptInCandidates[i].Path != path {
			t.Fatalf("candidates[%d] = %q, want %q", i, result.OptInCandidates[i].Path, path)
		}
	}
	for _, c := range result.OptInCandidates {
		if c.Path == root || strings.EqualFold(filepath.Base(filepath.Dir(c.Path)), "JetBrains") {
			t.Fatalf("parent/product root leaked: %q", c.Path)
		}
		base := filepath.Base(c.Path)
		if base != "caches" && base != "index" && base != "resharper-host" {
			t.Fatalf("excluded sibling selected: %q", c.Path)
		}
	}
}

func TestJetBrainsIDECaches_NoReviewSuggestionCommand(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	for _, suggestion := range result.ReviewSuggestions {
		if strings.Contains(strings.ToLower(suggestion.Tool), "jetbrains") ||
			strings.Contains(strings.ToLower(suggestion.Command), "invalidate") {
			t.Fatalf("unexpected jetbrains review suggestion: %#v", suggestion)
		}
	}
}

func TestJetBrainsIDECaches_ImmediateValidationOnExecute(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	root := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "data"})
	child := filepath.Join(root, "caches")

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		Validator:                 pathsafe.NewValidator([]string{child}),
		DetectRunningApplications: idleJetBrainsDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover must not delete protected path: %v", permanent.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d", result.Totals.OptInDeletedCount)
	}
}

func TestJetBrainsIDECaches_ParentAndProductRootNeverDeleted(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	root := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{
		"caches": "x",
		"index":  "y",
	})
	riderRoot := writeJetBrainsProductRoot(t, parent, "Rider2025.3", map[string]string{
		"caches":         "rc",
		"index":          "ri",
		"resharper-host": "rs",
	})
	permanent := &recordingPermanentRemover{}
	_ = executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion:    true,
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	for _, p := range permanent.paths {
		if p == parent || p == root || p == riderRoot {
			t.Fatalf("permanent remover received parent/product root %q", p)
		}
		base := filepath.Base(p)
		if base != "caches" && base != "index" && base != "resharper-host" {
			t.Fatalf("permanent path not allowlisted child: %q", p)
		}
	}
	if len(permanent.paths) != 5 {
		t.Fatalf("permanent paths = %v, want 5 children (2 IDEA + 3 Rider)", permanent.paths)
	}
}
