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

// jetbrainsProductFixture enumerates edition prefixes and logical product gates
// for the #208 baseline catalog (IntelliJ IDEA + PyCharm).
var jetbrainsProductFixture = []struct {
	dirName     string
	application string
	label       string
}{
	{dirName: "IntelliJIdea2024.1", application: clean.ApplicationIntelliJIDEA, label: "IDEA Ultimate"},
	{dirName: "IdeaIC2024.2", application: clean.ApplicationIntelliJIDEA, label: "IDEA Community"},
	{dirName: "PyCharm2024.1", application: clean.ApplicationPyCharm, label: "PyCharm Professional"},
	{dirName: "PyCharmCE2024.3", application: clean.ApplicationPyCharm, label: "PyCharm Community"},
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
		return []clean.RunningApplicationState{
			{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateIdle},
			{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
		}
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

	wantPaths := make([]string, 0, len(jetbrainsProductFixture)*2)
	for _, fx := range jetbrainsProductFixture {
		root := writeJetBrainsProductRoot(t, parent, fx.dirName, map[string]string{
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
		})
		wantPaths = append(wantPaths,
			filepath.Join(root, "caches"),
			filepath.Join(root, "index"),
		)
	}
	// Non-IDE / unknown roots under JetBrains parent.
	for _, decoy := range []string{
		"Toolbox", "Installations", "Transient", "Daemon", "Shared",
		"dotPeek", "ReSharper", "Rider2024.1", "WebStorm2024.1",
		"IntelliJIdea2019.3", "IntelliJIdea2020", "IntelliJIdea2020.0",
		"MyIntelliJIdea2024.1", "IntelliJIdea2024.1-backup",
		"PyCharmEdu2024.1", "consentOptions",
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
	})

	if len(result.OptInCandidates) != len(wantPaths) {
		t.Fatalf("candidates = %#v, want %d paths", result.OptInCandidates, len(wantPaths))
	}
	got := make(map[string]clean.OptInCandidate, len(result.OptInCandidates))
	for _, c := range result.OptInCandidates {
		if c.Category != clean.DevCacheCategoryJetBrainsIDECaches {
			t.Fatalf("category = %q", c.Category)
		}
		if c.PlannedAction != "move_to_recycle_bin" {
			t.Fatalf("planned action = %q", c.PlannedAction)
		}
		base := filepath.Base(c.Path)
		if base != "caches" && base != "index" {
			t.Fatalf("unexpected child candidate %q", c.Path)
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
	// Deterministic product-catalog then version then child order:
	// IDEA 2024.1 caches/index, IDEA Community 2024.2 caches/index,
	// PyCharm Pro 2024.1, PyCharm CE 2024.3.
	if result.OptInCandidates[0].Path != wantPaths[0] {
		t.Fatalf("first candidate = %q, want %q", result.OptInCandidates[0].Path, wantPaths[0])
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
	ideaCaches := filepath.Join(ideaRoot, "caches")
	pyCaches := filepath.Join(pyRoot, "caches")

	t.Run("running IDEA keeps idle PyCharm", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateRunning},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
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
		foundPy := false
		for _, c := range result.OptInCandidates {
			if c.Path == pyCaches || c.Path == filepath.Join(pyRoot, "index") {
				foundPy = true
			}
		}
		if !foundPy {
			t.Fatalf("candidates = %#v, want PyCharm children", result.OptInCandidates)
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
		if len(apps) != 2 || apps[0] != clean.ApplicationIntelliJIDEA || apps[1] != clean.ApplicationPyCharm {
			t.Fatalf("running applications = %#v, want [intellij_idea pycharm]", result.RunningApplications)
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

	t.Run("any IDEA edition running skips all IDEA versions", func(t *testing.T) {
		// Second IDEA edition root.
		idea2 := writeJetBrainsProductRoot(t, parent, "IdeaIC2023.3", map[string]string{"caches": "ic"})
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateRunning},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
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
				// Shared pre idle for both; post for first product (IDEA) becomes running.
				if call == 1 {
					return []clean.RunningApplicationState{
						{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateIdle},
						{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
					}
				}
				// Subsequent posts: IDEA running, PyCharm idle.
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateRunning},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(ideaRoot)) ||
				strings.Contains(strings.ToLower(c.Path), "ideaic") {
				t.Fatalf("IDEA child survived post-gate: %#v", result.OptInCandidates)
			}
		}
		foundPy := false
		for _, c := range result.OptInCandidates {
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(pyRoot)) {
				foundPy = true
			}
		}
		if !foundPy {
			t.Fatalf("candidates = %#v, want PyCharm retained after IDEA post discard", result.OptInCandidates)
		}
	})

	t.Run("unknown product state discards only its scope", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn: []string{clean.DevCacheCategoryJetBrainsIDECaches},
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationIntelliJIDEA, State: clean.RunningApplicationStateUnknown, Message: "snapshot failed"},
					{Application: clean.ApplicationPyCharm, State: clean.RunningApplicationStateIdle},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		for _, c := range result.OptInCandidates {
			if strings.HasPrefix(strings.ToLower(c.Path), strings.ToLower(ideaRoot)) {
				t.Fatalf("IDEA survived unknown: %#v", result.OptInCandidates)
			}
		}
		if len(result.OptInCandidates) == 0 {
			t.Fatal("want PyCharm candidates under unknown IDEA")
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

	adapter := &recordingRecycleBinAdapter{}
	recorder := &recordingHistoryRecorder{}
	execResult := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		RecycleBinAdapter:         adapter,
		HistoryRecorder:           recorder,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(adapter.paths) != 1 || adapter.paths[0] != executeChild {
		t.Fatalf("adapter paths = %v, want only %q", adapter.paths, executeChild)
	}
	for _, p := range adapter.paths {
		if p == filepath.Join(previewRoot, "caches") {
			t.Fatal("execute trusted dry-run path")
		}
	}
	if execResult.Totals.OptInDeletedCount != 1 {
		t.Fatalf("opt-in deleted = %d", execResult.Totals.OptInDeletedCount)
	}
	found := false
	for _, item := range recorder.items {
		if item.Path == executeChild {
			found = true
			if item.Rule != clean.DevCacheCategoryJetBrainsIDECaches {
				t.Fatalf("history rule = %q", item.Rule)
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
	ideaCaches := filepath.Join(ideaRoot, "caches")
	ideaIndex := filepath.Join(ideaRoot, "index")
	pyCaches := filepath.Join(pyRoot, "caches")

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
		foundPy := false
		for _, c := range result.OptInCandidates {
			if c.Path == pyCaches {
				foundPy = true
			}
		}
		if !foundPy {
			t.Fatalf("sibling product suppressed incorrectly: %#v", result.OptInCandidates)
		}
	})

	t.Run("protect one child keeps sibling", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
			Validator:                 pathsafe.NewValidator([]string{ideaCaches}),
			DetectRunningApplications: idleJetBrainsDetector(),
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})
		foundIndex := false
		for _, c := range result.OptInCandidates {
			if c.Path == ideaCaches {
				t.Fatalf("protected child leaked: %#v", result.OptInCandidates)
			}
			if c.Path == ideaIndex {
				foundIndex = true
			}
		}
		if !foundIndex {
			t.Fatalf("sibling child suppressed: %#v", result.OptInCandidates)
		}
	})
}

