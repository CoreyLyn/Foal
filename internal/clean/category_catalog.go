package clean

import (
	"fmt"
	"strings"
)

type ReportCategory string

const (
	ReportCategorySystem         ReportCategory = "System"
	ReportCategoryUserEssentials ReportCategory = "User essentials"
	ReportCategoryBrowsers       ReportCategory = "Browsers"
	ReportCategoryDeveloperTools ReportCategory = "Developer tools"
	ReportCategoryProtection     ReportCategory = "Protection"
)

const DefaultCategoryFoalOwnedTempSandboxes = "foal_owned_temp_sandboxes"

type CategoryEligibility string

const (
	CategoryEligibilityDefault            CategoryEligibility = "default"
	CategoryEligibilityOptIn              CategoryEligibility = "opt-in"
	CategoryEligibilityReviewOnly         CategoryEligibility = "review-only"
	CategoryEligibilityPermissionBoundary CategoryEligibility = "permission-boundary"
)

type RunningApplicationPolicy string

const (
	RunningApplicationPolicyNotApplicable          RunningApplicationPolicy = "not-applicable"
	RunningApplicationPolicyBrowserIdleBeforeAfter RunningApplicationPolicy = "browser-idle-before-and-after-inspection"
	RunningApplicationPolicyDistinctiveProcessIdle RunningApplicationPolicy = "distinctive-process-must-be-idle"
	RunningApplicationPolicySharedRuntime          RunningApplicationPolicy = "shared-runtime-not-attributable"
)

// CleanupCategoryDefinition is the path-free policy vocabulary shared by
// Clean callers. Discovery paths and executable configuration stay private to
// the Clean package.
type CleanupCategoryDefinition struct {
	Identifier               string                   `json:"identifier"`
	Label                    string                   `json:"label"`
	ReportCategory           ReportCategory           `json:"report_category"`
	Eligibility              CategoryEligibility      `json:"eligibility"`
	Aliases                  []string                 `json:"aliases"`
	RunningApplicationPolicy RunningApplicationPolicy `json:"running_application_policy"`
}

// CleanupCategorySummary is the stable, path-free projection intended for
// read-model consumers such as the Clean TUI.
type CleanupCategorySummary struct {
	Identifier               string                   `json:"identifier"`
	Label                    string                   `json:"label"`
	ReportCategory           ReportCategory           `json:"report_category"`
	Eligibility              CategoryEligibility      `json:"eligibility"`
	RunningApplicationPolicy RunningApplicationPolicy `json:"running_application_policy"`
}

// CleanupCategoryCatalog is an immutable metadata catalog. Constructing one
// validates identifiers and aliases so lookup can never be ambiguous.
type CleanupCategoryCatalog struct {
	definitions []CleanupCategoryDefinition
	lookup      map[string]int
}

func (c CleanupCategoryCatalog) Definitions() []CleanupCategoryDefinition {
	definitions := make([]CleanupCategoryDefinition, 0, len(c.definitions))
	for _, definition := range c.definitions {
		aliases := make([]string, len(definition.Aliases))
		copy(aliases, definition.Aliases)
		definition.Aliases = aliases
		definitions = append(definitions, definition)
	}
	return definitions
}

