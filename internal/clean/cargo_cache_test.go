package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// writeCargoHomeFixture builds a synthetic CARGO_HOME with allowlisted
// regenerable roots plus non-allowlisted siblings that must never become candidates.
func writeCargoHomeFixture(t *testing.T, cargoHome string) (cacheRoot, srcRoot string) {
	t.Helper()
	cacheRoot = filepath.Join(cargoHome, "registry", "cache")
	srcRoot = filepath.Join(cargoHome, "registry", "src")
	for _, dir := range []string{
		cacheRoot,
		srcRoot,
		filepath.Join(cargoHome, "bin"),
		filepath.Join(cargoHome, "registry", "index"),
		filepath.Join(cargoHome, "git", "db"),
		filepath.Join(cargoHome, "git", "checkouts"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, "serde-1.0.0.crate"), []byte("crate"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "lib.rs"), []byte("fn main(){}"), 0600); err != nil {
		t.Fatal(err)
	}
	// Non-allowlisted content that must not surface as candidates.
	for _, path := range []string{
		filepath.Join(cargoHome, "config.toml"),
		filepath.Join(cargoHome, ".crates.toml"),
		filepath.Join(cargoHome, "bin", "cargo-tool.exe"),
		filepath.Join(cargoHome, "registry", "index", "config.json"),
		filepath.Join(cargoHome, "git", "db", "repo.git"),
		filepath.Join(cargoHome, "git", "checkouts", "repo-abc"),
	} {
		if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return cacheRoot, srcRoot
}

func TestCargoCache_MultiRootDryRunSurfacesOnlyAllowlisted(t *testing.T) {
	cargoHome := t.TempDir()
	cacheRoot, srcRoot := writeCargoHomeFixture(t, cargoHome)

	t.Setenv("CARGO_HOME", cargoHome)
	// Keep real-path resolution; inject idle cargo only.
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryCargo},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{{
				Application: clean.ApplicationCargo,
				State:       clean.RunningApplicationStateIdle,
			}}
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 2 {
		t.Fatalf("opt-in candidates = %#v, want 2 allowlisted roots", result.OptInCandidates)
	}
	got := map[string]clean.OptInCandidate{}
	for _, c := range result.OptInCandidates {
		got[filepath.Clean(c.Path)] = c
		if c.Category != clean.DevCacheCategoryCargo {
			t.Fatalf("category = %q, want cargo-cache", c.Category)
		}
		if c.PlannedAction != string(clean.DeletionActionDeletePermanently) {
			t.Fatalf("planned_action = %q, want delete_permanently", c.PlannedAction)
		}
		if c.Bytes == 0 {
			t.Fatalf("bytes = 0 for %q", c.Path)
		}
	}
	if _, ok := got[filepath.Clean(cacheRoot)]; !ok {
		t.Fatalf("missing registry/cache candidate: %#v", result.OptInCandidates)
	}
	if _, ok := got[filepath.Clean(srcRoot)]; !ok {
		t.Fatalf("missing registry/src candidate: %#v", result.OptInCandidates)
	}

	// Non-allowlisted siblings and parents must never appear.
	banned := []string{
		cargoHome,
		filepath.Join(cargoHome, "bin"),
		filepath.Join(cargoHome, "config.toml"),
		filepath.Join(cargoHome, ".crates.toml"),
		filepath.Join(cargoHome, "registry"),
		filepath.Join(cargoHome, "registry", "index"),
		filepath.Join(cargoHome, "git"),
		filepath.Join(cargoHome, "git", "db"),
		filepath.Join(cargoHome, "git", "checkouts"),
	}
	for _, c := range result.OptInCandidates {
		for _, b := range banned {
			if filepath.Clean(c.Path) == filepath.Clean(b) {
				t.Fatalf("non-allowlisted path became candidate: %q", c.Path)
			}
		}
	}
}

func TestCargoCache_MissingAllowlistedChildIsSilentAbsence(t *testing.T) {
	cargoHome := t.TempDir()
	// Only registry/src exists; registry/cache missing → one candidate, no parent fallback.
	srcRoot := filepath.Join(cargoHome, "registry", "src")
	if err := os.MkdirAll(srcRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "lib.rs"), []byte("src"), 0600); err != nil {
		t.Fatal(err)
	}
	// Parent registry exists with non-allowlisted index sibling only for cache side.
	if err := os.MkdirAll(filepath.Join(cargoHome, "registry", "index"), 0700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CARGO_HOME", cargoHome)
	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryCargo},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{{
				Application: clean.ApplicationCargo,
				State:       clean.RunningApplicationStateIdle,
			}}
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 1 {
		t.Fatalf("candidates = %#v, want only registry/src", result.OptInCandidates)
	}
	if filepath.Clean(result.OptInCandidates[0].Path) != filepath.Clean(srcRoot) {
		t.Fatalf("path = %q, want %q", result.OptInCandidates[0].Path, srcRoot)
	}
	// No incomplete diagnostics for missing allowlisted siblings.
	for _, err := range result.Errors {
		if strings.Contains(strings.ToLower(err.Message), "cache") {
			t.Fatalf("missing allowlisted child must be silent: %#v", result.Errors)
		}
	}
}