func TestJetBrainsIDECaches_CapacityPreCheck(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	root := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{
		"caches": "12345678",
	})
	child := filepath.Join(root, "caches")

	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		RecycleBinAdapter:         adapter,
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
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter must not run when capacity insufficient: %v", adapter.paths)
	}
	found := false
	for _, skipped := range result.Skipped {
		if skipped.Path == child && skipped.Reason.Code == "recycle_bin_capacity" {
			found = true
		}
	}
	if !found {
		t.Fatalf("skipped = %#v, want recycle_bin_capacity for %q", result.Skipped, child)
	}
}

func TestJetBrainsIDECaches_Cancellation(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	_ = writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{"caches": "data"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := &recordingRecycleBinAdapter{}
	_ = executeCleanWithSafeCapacity(ctx, clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		RecycleBinAdapter:         adapter,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(adapter.paths) != 0 {
		t.Fatalf("canceled execute adapter paths = %v", adapter.paths)
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
		"IntelliJIdea", "IdeaIC", "PyCharmCE", "idea64.exe", "pycharm64.exe",
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

	adapter := &recordingRecycleBinAdapter{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		Validator:                 pathsafe.NewValidator([]string{child}),
		DetectRunningApplications: idleJetBrainsDetector(),
		RecycleBinAdapter:         adapter,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	if len(adapter.paths) != 0 {
		t.Fatalf("adapter must not delete protected path: %v", adapter.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("opt-in deleted = %d", result.Totals.OptInDeletedCount)
	}
}

func TestJetBrainsIDECaches_ParentAndProductRootNeverRecycled(t *testing.T) {
	_, parent := jetbrainsLocalAppData(t)
	root := writeJetBrainsProductRoot(t, parent, "IntelliJIdea2024.1", map[string]string{
		"caches": "x",
		"index":  "y",
	})
	adapter := &recordingRecycleBinAdapter{}
	_ = executeCleanWithSafeCapacity(context.Background(), clean.Options{
		OptIn:                     []string{clean.DevCacheCategoryJetBrainsIDECaches},
		DetectRunningApplications: idleJetBrainsDetector(),
		RecycleBinAdapter:         adapter,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "test_rule",
			DefaultEnabled: false,
		}},
	})
	for _, p := range adapter.paths {
		if p == parent || p == root {
			t.Fatalf("adapter received parent/product root %q", p)
		}
		base := filepath.Base(p)
		if base != "caches" && base != "index" {
			t.Fatalf("adapter path not allowlisted child: %q", p)
		}
	}
	if len(adapter.paths) != 2 {
		t.Fatalf("adapter paths = %v, want 2 children", adapter.paths)
	}
}