func NewCleanupCategoryCatalog(definitions []CleanupCategoryDefinition) (CleanupCategoryCatalog, error) {
	catalog := CleanupCategoryCatalog{
		definitions: make([]CleanupCategoryDefinition, 0, len(definitions)),
		lookup:      make(map[string]int, len(definitions)),
	}
	for _, definition := range definitions {
		definition.Identifier = strings.ToLower(strings.TrimSpace(definition.Identifier))
		definition.Label = strings.TrimSpace(definition.Label)
		if definition.Identifier == "" || definition.Label == "" || definition.ReportCategory == "" ||
			definition.Eligibility == "" || definition.RunningApplicationPolicy == "" {
			return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has incomplete metadata", definition.Identifier)
		}
		if !validReportCategory(definition.ReportCategory) || !validCategoryEligibility(definition.Eligibility) ||
			!validRunningApplicationPolicy(definition.RunningApplicationPolicy) {
			return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has unsupported metadata", definition.Identifier)
		}
		if _, exists := catalog.lookup[definition.Identifier]; exists {
			return CleanupCategoryCatalog{}, fmt.Errorf("duplicate cleanup category identifier or alias %q", definition.Identifier)
		}
		aliases := make([]string, len(definition.Aliases))
		copy(aliases, definition.Aliases)
		definition.Aliases = aliases
		index := len(catalog.definitions)
		catalog.lookup[definition.Identifier] = index
		for aliasIndex, alias := range definition.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				return CleanupCategoryCatalog{}, fmt.Errorf("cleanup category %q has an empty alias", definition.Identifier)
			}
			if _, exists := catalog.lookup[alias]; exists {
				return CleanupCategoryCatalog{}, fmt.Errorf("duplicate cleanup category identifier or alias %q", alias)
			}
			definition.Aliases[aliasIndex] = alias
			catalog.lookup[alias] = index
		}
		catalog.definitions = append(catalog.definitions, definition)
	}
	return catalog, nil
}

func validReportCategory(category ReportCategory) bool {
	switch category {
	case ReportCategorySystem, ReportCategoryUserEssentials, ReportCategoryBrowsers, ReportCategoryDeveloperTools, ReportCategoryProtection:
		return true
	default:
		return false
	}
}

func validCategoryEligibility(eligibility CategoryEligibility) bool {
	switch eligibility {
	case CategoryEligibilityDefault, CategoryEligibilityOptIn, CategoryEligibilityReviewOnly, CategoryEligibilityPermissionBoundary:
		return true
	default:
		return false
	}
}

func validRunningApplicationPolicy(policy RunningApplicationPolicy) bool {
	switch policy {
	case RunningApplicationPolicyNotApplicable, RunningApplicationPolicyBrowserIdleBeforeAfter,
		RunningApplicationPolicyDistinctiveProcessIdle, RunningApplicationPolicySharedRuntime:
		return true
	default:
		return false
	}
}

func (c CleanupCategoryCatalog) Summaries() []CleanupCategorySummary {
	summaries := make([]CleanupCategorySummary, 0, len(c.definitions))
	for _, definition := range c.definitions {
		summaries = append(summaries, summaryFromDefinition(definition))
	}
	return summaries
}

func (c CleanupCategoryCatalog) Summary(identifierOrAlias string) (CleanupCategorySummary, bool) {
	index, ok := c.lookup[strings.ToLower(strings.TrimSpace(identifierOrAlias))]
	if !ok {
		return CleanupCategorySummary{}, false
	}
	return summaryFromDefinition(c.definitions[index]), true
}

func summaryFromDefinition(definition CleanupCategoryDefinition) CleanupCategorySummary {
	return CleanupCategorySummary{
		Identifier:               definition.Identifier,
		Label:                    definition.Label,
		ReportCategory:           definition.ReportCategory,
		Eligibility:              definition.Eligibility,
		RunningApplicationPolicy: definition.RunningApplicationPolicy,
	}
}

type categoryCatalogEntry struct {
	definition            CleanupCategoryDefinition
	opportunity           bool
	developerCache        bool
	fixedLocalAppDataPath []string
	runningApplications   []string
}

