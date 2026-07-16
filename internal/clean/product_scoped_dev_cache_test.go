package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

// productScopedStructuredDiscoverer returns one structured child named "cache"
// under each product root for the npm-cache injection vehicle. Public category
// remains npm-cache; product identities live only on injected root scopes.
func productScopedStructuredDiscoverer() clean.DevCacheChildDiscoverer {
	return func(_ context.Context, category, root string) ([]string, bool) {
		if category != clean.DevCacheCategoryNPM {
			return nil, false
		}
		return []string{filepath.Join(root, "cache")}, true
	}
}

func writeProductScopedRoot(t *testing.T, parent, name, payload string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	child := filepath.Join(root, "cache")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "data.bin"), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestProductScopedDevCacheRoots_IndependentGatingThroughSharedClean exercises
// two injected root scopes under one category: one product running (or becoming
// unsafe after measurement) is discarded; the other stays reclaimable.
func TestProductScopedDevCacheRoots_IndependentGatingThroughSharedClean(t *testing.T) {
	base := t.TempDir()
	rootGo := writeProductScopedRoot(t, base, "product-go", "go!!")
	rootCargo := writeProductScopedRoot(t, base, "product-cargo", "cargo!")
	childGo := filepath.Join(rootGo, "cache")
	childCargo := filepath.Join(rootCargo, "cache")

	scopes := func(string) []clean.DevCacheRootScope {
		return []clean.DevCacheRootScope{
			{Path: rootGo, Application: clean.ApplicationGo},
			{Path: rootCargo, Application: clean.ApplicationCargo},
		}
	}

	t.Run("running product discards only its scope", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                   []string{clean.DevCacheCategoryNPM},
			DevCacheRootScopeResolver: scopes,
			DevCacheChildDiscoverer: productScopedStructuredDiscoverer(),
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationGo, State: clean.RunningApplicationStateIdle},
					{Application: clean.ApplicationCargo, State: clean.RunningApplicationStateRunning},
					// Noise must not appear in RunningApplications projection.
					{Application: clean.ApplicationBun, State: clean.RunningApplicationStateRunning},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 1 {
			t.Fatalf("opt-in candidates = %#v, want only go product child", result.OptInCandidates)
		}
		if result.OptInCandidates[0].Path != childGo {
			t.Fatalf("candidate path = %q, want %q", result.OptInCandidates[0].Path, childGo)
		}
		if result.OptInCandidates[0].Category != clean.DevCacheCategoryNPM {
			t.Fatalf("category = %q, want public category %q", result.OptInCandidates[0].Category, clean.DevCacheCategoryNPM)
		}
		if result.OptInCandidates[0].Bytes != 4 {
			t.Fatalf("candidate bytes = %d, want 4", result.OptInCandidates[0].Bytes)
		}
		if result.Totals.OptInReclaimableBytes != 4 {
			t.Fatalf("opt-in reclaimable = %d, want 4", result.Totals.OptInReclaimableBytes)
		}

		if len(result.Skipped) != 1 {
			t.Fatalf("skipped = %#v, want 1 cargo scope skip", result.Skipped)
		}
		if result.Skipped[0].Path != rootCargo {
			t.Fatalf("skip path = %q, want cargo root %q", result.Skipped[0].Path, rootCargo)
		}
		if result.Skipped[0].Bytes != 0 {
			t.Fatalf("skip bytes = %d, want 0 (pre-gate)", result.Skipped[0].Bytes)
		}

		// Scoped, deduped running-application identities for products that gated.
		gotApps := applicationIDs(result.RunningApplications)
		if len(gotApps) != 2 {
			t.Fatalf("running applications = %#v, want go+cargo only", result.RunningApplications)
		}
		if gotApps[0] != clean.ApplicationGo || gotApps[1] != clean.ApplicationCargo {
			t.Fatalf("running application order/ids = %v, want [go cargo]", gotApps)
		}
		for _, state := range result.RunningApplications {
			if state.Application == clean.ApplicationCargo && state.State != clean.RunningApplicationStateRunning {
				t.Fatalf("cargo state = %q, want running", state.State)
			}
			if state.Application == clean.ApplicationGo && state.State != clean.RunningApplicationStateIdle {
				t.Fatalf("go state = %q, want idle", state.State)
			}
			if state.Application == clean.ApplicationBun {
				t.Fatalf("unrelated bun state must not project: %#v", result.RunningApplications)
			}
		}
	})

	t.Run("post-measurement unsafe discards only that scope children", func(t *testing.T) {
		// Call order: pre snapshot once, then post for go (idle), post for cargo (running).
		call := 0
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryNPM},
			DevCacheRootScopeResolver: scopes,
			DevCacheChildDiscoverer:   productScopedStructuredDiscoverer(),
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				call++
				switch call {
				case 1: // shared pre
					return []clean.RunningApplicationState{
						{Application: clean.ApplicationGo, State: clean.RunningApplicationStateIdle},
						{Application: clean.ApplicationCargo, State: clean.RunningApplicationStateIdle},
					}
				case 2: // post go — still idle
					return []clean.RunningApplicationState{
						{Application: clean.ApplicationGo, State: clean.RunningApplicationStateIdle},
						{Application: clean.ApplicationCargo, State: clean.RunningApplicationStateIdle},
					}
				default: // post cargo — becomes running; discard cargo children only
					return []clean.RunningApplicationState{
						{Application: clean.ApplicationGo, State: clean.RunningApplicationStateIdle},
						{Application: clean.ApplicationCargo, State: clean.RunningApplicationStateRunning},
					}
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != childGo {
			t.Fatalf("opt-in candidates = %#v, want only go child %q", result.OptInCandidates, childGo)
		}
		if result.OptInCandidates[0].Bytes != 4 {
			t.Fatalf("go bytes = %d, want 4 (cargo measure discarded)", result.OptInCandidates[0].Bytes)
		}
		if len(result.Skipped) != 1 || result.Skipped[0].Path != rootCargo {
			t.Fatalf("skipped = %#v, want cargo root skip", result.Skipped)
		}
		if result.Skipped[0].Bytes != 0 {
			t.Fatalf("post-discard skip bytes = %d, want 0", result.Skipped[0].Bytes)
		}
		// Cargo post supersedes pre idle without reordering first-seen go.
		var cargoState clean.RunningApplicationStatus
		for _, state := range result.RunningApplications {
			if state.Application == clean.ApplicationCargo {
				cargoState = state.State
			}
		}
		if cargoState != clean.RunningApplicationStateRunning {
			t.Fatalf("cargo latest state = %q, want running (post supersedes)", cargoState)
		}
		// cargo child must never appear as a candidate after post discard
		for _, c := range result.OptInCandidates {
			if c.Path == childCargo {
				t.Fatalf("cargo child survived post-gate: %#v", result.OptInCandidates)
			}
		}
	})

	t.Run("unknown product state discards only its scope", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryNPM},
			DevCacheRootScopeResolver: scopes,
			DevCacheChildDiscoverer:   productScopedStructuredDiscoverer(),
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationGo, State: clean.RunningApplicationStateUnknown, Message: "snapshot failed"},
					{Application: clean.ApplicationCargo, State: clean.RunningApplicationStateIdle},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != childCargo {
			t.Fatalf("opt-in candidates = %#v, want only cargo child", result.OptInCandidates)
		}
		if len(result.Skipped) != 1 || result.Skipped[0].Path != rootGo {
			t.Fatalf("skipped = %#v, want go root skip", result.Skipped)
		}
	})

	t.Run("missing required product state discards only its scope", func(t *testing.T) {
		result := clean.DryRun(context.Background(), clean.Options{
			OptIn:                     []string{clean.DevCacheCategoryNPM},
			DevCacheRootScopeResolver: scopes,
			DevCacheChildDiscoverer:   productScopedStructuredDiscoverer(),
			DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
				// Cargo absent from snapshot → fail closed for cargo only.
				return []clean.RunningApplicationState{
					{Application: clean.ApplicationGo, State: clean.RunningApplicationStateIdle},
				}
			},
			DiscoverOpportunities:     noOpportunities,
			DiscoverReviewSuggestions: noReviewSuggestions,
		})

		if len(result.OptInCandidates) != 1 || result.OptInCandidates[0].Path != childGo {
			t.Fatalf("opt-in candidates = %#v, want only go child", result.OptInCandidates)
		}
		if len(result.Skipped) != 1 || result.Skipped[0].Path != rootCargo {
			t.Fatalf("skipped = %#v, want cargo root skip", result.Skipped)
		}
	})
}

func TestProductScopedDevCacheRoots_CategoryWideDistinctiveStillWorks(t *testing.T) {
	// Empty Application on scopes keeps category-wide distinctive-process gate.
	root := t.TempDir()
	cachePath := filepath.Join(root, "go-build")
	if err := os.Mkdir(cachePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "f"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		OptIn: []string{clean.DevCacheCategoryGo},
		DevCacheRootScopeResolver: func(string) []clean.DevCacheRootScope {
			return []clean.DevCacheRootScope{{Path: cachePath}} // no Application
		},
		DetectRunningApplications: func(context.Context) []clean.RunningApplicationState {
			return []clean.RunningApplicationState{
				{Application: clean.ApplicationGo, State: clean.RunningApplicationStateRunning},
			}
		},
		DiscoverOpportunities:     noOpportunities,
		DiscoverReviewSuggestions: noReviewSuggestions,
	})

	if len(result.OptInCandidates) != 0 {
		t.Fatalf("candidates = %#v, want none under category-wide running go", result.OptInCandidates)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Path != cachePath {
		t.Fatalf("skipped = %#v, want whole-root category-wide skip", result.Skipped)
	}
}
