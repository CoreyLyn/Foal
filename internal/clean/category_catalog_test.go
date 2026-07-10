package clean_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestCanonicalCleanupCategoryCatalogProvidesStableCompleteSummaries(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	summaries := catalog.Summaries()
	definitions := catalog.Definitions()

	wantIdentifiers := []string{
		"foal_owned_temp_sandboxes",
		"user_temp",
		"crash_dumps",
		"windows_error_reporting",
		"explorer_thumbnail_cache",
		"inet_cache",
		"d3d_shader_cache",
		"nvidia_dx_cache",
		"browser_cache",
		"npm-cache",
		"go-cache",
		"pip-cache",
		"cargo-cache",
		"nuget-cache",
		"corepack-cache",
		"administrator_only_caches",
	}
	gotIdentifiers := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		gotIdentifiers = append(gotIdentifiers, summary.Identifier)
		if strings.TrimSpace(summary.Label) == "" || strings.TrimSpace(string(summary.ReportCategory)) == "" ||
			strings.TrimSpace(string(summary.Eligibility)) == "" || strings.TrimSpace(string(summary.RunningApplicationPolicy)) == "" {
			t.Fatalf("incomplete category summary: %#v", summary)
		}
	}
	if !reflect.DeepEqual(gotIdentifiers, wantIdentifiers) {
		t.Fatalf("category order = %#v, want %#v", gotIdentifiers, wantIdentifiers)
	}
	if len(definitions) != len(summaries) {
		t.Fatalf("definitions/summaries = %d/%d", len(definitions), len(summaries))
	}
	for _, definition := range definitions {
		if definition.Aliases == nil {
			t.Fatalf("category %q does not expose accepted aliases", definition.Identifier)
		}
	}
	permissionBoundary, ok := catalog.Summary("administrator_only_caches")
	if !ok || permissionBoundary.ReportCategory != clean.ReportCategorySystem ||
		permissionBoundary.Eligibility != clean.CategoryEligibilityPermissionBoundary {
		t.Fatalf("permission boundary summary = %#v, %t", permissionBoundary, ok)
	}

	encoded, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"candidate_paths", "cache_path", "local_app_data_path", "roots"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("category summaries expose executable path data: %s", encoded)
		}
	}
}

func TestCategoryCatalogRejectsInvalidDefinitions(t *testing.T) {
	valid := clean.CleanupCategoryDefinition{
		Identifier:               "first",
		Label:                    "First",
		ReportCategory:           clean.ReportCategorySystem,
		Eligibility:              clean.CategoryEligibilityReviewOnly,
		Aliases:                  []string{"first-alias"},
		RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
	}

	tests := []struct {
		name        string
		definitions []clean.CleanupCategoryDefinition
	}{
		{name: "duplicate identifier", definitions: []clean.CleanupCategoryDefinition{valid, valid}},
		{name: "duplicate alias", definitions: []clean.CleanupCategoryDefinition{valid, {
			Identifier: "second", Label: "Second", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityReviewOnly, Aliases: []string{"first-alias"},
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "alias shadows identifier", definitions: []clean.CleanupCategoryDefinition{valid, {
			Identifier: "second", Label: "Second", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityReviewOnly, Aliases: []string{"first"},
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing label", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-label", ReportCategory: clean.ReportCategorySystem,
			Eligibility:              clean.CategoryEligibilityReviewOnly,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing grouping", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-group", Label: "Missing group",
			Eligibility:              clean.CategoryEligibilityReviewOnly,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing eligibility", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-eligibility", Label: "Missing eligibility", ReportCategory: clean.ReportCategorySystem,
			RunningApplicationPolicy: clean.RunningApplicationPolicyNotApplicable,
		}}},
		{name: "missing running policy", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "missing-policy", Label: "Missing policy", ReportCategory: clean.ReportCategorySystem,
			Eligibility: clean.CategoryEligibilityReviewOnly,
		}}},
		{name: "unsupported metadata", definitions: []clean.CleanupCategoryDefinition{{
			Identifier: "unsupported", Label: "Unsupported", ReportCategory: "Other",
			Eligibility: "sometimes", RunningApplicationPolicy: "best-effort",
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := clean.NewCleanupCategoryCatalog(tt.definitions); err == nil {
				t.Fatal("NewCleanupCategoryCatalog() error = nil")
			}
		})
	}
}

func TestFixedPathOpportunityUsesCanonicalCatalogVocabulary(t *testing.T) {
	catalog := clean.CanonicalCleanupCategoryCatalog()
	summary, ok := catalog.Summary(clean.OpportunityCategoryCrashDumps)
	if !ok {
		t.Fatal("crash_dumps missing from canonical catalog")
	}
	if summary.Label != "Crash dumps" || summary.ReportCategory != clean.ReportCategorySystem ||
		summary.Eligibility != clean.CategoryEligibilityOptIn ||
		summary.RunningApplicationPolicy != clean.RunningApplicationPolicyNotApplicable {
		t.Fatalf("crash_dumps summary = %#v", summary)
	}

	enabled, invalid, _ := clean.NormalizedOptInSet([]string{"CRASH_DUMPS"})
	if len(invalid) != 0 || !enabled[clean.OpportunityCategoryCrashDumps] {
		t.Fatalf("NormalizedOptInSet() = %#v, %#v; want canonical crash_dumps", enabled, invalid)
	}
}