func TestCargoCache_ImpactNoticeMentionsCrateSources(t *testing.T) {
	cargoHome := t.TempDir()
	_, _ = writeCargoHomeFixture(t, cargoHome)
	t.Setenv("CARGO_HOME", cargoHome)

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryCargo},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{{
				Application: clean.ApplicationCargo,
				State:       clean.RunningApplicationStateIdle,
			}}
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})
	if len(result.OptInCandidates) == 0 {
		t.Fatalf("expected candidates for impact notice")
	}
	model := clean.NewPreviewReadModel(result)
	found := false
	for _, notice := range model.Notices {
		msg := strings.ToLower(notice.Message)
		if notice.Kind == "opt_in_impact" &&
			strings.Contains(msg, "crate") &&
			(strings.Contains(msg, "source") || strings.Contains(msg, "re-extract")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("notices = %#v, want cargo re-download/source impact", model.Notices)
	}
}

func TestCargoCache_ExecuteRequiresPermanentAuthorization(t *testing.T) {
	cargoHome := t.TempDir()
	cacheRoot, srcRoot := writeCargoHomeFixture(t, cargoHome)
	t.Setenv("CARGO_HOME", cargoHome)

	idle := func(context.Context) []clean.RunningApplicationState {
		return []clean.RunningApplicationState{{
			Application: clean.ApplicationCargo,
			State:       clean.RunningApplicationStateIdle,
		}}
	}

	t.Run("without allow-permanent skips only permanent cargo candidates", func(t *testing.T) {
		permanent := &recordingPermanentRemover{}
		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			AllowPermanentDeletion:    false,
			OptIn:                     []string{clean.DevCacheCategoryCargo},
			DetectRunningApplications: idle,
			PermanentRemover:          permanent,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "no-default",
				DefaultEnabled: false,
			}},
		})
		if len(permanent.paths) != 0 {
			t.Fatalf("permanent remover called without auth: %v", permanent.paths)
		}
		if len(result.Skipped) != 2 {
			t.Fatalf("skipped = %#v, want 2 unauthorized permanent cargo roots", result.Skipped)
		}
		for _, s := range result.Skipped {
			if s.Reason.Code != "permanent_deletion_not_authorized" {
				t.Fatalf("skip code = %q, want permanent_deletion_not_authorized", s.Reason.Code)
			}
			if s.Rule != clean.DevCacheCategoryCargo {
				t.Fatalf("skip rule = %q", s.Rule)
			}
			if s.PlannedAction != string(clean.DeletionActionDeletePermanently) {
				t.Fatalf("planned action = %q", s.PlannedAction)
			}
		}
		for _, root := range []string{cacheRoot, srcRoot} {
			if _, err := os.Lstat(root); err != nil {
				t.Fatalf("unauthorized root must remain %q: %v", root, err)
			}
		}
	})

	t.Run("with allow-permanent deletes both roots", func(t *testing.T) {
		// Rebuild fixture; previous subtest left files in place but re-isolate.
		cargoHome2 := t.TempDir()
		cache2, src2 := writeCargoHomeFixture(t, cargoHome2)
		t.Setenv("CARGO_HOME", cargoHome2)

		permanent := &recordingPermanentRemover{}
		result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
			AllowPermanentDeletion:    true,
			OptIn:                     []string{clean.DevCacheCategoryCargo},
			DetectRunningApplications: idle,
			PermanentRemover:          permanent,
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
			Rules: []clean.Rule{{
				ID:             "no-default",
				DefaultEnabled: false,
			}},
		})
		if len(permanent.paths) != 2 {
			t.Fatalf("permanent paths = %v, want both cargo roots", permanent.paths)
		}
		got := map[string]bool{}
		for _, p := range permanent.paths {
			got[filepath.Clean(p)] = true
		}
		if !got[filepath.Clean(cache2)] || !got[filepath.Clean(src2)] {
			t.Fatalf("permanent paths = %v, want %q and %q", permanent.paths, cache2, src2)
		}
		if result.Totals.OptInDeletedCount != 2 {
			t.Fatalf("OptInDeletedCount = %d, want 2", result.Totals.OptInDeletedCount)
		}
		if result.Totals.PermanentlyDeletedBytes == 0 {
			t.Fatalf("permanently_deleted_bytes = 0")
		}
	})
}

func TestCargoCache_RunningCargoFailClosed(t *testing.T) {
	cargoHome := t.TempDir()
	_, _ = writeCargoHomeFixture(t, cargoHome)
	t.Setenv("CARGO_HOME", cargoHome)

	permanent := &recordingPermanentRemover{}
	result := executeCleanWithSafeCapacity(context.Background(), clean.Options{
		AllowPermanentDeletion: true,
		OptIn:                  []string{clean.DevCacheCategoryCargo},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{{
				Application: clean.ApplicationCargo,
				State:       clean.RunningApplicationStateRunning,
			}}
		},
		PermanentRemover:          permanent,
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
		Rules: []clean.Rule{{
			ID:             "no-default",
			DefaultEnabled: false,
		}},
	})
	if len(permanent.paths) != 0 {
		t.Fatalf("permanent remover called while cargo running: %v", permanent.paths)
	}
	if result.Totals.OptInDeletedCount != 0 {
		t.Fatalf("OptInDeletedCount = %d, want 0", result.Totals.OptInDeletedCount)
	}
	// Both allowlisted roots should be gated (whole-root mode per path).
	if len(result.Skipped) != 2 {
		t.Fatalf("skipped = %#v, want 2 running skips", result.Skipped)
	}
	for _, s := range result.Skipped {
		if s.Reason.Code != "dev_tool_running" {
			t.Fatalf("skip code = %q, want dev_tool_running", s.Reason.Code)
		}
	}
}