var canonicalCategoryEntries = []categoryCatalogEntry{
	{definition: categoryDefinition(DefaultCategoryFoalOwnedTempSandboxes, "Foal-owned temp sandboxes", ReportCategoryUserEssentials, CategoryEligibilityDefault, RunningApplicationPolicyNotApplicable)},
	{definition: categoryDefinition(OpportunityCategoryUserTemp, "User temp", ReportCategoryUserEssentials, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true},
	{definition: categoryDefinition(OpportunityCategoryCrashDumps, "Crash dumps", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"CrashDumps"}},
	{definition: categoryDefinition(OpportunityCategoryWindowsErrorReporting, "Windows Error Reporting", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "WER"}},
	{definition: categoryDefinition(OpportunityCategoryExplorerThumbnailCache, "Explorer thumbnail cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "Explorer"}},
	{definition: categoryDefinition(OpportunityCategoryINetCache, "INetCache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"Microsoft", "Windows", "INetCache"}},
	{definition: categoryDefinition(OpportunityCategoryD3DShaderCache, "D3D shader cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"D3DSCache"}},
	{definition: categoryDefinition(OpportunityCategoryNVIDIADXCache, "NVIDIA DX cache", ReportCategorySystem, CategoryEligibilityOptIn, RunningApplicationPolicyNotApplicable), opportunity: true, fixedLocalAppDataPath: []string{"NVIDIA", "DXCache"}},
	{definition: categoryDefinition(OpportunityCategoryBrowserCache, "Browser cache", ReportCategoryBrowsers, CategoryEligibilityOptIn, RunningApplicationPolicyBrowserIdleBeforeAfter), opportunity: true},
	{definition: categoryDefinition(DevCacheCategoryNPM, "npm cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime), developerCache: true},
	{definition: categoryDefinition(DevCacheCategoryGo, "Go build cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle), developerCache: true, runningApplications: []string{ApplicationGo}},
	{definition: categoryDefinition(DevCacheCategoryPip, "pip cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime), developerCache: true},
	{definition: categoryDefinition(DevCacheCategoryCargo, "Cargo cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle), developerCache: true, runningApplications: []string{ApplicationCargo}},
	{definition: categoryDefinition(DevCacheCategoryNuGet, "NuGet cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicyDistinctiveProcessIdle), developerCache: true, runningApplications: []string{ApplicationDotNet, ApplicationNuGet}},
	{definition: categoryDefinition(DevCacheCategoryCorepack, "Corepack cache", ReportCategoryDeveloperTools, CategoryEligibilityOptIn, RunningApplicationPolicySharedRuntime), developerCache: true},
	{definition: categoryDefinition("administrator_only_caches", "Administrator-only caches", ReportCategorySystem, CategoryEligibilityPermissionBoundary, RunningApplicationPolicyNotApplicable)},
}

func categoryDefinition(identifier, label string, reportCategory ReportCategory, eligibility CategoryEligibility, runningPolicy RunningApplicationPolicy) CleanupCategoryDefinition {
	return CleanupCategoryDefinition{
		Identifier:               identifier,
		Label:                    label,
		ReportCategory:           reportCategory,
		Eligibility:              eligibility,
		Aliases:                  []string{},
		RunningApplicationPolicy: runningPolicy,
	}
}

var canonicalCleanupCategoryCatalog = mustCleanupCategoryCatalog(canonicalCategoryEntries)

func mustCleanupCategoryCatalog(entries []categoryCatalogEntry) CleanupCategoryCatalog {
	definitions := make([]CleanupCategoryDefinition, 0, len(entries))
	for _, entry := range entries {
		definitions = append(definitions, entry.definition)
	}
	catalog, err := NewCleanupCategoryCatalog(definitions)
	if err != nil {
		panic(err)
	}
	return catalog
}

func CanonicalCleanupCategoryCatalog() CleanupCategoryCatalog {
	return canonicalCleanupCategoryCatalog
}

func canonicalCategoryEntry(identifier string) (categoryCatalogEntry, bool) {
	summary, ok := canonicalCleanupCategoryCatalog.Summary(identifier)
	if !ok {
		return categoryCatalogEntry{}, false
	}
	for _, entry := range canonicalCategoryEntries {
		if entry.definition.Identifier == summary.Identifier {
			return entry, true
		}
	}
	return categoryCatalogEntry{}, false
}

func selectableCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.definition.Eligibility == CategoryEligibilityOptIn {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

func opportunityCategoryIDs(includeBrowser bool) []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if !entry.opportunity || (!includeBrowser && entry.definition.Identifier == OpportunityCategoryBrowserCache) {
			continue
		}
		identifiers = append(identifiers, entry.definition.Identifier)
	}
	return identifiers
}

func developerCacheCategoryIDs() []string {
	var identifiers []string
	for _, entry := range canonicalCategoryEntries {
		if entry.developerCache {
			identifiers = append(identifiers, entry.definition.Identifier)
		}
	}
	return identifiers
}

func categoryReportGroup(identifier string) ReportCategory {
	entry, ok := canonicalCategoryEntry(identifier)
	if !ok {
		return ""
	}
	return entry.definition.ReportCategory
}
