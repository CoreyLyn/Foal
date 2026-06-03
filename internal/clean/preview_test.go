package clean_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestDryRunReportsCandidateContractWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(candidate, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			Roots:          []string{root},
		}},
	})

	if result.Status != "preview" || result.Mode != "dry_run" {
		t.Fatalf("status/mode = %q/%q, want preview/dry_run", result.Status, result.Mode)
	}
	if len(result.DefaultRuleCatalog) != 1 || !result.DefaultRuleCatalog[0].DefaultEnabled {
		t.Fatalf("default rule catalog = %#v, want one default-enabled rule", result.DefaultRuleCatalog)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want one candidate", result.Candidates)
	}
	got := result.Candidates[0]
	if got.Path != candidate || got.Bytes != 5 || got.Rule != "test_default_rule" || got.PlannedAction != "move_to_recycle_bin" {
		t.Fatalf("candidate = %#v, want path/size/rule/planned action", got)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("dry-run deleted or changed candidate: %v", err)
	}
	if result.Totals.CandidateCount != 1 || result.Totals.CandidateBytes != 5 || result.Totals.SkippedCount != 0 {
		t.Fatalf("totals = %#v, want one candidate and no skipped", result.Totals)
	}
}

func TestDryRunSkipsUnsafePathsThroughPathSafetyValidation(t *testing.T) {
	result := clean.DryRun(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:             "test_default_rule",
			Description:    "test default rule",
			DefaultEnabled: true,
			CandidatePaths: []string{`\\?\C:\Windows\System32`},
		}},
	})

	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want none", result.Candidates)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %#v, want one skipped unsafe path", result.Skipped)
	}
	skipped := result.Skipped[0]
	if skipped.Path != `\\?\C:\Windows\System32` || skipped.Rule != "test_default_rule" {
		t.Fatalf("skipped = %#v, want original path and rule", skipped)
	}
	if skipped.Reason.Code != "protected_path" || skipped.Reason.Message == "" || !skipped.Reason.Recoverable {
		t.Fatalf("reason = %#v, want recoverable protected_path", skipped.Reason)
	}
}

func TestDryRunUsesOnlyDefaultEnabledRules(t *testing.T) {
	root := t.TempDir()
	defaultPath := filepath.Join(root, "default.tmp")
	optInPath := filepath.Join(root, "opt-in.tmp")
	if err := os.WriteFile(defaultPath, []byte("default"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(optInPath, []byte("opt-in"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		Rules: []clean.Rule{
			{
				ID:             "approved_default_rule",
				Description:    "approved default rule",
				DefaultEnabled: true,
				CandidatePaths: []string{defaultPath},
			},
			{
				ID:             "future_opt_in_rule",
				Description:    "future opt-in rule",
				DefaultEnabled: false,
				CandidatePaths: []string{optInPath},
			},
		},
	})

	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want only the default-enabled rule", result.Candidates)
	}
	if result.Candidates[0].Path != defaultPath || result.Candidates[0].Rule != "approved_default_rule" {
		t.Fatalf("candidate = %#v, want default-enabled path only", result.Candidates[0])
	}
	if _, err := os.Stat(optInPath); err != nil {
		t.Fatalf("dry-run touched opt-in path: %v", err)
	}
}

func TestDryRunRootRuleHonorsCandidateNamePrefixes(t *testing.T) {
	root := t.TempDir()
	foalOwned := filepath.Join(root, "foal-owned.tmp")
	unrelated := filepath.Join(root, "vscode-cache.tmp")
	if err := os.WriteFile(foalOwned, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}

	result := clean.DryRun(context.Background(), clean.Options{
		Rules: []clean.Rule{{
			ID:                    "foal_owned_temp_sandboxes",
			Description:           "Foal-owned temporary sandbox entries",
			DefaultEnabled:        true,
			Roots:                 []string{root},
			CandidateNamePrefixes: []string{"foal-"},
		}},
	})

	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want only Foal-owned temp candidate", result.Candidates)
	}
	if result.Candidates[0].Path != foalOwned {
		t.Fatalf("candidate path = %q, want %q", result.Candidates[0].Path, foalOwned)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("dry-run touched unrelated temp path: %v", err)
	}
}
